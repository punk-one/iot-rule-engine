package plugin

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestScanHonorsOlderPinnedArtifactAfterRestart(t *testing.T) {
	for _, fixture := range []struct{ category, id, source string }{
		{"processors", "factory-process-fixture", "./testdata/factoryplugin"},
		{"connectors", "connect-dummy", "../connector/testdata/dummy_connector"},
	} {
		t.Run(fixture.category, func(t *testing.T) {
			root := t.TempDir()
			dir := filepath.Join(root, fixture.category)
			if err := os.MkdirAll(dir, 0700); err != nil {
				t.Fatal(err)
			}
			for _, version := range []string{"1.0.1", "1.0.2"} {
				output := filepath.Join(dir, fixture.id+"-"+version+".so")
				cmd := exec.Command("go", "build", "-ldflags", "-X main.fixtureVersion="+version, "-o", output, fixture.source)
				if raw, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("fixture: %v %s", err, raw)
				}
			}
			alias := filepath.Join(dir, fixture.id+"-latest.so")
			if err := os.Symlink(fixture.id+"-1.0.1.so", alias); err != nil {
				t.Fatal(err)
			}
			loader := NewLoader(root)
			defer loader.Close()
			if err := loader.ScanPlugins(); err != nil {
				t.Fatal(err)
			}
			client, err := loader.GetPluginClient(fixture.id)
			if err != nil {
				t.Fatal(err)
			}
			if client.Info.Version != "1.0.1" || client.Path != alias {
				t.Fatalf("explicit rollback lost: version=%s path=%s", client.Info.Version, client.Path)
			}
		})
	}
}
