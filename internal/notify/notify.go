// Package notify sends panel events to Discord webhooks, as Components V2 messages (the same layout Latch uses).
//
// Only Discord webhook URLs are accepted (discord.com / discordapp.com /api/webhooks/...), so this can't be used to
// send data anywhere else. Messages carry the event, the server's name and figures like versions or player counts,
// never config, keys or console output.
package notify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/heylatch/latchadmin/internal/netx"
)

// Events a webhook can subscribe to, grouped for the settings page.
var Groups = []struct {
	ID     string   `json:"id"`
	Events []string `json:"events"`
}{
	{ID: "servers", Events: []string{"server.crash", "server.ready", "server.stop", "server.restart"}},
	{ID: "updates", Events: []string{"game.available", "game.updated", "game.failed", "panel.available", "script.updated"}},
	{ID: "players", Events: []string{"players.record"}},
	{ID: "latch", Events: []string{"latch.lost"}},
}

// ValidEvent reports whether e is a known event.
func ValidEvent(e string) bool {
	for _, g := range Groups {
		for _, x := range g.Events {
			if x == e {
				return true
			}
		}
	}
	return false
}

var webhookRe = regexp.MustCompile(`^https://(?:(?:canary|ptb)\.)?(?:discord\.com|discordapp\.com)/api/webhooks/\d{5,30}/[\w-]{20,120}$`)

// ValidURL accepts Discord webhook URLs only.
func ValidURL(u string) bool { return webhookRe.MatchString(strings.TrimSpace(u)) }

// MaskURL shows enough of a webhook URL to recognise it; the last part is a secret.
func MaskURL(u string) string {
	p, err := url.Parse(u)
	if err != nil {
		return ""
	}
	parts := strings.Split(strings.Trim(p.Path, "/"), "/")
	if len(parts) < 4 {
		return p.Host
	}
	tok := parts[3]
	if len(tok) > 4 {
		tok = tok[len(tok)-4:]
	}
	return fmt.Sprintf("%s/api/webhooks/%s/••••••%s", p.Host, parts[2], tok)
}

// Field is one detail line: Label is a phrase key ("f.exit"), Value the text (or "@phrase" / "@phrase|name=value").
type Field struct {
	Label string
	Value string
}

// Event is something to tell the owner about.
type Event struct {
	Kind     string // one of the events above, or "test"
	ServerID string // "" for panel-wide events
	Server   string // server name
	Vars     map[string]string
	Fields   []Field
	Link     string // path in the panel ("/servers/<id>") for the "Open in panel" button
	At       time.Time
}

const isComponentsV2 = 1 << 15
const avatarURL = "https://cdn.heylatch.app/brand/mark-bg-512.png"

// esc keeps names and other free text from changing the message: no line breaks, no markdown, no links.
var controlChars = regexp.MustCompile("[\u0000-\u001f\u007f  ]+")
var mdChars = regexp.MustCompile(`([\\*_~` + "`" + `|>#\[\]()<:.])`)

func esc(s string, max int) string {
	s = controlChars.ReplaceAllString(s, " ")
	if len([]rune(s)) > max {
		s = string([]rune(s)[:max])
	}
	return mdChars.ReplaceAllString(s, `\$1`)
}

func phrases(lang string) map[string]string {
	if t, ok := texts[lang]; ok {
		return t
	}
	return texts["en"]
}

// Message builds the Components V2 payload for an event in a language ("en" or "tr"). panelURL, when set, is the
// panel's public address for the "Open in panel" button.
func Message(e Event, lang, panel, panelURL string) map[string]any {
	tr := phrases(lang)
	fill := func(s string, bold bool) string {
		name := esc(e.Server, 60)
		if bold {
			name = "**" + name + "**"
		}
		s = strings.ReplaceAll(s, "{server}", name)
		for k, v := range e.Vars {
			s = strings.ReplaceAll(s, "{"+k+"}", esc(resolve(lang, v), 200))
		}
		return s
	}
	title := fill(tr[e.Kind+".title"], false)
	text := fill(tr[e.Kind+".text"], true)

	blocks := []map[string]any{{"type": 10, "content": "### " + title + "\n" + text}}
	var lines []string
	for _, f := range e.Fields {
		if v := resolve(lang, f.Value); v != "" {
			lines = append(lines, fmt.Sprintf("**%s:** %s", tr[f.Label], esc(v, 300)))
		}
	}
	if len(lines) > 0 {
		blocks = append(blocks, map[string]any{"type": 10, "content": strings.Join(lines, "\n")})
	}
	blocks = append(blocks,
		map[string]any{"type": 14},
		map[string]any{"type": 10, "content": fmt.Sprintf("-# LatchAdmin · %s · <t:%d:f>", esc(panel, 60), e.At.Unix())},
	)
	if panelURL != "" {
		link := strings.TrimRight(panelURL, "/") + e.Link
		blocks = append(blocks, map[string]any{"type": 1, "components": []map[string]any{{"type": 2, "style": 5, "label": tr["openPanel"], "url": link}}})
	}
	return map[string]any{
		"username":         "LatchAdmin",
		"avatar_url":       avatarURL,
		"flags":            isComponentsV2,
		"allowed_mentions": map[string]any{"parse": []string{}},
		"components":       []map[string]any{{"type": 17, "components": blocks}},
	}
}

// Send posts one message. Discord's rate limit (429) is honoured once.
func Send(ctx context.Context, webhook string, msg map[string]any) error {
	if !ValidURL(webhook) {
		return errors.New("not a Discord webhook URL")
	}
	// Plain channel webhooks may send non-interactive components (link buttons included) with this flag.
	return call(ctx, http.MethodPost, strings.TrimSpace(webhook)+"?with_components=true", msg, nil)
}

// ErrUnknownMessage means the message to edit was deleted in Discord.
var ErrUnknownMessage = errors.New("the message was deleted in Discord")

// call sends one request to a webhook URL (or one of its message URLs) and turns Discord's answers into errors
// that never contain the URL. Discord's rate limit (429) is honoured once.
func call(ctx context.Context, method, target string, body, out any) error {
	for attempt := 0; attempt < 2; attempt++ {
		err := netx.JSON(ctx, method, target, nil, body, out)
		// Transport errors quote the request URL, and a webhook URL is a secret: keep only the cause.
		var ue *url.Error
		if errors.As(err, &ue) {
			return errors.New("couldn't reach Discord: " + Redact(ue.Err.Error()))
		}
		var he *netx.HTTPError
		if errors.As(err, &he) && he.Status == http.StatusTooManyRequests && attempt == 0 {
			time.Sleep(2 * time.Second)
			continue
		}
		if errors.As(err, &he) {
			var res struct {
				Code int `json:"code"`
			}
			_ = json.Unmarshal(he.Body, &res)
			switch {
			case res.Code == 10008:
				return ErrUnknownMessage
			case he.Status == http.StatusNotFound, he.Status == http.StatusUnauthorized:
				return errors.New("Discord says this webhook doesn't exist anymore")
			}
			return fmt.Errorf("Discord answered HTTP %d: %s", he.Status, Redact(string(he.Body)))
		}
		return err
	}
	return errors.New("Discord rate limit")
}

var linkRe = regexp.MustCompile(`https?://\S+`)

// Redact removes links (webhook URLs, signed download links) from text that is stored or sent, and caps it.
func Redact(s string) string {
	s = linkRe.ReplaceAllString(s, "[link]")
	if len(s) > 300 {
		s = s[:300] + "…"
	}
	return s
}

// Text returns a phrase in a language.
func Text(lang, key string) string { return phrases(lang)[key] }

// resolve turns "@key" or "@key|name=value" into a phrase in the webhook's language.
func resolve(lang, v string) string {
	if !strings.HasPrefix(v, "@") {
		return v
	}
	key, args, _ := strings.Cut(v[1:], "|")
	out := Text(lang, key)
	if args != "" {
		name, val, _ := strings.Cut(args, "=")
		out = strings.ReplaceAll(out, "{"+name+"}", val)
	}
	return out
}

var texts = map[string]map[string]string{
	"en": {
		"server.crash.title":     "{server} crashed",
		"server.crash.text":      "The server stopped unexpectedly. {restart}",
		"server.ready.title":     "{server} is online",
		"server.ready.text":      "The server started and accepts players.",
		"server.stop.title":      "{server} stopped",
		"server.stop.text":       "Someone stopped the server from the panel. It stays off until it's started again.",
		"server.restart.title":   "{server}: scheduled restart",
		"server.restart.text":    "The daily restart is starting. Players are disconnected for a moment.",
		"game.available.title":   "{game} {version} is out",
		"game.available.text":    "A new game version is available for {server}. {urgent}Players' launchers update too, so update the server soon.",
		"game.updated.title":     "{server} updated to {version}",
		"game.updated.text":      "The new runtime is installed. A backup of the previous version was made first.",
		"game.failed.title":      "{server}: update failed",
		"game.failed.text":       "The update didn't go through and the previous version was put back.",
		"panel.available.title":  "LatchAdmin {version} is out",
		"panel.available.text":   "A new panel version is available. {urgent}",
		"script.updated.title":   "{script} updated on {server}",
		"script.updated.text":    "A new version was installed from Latch automatically. {applies}",
		"players.record.title":   "{server}: new player record",
		"players.record.text":    "More players are online than ever before on this server.",
		"latch.lost.title":       "Latch link stopped working",
		"latch.lost.text":        "The account key was revoked on heylatch.app. Installed scripts keep running, but won't update until the panel is linked again from its Latch page.",
		"test.title":             "LatchAdmin is connected",
		"test.text":              "This channel is set up. The events you picked will show up here.",
		"urgent":                 "The Latch team marked it urgent. ",
		"restartIn":              "LatchAdmin restarts it in {in}.",
		"noRestart":              "Crash restarts are off, so it stays down.",
		"appliesNow":             "The resource was reloaded.",
		"appliesRestart":         "It runs from the next server restart.",
		"openPanel":              "Open in panel",
		"board.summary":          "**{online}** of {total} servers online",
		"board.players":          "**{n}** players",
		"board.player":           "**{n}** player",
		"board.state.online":     "Online",
		"board.state.starting":   "Starting",
		"board.state.restarting": "Restarting",
		"board.state.stopping":   "Shutting down",
		"board.state.updating":   "Updating",
		"board.state.offline":    "Offline",
		"board.f.players":        "Players",
		"board.f.address":        "Connect",
		"board.f.since":          "Started",
		"board.updated":          "Updated",
		"allServers":             "All",
		"f.server":               "Server",
		"f.game":                 "Game",
		"f.version":              "Version",
		"f.exit":                 "Exit",
		"f.uptime":               "Ran for",
		"f.players":              "Players online",
		"f.crashes":              "Crashes in a row",
		"f.port":                 "Port",
		"f.capacity":             "Slots",
		"f.startup":              "Started in",
		"f.installed":            "Installed",
		"f.latest":               "New",
		"f.backup":               "Backup",
		"f.target":               "Target version",
		"f.error":                "Error",
		"f.note":                 "Note",
		"seeConsole":             "The details are in the server's console.",
		"f.previous":             "Previous version",
		"f.record":               "New record",
		"f.previousRecord":       "Previous record",
		"f.time":                 "Time",
		"f.events":               "Events",
		"f.servers":              "Servers",
	},
	"tr": {
		"server.crash.title":     "{server} çöktü",
		"server.crash.text":      "Sunucu beklenmedik şekilde durdu. {restart}",
		"server.ready.title":     "{server} açık",
		"server.ready.text":      "Sunucu başladı ve oyuncu kabul ediyor.",
		"server.stop.title":      "{server} durdu",
		"server.stop.text":       "Sunucu panelden durduruldu. Yeniden başlatılana kadar kapalı kalır.",
		"server.restart.title":   "{server}: planlı yeniden başlatma",
		"server.restart.text":    "Günlük yeniden başlatma başlıyor. Oyuncuların bağlantısı kısa süre kesilir.",
		"game.available.title":   "{game} {version} çıktı",
		"game.available.text":    "{server} için yeni oyun sürümü var. {urgent}Oyuncuların launcher'ı da güncelleneceği için sunucuyu yakında güncelle.",
		"game.updated.title":     "{server} {version} sürümüne güncellendi",
		"game.updated.text":      "Yeni sürüm kuruldu. Önce eski sürümün yedeği alındı.",
		"game.failed.title":      "{server}: güncelleme başarısız",
		"game.failed.text":       "Güncelleme tamamlanamadı ve önceki sürüm geri yüklendi.",
		"panel.available.title":  "LatchAdmin {version} çıktı",
		"panel.available.text":   "Yeni bir panel sürümü var. {urgent}",
		"script.updated.title":   "{script}, {server} sunucusunda güncellendi",
		"script.updated.text":    "Latch'ten otomatik olarak yeni sürüm kuruldu. {applies}",
		"players.record.title":   "{server}: yeni oyuncu rekoru",
		"players.record.text":    "Bu sunucuda şimdiye kadarki en yüksek çevrimiçi oyuncu sayısına ulaşıldı.",
		"latch.lost.title":       "Latch bağlantısı çalışmıyor",
		"latch.lost.text":        "Hesap anahtarı heylatch.app'te iptal edilmiş. Kurulu script'ler çalışmaya devam eder ama panel Latch sayfasından yeniden bağlanana kadar güncellenmez.",
		"test.title":             "LatchAdmin bağlandı",
		"test.text":              "Bu kanal hazır. Seçtiğin olaylar burada görünecek.",
		"urgent":                 "Latch ekibi bu sürümü acil olarak işaretledi. ",
		"restartIn":              "LatchAdmin {in} sonra yeniden başlatacak.",
		"noRestart":              "Çökme sonrası yeniden başlatma kapalı, sunucu kapalı kalır.",
		"appliesNow":             "Resource yeniden yüklendi.",
		"appliesRestart":         "Bir sonraki sunucu yeniden başlatmasında devreye girer.",
		"openPanel":              "Panelde aç",
		"board.summary":          "{total} sunucudan **{online}** tanesi açık",
		"board.players":          "**{n}** oyuncu",
		"board.player":           "**{n}** oyuncu",
		"board.state.online":     "Açık",
		"board.state.starting":   "Başlıyor",
		"board.state.restarting": "Yeniden başlıyor",
		"board.state.stopping":   "Kapanıyor",
		"board.state.updating":   "Güncelleniyor",
		"board.state.offline":    "Kapalı",
		"board.f.players":        "Oyuncular",
		"board.f.address":        "Bağlan",
		"board.f.since":          "Açıldı",
		"board.updated":          "Güncellendi",
		"allServers":             "Tümü",
		"f.server":               "Sunucu",
		"f.game":                 "Oyun",
		"f.version":              "Sürüm",
		"f.exit":                 "Çıkış",
		"f.uptime":               "Çalışma süresi",
		"f.players":              "Çevrimiçi oyuncu",
		"f.crashes":              "Art arda çökme",
		"f.port":                 "Port",
		"f.capacity":             "Slot",
		"f.startup":              "Açılış süresi",
		"f.installed":            "Kurulu",
		"f.latest":               "Yeni",
		"f.backup":               "Yedek",
		"f.target":               "Hedef sürüm",
		"f.error":                "Hata",
		"f.note":                 "Not",
		"seeConsole":             "Ayrıntılar sunucunun konsolunda.",
		"f.previous":             "Önceki sürüm",
		"f.record":               "Yeni rekor",
		"f.previousRecord":       "Önceki rekor",
		"f.time":                 "Saat",
		"f.events":               "Olaylar",
		"f.servers":              "Sunucular",
	},
}
