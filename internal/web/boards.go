package web

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/heylatch/latchadmin/internal/notify"
	"github.com/heylatch/latchadmin/internal/secret"
	"github.com/heylatch/latchadmin/internal/store"
)

// ---------- Public status boards ----------

func boardView(b store.StatusBoard) map[string]any {
	servers, buttons := b.Servers, b.Buttons
	if servers == nil {
		servers = []store.BoardServer{}
	}
	if buttons == nil {
		buttons = []store.BoardButton{}
	}
	return map[string]any{"id": b.ID, "name": b.Name, "url": notify.MaskURL(secret.MustOpen(b.URL)), "lang": b.Lang, "title": b.Title, "note": b.Note,
		"servers": servers, "buttons": buttons, "enabled": b.Enabled, "posted": b.MessageID != "", "lastSentAt": b.LastSentAt, "lastError": b.LastError}
}

func (s *Server) listBoards(w http.ResponseWriter, r *http.Request, _ ctxUser) error {
	out := []map[string]any{}
	s.App.Store.Read(func(st *store.State) {
		for _, b := range st.StatusBoards {
			out = append(out, boardView(b))
		}
	})
	writeJSON(w, 200, map[string]any{"boards": out})
	return nil
}

// saveBoard creates (POST) or changes (PATCH) a board, then posts or edits its message right away. On change, an
// empty URL keeps the stored one; a new URL starts a new message in the new channel.
func (s *Server) saveBoard(w http.ResponseWriter, r *http.Request, u ctxUser) error {
	var in struct {
		Name, URL, Lang, Title, Note string
		Servers                      []store.BoardServer
		Buttons                      []store.BoardButton
		Enabled                      bool
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	id := r.PathValue("bid")
	in.Name, in.Title = strings.TrimSpace(in.Name), strings.TrimSpace(in.Title)
	if in.Name == "" || len(in.Name) > 60 {
		return fail(400, "badName", "Name must be 1 to 60 characters.")
	}
	if in.Title == "" || len([]rune(in.Title)) > 80 {
		return fail(400, "badTitle", "Title must be 1 to 80 characters.")
	}
	if in.Lang != "tr" {
		in.Lang = "en"
	}
	in.Note = notify.CleanNote(in.Note)

	var servers []store.BoardServer
	var ids []string
	for _, e := range in.Servers {
		e.Address = strings.TrimSpace(e.Address)
		if e.Address != "" && !notify.ValidAddress(e.Address) {
			return fail(400, "badAddress", "Write the address as host:port, for example play.example.com:27015.")
		}
		dup := false
		for _, x := range servers {
			dup = dup || x.ServerID == e.ServerID
		}
		if !dup {
			servers = append(servers, store.BoardServer{ServerID: e.ServerID, Address: e.Address})
			ids = append(ids, e.ServerID)
		}
	}
	// Only servers you can see, and only ones that exist.
	allowed, err := s.grantableServers(u, false, ids)
	if err != nil {
		return err
	}
	kept := servers[:0]
	for _, e := range servers {
		for _, id := range allowed {
			if id == e.ServerID {
				kept = append(kept, e)
			}
		}
	}
	servers = kept
	if len(servers) == 0 {
		return fail(400, "noServers", "Choose at least one server.")
	}
	if len(servers) > notify.MaxBoardServers {
		return fail(400, "tooManyServers", "A status message can show at most 10 servers.")
	}

	if len(in.Buttons) > 3 {
		return fail(400, "tooManyButtons", "At most 3 buttons.")
	}
	var buttons []store.BoardButton
	for _, b := range in.Buttons {
		b.Label, b.URL = strings.TrimSpace(b.Label), strings.TrimSpace(b.URL)
		if b.Label == "" && b.URL == "" {
			continue
		}
		if b.Label == "" || len([]rune(b.Label)) > 40 {
			return fail(400, "badButton", "Button labels must be 1 to 40 characters.")
		}
		if !notify.ValidButtonURL(b.URL) {
			return fail(400, "badButton", "Button links must start with https://.")
		}
		buttons = append(buttons, b)
	}

	var sealed string
	if in.URL != "" || id == "" {
		if !notify.ValidURL(in.URL) {
			return fail(400, "badUrl", "Paste a Discord webhook URL (Server settings > Integrations > Webhooks > Copy webhook URL).")
		}
		if sealed, err = secret.Seal(strings.TrimSpace(in.URL)); err != nil {
			return err
		}
	}
	var saved store.StatusBoard
	var old store.StatusBoard // the message left behind when the webhook changes
	err = s.App.Store.Update(func(st *store.State) error {
		if id == "" {
			if len(st.StatusBoards) >= 10 {
				return fail(400, "tooMany", "At most 10 status messages.")
			}
			saved = store.StatusBoard{ID: store.NewID(), Name: in.Name, URL: sealed, Lang: in.Lang, Title: in.Title, Note: in.Note,
				Servers: servers, Buttons: buttons, Enabled: in.Enabled}
			st.StatusBoards = append(st.StatusBoards, saved)
			return nil
		}
		for i := range st.StatusBoards {
			b := &st.StatusBoards[i]
			if b.ID != id {
				continue
			}
			b.Name, b.Lang, b.Title, b.Note, b.Servers, b.Buttons, b.Enabled = in.Name, in.Lang, in.Title, in.Note, servers, buttons, in.Enabled
			if sealed != "" && secret.MustOpen(sealed) != secret.MustOpen(b.URL) {
				old = *b
				b.URL, b.MessageID, b.LastHash, b.LastError = sealed, "", "", ""
			}
			saved = *b
			return nil
		}
		return store.ErrNotFound
	})
	if err != nil {
		return err
	}
	s.App.RemoveBoardMessage(old)
	s.App.Store.Audit(u.Username, "board.save", saved.Name, map[string]string{"servers": strings.Join(ids, ",")})
	if saved.Enabled {
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		_ = s.App.RefreshBoard(ctx, saved.ID, false) // an error shows on the card
	}
	return s.writeBoard(w, saved.ID)
}

func (s *Server) writeBoard(w http.ResponseWriter, id string) error {
	var out map[string]any
	s.App.Store.Read(func(st *store.State) {
		for _, b := range st.StatusBoards {
			if b.ID == id {
				out = boardView(b)
			}
		}
	})
	if out == nil {
		return errNotFound
	}
	writeJSON(w, 200, out)
	return nil
}

func (s *Server) deleteBoard(w http.ResponseWriter, r *http.Request, u ctxUser) error {
	id := r.PathValue("bid")
	var removed store.StatusBoard
	if err := s.App.Store.Update(func(st *store.State) error {
		for i, b := range st.StatusBoards {
			if b.ID == id {
				removed = b
				st.StatusBoards = append(st.StatusBoards[:i], st.StatusBoards[i+1:]...)
				return nil
			}
		}
		return store.ErrNotFound
	}); err != nil {
		return err
	}
	s.App.RemoveBoardMessage(removed)
	s.App.Store.Audit(u.Username, "board.delete", removed.Name, nil)
	writeJSON(w, 200, map[string]bool{"ok": true})
	return nil
}

// refreshBoard edits the message now, or with Repost sends it again at the bottom of the channel.
func (s *Server) refreshBoard(w http.ResponseWriter, r *http.Request, _ ctxUser) error {
	var in struct{ Repost bool }
	if r.ContentLength > 0 {
		if err := decode(r, &in); err != nil {
			return err
		}
	}
	id := r.PathValue("bid")
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	if err := s.App.RefreshBoard(ctx, id, in.Repost); err != nil {
		if err == store.ErrNotFound {
			return errNotFound
		}
		return badRequest(err)
	}
	return s.writeBoard(w, id)
}
