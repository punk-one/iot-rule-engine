package scheduler

import (
	"testing"
	"time"

	"github.com/punk-one/iot-rule-plugin-sdk"
)

func TestFifoSchedulerSubmitTakesTaskOwnership(t *testing.T) {
	received := make(chan *DispatchTask, 1)
	scheduler := NewFifoScheduler(1, "reject", func(task *DispatchTask) error {
		received <- task
		return nil
	})
	task := &DispatchTask{
		Event: sdk.Event{
			Payload:  map[string]interface{}{"nested": map[string]interface{}{"value": "original"}},
			Metadata: map[string]string{"trace_id": "trace-1"},
		},
	}

	if err := scheduler.Submit(task); err != nil {
		t.Fatalf("Submit failed: %v", err)
	}
	task.Event.Metadata["trace_id"] = "mutated"
	task.Event.Payload["nested"].(map[string]interface{})["value"] = "mutated"

	if err := scheduler.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer scheduler.Stop()

	select {
	case owned := <-received:
		if owned.Event.Metadata["trace_id"] != "trace-1" {
			t.Fatal("queued task shares Metadata with submitter")
		}
		if got := owned.Event.Payload["nested"].(map[string]interface{})["value"]; got != "original" {
			t.Fatalf("queued task shares nested Payload with submitter: %v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for queued task")
	}
}
