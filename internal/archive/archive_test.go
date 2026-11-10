package archive

import (
	"archive/zip"
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func makeZip(t *testing.T, files map[string]string, mode map[string]os.FileMode) *zip.Reader {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for name, body := range files {
		h := &zip.FileHeader{Name: name, Method: zip.Deflate}
		if m, ok := mode[name]; ok {
			h.SetMode(m)
		}
		f, err := w.CreateHeader(h)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = f.Write([]byte(body))
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	r, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestCleanNameRejectsEscapes(t *testing.T) {
	for _, n := range []string{"../x", "a/../../x", "/etc/passwd", `a\b`, "C:/x", "a//b", "./a", "", ".../x", ".. /x", "a/b.", "NUL", "con.txt", "a/COM1.js", "x:stream", "a/b\x01"} {
		if _, err := CleanName(n); !errors.Is(err, ErrUnsafe) {
			t.Errorf("%q accepted", n)
		}
	}
	for _, n := range []string{"a", "a/b.js", "dir/"} {
		if _, err := CleanName(n); err != nil {
			t.Errorf("%q rejected: %v", n, err)
		}
	}
}

func TestExtractRefusesZipSlip(t *testing.T) {
	dir := t.TempDir()
	zr := makeZip(t, map[string]string{"ok.txt": "1", "../evil.txt": "x"}, nil)
	if _, err := Extract(zr, dir, Options{}); !errors.Is(err, ErrUnsafe) {
		t.Fatalf("expected ErrUnsafe, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(dir), "evil.txt")); err == nil {
		t.Fatal("file escaped the target")
	}
}

func TestExtractRefusesSymlinkEntries(t *testing.T) {
	zr := makeZip(t, map[string]string{"link": "/etc/passwd"}, map[string]os.FileMode{"link": os.ModeSymlink | 0o777})
	if _, err := Extract(zr, t.TempDir(), Options{}); !errors.Is(err, ErrUnsafe) {
		t.Fatalf("expected ErrUnsafe, got %v", err)
	}
}

func TestExtractPrefixKeepAndLimit(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	zr := makeZip(t, map[string]string{"pkg/config.json": "theirs", "pkg/main.js": "code", "other/x": "no"}, nil)
	res, err := Extract(zr, dir, Options{Prefix: "pkg/", Keep: func(rel string) bool { return GlobMatch([]string{"config.json"}, rel) }})
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "config.json")); string(b) != "mine" {
		t.Error("kept file was overwritten")
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "main.js")); string(b) != "code" {
		t.Error("file not extracted")
	}
	if _, err := os.Stat(filepath.Join(dir, "x")); err == nil {
		t.Error("entry outside prefix extracted")
	}
	if res.Files != 1 || len(res.Kept) != 1 {
		t.Errorf("result %+v", res)
	}
	big := makeZip(t, map[string]string{"a": string(make([]byte, 2000))}, nil)
	if _, err := Extract(big, t.TempDir(), Options{MaxBytes: 1000}); err == nil {
		t.Error("size limit not enforced")
	}
}

func TestGlobMatch(t *testing.T) {
	cases := map[string]bool{"config.json": true, "data/a/b.json": true, "x.config.json": true, "main.js": false, "sub/config.json": false}
	for rel, want := range cases {
		if got := GlobMatch([]string{"config.json", "data/**", "*.config.json"}, rel); got != want {
			t.Errorf("%s: got %v", rel, got)
		}
	}
}

// A Windows junction inside the target must stop extraction: Go reports junctions as irregular files, not links.
func TestExtractRefusesJunction(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("junctions are Windows only")
	}
	dest, outside := t.TempDir(), t.TempDir()
	if err := exec.Command("cmd", "/c", "mklink", "/J", filepath.Join(dest, "sub"), outside).Run(); err != nil {
		t.Skipf("mklink /J: %v", err)
	}
	_, err := Extract(makeZip(t, map[string]string{"sub/pwned.txt": "x"}, nil), dest, Options{})
	if !errors.Is(err, ErrUnsafe) {
		t.Fatalf("want ErrUnsafe, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(outside, "pwned.txt")); err == nil {
		t.Fatal("wrote through the junction")
	}
}

func TestKeepIgnoresCaseOnWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("case-insensitive file names are a Windows thing")
	}
	if !GlobMatch([]string{"config.json"}, "Config.JSON") {
		t.Fatal("Config.JSON should match config.json")
	}
}
