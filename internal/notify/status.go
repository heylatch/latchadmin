package notify

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// A status board is one public message that the panel edits in place: which servers are up, how many players are
// on, and the address players connect to. It carries only what the owner chose to show.

// BoardEntry is one server on a board.
type BoardEntry struct {
	Name       string
	Game       string // "Arcane MP 1.12.2"
	State      string // online, starting, restarting, stopping, updating, offline
	Players    *int
	MaxPlayers int
	Address    string
	Since      time.Time // online since
}

// Button is a link button under the board.
type Button struct{ Label, URL string }

// Board is everything a status message shows.
type Board struct {
	Title   string
	Note    string
	Servers []BoardEntry
	Buttons []Button
	At      time.Time
}

// MaxBoardServers keeps a board under Discord's 40-component limit with room for the header and buttons.
const MaxBoardServers = 10

const (
	colorOnline  = 0x22c55e
	colorBusy    = 0xf59e0b
	colorOffline = 0xef4444
)

var addressRe = regexp.MustCompile(`^[A-Za-z0-9.\-:\[\]]{1,100}$`)

// ValidAddress accepts "play.example.com:27015", "203.0.113.5:27015" or "[2001:db8::1]:27015".
func ValidAddress(a string) bool { return addressRe.MatchString(a) }

// ValidButtonURL accepts https links Discord will show as a link button.
func ValidButtonURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && u.Host != "" && len(raw) <= 512 && !strings.ContainsAny(raw, " \t\r\n")
}

var noteControl = regexp.MustCompile("[\u0000-\u0009\u000b-\u001f\u007f]+")

// CleanNote keeps the owner's own text (markdown allowed: it's their channel) but drops control characters and
// caps it at 1000 characters and 15 lines.
func CleanNote(s string) string {
	s = strings.TrimSpace(noteControl.ReplaceAllString(strings.ReplaceAll(s, "\r\n", "\n"), ""))
	if r := []rune(s); len(r) > 1000 {
		s = string(r[:1000])
	}
	if lines := strings.Split(s, "\n"); len(lines) > 15 {
		s = strings.Join(lines[:15], "\n")
	}
	return s
}

// StatusMessage builds the Components V2 payload for a board in a language ("en" or "tr").
func StatusMessage(b Board, lang string) map[string]any {
	tr := phrases(lang)
	online, players, known := 0, 0, false
	for _, s := range b.Servers {
		if s.State == "online" {
			online++
			if s.Players != nil {
				players += *s.Players
				known = true
			}
		}
	}
	summary := strings.NewReplacer("{online}", fmt.Sprint(online), "{total}", fmt.Sprint(len(b.Servers))).Replace(tr["board.summary"])
	if known {
		key := "board.players"
		if players == 1 {
			key = "board.player"
		}
		summary += " · " + strings.ReplaceAll(tr[key], "{n}", fmt.Sprint(players))
	}
	head := []map[string]any{{"type": 10, "content": "## " + esc(b.Title, 80) + "\n" + summary}}
	if b.Note != "" {
		head = append(head, map[string]any{"type": 10, "content": b.Note})
	}
	parts := []map[string]any{{"type": 17, "components": head}}

	for _, s := range b.Servers {
		color := colorOffline
		switch s.State {
		case "online":
			color = colorOnline
		case "starting", "restarting", "stopping", "updating":
			color = colorBusy
		}
		lines := []string{"### " + esc(s.Name, 60), "**" + tr["board.state."+s.State] + "** · " + esc(s.Game, 60)}
		if s.State == "online" && s.Players != nil {
			p := fmt.Sprint(*s.Players)
			if s.MaxPlayers > 0 {
				p += " / " + fmt.Sprint(s.MaxPlayers)
			}
			lines = append(lines, fmt.Sprintf("**%s:** %s", tr["board.f.players"], p))
		}
		if s.Address != "" {
			lines = append(lines, fmt.Sprintf("**%s:** `%s`", tr["board.f.address"], strings.ReplaceAll(s.Address, "`", "")))
		}
		if s.State == "online" && !s.Since.IsZero() {
			lines = append(lines, fmt.Sprintf("**%s:** <t:%d:R>", tr["board.f.since"], s.Since.Unix()))
		}
		parts = append(parts, map[string]any{"type": 17, "accent_color": color, "components": []map[string]any{{"type": 10, "content": strings.Join(lines, "\n")}}})
	}

	parts = append(parts, map[string]any{"type": 10, "content": fmt.Sprintf("-# %s <t:%d:R> · LatchAdmin", tr["board.updated"], b.At.Unix())})
	if len(b.Buttons) > 0 {
		var btns []map[string]any
		for _, x := range b.Buttons {
			btns = append(btns, map[string]any{"type": 2, "style": 5, "label": x.Label, "url": x.URL})
		}
		parts = append(parts, map[string]any{"type": 1, "components": btns})
	}
	return map[string]any{
		"username":         "LatchAdmin",
		"avatar_url":       avatarURL,
		"flags":            isComponentsV2,
		"allowed_mentions": map[string]any{"parse": []string{}},
		"components":       parts,
	}
}

// BoardHash identifies what a board shows, leaving out the "updated" time, so the panel edits only on a change.
func BoardHash(b Board, lang string) string {
	b.At = time.Time{}
	raw, _ := json.Marshal(StatusMessage(b, lang))
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

var messageIDRe = regexp.MustCompile(`^\d{5,30}$`)

// PostMessage sends a new message and returns its id.
func PostMessage(ctx context.Context, webhook string, msg map[string]any) (string, error) {
	if !ValidURL(webhook) {
		return "", errors.New("not a Discord webhook URL")
	}
	var out struct {
		ID string `json:"id"`
	}
	if err := call(ctx, http.MethodPost, strings.TrimSpace(webhook)+"?wait=true&with_components=true", msg, &out); err != nil {
		return "", err
	}
	if !messageIDRe.MatchString(out.ID) {
		return "", errors.New("Discord didn't return the message")
	}
	return out.ID, nil
}

// EditMessage replaces a message the webhook sent. It returns ErrUnknownMessage when the message was deleted.
func EditMessage(ctx context.Context, webhook, id string, msg map[string]any) error {
	if !ValidURL(webhook) || !messageIDRe.MatchString(id) {
		return errors.New("not a Discord webhook message")
	}
	edit := map[string]any{}
	for k, v := range msg {
		if k != "username" && k != "avatar_url" { // fixed when the message was sent
			edit[k] = v
		}
	}
	return call(ctx, http.MethodPatch, strings.TrimSpace(webhook)+"/messages/"+id+"?with_components=true", edit, nil)
}

// DeleteMessage removes a message the webhook sent. A message that's already gone is not an error.
func DeleteMessage(ctx context.Context, webhook, id string) error {
	if !ValidURL(webhook) || !messageIDRe.MatchString(id) {
		return errors.New("not a Discord webhook message")
	}
	err := call(ctx, http.MethodDelete, strings.TrimSpace(webhook)+"/messages/"+id, nil, nil)
	if errors.Is(err, ErrUnknownMessage) {
		return nil
	}
	return err
}
