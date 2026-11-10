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
	"slices"
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
	Token    string `json:"token"`   // optional MafiaHub server token
}

func validName(n string) error {
	if n = strings.TrimSpace(n); n == "" || len(n) > 60 {
		return errors.New("name must be 1 to 60 characters")
	}
	return nil
}

// InstallServer downloads a game into an empty folder and adds the server when done.
func (a *App) InstallServer(in NewServer, user string, onCreated func(id string)) (*Job, error) {
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
	if err := a.folderClash(dir); err != nil {
		return nil, err
	}
	a.mu.Lock()
	if a.installing == nil {
		a.installing = map[string]bool{}
	}
	key := strings.ToLower(dir)
	if a.installing[key] {
		a.mu.Unlock()
		return nil, errors.New("an install into this folder is already running")
	}
	a.installing[key] = true
	a.mu.Unlock()
	unreserve := func() {
		a.mu.Lock()
		delete(a.installing, key)
		a.mu.Unlock()
	}
	if in.Port == 0 {
		in.Port = g.DefaultPort
	}
	if in.Port < 1 || in.Port > 65535 {
		unreserve()
		return nil, errors.New("port must be between 1 and 65535")
	}
	latest := a.latestGame(g.ID)
	if g.Exe == "" {
		unreserve()
		return nil, fmt.Errorf("%s has no public release to install yet; add an existing server instead", g.Label)
	}
	return a.Jobs.Start("install", "", func(ctx context.Context, j *Job) (any, error) {
		defer unreserve()
		j.Set("download", 0, 0)
		v, err := release.Install(ctx, g, dir, in.Port, release.Options{Version: latest, Examples: in.Examples, Token: strings.TrimSpace(in.Token), Progress: func(p release.Progress) { j.Set(p.Stage, p.Done, p.Total) }})
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
		if onCreated != nil {
			onCreated(sv.ID)
		}
		return map[string]string{"serverId": sv.ID, "version": v}, nil
	})
}

// ImportPreview is what LatchAdmin found in an existing server folder.
type ImportPreview struct {
	Dir       string   `json:"dir"`
	Game      string   `json:"game"`
	Exe       string   `json:"exe"`
	Port      int      `json:"port"`
	EnvKeys   []string `json:"envKeys"` // from server.env / .env / latch.env (values never sent to the browser)
	EnvFrom   string   `json:"envFrom,omitempty"`
	Resources int      `json:"resources"`
	Latch     int      `json:"latch"` // Latch-protected resources
	TokenSet  bool     `json:"tokenSet"`
	// Candidates: when the game isn't recognised, files that look like a server binary; the user picks one and
	// the game.
	Candidates []string `json:"candidates,omitempty"`
}

// PreviewImport inspects a folder without changing anything.
func (a *App) PreviewImport(dir string) (ImportPreview, error) {
	dir, err := cleanDir(dir)
	if err != nil {
		return ImportPreview{}, err
	}
	g, ok := release.Detect(dir)
	var p ImportPreview
	if ok {
		exe, _ := release.FindExe(g, dir, "")
		p = ImportPreview{Dir: dir, Game: g.ID, Exe: filepath.Base(exe), Port: configPort(dir, g.DefaultPort)}
	} else {
		c := release.Candidates(dir)
		if len(c) == 0 {
			return ImportPreview{}, errors.New("no server binary found here (a file named like ArcaneMPServer.exe)")
		}
		p = ImportPreview{Dir: dir, Port: configPort(dir, 27015), Candidates: c}
	}
	p.TokenSet = TokenSet(dir)
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

// ImportServer adds an existing server folder. No file in it is changed. game and exe are needed only when the
// binary isn't recognised.
func (a *App) ImportServer(dir, name, game, exe, user string) (store.Server, error) {
	p, err := a.PreviewImport(dir)
	if err != nil {
		return store.Server{}, err
	}
	if p.Game == "" {
		g, ok := games.ByID(game)
		if !ok {
			return store.Server{}, errors.New("choose the game")
		}
		if !slices.Contains(p.Candidates, exe) {
			return store.Server{}, errors.New("choose the server binary")
		}
		p.Game, p.Exe, p.Port = g.ID, exe, configPort(p.Dir, g.DefaultPort)
	} else {
		exe = ""
	}
	if err := validName(name); err != nil {
		return store.Server{}, err
	}
	if err := a.folderClash(p.Dir); err != nil {
		return store.Server{}, err
	}
	env, _ := readEnvFiles(p.Dir)
	for i := range env {
		if env[i].Secret {
			if env[i].Value, err = secret.Seal(env[i].Value); err != nil {
				return store.Server{}, err
			}
		}
	}
	sv := store.Server{ID: store.NewID(), Name: strings.TrimSpace(name), Game: p.Game, Dir: p.Dir, Exe: exe, Port: p.Port, Env: env, Created: time.Now().UTC(), AutoRestart: true, GameUpdate: "manual"}
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

// Variables that change how the server binary loads code or which programs it finds. A panel user who can edit
// variables must not be able to point the server at a library they uploaded as a resource.
var reservedEnv = map[string]bool{
	"PATH": true, "PATHEXT": true, "SYSTEMROOT": true, "WINDIR": true, "COMSPEC": true, "PSMODULEPATH": true, "SSLKEYLOGFILE": true,
	"NODE_OPTIONS": true, "NODE_PATH": true, "NODE_EXTRA_CA_CERTS": true, "NODE_TLS_REJECT_UNAUTHORIZED": true,
	"NODE_REPL_EXTERNAL_MODULE": true,
}
var reservedPrefixes = []string{"LD_", "DYLD_", "COR_", "COMPLUS_", "DOTNET_", "CORECLR_", "MALLOC_", "GCONV_", "NODE_", "OPENSSL_", "SSL_"}

// ReservedEnvKey reports whether a variable can't be set from the panel.
func ReservedEnvKey(k string) bool {
	k = strings.ToUpper(k)
	if k == "NODE_ENV" {
		return false // only names a mode ("production"); scripts read it
	}
	if reservedEnv[k] {
		return true
	}
	for _, p := range reservedPrefixes {
		if strings.HasPrefix(k, p) {
			return true
		}
	}
	return false
}

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
	sort.Slice(out.Entries, func(i, j int) bool {
		return strings.ToLower(out.Entries[i].Name) < strings.ToLower(out.Entries[j].Name)
	})
	return out, nil
}

// ---------- Config files ----------

// The MafiaHub server token never goes to the browser: the editor shows this instead and saving it keeps the
// stored token.
const tokenMask = "********"
const tokenKey = "server-token"

// secretKey reports whether a server.json value is a secret ("server-token", "package-secret"): the editor shows
// it masked, and saving the mask keeps the stored value.
func secretKey(k string) bool {
	k = strings.ToLower(k)
	return strings.Contains(k, "token") || strings.Contains(k, "secret")
}

func readConfigMap(dir string) (map[string]any, error) {
	raw, err := os.ReadFile(filepath.Join(dir, "server.json"))
	if errors.Is(err, os.ErrNotExist) {
		return map[string]any{}, nil
	}
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("server.json isn't valid JSON: %w", err)
	}
	if m == nil {
		m = map[string]any{}
	}
	return m, nil
}

// TokenSet reports whether server.json has a MafiaHub server token.
func TokenSet(dir string) bool {
	m, err := readConfigMap(dir)
	if err != nil {
		return false
	}
	s, _ := m[tokenKey].(string)
	return strings.TrimSpace(s) != ""
}

// ReadConfig returns server.json with the server token masked.
func ReadConfig(dir string) (json.RawMessage, error) {
	m, err := readConfigMap(dir)
	if err != nil {
		raw, rerr := os.ReadFile(filepath.Join(dir, "server.json"))
		if rerr != nil {
			return nil, err
		}
		// Broken JSON is shown as is so it can be fixed; a secret in it can't be masked reliably, so refuse.
		if low := strings.ToLower(string(raw)); strings.Contains(low, "token") || strings.Contains(low, "secret") {
			return nil, err
		}
		return raw, nil
	}
	for k, v := range m {
		if s, ok := v.(string); ok && s != "" && secretKey(k) {
			m[k] = tokenMask
		}
	}
	return json.MarshalIndent(m, "", "  ")
}

// SetToken stores or clears the MafiaHub server token.
func SetToken(dir, token string) error {
	m, err := readConfigMap(dir)
	if err != nil {
		return err
	}
	token = strings.TrimSpace(token)
	if token == tokenMask || strings.ContainsAny(token, "\r\n") || len(token) > 512 {
		return errors.New("invalid token")
	}
	if token == "" {
		delete(m, tokenKey)
	} else {
		m[tokenKey] = token
	}
	pretty, _ := json.MarshalIndent(m, "", "  ")
	p := filepath.Join(dir, "server.json")
	if err := saveWithBackup(p, pretty); err != nil {
		return err
	}
	if token == "" {
		_ = os.Remove(p + ".bak") // the backup still holds the token that was just removed
	}
	return nil
}

// WriteConfig validates and saves server.json, keeping the previous one as server.json.bak.
func WriteConfig(dir string, raw []byte) error {
	var v map[string]any
	if err := json.Unmarshal(raw, &v); err != nil {
		return fmt.Errorf("not valid JSON: %w", err)
	}
	// The editor shows secrets masked; saving the mask keeps the stored value.
	old, _ := readConfigMap(dir)
	for k, val := range v {
		if s, _ := val.(string); s == tokenMask && secretKey(k) {
			if t, ok := old[k]; ok {
				v[k] = t
			} else {
				delete(v, k)
			}
		}
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
		_ = os.WriteFile(path+".bak", old, 0o600)
	}
	// 0600: server.json holds the server token, and other users on the machine have no business reading it.
	tmp := path + ".latchadmin-tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
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
