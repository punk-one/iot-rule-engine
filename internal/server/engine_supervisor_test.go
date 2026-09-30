package server

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestSuperviseServicesCancelsAndWaitsForSiblings(t *testing.T) {
	stopped := make(chan struct{})
	err := superviseServices(context.Background(), []supervisedService{
		{name: "failed", run: func(context.Context) error { return errors.New("boom") }},
		{name: "sibling", run: func(ctx context.Context) error {
			<-ctx.Done()
			close(stopped)
			return nil
		}},
	})
	if err == nil || !strings.Contains(err.Error(), "failed: boom") {
		t.Fatalf("unexpected supervisor error: %v", err)
	}
	select {
	case <-stopped:
	default:
		t.Fatal("supervisor returned before sibling shutdown completed")
	}
}

func TestSuperviseServicesTreatsCallerCancellationAsCleanShutdown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := superviseServices(ctx, []supervisedService{{name: "service", run: func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	}}})
	if err != nil {
		t.Fatalf("expected clean shutdown, got %v", err)
	}
}

func TestSuperviseServicesRejectsUnexpectedCleanExit(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err := superviseServices(ctx, []supervisedService{{name: "service", run: func(context.Context) error { return nil }}})
	if err == nil || !strings.Contains(err.Error(), "exited unexpectedly") {
		t.Fatalf("unexpected result: %v", err)
	}
}
