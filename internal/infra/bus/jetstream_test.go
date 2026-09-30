package bus

import (
	"os"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"
)

func TestBuildConnOptionsWithCreds(t *testing.T) {
	userKP, err := nkeys.CreateUser()
	if err != nil {
		t.Fatalf("failed to create user nkey: %v", err)
	}
	seed, err := userKP.Seed()
	if err != nil {
		t.Fatalf("failed to create user seed: %v", err)
	}

	credsFile, err := os.CreateTemp(t.TempDir(), "*.creds")
	if err != nil {
		t.Fatalf("failed to create temp creds file: %v", err)
	}
	credsContent := "-----BEGIN NATS USER JWT-----\n" +
		"dummy.jwt\n" +
		"------END NATS USER JWT------\n\n" +
		"-----BEGIN USER NKEY SEED-----\n" +
		string(seed) + "\n" +
		"------END USER NKEY SEED------\n"
	if err := os.WriteFile(credsFile.Name(), []byte(credsContent), 0o600); err != nil {
		t.Fatalf("failed to write temp creds file: %v", err)
	}

	opts, err := buildConnOptions(NATSOptions{
		AuthMode:  "creds",
		CredsFile: credsFile.Name(),
	})
	if err != nil {
		t.Fatalf("expected creds auth to build options, got error: %v", err)
	}
	if len(opts) < 1 {
		t.Fatalf("expected at least 1 option, got %d", len(opts))
	}

	natsOpts := &nats.Options{}
	if err := opts[0](natsOpts); err != nil {
		t.Fatalf("expected option to apply, got error: %v", err)
	}
	if natsOpts.UserJWT == nil {
		t.Fatal("expected UserJWT callback to be set for creds auth")
	}
	if natsOpts.SignatureCB == nil {
		t.Fatal("expected SignatureCB callback to be set for creds auth")
	}
}

func TestBuildConnOptionsWithMissingCredsFile(t *testing.T) {
	_, err := buildConnOptions(NATSOptions{AuthMode: "creds"})
	if err == nil {
		t.Fatal("expected error when creds auth is enabled without creds_file")
	}
}

func TestBuildConnOptionsWithReconnectSettings(t *testing.T) {
	opts, err := buildConnOptions(NATSOptions{
		MaxReconnects: -1,
		ReconnectWait: 3 * time.Second,
	})
	if err != nil {
		t.Fatalf("expected reconnect options to build, got error: %v", err)
	}

	natsOpts := &nats.Options{}
	for _, opt := range opts {
		if err := opt(natsOpts); err != nil {
			t.Fatalf("expected option to apply, got error: %v", err)
		}
	}

	if natsOpts.MaxReconnect != -1 {
		t.Fatalf("unexpected max reconnects: %d", natsOpts.MaxReconnect)
	}
	if natsOpts.ReconnectWait != 3*time.Second {
		t.Fatalf("unexpected reconnect wait: %v", natsOpts.ReconnectWait)
	}
}

func TestBuildConnOptionsWithUnsupportedAuthMode(t *testing.T) {
	_, err := buildConnOptions(NATSOptions{AuthMode: "unknown"})
	if err == nil {
		t.Fatal("expected error for unsupported auth mode")
	}
}
