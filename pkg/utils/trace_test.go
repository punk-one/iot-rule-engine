package utils

import (
	"testing"

	sdk "github.com/punk-one/iot-rule-plugin-sdk"
)

func TestNormalizeTraceIDFromTraceParent(t *testing.T) {
	traceParent := "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
	if got := NormalizeTraceID(traceParent); got != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Fatalf("expected traceparent trace-id, got %q", got)
	}
}

func TestMetadataTraceIDPrefersStandardKeys(t *testing.T) {
	metadata := map[string]string{
		"x-request-id": "request-1",
		"traceparent":  "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",
	}
	if got := MetadataTraceID(metadata); got != "request-1" {
		t.Fatalf("expected first normalized metadata trace id, got %q", got)
	}
}

func TestEnsureEventTraceIDUsesCandidateThenSetsMetadata(t *testing.T) {
	event := &sdk.Event{}
	traceID := EnsureEventTraceID(event, "candidate-trace")
	if traceID != "candidate-trace" {
		t.Fatalf("expected candidate trace id, got %q", traceID)
	}
	if sdk.GetTraceID(*event) != "candidate-trace" {
		t.Fatalf("expected event trace id to be set, got %q", sdk.GetTraceID(*event))
	}
}

func TestEnsureEventTraceIDGeneratesWhenMissing(t *testing.T) {
	event := &sdk.Event{}
	traceID := EnsureEventTraceID(event)
	if traceID == "" {
		t.Fatal("expected generated trace id")
	}
	if sdk.GetTraceID(*event) == "" {
		t.Fatal("expected generated trace id to be written to event")
	}
}
