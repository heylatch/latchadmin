package app

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/heylatch/latchadmin/internal/games"
	"github.com/heylatch/latchadmin/internal/release"
	"github.com/heylatch/latchadmin/internal/resources"
	"github.com/heylatch/latchadmin/internal/secret"
	"github.com/heylatch/latchadmin/internal/store"
)

// NewServer is what the "add server" form sends.
type NewServer struct {
	Name     string `json:"name"`
	Game     string `json:"game"`
	Dir      string `json:"dir"`
	Port     int    `json:"port"`
	Examples bool   `json:"examples"`
	License  string `json:"license"` // optional LATCH_LICENSE
}

func cleanDir(dir string) (string, error) {
	dir = strings.TrimSpace(dir)
	if dir == "" || !filepath.IsAbs(dir) {
		return "", errors.New("enter a full folder path")
	}
	return filepath.Clean(dir), nil
}

func (a *App) dirInUse(dir string) bool {
	used := false
	a.Store.Read(func(st *store.State) {
		for _, sv := range st.Servers {
			if strings.EqualFold(filepath.Clean(sv.Dir), dir) {
				used = true
			}
		}
	})
	return used
}

func validName(n string) error {
	if n = strings.TrimSpace(n); n == "" || len(n) > 60 {
		return errors.New("name must be 1 to 60 characters")
	}
	return nil
}

// InstallServer downloads a game into an empty folder and adds the server when done.
func (a *App) InstallServer(in NewServer, user string) (*Job, error) {
	g, ok := games.ByID(in.Game)
	if !ok {
		return nil, errors.New("unknown game")
	}
	if err := validName(in.Name); err != nil {
		return nil, err
	}
	dir, err := cleanDir(in.Dir)
	if err != nil {
		return nil, err
	}
	if a.dirInUse(dir) {
		return nil, errors.New("another server already uses this folder")
	}
	if in.Port == 0 {
		in.Port = g.DefaultPort
	}
	if in.Port < 1 || in.Port > 65535 {
		return nil, errors.New("port must be between 1 and 65535")
	}
	latest := a.latestGame(g.ID)
	return a.Jobs.Start("install", "", func(ctx context.Context, j *Job) (any, error) {
		j.Set("download", 0, 0)
		v, err := release.Install(ctx, g, dir, in.Port, release.Options{Version: latest, Examples: in.Examples, Progress: func(p release.Progress) { j.Set(p.Stage, p.Done, p.Total) }})
		if err != nil {
			return nil, err
		}
		sv := store.Server{ID: store.NewID(), Name: strings.TrimSpace(in.Name), Game: g.ID, Dir: dir, Port: in.Port, Version: v, Created: time.Now().UTC(), AutoRestart: true, GameUpdate: "manual"}
		if lic := strings.TrimSpace(in.License); lic != "" {
			sealed, err := secret.Seal(lic)
			if err != nil {
				return nil, err
			}
			sv.Env = append(sv.Env, store.EnvVar{Key: "LATCH_LICENSE", Value: sealed, Secret: true})
		}
		if err := a.Store.Update(func(st *store.State) error { st.Servers = append(st.Servers, sv); return nil }); err != nil {
			return nil, err
		}
		a.Store.Audit(user, "server.install", sv.Name, map[string]string{"game": g.ID, "version": v, "dir": dir})
		return map[string]string{"serverId": sv.ID, "version": v}, nil
	})
}

// ImportPreview is what LatchAdmin found in an existing server folder.
type ImportPreview struct {
	Dir       string   `json:"dir"`
	Game      string   `json:"game"`
	Exe       string   `json:"exe"`
	Port      int      `json:"port"`
	EnvKeys   []string `json:"envKeys"`   // from server.env / .env / latch.env (values never sent to the browser)
	EnvFrom   string   `json:"envFrom,omitempty"`
	Resources int      `json:"resources"`
	Latch     int      `json:"latch"` // Latch-protected resources
}

// PreviewImport inspects a folder without changing anything.
func (a *App) PreviewImport(dir string) (ImportPreview, error) {
	dir, err := cleanDir(dir)
	if err != nil {
		return ImportPreview{}, err
	}
	g, ok := release.Detect(dir)
	if !ok {
		var names []string
		for _, g := range games.All {
			names = append(names, g.ExeNames()[0])
		}
		return ImportPreview{}, fmt.Errorf("no server found here (looked for %s)", strings.Join(names, ", "))
	}
	exe, _ := release.FindExe(g, dir)
	p := ImportPreview{Dir: dir, Game: g.ID, Exe: filepath.Base(exe), Port: configPort(dir, g.DefaultPort)}
	if env, from := readEnvFiles(dir); from != "" {
		p.EnvFrom = from
		for _, e := range env {
			p.EnvKeys = append(p.EnvKeys, e.Key)
		}
	}
	list, _ := resources.List(dir)
	for _, r := range list {
		p.Resources++
		if r.Latch != nil {
			p.Latch++
		}
	}
	return p, nil
}

// ImportServer adds an existing server folder. No file in it is changed.
func (a *App) ImportServer(dir, name, user string) (store.Server, error) {
	p, err := a.PreviewImport(dir)
	if err != nil {
		return store.Server{}, err
	}
	if err := validName(name); err != nil {
		return store.Server{}, err
	}
	if a.dirInUse(p.Dir) {
		return store.Server{}, errors.New("another server already uses this folder")
	}
	env, _ := readEnvFiles(p.Dir)
	for i := range env {
		if env[i].Secret {
			if env[i].Value, err = secret.Seal(env[i].Value); err != nil {
				return store.Server{}, err
			}
		}
	}
	sv := store.Server{ID: store.NewID(), Name: strings.TrimSpace(name), Game: p.Game, Dir: p.Dir, Port: p.Port, Env: env, Created: time.Now().UTC(), AutoRestart: true, GameUpdate: "manual"}
	if err := a.Store.Update(func(st *store.State) error { st.Servers = append(st.Servers, sv); return nil }); err != nil {
		return store.Server{}, err
	}
	a.Store.Audit(user, "server.import", sv.Name, map[string]string{"dir": sv.Dir, "game": sv.Game})
	return sv, nil
}

func configPort(dir string, def int) int {
	raw, err := os.ReadFile(filepath.Join(dir, "server.json"))
	if err != nil {
		return def
	}
	var c struct {
		Port int `json:"port"`
	}
	if json.Unmarshal(raw, &c) == nil && c.Port > 0 {
		return c.Port
	}
	return def
}

var envKeyRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,127}$`)
var secretKeyRe = regexp.MustCompile(`(?i)(pass|secret|token|key|license|pwd|auth)`)

// IsSecretKey guesses whether an env name holds a secret; such values are sealed and masked.
func IsSecretKey(k string) bool { return secretKeyRe.MatchString(k) }

// ValidEnvKey checks an environment variable name.
func ValidEnvKey(k string) bool { return envKeyRe.MatchString(k) }

// readEnvFiles reads KEY=VALUE files a start script would load (server.env, .env, latch.env, in that order;
// later files don't override earlier ones).
func readEnvFiles(dir string) ([]store.EnvVar, string) {
	var out []store.EnvVar
	seen := map[string]bool{}
	var from []string
	for _, name := range []string{"server.env", ".env", "latch.env"} {
		raw, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		from = append(from, name)
		sc := bufio.NewScanner(bytes.NewReader(raw))
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			line = strings.TrimPrefix(line, "export ")
			k, v, ok := strings.Cut(line, "=")
			k = strings.TrimSpace(k)
			if !ok || !ValidEnvKey(k) || seen[k] {
				continue
			}
			v = strings.TrimSpace(v)
			if len(v) >= 2 && (v[0] == '"' && v[len(v)-1] == '"' || v[0] == '\'' && v[len(v)-1] == '\'') {
				v = v[1 : len(v)-1]
			}
			seen[k] = true
			out = append(out, store.EnvVar{Key: k, Value: v, Secret: IsSecretKey(k)})
		}
	}
	return out, strings.Join(from, ", ")
}

// ---------- Folder browser (for picking an install or import folder) ----------

type DirEntry struct {
	Name   string `json:"name"`
	Path   string `json:"path"`
	Server string `json:"server,omitempty"` // game id when the folder holds a server
}

type DirListing struct {
	Path    string     `json:"path"`
	Parent  string     `json:"parent"`
	Entries []DirEntry `json:"entries"`
}

// Browse lists sub-folders of path ("" = drives on Windows, / elsewhere). Owner-only in the web layer.
func Browse(path string) (DirListing, error) {
	if path == "" {
		if runtime.GOOS == "windows" {
			var out DirListing
			for c := 'C'; c <= 'Z'; c++ {
				d := string(c) + `:\`
				if _, err := os.Stat(d); err == nil {
					out.Entries = append(out.Entries, DirEntry{Name: d, Path: d})
				}
			}
			return out, nil
		}
		path = "/"
	}
	path = filepath.Clean(path)
	if !filepath.IsAbs(path) {
		return DirListing{}, errors.New("path must be absolute")
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return DirListing{}, err
	}
	out := DirListing{Path: path}
	if parent := filepath.Dir(path); parent != path {
		out.Parent = parent
	} else if runtime.GOOS == "windows" {
		out.Parent = "" // back to the drive list
	}
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") || strings.HasPrefix(e.Name(), "$") {
			continue
		}
		p := filepath.Join(path, e.Name())
		d := DirEntry{Name: e.Name(), Path: p}
		if g, ok := release.Detect(p); ok {
			d.Server = g.ID
		}
		out.Entries = append(out.Entries, d)
		if len(out.Entries) >= 500 {
			break
		}
	}
	sort.Slice(out.Entries, func(i, j int) bool { return strings.ToLower(out.Entries[i].Name) < strings.ToLower(out.Entries[j].Name) })
	return out, nil
}

// ---------- Config files ----------

// ReadConfig returns server.json (empty object when missing).
func ReadConfig(dir string) (json.RawMessage, error) {
	raw, err := os.ReadFile(filepath.Join(dir, "server.json"))
	if errors.Is(err, os.ErrNotExist) {
		return json.RawMessage("{}"), nil
	}
	return raw, err
}

// WriteConfig validates and saves server.json, keeping the previous one as server.json.bak.
func WriteConfig(dir string, raw []byte) error {
	var v map[string]any
	if err := json.Unmarshal(raw, &v); err != nil {
		return fmt.Errorf("not valid JSON: %w", err)
	}
	if p, ok := v["port"]; ok {
		if f, ok := p.(float64); !ok || f < 1 || f > 65535 || f != float64(int(f)) {
			return errors.New("port must be a whole number between 1 and 65535")
		}
	}
	pretty, _ := json.MarshalIndent(v, "", "  ")
	return saveWithBackup(filepath.Join(dir, "server.json"), pretty)
}

func saveWithBackup(path string, data []byte) error {
	if old, err := os.ReadFile(path); err == nil {
		_ = os.WriteFile(path+".bak", old, 0o644)
	}
	tmp := path + ".latchadmin-tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// DataFiles lists the JSON files in data/ (per-resource configs).
func DataFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(dir, "data"))
	if errors.Is(err, os.ErrNotExist) {
		return []string{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := []string{}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(strings.ToLower(e.Name()), ".json") {
			out = append(out, e.Name())
		}
	}
	return out, nil
}

func dataPath(dir, name string) (string, error) {
	if name != filepath.Base(name) || !strings.HasSuffix(strings.ToLower(name), ".json") || strings.HasPrefix(name, ".") {
		return "", errors.New("bad file name")
	}
	return filepath.Join(dir, "data", name), nil
}

func ReadDataFile(dir, name string) ([]byte, error) {
	p, err := dataPath(dir, name)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(p)
}

// WriteDataFile saves a data/*.json file after checking it parses.
func WriteDataFile(dir, name string, raw []byte) error {
	p, err := dataPath(dir, name)
	if err != nil {
		return err
	}
	if !json.Valid(raw) {
		return errors.New("not valid JSON")
	}
	return saveWithBackup(p, raw)
}
