package app

import (
	"context"
	"errors"
	"time"

	"github.com/heylatch/latchadmin/internal/notify"
	"github.com/heylatch/latchadmin/internal/secret"
	"github.com/heylatch/latchadmin/internal/store"
	"github.com/heylatch/latchadmin/internal/supervisor"
)

const (
	boardRefresh = 5 * time.Minute  // edit at least this often, so "Updated" shows the panel is alive
	boardRetry   = 10 * time.Minute // after a failure, wait this long before trying again
)

// boardState is how a server's state reads on a public board.
func boardState(st supervisor.Status) string {
	switch st.State {
	case supervisor.Running:
		return "online"
	case supervisor.Starting:
		return "starting"
	case supervisor.Stopping:
		return "stopping"
	case supervisor.Updating:
		return "updating"
	case supervisor.Crashed:
		if !st.NextStart.IsZero() {
			return "restarting"
		}
	}
	return "offline"
}

// boardData collects what a board shows right now. Servers removed from the panel are left out.
func (a *App) boardData(b store.StatusBoard) notify.Board {
	out := notify.Board{Title: b.Title, Note: b.Note, At: time.Now()}
	for _, x := range b.Buttons {
		out.Buttons = append(out.Buttons, notify.Button{Label: x.Label, URL: x.URL})
	}
	for _, e := range b.Servers {
		sv, ok := a.Store.Server(e.ServerID)
		if !ok {
			continue
		}
		st := a.Proc(sv.ID).Status()
		since := st.ReadyAt
		if since.IsZero() {
			since = st.StartedAt
		}
		out.Servers = append(out.Servers, notify.BoardEntry{Name: sv.Name, Game: gameLine(sv), State: boardState(st), Players: st.Players,
			MaxPlayers: st.MaxPlayers, Address: e.Address, Since: since})
	}
	return out
}

// updateBoards edits every enabled board whose content changed (or that wasn't edited for a while).
func (a *App) updateBoards(ctx context.Context) {
	a.boardMu.Lock()
	defer a.boardMu.Unlock()
	var boards []store.StatusBoard
	a.Store.Read(func(st *store.State) { boards = append(boards, st.StatusBoards...) })
	for _, b := range boards {
		if !b.Enabled {
			continue
		}
		if b.LastError != "" && time.Since(b.LastTryAt) < boardRetry {
			continue
		}
		_ = a.sendBoard(ctx, b, false, false)
	}
}

// RefreshBoard edits a board now. With repost, the old message is deleted and a new one is sent, so the board
// moves to the bottom of the channel.
func (a *App) RefreshBoard(ctx context.Context, id string, repost bool) error {
	a.boardMu.Lock()
	defer a.boardMu.Unlock()
	var b store.StatusBoard
	found := false
	a.Store.Read(func(st *store.State) {
		for _, x := range st.StatusBoards {
			if x.ID == id {
				b, found = x, true
			}
		}
	})
	if !found {
		return store.ErrNotFound
	}
	return a.sendBoard(ctx, b, true, repost)
}

// RemoveBoardMessage deletes a board's message from Discord (best effort, used when the board is deleted).
func (a *App) RemoveBoardMessage(b store.StatusBoard) {
	if b.MessageID == "" {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		a.boardMu.Lock()
		defer a.boardMu.Unlock()
		_ = notify.DeleteMessage(ctx, secret.MustOpen(b.URL), b.MessageID)
	}()
}

// sendBoard edits the board's message, or sends a new one when there's none yet or it was deleted in Discord.
// The caller holds boardMu, so two updates never post two messages.
func (a *App) sendBoard(ctx context.Context, b store.StatusBoard, force, repost bool) error {
	data := a.boardData(b)
	hash := notify.BoardHash(data, b.Lang)
	if !force && b.MessageID != "" && b.LastError == "" && hash == b.LastHash && time.Since(b.LastSentAt) < boardRefresh {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	hook := secret.MustOpen(b.URL)
	msg := notify.StatusMessage(data, b.Lang)
	id := b.MessageID
	var err error
	if repost && id != "" {
		if err = notify.DeleteMessage(ctx, hook, id); err == nil {
			id = ""
		}
	}
	if err == nil && id != "" {
		if err = notify.EditMessage(ctx, hook, id, msg); errors.Is(err, notify.ErrUnknownMessage) {
			id, err = "", nil
		}
	}
	if err == nil && id == "" {
		id, err = notify.PostMessage(ctx, hook, msg)
	}
	_ = a.Store.Update(func(st *store.State) error {
		for i := range st.StatusBoards {
			x := &st.StatusBoards[i]
			if x.ID != b.ID {
				continue
			}
			x.LastTryAt = time.Now().UTC()
			if err != nil {
				x.LastError = notify.Redact(err.Error())
				return nil
			}
			// The board may have been edited meanwhile; keep its settings and only record the message.
			x.MessageID, x.LastHash, x.LastSentAt, x.LastError = id, hash, x.LastTryAt, ""
		}
		return nil
	})
	return err
}

// boardsOffline is called on shutdown, after the servers stopped, so public boards don't keep saying "online".
func (a *App) boardsOffline() {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	a.boardMu.Lock()
	defer a.boardMu.Unlock()
	var boards []store.StatusBoard
	a.Store.Read(func(st *store.State) { boards = append(boards, st.StatusBoards...) })
	for _, b := range boards {
		if b.Enabled && b.MessageID != "" && ctx.Err() == nil {
			_ = a.sendBoard(ctx, b, true, false)
		}
	}
}
