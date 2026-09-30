package rulesdb

import (
	"encoding/json"
	"testing"
	"time"

	sdk "github.com/punk-one/iot-rule-plugin-sdk"
	"github.com/punk-one/iot-rule-plugin-sdk/schema"
	"github.com/stretchr/testify/assert"
)

func TestSQLiteCRUD(t *testing.T) {
	tempDir := t.TempDir()
	cfg := Config{DataDir: tempDir}

	db, err := OpenSQLite(cfg)
	assert.NoError(t, err)
	defer db.Close()

	// 1. Save and Get Rule
	rule := &schema.RuleFlow{
		ID:              "test-rule",
		Name:            "Test Rule",
		LifecycleStatus: "draft",
		Version:         1,
	}
	err = db.SaveRule(rule)
	assert.NoError(t, err)

	rule2, err := db.GetRule("test-rule")
	assert.NoError(t, err)
	assert.Equal(t, "Test Rule", rule2.Name)
	assert.Equal(t, "draft", rule2.LifecycleStatus)

	// 2. List Rules
	rules, err := db.ListRules()
	assert.NoError(t, err)
	assert.Len(t, rules, 1)

	// 3. Update Rule (version bump)
	rule.Name = "Updated Rule"
	err = db.SaveRule(rule)
	assert.NoError(t, err)

	rule3, err := db.GetRule("test-rule")
	assert.NoError(t, err)
	assert.Equal(t, 2, rule3.Version)
	assert.Equal(t, "Updated Rule", rule3.Name)

	// 4. Snapshots
	snapshots, err := db.ListSnapshots("test-rule")
	assert.NoError(t, err)
	assert.Len(t, snapshots, 1) // One snapshot created because version bumped from 1 to 2
	assert.Equal(t, 2, snapshots[0].Version)

	// 5. Config
	configData := []byte(`{"key": "value"}`)
	err = db.SaveConfig(configData)
	assert.NoError(t, err)

	config2, err := db.GetConfig()
	assert.NoError(t, err)
	assert.Equal(t, configData, config2)

	// 6. Watch
	updateChan := make(chan string, 1)
	cancel, err := db.WatchRules(func(ruleID string, data []byte) error {
		updateChan <- ruleID
		return nil
	}, nil)
	assert.NoError(t, err)

	// Trigger update
	rule.Name = "Final Rule"
	db.SaveRule(rule)

	select {
	case id := <-updateChan:
		assert.Equal(t, "test-rule", id)
	case <-time.After(3 * time.Second):
		t.Fatal("Watch timeout")
	}
	cancel()
}

func TestFactoryOpen(t *testing.T) {
	tempDir1 := t.TempDir()

	// Test SQLite
	db, err := Open(Config{DataDir: tempDir1, StorageType: "sqlite"})
	assert.NoError(t, err)
	if assert.NotNil(t, db) {
		assert.Equal(t, "sqlite", string(OpenByType_Helper_GetStorageType(db)))
		db.Close()
	}
}

func TestSQLiteWatchTriggersUpdateForNewRule(t *testing.T) {
	tempDir := t.TempDir()
	cfg := Config{DataDir: tempDir}

	db, err := OpenSQLite(cfg)
	assert.NoError(t, err)
	defer db.Close()

	updateChan := make(chan string, 1)
	cancel, err := db.WatchRules(func(ruleID string, data []byte) error {
		updateChan <- ruleID
		return nil
	}, nil)
	assert.NoError(t, err)
	defer cancel()

	// 新增规则：Watcher 必须触发 onUpdate（否则创建+部署会漏加载）
	newRule := &schema.RuleFlow{
		ID:              "new-rule",
		Name:            "New Rule",
		LifecycleStatus: "draft",
		Version:         1,
		Enabled:         true,
	}
	err = db.SaveRule(newRule)
	assert.NoError(t, err)

	select {
	case id := <-updateChan:
		assert.Equal(t, "new-rule", id)
	case <-time.After(3 * time.Second):
		t.Fatal("Watch timeout for new rule update event")
	}
}

func TestSQLiteWatchRulesSharesSinglePollingLoopAcrossSubscribers(t *testing.T) {
	tempDir := t.TempDir()
	db, err := OpenSQLite(Config{DataDir: tempDir})
	assert.NoError(t, err)
	defer db.Close()

	update1 := make(chan string, 1)
	update2 := make(chan string, 1)

	cancel1, err := db.WatchRules(func(ruleID string, data []byte) error {
		update1 <- ruleID
		return nil
	}, nil)
	assert.NoError(t, err)
	defer cancel1()

	cancel2, err := db.WatchRules(func(ruleID string, data []byte) error {
		update2 <- ruleID
		return nil
	}, nil)
	assert.NoError(t, err)
	defer cancel2()

	assert.Equal(t, 1, db.ruleWatchLoopStarts, "expected multiple WatchRules subscribers to share one polling loop")

	rule := &schema.RuleFlow{
		ID:              "shared-loop-rule",
		Name:            "Shared Loop Rule",
		LifecycleStatus: "draft",
		Version:         1,
		Enabled:         true,
	}
	err = db.SaveRule(rule)
	assert.NoError(t, err)

	select {
	case id := <-update1:
		assert.Equal(t, rule.ID, id)
	case <-time.After(3 * time.Second):
		t.Fatal("first watcher timeout")
	}

	select {
	case id := <-update2:
		assert.Equal(t, rule.ID, id)
	case <-time.After(3 * time.Second):
		t.Fatal("second watcher timeout")
	}
}

func TestSQLiteConnectorResourceCRUD(t *testing.T) {
	tempDir := t.TempDir()
	db, err := OpenSQLite(Config{DataDir: tempDir})
	assert.NoError(t, err)
	defer db.Close()

	resource := &sdk.ConnectorResource{
		ID:            "conn-plc-01",
		Name:          "PLC 01",
		PluginID:      "connect-s7",
		SessionPolicy: "split_rw",
		WarmupPolicy:  sdk.WarmupPolicyPreload,
		Connection:    json.RawMessage(`{"host":"192.0.2.10","rack":0,"slot":1}`),
		Enabled:       true,
	}

	err = db.SaveConnectorResource(resource)
	assert.NoError(t, err)

	got, err := db.GetConnectorResource(resource.ID)
	assert.NoError(t, err)
	if assert.NotNil(t, got) {
		assert.Equal(t, resource.PluginID, got.PluginID)
		assert.Equal(t, resource.WarmupPolicy, got.WarmupPolicy)
		assert.Equal(t, resource.SessionPolicy, got.SessionPolicy)
	}

	list, err := db.ListConnectorResources()
	assert.NoError(t, err)
	if assert.Len(t, list, 1) {
		assert.Equal(t, resource.ID, list[0].ID)
	}

	resource.Name = "PLC 01 Updated"
	resource.Enabled = false
	err = db.SaveConnectorResource(resource)
	assert.NoError(t, err)

	got, err = db.GetConnectorResource(resource.ID)
	assert.NoError(t, err)
	if assert.NotNil(t, got) {
		assert.Equal(t, "PLC 01 Updated", got.Name)
		assert.False(t, got.Enabled)
	}

	err = db.DeleteConnectorResource(resource.ID)
	assert.NoError(t, err)

	_, err = db.GetConnectorResource(resource.ID)
	assert.Error(t, err)
}

func TestSQLiteWatchConnectorResources(t *testing.T) {
	tempDir := t.TempDir()
	db, err := OpenSQLite(Config{DataDir: tempDir})
	assert.NoError(t, err)
	defer db.Close()

	updateChan := make(chan string, 2)
	deleteChan := make(chan string, 1)
	cancel, err := db.WatchConnectorResources(func(resourceID string, data []byte) error {
		updateChan <- resourceID
		return nil
	}, func(resourceID string) error {
		deleteChan <- resourceID
		return nil
	})
	assert.NoError(t, err)
	defer cancel()

	resource := &sdk.ConnectorResource{
		ID:       "conn-watch-01",
		PluginID: "connect-dummy",
		Enabled:  true,
	}
	err = db.SaveConnectorResource(resource)
	assert.NoError(t, err)

	select {
	case id := <-updateChan:
		assert.Equal(t, resource.ID, id)
	case <-time.After(3 * time.Second):
		t.Fatal("connector watch timeout on create")
	}

	resource.Name = "Updated"
	err = db.SaveConnectorResource(resource)
	assert.NoError(t, err)

	select {
	case id := <-updateChan:
		assert.Equal(t, resource.ID, id)
	case <-time.After(3 * time.Second):
		t.Fatal("connector watch timeout on update")
	}

	err = db.DeleteConnectorResource(resource.ID)
	assert.NoError(t, err)

	select {
	case id := <-deleteChan:
		assert.Equal(t, resource.ID, id)
	case <-time.After(3 * time.Second):
		t.Fatal("connector watch timeout on delete")
	}
}

// Helper to check type during test
func OpenByType_Helper_GetStorageType(db RulesDB) string {
	switch db.(type) {
	case *SQLiteDB:
		return "sqlite"
	}
	return "unknown"
}
