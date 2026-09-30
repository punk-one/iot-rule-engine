package bus

import (
	"errors"
	"fmt"
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	runtimekv "github.com/punk-one/iot-rule-engine/internal/infra/storage/runtime_kv"
)

func TestJetStreamReconnectRecoversAfterServerRestart(t *testing.T) {
	port := getFreePort(t)
	serverURL := fmt.Sprintf("nats://127.0.0.1:%d", port)
	storeDir := filepath.Join(t.TempDir(), "nats-store")

	srv := runJetStreamServer(t, port, storeDir)
	defer func() {
		if srv != nil {
			srv.Shutdown()
			srv.WaitForShutdown()
		}
	}()

	if err := Init(NATSOptions{
		URL:           serverURL,
		MaxReconnects: -1,
		ReconnectWait: 100 * time.Millisecond,
	}); err != nil {
		t.Fatalf("failed to init bus: %v", err)
	}
	defer Close()

	received := make(chan string, 8)
	if err := SubscribeAllRuleInputsManualAck("reconnect-recovery", func(msg *nats.Msg) {
		received <- string(msg.Data)
		_ = msg.Ack()
	}); err != nil {
		t.Fatalf("failed to subscribe rule inputs: %v", err)
	}

	if err := PublishRuleInput("rule-1", []byte("before-restart")); err != nil {
		t.Fatalf("failed to publish before restart: %v", err)
	}
	waitForPayload(t, received, "before-restart", 5*time.Second)

	kv := runtimekv.GetKV()
	if kv == nil {
		t.Fatal("runtime KV should be initialized")
	}
	if err := kv.PutRaw("before-restart", []byte("ok")); err != nil {
		t.Fatalf("failed to write runtime kv before restart: %v", err)
	}

	srv.Shutdown()
	srv.WaitForShutdown()
	srv = nil

	srv = runJetStreamServer(t, port, storeDir)

	if err := waitForRecoveredDelivery(received, 15*time.Second, func(payload string) error {
		return PublishRuleInput("rule-1", []byte(payload))
	}); err != nil {
		t.Fatalf("failed to observe recovered delivery after restart: %v", err)
	}

	if err := waitForCondition(10*time.Second, 100*time.Millisecond, func() error {
		kv := runtimekv.GetKV()
		if kv == nil {
			return errors.New("runtime KV is nil")
		}
		return kv.PutRaw("after-restart", []byte("ok"))
	}); err != nil {
		t.Fatalf("failed to write runtime kv after restart: %v", err)
	}
}

func runJetStreamServer(t *testing.T, port int, storeDir string) *natsserver.Server {
	t.Helper()

	opts := &natsserver.Options{
		Host:      "127.0.0.1",
		Port:      port,
		JetStream: true,
		StoreDir:  storeDir,
		NoLog:     true,
		NoSigs:    true,
	}

	srv, err := natsserver.NewServer(opts)
	if err != nil {
		t.Fatalf("failed to create nats-server: %v", err)
	}

	go srv.Start()
	if !srv.ReadyForConnections(10 * time.Second) {
		srv.Shutdown()
		t.Fatal("embedded nats-server was not ready in time")
	}

	return srv
}

func getFreePort(t *testing.T) int {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to allocate free port: %v", err)
	}
	defer listener.Close()

	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("unexpected listener addr type: %T", listener.Addr())
	}
	return addr.Port
}

func waitForPayload(t *testing.T, ch <-chan string, expected string, timeout time.Duration) {
	t.Helper()

	select {
	case got := <-ch:
		if got != expected {
			t.Fatalf("unexpected payload: got %q want %q", got, expected)
		}
	case <-time.After(timeout):
		t.Fatalf("timed out waiting for payload %q", expected)
	}
}

func waitForRecoveredDelivery(receiveCh <-chan string, timeout time.Duration, publish func(payload string) error) error {
	deadline := time.Now().Add(timeout)
	attempt := 0

	for time.Now().Before(deadline) {
		attempt++
		payload := fmt.Sprintf("after-restart-%d", attempt)
		if err := publish(payload); err != nil {
			time.Sleep(100 * time.Millisecond)
			continue
		}

		select {
		case got := <-receiveCh:
			if strings.HasPrefix(got, "after-restart-") {
				return nil
			}
			return fmt.Errorf("unexpected payload during recovery: %s", got)
		case <-time.After(300 * time.Millisecond):
		}
	}

	return errors.New("no post-restart payload was delivered before timeout")
}

func waitForCondition(timeout, interval time.Duration, fn func() error) error {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		if err := fn(); err == nil {
			return nil
		} else {
			lastErr = err
		}
		time.Sleep(interval)
	}
	if lastErr == nil {
		lastErr = errors.New("condition not satisfied")
	}
	return lastErr
}
