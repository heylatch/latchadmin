package app

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/heylatch/latchadmin/internal/bridge"
	"github.com/heylatch/latchadmin/internal/secret"
	"github.com/heylatch/latchadmin/internal/store"
	"github.com/heylatch/latchadmin/internal/supervisor"
)

// In-game notices: announcements and restart warnings shown on players' screens through the latchadmin bridge
// resource. The bridge asks for them over a loopback-only address (BridgeURL) with its server's own key; nothing
// outside this machine can reach that address.

// DefaultRestartWarnings are the minutes before a scheduled restart when players are warned.
var DefaultRestartWarnings = []int{10, 5, 1}

type queuedNotice struct {
	n  bridge.Notice
	at time.Time
}

type noticeHub struct {
	mu    sync.Mutex
	queue map[string][]queuedNotice
	wake  map[string]chan struct{}
	seen  map[string]time.Time
}

func (h *noticeHub) wakeChan(id string) chan struct{} {
	if h.wake == nil {
		h.wake = map[string]chan struct{}{}
	}
	c, ok := h.wake[id]
	if !ok {
		c = make(chan struct{}, 1)
		h.wake[id] = c
	}
	return c
}

func (h *noticeHub) push(id string, n bridge.Notice) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.queue == nil {
		h.queue = map[string][]queuedNotice{}
	}
	q := append(h.queue[id], queuedNotice{n: n, at: time.Now()})
	if len(q) > 20 {
		q = q[len(q)-20:]
	}
	h.queue[id] = q
	select {
	case h.wakeChan(id) <- struct{}{}:
	default:
	}
}

// take returns the queued notices that are still fresh (a notice older than two minutes is stale).
func (h *noticeHub) take(id string) []bridge.Notice {
	h.mu.Lock()
	defer h.mu.Unlock()
	var out []bridge.Notice
	for _, q := range h.queue[id] {
		if time.Since(q.at) < 2*time.Minute {
			n := q.n
			if n.Kind == "restart" && n.Seconds > 0 {
				// Waited in the queue: count that time off.
				n.Seconds -= int(time.Since(q.at).Seconds())
				if n.Seconds < 1 {
					continue
				}
			}
			out = append(out, n)
		}
	}
	delete(h.queue, id)
	return out
}

// NoticesConnected reports whether the server's bridge asked for notices recently (it keeps a request open).
func (a *App) NoticesConnected(id string) bool {
	a.notices.mu.Lock()
	defer a.notices.mu.Unlock()
	return time.Since(a.notices.seen[id]) < 45*time.Second
}

// BridgePoll answers the bridge resource: it waits up to 25 seconds for notices for the server whose key this is.
func (a *App) BridgePoll(ctx context.Context, key string) ([]bridge.Notice, error) {
	id := ""
	for _, sv := range a.Store.Servers() {
		if sv.BridgeKey == "" || sv.NoticesOff {
			continue
		}
		if want := secret.MustOpen(sv.BridgeKey); want != "" && subtle.ConstantTimeCompare([]byte(want), []byte(key)) == 1 {
			id = sv.ID
		}
	}
	if id == "" {
		return nil, errors.New("unknown key")
	}
	mark := func() {
		a.notices.mu.Lock()
		if a.notices.seen == nil {
			a.notices.seen = map[string]time.Time{}
		}
		a.notices.seen[id] = time.Now()
		a.notices.mu.Unlock()
	}
	mark()
	defer mark()
	if n := a.notices.take(id); len(n) > 0 {
		return n, nil
	}
	a.notices.mu.Lock()
	wake := a.notices.wakeChan(id)
	a.notices.mu.Unlock()
	timer := time.NewTimer(25 * time.Second)
	defer timer.Stop()
	select {
	case <-wake:
	case <-timer.C:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return a.notices.take(id), nil
}

// prepareBridge runs before a server starts: it installs or removes the bridge resource and returns the variables
// that tell the bridge where to ask and with which key.
func (a *App) prepareBridge(sv store.Server) []string {
	p := a.Proc(sv.ID)
	if sv.NoticesOff || a.BridgeURL == "" {
		if err := bridge.Remove(sv.Dir); err != nil && !errors.Is(err, bridge.ErrNotOurs) {
			p.Panelf("Couldn't remove the latchadmin resource: %v", err)
		}
		return nil
	}
	key := secret.MustOpen(sv.BridgeKey)
	if key == "" {
		b := make([]byte, 32)
		_, _ = rand.Read(b)
		key = hex.EncodeToString(b)
		sealed, err := secret.Seal(key)
		if err != nil {
			return nil
		}
		_ = a.Store.UpdateServer(sv.ID, func(x *store.Server) error { x.BridgeKey = sealed; return nil })
	}
	if changed, err := bridge.Install(sv.Dir); err != nil {
		p.Panelf("In-game notices are off: %v", err)
		return nil
	} else if changed {
		p.Panelf("Installed the latchadmin resource %s (in-game announcements and restart warnings)", bridge.Version())
	}
	return []string{"LATCHADMIN_BRIDGE_URL=" + a.BridgeURL, "LATCHADMIN_BRIDGE_TOKEN=" + key}
}

// Announce shows a message on every player's screen. It reports whether the server's bridge is connected (if it
// isn't, the message waits two minutes for it, then is dropped).
func (a *App) Announce(id, title, text, author string) (bool, error) {
	sv, ok := a.Store.Server(id)
	if !ok {
		return false, store.ErrNotFound
	}
	if sv.NoticesOff {
		return false, errors.New("in-game notices are turned off for this server (Settings)")
	}
	if a.Proc(id).Status().State != supervisor.Running {
		return false, errors.New("the server isn't running")
	}
	title, text = clean(title, 80), clean(text, 400)
	if title == "" && text == "" {
		return false, errors.New("write a message")
	}
	n := bridge.Announcement(sv.NoticeLang, title, text, author)
	n.ID = newNoticeID()
	a.notices.push(id, n)
	a.Proc(id).Panelf("Announcement: %s", strings.TrimSpace(title+" "+text))
	return a.NoticesConnected(id), nil
}

func clean(s string, max int) string {
	s = strings.TrimSpace(strings.Map(func(r rune) rune {
		if r == '\n' {
			return r
		}
		if r < 32 || r == 127 {
			return -1
		}
		return r
	}, s))
	if utf8.RuneCountInString(s) > max {
		s = string([]rune(s)[:max])
	}
	return s
}

func newNoticeID() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// warned remembers sent restart warnings ("<server>|<restart time>|<minutes>"), so each goes out once.
var warned = map[string]time.Time{}

// warnRestarts sends restart warnings for scheduled restarts coming up. Called by the scheduler every 30 seconds.
func (a *App) warnRestarts(now time.Time) {
	for k, at := range warned {
		if now.Sub(at) > 2*time.Hour {
			delete(warned, k)
		}
	}
	for _, sv := range a.Store.Servers() {
		if sv.NoticesOff || len(sv.Restarts) == 0 || a.Proc(sv.ID).Status().State != supervisor.Running {
			continue
		}
		mins := sv.RestartWarnings
		if mins == nil {
			mins = DefaultRestartWarnings
		}
		for _, hm := range sv.Restarts {
			at, err := time.ParseInLocation("15:04", hm, now.Location())
			if err != nil {
				continue
			}
			next := time.Date(now.Year(), now.Month(), now.Day(), at.Hour(), at.Minute(), 0, 0, now.Location())
			if !next.After(now) {
				next = next.Add(24 * time.Hour)
			}
			left := next.Sub(now)
			for _, m := range mins {
				w := time.Duration(m) * time.Minute
				key := fmt.Sprintf("%s|%d|%d", sv.ID, next.Unix(), m)
				// The scheduler ticks every 30 seconds, so a warning is due within 45 seconds of its time.
				if left <= w && left > w-45*time.Second && warned[key].IsZero() {
					warned[key] = now
					n := bridge.RestartWarning(sv.NoticeLang, int(left.Seconds()))
					n.ID = newNoticeID()
					a.notices.push(sv.ID, n)
					a.Proc(sv.ID).Panelf("Restart warning sent: %d minute(s) to the scheduled restart", m)
				}
			}
		}
	}
}
