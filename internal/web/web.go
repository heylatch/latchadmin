// Package web serves the panel's API and its embedded web UI.
//
// Security: sessions are random tokens in an HttpOnly, SameSite=Strict cookie (only their SHA-256 is stored). Every
// state-changing request must carry the X-LatchAdmin header, which a browser only sends from the panel's own pages
// (there is no CORS), and its Origin, when present, must match the panel's host. Logins are rate limited per IP.
package web

import (
	"encoding/json"
	"errors"
	"io/fs"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/heylatch/latchadmin/internal/app"
	"github.com/heylatch/latchadmin/internal/auth"
	"github.com/heylatch/latchadmin/internal/perms"
	"github.com/heylatch/latchadmin/internal/store"
)

const cookieName = "latchadmin_session"
const sessionTTL = 7 * 24 * time.Hour

type Server struct {
	App    *app.App
	UI     fs.FS // built web UI; nil serves a placeholder
	mux    *http.ServeMux
	logins *auth.Limiter
	setups *auth.Limiter
	OnPIN  func(pin string) // prints a rotated setup PIN to the console
}

func New(a *app.App, ui fs.FS) *Server {
	s := &Server{App: a, UI: ui, mux: http.NewServeMux(), logins: auth.NewLimiter(10, 15*time.Minute), setups: auth.NewLimiter(20, 15*time.Minute)}
	s.routes()
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h := w.Header()
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("X-Frame-Options", "DENY")
	h.Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; script-src 'self'; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
	if r.TLS != nil {
		h.Set("Strict-Transport-Security", "max-age=31536000")
	}
	s.mux.ServeHTTP(w, r)
}

// ---------- helpers ----------

type apiError struct {
	Status  int
	Code    string
	Message string
}

func (e *apiError) Error() string { return e.Message }

func fail(status int, code, msg string) error { return &apiError{status, code, msg} }

var (
	errUnauthorized = fail(401, "unauthorized", "Sign in again.")
	errForbidden    = fail(403, "forbidden", "You don't have permission to do this.")
	errNotFound     = fail(404, "notFound", "Not found.")
)

func badRequest(err error) error { return fail(400, "invalid", err.Error()) }

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, err error) {
	var ae *apiError
	if errors.As(err, &ae) {
		writeJSON(w, ae.Status, map[string]any{"error": map[string]string{"code": ae.Code, "message": ae.Message}})
		return
	}
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, errNotFound)
		return
	}
	writeJSON(w, 400, map[string]any{"error": map[string]string{"code": "failed", "message": err.Error()}})
}

func decode(r *http.Request, v any) error {
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 4<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return fail(400, "invalid", "Invalid request: "+err.Error())
	}
	return nil
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// ---------- sessions ----------

type ctxUser struct {
	store.User
	token string
}

func (s *Server) user(r *http.Request) (ctxUser, bool) {
	c, err := r.Cookie(cookieName)
	if err != nil || c.Value == "" {
		return ctxUser{}, false
	}
	hash := auth.HashToken(c.Value)
	var out ctxUser
	ok := false
	s.App.Store.Read(func(st *store.State) {
		for _, se := range st.Sessions {
			if se.TokenHash == hash && time.Now().Before(se.ExpiresAt) {
				for _, u := range st.Users {
					if u.ID == se.UserID {
						out, ok = ctxUser{User: u, token: c.Value}, true
					}
				}
			}
		}
	})
	return out, ok
}

func (s *Server) newSession(w http.ResponseWriter, r *http.Request, userID string) error {
	token := auth.NewToken()
	now := time.Now().UTC()
	err := s.App.Store.Update(func(st *store.State) error {
		kept := st.Sessions[:0]
		for _, se := range st.Sessions {
			if now.Before(se.ExpiresAt) {
				kept = append(kept, se)
			}
		}
		st.Sessions = append(kept, store.Session{TokenHash: auth.HashToken(token), UserID: userID, ExpiresAt: now.Add(sessionTTL), CreatedAt: now, IP: clientIP(r)})
		return nil
	})
	if err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: token, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: r.TLS != nil, MaxAge: int(sessionTTL.Seconds())})
	return nil
}

func (s *Server) endSessions(userID, exceptHash string) {
	_ = s.App.Store.Update(func(st *store.State) error {
		kept := st.Sessions[:0]
		for _, se := range st.Sessions {
			if se.UserID != userID || se.TokenHash == exceptHash {
				kept = append(kept, se)
			}
		}
		st.Sessions = kept
		return nil
	})
}

// ---------- middleware ----------

type handler func(w http.ResponseWriter, r *http.Request, u ctxUser) error

// sameOrigin is the CSRF check for state-changing requests.
func sameOrigin(r *http.Request) bool {
	if r.Header.Get("X-LatchAdmin") != "1" {
		return false
	}
	if o := r.Header.Get("Origin"); o != "" {
		o = strings.TrimPrefix(strings.TrimPrefix(o, "https://"), "http://")
		return strings.EqualFold(o, r.Host)
	}
	return true
}

// Access levels for api(): public needs no session, signedIn any session, anything else is a permission.
const (
	public   = "-"
	signedIn = ""
)

// api wraps a handler with the CSRF check and, unless public, a session holding perm.
func (s *Server) api(perm string, h handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead && !sameOrigin(r) {
			writeErr(w, fail(403, "csrf", "Request blocked: it didn't come from the panel."))
			return
		}
		var u ctxUser
		if perm != public {
			var ok bool
			if u, ok = s.user(r); !ok {
				writeErr(w, errUnauthorized)
				return
			}
			if perm != signedIn && !perms.Has(u.Permissions, perm) {
				writeErr(w, errForbidden)
				return
			}
			// Per-server access: a server the user can't access answers as if it didn't exist.
			if id := r.PathValue("id"); id != "" && strings.HasPrefix(r.URL.Path, "/api/servers/") && !u.CanServer(id) {
				writeErr(w, errNotFound)
				return
			}
		}
		if err := h(w, r, u); err != nil {
			writeErr(w, err)
		}
	}
}
