// Package store keeps the panel's state in one JSON file (latchadmin.json in the data folder), written atomically.
// Secrets inside it are sealed with package secret. The audit log is a separate append-only JSONL file.
package store

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/heylatch/latchadmin/internal/perms"
)

type User struct {
	ID           string   `json:"id"`
	Username     string   `json:"username"`
	PasswordHash string   `json:"passwordHash"`
	Permissions  []string `json:"permissions"`
	// AllServers grants access to every server, including ones added later; otherwise only those in Servers.
	AllServers bool      `json:"allServers"`
	Servers    []string  `json:"servers"`
	Role       string    `json:"role,omitempty"`       // LatchAdmin 0.1 only; converted to Permissions on load
	TOTPSecret string    `json:"totpSecret,omitempty"` // sealed; empty when 2FA is off
	CreatedAt  time.Time `json:"createdAt"`
}

type Session struct {
	TokenHash string    `json:"tokenHash"`
	UserID    string    `json:"userId"`
	ExpiresAt time.Time `json:"expiresAt"`
	CreatedAt time.Time `json:"createdAt"`
	IP        string    `json:"ip"`
}

type EnvVar struct {
	Key    string `json:"key"`
	Value  string `json:"value"` // sealed when Secret
	Secret bool   `json:"secret"`
}

// Script update policy for a Latch script on one server.
const (
	ScriptManual  = "manual"  // only when someone clicks Update
	ScriptRestart = "restart" // install new versions; they take effect at the next server restart
	ScriptNow     = "now"     // install and restart the resource right away
)

type Server struct {
	ID      string    `json:"id"`
	Name    string    `json:"name"`
	Game    string    `json:"game"`
	Dir     string    `json:"dir"`
	Exe     string    `json:"exe,omitempty"` // server binary file name, when the game has no known one
	Port    int       `json:"port"`
	Version string    `json:"version"` // installed game runtime
	Env     []EnvVar  `json:"env"`
	Created time.Time `json:"createdAt"`

	AutoStart   bool     `json:"autoStart"`   // start with the panel
	AutoRestart bool     `json:"autoRestart"` // restart after a crash
	Restarts    []string `json:"restarts"`    // daily restarts, "HH:MM" local time
	// WasRunning is set while the server runs, so a panel restart (e.g. a self-update) brings it back.
	WasRunning bool `json:"wasRunning"`

	ScriptPolicy map[string]string `json:"scriptPolicy"` // Latch script id → policy
	// GameUpdate: "manual" or "restart" (install a new game version at the next scheduled restart).
	GameUpdate string `json:"gameUpdate"`
}

type Latch struct {
	Key          string          `json:"key"` // sealed lta_ key; empty when not linked
	AccountID    string          `json:"accountId"`
	DisplayName  string          `json:"displayName"`
	LinkedAt     time.Time       `json:"linkedAt"`
	Disconnected bool            `json:"disconnected"` // the key stopped working (revoked on heylatch.app)
	LastReportAt time.Time       `json:"lastReportAt"`
	LastReport   json.RawMessage `json:"lastReport,omitempty"` // exactly what was sent, shown on the Latch page
}

type Settings struct {
	PanelName string `json:"panelName"`
	// SelfUpdate: "off", "urgent" (default) or "all". Updates install only while no server is running or at a
	// scheduled restart, so players are never kicked by a panel update.
	SelfUpdate string `json:"selfUpdate"`
	// CheckLatch: read versions from the Latch manifest (anonymous GET). Off = GitHub only, no Latch contact.
	CheckLatch bool `json:"checkLatch"`
}

type State struct {
	InstanceID string    `json:"instanceId"`
	Settings   Settings  `json:"settings"`
	Users      []User    `json:"users"`
	Sessions   []Session `json:"sessions"`
	Servers    []Server  `json:"servers"`
	Latch      Latch     `json:"latch"`
}

type Store struct {
	mu    sync.RWMutex
	path  string
	audit string
	st    State
}

// Open loads (or creates) the state in dataDir.
func Open(dataDir string) (*Store, error) {
	s := &Store{path: filepath.Join(dataDir, "latchadmin.json"), audit: filepath.Join(dataDir, "audit.jsonl")}
	b, err := os.ReadFile(s.path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		s.st = State{InstanceID: NewID() + NewID(), Settings: Settings{PanelName: "LatchAdmin", SelfUpdate: "urgent", CheckLatch: true}}
		return s, s.save()
	case err != nil:
		return nil, err
	}
	if err := json.Unmarshal(b, &s.st); err != nil {
		return nil, errors.New("latchadmin.json is damaged: " + err.Error())
	}
	for i, u := range s.st.Users {
		if u.Permissions == nil {
			s.st.Users[i].Permissions = perms.FromRole(u.Role)
			s.st.Users[i].Role = ""
			s.st.Users[i].AllServers = true
		}
	}
	return s, nil
}

// NewID returns 16 random hex characters.
func NewID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// Read runs fn with a copy-safe view of the state. fn must not keep references.
func (s *Store) Read(fn func(st *State)) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	fn(&s.st)
}

// Update changes the state and saves it. If fn returns an error nothing is saved.
func (s *Store) Update(fn func(st *State) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	before, _ := json.Marshal(s.st)
	if err := fn(&s.st); err != nil {
		_ = json.Unmarshal(before, &s.st)
		return err
	}
	return s.save()
}

func (s *Store) save() error {
	b, err := json.MarshalIndent(s.st, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

// Server returns a copy of a server by id.
func (s *Store) Server(id string) (Server, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, sv := range s.st.Servers {
		if sv.ID == id {
			return sv.Clone(), true
		}
	}
	return Server{}, false
}

// Servers returns copies of every server, safe to use outside the lock.
func (s *Store) Servers() []Server {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Server, 0, len(s.st.Servers))
	for _, sv := range s.st.Servers {
		out = append(out, sv.Clone())
	}
	return out
}

// Clone copies a server, including its slices and map, so it can't be changed through the store.
func (sv Server) Clone() Server {
	sv.Env = append([]EnvVar(nil), sv.Env...)
	sv.Restarts = append([]string(nil), sv.Restarts...)
	if sv.ScriptPolicy != nil {
		m := make(map[string]string, len(sv.ScriptPolicy))
		for k, v := range sv.ScriptPolicy {
			m[k] = v
		}
		sv.ScriptPolicy = m
	}
	return sv
}

// UpdateServer changes one server.
func (s *Store) UpdateServer(id string, fn func(sv *Server) error) error {
	return s.Update(func(st *State) error {
		for i := range st.Servers {
			if st.Servers[i].ID == id {
				return fn(&st.Servers[i])
			}
		}
		return ErrNotFound
	})
}

var ErrNotFound = errors.New("not found")

// AuditEntry is one line of the audit log.
type AuditEntry struct {
	At     time.Time         `json:"at"`
	User   string            `json:"user"`
	Action string            `json:"action"`
	Target string            `json:"target,omitempty"`
	Detail map[string]string `json:"detail,omitempty"`
}

// Audit appends to the audit log. Failures are ignored (the action already happened).
func (s *Store) Audit(user, action, target string, detail map[string]string) {
	b, _ := json.Marshal(AuditEntry{At: time.Now().UTC(), User: user, Action: action, Target: target, Detail: detail})
	f, err := os.OpenFile(s.audit, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.Write(append(b, '\n'))
}

// AuditTail returns up to n most recent entries, newest first.
func (s *Store) AuditTail(n int) []AuditEntry {
	b, err := os.ReadFile(s.audit)
	if err != nil {
		return nil
	}
	var out []AuditEntry
	lines := splitLines(b)
	for i := len(lines) - 1; i >= 0 && len(out) < n; i-- {
		var e AuditEntry
		if json.Unmarshal(lines[i], &e) == nil {
			out = append(out, e)
		}
	}
	return out
}

func splitLines(b []byte) [][]byte {
	var out [][]byte
	start := 0
	for i, c := range b {
		if c == '\n' {
			if i > start {
				out = append(out, b[start:i])
			}
			start = i + 1
		}
	}
	if start < len(b) {
		out = append(out, b[start:])
	}
	return out
}

// CanServer reports whether a user may see and act on a server.
func (u User) CanServer(id string) bool {
	if u.AllServers || perms.Has(u.Permissions, perms.All) {
		return true
	}
	for _, x := range u.Servers {
		if x == id {
			return true
		}
	}
	return false
}
