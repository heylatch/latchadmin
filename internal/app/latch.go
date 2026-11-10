package app

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/heylatch/latchadmin/internal/buildinfo"
	"github.com/heylatch/latchadmin/internal/latch"
	"github.com/heylatch/latchadmin/internal/notify"
	"github.com/heylatch/latchadmin/internal/secret"
	"github.com/heylatch/latchadmin/internal/store"
	"github.com/heylatch/latchadmin/internal/supervisor"
)

// ---------- Linking ----------

type pendingLink struct {
	mu                sync.Mutex
	UserCode          string    `json:"userCode"`
	VerifyURL         string    `json:"verifyUrl"`
	VerifyURLComplete string    `json:"verifyUrlComplete"`
	ExpiresAt         time.Time `json:"expiresAt"`
	Status            string    `json:"status"` // pending, approved, denied, expired, error
	Error             string    `json:"error,omitempty"`
}

// LinkSnapshot is the state of an ongoing link for the UI.
type LinkSnapshot struct {
	UserCode          string    `json:"userCode"`
	VerifyURL         string    `json:"verifyUrl"`
	VerifyURLComplete string    `json:"verifyUrlComplete"`
	ExpiresAt         time.Time `json:"expiresAt"`
	Status            string    `json:"status"`
	Error             string    `json:"error,omitempty"`
}

func (a *App) LinkStatus() *LinkSnapshot {
	a.mu.Lock()
	l := a.link
	a.mu.Unlock()
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return &LinkSnapshot{UserCode: l.UserCode, VerifyURL: l.VerifyURL, VerifyURLComplete: l.VerifyURLComplete, ExpiresAt: l.ExpiresAt, Status: l.Status, Error: l.Error}
}

// StartLink asks Latch for a code and waits in the background for the owner to approve it on heylatch.app.
func (a *App) StartLink(ctx context.Context, user string) (*LinkSnapshot, error) {
	var name string
	a.Store.Read(func(st *store.State) { name = st.Settings.PanelName })
	d, err := latch.StartLink(ctx, name)
	if err != nil {
		return nil, err
	}
	l := &pendingLink{UserCode: d.UserCode, VerifyURL: d.VerifyURL, VerifyURLComplete: d.VerifyURLComplete, ExpiresAt: time.Now().Add(time.Duration(d.ExpiresIn) * time.Second), Status: "pending"}
	a.mu.Lock()
	a.link = l
	a.mu.Unlock()
	go a.pollLink(l, d, user)
	return a.LinkStatus(), nil
}

func (a *App) pollLink(l *pendingLink, d latch.DeviceStart, user string) {
	interval := time.Duration(max(d.Interval, 3)) * time.Second
	set := func(status, msg string) {
		l.mu.Lock()
		l.Status, l.Error = status, msg
		l.mu.Unlock()
	}
	for time.Now().Before(l.ExpiresAt) {
		time.Sleep(interval)
		a.mu.Lock()
		current := a.link == l
		a.mu.Unlock()
		if !current {
			return // replaced by a newer attempt or cancelled
		}
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		status, key, err := latch.PollLink(ctx, d.DeviceCode)
		cancel()
		if err != nil {
			continue
		}
		switch status {
		case "approved":
			if err := a.SetKey(context.Background(), key, user); err != nil {
				set("error", err.Error())
				return
			}
			set("approved", "")
			return
		case "denied", "expired":
			set(status, "")
			return
		}
	}
	set("expired", "")
}

// CancelLink abandons an ongoing link.
func (a *App) CancelLink() {
	a.mu.Lock()
	a.link = nil
	a.mu.Unlock()
}

// SetKey stores an account key after checking it with Latch.
func (a *App) SetKey(ctx context.Context, key, user string) error {
	key = strings.TrimSpace(key)
	if !strings.HasPrefix(key, "lta_") {
		return errors.New("this isn't an account API key (they start with lta_). Create one under Settings > API keys on heylatch.app")
	}
	acct, err := latch.Me(ctx, key)
	if err != nil {
		return err
	}
	sealed, err := secret.Seal(key)
	if err != nil {
		return err
	}
	if err := a.Store.Update(func(st *store.State) error {
		st.Latch = store.Latch{Key: sealed, AccountID: acct.AccountID, DisplayName: acct.DisplayName, LinkedAt: time.Now().UTC()}
		return nil
	}); err != nil {
		return err
	}
	a.Store.Audit(user, "latch.link", acct.AccountID, nil)
	go a.report(context.Background())
	return nil
}

// Unlink forgets the key. It keeps working on Latch until revoked there.
func (a *App) Unlink(user string) error {
	err := a.Store.Update(func(st *store.State) error { st.Latch = store.Latch{}; return nil })
	if err == nil {
		a.Store.Audit(user, "latch.unlink", "", nil)
	}
	return err
}

func (a *App) key() string {
	var k string
	a.Store.Read(func(st *store.State) {
		if !st.Latch.Disconnected {
			k = st.Latch.Key
		}
	})
	return secret.MustOpen(k)
}

func (a *App) keyFailed(err error) {
	if errors.Is(err, latch.ErrUnauthorized) {
		was := false
		_ = a.Store.Update(func(st *store.State) error { was, st.Latch.Disconnected = st.Latch.Disconnected, true; return nil })
		if !was {
			a.Notify(notify.Event{Kind: "latch.lost"})
		}
	}
}

// ---------- Report ----------

// BuildReport is the exact payload sent to Latch. It is also shown on the Latch page.
func (a *App) BuildReport() latch.Report {
	var r latch.Report
	var servers []store.Server
	a.Store.Read(func(st *store.State) {
		r = latch.Report{InstanceID: st.InstanceID, Name: st.Settings.PanelName, PanelVersion: buildinfo.Version, OS: latch.OSName()}
		for _, sv := range st.Servers {
			servers = append(servers, sv.Clone())
		}
	})
	if len(r.Name) > 60 {
		r.Name = r.Name[:60]
	}
	r.Servers = []latch.ReportServer{}
	for _, sv := range servers {
		if len(r.Servers) == 20 {
			break
		}
		st := a.Proc(sv.ID).Status()
		state := string(st.State)
		if state == "stopping" {
			state = "running"
		}
		var ver *string
		if sv.Version != "" {
			v := sv.Version
			ver = &v
		}
		rs := latch.ReportServer{Game: sv.Game, GameVersion: ver, State: state, Scripts: []latch.ReportScript{}}
		for id, res := range latch.Installed(sv.Dir) {
			if len(rs.Scripts) == 200 {
				break
			}
			rs.Scripts = append(rs.Scripts, latch.ReportScript{ScriptID: id, Version: res.Latch.Version})
		}
		r.Servers = append(r.Servers, rs)
	}
	return r
}

func (a *App) report(ctx context.Context) {
	key := a.key()
	if key == "" {
		return
	}
	r := a.BuildReport()
	m, err := latch.SendReport(ctx, key, r)
	if err != nil {
		a.keyFailed(err)
		var ae *latch.APIError
		if errors.As(err, &ae) && ae.Code == "panelInstanceTaken" {
			// Our id is registered to another account (a copied data folder): take a fresh one.
			_ = a.Store.Update(func(st *store.State) error { st.InstanceID = store.NewID() + store.NewID(); return nil })
		}
		return
	}
	raw, _ := json.Marshal(r)
	_ = a.Store.Update(func(st *store.State) error {
		st.Latch.LastReportAt, st.Latch.LastReport = time.Now().UTC(), raw
		return nil
	})
	a.setManifest(m)
}

// ---------- Scripts ----------

// ScriptRow is one Library script with its state on each server.
type ScriptRow struct {
	Item    latch.LibraryItem         `json:"item"`
	Servers map[string]ScriptOnServer `json:"servers"`
}

type ScriptOnServer struct {
	Installed string `json:"installed,omitempty"`
	Folder    string `json:"folder,omitempty"`
	Enabled   bool   `json:"enabled"`
	Policy    string `json:"policy"`
}

// Library lists the owner's MafiaHub scripts and where each is installed.
func (a *App) Library(ctx context.Context) ([]ScriptRow, error) {
	key := a.key()
	if key == "" {
		return nil, errors.New("link a Latch account first")
	}
	items, err := latch.Library(ctx, key)
	if err != nil {
		a.keyFailed(err)
		return nil, err
	}
	var servers []store.Server
	servers = a.Store.Servers()
	var out []ScriptRow
	for _, it := range items {
		if it.Script.Ecosystem != "mafiahub" {
			continue
		}
		row := ScriptRow{Item: it, Servers: map[string]ScriptOnServer{}}
		for _, sv := range servers {
			s := ScriptOnServer{Policy: policyOf(sv, it.Script.ID)}
			if r, ok := latch.Installed(sv.Dir)[it.Script.ID]; ok {
				s.Installed, s.Folder, s.Enabled = r.Latch.Version, r.Folder, r.Enabled
			}
			row.Servers[sv.ID] = s
		}
		out = append(out, row)
	}
	return out, nil
}

func policyOf(sv store.Server, scriptID string) string {
	if p, ok := sv.ScriptPolicy[scriptID]; ok {
		return p
	}
	return store.ScriptManual
}

// InstallScript installs or updates a Library script on a server. With restart, a running server reloads it.
func (a *App) InstallScript(ctx context.Context, serverID, scriptID, user string, restart bool) (latch.InstallResult, error) {
	key := a.key()
	if key == "" {
		return latch.InstallResult{}, errors.New("link a Latch account first")
	}
	sv, ok := a.Store.Server(serverID)
	if !ok {
		return latch.InstallResult{}, store.ErrNotFound
	}
	items, err := latch.Library(ctx, key)
	if err != nil {
		a.keyFailed(err)
		return latch.InstallResult{}, err
	}
	for _, it := range items {
		if it.Script.ID != scriptID {
			continue
		}
		if !it.Usable() {
			return latch.InstallResult{}, errors.New("this script can't be installed (access revoked, not published, or not a MafiaHub script)")
		}
		res, err := latch.Install(ctx, key, sv.Dir, it, *it.Script.LatestVersion)
		if err != nil {
			a.keyFailed(err)
			return res, err
		}
		p := a.Proc(serverID)
		if !res.UpToDate {
			a.Store.Audit(user, "script.install", sv.Name, map[string]string{"script": it.Script.Slug, "version": res.Version})
			p.Panelf("Installed %s %s from Latch", it.Script.Slug, res.Version)
			if restart && p.Status().State == supervisor.Running {
				_ = p.Send("restart " + resourceName(sv.Dir, res.Folder))
			}
		}
		return res, nil
	}
	return latch.InstallResult{}, errors.New("this script isn't in your Library")
}

// resourceName is the name the server knows a resource by: its package.json name, else the folder.
func resourceName(dir, folder string) string {
	for id, r := range latch.Installed(dir) {
		_ = id
		if r.Folder == folder && r.Name != "" {
			return r.Name
		}
	}
	return folder
}

// SetScriptPolicy chooses how a script is kept up to date on a server.
func (a *App) SetScriptPolicy(serverID, scriptID, policy string) error {
	switch policy {
	case store.ScriptManual, store.ScriptRestart, store.ScriptNow:
	default:
		return errors.New("unknown policy")
	}
	return a.Store.UpdateServer(serverID, func(sv *store.Server) error {
		if sv.ScriptPolicy == nil {
			sv.ScriptPolicy = map[string]string{}
		}
		sv.ScriptPolicy[scriptID] = policy
		return nil
	})
}

// autoScripts installs new versions of scripts whose policy allows it.
func (a *App) autoScripts(ctx context.Context) {
	key := a.key()
	if key == "" {
		return
	}
	items, err := latch.Library(ctx, key)
	if err != nil {
		a.keyFailed(err)
		return
	}
	var servers []store.Server
	servers = a.Store.Servers()
	for _, sv := range servers {
		installed := latch.Installed(sv.Dir)
		for _, it := range items {
			pol := policyOf(sv, it.Script.ID)
			cur, ok := installed[it.Script.ID]
			if pol == store.ScriptManual || !ok || !it.Usable() || compare(*it.Script.LatestVersion, cur.Latch.Version) <= 0 {
				continue
			}
			res, err := a.InstallScript(ctx, sv.ID, it.Script.ID, "auto-update", pol == store.ScriptNow)
			if err == nil && !res.UpToDate {
				applies := "@appliesRestart"
				if pol == store.ScriptNow {
					applies = "@appliesNow"
				}
				a.Notify(notify.Event{Kind: "script.updated", ServerID: sv.ID, Server: sv.Name, Vars: map[string]string{"script": it.Script.Name, "version": res.Version, "applies": applies}})
			}
			if err != nil {
				a.Proc(sv.ID).Panelf("Automatic update of %s failed: %v", it.Script.Slug, err)
			}
		}
	}
}
