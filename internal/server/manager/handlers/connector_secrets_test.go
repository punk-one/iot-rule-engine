package handlers

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestConnectorConnectionRedactionAndMerge(t *testing.T) {
	stored := json.RawMessage(`{"host":"plc.local","password":"sensitive-value","nested":{"token":"token-value","port":102}}`)
	redacted := redactConnectorConnection(stored)
	if strings.Contains(string(redacted), "sensitive-value") || strings.Contains(string(redacted), "token-value") {
		t.Fatalf("redacted connection leaked a secret: %s", redacted)
	}
	incoming := json.RawMessage(`{"host":"plc-new.local","password":"__IOT_RULE_SECRET_SET__","nested":{"token":"__IOT_RULE_SECRET_SET__","port":103}}`)
	merged := mergeConnectorSecrets(incoming, stored)
	if !strings.Contains(string(merged), "sensitive-value") || !strings.Contains(string(merged), "token-value") {
		t.Fatalf("secret placeholders did not preserve stored values: %s", merged)
	}
	if !strings.Contains(string(merged), "plc-new.local") {
		t.Fatalf("non-secret update was lost: %s", merged)
	}
}
