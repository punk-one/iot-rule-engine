package runtime_kv

import (
	"fmt"
	"net"
	"path/filepath"
	"testing"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
)

func TestInitKVUsesExistingBucketsWhenCreateReturnsStreamExists(t *testing.T) {
	globalKV = nil

	port := getFreePort(t)
	storeDir := filepath.Join(t.TempDir(), "nats-store")
	srv := runJetStreamServer(t, port, storeDir)
	defer func() {
		srv.Shutdown()
		srv.WaitForShutdown()
		globalKV = nil
	}()

	nc, err := nats.Connect(fmt.Sprintf("nats://127.0.0.1:%d", port))
	if err != nil {
		t.Fatalf("failed to connect to embedded nats-server: %v", err)
	}
	defer nc.Close()

	js, err := nc.JetStream()
	if err != nil {
		t.Fatalf("failed to get JetStream context: %v", err)
	}

	if _, err := js.CreateKeyValue(&nats.KeyValueConfig{
		Bucket:      "RUNTIME_STATE",
		Description: "existing runtime bucket with different description",
		TTL:         0,
	}); err != nil {
		t.Fatalf("failed to pre-create RUNTIME_STATE bucket: %v", err)
	}

	if _, err := js.CreateKeyValue(&nats.KeyValueConfig{
		Bucket:      "PLUGIN_REGISTRY",
		Description: "existing plugin bucket with different description",
		TTL:         0,
	}); err != nil {
		t.Fatalf("failed to pre-create PLUGIN_REGISTRY bucket: %v", err)
	}

	if err := InitKV(js); err != nil {
		t.Fatalf("InitKV should reuse existing buckets, got error: %v", err)
	}

	if globalKV == nil {
		t.Fatal("globalKV should be initialized")
	}

	if err := globalKV.PutRaw("health", []byte("ok")); err != nil {
		t.Fatalf("runtime KV should be writable after initialization: %v", err)
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
