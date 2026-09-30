package state

import (
	"context"
	"testing"
	"time"
)

func TestMemoryBackendSetWithTTLExpires(t *testing.T) {
	backend := NewMemoryBackend()
	ctx := context.Background()

	if err := backend.SetWithTTL(ctx, "k1", []byte("v1"), 20*time.Millisecond); err != nil {
		t.Fatalf("SetWithTTL failed: %v", err)
	}

	got, err := backend.Get(ctx, "k1")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if string(got) != "v1" {
		t.Fatalf("expected v1, got %q", got)
	}

	time.Sleep(30 * time.Millisecond)

	got, err = backend.Get(ctx, "k1")
	if err != nil {
		t.Fatalf("Get after ttl failed: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected expired value to be missing, got %q", got)
	}
}

func TestStateManagerSetStateWithTTLExpires(t *testing.T) {
	manager := NewStateManager(NewMemoryBackend())
	type snapshot struct {
		Value int `json:"value"`
	}

	if err := manager.SetStateWithTTL(context.Background(), "state:test", &snapshot{Value: 9}, 20*time.Millisecond); err != nil {
		t.Fatalf("SetStateWithTTL failed: %v", err)
	}

	var got snapshot
	if err := manager.GetState(context.Background(), "state:test", &got); err != nil {
		t.Fatalf("GetState failed: %v", err)
	}
	if got.Value != 9 {
		t.Fatalf("expected 9, got %d", got.Value)
	}

	time.Sleep(30 * time.Millisecond)

	if err := manager.GetState(context.Background(), "state:test", &got); err == nil {
		t.Fatal("expected state to expire")
	}
}
