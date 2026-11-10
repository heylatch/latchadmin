// Package app ties the panel together: servers and their processes, long-running jobs, the scheduler, the Latch
// link and the update checks. The web package is a thin HTTP layer over it.
package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/heylatch/latchadmin/internal/auth"
	"github.com/heylatch/latchadmin/internal/games"
	"github.com/heylatch/latchadmin/internal/metrics"
	"github.com/heylatch/latchadmin/internal/release"
	"github.com/heylatch/latchadmin/internal/secret"
	"github.com/heylatch/latchadmin/internal/store"
	"github.com/heylatch/latchadmin/internal/supervisor"
)

type App struct {
	Store   *store.Store
	DataDir string
	PIN     auth.SetupPIN
	Jobs    *Jobs
	Logf    func(format string, a ...any)

	mu           sync.Mutex
	procs        map[string]*supervisor.Proc
	shuttingDown bool

	versions versionCache
	Metrics  *metrics.Store
	host     metrics.HostSampler
	link     *pendingLink
	// pendingSelf is a verified panel update waiting for a quiet moment (no server running or a scheduled restart).
	pendingSelf string
	// Restart is called after a self-update is swapped in. Set by main.
	Restart func(exe string)
}

func New(dataDir string) (*App, error) {
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, err
	}
	if err := secret.Init(dataDir); err != nil {
		return nil, err
	}
	st, err := store.Open(dataDir)
	if err != nil {
		return nil, err
	}
	a := &App{Store: st, DataDir: dataDir, procs: map[string]*supervisor.Proc{}, Jobs: newJobs(), Logf: log.Printf, Metrics: metrics.Open(filepath.Join(dataDir, "metrics.json.gz"))}
	return a, nil
}

// NeedsSetup is true until the first (owner) account exists.
func (a *App) NeedsSetup() bool {
	need := false
	a.Store.Read(func(st *store.State) { need = len(st.Users) == 0 })
	return need
}

// ---------- Processes ----------

// Proc returns the process handle for a server, creating it on first use.
func (a *App) Proc(id string) *supervisor.Proc {
	a.mu.Lock()
	defer a.mu.Unlock()
	if p, ok := a.procs[id]; ok {
		return p
	}
	p := supervisor.New(id, filepath.Join(a.DataDir, "servers", id, "console"), func() (supervisor.Spec, error) { return a.spec(id) })
	p.OnState = func(s supervisor.State) { a.onState(id, s) }
	a.procs[id] = p
	return p
}

func (a *App) spec(id string) (supervisor.Spec, error) {
	sv, ok := a.Store.Server(id)
	if !ok {
		return supervisor.Spec{}, store.ErrNotFound
	}
	g, ok := games.ByID(sv.Game)
	if !ok {
		return supervisor.Spec{}, fmt.Errorf("unknown game %q", sv.Game)
	}
	exe, err := release.FindExe(g, sv.Dir)
	if err != nil {
		return supervisor.Spec{}, err
	}
	return supervisor.Spec{Exe: exe, Dir: sv.Dir, Env: buildEnv(sv), Port: sv.Port, AutoRestart: sv.AutoRestart}, nil
}

// Only what a server needs from the panel's own environment is passed on; the rest comes from the server's env list.
var passEnv = []string{"PATH", "SYSTEMROOT", "WINDIR", "COMSPEC", "PATHEXT", "TEMP", "TMP", "HOME", "USERPROFILE",
	"APPDATA", "LOCALAPPDATA", "PROGRAMDATA", "LANG", "LC_ALL", "TZ", "NUMBER_OF_PROCESSORS", "PROCESSOR_ARCHITECTURE", "SYSTEMDRIVE"}

func buildEnv(sv store.Server) []string {
	var env []string
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		for _, p := range passEnv {
			if strings.EqualFold(k, p) {
				env = append(env, kv)
				break
			}
		}
	}
	if runtime.GOOS == "linux" {
		// libnode.so ships next to the server binary.
		env = append(env, "LD_LIBRARY_PATH="+sv.Dir)
	}
	for _, e := range sv.Env {
		v := e.Value
		if e.Secret {
			v = secret.MustOpen(e.Value)
		}
		env = append(env, e.Key+"="+v)
	}
	return env
}

func (a *App) onState(id string, s supervisor.State) {
	switch s {
	case supervisor.Starting:
		a.Metrics.AddEvent(id, "start", "")
	case supervisor.Running:
		a.Metrics.AddEvent(id, "ready", "")
		// The command list (for console suggestions) and the player count, once the console answers.
		go func() {
			p := a.Proc(id)
			time.Sleep(2 * time.Second)
			p.RefreshCommands()
			p.RefreshPlayers()
		}()
	case supervisor.Stopped:
		a.Metrics.AddEvent(id, "stop", "")
	case supervisor.Crashed:
		a.Metrics.AddEvent(id, "crash", a.Proc(id).Status().LastExit)
	}
	a.mu.Lock()
	quiet := a.shuttingDown
	a.mu.Unlock()
	if quiet {
		return
	}
	switch s {
	case supervisor.Starting, supervisor.Running:
		_ = a.Store.UpdateServer(id, func(sv *store.Server) error { sv.WasRunning = true; return nil })
	case supervisor.Stopped:
		_ = a.Store.UpdateServer(id, func(sv *store.Server) error { sv.WasRunning = false; return nil })
	}
}

// Running reports whether any server is up.
func (a *App) Running() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, p := range a.procs {
		switch p.Status().State {
		case supervisor.Starting, supervisor.Running, supervisor.Stopping, supervisor.Updating:
			return true
		}
	}
	return false
}

// Boot starts servers marked to start with the panel or that were running when it stopped.
func (a *App) Boot() {
	var ids []string
	a.Store.Read(func(st *store.State) {
		for _, sv := range st.Servers {
			if sv.AutoStart || sv.WasRunning {
				ids = append(ids, sv.ID)
			}
		}
	})
	for _, id := range ids {
		if err := a.Proc(id).Start(); err != nil {
			a.Logf("start %s: %v", id, err)
		}
	}
}

// Shutdown stops every server gracefully. Their "was running" mark is kept, so they come back with the panel.
func (a *App) Shutdown() {
	a.mu.Lock()
	a.shuttingDown = true
	procs := make([]*supervisor.Proc, 0, len(a.procs))
	for _, p := range a.procs {
		procs = append(procs, p)
	}
	a.mu.Unlock()
	var wg sync.WaitGroup
	for _, p := range procs {
		wg.Add(1)
		go func(p *supervisor.Proc) { defer wg.Done(); _ = p.Stop() }(p)
	}
	wg.Wait()
	_ = a.Metrics.Save()
}

// ---------- Servers ----------

var errBusy = errors.New("the server is busy with another job")

// DeleteServer removes a server from the panel. Its files are left on disk.
func (a *App) DeleteServer(id string) error {
	p := a.Proc(id)
	if s := p.Status().State; s != supervisor.Stopped && s != supervisor.Crashed {
		return errors.New("stop the server first")
	}
	_ = p.Stop()
	a.mu.Lock()
	delete(a.procs, id)
	a.Metrics.Forget(id)
	a.mu.Unlock()
	return a.Store.Update(func(st *store.State) error {
		for i, sv := range st.Servers {
			if sv.ID == id {
				st.Servers = append(st.Servers[:i], st.Servers[i+1:]...)
				return nil
			}
		}
		return store.ErrNotFound
	})
}

// UpdateGame stops the server if needed, installs the latest game release (with a backup) and starts it again.
func (a *App) UpdateGame(id, version, user string) (*Job, error) {
	sv, ok := a.Store.Server(id)
	if !ok {
		return nil, store.ErrNotFound
	}
	g, _ := games.ByID(sv.Game)
	return a.Jobs.Start("gameUpdate", id, func(ctx context.Context, j *Job) (any, error) {
		p := a.Proc(id)
		wasUp := p.Status().State == supervisor.Running || p.Status().State == supervisor.Starting
		if wasUp {
			j.Set("stop", 0, 0)
			_ = p.Stop()
		}
		if err := p.SetUpdating(true); err != nil {
			return nil, err
		}
		defer p.SetUpdating(false)
		p.Panelf("Updating %s %s → %s", g.Label, sv.Version, version)
		v, backup, err := release.Update(ctx, g, sv.Dir, orUnknown(sv.Version), release.Options{Version: version, Progress: func(pr release.Progress) { j.Set(pr.Stage, pr.Done, pr.Total) }})
		if err != nil {
			p.Panelf("Update failed: %v", err)
			a.Store.Audit(user, "game.updateFailed", sv.Name, map[string]string{"error": err.Error()})
			return nil, err
		}
		_ = a.Store.UpdateServer(id, func(s *store.Server) error { s.Version = v; return nil })
		a.Store.Audit(user, "game.update", sv.Name, map[string]string{"from": sv.Version, "to": v, "backup": backup.Name})
		p.Panelf("Updated to %s. Backup: backups/%s", v, backup.Name)
		_ = p.SetUpdating(false)
		if wasUp {
			if err := p.Start(); err != nil {
				return map[string]string{"version": v}, err
			}
		}
		return map[string]string{"version": v, "backup": backup.Name}, nil
	})
}

func orUnknown(v string) string {
	if v == "" {
		return "unknown"
	}
	return v
}

// RestoreBackup puts a game backup back. The server must be stopped.
func (a *App) RestoreBackup(id, name, user string) error {
	sv, ok := a.Store.Server(id)
	if !ok {
		return store.ErrNotFound
	}
	p := a.Proc(id)
	if err := p.SetUpdating(true); err != nil {
		return err
	}
	defer p.SetUpdating(false)
	if err := release.Restore(sv.Dir, name); err != nil {
		return err
	}
	v := release.BackupVersion(sv.Dir, name)
	_ = a.Store.UpdateServer(id, func(s *store.Server) error {
		if v != "" && v != "unknown" {
			s.Version = v
		}
		return nil
	})
	a.Store.Audit(user, "game.restore", sv.Name, map[string]string{"backup": name})
	p.Panelf("Restored backup %s", name)
	return nil
}

// ---------- Scheduler ----------

// Run starts the background loops. It returns when ctx ends.
func (a *App) Run(ctx context.Context) {
	go a.every(ctx, 30*time.Minute, 5*time.Second, a.checkVersions)
	go a.every(ctx, 15*time.Minute, 20*time.Second, a.report)
	go a.every(ctx, 30*time.Minute, 2*time.Minute, a.autoScripts)
	go a.every(ctx, 30*time.Second, 30*time.Second, a.schedule)
	go a.every(ctx, metrics.Interval, metrics.Interval, a.collect)
	go a.every(ctx, 5*time.Minute, 5*time.Minute, func(context.Context) { _ = a.Metrics.Save() })
	<-ctx.Done()
}

func (a *App) every(ctx context.Context, period, first time.Duration, fn func(context.Context)) {
	t := time.NewTimer(first)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			func() {
				defer func() {
					if r := recover(); r != nil {
						a.Logf("background task panicked: %v", r)
					}
				}()
				fn(ctx)
			}()
			t.Reset(period)
		}
	}
}

var lastFired = map[string]string{}

// schedule runs daily restarts. A restart is also when a deferred game or panel update is applied.
func (a *App) schedule(ctx context.Context) {
	now := time.Now()
	hm := now.Format("15:04")
	day := now.Format("2006-01-02")
	var due []store.Server
	a.Store.Read(func(st *store.State) {
		for _, sv := range st.Servers {
			for _, r := range sv.Restarts {
				if r == hm && lastFired[sv.ID] != day+hm {
					due = append(due, sv)
				}
			}
		}
	})
	for _, sv := range due {
		lastFired[sv.ID] = day + hm
		p := a.Proc(sv.ID)
		if p.Status().State != supervisor.Running {
			continue
		}
		p.Panelf("Scheduled restart")
		if a.pendingSelfUpdate() != "" {
			// Every server comes back after the panel restarts.
			a.applySelfUpdate(ctx)
			return
		}
		if sv.GameUpdate == "restart" {
			if latest := a.latestGame(sv.Game); latest != "" && sv.Version != "" && compare(latest, sv.Version) > 0 {
				_, _ = a.UpdateGame(sv.ID, latest, "scheduler")
				continue
			}
		}
		go func() { _ = p.Restart() }()
	}
	if a.pendingSelfUpdate() != "" && !a.Running() {
		a.applySelfUpdate(ctx)
	}
}
