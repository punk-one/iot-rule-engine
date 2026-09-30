package rulesdb

import (
	"errors"
	"fmt"
	"testing"

	"github.com/punk-one/iot-rule-plugin-sdk/schema"
)

func TestSQLiteRejectsStaleRuleSave(t *testing.T) {
	db, err := OpenSQLite(Config{DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	rule := &schema.RuleFlow{ID: "versioned", Name: "v1"}
	if err := db.SaveRule(rule); err != nil {
		t.Fatal(err)
	}
	stale := *rule
	rule.Name = "v2"
	if err := db.SaveRule(rule); err != nil {
		t.Fatal(err)
	}
	stale.Name = "stale"
	if err := db.SaveRule(&stale); !errors.Is(err, ErrRuleVersionConflict) {
		t.Fatalf("expected version conflict, got %v", err)
	}
	stored, err := db.GetRule(rule.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Name != "v2" || stored.Version != 2 {
		t.Fatalf("stale save changed stored rule: %#v", stored)
	}
}

func TestSQLiteWatcherRetriesFailedDelivery(t *testing.T) {
	db, err := OpenSQLite(Config{DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.SaveRule(&schema.RuleFlow{ID: "watched", Name: "v1"}); err != nil {
		t.Fatal(err)
	}
	attempts := 0
	cancel, err := db.WatchRules(func(string, []byte) error {
		attempts++
		if attempts == 1 {
			return fmt.Errorf("temporary failure")
		}
		return nil
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()
	rule, _ := db.GetRule("watched")
	rule.Name = "v2"
	if err := db.SaveRule(rule); err != nil {
		t.Fatal(err)
	}
	db.pollSQLiteChanges()
	db.pollSQLiteChanges()
	if attempts != 2 {
		t.Fatalf("expected failed delivery to be retried, got %d attempts", attempts)
	}
}
