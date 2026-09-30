package bus

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/cespare/xxhash/v2"
	"github.com/nats-io/nats.go"
	runtimekv "github.com/punk-one/iot-rule-engine/internal/infra/storage/runtime_kv"
	pkglog "github.com/punk-one/iot-rule-engine/pkg/log"
)

// JetStreamClient 封装 NATS JetStream 客户端
type JetStreamClient struct {
	nc            *nats.Conn
	js            nats.JetStreamContext
	ctx           *nats.StreamConfig
	subMu         sync.Mutex
	nextSubID     uint64
	subscriptions map[uint64]*managedSubscription
}

var (
	// 全局 JetStream 客户端实例
	globalJS *JetStreamClient
	// 串行化重连后的基础设施恢复，避免并发重复建流/建 KV
	infraRecoverMu sync.Mutex
)

type managedSubscription struct {
	subject    string
	queueGroup string
	mode       string
	durable    string
	sub        *nats.Subscription
	create     func(nats.JetStreamContext) (*nats.Subscription, error)
	closed     bool
}

// SubscriptionHandle owns one recoverable subscription. Close removes it from
// the reconnect registry and unsubscribes the active NATS consumer exactly once.
type SubscriptionHandle struct {
	client *JetStreamClient
	id     uint64
	once   sync.Once
	err    error
}

func (h *SubscriptionHandle) Close() error {
	if h == nil || h.client == nil {
		return nil
	}
	h.once.Do(func() {
		h.client.subMu.Lock()
		managed := h.client.subscriptions[h.id]
		if managed != nil {
			managed.closed = true
			delete(h.client.subscriptions, h.id)
		}
		h.client.subMu.Unlock()
		if managed != nil && managed.sub != nil && managed.sub.IsValid() {
			h.err = managed.sub.Drain()
		}
	})
	return h.err
}

// NATSOptions NATS 连接配置
type NATSOptions struct {
	Mode          string
	Host          string
	Port          int
	StoreDir      string
	URL           string
	AuthMode      string // none, user, token, tls, creds
	MaxReconnects int
	ReconnectWait time.Duration
	Username      string
	Password      string
	Token         string
	CertFile      string
	KeyFile       string
	CAFile        string
	CredsFile     string
}

func buildConnOptions(opts NATSOptions) ([]nats.Option, error) {
	var natsOpts []nats.Option

	switch strings.TrimSpace(opts.AuthMode) {
	case "", "none":
	case "user":
		natsOpts = append(natsOpts, nats.UserInfo(opts.Username, opts.Password))
	case "token":
		natsOpts = append(natsOpts, nats.Token(opts.Token))
	case "tls":
		if opts.CertFile != "" && opts.KeyFile != "" {
			natsOpts = append(natsOpts, nats.ClientCert(opts.CertFile, opts.KeyFile))
		}
		if opts.CAFile != "" {
			natsOpts = append(natsOpts, nats.RootCAs(opts.CAFile))
		}
	case "creds":
		if strings.TrimSpace(opts.CredsFile) == "" {
			return nil, fmt.Errorf("nats creds auth requires creds_file")
		}
		natsOpts = append(natsOpts, nats.UserCredentials(opts.CredsFile))
	default:
		return nil, fmt.Errorf("unsupported NATS auth_mode: %s", opts.AuthMode)
	}

	if opts.MaxReconnects != 0 {
		natsOpts = append(natsOpts, nats.MaxReconnects(opts.MaxReconnects))
	}
	if opts.ReconnectWait > 0 {
		natsOpts = append(natsOpts, nats.ReconnectWait(opts.ReconnectWait))
	}

	natsOpts = append(natsOpts,
		nats.DisconnectErrHandler(func(nc *nats.Conn, err error) {
			if err != nil {
				pkglog.Warn("NATS disconnected from %s: %v", nc.ConnectedUrl(), err)
				return
			}
			pkglog.Warn("NATS disconnected from %s", nc.ConnectedUrl())
		}),
		nats.ReconnectHandler(func(nc *nats.Conn) {
			go recoverAfterReconnect(nc)
		}),
		nats.ClosedHandler(func(nc *nats.Conn) {
			if err := nc.LastError(); err != nil {
				pkglog.Error("NATS connection closed: %v", err)
				return
			}
			pkglog.Warn("NATS connection closed")
		}),
		nats.ErrorHandler(func(nc *nats.Conn, sub *nats.Subscription, err error) {
			if sub != nil {
				pkglog.Error("NATS async error on subject %s: %v", sub.Subject, err)
				return
			}
			pkglog.Error("NATS async error: %v", err)
		}),
	)

	return natsOpts, nil
}

func recoverAfterReconnect(nc *nats.Conn) {
	pkglog.Info("NATS reconnected to %s, recovering JetStream infrastructure...", nc.ConnectedUrl())

	retryDelay := 100 * time.Millisecond
	for attempt := 1; attempt <= 20; attempt++ {
		if err := recoverInfrastructure(nc); err != nil {
			pkglog.Error("Failed to recover JetStream infrastructure after reconnect (attempt %d): %v", attempt, err)
			time.Sleep(retryDelay)
			if retryDelay < 2*time.Second {
				retryDelay *= 2
			}
			continue
		}
		pkglog.Info("JetStream infrastructure recovered after reconnect")
		return
	}

	pkglog.Error("JetStream infrastructure recovery exhausted after reconnect")
}

func recoverInfrastructure(nc *nats.Conn) error {
	infraRecoverMu.Lock()
	defer infraRecoverMu.Unlock()

	js, err := nc.JetStream()
	if err != nil {
		return fmt.Errorf("failed to get JetStream context: %w", err)
	}

	if globalJS == nil {
		globalJS = &JetStreamClient{
			subscriptions: make(map[uint64]*managedSubscription),
		}
	}
	globalJS.nc = nc
	globalJS.js = js

	if err := initStreams(js); err != nil {
		return fmt.Errorf("failed to initialize streams: %w", err)
	}

	if err := runtimekv.InitKV(js); err != nil {
		return fmt.Errorf("failed to initialize runtime KV: %w", err)
	}

	if err := globalJS.recoverSubscriptions(); err != nil {
		return fmt.Errorf("failed to recover subscriptions: %w", err)
	}

	return nil
}

func (c *JetStreamClient) addSubscription(subject, queueGroup, mode string, create func(nats.JetStreamContext) (*nats.Subscription, error)) (*SubscriptionHandle, error) {
	sub, err := create(c.js)
	if err != nil {
		return nil, err
	}
	if err := waitForSubscriptionHealthy(sub, 5*time.Second, 50*time.Millisecond); err != nil {
		if sub.IsValid() {
			_ = sub.Unsubscribe()
		}
		return nil, fmt.Errorf("subscription %s is not healthy after creation: %w", subject, err)
	}
	if c.nc != nil {
		if err := c.nc.FlushTimeout(5 * time.Second); err != nil {
			if sub.IsValid() {
				_ = sub.Unsubscribe()
			}
			return nil, fmt.Errorf("subscription %s flush failed: %w", subject, err)
		}
	}

	c.subMu.Lock()
	defer c.subMu.Unlock()

	c.nextSubID++
	id := c.nextSubID
	c.subscriptions[id] = &managedSubscription{
		subject:    subject,
		queueGroup: queueGroup,
		mode:       mode,
		durable:    durableNameForSubscription(mode, queueGroup, subject),
		sub:        sub,
		create:     create,
	}

	return &SubscriptionHandle{client: c, id: id}, nil
}

func (c *JetStreamClient) recoverSubscriptions() error {
	c.subMu.Lock()
	snapshot := make([]*managedSubscription, 0, len(c.subscriptions))
	for _, managed := range c.subscriptions {
		snapshot = append(snapshot, managed)
	}
	c.subMu.Unlock()

	var failed []string
	for _, managed := range snapshot {
		if err := c.ensureSubscription(managed); err != nil {
			failed = append(failed, fmt.Sprintf("%s[%s|%s]: %v", managed.mode, managed.subject, managed.queueGroup, err))
		}
	}
	if len(failed) > 0 {
		return fmt.Errorf("%s", strings.Join(failed, "; "))
	}
	return nil
}

func (c *JetStreamClient) ensureSubscription(managed *managedSubscription) error {
	c.subMu.Lock()
	if managed.closed {
		c.subMu.Unlock()
		return nil
	}
	sub := managed.sub
	c.subMu.Unlock()

	if subscriptionHealthy(sub) {
		return nil
	}

	newSub, err := managed.create(c.js)
	if err != nil {
		return err
	}
	if err := waitForSubscriptionHealthy(newSub, 5*time.Second, 50*time.Millisecond); err != nil {
		if newSub.IsValid() {
			_ = newSub.Unsubscribe()
		}
		return fmt.Errorf("subscription %s is not healthy after recreation: %w", managed.subject, err)
	}
	if c.nc != nil {
		if err := c.nc.FlushTimeout(5 * time.Second); err != nil {
			if newSub.IsValid() {
				_ = newSub.Unsubscribe()
			}
			return fmt.Errorf("subscription %s flush failed after recreation: %w", managed.subject, err)
		}
	}

	c.subMu.Lock()
	if managed.closed {
		c.subMu.Unlock()
		if newSub.IsValid() {
			_ = newSub.Unsubscribe()
		}
		return nil
	}
	oldSub := managed.sub
	managed.sub = newSub
	c.subMu.Unlock()

	if oldSub != nil && oldSub != newSub && oldSub.IsValid() {
		_ = oldSub.Unsubscribe()
	}

	pkglog.Info("Recovered subscription %s (queue: %s, mode: %s)", managed.subject, managed.queueGroup, managed.mode)
	return nil
}

func subscriptionHealthy(sub *nats.Subscription) bool {
	if sub == nil || !sub.IsValid() {
		return false
	}
	if _, err := sub.ConsumerInfo(); err != nil {
		return false
	}
	return true
}

func waitForSubscriptionHealthy(sub *nats.Subscription, timeout, interval time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if subscriptionHealthy(sub) {
			return nil
		}
		time.Sleep(interval)
	}
	return fmt.Errorf("subscription did not become healthy within %v", timeout)
}

func durableNameForSubscription(mode, queueGroup, subject string) string {
	return fmt.Sprintf("sub_%x", xxhash.Sum64String(mode+"|"+queueGroup+"|"+subject))
}

func consumerMatches(js nats.JetStreamContext, stream, durable, subject, queueGroup string) bool {
	if js == nil || stream == "" || durable == "" {
		return false
	}

	info, err := js.ConsumerInfo(stream, durable)
	if err != nil || info == nil {
		return false
	}

	return info.Config.FilterSubject == subject && info.Config.DeliverGroup == queueGroup
}

func preferredDurableName(js nats.JetStreamContext, mode, queueGroup, subject string) string {
	hashed := durableNameForSubscription(mode, queueGroup, subject)
	if js == nil || queueGroup == "" {
		return hashed
	}

	stream, err := js.StreamNameBySubject(subject)
	if err != nil || stream == "" {
		return hashed
	}

	if consumerMatches(js, stream, queueGroup, subject, queueGroup) {
		return queueGroup
	}
	if consumerMatches(js, stream, hashed, subject, queueGroup) {
		return hashed
	}

	return hashed
}

// InitJetStream 初始化 JetStream 连接
func InitJetStream(opts NATSOptions) error {
	natsOpts, err := buildConnOptions(opts)
	if err != nil {
		return err
	}

	nc, err := nats.Connect(opts.URL, natsOpts...)
	if err != nil {
		return fmt.Errorf("failed to connect to NATS: %w", err)
	}

	if err := recoverInfrastructure(nc); err != nil {
		nc.Close()
		return err
	}

	pkglog.Info("JetStream initialized successfully")
	return nil
}

// initStreams 创建必需的 JetStream Streams
func initStreams(js nats.JetStreamContext) error {
	streams := []*nats.StreamConfig{
		{
			Name:      "DATA_INGEST",
			Subjects:  []string{"data.ingest.>"},
			Retention: nats.WorkQueuePolicy,
			Storage:   nats.FileStorage,
			MaxAge:    24 * time.Hour,
		},
		{
			Name:      "RULE_PIPE",
			Subjects:  []string{"rule.input.>"},
			Retention: nats.LimitsPolicy, // 修改为 LimitsPolicy
			Storage:   nats.FileStorage,
			MaxAge:    24 * time.Hour,
		},
		{
			Name:      "RULE_EDGE",
			Subjects:  []string{"rule.edge.>"},
			Retention: nats.LimitsPolicy, // 修改为 LimitsPolicy
			Storage:   nats.FileStorage,
			MaxAge:    24 * time.Hour,
		},
		{
			Name:      "RULE_DLQ",
			Subjects:  []string{"rule.dlq.>"},
			Retention: nats.LimitsPolicy,
			Storage:   nats.FileStorage,
			MaxAge:    7 * 24 * time.Hour,
		},
		{
			Name:      "RULE_ACK",
			Subjects:  []string{"rule.ack.>"},
			Retention: nats.LimitsPolicy,
			Storage:   nats.FileStorage,
			MaxAge:    7 * 24 * time.Hour,
		},
		{
			Name:      "SYS_TRACE",
			Subjects:  []string{"sys.trace.>"},
			Retention: nats.LimitsPolicy,
			Storage:   nats.MemoryStorage,
			MaxAge:    2 * time.Hour,
			MaxBytes:  256 * 1024 * 1024, // 256MB
		},
	}

	for _, cfg := range streams {
		// 先尝试获取 Stream，检查是否已存在
		_, err := js.StreamInfo(cfg.Name)
		if err == nil {
			// Stream 已存在，尝试更新配置
			_, updateErr := js.UpdateStream(cfg)
			if updateErr != nil {
				// 如果更新失败，只记录警告，不阻止启动
				pkglog.Warn("Stream %s already exists, but failed to update: %v", cfg.Name, updateErr)
			} else {
				pkglog.Info("Stream %s already exists, configuration updated", cfg.Name)
			}
			continue
		}

		// Stream 不存在，创建新的 Stream
		_, err = js.AddStream(cfg)
		if err != nil {
			// 检查是否是 Stream 已存在的错误（可能在其他地方创建了）
			errStr := err.Error()
			isStreamExists := errStr == "stream name already in use" ||
				errStr == "nats: stream name already in use" ||
				errStr == "nats: subjects overlap with an existing stream" ||
				errStr == "subjects overlap with an existing stream"

			if isStreamExists {
				// Stream 已存在，只记录信息
				pkglog.Info("Stream %s already exists (created elsewhere)", cfg.Name)
			} else {
				// 其他错误，返回
				return fmt.Errorf("failed to add stream %s: %w", cfg.Name, err)
			}
		} else {
			pkglog.Info("Stream %s initialized", cfg.Name)
		}
	}

	return nil
}

// GetJetStream 获取全局 JetStream 客户端
func GetJetStream() *JetStreamClient {
	return globalJS
}

// GetConn 获取底层 NATS 连接
func (c *JetStreamClient) GetConn() *nats.Conn {
	return c.nc
}

// Publish 发布消息到指定主题
func (c *JetStreamClient) Publish(subject string, msg interface{}) error {
	var data []byte
	var err error

	// 如果已经是 []byte，直接使用
	if b, ok := msg.([]byte); ok {
		data = b
	} else {
		data, err = json.Marshal(msg)
		if err != nil {
			return fmt.Errorf("failed to marshal message: %w", err)
		}
	}

	_, err = c.js.Publish(subject, data)
	if err != nil {
		return fmt.Errorf("failed to publish to %s: %w", subject, err)
	}

	pkglog.Debug("Published message to %s", subject)
	return nil
}

// SubscribeQueue 订阅主题（使用队列组实现负载均衡）
func (c *JetStreamClient) SubscribeQueue(subject string, queueGroup string, handler func([]byte) error) error {
	_, err := c.SubscribeQueueManaged(subject, queueGroup, handler)
	return err
}

func (c *JetStreamClient) SubscribeQueueManaged(subject string, queueGroup string, handler func([]byte) error) (*SubscriptionHandle, error) {
	durable := preferredDurableName(c.js, "queue", queueGroup, subject)
	handle, err := c.addSubscription(subject, queueGroup, "queue", func(js nats.JetStreamContext) (*nats.Subscription, error) {
		return js.QueueSubscribe(subject, queueGroup, func(msg *nats.Msg) {
			if err := handler(msg.Data); err != nil {
				pkglog.Error("Handler error for subject %s: %v", subject, err)
				if nakErr := msg.Nak(); nakErr != nil {
					pkglog.Error("Failed to NAK message for subject %s: %v", subject, nakErr)
				}
			} else {
				if ackErr := msg.Ack(); ackErr != nil {
					pkglog.Error("Failed to ACK message for subject %s: %v", subject, ackErr)
				}
			}
		}, nats.Durable(durable), nats.ManualAck())
	})
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe to %s: %w", subject, err)
	}

	pkglog.Info("Subscribed to %s with queue group %s", subject, queueGroup)
	return handle, nil
}

// SubscribeQueueManualAck 订阅主题（支持手动 Ack，用于异步处理）
func (c *JetStreamClient) SubscribeQueueManualAck(subject string, queueGroup string, handler func(*nats.Msg)) error {
	_, err := c.SubscribeQueueManualAckManaged(subject, queueGroup, handler)
	return err
}

func (c *JetStreamClient) SubscribeQueueManualAckManaged(subject string, queueGroup string, handler func(*nats.Msg)) (*SubscriptionHandle, error) {
	durable := preferredDurableName(c.js, "queue_manual_ack", queueGroup, subject)
	handle, err := c.addSubscription(subject, queueGroup, "queue_manual_ack", func(js nats.JetStreamContext) (*nats.Subscription, error) {
		return js.QueueSubscribe(subject, queueGroup, handler, nats.Durable(durable), nats.ManualAck())
	})
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe to %s: %w", subject, err)
	}
	pkglog.Info("Subscribed to %s with queue group %s (Manual Ack)", subject, queueGroup)
	return handle, nil
}

// Subscribe 订阅主题（不使用队列组）
func (c *JetStreamClient) Subscribe(subject string, handler func([]byte) error) error {
	_, err := c.SubscribeManaged(subject, handler)
	return err
}

func (c *JetStreamClient) SubscribeManaged(subject string, handler func([]byte) error) (*SubscriptionHandle, error) {
	durable := durableNameForSubscription("subscribe", "", subject)
	handle, err := c.addSubscription(subject, "", "subscribe", func(js nats.JetStreamContext) (*nats.Subscription, error) {
		return js.Subscribe(subject, func(msg *nats.Msg) {
			if err := handler(msg.Data); err != nil {
				pkglog.Error("Handler error for subject %s: %v", subject, err)
				if nakErr := msg.Nak(); nakErr != nil {
					pkglog.Error("Failed to NAK message for subject %s: %v", subject, nakErr)
				}
			} else {
				if ackErr := msg.Ack(); ackErr != nil {
					pkglog.Error("Failed to ACK message for subject %s: %v", subject, ackErr)
				}
			}
		}, nats.Durable(durable), nats.ManualAck())
	})
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe to %s: %w", subject, err)
	}

	pkglog.Info("Subscribed to %s", subject)
	return handle, nil
}

// Drain 优雅关闭：等待正在处理的消息完成
func (c *JetStreamClient) Drain() error {
	if c.nc != nil {
		return c.nc.Drain()
	}
	return nil
}

// Close 关闭连接
func (c *JetStreamClient) Close() {
	c.subMu.Lock()
	managed := make([]*managedSubscription, 0, len(c.subscriptions))
	for id, subscription := range c.subscriptions {
		subscription.closed = true
		managed = append(managed, subscription)
		delete(c.subscriptions, id)
	}
	c.subMu.Unlock()
	for _, subscription := range managed {
		if subscription.sub != nil && subscription.sub.IsValid() {
			_ = subscription.sub.Unsubscribe()
		}
	}
	if c.nc != nil {
		c.nc.Close()
	}
}

// StreamInfo returns stream information.
func (c *JetStreamClient) StreamInfo(stream string) (*nats.StreamInfo, error) {
	return c.js.StreamInfo(stream)
}

// GetMsg returns a raw stream message by sequence.
func (c *JetStreamClient) GetMsg(stream string, seq uint64) (*nats.RawStreamMsg, error) {
	return c.js.GetMsg(stream, seq)
}
