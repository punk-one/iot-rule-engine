package middleware

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/punk-one/iot-rule-plugin-sdk"
)

func TestEventContext_Basic(t *testing.T) {
	event := &sdk.Event{
		ID:      "test-event-1",
		Payload: map[string]interface{}{"key": "value"},
	}

	ctx := NewEventContext(context.Background(), "rule-1", "node-1", event)

	if ctx.RuleID != "rule-1" {
		t.Errorf("Expected RuleID 'rule-1', got '%s'", ctx.RuleID)
	}

	if ctx.FromNode != "node-1" {
		t.Errorf("Expected FromNode 'node-1', got '%s'", ctx.FromNode)
	}

	if ctx.Event.ID != "test-event-1" {
		t.Errorf("Expected Event ID 'test-event-1', got '%s'", ctx.Event.ID)
	}
}

func TestEventContext_Metadata(t *testing.T) {
	event := &sdk.Event{ID: "test"}
	ctx := NewEventContext(context.Background(), "rule-1", "node-1", event)

	// Set metadata
	ctx.Set("key1", "value1")
	ctx.Set("key2", 123)

	// Get metadata
	val, ok := ctx.Get("key1")
	if !ok || val != "value1" {
		t.Errorf("Expected 'value1', got '%v'", val)
	}

	// GetString
	strVal := ctx.GetString("key1")
	if strVal != "value1" {
		t.Errorf("Expected 'value1', got '%s'", strVal)
	}

	// Non-existent key
	strVal = ctx.GetString("nonexistent")
	if strVal != "" {
		t.Errorf("Expected empty string for non-existent key, got '%s'", strVal)
	}
}

func TestEventContext_Errors(t *testing.T) {
	event := &sdk.Event{ID: "test"}
	ctx := NewEventContext(context.Background(), "rule-1", "node-1", event)

	if ctx.HasErrors() {
		t.Error("Should not have errors initially")
	}

	ctx.AddError(context.DeadlineExceeded)
	if !ctx.HasErrors() {
		t.Error("Should have errors after adding one")
	}

	if len(ctx.Errors) != 1 {
		t.Errorf("Expected 1 error, got %d", len(ctx.Errors))
	}
}

func TestEventContext_Stop(t *testing.T) {
	event := &sdk.Event{ID: "test"}
	ctx := NewEventContext(context.Background(), "rule-1", "node-1", event)

	if ctx.IsStopped() {
		t.Error("Should not be stopped initially")
	}

	ctx.Stop()
	if !ctx.IsStopped() {
		t.Error("Should be stopped after calling Stop()")
	}
}

func TestPipeline_Empty(t *testing.T) {
	pipeline := NewPipeline()
	event := &sdk.Event{ID: "test"}
	ctx := NewEventContext(context.Background(), "rule-1", "node-1", event)

	err := pipeline.Execute(ctx)
	if err != nil {
		t.Errorf("Empty pipeline should not return error, got: %v", err)
	}
}

func TestPipeline_SingleMiddleware(t *testing.T) {
	executed := false

	pipeline := NewPipeline()
	pipeline.UseFunc("test", func(ctx *EventContext, next NextFunc) error {
		executed = true
		return next()
	})

	event := &sdk.Event{ID: "test"}
	ctx := NewEventContext(context.Background(), "rule-1", "node-1", event)

	err := pipeline.Execute(ctx)
	if err != nil {
		t.Errorf("Pipeline should not return error, got: %v", err)
	}

	if !executed {
		t.Error("Middleware should have been executed")
	}
}

func TestPipeline_MultipleMiddlewares(t *testing.T) {
	order := []int{}

	pipeline := NewPipeline()
	pipeline.UseFunc("first", func(ctx *EventContext, next NextFunc) error {
		order = append(order, 1)
		err := next()
		order = append(order, 4)
		return err
	})
	pipeline.UseFunc("second", func(ctx *EventContext, next NextFunc) error {
		order = append(order, 2)
		err := next()
		order = append(order, 3)
		return err
	})

	event := &sdk.Event{ID: "test"}
	ctx := NewEventContext(context.Background(), "rule-1", "node-1", event)

	err := pipeline.Execute(ctx)
	if err != nil {
		t.Errorf("Pipeline should not return error, got: %v", err)
	}

	expected := []int{1, 2, 3, 4}
	if len(order) != len(expected) {
		t.Errorf("Expected order %v, got %v", expected, order)
	}

	for i, v := range expected {
		if order[i] != v {
			t.Errorf("Expected order[%d]=%d, got %d", i, v, order[i])
		}
	}
}

func TestPipeline_StopPropagation(t *testing.T) {
	secondCalled := false

	pipeline := NewPipeline()
	pipeline.UseFunc("first", func(ctx *EventContext, next NextFunc) error {
		ctx.Stop()
		return next()
	})
	pipeline.UseFunc("second", func(ctx *EventContext, next NextFunc) error {
		secondCalled = true
		return next()
	})

	event := &sdk.Event{ID: "test"}
	ctx := NewEventContext(context.Background(), "rule-1", "node-1", event)

	pipeline.Execute(ctx)

	if secondCalled {
		t.Error("Second middleware should not be called when stopped")
	}
}

func TestPipeline_Clone(t *testing.T) {
	pipeline := NewPipeline()
	pipeline.UseFunc("test", func(ctx *EventContext, next NextFunc) error {
		return next()
	})

	cloned := pipeline.Clone()

	if cloned.Len() != pipeline.Len() {
		t.Errorf("Cloned pipeline should have same length, expected %d, got %d", pipeline.Len(), cloned.Len())
	}

	// Modify original
	pipeline.UseFunc("another", func(ctx *EventContext, next NextFunc) error {
		return next()
	})

	if cloned.Len() == pipeline.Len() {
		t.Error("Cloned pipeline should not be affected by original modifications")
	}
}

func TestValidatorMiddleware(t *testing.T) {
	tests := []struct {
		name         string
		event        *sdk.Event
		maxPayloadKB int
		shouldFail   bool
	}{
		{
			name:         "Valid event",
			event:        &sdk.Event{ID: "test-1", Payload: map[string]interface{}{"key": "value"}},
			maxPayloadKB: 256,
			shouldFail:   false,
		},
		{
			name:         "Nil event",
			event:        nil,
			maxPayloadKB: 256,
			shouldFail:   true,
		},
		{
			name:         "Empty event ID",
			event:        &sdk.Event{ID: "", Payload: map[string]interface{}{}},
			maxPayloadKB: 256,
			shouldFail:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := NewValidatorMiddleware(tt.maxPayloadKB)
			ctx := NewEventContext(context.Background(), "rule-1", "node-1", tt.event)

			err := validator.Process(ctx, func() error { return nil })

			if tt.shouldFail && err == nil {
				t.Error("Expected error but got nil")
			}
			if !tt.shouldFail && err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}
		})
	}
}

func TestLimiterMiddleware(t *testing.T) {
	limiter := NewLimiterMiddleware(100, 50)
	event := &sdk.Event{ID: "test"}
	ctx := NewEventContext(context.Background(), "rule-1", "node-1", event)

	called := false
	err := limiter.Process(ctx, func() error {
		called = true
		return nil
	})

	if err != nil {
		t.Errorf("Limiter should not return error: %v", err)
	}

	if !called {
		t.Error("Next function should be called")
	}
}

func TestDedupMiddleware(t *testing.T) {
	dedup := NewDedupMiddleware(1000) // 1 second window
	event := &sdk.Event{ID: "test-dedup-1"}
	ctx := NewEventContext(context.Background(), "rule-1", "node-1", event)

	// First call should pass
	callCount := 0
	err := dedup.Process(ctx, func() error {
		callCount++
		return nil
	})

	if err != nil {
		t.Errorf("First call should not return error: %v", err)
	}
	if callCount != 1 {
		t.Errorf("Expected callCount=1, got %d", callCount)
	}

	// Second call with same event ID should be deduplicated
	ctx2 := NewEventContext(context.Background(), "rule-1", "node-1", event)
	err = dedup.Process(ctx2, func() error {
		callCount++
		return nil
	})

	if err != nil {
		t.Errorf("Dedup should not return error: %v", err)
	}
	// callCount should still be 1 because the event was deduplicated
	if callCount != 1 {
		t.Errorf("Expected callCount=1 after dedup, got %d", callCount)
	}
	if !ctx2.IsStopped() {
		t.Error("Context should be stopped after deduplication")
	}
}

func TestTracingMiddleware(t *testing.T) {
	tracing := NewTracingMiddleware()
	event := &sdk.Event{
		ID:       "test",
		Metadata: nil, // No metadata initially
	}
	ctx := NewEventContext(context.Background(), "rule-1", "node-1", event)

	err := tracing.Process(ctx, func() error { return nil })

	if err == nil {
		t.Fatal("expected tracing to reject event without trace_id")
	}
	if !strings.Contains(err.Error(), "missing trace_id") {
		t.Fatalf("expected missing trace_id error, got %v", err)
	}
	if !ctx.HasErrors() {
		t.Fatal("expected context to record error")
	}
	if !ctx.IsStopped() {
		t.Fatal("expected context to stop on missing trace_id")
	}
}

func TestTracingMiddlewarePreservesExistingTraceID(t *testing.T) {
	tracing := NewTracingMiddleware()
	event := &sdk.Event{
		ID: "test",
		Metadata: map[string]string{
			"trace_id": "existing-trace",
		},
	}
	ctx := NewEventContext(context.Background(), "rule-1", "node-1", event)

	err := tracing.Process(ctx, func() error { return nil })
	if err != nil {
		t.Errorf("Tracing should not return error: %v", err)
	}

	if sdk.GetTraceID(*event) != "existing-trace" {
		t.Errorf("Expected trace_id to be preserved, got %q", sdk.GetTraceID(*event))
	}
}

func TestMetricsMiddleware(t *testing.T) {
	metrics := NewMetricsMiddleware()
	event := &sdk.Event{ID: "test"}
	ctx := NewEventContext(context.Background(), "rule-1", "node-1", event)

	err := metrics.Process(ctx, func() error {
		time.Sleep(10 * time.Millisecond)
		return nil
	})

	if err != nil {
		t.Errorf("Metrics should not return error: %v", err)
	}

	latency, ok := ctx.Get("latency_ms")
	if !ok {
		t.Error("latency_ms should be set")
	}

	if latencyMs, ok := latency.(float64); ok {
		if latencyMs < 10 {
			t.Errorf("Expected latency >= 10ms, got %.2fms", latencyMs)
		}
	} else {
		t.Error("latency_ms should be float64")
	}
}
