package dispatcher

import (
	"errors"
	"testing"
	"time"

	"github.com/punk-one/iot-rule-engine/internal/core/dispatcher/scheduler"
	sdk "github.com/punk-one/iot-rule-plugin-sdk"
)

type stoppingScheduler struct{ onStop func() }

func (s *stoppingScheduler) Start() error                         { return nil }
func (s *stoppingScheduler) Submit(*scheduler.DispatchTask) error { return nil }
func (s *stoppingScheduler) Stop() error                          { s.onStop(); return nil }

func TestStopDoesNotHoldPublishLockWhileJoiningWorkers(t *testing.T) {
	d := NewDispatcher("rule")
	d.started = true
	d.scheduler = &stoppingScheduler{onStop: func() {
		if err := d.PublishEventToNodes("source", []string{"sink"}, sdk.NewEvent(nil, nil)); !errors.Is(err, ErrDispatcherNotStarted) {
			t.Errorf("publish during stop: %v", err)
		}
	}}
	done := make(chan struct{})
	go func() { d.Stop(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("stop deadlocked with an emitting worker")
	}
}

func TestReliableEmitReturnsPersistenceFailure(t *testing.T) {
	d := NewDispatcherWithConfig("rule", time.Second, true, "reliable", nil)
	d.started = true
	d.RegisterRule("rule", map[string][]string{"source": {"a", "b"}})
	expected := errors.New("disk unavailable")
	calls := 0
	d.publishRuleEdge = func(rule, from, to string, payload []byte) error {
		calls++
		if to == "b" {
			return expected
		}
		return nil
	}
	e := sdk.NewEvent(map[string]interface{}{"value": 1}, nil)
	if err := d.OnPluginEmit("rule", "source", &e); !errors.Is(err, expected) {
		t.Fatalf("persistence error was hidden: %v", err)
	}
	if calls != 2 {
		t.Fatalf("persisted branches: %d", calls)
	}
}
