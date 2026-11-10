package app

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/heylatch/latchadmin/internal/buildinfo"
	"github.com/heylatch/latchadmin/internal/games"
	"github.com/heylatch/latchadmin/internal/notify"
	"github.com/heylatch/latchadmin/internal/secret"
	"github.com/heylatch/latchadmin/internal/store"
)

// Notify queues an event for the Discord webhooks that asked for it. It never blocks: if the queue is full
// (Discord down for a long time), the event is dropped.
func (a *App) Notify(e notify.Event) {
	if e.At.IsZero() {
		e.At = time.Now()
	}
	if a.notes == nil {
		return
	}
	select {
	case a.notes <- e:
	default:
	}
}

// deliverLoop sends queued events one by one, so Discord sees them in order and its rate limit is respected.
func (a *App) deliverLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case e := <-a.notes:
			a.deliver(ctx, e)
		}
	}
}

func (a *App) deliver(ctx context.Context, e notify.Event) {
	var hooks []store.Webhook
	var panel string
	a.Store.Read(func(st *store.State) {
		panel = st.Settings.PanelName
		for _, w := range st.Webhooks {
			if !w.Enabled || !slices.Contains(w.Events, e.Kind) {
				continue
			}
			if e.ServerID != "" && len(w.Servers) > 0 && !slices.Contains(w.Servers, e.ServerID) {
				continue
			}
			hooks = append(hooks, w)
		}
	})
	for _, w := range hooks {
		a.SendWebhook(ctx, w, e, panel)
		time.Sleep(500 * time.Millisecond) // stay well under Discord's per-webhook limit
	}
}

// SendWebhook posts one event to one webhook and records the outcome.
func (a *App) SendWebhook(ctx context.Context, w store.Webhook, e notify.Event, panel string) error {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	var public string
	a.Store.Read(func(st *store.State) { public = st.Settings.PublicURL })
	err := notify.Send(ctx, secret.MustOpen(w.URL), notify.Message(e, w.Lang, panel, public))
	_ = a.Store.Update(func(st *store.State) error {
		for i := range st.Webhooks {
			if st.Webhooks[i].ID == w.ID {
				if err != nil {
					st.Webhooks[i].LastError = notify.Redact(err.Error())
				} else {
					st.Webhooks[i].LastError, st.Webhooks[i].LastSentAt = "", time.Now().UTC()
				}
			}
		}
		return nil
	})
	return err
}

// once reports whether key hasn't been notified yet, and marks it.
func (a *App) once(key string) bool {
	first := false
	_ = a.Store.Update(func(st *store.State) error {
		if st.Notified == nil {
			st.Notified = map[string]bool{}
		}
		if !st.Notified[key] {
			st.Notified[key], first = true, true
		}
		return nil
	})
	return first
}

// notifyVersions tells about new game and panel releases, once per version.
func (a *App) notifyVersions() {
	for _, sv := range a.Store.Servers() {
		if sv.Version == "" {
			continue
		}
		info := a.Product(sv.Game, sv.Version)
		if info.Latest == "" || compare(info.Latest, sv.Version) <= 0 || !a.once(fmt.Sprintf("game.available:%s:%s", sv.ID, info.Latest)) {
			continue
		}
		g, _ := games.ByID(sv.Game)
		urgent := ""
		if info.Urgent != nil {
			urgent = "@urgent"
		}
		fields := []notify.Field{{Label: "f.installed", Value: sv.Version}, {Label: "f.latest", Value: info.Latest}}
		if info.Urgent != nil && info.Urgent.Note != nil {
			fields = append(fields, notify.Field{Label: "f.note", Value: *info.Urgent.Note})
		}
		a.Notify(notify.Event{Kind: "game.available", ServerID: sv.ID, Server: sv.Name, Link: "/servers/" + sv.ID + "/updates", Vars: map[string]string{"game": g.Label, "version": info.Latest, "urgent": urgent}, Fields: fields})
	}
	if buildinfo.IsDev() {
		return
	}
	info := a.Product("latchadmin", buildinfo.Version)
	if info.Latest != "" && compare(info.Latest, buildinfo.Version) > 0 && a.once("panel.available:"+info.Latest) {
		urgent := ""
		if info.Urgent != nil {
			urgent = "@urgent"
		}
		fields := []notify.Field{{Label: "f.installed", Value: buildinfo.Version}, {Label: "f.latest", Value: info.Latest}}
		if info.Urgent != nil && info.Urgent.Note != nil {
			fields = append(fields, notify.Field{Label: "f.note", Value: *info.Urgent.Note})
		}
		a.Notify(notify.Event{Kind: "panel.available", Link: "/settings", Vars: map[string]string{"version": info.Latest, "urgent": urgent}, Fields: fields})
	}
}

// recordPlayers sends "new player record" when a server beats its best (from 5 players up, so a test server
// doesn't post every time someone joins).
func (a *App) recordPlayers(sv store.Server, players int) {
	if players < 5 {
		return
	}
	beat, previous := false, 0
	_ = a.Store.Update(func(st *store.State) error {
		if st.PlayerRecords == nil {
			st.PlayerRecords = map[string]int{}
		}
		if previous = st.PlayerRecords[sv.ID]; players > previous {
			st.PlayerRecords[sv.ID], beat = players, true
		}
		return nil
	})
	if beat {
		fields := []notify.Field{{Label: "f.record", Value: fmt.Sprint(players)}}
		if previous > 0 {
			fields = append(fields, notify.Field{Label: "f.previousRecord", Value: fmt.Sprint(previous)})
		}
		if max := a.Proc(sv.ID).Status().MaxPlayers; max > 0 {
			fields = append(fields, notify.Field{Label: "f.capacity", Value: fmt.Sprint(max)})
		}
		a.Notify(notify.Event{Kind: "players.record", ServerID: sv.ID, Server: sv.Name, Link: "/servers/" + sv.ID + "/performance", Fields: fields})
	}
}

// duration writes "2h 13m", "4m 10s" or "12s".
func duration(d time.Duration) string {
	d = d.Round(time.Second)
	switch {
	case d >= 24*time.Hour:
		return fmt.Sprintf("%dd %dh", int(d.Hours())/24, int(d.Hours())%24)
	case d >= time.Hour:
		return fmt.Sprintf("%dh %dm", int(d.Hours()), int(d.Minutes())%60)
	case d >= time.Minute:
		return fmt.Sprintf("%dm %ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%ds", int(d.Seconds()))
}

// gameLine is "Arcane MP 1.12.2" for a server.
func gameLine(sv store.Server) string {
	g, _ := games.ByID(sv.Game)
	if sv.Version == "" {
		return g.Label
	}
	return g.Label + " " + sv.Version
}
