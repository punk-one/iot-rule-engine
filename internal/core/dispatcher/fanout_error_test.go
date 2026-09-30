package dispatcher

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	sdk "github.com/punk-one/iot-rule-plugin-sdk"
)

func TestPublishEventToNodesReturnsPartialFailures(t *testing.T) {
	d := NewDispatcher("rule-1")
	d.mu.Lock()
	d.started = true
	d.mu.Unlock()

	var published []string
	d.publishRuleEdge = func(_, _, toNodeID string, _ []byte) error {
		published = append(published, toNodeID)
		if toNodeID == "failed" {
			return errors.New("broker unavailable")
		}
		return nil
	}

	err := d.PublishEventToNodes("source", []string{"ok-a", "failed", "ok-b"}, sdk.NewEvent(map[string]interface{}{"value": 1}, nil))
	if err == nil || !strings.Contains(err.Error(), "source->failed") || !strings.Contains(err.Error(), "broker unavailable") {
		t.Fatalf("expected structured partial failure, got %v", err)
	}
	if len(published) != 3 {
		t.Fatalf("fan-out stopped early: published=%v", published)
	}
}

func TestDeduplicatorRollbackAllowsRetryAndCommitDeduplicates(t *testing.T) {
	dedup := NewDeduplicator(time.Second)
	t.Cleanup(dedup.Stop)

	first, duplicate := dedup.Reserve("rule", "source", "sink", "event")
	if duplicate || first == nil {
		t.Fatal("expected first reservation")
	}
	first.Rollback()

	retry, duplicate := dedup.Reserve("rule", "source", "sink", "event")
	if duplicate || retry == nil {
		t.Fatal("expected retry reservation after rollback")
	}
	retry.Commit()
	if reservation, duplicate := dedup.Reserve("rule", "source", "sink", "event"); !duplicate || reservation != nil {
		t.Fatal("expected committed delivery to be deduplicated")
	}
}

func TestDeduplicatorConcurrentReservationWaitsForOutcome(t *testing.T) {
	dedup := NewDeduplicator(time.Second)
	t.Cleanup(dedup.Stop)

	first, duplicate := dedup.Reserve("rule", "source", "sink", "event")
	if duplicate {
		t.Fatal("unexpected first duplicate")
	}

	var wg sync.WaitGroup
	wg.Add(1)
	result := make(chan bool, 1)
	go func() {
		defer wg.Done()
		reservation, duplicate := dedup.Reserve("rule", "source", "sink", "event")
		if reservation != nil {
			reservation.Commit()
		}
		result <- duplicate
	}()

	select {
	case <-result:
		t.Fatal("concurrent reservation returned before the first outcome")
	case <-time.After(20 * time.Millisecond):
	}
	first.Rollback()
	wg.Wait()
	if duplicate := <-result; duplicate {
		t.Fatal("waiter should reserve after predecessor rollback")
	}
}

func TestDispatcherQueueRejectionDoesNotPoisonDeduplication(t *testing.T) {
	d := NewDispatcherWithSecurityConfig("audit", time.Second, false, "best_effort", nil, 1, 100, 100, 1000, 256, "reject")
	t.Cleanup(d.deduplicator.Stop)
	d.RegisterRule("audit", map[string][]string{"source": {"sink"}})

	fill := sdk.NewEvent(nil, nil)
	retry := sdk.NewEvent(nil, nil)
	if err := d.OnPluginEmit("audit", "source", &fill); err != nil {
		t.Fatalf("failed to fill queue: %v", err)
	}
	if err := d.OnPluginEmit("audit", "source", &retry); err == nil {
		t.Fatal("expected first full-queue submission to fail")
	}
	if err := d.OnPluginEmit("audit", "source", &retry); err == nil {
		t.Fatal("expected retry to fail while queue remains full, not be deduplicated")
	}
}

func TestPublishEventToNodesReturnsNilWhenAllBranchesSucceed(t *testing.T) {
	d := NewDispatcher("rule-1")
	d.mu.Lock()
	d.started = true
	d.mu.Unlock()
	d.publishRuleEdge = func(_, _, _ string, _ []byte) error { return nil }

	if err := d.PublishEventToNodes("source", []string{"a", "b"}, sdk.NewEvent(nil, nil)); err != nil {
		t.Fatalf("unexpected publish error: %v", err)
	}
}
