package bridge

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallUpdateRemove(t *testing.T) {
	dir := t.TempDir()
	changed, err := Install(dir)
	if err != nil || !changed {
		t.Fatalf("first install: %v %v", changed, err)
	}
	for _, f := range []string{"package.json", "server.js", "client.js", "ui/index.html"} {
		if _, err := os.Stat(filepath.Join(dir, "resources", Folder, filepath.FromSlash(f))); err != nil {
			t.Fatalf("missing %s", f)
		}
	}
	if changed, _ := Install(dir); changed {
		t.Fatal("second install changed an up-to-date resource")
	}
	// A hand-edited file is put back.
	_ = os.WriteFile(filepath.Join(dir, "resources", Folder, "server.js"), []byte("// edited"), 0o644)
	if changed, _ := Install(dir); !changed {
		t.Fatal("edited file not restored")
	}
	if err := Remove(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "resources", Folder)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("not removed")
	}
}

func TestLeavesSomeoneElsesFolderAlone(t *testing.T) {
	dir := t.TempDir()
	theirs := filepath.Join(dir, "resources", Folder)
	_ = os.MkdirAll(theirs, 0o755)
	_ = os.WriteFile(filepath.Join(theirs, "package.json"), []byte(`{"name":"latchadmin","version":"9.9.9"}`), 0o644)
	if _, err := Install(dir); !errors.Is(err, ErrNotOurs) {
		t.Fatalf("install over someone else's folder: %v", err)
	}
	if err := Remove(dir); !errors.Is(err, ErrNotOurs) {
		t.Fatalf("remove someone else's folder: %v", err)
	}
	if b, _ := os.ReadFile(filepath.Join(theirs, "package.json")); !strings.Contains(string(b), "9.9.9") {
		t.Fatal("their folder was changed")
	}
}

func TestNotices(t *testing.T) {
	n := RestartWarning("tr", 299)
	if n.Kind != "restart" || n.Seconds != 299 || !strings.Contains(n.Title, "5 dakika") {
		t.Fatalf("%+v", n)
	}
	if a := Announcement("en", "Hi", "There", "ilimix"); a.Label != "Announcement from ilimix" {
		t.Fatalf("%+v", a)
	}
}
