package bus

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	pkglog "github.com/punk-one/iot-rule-engine/pkg/log"
)

const (
	NATSModeInternal = "internal"
	NATSModeExternal = "external"
)

var (
	// busInited 标记 bus 是否已初始化
	busInited bool
	// busMu 保护 busInited 的并发访问
	busMu              sync.Mutex
	embeddedNATSServer *natsserver.Server
	activeNATSOptions  NATSOptions
)

// Init 初始化消息总线（JetStream + Runtime KV）
//
// ⚠️ 规范：MUST be called only once by Engine during process bootstrap.
// Other components (Manager, Executor, Pipeline, Plugin) MUST NOT call Init.
//
// 架构规则：
// - bus.Init / bus.Close 是 Engine 生命周期的一部分
// - 总线 ≠ 组件资源，而是进程级基础设施
// - 多次初始化会导致：多个 NATS 连接、多个 JetStreamContext、Close 顺序混乱、消息丢失
func Init(opts NATSOptions) error {
	busMu.Lock()
	defer busMu.Unlock()

	if busInited {
		return errors.New("bus.Init can only be called once by Engine")
	}

	resolved, embedded, err := prepareNATS(opts)
	if err != nil {
		return err
	}

	// 初始化 JetStream
	if err := InitJetStream(resolved); err != nil {
		closeJetStreamClient()
		shutdownEmbeddedNATS(embedded)
		return fmt.Errorf("failed to initialize JetStream: %w", err)
	}

	embeddedNATSServer = embedded
	activeNATSOptions = resolved
	busInited = true
	pkglog.Info("Bus initialized successfully in %s mode", resolved.Mode)
	return nil
}

// ActiveNATSOptions returns the resolved connection options after Init. For an
// embedded server, URL contains the actual address selected for port 0.
func ActiveNATSOptions() (NATSOptions, bool) {
	busMu.Lock()
	defer busMu.Unlock()
	return activeNATSOptions, busInited
}

// Close 关闭所有连接（优雅关闭）
//
// ⚠️ 规范：MUST be called only by Engine during shutdown.
// Other components MUST NOT call Close.
//
// 关闭顺序：
// 1. Drain() - 等待正在处理的消息完成
// 2. Close() - 关闭连接
func Close() {
	busMu.Lock()
	defer busMu.Unlock()

	if !busInited {
		// 未初始化，无需关闭
		return
	}

	closeJetStreamClient()
	shutdownEmbeddedNATS(embeddedNATSServer)
	embeddedNATSServer = nil
	activeNATSOptions = NATSOptions{}
	busInited = false
	pkglog.Info("Bus closed successfully")
}

func prepareNATS(opts NATSOptions) (NATSOptions, *natsserver.Server, error) {
	mode := strings.ToLower(strings.TrimSpace(opts.Mode))
	if mode == "" {
		// Preserve compatibility for direct bus users that historically supplied only URL.
		if strings.TrimSpace(opts.URL) != "" {
			mode = NATSModeExternal
		} else {
			mode = NATSModeInternal
		}
	}
	opts.Mode = mode

	switch mode {
	case NATSModeExternal:
		if strings.TrimSpace(opts.URL) == "" {
			return NATSOptions{}, nil, errors.New("external NATS mode requires URL")
		}
		return opts, nil, nil
	case NATSModeInternal:
		return startEmbeddedNATS(opts)
	default:
		return NATSOptions{}, nil, fmt.Errorf("unsupported NATS mode: %s", opts.Mode)
	}
}

func startEmbeddedNATS(opts NATSOptions) (NATSOptions, *natsserver.Server, error) {
	if opts.Port < 0 || opts.Port > 65535 {
		return NATSOptions{}, nil, fmt.Errorf("internal NATS port must be between 0 and 65535, got %d", opts.Port)
	}
	if strings.TrimSpace(opts.Host) == "" {
		opts.Host = "127.0.0.1"
	}

	listenPort := opts.Port
	if listenPort == 0 {
		listenPort = natsserver.RANDOM_PORT
	}
	serverOpts := &natsserver.Options{
		Host:      opts.Host,
		Port:      listenPort,
		JetStream: true,
		StoreDir:  opts.StoreDir,
		NoLog:     true,
		NoSigs:    true,
	}
	srv, err := natsserver.NewServer(serverOpts)
	if err != nil {
		return NATSOptions{}, nil, fmt.Errorf("failed to create embedded NATS server: %w", err)
	}
	go srv.Start()
	if !srv.ReadyForConnections(10 * time.Second) {
		shutdownEmbeddedNATS(srv)
		return NATSOptions{}, nil, errors.New("embedded NATS server did not become ready within 10s")
	}

	opts.URL = srv.ClientURL()
	opts.AuthMode = "none"
	opts.Username = ""
	opts.Password = ""
	opts.Token = ""
	opts.CertFile = ""
	opts.KeyFile = ""
	opts.CAFile = ""
	opts.CredsFile = ""
	pkglog.Info("Embedded NATS server started at %s", opts.URL)
	return opts, srv, nil
}

func closeJetStreamClient() {
	if globalJS != nil {
		js := globalJS
		globalJS = nil
		js.subMu.Lock()
		js.subscriptions = nil
		js.subMu.Unlock()
		// 先 Drain，等待正在处理的消息完成
		if err := js.Drain(); err != nil {
			pkglog.Error("Error draining NATS connection: %v", err)
		}
		// 然后关闭连接
		js.Close()
	}
}

func shutdownEmbeddedNATS(srv *natsserver.Server) {
	if srv == nil {
		return
	}
	srv.Shutdown()
	srv.WaitForShutdown()
}
