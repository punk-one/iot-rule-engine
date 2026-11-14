package rulesdb

import (
	"encoding/json"
	sdk "github.com/punk-one/iot-rule-plugin-sdk"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConnectorEncryptionSurvivesReopenAndRejectsMissingKey(t *testing.T) {
	dir := t.TempDir()
	db, err := OpenSQLite(Config{DataDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	r := &sdk.ConnectorResource{ID: "encrypted", PluginID: "connect-mqtt", Connection: json.RawMessage(`{"password":"synthetic-password"}`)}
	if err = db.SaveConnectorResource(r); err != nil {
		t.Fatal(err)
	}
	var raw string
	if err = db.db.QueryRow("SELECT resource FROM connectors WHERE id=?", r.ID).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(raw, "synthetic-password") || !strings.HasPrefix(raw, connectorCipherPrefix) {
		t.Fatal("record is not encrypted")
	}
	sealed := []byte(strings.TrimPrefix(raw, connectorCipherPrefix))
	sealed[len(sealed)/2] ^= 1
	if _, err := db.decodeConnector(connectorCipherPrefix + string(sealed)); err == nil {
		t.Fatal("tampering accepted")
	}
	db.Close()
	db, err = OpenSQLite(Config{DataDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	got, err := db.GetConnectorResource(r.ID)
	if err != nil || string(got.Connection) != string(r.Connection) {
		t.Fatalf("reopen: %v", err)
	}
	db.Close()
	if err = os.Remove(filepath.Join(dir, "connector.key")); err != nil {
		t.Fatal(err)
	}
	if db, err = OpenSQLite(Config{DataDir: dir}); err == nil {
		db.Close()
		t.Fatal("missing key accepted")
	}
}

func TestConnectorPlaintextMigration(t *testing.T) {
	dir := t.TempDir()
	db, err := OpenSQLite(Config{DataDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	raw := `{"id":"legacy","plugin_id":"connect-mqtt","connection":{"password":"legacy-marker"}}`
	if _, err = db.db.Exec("INSERT INTO connectors(id,plugin_id,name,enabled,resource) VALUES('legacy','connect-mqtt','legacy',1,?)", raw); err != nil {
		t.Fatal(err)
	}
	db.Close()
	db, err = OpenSQLite(Config{DataDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	got, err := db.GetConnectorResource("legacy")
	if err != nil || !strings.Contains(string(got.Connection), "legacy-marker") {
		t.Fatalf("migration read: %v", err)
	}
	var stored string
	if err = db.db.QueryRow("SELECT resource FROM connectors WHERE id='legacy'").Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stored, "legacy-marker") {
		t.Fatal("legacy plaintext retained in active record")
	}
}
