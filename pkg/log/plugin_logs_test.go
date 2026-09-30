package log

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	sdk "github.com/punk-one/iot-rule-plugin-sdk"
)

func TestPluginSDKLoggerWritesToRuleFile(t *testing.T) {
	pluginDir := filepath.Join(t.TempDir(), "plugins")

	if err := Init(Config{
		File:             filepath.Join(pluginDir, "..", "engine.log"),
		MaxSize:          1,
		MaxFiles:         2,
		MaxBackups:       1,
		PluginDir:        pluginDir,
		PluginMaxSize:    1,
		PluginMaxFiles:   2,
		PluginMaxBackups: 1,
	}); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	t.Cleanup(func() {
		_ = Init(Config{})
	})

	logger := NewPluginSDKLogger("rule-1", "source-mqtt", "node-1")
	logger.Info("connected", sdk.Field{Key: "topic", Value: "devices/1"})

	data, err := os.ReadFile(filepath.Join(pluginDir, "rule-1.log"))
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "connected") {
		t.Fatalf("expected plugin log to contain message, got %q", content)
	}
	if !strings.Contains(content, "plugin_id=source-mqtt") {
		t.Fatalf("expected plugin log to contain plugin id, got %q", content)
	}
	if !strings.Contains(content, "topic=devices/1") {
		t.Fatalf("expected plugin log to contain fields, got %q", content)
	}
}

func TestPluginProcessLogSinkFlushesBufferedOutput(t *testing.T) {
	pluginDir := filepath.Join(t.TempDir(), "plugins")

	if err := Init(Config{
		PluginDir:        pluginDir,
		PluginMaxSize:    1,
		PluginMaxFiles:   2,
		PluginMaxBackups: 1,
	}); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	t.Cleanup(func() {
		_ = Init(Config{})
	})

	sink := NewPluginProcessLogSink()
	if _, err := sink.Write([]byte("bootstrap line\n")); err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if err := sink.BindRuleID("rule-demo"); err != nil {
		t.Fatalf("BindRuleID failed: %v", err)
	}
	if _, err := sink.Write([]byte("runtime line\n")); err != nil {
		t.Fatalf("Write after bind failed: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(pluginDir, "rule-demo.log"))
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "bootstrap line") {
		t.Fatalf("expected buffered log to be flushed, got %q", content)
	}
	if !strings.Contains(content, "runtime line") {
		t.Fatalf("expected bound writer to receive runtime output, got %q", content)
	}
}

func TestPluginProcessLogSinkWritesToAllBoundRules(t *testing.T) {
	pluginDir := filepath.Join(t.TempDir(), "plugins")

	if err := Init(Config{
		PluginDir:        pluginDir,
		PluginMaxSize:    1,
		PluginMaxFiles:   2,
		PluginMaxBackups: 1,
	}); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	t.Cleanup(func() {
		_ = Init(Config{})
	})

	sink := NewPluginProcessLogSink()
	if err := sink.BindRuleID("rule-a"); err != nil {
		t.Fatalf("BindRuleID rule-a failed: %v", err)
	}
	if err := sink.BindRuleID("rule-b"); err != nil {
		t.Fatalf("BindRuleID rule-b failed: %v", err)
	}
	if _, err := sink.Write([]byte("shared runtime line\n")); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	for _, ruleID := range []string{"rule-a", "rule-b"} {
		data, err := os.ReadFile(filepath.Join(pluginDir, ruleID+".log"))
		if err != nil {
			t.Fatalf("ReadFile %s failed: %v", ruleID, err)
		}
		if !strings.Contains(string(data), "shared runtime line") {
			t.Fatalf("expected %s log to contain shared line, got %q", ruleID, string(data))
		}
	}
}
