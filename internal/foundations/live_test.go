package foundations

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestLiveInstall downloads the real latest release. It runs only with LATCHADMIN_LIVE=1 (it needs the internet).
func TestLiveInstall(t *testing.T) {
	if os.Getenv("LATCHADMIN_LIVE") == "" {
		t.Skip("set LATCHADMIN_LIVE=1 to download the real Foundations release")
	}
	ctx := context.Background()
	rel, err := Latest(ctx)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	// An existing config must survive, and an old resource folder must move to backups/.
	_ = os.MkdirAll(filepath.Join(dir, "data"), 0o755)
	_ = os.WriteFile(filepath.Join(dir, "data", "hmp-core.json"), []byte(`{"mine":true}`), 0o600)
	_ = os.MkdirAll(filepath.Join(dir, "resources", "hmp-core"), 0o755)
	_ = os.WriteFile(filepath.Join(dir, "resources", "hmp-core", "package.json"), []byte(`{"version":"0.0.1"}`), 0o600)
	_ = os.MkdirAll(filepath.Join(dir, "resources", "charselect"), 0o755)

	res, err := Install(ctx, dir, rel, func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("installed %s over %s: %d resources, %d configs, backup %s", res.Version, res.Previous, res.Resources, len(res.Configs), res.Backup)
	if res.Resources < 20 || Installed(dir) != rel.Version || res.Previous != "0.0.1" || res.Backup == "" {
		t.Fatalf("unexpected result %+v, installed %q", res, Installed(dir))
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "data", "hmp-core.json")); string(b) != `{"mine":true}` {
		t.Fatal("an existing config was overwritten")
	}
	if _, err := os.Stat(filepath.Join(dir, "data", "hmp-mysql.json")); err != nil {
		t.Fatal("example config not copied")
	}
	if c := Conflicts(dir); len(c) != 1 || c[0] != "charselect" {
		t.Fatalf("conflicts %v", c)
	}
}
