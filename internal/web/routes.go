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
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/heylatch/latchadmin/internal/app"
	"github.com/heylatch/latchadmin/internal/auth"
	"github.com/heylatch/latchadmin/internal/bridge"
	"github.com/heylatch/latchadmin/internal/buildinfo"
	"github.com/heylatch/latchadmin/internal/dbsetup"
	"github.com/heylatch/latchadmin/internal/games"
	"github.com/heylatch/latchadmin/internal/metrics"
	"github.com/heylatch/latchadmin/internal/notify"
	"github.com/heylatch/latchadmin/internal/perms"
	"github.com/heylatch/latchadmin/internal/release"
	"github.com/heylatch/latchadmin/internal/resources"
	"github.com/heylatch/latchadmin/internal/secret"
	"github.com/heylatch/latchadmin/internal/store"
	"github.com/heylatch/latchadmin/internal/supervisor"
)

func (s *Server) routes() {
	m := s.mux
	// Session and setup
	m.HandleFunc("GET /api/session", s.api(public, s.session))
	m.HandleFunc("POST /api/setup", s.api(public, s.setup))
	m.HandleFunc("POST /api/login", s.api(public, s.login))
	m.HandleFunc("POST /api/logout", s.api(signedIn, s.logout))
	m.HandleFunc("POST /api/me/password", s.api(signedIn, s.changePassword))
	m.HandleFunc("POST /api/me/totp/setup", s.api(signedIn, s.totpSetup))
	m.HandleFunc("POST /api/me/totp", s.api(signedIn, s.totpEnable))
	m.HandleFunc("DELETE /api/me/totp", s.api(signedIn, s.totpDisable))
	// Users
	m.HandleFunc("GET /api/permissions", s.api(signedIn, s.permissionList))
	m.HandleFunc("GET /api/users", s.api("panel.users", s.listUsers))
	m.HandleFunc("POST /api/users", s.api("panel.users", s.createUser))
	m.HandleFunc("PATCH /api/users/{id}", s.api("panel.users", s.updateUser))
	m.HandleFunc("DELETE /api/users/{id}", s.api("panel.users", s.deleteUser))
	// Panel
	m.HandleFunc("GET /api/panel", s.api(signedIn, s.panel))
	m.HandleFunc("PATCH /api/panel/settings", s.api("panel.settings", s.panelSettings))
	m.HandleFunc("POST /api/panel/check", s.api("panel.update", s.panelCheck))
	m.HandleFunc("POST /api/panel/update", s.api("panel.update", s.panelUpdate))
	m.HandleFunc("GET /api/audit", s.api("panel.activity", s.audit))
	m.HandleFunc("GET /api/browse", s.api(perms.All, s.browse)) // lists the whole disk: only for full access
	m.HandleFunc("GET /api/stats", s.api(signedIn, s.stats))
	// Discord notifications
	m.HandleFunc("GET /api/webhooks", s.api("panel.settings", s.listWebhooks))
	m.HandleFunc("POST /api/webhooks", s.api("panel.settings", s.saveWebhook))
	m.HandleFunc("PATCH /api/webhooks/{wid}", s.api("panel.settings", s.saveWebhook))
	m.HandleFunc("DELETE /api/webhooks/{wid}", s.api("panel.settings", s.deleteWebhook))
	m.HandleFunc("POST /api/webhooks/{wid}/test", s.api("panel.settings", s.testWebhook))
	m.HandleFunc("GET /api/status-boards", s.api("panel.settings", s.listBoards))
	m.HandleFunc("POST /api/status-boards", s.api("panel.settings", s.saveBoard))
	m.HandleFunc("PATCH /api/status-boards/{bid}", s.api("panel.settings", s.saveBoard))
	m.HandleFunc("DELETE /api/status-boards/{bid}", s.api("panel.settings", s.deleteBoard))
	m.HandleFunc("POST /api/status-boards/{bid}/refresh", s.api("panel.settings", s.refreshBoard))
	// Servers
	m.HandleFunc("GET /api/games", s.api(signedIn, s.games))
	m.HandleFunc("GET /api/servers", s.api(signedIn, s.listServers))
	m.HandleFunc("POST /api/servers", s.api("server.add", s.installServer))
	// Importing runs whatever binary sits in the chosen folder, so it needs every permission (like the folder browser).
	m.HandleFunc("POST /api/servers/import/preview", s.api(perms.All, s.importPreview))
	m.HandleFunc("POST /api/servers/import", s.api(perms.All, s.importServer))
	m.HandleFunc("GET /api/servers/{id}", s.api(signedIn, s.getServer))
	m.HandleFunc("PATCH /api/servers/{id}", s.api("server.settings", s.patchServer))
	m.HandleFunc("DELETE /api/servers/{id}", s.api("server.remove", s.deleteServer))
	m.HandleFunc("POST /api/servers/{id}/{action}", s.api(signedIn, s.serverAction)) // permission checked per action
	m.HandleFunc("POST /api/servers/{id}/command", s.api("console.write", s.command))
	m.HandleFunc("POST /api/servers/{id}/announce", s.api("console.write", s.announce))
	m.HandleFunc("GET /api/servers/{id}/resmon", s.api(signedIn, s.resmon))
	m.HandleFunc("GET /api/servers/{id}/foundations", s.api("resources.manage", s.foundationsStatus))
	m.HandleFunc("POST /api/servers/{id}/foundations", s.api("resources.manage", s.installFoundations))
	// The panel connects to whatever database host is entered, so only accounts with every permission set one up.
	m.HandleFunc("POST /api/servers/{id}/database", s.api(perms.All, s.setDatabase))
	m.HandleFunc("GET /api/servers/{id}/console", s.api("console.view", s.console))
	m.HandleFunc("GET /api/servers/{id}/logs", s.api("console.view", s.logFiles))
	m.HandleFunc("GET /api/servers/{id}/logs/{file}", s.api("console.view", s.logFile))
	m.HandleFunc("GET /api/servers/{id}/stats", s.api(signedIn, s.serverStats))
	m.HandleFunc("GET /api/servers/{id}/resources", s.api(signedIn, s.listResources))
	m.HandleFunc("POST /api/servers/{id}/resources", s.api("resources.manage", s.uploadResource))
	m.HandleFunc("POST /api/servers/{id}/resources/{folder}/{action}", s.api("resources.manage", s.resourceAction))
	m.HandleFunc("DELETE /api/servers/{id}/resources/{folder}", s.api("resources.manage", s.removeResource))
	m.HandleFunc("GET /api/servers/{id}/config", s.api("config.view", s.getConfig))
	m.HandleFunc("PUT /api/servers/{id}/config", s.api("config.edit", s.putConfig))
	m.HandleFunc("PUT /api/servers/{id}/token", s.api("config.edit", s.putToken))
	m.HandleFunc("GET /api/servers/{id}/env", s.api("config.view", s.getEnv))
	m.HandleFunc("PUT /api/servers/{id}/env", s.api("config.edit", s.putEnv))
	m.HandleFunc("GET /api/servers/{id}/data", s.api("config.view", s.listData))
	m.HandleFunc("GET /api/servers/{id}/data/{file}", s.api("config.view", s.getData))
	m.HandleFunc("PUT /api/servers/{id}/data/{file}", s.api("config.edit", s.putData))
	m.HandleFunc("GET /api/servers/{id}/backups", s.api("updates.game", s.backups))
	m.HandleFunc("POST /api/servers/{id}/backups/{name}/restore", s.api("updates.game", s.restore))
	m.HandleFunc("GET /api/jobs/{id}", s.api(signedIn, s.job))
	// Latch
	m.HandleFunc("GET /api/latch", s.api(signedIn, s.latchStatus))
	m.HandleFunc("POST /api/latch/link", s.api("panel.latch", s.latchLink))
	m.HandleFunc("DELETE /api/latch/link", s.api("panel.latch", s.latchCancelLink))
	m.HandleFunc("POST /api/latch/key", s.api("panel.latch", s.latchKey))
	m.HandleFunc("DELETE /api/latch", s.api("panel.latch", s.latchUnlink))
	m.HandleFunc("GET /api/latch/library", s.api(signedIn, s.latchLibrary))
	m.HandleFunc("POST /api/servers/{id}/scripts/{script}/install", s.api("updates.scripts", s.scriptInstall))
	m.HandleFunc("PUT /api/servers/{id}/scripts/{script}/policy", s.api("updates.scripts", s.scriptPolicy))
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
	servers := u.Servers
	if servers == nil {
		servers = []string{}
	}
	return map[string]any{"id": u.ID, "username": u.Username, "permissions": u.Permissions, "allServers": u.AllServers || perms.Has(u.Permissions, perms.All), "servers": servers, "totp": u.TOTPSecret != "", "createdAt": u.CreatedAt}
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
	// The lock never applies on the machine itself, so a stranger can't keep the owner out of setup.
	if s.setupFails.Load() >= maxSetupFails && !isLocal(ip) {
		return fail(423, "setupLocked", "Setup is locked after too many wrong PINs. Open the panel on this machine (localhost), or restart LatchAdmin.")
	}
	if ok, _ := s.setups.Take(limitKey(ip)); !ok {
		return fail(429, "rateLimited", "Too many tries. Wait a few minutes.")
	}
	ok, rotated := s.App.PIN.Check(in.PIN)
	if !ok {
		s.setupFails.Add(1)
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
	u := store.User{ID: store.NewID(), Username: in.Username, PasswordHash: hash, Permissions: []string{perms.All}, AllServers: true, CreatedAt: time.Now().UTC()}
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
	// Every attempt is counted before the password is hashed; a successful one is refunded below.
	ipKey, nameKey := limitKey(ip), strings.ToLower(strings.TrimSpace(in.Username))
	if ok, wait := s.logins.Take(ipKey); !ok {
		return fail(429, "rateLimited", fmt.Sprintf("Too many failed sign-ins. Try again in %d minutes.", int(wait.Minutes())+1))
	}
	// Per account, but not on the machine itself: a stranger hammering a username can't lock its owner out of
	// http://localhost:40120 (or an SSH tunnel).
	if !isLocal(ip) {
		if ok, wait := s.names.Take(nameKey); !ok {
			return fail(429, "rateLimited", fmt.Sprintf("Too many failed sign-ins for this account. Try again in %d minutes, or sign in on the machine itself.", int(wait.Minutes())+1))
		}
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
		s.App.Store.Audit(in.Username, "login.failed", "", map[string]string{"ip": ip})
		return fail(401, "badLogin", "Wrong username or password.")
	}
	if u.TOTPSecret != "" {
		if in.Code == "" {
			return fail(401, "codeRequired", "Enter the code from your authenticator app.")
		}
		sec, err := secret.Open(u.TOTPSecret)
		if err != nil || sec == "" {
			// Never fall back to "no 2FA" when the secret can't be read (e.g. data copied to another account).
			return fail(401, "totpUnreadable", "Two-factor settings can't be read on this machine. Ask an owner to reset your 2FA.")
		}
		step, ok := auth.MatchTOTP(sec, in.Code, u.TOTPLastStep)
		if !ok {
			return fail(401, "badCode", "That code is wrong, expired or already used.")
		}
		// Check and record under the store's lock: two requests with the same code can't both get in.
		if err := s.App.Store.Update(func(st *store.State) error {
			for i := range st.Users {
				if st.Users[i].ID == u.ID {
					if st.Users[i].TOTPLastStep >= step {
						return fail(401, "badCode", "That code is wrong, expired or already used.")
					}
					st.Users[i].TOTPLastStep = step
				}
			}
			return nil
		}); err != nil {
			return err
		}
	}
	s.logins.Refund(ipKey)
	if !isLocal(ip) {
		s.names.Refund(nameKey)
	}
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
	if u.TOTPSecret != "" {
		return fail(400, "totpOn", "Two-factor authentication is already on. Turn it off first (that asks for your password).")
	}
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

func (s *Server) permissionList(w http.ResponseWriter, r *http.Request, _ ctxUser) error {
	writeJSON(w, 200, perms.Groups)
	return nil
}

// grantable checks a permission set: known permissions only, and none the granting user lacks.
func grantable(me ctxUser, set []string) ([]string, error) {
	clean := perms.Clean(set)
	for _, p := range clean {
		if !perms.Has(me.Permissions, p) {
			return nil, fail(403, "cantGrant", "You can't give a permission you don't have yourself.")
		}
	}
	return clean, nil
}

// outranks: a user may only change someone whose permissions and server access they hold themselves, so nobody
// can take over (reset the password of) an account more powerful than their own.
func outranks(me ctxUser, target store.User) bool {
	for _, p := range target.Permissions {
		if !perms.Has(me.Permissions, p) {
			return false
		}
	}
	if target.AllServers || perms.Has(target.Permissions, perms.All) {
		return me.AllServers || perms.Has(me.Permissions, perms.All)
	}
	for _, id := range target.Servers {
		if !me.CanServer(id) {
			return false
		}
	}
	return true
}

// setUserChecked changes a user and, in the same store update, refuses a change that leaves nobody with all
// permissions.
func (s *Server) setUserChecked(id string, fn func(*store.User), lastAll *bool) error {
	return s.App.Store.Update(func(st *store.State) error {
		found := false
		for i := range st.Users {
			if st.Users[i].ID == id {
				fn(&st.Users[i])
				found = true
			}
		}
		if !found {
			return store.ErrNotFound
		}
		if *lastAll {
			for _, u := range st.Users {
				if perms.Has(u.Permissions, perms.All) {
					return nil
				}
			}
			return fail(400, "lastAll", "Someone must keep all permissions. Give them to another user first.")
		}
		return nil
	})
}

// grantCreator gives a user with limited server access the server they just added.
func (s *Server) grantCreator(u ctxUser, id string) {
	if u.CanServer(id) {
		return
	}
	_ = s.setUser(u.ID, func(x *store.User) { x.Servers = append(x.Servers, id) })
}

// grantableServers checks a server list: existing servers only, and only ones the granting user can access.
func (s *Server) grantableServers(me ctxUser, all bool, list []string) ([]string, error) {
	if all {
		if !me.AllServers && !perms.Has(me.Permissions, perms.All) {
			return nil, fail(403, "cantGrant", "You can't give access to all servers: you don't have it yourself.")
		}
		return []string{}, nil
	}
	out := []string{}
	seen := map[string]bool{}
	for _, id := range list {
		if seen[id] {
			continue
		}
		if _, ok := s.App.Store.Server(id); !ok {
			continue
		}
		if !me.CanServer(id) {
			return nil, fail(403, "cantGrant", "You can't give access to a server you can't access yourself.")
		}
		seen[id] = true
		out = append(out, id)
	}
	return out, nil
}

func (s *Server) createUser(w http.ResponseWriter, r *http.Request, me ctxUser) error {
	var in struct {
		Username, Password string
		Permissions        []string
		AllServers         bool
		Servers            []string
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	if !usernameRe.MatchString(in.Username) {
		return fail(400, "badUsername", "Username: 3 to 32 letters, numbers, dots, dashes or underscores.")
	}
	set, err := grantable(me, in.Permissions)
	if err != nil {
		return err
	}
	if err := auth.ValidPassword(in.Password); err != nil {
		return fail(400, "badPassword", "Password: "+err.Error()+".")
	}
	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		return err
	}
	servers, err := s.grantableServers(me, in.AllServers, in.Servers)
	if err != nil {
		return err
	}
	u := store.User{ID: store.NewID(), Username: in.Username, PasswordHash: hash, Permissions: set, AllServers: in.AllServers, Servers: servers, CreatedAt: time.Now().UTC()}
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
	s.App.Store.Audit(me.Username, "user.create", u.Username, map[string]string{"permissions": strings.Join(set, ",")})
	writeJSON(w, 201, userView(u))
	return nil
}

func (s *Server) updateUser(w http.ResponseWriter, r *http.Request, me ctxUser) error {
	var in struct {
		Permissions *[]string
		AllServers  *bool
		Servers     []string
		Password    string
		ResetTOTP   bool
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	id := r.PathValue("id")
	var set []string
	if in.Permissions != nil {
		var err error
		if set, err = grantable(me, *in.Permissions); err != nil {
			return err
		}

	}
	var hash string
	if in.Password != "" {
		if err := auth.ValidPassword(in.Password); err != nil {
			return fail(400, "badPassword", "Password: "+err.Error()+".")
		}
		hash, _ = auth.HashPassword(in.Password)
	}
	var name string
	var target store.User
	s.App.Store.Read(func(st *store.State) {
		for _, u := range st.Users {
			if u.ID == id {
				target = u
			}
		}
	})
	if target.ID == "" {
		return errNotFound
	}
	if !outranks(me, target) {
		return errForbidden
	}
	var servers []string
	if in.AllServers != nil {
		var err error
		if servers, err = s.grantableServers(me, *in.AllServers, in.Servers); err != nil {
			return err
		}
	}
	lastAll := false
	if err := s.setUserChecked(id, func(x *store.User) {
		name = x.Username
		if in.Permissions != nil {
			x.Permissions = set
		}
		lastAll = in.Permissions != nil && !perms.Has(set, perms.All)
		if in.AllServers != nil {
			x.AllServers, x.Servers = *in.AllServers, servers
		}
		if hash != "" {
			x.PasswordHash = hash
		}
		if in.ResetTOTP {
			x.TOTPSecret = ""
		}
	}, &lastAll); err != nil {
		return err
	}
	if hash != "" || in.ResetTOTP {
		s.endSessions(id, "")
	}
	detail := map[string]string{"password": fmt.Sprint(hash != ""), "resetTotp": fmt.Sprint(in.ResetTOTP)}
	if in.Permissions != nil {
		detail["permissions"] = strings.Join(set, ",")
	}
	s.App.Store.Audit(me.Username, "user.update", name, detail)
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
				if !outranks(me, u) {
					return errForbidden
				}
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
		PublicURL  *string
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
		if in.PublicURL != nil {
			u := strings.TrimRight(strings.TrimSpace(*in.PublicURL), "/")
			if u != "" {
				p, err := url.Parse(u)
				if err != nil || (p.Scheme != "https" && p.Scheme != "http") || p.Host == "" || p.User != nil || p.RawQuery != "" || p.Fragment != "" {
					return fail(400, "badUrl", "Enter the address you open the panel at, like https://panel.example.com.")
				}
			}
			st.Settings.PublicURL = u
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
		Latest      string `json:"latest"`
		Installable bool   `json:"installable"` // a public release exists and its binary name is known
	}
	var out []gameView
	for _, g := range games.All {
		latest := s.App.Product(g.ID, "").Latest
		out = append(out, gameView{g, latest, g.Exe != ""})
	}
	writeJSON(w, 200, out)
	return nil
}

type serverView struct {
	store.Server
	Env      []envView            `json:"env"`
	Status   supervisor.Status    `json:"status"`
	Update   app.ProductInfo      `json:"update"`
	Job      *app.Job             `json:"job,omitempty"`
	GameName string               `json:"gameName"`
	TokenSet bool                 `json:"tokenSet"`
	Commands []supervisor.Command `json:"commands"`
	Notices  noticesView          `json:"notices"`
}

type noticesView struct {
	Enabled   bool   `json:"enabled"`
	Connected bool   `json:"connected"`
	Version   string `json:"version"`
	Warnings  []int  `json:"warnings"`
	Lang      string `json:"lang"`
}

type envView struct {
	Key    string `json:"key"`
	Value  string `json:"value"` // "" for secrets
	Secret bool   `json:"secret"`
	Set    bool   `json:"set"`
}

// view builds the server JSON. Env values are included only for admins and owners.
func (s *Server) view(sv store.Server, set []string) serverView {
	warnings := sv.RestartWarnings
	if warnings == nil {
		warnings = app.DefaultRestartWarnings
	}
	lang := sv.NoticeLang
	if lang == "" {
		lang = "en"
	}
	notices := noticesView{Enabled: !sv.NoticesOff, Connected: s.App.NoticesConnected(sv.ID), Version: bridge.Version(), Warnings: warnings, Lang: lang}
	sv.BridgeKey = "" // the bridge's key never leaves the panel
	v := serverView{Server: sv, Status: s.App.Proc(sv.ID).Status(), Update: s.App.Product(sv.Game, sv.Version), Notices: notices}
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
		if !perms.Has(set, "config.view") {
			break
		}
		ev := envView{Key: e.Key, Secret: e.Secret, Set: e.Value != ""}
		if !e.Secret {
			ev.Value = e.Value
		}
		v.Env = append(v.Env, ev)
	}
	v.Server.Env = nil
	v.Commands = s.App.Proc(sv.ID).Commands()
	v.TokenSet = app.TokenSet(sv.Dir)
	if j := s.App.Jobs.Active(sv.ID); j != nil {
		snap := j.Snapshot()
		v.Job = &snap
	}
	return v
}

func (s *Server) listServers(w http.ResponseWriter, r *http.Request, u ctxUser) error {
	var list []store.Server
	list = s.App.Store.Servers()
	out := make([]serverView, 0, len(list))
	for _, sv := range list {
		if !u.CanServer(sv.ID) {
			continue
		}
		out = append(out, s.view(sv, u.Permissions))
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

func (s *Server) getServer(w http.ResponseWriter, r *http.Request, u ctxUser) error {
	sv, err := s.server(r)
	if err != nil {
		return err
	}
	writeJSON(w, 200, s.view(sv, u.Permissions))
	return nil
}

func (s *Server) installServer(w http.ResponseWriter, r *http.Request, u ctxUser) error {
	var in app.NewServer
	if err := decode(r, &in); err != nil {
		return err
	}
	// The database is set up first, so a wrong password shows on the form instead of after a long download.
	var db *dbsetup.Conn
	if in.Database != nil && in.Foundations {
		if !perms.Has(u.Permissions, perms.All) {
			return fail(403, "forbidden", "Setting up a database needs every permission.")
		}
		c, err := s.App.PrepareDatabase(r.Context(), *in.Database)
		if err != nil {
			return fail(400, "database", sentence("Database: "+err.Error()))
		}
		db = &c
	}
	j, err := s.App.InstallServer(in, db, u.Username, func(id string) { s.grantCreator(u, id) })
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
	var in struct{ Dir, Name, Game, Exe string }
	if err := decode(r, &in); err != nil {
		return err
	}
	sv, err := s.App.ImportServer(in.Dir, in.Name, in.Game, in.Exe, u.Username)
	if err != nil {
		return badRequest(err)
	}
	s.grantCreator(u, sv.ID)
	writeJSON(w, 201, s.view(sv, u.Permissions))
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
		NoticesOff  *bool
		Warnings    *[]int
		NoticeLang  *string
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
		if in.NoticesOff != nil {
			x.NoticesOff = *in.NoticesOff
		}
		if in.Warnings != nil {
			seen := map[int]bool{}
			list := []int{}
			for _, m := range *in.Warnings {
				if m < 1 || m > 60 {
					return fail(400, "badWarning", "Restart warnings are 1 to 60 minutes before the restart.")
				}
				if !seen[m] {
					seen[m] = true
					list = append(list, m)
				}
			}
			if len(list) > 5 {
				return fail(400, "tooMany", "At most 5 restart warnings.")
			}
			sort.Sort(sort.Reverse(sort.IntSlice(list)))
			x.RestartWarnings = list
		}
		if in.NoticeLang != nil {
			if *in.NoticeLang != "en" && *in.NoticeLang != "tr" {
				return badRequest(errors.New("notice language must be en or tr"))
			}
			x.NoticeLang = *in.NoticeLang
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
	need := "server.control"
	if action == "update" {
		need = "updates.game"
	}
	if !perms.Has(u.Permissions, need) {
		return errForbidden
	}
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
	writeJSON(w, 200, s.view(sv, u.Permissions))
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
			if cur, ok := s.user(r); !ok || !perms.Has(cur.Permissions, "console.view") || !cur.CanServer(sv.ID) {
				return nil
			}
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
	var c struct {
		Port int `json:"port"`
	}
	if json.Unmarshal(raw, &c) == nil && c.Port > 0 && c.Port != sv.Port {
		_ = s.App.Store.UpdateServer(sv.ID, func(x *store.Server) error { x.Port = c.Port; return nil })
	}
	s.App.Store.Audit(u.Username, "config.save", sv.Name, map[string]string{"file": "server.json"})
	writeJSON(w, 200, map[string]bool{"ok": true})
	return nil
}

func (s *Server) putToken(w http.ResponseWriter, r *http.Request, u ctxUser) error {
	var in struct{ Token string }
	if err := decode(r, &in); err != nil {
		return err
	}
	sv, err := s.server(r)
	if err != nil {
		return err
	}
	if err := app.SetToken(sv.Dir, in.Token); err != nil {
		return badRequest(err)
	}
	action := "token.set"
	if strings.TrimSpace(in.Token) == "" {
		action = "token.clear"
	}
	s.App.Store.Audit(u.Username, action, sv.Name, nil)
	writeJSON(w, 200, map[string]bool{"ok": true, "tokenSet": app.TokenSet(sv.Dir)})
	return nil
}

func (s *Server) getEnv(w http.ResponseWriter, r *http.Request, u ctxUser) error {
	sv, err := s.server(r)
	if err != nil {
		return err
	}
	writeJSON(w, 200, s.view(sv, u.Permissions).Env)
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
		if app.ReservedEnvKey(e.Key) {
			return fail(400, "reservedKey", e.Key+" can't be set here: it changes how the server program loads code.")
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
			// Secrets are write-only: showing one needs its value typed again, or anyone with config.edit could
			// read every secret by un-marking it.
			return fail(400, "secretValueNeeded", e.Key+": enter the value again to stop keeping it secret.")
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

func (s *Server) job(w http.ResponseWriter, r *http.Request, u ctxUser) error {
	j, ok := s.App.Jobs.Get(r.PathValue("id"))
	if !ok {
		return errNotFound
	}
	if sid := j.Snapshot().ServerID; sid != "" && !u.CanServer(sid) {
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
		"nextReport": nil, "lastReport": nil,
		"link": nil,
	}
	// The report covers every server and the link code is for linking: only for those who manage the link.
	if perms.Has(u.Permissions, "panel.latch") {
		out["nextReport"], out["lastReport"], out["link"] = s.App.BuildReport(), l.LastReport, s.App.LinkStatus()
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

func (s *Server) latchLibrary(w http.ResponseWriter, r *http.Request, u ctxUser) error {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	rows, err := s.App.Library(ctx)
	if err != nil {
		return badRequest(err)
	}
	if rows == nil {
		rows = []app.ScriptRow{}
	}
	for _, row := range rows {
		for id := range row.Servers {
			if !u.CanServer(id) {
				delete(row.Servers, id)
			}
		}
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

// ---------- statistics ----------

var ranges = map[string]time.Duration{"1h": time.Hour, "6h": 6 * time.Hour, "24h": 24 * time.Hour, "7d": 7 * 24 * time.Hour}

const chartPoints = 240

func rangeFrom(r *http.Request) (int64, string) {
	key := r.URL.Query().Get("range")
	d, ok := ranges[key]
	if !ok {
		key, d = "24h", ranges["24h"]
	}
	return time.Now().Add(-d).Unix(), key
}

type serverStats struct {
	ID      string            `json:"id"`
	Name    string            `json:"name"`
	Game    string            `json:"game"`
	Status  supervisor.Status `json:"status"`
	Points  []metrics.Point   `json:"points"`
	Summary metrics.Summary   `json:"summary"`
}

func (s *Server) stats(w http.ResponseWriter, r *http.Request, u ctxUser) error {
	from, key := rangeFrom(r)
	var list []store.Server
	list = s.App.Store.Servers()
	servers := make([]serverStats, 0, len(list))
	for _, sv := range list {
		if !u.CanServer(sv.ID) {
			continue
		}
		servers = append(servers, serverStats{ID: sv.ID, Name: sv.Name, Game: sv.Game, Status: s.App.Proc(sv.ID).Status(), Points: s.App.Metrics.Range(sv.ID, from, chartPoints), Summary: s.App.Metrics.Summary(sv.ID, from)})
	}
	writeJSON(w, 200, map[string]any{
		"range":   key,
		"from":    from,
		"host":    map[string]any{"info": s.App.Metrics.HostInfo(), "points": s.App.Metrics.Range("host", from, chartPoints), "summary": s.App.Metrics.Summary("host", from)},
		"servers": servers,
	})
	return nil
}

func (s *Server) serverStats(w http.ResponseWriter, r *http.Request, _ ctxUser) error {
	sv, err := s.server(r)
	if err != nil {
		return err
	}
	from, key := rangeFrom(r)
	events := s.App.Metrics.EventsSince(sv.ID, from)
	if len(events) > 100 {
		events = events[:100]
	}
	writeJSON(w, 200, map[string]any{
		"range":   key,
		"from":    from,
		"points":  s.App.Metrics.Range(sv.ID, from, chartPoints),
		"summary": s.App.Metrics.Summary(sv.ID, from),
		"events":  events,
		"host":    s.App.Metrics.HostInfo(),
	})
	return nil
}

// ---------- In-game notices and resource performance ----------

func (s *Server) announce(w http.ResponseWriter, r *http.Request, u ctxUser) error {
	var in struct {
		Title, Text string
		ShowName    bool
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	sv, err := s.server(r)
	if err != nil {
		return err
	}
	author := ""
	if in.ShowName {
		author = u.Username
	}
	connected, err := s.App.Announce(sv.ID, in.Title, in.Text, author)
	if err != nil {
		return badRequest(err)
	}
	s.App.Store.Audit(u.Username, "server.announce", sv.Name, map[string]string{"title": in.Title, "text": in.Text})
	writeJSON(w, 200, map[string]bool{"connected": connected})
	return nil
}

func (s *Server) resmon(w http.ResponseWriter, r *http.Request, _ ctxUser) error {
	sv, err := s.server(r)
	if err != nil {
		return err
	}
	window := time.Hour
	if r.URL.Query().Get("range") == "6h" {
		window = 6 * time.Hour
	}
	writeJSON(w, 200, s.App.ResourcePerformance(sv.ID, time.Now().Add(-window), 6))
	return nil
}

// BridgeHandler serves the latchadmin bridge resource. main runs it on a loopback-only listener, apart from the
// panel itself, so it's out of reach of anything but this machine. The bridge proves which server it is with the
// key LatchAdmin gave that server.
func (s *Server) BridgeHandler() http.Handler {
	m := http.NewServeMux()
	m.HandleFunc("GET /bridge/poll", func(w http.ResponseWriter, r *http.Request) {
		key, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok || len(key) < 32 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		notices, err := s.App.BridgePoll(r.Context(), key)
		if err != nil {
			if r.Context().Err() == nil {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
			}
			return
		}
		if notices == nil {
			notices = []bridge.Notice{}
		}
		writeJSON(w, 200, map[string]any{"notices": notices})
	})
	return m
}

// ---------- Foundations and database ----------

// sentence ends s with a full stop unless it already ends with punctuation.
func sentence(s string) string {
	if strings.HasSuffix(s, ".") || strings.HasSuffix(s, "?") || strings.HasSuffix(s, "!") {
		return s
	}
	return s + "."
}

func (s *Server) foundationsStatus(w http.ResponseWriter, r *http.Request, _ ctxUser) error {
	st, err := s.App.FoundationsStatus(r.Context(), r.PathValue("id"))
	if err != nil {
		return err
	}
	writeJSON(w, 200, st)
	return nil
}

func (s *Server) installFoundations(w http.ResponseWriter, r *http.Request, u ctxUser) error {
	j, err := s.App.InstallFoundations(r.PathValue("id"), u.Username)
	if err != nil {
		return badRequest(err)
	}
	writeJSON(w, 202, j.Snapshot())
	return nil
}

func (s *Server) setDatabase(w http.ResponseWriter, r *http.Request, u ctxUser) error {
	var in app.DatabaseSetup
	if err := decode(r, &in); err != nil {
		return err
	}
	c, err := s.App.PrepareDatabase(r.Context(), in)
	if err != nil {
		return fail(400, "database", sentence("Database: "+err.Error()))
	}
	if err := s.App.SetDatabase(r.PathValue("id"), c, u.Username); err != nil {
		return err
	}
	st, err := s.App.FoundationsStatus(r.Context(), r.PathValue("id"))
	if err != nil {
		return err
	}
	writeJSON(w, 200, st)
	return nil
}

// ---------- Discord notifications ----------

func webhookView(w store.Webhook) map[string]any {
	servers := w.Servers
	if servers == nil {
		servers = []string{}
	}
	return map[string]any{"id": w.ID, "name": w.Name, "url": notify.MaskURL(secret.MustOpen(w.URL)), "lang": w.Lang, "events": w.Events,
		"servers": servers, "enabled": w.Enabled, "lastSentAt": w.LastSentAt, "lastError": w.LastError}
}

// inScope reports whether a user may see and change a webhook or status board covering these servers. One that
// reaches a server they can't access doesn't exist for them, like the server itself. allIfEmpty: an empty list
// means every server (webhooks), which needs access to all servers.
func inScope(u ctxUser, servers []string, allIfEmpty bool) bool {
	if u.AllServers || perms.Has(u.Permissions, perms.All) {
		return true
	}
	if len(servers) == 0 {
		return !allIfEmpty
	}
	for _, id := range servers {
		if !u.CanServer(id) {
			return false
		}
	}
	return true
}

func (s *Server) listWebhooks(w http.ResponseWriter, r *http.Request, u ctxUser) error {
	out := []map[string]any{}
	s.App.Store.Read(func(st *store.State) {
		for _, h := range st.Webhooks {
			if inScope(u, h.Servers, true) {
				out = append(out, webhookView(h))
			}
		}
	})
	writeJSON(w, 200, map[string]any{"webhooks": out, "groups": notify.Groups})
	return nil
}

// saveWebhook creates (POST) or changes (PATCH) a webhook. On change, an empty URL keeps the stored one.
func (s *Server) saveWebhook(w http.ResponseWriter, r *http.Request, u ctxUser) error {
	var in struct {
		Name    string
		URL     string
		Lang    string
		Events  []string
		Servers []string
		Enabled bool
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	id := r.PathValue("wid")
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len(in.Name) > 60 {
		return fail(400, "badName", "Name must be 1 to 60 characters.")
	}
	if in.Lang != "tr" {
		in.Lang = "en"
	}
	var events []string
	for _, e := range in.Events {
		if notify.ValidEvent(e) && !slices.Contains(events, e) {
			events = append(events, e)
		}
	}
	if len(events) == 0 {
		return fail(400, "noEvents", "Choose at least one event.")
	}
	// Same rule as granting server access: "all servers" needs all-server access, and only servers you can see.
	servers, err := s.grantableServers(u, len(in.Servers) == 0, in.Servers)
	if err != nil {
		return err
	}
	var sealed string
	if in.URL != "" || id == "" {
		if !notify.ValidURL(in.URL) {
			return fail(400, "badUrl", "Paste a Discord webhook URL (Server settings > Integrations > Webhooks > Copy webhook URL).")
		}
		var err error
		if sealed, err = secret.Seal(strings.TrimSpace(in.URL)); err != nil {
			return err
		}
	}
	var saved store.Webhook
	err = s.App.Store.Update(func(st *store.State) error {
		if id == "" {
			if len(st.Webhooks) >= 20 {
				return fail(400, "tooMany", "At most 20 webhooks.")
			}
			saved = store.Webhook{ID: store.NewID(), Name: in.Name, URL: sealed, Lang: in.Lang, Events: events, Servers: servers, Enabled: in.Enabled}
			st.Webhooks = append(st.Webhooks, saved)
			return nil
		}
		for i := range st.Webhooks {
			if st.Webhooks[i].ID == id && inScope(u, st.Webhooks[i].Servers, true) {
				h := &st.Webhooks[i]
				h.Name, h.Lang, h.Events, h.Servers, h.Enabled = in.Name, in.Lang, events, servers, in.Enabled
				if sealed != "" {
					h.URL, h.LastError = sealed, ""
				}
				saved = *h
				return nil
			}
		}
		return store.ErrNotFound
	})
	if err != nil {
		return err
	}
	s.App.Store.Audit(u.Username, "webhook.save", saved.Name, map[string]string{"events": strings.Join(events, ",")})
	writeJSON(w, 200, webhookView(saved))
	return nil
}

func (s *Server) deleteWebhook(w http.ResponseWriter, r *http.Request, u ctxUser) error {
	id := r.PathValue("wid")
	var name string
	if err := s.App.Store.Update(func(st *store.State) error {
		for i, h := range st.Webhooks {
			if h.ID == id && inScope(u, h.Servers, true) {
				name = h.Name
				st.Webhooks = append(st.Webhooks[:i], st.Webhooks[i+1:]...)
				return nil
			}
		}
		return store.ErrNotFound
	}); err != nil {
		return err
	}
	s.App.Store.Audit(u.Username, "webhook.delete", name, nil)
	writeJSON(w, 200, map[string]bool{"ok": true})
	return nil
}

func (s *Server) testWebhook(w http.ResponseWriter, r *http.Request, u ctxUser) error {
	id := r.PathValue("wid")
	var hook store.Webhook
	var panel string
	found := false
	s.App.Store.Read(func(st *store.State) {
		panel = st.Settings.PanelName
		for _, h := range st.Webhooks {
			if h.ID == id && inScope(u, h.Servers, true) {
				hook, found = h, true
			}
		}
	})
	if !found {
		return errNotFound
	}
	scope := "@allServers"
	if len(hook.Servers) > 0 {
		scope = fmt.Sprint(len(hook.Servers))
	}
	test := notify.Event{Kind: "test", At: time.Now(), Fields: []notify.Field{{Label: "f.events", Value: fmt.Sprint(len(hook.Events))}, {Label: "f.servers", Value: scope}}}
	if err := s.App.SendWebhook(r.Context(), hook, test, panel); err != nil {
		return badRequest(err)
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
	return nil
}
