package bus

import (
	"net/url"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestInitStartsEmbeddedNATSOnRandomPort(t *testing.T) {
	err := Init(NATSOptions{
		Mode:      NATSModeInternal,
		Host:      "127.0.0.1",
		Port:      0,
		StoreDir:  filepath.Join(t.TempDir(), "nats"),
		AuthMode:  "creds",
		Password:  "must-not-be-used",
		CredsFile: "must-not-be-used.creds",
	})
	if err != nil {
		t.Fatalf("Init returned error: %v", err)
	}

	active, ok := ActiveNATSOptions()
	if !ok {
		Close()
		t.Fatal("expected active NATS options")
	}
	parsed, err := url.Parse(active.URL)
	if err != nil {
		Close()
		t.Fatalf("invalid active URL %q: %v", active.URL, err)
	}
	if parsed.Hostname() != "127.0.0.1" || parsed.Port() == "" || parsed.Port() == "0" {
		Close()
		t.Fatalf("expected a resolved loopback random port, got %q", active.URL)
	}
	if active.Mode != NATSModeInternal || active.AuthMode != "none" || active.CredsFile != "" {
		Close()
		t.Fatalf("unexpected active embedded options: %+v", active)
	}
	if embeddedNATSServer == nil || globalJS == nil || !globalJS.nc.IsConnected() {
		Close()
		t.Fatal("embedded NATS and JetStream client should be running")
	}

	Close()
	if embeddedNATSServer != nil || busInited {
		t.Fatal("Close did not clear embedded NATS lifecycle state")
	}
}

func TestPrepareNATSPreservesLegacyExternalURL(t *testing.T) {
	resolved, srv, err := prepareNATS(NATSOptions{URL: "nats://127.0.0.1:4222"})
	if err != nil {
		t.Fatalf("prepareNATS returned error: %v", err)
	}
	if srv != nil || resolved.Mode != NATSModeExternal {
		t.Fatalf("expected URL-only options to resolve as external, got mode=%q server=%v", resolved.Mode, srv)
	}
}

func TestManagedSubscriptionCloseIsIdempotentAndStopsDelivery(t *testing.T) {
	if err := Init(NATSOptions{
		Mode:     NATSModeInternal,
		Host:     "127.0.0.1",
		Port:     0,
		StoreDir: filepath.Join(t.TempDir(), "nats"),
	}); err != nil {
		t.Fatalf("Init returned error: %v", err)
	}
	t.Cleanup(Close)

	js := GetJetStream()
	js.subMu.Lock()
	baseline := len(js.subscriptions)
	js.subMu.Unlock()

	var delivered atomic.Int32
	handle, err := js.SubscribeQueueManaged(
		"data.ingest.managed-subscription-test",
		"managed-subscription-test",
		func([]byte) error {
			delivered.Add(1)
			return nil
		},
	)
	if err != nil {
		t.Fatalf("SubscribeQueueManaged returned error: %v", err)
	}

	js.subMu.Lock()
	registered := len(js.subscriptions)
	js.subMu.Unlock()
	if registered != baseline+1 {
		t.Fatalf("expected one managed subscription, baseline=%d registered=%d", baseline, registered)
	}

	if err := handle.Close(); err != nil {
		t.Fatalf("first Close returned error: %v", err)
	}
	if err := handle.Close(); err != nil {
		t.Fatalf("second Close returned error: %v", err)
	}

	js.subMu.Lock()
	remaining := len(js.subscriptions)
	js.subMu.Unlock()
	if remaining != baseline {
		t.Fatalf("managed subscription remained registered: baseline=%d remaining=%d", baseline, remaining)
	}

	if err := js.Publish("data.ingest.managed-subscription-test", []byte("after-close")); err != nil {
		t.Fatalf("Publish returned error: %v", err)
	}
	time.Sleep(150 * time.Millisecond)
	if got := delivered.Load(); got != 0 {
		t.Fatalf("closed subscription received %d messages", got)
	}
}
