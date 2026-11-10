package web

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/heylatch/latchadmin/internal/app"
	"github.com/heylatch/latchadmin/internal/auth"
	"github.com/heylatch/latchadmin/internal/buildinfo"
	"github.com/heylatch/latchadmin/internal/games"
	"github.com/heylatch/latchadmin/internal/release"
	"github.com/heylatch/latchadmin/internal/resources"
	"github.com/heylatch/latchadmin/internal/secret"
	"github.com/heylatch/latchadmin/internal/store"
	"github.com/heylatch/latchadmin/internal/supervisor"
)

const (
	viewer = store.RoleViewer
	admin  = store.RoleAdmin
	owner  = store.RoleOwner
)

func (s *Server) routes() {
	m := s.mux
	// Session and setup
	m.HandleFunc("GET /api/session", s.api("", s.session))
	m.HandleFunc("POST /api/setup", s.api("", s.setup))
	m.HandleFunc("POST /api/login", s.api("", s.login))
	m.HandleFunc("POST /api/logout", s.api(viewer, s.logout))
	m.HandleFunc("POST /api/me/password", s.api(viewer, s.changePassword))
	m.HandleFunc("POST /api/me/totp/setup", s.api(viewer, s.totpSetup))
	m.HandleFunc("POST /api/me/totp", s.api(viewer, s.totpEnable))
	m.HandleFunc("DELETE /api/me/totp", s.api(viewer, s.totpDisable))
	// Users
	m.HandleFunc("GET /api/users", s.api(owner, s.listUsers))
	m.HandleFunc("POST /api/users", s.api(owner, s.createUser))
	m.HandleFunc("PATCH /api/users/{id}", s.api(owner, s.updateUser))
	m.HandleFunc("DELETE /api/users/{id}", s.api(owner, s.deleteUser))
	// Panel
	m.HandleFunc("GET /api/panel", s.api(viewer, s.panel))
	m.HandleFunc("PATCH /api/panel/settings", s.api(owner, s.panelSettings))
	m.HandleFunc("POST /api/panel/check", s.api(admin, s.panelCheck))
	m.HandleFunc("POST /api/panel/update", s.api(owner, s.panelUpdate))
	m.HandleFunc("GET /api/audit", s.api(admin, s.audit))
	m.HandleFunc("GET /api/browse", s.api(owner, s.browse))
	// Servers
	m.HandleFunc("GET /api/games", s.api(viewer, s.games))
	m.HandleFunc("GET /api/servers", s.api(viewer, s.listServers))
	m.HandleFunc("POST /api/servers", s.api(owner, s.installServer))
	m.HandleFunc("POST /api/servers/import/preview", s.api(owner, s.importPreview))
	m.HandleFunc("POST /api/servers/import", s.api(owner, s.importServer))
	m.HandleFunc("GET /api/servers/{id}", s.api(viewer, s.getServer))
	m.HandleFunc("PATCH /api/servers/{id}", s.api(admin, s.patchServer))
	m.HandleFunc("DELETE /api/servers/{id}", s.api(owner, s.deleteServer))
	m.HandleFunc("POST /api/servers/{id}/{action}", s.api(admin, s.serverAction))
	m.HandleFunc("POST /api/servers/{id}/command", s.api(admin, s.command))
	m.HandleFunc("GET /api/servers/{id}/console", s.api(viewer, s.console))
	m.HandleFunc("GET /api/servers/{id}/logs", s.api(viewer, s.logFiles))
	m.HandleFunc("GET /api/servers/{id}/logs/{file}", s.api(viewer, s.logFile))
	m.HandleFunc("GET /api/servers/{id}/resources", s.api(viewer, s.listResources))
	m.HandleFunc("POST /api/servers/{id}/resources", s.api(admin, s.uploadResource))
	m.HandleFunc("POST /api/servers/{id}/resources/{folder}/{action}", s.api(admin, s.resourceAction))
	m.HandleFunc("DELETE /api/servers/{id}/resources/{folder}", s.api(admin, s.removeResource))
	m.HandleFunc("GET /api/servers/{id}/config", s.api(admin, s.getConfig))
	m.HandleFunc("PUT /api/servers/{id}/config", s.api(admin, s.putConfig))
	m.HandleFunc("GET /api/servers/{id}/env", s.api(admin, s.getEnv))
	m.HandleFunc("PUT /api/servers/{id}/env", s.api(admin, s.putEnv))
	m.HandleFunc("GET /api/servers/{id}/data", s.api(admin, s.listData))
	m.HandleFunc("GET /api/servers/{id}/data/{file}", s.api(admin, s.getData))
	m.HandleFunc("PUT /api/servers/{id}/data/{file}", s.api(admin, s.putData))
	m.HandleFunc("GET /api/servers/{id}/backups", s.api(admin, s.backups))
	m.HandleFunc("POST /api/servers/{id}/backups/{name}/restore", s.api(admin, s.restore))
	m.HandleFunc("GET /api/jobs/{id}", s.api(viewer, s.job))
	// Latch
	m.HandleFunc("GET /api/latch", s.api(viewer, s.latchStatus))
	m.HandleFunc("POST /api/latch/link", s.api(owner, s.latchLink))
	m.HandleFunc("DELETE /api/latch/link", s.api(owner, s.latchCancelLink))
	m.HandleFunc("POST /api/latch/key", s.api(owner, s.latchKey))
	m.HandleFunc("DELETE /api/latch", s.api(owner, s.latchUnlink))
	m.HandleFunc("GET /api/latch/library", s.api(viewer, s.latchLibrary))
	m.HandleFunc("POST /api/servers/{id}/scripts/{script}/install", s.api(admin, s.scriptInstall))
	m.HandleFunc("PUT /api/servers/{id}/scripts/{script}/policy", s.api(admin, s.scriptPolicy))
	// UI
	m.HandleFunc("/", s.static)
}

// ---------- session ----------

func (s *Server) session(w http.ResponseWriter, r *http.Request, _ ctxUser) error {
	out := map[string]any{"setup": s.App.NeedsSetup(), "version": buildinfo.Version}
	if u, ok := s.user(r); ok {
		out["user"] = userView(u.User)
	}
	writeJSON(w, 200, out)
	return nil
}

func userView(u store.User) map[string]any {
	return map[string]any{"id": u.ID, "username": u.Username, "role": u.Role, "totp": u.TOTPSecret != "", "createdAt": u.CreatedAt}
}

var usernameRe = regexp.MustCompile(`^[A-Za-z0-9_.-]{3,32}$`)

func (s *Server) setup(w http.ResponseWriter, r *http.Request, _ ctxUser) error {
	var in struct{ PIN, Username, Password string }
	if err := decode(r, &in); err != nil {
		return err
	}
	if !s.App.NeedsSetup() {
		return fail(409, "done", "Setup is already finished. Sign in.")
	}
	ip := clientIP(r)
	if blocked, _ := s.setups.Blocked(ip); blocked {
		return fail(429, "rateLimited", "Too many tries. Wait a few minutes.")
	}
	ok, rotated := s.App.PIN.Check(in.PIN)
	if !ok {
		s.setups.Fail(ip)
		if rotated != "" && s.OnPIN != nil {
			s.OnPIN(rotated)
			return fail(400, "pinRotated", "Too many wrong PINs. A new PIN is in the panel's console.")
		}
		return fail(400, "badPin", "Wrong PIN. It's printed in the window where LatchAdmin runs.")
	}
	if !usernameRe.MatchString(in.Username) {
		return fail(400, "badUsername", "Username: 3 to 32 letters, numbers, dots, dashes or underscores.")
	}
	if err := auth.ValidPassword(in.Password); err != nil {
		return fail(400, "badPassword", "Password: "+err.Error()+".")
	}
	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		return err
	}
	u := store.User{ID: store.NewID(), Username: in.Username, PasswordHash: hash, Role: store.RoleOwner, CreatedAt: time.Now().UTC()}
	if err := s.App.Store.Update(func(st *store.State) error {
		if len(st.Users) > 0 {
			return fail(409, "done", "Setup is already finished.")
		}
		st.Users = append(st.Users, u)
		return nil
	}); err != nil {
		return err
	}
	s.App.PIN.Clear()
	s.App.Store.Audit(u.Username, "setup", "", map[string]string{"ip": ip})
	if err := s.newSession(w, r, u.ID); err != nil {
		return err
	}
	writeJSON(w, 200, map[string]any{"user": userView(u)})
	return nil
}

func (s *Server) login(w http.ResponseWriter, r *http.Request, _ ctxUser) error {
	var in struct{ Username, Password, Code string }
	if err := decode(r, &in); err != nil {
		return err
	}
	ip := clientIP(r)
	if blocked, wait := s.logins.Blocked(ip); blocked {
		return fail(429, "rateLimited", fmt.Sprintf("Too many failed sign-ins. Try again in %d minutes.", int(wait.Minutes())+1))
	}
	var u store.User
	found := false
	s.App.Store.Read(func(st *store.State) {
		for _, x := range st.Users {
			if strings.EqualFold(x.Username, in.Username) {
				u, found = x, true
			}
		}
	})
	// Hash even when the user doesn't exist, so timing doesn't reveal usernames.
	hash := u.PasswordHash
	if !found {
		hash = "$argon2id$v=19$m=65536,t=2,p=2$c2FsdHNhbHRzYWx0c2FsdA$AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	}
	if !auth.CheckPassword(hash, in.Password) || !found {
		s.logins.Fail(ip)
		s.App.Store.Audit(in.Username, "login.failed", "", map[string]string{"ip": ip})
		return fail(401, "badLogin", "Wrong username or password.")
	}
	if u.TOTPSecret != "" {
		if in.Code == "" {
			return fail(401, "codeRequired", "Enter the code from your authenticator app.")
		}
		if !auth.CheckTOTP(secret.MustOpen(u.TOTPSecret), in.Code) {
			s.logins.Fail(ip)
			return fail(401, "badCode", "That code is wrong or expired.")
		}
	}
	s.logins.Reset(ip)
	s.App.Store.Audit(u.Username, "login", "", map[string]string{"ip": ip})
	if err := s.newSession(w, r, u.ID); err != nil {
		return err
	}
	writeJSON(w, 200, map[string]any{"user": userView(u)})
	return nil
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request, u ctxUser) error {
	hash := auth.HashToken(u.token)
	_ = s.App.Store.Update(func(st *store.State) error {
		kept := st.Sessions[:0]
		for _, se := range st.Sessions {
			if se.TokenHash != hash {
				kept = append(kept, se)
			}
		}
		st.Sessions = kept
		return nil
	})
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteStrictMode})
	writeJSON(w, 200, map[string]bool{"ok": true})
	return nil
}

func (s *Server) changePassword(w http.ResponseWriter, r *http.Request, u ctxUser) error {
	var in struct{ Current, Next string }
	if err := decode(r, &in); err != nil {
		return err
	}
	if !auth.CheckPassword(u.PasswordHash, in.Current) {
		return fail(400, "badPassword", "Your current password is wrong.")
	}
	if err := auth.ValidPassword(in.Next); err != nil {
		return fail(400, "badPassword", "New password: "+err.Error()+".")
	}
	hash, err := auth.HashPassword(in.Next)
	if err != nil {
		return err
	}
	if err := s.setUser(u.ID, func(x *store.User) { x.PasswordHash = hash }); err != nil {
		return err
	}
	s.endSessions(u.ID, auth.HashToken(u.token)) // other devices sign out
	s.App.Store.Audit(u.Username, "password.change", "", nil)
	writeJSON(w, 200, map[string]bool{"ok": true})
	return nil
}

func (s *Server) setUser(id string, fn func(*store.User)) error {
	return s.App.Store.Update(func(st *store.State) error {
		for i := range st.Users {
			if st.Users[i].ID == id {
				fn(&st.Users[i])
				return nil
			}
		}
		return store.ErrNotFound
	})
}

// pending TOTP secrets, per user, until confirmed with a code
var (
	totpMu      sync.Mutex
	pendingTOTP = map[string]string{}
)

func (s *Server) totpSetup(w http.ResponseWriter, r *http.Request, u ctxUser) error {
	sec := auth.NewTOTPSecret()
	totpMu.Lock()
	pendingTOTP[u.ID] = sec
	totpMu.Unlock()
	writeJSON(w, 200, map[string]string{"secret": sec, "uri": auth.TOTPURI(sec, u.Username, "LatchAdmin")})
	return nil
}

func (s *Server) totpEnable(w http.ResponseWriter, r *http.Request, u ctxUser) error {
	var in struct{ Code string }
	if err := decode(r, &in); err != nil {
		return err
	}
	totpMu.Lock()
	sec := pendingTOTP[u.ID]
	totpMu.Unlock()
	if sec == "" || !auth.CheckTOTP(sec, in.Code) {
		return fail(400, "badCode", "That code doesn't match. Check the time on your phone and try again.")
	}
	sealed, err := secret.Seal(sec)
	if err != nil {
		return err
	}
	totpMu.Lock()
	delete(pendingTOTP, u.ID)
	totpMu.Unlock()
	if err := s.setUser(u.ID, func(x *store.User) { x.TOTPSecret = sealed }); err != nil {
		return err
	}
	s.App.Store.Audit(u.Username, "totp.on", "", nil)
	writeJSON(w, 200, map[string]bool{"ok": true})
	return nil
}

func (s *Server) totpDisable(w http.ResponseWriter, r *http.Request, u ctxUser) error {
	var in struct{ Password string }
	if err := decode(r, &in); err != nil {
		return err
	}
	if !auth.CheckPassword(u.PasswordHash, in.Password) {
		return fail(400, "badPassword", "Wrong password.")
	}
	if err := s.setUser(u.ID, func(x *store.User) { x.TOTPSecret = "" }); err != nil {
		return err
	}
	s.App.Store.Audit(u.Username, "totp.off", "", nil)
	writeJSON(w, 200, map[string]bool{"ok": true})
	return nil
}

// ---------- users ----------

func (s *Server) listUsers(w http.ResponseWriter, r *http.Request, _ ctxUser) error {
	var out []map[string]any
	s.App.Store.Read(func(st *store.State) {
		for _, u := range st.Users {
			out = append(out, userView(u))
		}
	})
	writeJSON(w, 200, out)
	return nil
}

func validRole(r store.Role) bool { return r == store.RoleOwner || r == store.RoleAdmin || r == store.RoleViewer }

func (s *Server) createUser(w http.ResponseWriter, r *http.Request, me ctxUser) error {
	var in struct {
		Username, Password string
		Role               store.Role
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	if !usernameRe.MatchString(in.Username) {
		return fail(400, "badUsername", "Username: 3 to 32 letters, numbers, dots, dashes or underscores.")
	}
	if !validRole(in.Role) {
		return badRequest(errors.New("unknown role"))
	}
	if err := auth.ValidPassword(in.Password); err != nil {
		return fail(400, "badPassword", "Password: "+err.Error()+".")
	}
	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		return err
	}
	u := store.User{ID: store.NewID(), Username: in.Username, PasswordHash: hash, Role: in.Role, CreatedAt: time.Now().UTC()}
	if err := s.App.Store.Update(func(st *store.State) error {
		for _, x := range st.Users {
			if strings.EqualFold(x.Username, u.Username) {
				return fail(409, "taken", "That username is taken.")
			}
		}
		st.Users = append(st.Users, u)
		return nil
	}); err != nil {
		return err
	}
	s.App.Store.Audit(me.Username, "user.create", u.Username, map[string]string{"role": string(u.Role)})
	writeJSON(w, 201, userView(u))
	return nil
}

func (s *Server) ownerCount() int {
	n := 0
	s.App.Store.Read(func(st *store.State) {
		for _, u := range st.Users {
			if u.Role == store.RoleOwner {
				n++
			}
		}
	})
	return n
}

func (s *Server) updateUser(w http.ResponseWriter, r *http.Request, me ctxUser) error {
	var in struct {
		Role     store.Role
		Password string
		ResetTOTP bool
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	id := r.PathValue("id")
	if in.Role != "" && !validRole(in.Role) {
		return badRequest(errors.New("unknown role"))
	}
	if in.Role != "" && in.Role != store.RoleOwner && id == me.ID && s.ownerCount() == 1 {
		return fail(400, "lastOwner", "You're the only owner. Make someone else owner first.")
	}
	var hash string
	if in.Password != "" {
		if err := auth.ValidPassword(in.Password); err != nil {
			return fail(400, "badPassword", "Password: "+err.Error()+".")
		}
		hash, _ = auth.HashPassword(in.Password)
	}
	var name string
	if err := s.setUser(id, func(x *store.User) {
		name = x.Username
		if in.Role != "" {
			x.Role = in.Role
		}
		if hash != "" {
			x.PasswordHash = hash
		}
		if in.ResetTOTP {
			x.TOTPSecret = ""
		}
	}); err != nil {
		return err
	}
	if hash != "" || in.ResetTOTP {
		s.endSessions(id, "")
	}
	s.App.Store.Audit(me.Username, "user.update", name, map[string]string{"role": string(in.Role), "password": fmt.Sprint(hash != ""), "resetTotp": fmt.Sprint(in.ResetTOTP)})
	writeJSON(w, 200, map[string]bool{"ok": true})
	return nil
}

func (s *Server) deleteUser(w http.ResponseWriter, r *http.Request, me ctxUser) error {
	id := r.PathValue("id")
	if id == me.ID {
		return fail(400, "self", "You can't remove yourself.")
	}
	var name string
	if err := s.App.Store.Update(func(st *store.State) error {
		for i, u := range st.Users {
			if u.ID == id {
				name = u.Username
				st.Users = append(st.Users[:i], st.Users[i+1:]...)
				return nil
			}
		}
		return store.ErrNotFound
	}); err != nil {
		return err
	}
	s.endSessions(id, "")
	s.App.Store.Audit(me.Username, "user.delete", name, nil)
	writeJSON(w, 200, map[string]bool{"ok": true})
	return nil
}

// ---------- panel ----------

func (s *Server) panel(w http.ResponseWriter, r *http.Request, _ ctxUser) error {
	var settings store.Settings
	var instance string
	s.App.Store.Read(func(st *store.State) { settings, instance = st.Settings, st.InstanceID })
	source, checked, verr := s.App.VersionsMeta()
	writeJSON(w, 200, map[string]any{
		"version": buildinfo.Version, "settings": settings, "instanceId": instance, "update": s.App.PanelUpdate(),
		"versions": map[string]any{"source": source, "checkedAt": checked, "error": verr},
	})
	return nil
}

func (s *Server) panelSettings(w http.ResponseWriter, r *http.Request, u ctxUser) error {
	var in struct {
		PanelName  *string
		SelfUpdate *string
		CheckLatch *bool
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	if err := s.App.Store.Update(func(st *store.State) error {
		if in.PanelName != nil {
			n := strings.TrimSpace(*in.PanelName)
			if n == "" || len(n) > 60 {
				return fail(400, "badName", "Panel name must be 1 to 60 characters.")
			}
			st.Settings.PanelName = n
		}
		if in.SelfUpdate != nil {
			switch *in.SelfUpdate {
			case "off", "urgent", "all":
				st.Settings.SelfUpdate = *in.SelfUpdate
			default:
				return badRequest(errors.New("unknown update policy"))
			}
		}
		if in.CheckLatch != nil {
			st.Settings.CheckLatch = *in.CheckLatch
		}
		return nil
	}); err != nil {
		return err
	}
	s.App.Store.Audit(u.Username, "settings.update", "", nil)
	return s.panel(w, r, u)
}

func (s *Server) panelCheck(w http.ResponseWriter, r *http.Request, u ctxUser) error {
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	s.App.CheckNow(ctx)
	return s.panel(w, r, u)
}

func (s *Server) panelUpdate(w http.ResponseWriter, r *http.Request, u ctxUser) error {
	var in struct{ Now bool }
	if err := decode(r, &in); err != nil {
		return err
	}
	pu := s.App.PanelUpdate()
	if pu.Dev {
		return fail(400, "dev", "This is a development build; it doesn't update itself.")
	}
	if pu.Pending == "" {
		if pu.Info.Latest == "" || app.Compare(pu.Info.Latest, pu.Current) <= 0 {
			return fail(400, "upToDate", "LatchAdmin is up to date.")
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
		defer cancel()
		if err := s.App.PrepareSelfUpdate(ctx, pu.Info.Latest); err != nil {
			return err
		}
	}
	s.App.Store.Audit(u.Username, "panel.update", pu.Info.Latest, map[string]string{"now": fmt.Sprint(in.Now)})
	if in.Now || !s.App.Running() {
		s.App.ApplyNow(context.Background())
		writeJSON(w, 202, map[string]string{"status": "restarting"})
		return nil
	}
	writeJSON(w, 200, map[string]string{"status": "queued"})
	return nil
}

func (s *Server) audit(w http.ResponseWriter, r *http.Request, _ ctxUser) error {
	writeJSON(w, 200, s.App.Store.AuditTail(300))
	return nil
}

func (s *Server) browse(w http.ResponseWriter, r *http.Request, _ ctxUser) error {
	l, err := app.Browse(r.URL.Query().Get("path"))
	if err != nil {
		return badRequest(err)
	}
	writeJSON(w, 200, l)
	return nil
}

// ---------- servers ----------

func (s *Server) games(w http.ResponseWriter, r *http.Request, _ ctxUser) error {
	type gameView struct {
		games.Game
		Latest string `json:"latest"`
	}
	var out []gameView
	for _, g := range games.All {
		out = append(out, gameView{g, s.App.Product(g.ID, "").Latest})
	}
	writeJSON(w, 200, out)
	return nil
}

type serverView struct {
	store.Server
	Env      []envView          `json:"env"`
	Status   supervisor.Status  `json:"status"`
	Update   app.ProductInfo    `json:"update"`
	Job      *app.Job           `json:"job,omitempty"`
	GameName string             `json:"gameName"`
}

type envView struct {
	Key    string `json:"key"`
	Value  string `json:"value"` // "" for secrets
	Secret bool   `json:"secret"`
	Set    bool   `json:"set"`
}

func (s *Server) view(sv store.Server) serverView {
	v := serverView{Server: sv, Status: s.App.Proc(sv.ID).Status(), Update: s.App.Product(sv.Game, sv.Version)}
	if g, ok := games.ByID(sv.Game); ok {
		v.GameName = g.Label
	}
	// Version reported by the running server wins when the panel doesn't know it (imported servers).
	if v.Version == "" && v.Status.Health != nil && v.Status.Health.ModVersion != "" {
		v.Version = v.Status.Health.ModVersion
		v.Update = s.App.Product(sv.Game, v.Version)
	}
	v.Env = make([]envView, 0, len(sv.Env))
	for _, e := range sv.Env {
		ev := envView{Key: e.Key, Secret: e.Secret, Set: e.Value != ""}
		if !e.Secret {
			ev.Value = e.Value
		}
		v.Env = append(v.Env, ev)
	}
	v.Server.Env = nil
	if j := s.App.Jobs.Active(sv.ID); j != nil {
		snap := j.Snapshot()
		v.Job = &snap
	}
	return v
}

func (s *Server) listServers(w http.ResponseWriter, r *http.Request, _ ctxUser) error {
	var list []store.Server
	s.App.Store.Read(func(st *store.State) { list = append(list, st.Servers...) })
	out := make([]serverView, 0, len(list))
	for _, sv := range list {
		out = append(out, s.view(sv))
	}
	writeJSON(w, 200, out)
	return nil
}

func (s *Server) server(r *http.Request) (store.Server, error) {
	sv, ok := s.App.Store.Server(r.PathValue("id"))
	if !ok {
		return sv, errNotFound
	}
	return sv, nil
}

func (s *Server) getServer(w http.ResponseWriter, r *http.Request, _ ctxUser) error {
	sv, err := s.server(r)
	if err != nil {
		return err
	}
	writeJSON(w, 200, s.view(sv))
	return nil
}

func (s *Server) installServer(w http.ResponseWriter, r *http.Request, u ctxUser) error {
	var in app.NewServer
	if err := decode(r, &in); err != nil {
		return err
	}
	j, err := s.App.InstallServer(in, u.Username)
	if err != nil {
		return badRequest(err)
	}
	writeJSON(w, 202, j.Snapshot())
	return nil
}

func (s *Server) importPreview(w http.ResponseWriter, r *http.Request, _ ctxUser) error {
	var in struct{ Dir string }
	if err := decode(r, &in); err != nil {
		return err
	}
	p, err := s.App.PreviewImport(in.Dir)
	if err != nil {
		return badRequest(err)
	}
	writeJSON(w, 200, p)
	return nil
}

func (s *Server) importServer(w http.ResponseWriter, r *http.Request, u ctxUser) error {
	var in struct{ Dir, Name string }
	if err := decode(r, &in); err != nil {
		return err
	}
	sv, err := s.App.ImportServer(in.Dir, in.Name, u.Username)
	if err != nil {
		return badRequest(err)
	}
	writeJSON(w, 201, s.view(sv))
	return nil
}

var hmRe = regexp.MustCompile(`^([01]\d|2[0-3]):[0-5]\d$`)

func (s *Server) patchServer(w http.ResponseWriter, r *http.Request, u ctxUser) error {
	var in struct {
		Name        *string
		Port        *int
		AutoStart   *bool
		AutoRestart *bool
		Restarts    *[]string
		GameUpdate  *string
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	sv, err := s.server(r)
	if err != nil {
		return err
	}
	if err := s.App.Store.UpdateServer(sv.ID, func(x *store.Server) error {
		if in.Name != nil {
			n := strings.TrimSpace(*in.Name)
			if n == "" || len(n) > 60 {
				return fail(400, "badName", "Name must be 1 to 60 characters.")
			}
			x.Name = n
		}
		if in.Port != nil {
			if *in.Port < 1 || *in.Port > 65535 {
				return fail(400, "badPort", "Port must be between 1 and 65535.")
			}
			x.Port = *in.Port
		}
		if in.AutoStart != nil {
			x.AutoStart = *in.AutoStart
		}
		if in.AutoRestart != nil {
			x.AutoRestart = *in.AutoRestart
		}
		if in.Restarts != nil {
			if len(*in.Restarts) > 12 {
				return fail(400, "tooMany", "At most 12 daily restarts.")
			}
			seen := map[string]bool{}
			var list []string
			for _, t := range *in.Restarts {
				if !hmRe.MatchString(t) {
					return fail(400, "badTime", "Restart times use 24-hour HH:MM.")
				}
				if !seen[t] {
					seen[t] = true
					list = append(list, t)
				}
			}
			sort.Strings(list)
			x.Restarts = list
		}
		if in.GameUpdate != nil {
			if *in.GameUpdate != "manual" && *in.GameUpdate != "restart" {
				return badRequest(errors.New("unknown game update policy"))
			}
			x.GameUpdate = *in.GameUpdate
		}
		return nil
	}); err != nil {
		return err
	}
	s.App.Store.Audit(u.Username, "server.settings", sv.Name, nil)
	return s.getServer(w, r, u)
}

func (s *Server) deleteServer(w http.ResponseWriter, r *http.Request, u ctxUser) error {
	sv, err := s.server(r)
	if err != nil {
		return err
	}
	if err := s.App.DeleteServer(sv.ID); err != nil {
		return badRequest(err)
	}
	s.App.Store.Audit(u.Username, "server.remove", sv.Name, map[string]string{"dir": sv.Dir})
	writeJSON(w, 200, map[string]bool{"ok": true})
	return nil
}

func (s *Server) serverAction(w http.ResponseWriter, r *http.Request, u ctxUser) error {
	sv, err := s.server(r)
	if err != nil {
		return err
	}
	p := s.App.Proc(sv.ID)
	action := r.PathValue("action")
	switch action {
	case "start":
		err = p.Start()
	case "stop":
		go func() { _ = p.Stop() }()
	case "restart":
		go func() { _ = p.Restart() }()
	case "update":
		var in struct{ Version string }
		if err := decode(r, &in); err != nil {
			return err
		}
		if in.Version == "" {
			return badRequest(errors.New("no version given"))
		}
		j, err := s.App.UpdateGame(sv.ID, in.Version, u.Username)
		if err != nil {
			return badRequest(err)
		}
		writeJSON(w, 202, j.Snapshot())
		return nil
	default:
		return errNotFound
	}
	if err != nil {
		return badRequest(err)
	}
	s.App.Store.Audit(u.Username, "server."+action, sv.Name, nil)
	writeJSON(w, 200, s.view(sv))
	return nil
}

func (s *Server) command(w http.ResponseWriter, r *http.Request, u ctxUser) error {
	var in struct{ Command string }
	if err := decode(r, &in); err != nil {
		return err
	}
	sv, err := s.server(r)
	if err != nil {
		return err
	}
	if err := s.App.Proc(sv.ID).Send(in.Command); err != nil {
		return badRequest(err)
	}
	s.App.Store.Audit(u.Username, "console.command", sv.Name, map[string]string{"command": in.Command})
	writeJSON(w, 200, map[string]bool{"ok": true})
	return nil
}

// console streams the server's output as server-sent events.
func (s *Server) console(w http.ResponseWriter, r *http.Request, _ ctxUser) error {
	sv, err := s.server(r)
	if err != nil {
		return err
	}
	fl, ok := w.(http.Flusher)
	if !ok {
		return errors.New("streaming unsupported")
	}
	p := s.App.Proc(sv.ID)
	recent, ch, cancel := p.Subscribe()
	defer cancel()
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-store")
	h.Set("X-Accel-Buffering", "no")
	send := func(event string, v any) {
		b, _ := json.Marshal(v)
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, b)
	}
	send("history", recent)
	send("status", p.Status())
	fl.Flush()
	tick := time.NewTicker(5 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-r.Context().Done():
			return nil
		case l := <-ch:
			send("line", l)
			// Drain bursts before flushing.
			for i := 0; i < 200; i++ {
				select {
				case l := <-ch:
					send("line", l)
					continue
				default:
				}
				break
			}
			fl.Flush()
		case <-tick.C:
			send("status", p.Status())
			fl.Flush()
		}
	}
}

func (s *Server) logFiles(w http.ResponseWriter, r *http.Request, _ ctxUser) error {
	sv, err := s.server(r)
	if err != nil {
		return err
	}
	entries, _ := os.ReadDir(s.App.Proc(sv.ID).LogDir())
	out := []map[string]any{}
	for i := len(entries) - 1; i >= 0; i-- {
		if info, err := entries[i].Info(); err == nil && strings.HasSuffix(info.Name(), ".log") {
			out = append(out, map[string]any{"name": info.Name(), "size": info.Size()})
		}
	}
	writeJSON(w, 200, out)
	return nil
}

var logNameRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}\.log$`)

func (s *Server) logFile(w http.ResponseWriter, r *http.Request, _ ctxUser) error {
	sv, err := s.server(r)
	if err != nil {
		return err
	}
	name := r.PathValue("file")
	if !logNameRe.MatchString(name) {
		return errNotFound
	}
	f, err := os.Open(filepath.Join(s.App.Proc(sv.ID).LogDir(), name))
	if err != nil {
		return errNotFound
	}
	defer f.Close()
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+sv.Name+"-"+name+`"`)
	_, _ = io.Copy(w, f)
	return nil
}

// ---------- resources ----------

func (s *Server) listResources(w http.ResponseWriter, r *http.Request, _ ctxUser) error {
	sv, err := s.server(r)
	if err != nil {
		return err
	}
	list, err := resources.List(sv.Dir)
	if err != nil {
		return err
	}
	if list == nil {
		list = []resources.Resource{}
	}
	writeJSON(w, 200, list)
	return nil
}

func (s *Server) uploadResource(w http.ResponseWriter, r *http.Request, u ctxUser) error {
	sv, err := s.server(r)
	if err != nil {
		return err
	}
	r.Body = http.MaxBytesReader(w, r.Body, 512<<20)
	file, hdr, err := r.FormFile("file")
	if err != nil {
		return badRequest(errors.New("upload a .zip file (up to 512 MB)"))
	}
	defer file.Close()
	data, err := io.ReadAll(file)
	if err != nil {
		return err
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return badRequest(errors.New("that file isn't a zip"))
	}
	folder, err := resources.Upload(sv.Dir, zr, hdr.Filename)
	if err != nil {
		return badRequest(err)
	}
	s.App.Store.Audit(u.Username, "resource.upload", sv.Name, map[string]string{"resource": folder})
	s.App.Proc(sv.ID).Panelf("Resource %s uploaded", folder)
	writeJSON(w, 201, map[string]string{"folder": folder})
	return nil
}

func (s *Server) resourceAction(w http.ResponseWriter, r *http.Request, u ctxUser) error {
	sv, err := s.server(r)
	if err != nil {
		return err
	}
	folder, action := r.PathValue("folder"), r.PathValue("action")
	if !resources.ValidFolder(folder) {
		return errNotFound
	}
	p := s.App.Proc(sv.ID)
	running := p.Status().State == supervisor.Running
	name := folder
	if list, err := resources.List(sv.Dir); err == nil {
		for _, res := range list {
			if res.Folder == folder && res.Name != "" {
				name = res.Name
			}
		}
	}
	switch action {
	case "enable":
		if err := resources.SetEnabled(sv.Dir, folder, true); err != nil {
			return badRequest(err)
		}
		if running {
			_ = p.Send("refresh")
			_ = p.Send("ensure " + name)
		}
	case "disable":
		if running {
			_ = p.Send("stop " + name)
		}
		if err := resources.SetEnabled(sv.Dir, folder, false); err != nil {
			return badRequest(err)
		}
	case "restart", "stop", "start", "ensure":
		if !running {
			return badRequest(errors.New("the server isn't running"))
		}
		if err := p.Send(action + " " + name); err != nil {
			return badRequest(err)
		}
	default:
		return errNotFound
	}
	s.App.Store.Audit(u.Username, "resource."+action, sv.Name, map[string]string{"resource": folder})
	writeJSON(w, 200, map[string]bool{"ok": true})
	return nil
}

func (s *Server) removeResource(w http.ResponseWriter, r *http.Request, u ctxUser) error {
	sv, err := s.server(r)
	if err != nil {
		return err
	}
	folder := r.PathValue("folder")
	p := s.App.Proc(sv.ID)
	if p.Status().State == supervisor.Running {
		_ = p.Send("stop " + folder)
	}
	dst, err := resources.Remove(sv.Dir, folder)
	if err != nil {
		return badRequest(err)
	}
	rel, _ := filepath.Rel(sv.Dir, dst)
	s.App.Store.Audit(u.Username, "resource.remove", sv.Name, map[string]string{"resource": folder, "movedTo": rel})
	writeJSON(w, 200, map[string]string{"movedTo": rel})
	return nil
}

// ---------- config, env, data ----------

func (s *Server) getConfig(w http.ResponseWriter, r *http.Request, _ ctxUser) error {
	sv, err := s.server(r)
	if err != nil {
		return err
	}
	raw, err := app.ReadConfig(sv.Dir)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(raw)
	return nil
}

func (s *Server) putConfig(w http.ResponseWriter, r *http.Request, u ctxUser) error {
	sv, err := s.server(r)
	if err != nil {
		return err
	}
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		return err
	}
	if err := app.WriteConfig(sv.Dir, raw); err != nil {
		return badRequest(err)
	}
	var c struct{ Port int `json:"port"` }
	if json.Unmarshal(raw, &c) == nil && c.Port > 0 && c.Port != sv.Port {
		_ = s.App.Store.UpdateServer(sv.ID, func(x *store.Server) error { x.Port = c.Port; return nil })
	}
	s.App.Store.Audit(u.Username, "config.save", sv.Name, map[string]string{"file": "server.json"})
	writeJSON(w, 200, map[string]bool{"ok": true})
	return nil
}

func (s *Server) getEnv(w http.ResponseWriter, r *http.Request, u ctxUser) error {
	sv, err := s.server(r)
	if err != nil {
		return err
	}
	writeJSON(w, 200, s.view(sv).Env)
	return nil
}

// putEnv replaces the env list. A secret sent without a value keeps its stored value.
func (s *Server) putEnv(w http.ResponseWriter, r *http.Request, u ctxUser) error {
	var in []struct {
		Key    string
		Value  *string
		Secret bool
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	if len(in) > 200 {
		return badRequest(errors.New("at most 200 variables"))
	}
	sv, err := s.server(r)
	if err != nil {
		return err
	}
	old := map[string]store.EnvVar{}
	for _, e := range sv.Env {
		old[e.Key] = e
	}
	seen := map[string]bool{}
	var next []store.EnvVar
	for _, e := range in {
		if !app.ValidEnvKey(e.Key) {
			return fail(400, "badKey", fmt.Sprintf("%q isn't a valid variable name (letters, numbers, underscores).", e.Key))
		}
		if seen[e.Key] {
			return fail(400, "duplicate", e.Key+" appears twice.")
		}
		seen[e.Key] = true
		var value string
		switch {
		case e.Value != nil:
			value = *e.Value
			if len(value) > 8192 || strings.ContainsAny(value, "\x00") {
				return fail(400, "badValue", e.Key+": value is too long or invalid.")
			}
			if e.Secret {
				if value, err = secret.Seal(value); err != nil {
					return err
				}
			}
		case e.Secret && old[e.Key].Secret:
			value = old[e.Key].Value
		case old[e.Key].Secret && !e.Secret:
			value = secret.MustOpen(old[e.Key].Value) // un-marking a secret keeps its value, now readable
		default:
			value = old[e.Key].Value
		}
		next = append(next, store.EnvVar{Key: e.Key, Value: value, Secret: e.Secret})
	}
	if err := s.App.Store.UpdateServer(sv.ID, func(x *store.Server) error { x.Env = next; return nil }); err != nil {
		return err
	}
	keys := make([]string, 0, len(next))
	for _, e := range next {
		keys = append(keys, e.Key)
	}
	s.App.Store.Audit(u.Username, "env.save", sv.Name, map[string]string{"keys": strings.Join(keys, ",")})
	return s.getEnv(w, r, u)
}

func (s *Server) listData(w http.ResponseWriter, r *http.Request, _ ctxUser) error {
	sv, err := s.server(r)
	if err != nil {
		return err
	}
	files, err := app.DataFiles(sv.Dir)
	if err != nil {
		return err
	}
	writeJSON(w, 200, files)
	return nil
}

func (s *Server) getData(w http.ResponseWriter, r *http.Request, _ ctxUser) error {
	sv, err := s.server(r)
	if err != nil {
		return err
	}
	raw, err := app.ReadDataFile(sv.Dir, r.PathValue("file"))
	if err != nil {
		return errNotFound
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(raw)
	return nil
}

func (s *Server) putData(w http.ResponseWriter, r *http.Request, u ctxUser) error {
	sv, err := s.server(r)
	if err != nil {
		return err
	}
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 4<<20))
	if err != nil {
		return err
	}
	if err := app.WriteDataFile(sv.Dir, r.PathValue("file"), raw); err != nil {
		return badRequest(err)
	}
	s.App.Store.Audit(u.Username, "config.save", sv.Name, map[string]string{"file": "data/" + r.PathValue("file")})
	writeJSON(w, 200, map[string]bool{"ok": true})
	return nil
}

func (s *Server) backups(w http.ResponseWriter, r *http.Request, _ ctxUser) error {
	sv, err := s.server(r)
	if err != nil {
		return err
	}
	list := release.Backups(sv.Dir)
	if list == nil {
		list = []release.Backup{}
	}
	writeJSON(w, 200, list)
	return nil
}

func (s *Server) restore(w http.ResponseWriter, r *http.Request, u ctxUser) error {
	sv, err := s.server(r)
	if err != nil {
		return err
	}
	if err := s.App.RestoreBackup(sv.ID, r.PathValue("name"), u.Username); err != nil {
		return badRequest(err)
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
	return nil
}

func (s *Server) job(w http.ResponseWriter, r *http.Request, _ ctxUser) error {
	j, ok := s.App.Jobs.Get(r.PathValue("id"))
	if !ok {
		return errNotFound
	}
	writeJSON(w, 200, j.Snapshot())
	return nil
}

// ---------- latch ----------

func (s *Server) latchStatus(w http.ResponseWriter, r *http.Request, u ctxUser) error {
	var l store.Latch
	var check bool
	s.App.Store.Read(func(st *store.State) { l, check = st.Latch, st.Settings.CheckLatch })
	out := map[string]any{
		"linked": l.Key != "", "disconnected": l.Disconnected, "accountId": l.AccountID, "displayName": l.DisplayName,
		"linkedAt": l.LinkedAt, "lastReportAt": l.LastReportAt, "checkLatch": check,
		// Exactly what goes to Latch on the next report, and what went last time.
		"nextReport": s.App.BuildReport(), "lastReport": l.LastReport,
		"link": s.App.LinkStatus(),
	}
	writeJSON(w, 200, out)
	return nil
}

func (s *Server) latchLink(w http.ResponseWriter, r *http.Request, u ctxUser) error {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	l, err := s.App.StartLink(ctx, u.Username)
	if err != nil {
		return badRequest(err)
	}
	writeJSON(w, 200, l)
	return nil
}

func (s *Server) latchCancelLink(w http.ResponseWriter, r *http.Request, _ ctxUser) error {
	s.App.CancelLink()
	writeJSON(w, 200, map[string]bool{"ok": true})
	return nil
}

func (s *Server) latchKey(w http.ResponseWriter, r *http.Request, u ctxUser) error {
	var in struct{ Key string }
	if err := decode(r, &in); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	if err := s.App.SetKey(ctx, in.Key, u.Username); err != nil {
		return badRequest(err)
	}
	return s.latchStatus(w, r, u)
}

func (s *Server) latchUnlink(w http.ResponseWriter, r *http.Request, u ctxUser) error {
	if err := s.App.Unlink(u.Username); err != nil {
		return err
	}
	return s.latchStatus(w, r, u)
}

func (s *Server) latchLibrary(w http.ResponseWriter, r *http.Request, _ ctxUser) error {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	rows, err := s.App.Library(ctx)
	if err != nil {
		return badRequest(err)
	}
	if rows == nil {
		rows = []app.ScriptRow{}
	}
	writeJSON(w, 200, rows)
	return nil
}

func (s *Server) scriptInstall(w http.ResponseWriter, r *http.Request, u ctxUser) error {
	var in struct{ Restart bool }
	if err := decode(r, &in); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()
	res, err := s.App.InstallScript(ctx, r.PathValue("id"), r.PathValue("script"), u.Username, in.Restart)
	if err != nil {
		return badRequest(err)
	}
	writeJSON(w, 200, res)
	return nil
}

func (s *Server) scriptPolicy(w http.ResponseWriter, r *http.Request, u ctxUser) error {
	var in struct{ Policy string }
	if err := decode(r, &in); err != nil {
		return err
	}
	if err := s.App.SetScriptPolicy(r.PathValue("id"), r.PathValue("script"), in.Policy); err != nil {
		return badRequest(err)
	}
	s.App.Store.Audit(u.Username, "script.policy", r.PathValue("id"), map[string]string{"script": r.PathValue("script"), "policy": in.Policy})
	writeJSON(w, 200, map[string]bool{"ok": true})
	return nil
}

// ---------- UI ----------

func (s *Server) static(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		writeErr(w, errNotFound)
		return
	}
	if s.UI == nil {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = io.WriteString(w, "LatchAdmin "+buildinfo.Version+": the web UI wasn't built into this binary. Run `npm run build` in ui/ and rebuild.")
		return
	}
	p := strings.TrimPrefix(r.URL.Path, "/")
	if p == "" {
		p = "index.html"
	}
	if _, err := fs.Stat(s.UI, p); err != nil {
		p = "index.html" // client-side routes
	}
	if strings.HasPrefix(p, "assets/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "no-cache")
	}
	http.ServeFileFS(w, r, s.UI, p)
}
