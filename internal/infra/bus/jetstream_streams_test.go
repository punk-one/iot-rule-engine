package bus

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/nats-io/nats.go"
)

func TestInitStreamsConfiguresSysTraceMemoryLimit(t *testing.T) {
	port := getFreePort(t)
	serverURL := fmt.Sprintf("nats://127.0.0.1:%d", port)
	storeDir := filepath.Join(t.TempDir(), "nats-store")

	srv := runJetStreamServer(t, port, storeDir)
	defer func() {
		srv.Shutdown()
		srv.WaitForShutdown()
	}()

	nc, err := nats.Connect(serverURL)
	if err != nil {
		t.Fatalf("failed to connect to nats: %v", err)
	}
	defer nc.Close()

	js, err := nc.JetStream()
	if err != nil {
		t.Fatalf("failed to get JetStream context: %v", err)
	}

	if err := initStreams(js); err != nil {
		t.Fatalf("initStreams failed: %v", err)
	}

	info, err := js.StreamInfo("SYS_TRACE")
	if err != nil {
		t.Fatalf("failed to fetch SYS_TRACE info: %v", err)
	}

	const wantMaxBytes = 256 * 1024 * 1024
	if info.Config.MaxBytes != wantMaxBytes {
		t.Fatalf("SYS_TRACE max bytes mismatch: got %d want %d", info.Config.MaxBytes, wantMaxBytes)
	}
}
