package log

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitWritesToConfiguredFile(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "engine.log")

	if err := Init(Config{
		Level:      "debug",
		File:       logPath,
		MaxSize:    1,
		MaxFiles:   2,
		MaxBackups: 1,
	}); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	t.Cleanup(func() {
		_ = Init(Config{})
	})

	Info("hello %s", "engine")

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "hello engine") {
		t.Fatalf("expected log file to contain message, got %q", content)
	}
}

func TestInitRejectsInvalidLevel(t *testing.T) {
	err := Init(Config{Level: "definitely-invalid"})
	if err == nil {
		t.Fatal("expected invalid level error")
	}
}
