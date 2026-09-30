package rpc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEngineRPCEmitPathHasNoAgentDebugFileWrites(t *testing.T) {
	path := filepath.Join("engine_rpc.go")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	content := string(data)
	for _, marker := range []string{
		"/home/sun/workspace/iot-rule-engine/.cursor/debug.log",
		`"sessionId":    "debug-session"`,
		`"hypothesisId": "engine_rpc_emit"`,
	} {
		if strings.Contains(content, marker) {
			t.Fatalf("unexpected debug instrumentation marker %q found in %s", marker, path)
		}
	}
}
