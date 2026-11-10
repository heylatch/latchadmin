// Package release installs, updates and rolls back MafiaHub game server runtimes from api.mafiahub.dev.
//
// Only the server folder of the release zip is downloaded (HTTP range requests), never the game client. An update
// backs up the current runtime files, server.json and data/ first; a rollback copies them back.
package release

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/heylatch/latchadmin/internal/archive"
	"github.com/heylatch/latchadmin/internal/games"
	"github.com/heylatch/latchadmin/internal/netx"
)

const api = "https://api.mafiahub.dev/mods/"

var versionRe = regexp.MustCompile(`^\d+\.\d+\.\d+$`)

// Latest returns the newest released version of a game.
func Latest(ctx context.Context, g games.Game) (string, error) {
	var body struct {
		Version  string `json:"version"`
		Download struct {
			Available bool `json:"available"`
		} `json:"download"`
	}
	if err := netx.JSON(ctx, http.MethodGet, api+g.Slug+"/releases/latest", nil, nil, &body); err != nil {
		return "", err
	}
	if !versionRe.MatchString(body.Version) {
		return "", fmt.Errorf("unexpected version %q", body.Version)
	}
	if !body.Download.Available {
		return "", errors.New("the latest release has no download yet")
	}
	return body.Version, nil
}

// downloadURL asks MafiaHub for the short-lived download link of the latest release.
func downloadURL(g games.Game) func(context.Context) (string, error) {
	return func(ctx context.Context) (string, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, api+g.Slug+"/releases/latest/download", nil)
		if err != nil {
			return "", err
		}
		// Read the redirect instead of following it: the link itself is what the ranged reader needs.
		res, err := netx.DoNoRedirect(req)
		if err != nil {
			return "", err
		}
		res.Body.Close()
		loc := res.Header.Get("Location")
		if res.StatusCode/100 != 3 || loc == "" {
			return "", fmt.Errorf("download link: HTTP %d", res.StatusCode)
		}
		return loc, nil
	}
}

// Progress reports a long step.
type Progress struct {
	Stage string `json:"stage"` // "download", "backup", "install"
	Done  int64  `json:"done"`
	Total int64  `json:"total"`
}

type Options struct {
	// Version the caller expects; the install fails if MafiaHub's latest is different (it moved meanwhile).
	Version string
	// Examples: also copy the example resources shipped in the zip (fresh installs only).
	Examples bool
	// Token: the server token from the MafiaHub panel (lists the server in the in-game browser).
	Token    string
	Progress func(Progress)
}

// open fetches the zip directory and finds the release's root folder and runtime files.
func open(ctx context.Context, g games.Game, want string) (*zip.Reader, string, []*zip.File, error) {
	f, err := netx.OpenRanged(ctx, downloadURL(g))
	if err != nil {
		return nil, "", nil, err
	}
	zr, err := zip.NewReader(f, f.Size())
	if err != nil {
		return nil, "", nil, fmt.Errorf("release zip: %w", err)
	}
	root := archive.CommonRoot(zr.File)
	if root == "" {
		return nil, "", nil, errors.New("release zip has no single top folder")
	}
	if want != "" && !strings.HasSuffix(strings.TrimSuffix(root, "/"), "-"+want) {
		return nil, "", nil, fmt.Errorf("MafiaHub now serves %s, not %s. Check for updates again", strings.TrimSuffix(root, "/"), want)
	}
	prefix := root + games.ServerFolder() + "/"
	var runtime []*zip.File
	for _, zf := range zr.File {
		if strings.HasPrefix(zf.Name, prefix) && !strings.HasSuffix(zf.Name, "/") {
			runtime = append(runtime, zf)
		}
	}
	if len(runtime) == 0 {
		return nil, "", nil, fmt.Errorf("release has no %s folder for this OS", games.ServerFolder())
	}
	return zr, root, runtime, nil
}

func versionOf(root string) string {
	r := strings.TrimSuffix(root, "/")
	return r[strings.LastIndexByte(r, '-')+1:]
}

func totalSize(files []*zip.File) int64 {
	var n int64
	for _, f := range files {
		n += int64(f.UncompressedSize64)
	}
	return n
}

// Install puts a fresh server into dir (which must be empty or not exist yet) and returns the installed version.
func Install(ctx context.Context, g games.Game, dir string, port int, opt Options) (string, error) {
	if entries, err := os.ReadDir(dir); err == nil && len(entries) > 0 {
		return "", errors.New("the folder is not empty; choose an empty folder or import the existing server")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	zr, root, runtime, err := open(ctx, g, opt.Version)
	if err != nil {
		return "", err
	}
	total := totalSize(runtime)
	report := func(done int64) {
		if opt.Progress != nil {
			opt.Progress(Progress{Stage: "download", Done: done, Total: total})
		}
	}
	if _, err := archive.Extract(zr, dir, archive.Options{Prefix: root + games.ServerFolder() + "/", Progress: report}); err != nil {
		return "", err
	}
	for _, sub := range []string{"resources", "data", "logs"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o755); err != nil {
			return "", err
		}
	}
	if opt.Examples {
		_, _ = archive.Extract(zr, filepath.Join(dir, "resources"), archive.Options{Prefix: root + "resources/", Keep: func(string) bool { return true }})
	}
	if err := writeInitialConfig(dir, port, opt.Token); err != nil {
		return "", err
	}
	return versionOf(root), nil
}

// writeInitialConfig sets the port and the MafiaHub server token. The server fills in the rest with defaults.
// The token goes in server.json, not on the command line, where other users of the machine could read it.
func writeInitialConfig(dir string, port int, token string) error {
	p := filepath.Join(dir, "server.json")
	if _, err := os.Stat(p); err == nil {
		return nil
	}
	cfg := map[string]any{"port": port}
	if token != "" {
		cfg["server-token"] = token
	}
	b, _ := json.MarshalIndent(cfg, "", "  ")
	return os.WriteFile(p, b, 0o600) // holds the server token
}

// Backup is a saved copy made before an update.
type Backup struct {
	Name      string    `json:"name"`
	Version   string    `json:"version"` // version that was replaced
	CreatedAt time.Time `json:"createdAt"`
	Files     []string  `json:"files"`
}

const backupDir = "backups"

// Update replaces the runtime in dir with the latest release. The server must be stopped.
// Returns the new version and the backup it made.
func Update(ctx context.Context, g games.Game, dir, current string, opt Options) (string, Backup, error) {
	zr, root, runtime, err := open(ctx, g, opt.Version)
	if err != nil {
		return "", Backup{}, err
	}
	prefix := root + games.ServerFolder() + "/"
	staging := filepath.Join(dir, ".latchadmin-staging")
	_ = os.RemoveAll(staging)
	if err := os.MkdirAll(staging, 0o755); err != nil {
		return "", Backup{}, err
	}
	defer os.RemoveAll(staging)
	total := totalSize(runtime)
	if _, err := archive.Extract(zr, staging, archive.Options{Prefix: prefix, Progress: func(done int64) {
		if opt.Progress != nil {
			opt.Progress(Progress{Stage: "download", Done: done, Total: total})
		}
	}}); err != nil {
		return "", Backup{}, err
	}
	var names []string
	for _, f := range runtime {
		names = append(names, strings.TrimPrefix(f.Name, prefix))
	}
	if opt.Progress != nil {
		opt.Progress(Progress{Stage: "backup"})
	}
	b, err := makeBackup(dir, current, names)
	if err != nil {
		return "", Backup{}, fmt.Errorf("backup failed, nothing was changed: %w", err)
	}
	if opt.Progress != nil {
		opt.Progress(Progress{Stage: "install"})
	}
	for _, n := range names {
		dst := filepath.Join(dir, filepath.FromSlash(n))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return "", b, err
		}
		if err := os.Rename(filepath.Join(staging, filepath.FromSlash(n)), dst); err != nil {
			// Half-replaced runtime: put the old files back.
			_ = Restore(dir, b.Name)
			return "", b, fmt.Errorf("replacing %s: %w (the previous version was restored)", n, err)
		}
	}
	return versionOf(root), b, nil
}

func makeBackup(dir, version string, runtime []string) (Backup, error) {
	stamp := time.Now().UTC().Format("20060102-150405")
	b := Backup{Name: "pre-" + version + "-" + stamp, Version: version, CreatedAt: time.Now().UTC()}
	dst := filepath.Join(dir, backupDir, b.Name)
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return b, err
	}
	files := append([]string{}, runtime...)
	files = append(files, "server.json")
	for _, rel := range files {
		src := filepath.Join(dir, filepath.FromSlash(rel))
		if _, err := os.Stat(src); errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err := copyFile(src, filepath.Join(dst, filepath.FromSlash(rel))); err != nil {
			return b, err
		}
		b.Files = append(b.Files, rel)
	}
	if err := copyTree(filepath.Join(dir, "data"), filepath.Join(dst, "data")); err != nil {
		return b, err
	}
	meta, _ := json.MarshalIndent(b, "", "  ")
	if err := os.WriteFile(filepath.Join(dst, "backup.json"), meta, 0o644); err != nil {
		return b, err
	}
	pruneBackups(dir, keepBackups)
	return b, nil
}

const keepBackups = 5

// pruneBackups deletes all but the newest keep backups (each holds a full runtime and data/).
func pruneBackups(dir string, keep int) {
	list := Backups(dir)
	for i := keep; i < len(list); i++ {
		_ = os.RemoveAll(filepath.Join(dir, backupDir, list[i].Name))
	}
}

// Backups lists the backups in dir, newest first.
func Backups(dir string) []Backup {
	entries, _ := os.ReadDir(filepath.Join(dir, backupDir))
	var out []Backup
	for _, e := range entries {
		raw, err := os.ReadFile(filepath.Join(dir, backupDir, e.Name(), "backup.json"))
		var b Backup
		if err == nil && json.Unmarshal(raw, &b) == nil && b.Name == e.Name() {
			out = append(out, b)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

// Restore copies a backup's runtime files, server.json and data/ back. The server must be stopped.
func Restore(dir, name string) error {
	if _, err := archive.CleanName(name); err != nil || strings.Contains(name, "/") {
		return errors.New("bad backup name")
	}
	src := filepath.Join(dir, backupDir, name)
	raw, err := os.ReadFile(filepath.Join(src, "backup.json"))
	if err != nil {
		return err
	}
	var b Backup
	if err := json.Unmarshal(raw, &b); err != nil {
		return err
	}
	for _, rel := range b.Files {
		if _, err := archive.CleanName(rel); err != nil {
			return err
		}
		if err := copyFile(filepath.Join(src, filepath.FromSlash(rel)), filepath.Join(dir, filepath.FromSlash(rel))); err != nil {
			return err
		}
	}
	if _, err := os.Stat(filepath.Join(src, "data")); err == nil {
		if err := copyTree(filepath.Join(src, "data"), filepath.Join(dir, "data")); err != nil {
			return err
		}
	}
	return nil
}

// BackupVersion returns the game version stored in a backup.
func BackupVersion(dir, name string) string {
	for _, b := range Backups(dir) {
		if b.Name == name {
			return b.Version
		}
	}
	return ""
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	info, err := in.Stat()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	tmp := dst + ".latchadmin-tmp"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, dst)
}

func copyTree(src, dst string) error {
	if _, err := os.Stat(src); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return filepath.WalkDir(src, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		if d.Type()&(os.ModeSymlink|os.ModeIrregular) != 0 {
			return nil // links and junctions are never copied
		}
		if d.IsDir() {
			return os.MkdirAll(filepath.Join(dst, rel), 0o755)
		}
		return copyFile(p, filepath.Join(dst, rel))
	})
}

// FindExe returns the server binary in dir for game g. exe, when set, is the file name chosen for this server.
func FindExe(g games.Game, dir, exe string) (string, error) {
	if exe != "" {
		if exe != filepath.Base(exe) || strings.HasPrefix(exe, ".") {
			return "", errors.New("bad server binary name")
		}
		p := filepath.Join(dir, exe)
		if info, err := os.Stat(p); err == nil && info.Mode().IsRegular() {
			return p, nil
		}
		return "", fmt.Errorf("%s not found in %s", exe, dir)
	}
	if len(g.ExeNames()) == 0 {
		return "", errors.New("choose this server's binary in its settings")
	}
	for _, n := range g.ExeNames() {
		p := filepath.Join(dir, n)
		if info, err := os.Stat(p); err == nil && info.Mode().IsRegular() {
			return p, nil
		}
	}
	return "", fmt.Errorf("%s not found in %s", g.ExeNames()[0], dir)
}

// Candidates lists files in dir that look like a MafiaHub server binary ("<Name>Server", ".exe" on Windows).
func Candidates(dir string) []string {
	entries, _ := os.ReadDir(dir)
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		n := e.Name()
		base := strings.TrimSuffix(n, ".exe")
		if runtime.GOOS == "windows" && !strings.HasSuffix(strings.ToLower(n), ".exe") {
			continue
		}
		if strings.HasSuffix(base, "Server") && !strings.EqualFold(base, "crashpad_handler") {
			out = append(out, n)
		}
	}
	return out
}

// Detect guesses which game a folder holds, for importing an existing server.
func Detect(dir string) (games.Game, bool) {
	for _, g := range games.All {
		if _, err := FindExe(g, dir, ""); err == nil {
			return g, true
		}
	}
	return games.Game{}, false
}
