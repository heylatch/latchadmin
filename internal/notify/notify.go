// Package notify sends panel events to Discord webhooks.
//
// Only Discord webhook URLs are accepted (discord.com / discordapp.com /api/webhooks/...), so this can't be used to
// send data anywhere else. Messages carry the event and the server's name, never config, keys or console output.
package notify

import (
	"context"
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

// MaskURL shows enough of a webhook URL to recognise it.
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
	return fmt.Sprintf("%s/api/webhooks/%s/…%s", p.Host, parts[2], tok)
}

// Event is something to tell the owner about.
type Event struct {
	Kind     string // one of the events above
	ServerID string // "" for panel-wide events
	Server   string // server name
	Vars     map[string]string
	At       time.Time
}

// Level picks the embed color; Discord shows it as the bar on the left of the message.
func level(kind string) int {
	switch kind {
	case "server.crash", "game.failed", "latch.lost":
		return 0xe34948 // red
	case "server.ready", "game.updated", "script.updated":
		return 0x1baf7a // green
	case "game.available", "panel.available":
		return 0xeda100 // amber
	case "players.record":
		return 0x6250d6 // violet
	default:
		return 0x2a78d6 // blue
	}
}

type embed struct {
	Title       string  `json:"title"`
	Description string  `json:"description,omitempty"`
	Color       int     `json:"color"`
	Fields      []field `json:"fields,omitempty"`
	Timestamp   string  `json:"timestamp"`
	Footer      footer  `json:"footer"`
}

type field struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Inline bool   `json:"inline"`
}

type footer struct {
	Text string `json:"text"`
}

// Message renders an event in a language ("en" or "tr").
func Message(e Event, lang, panel string) map[string]any {
	tr := texts["en"]
	if t, ok := texts[lang]; ok {
		tr = t
	}
	title, desc := tr[e.Kind+".title"], tr[e.Kind+".text"]
	for k, v := range e.Vars {
		v = resolve(lang, v)
		title = strings.ReplaceAll(title, "{"+k+"}", v)
		desc = strings.ReplaceAll(desc, "{"+k+"}", v)
	}
	title = strings.ReplaceAll(title, "{server}", e.Server)
	desc = strings.ReplaceAll(desc, "{server}", e.Server)
	em := embed{Title: title, Description: desc, Color: level(e.Kind), Timestamp: e.At.UTC().Format(time.RFC3339), Footer: footer{Text: "LatchAdmin · " + panel}}
	if e.Kind == "test" {
		em = embed{Title: tr["test.title"], Description: tr["test.text"], Color: level(e.Kind), Timestamp: e.At.UTC().Format(time.RFC3339), Footer: footer{Text: "LatchAdmin · " + panel}}
	}
	return map[string]any{"username": "LatchAdmin", "embeds": []embed{em}, "allowed_mentions": map[string]any{"parse": []string{}}}
}

// Send posts one message. Discord's rate limit (429) is honoured once.
func Send(ctx context.Context, webhook string, msg map[string]any) error {
	if !ValidURL(webhook) {
		return errors.New("not a Discord webhook URL")
	}
	for attempt := 0; attempt < 2; attempt++ {
		err := netx.JSON(ctx, http.MethodPost, webhook, nil, msg, nil)
		var he *netx.HTTPError
		if errors.As(err, &he) && he.Status == http.StatusTooManyRequests && attempt == 0 {
			time.Sleep(2 * time.Second)
			continue
		}
		if errors.As(err, &he) {
			switch he.Status {
			case http.StatusNotFound, http.StatusUnauthorized:
				return errors.New("Discord says this webhook doesn't exist anymore")
			}
			return fmt.Errorf("Discord answered HTTP %d", he.Status)
		}
		return err
	}
	return errors.New("Discord rate limit")
}

var texts = map[string]map[string]string{
	"en": {
		"server.crash.title":    "{server} crashed",
		"server.crash.text":     "The server stopped unexpectedly ({exit}). {restart}",
		"server.ready.title":    "{server} is online",
		"server.ready.text":     "The server started and accepts players.",
		"server.stop.title":     "{server} stopped",
		"server.stop.text":      "The server was stopped.",
		"server.restart.title":  "{server}: scheduled restart",
		"server.restart.text":   "The daily {time} restart is starting.",
		"game.available.title":  "{game} {version} is out",
		"game.available.text":   "{server} runs {current}. {urgent}Update it from the server's Updates tab.",
		"game.updated.title":    "{server} updated to {version}",
		"game.updated.text":     "A backup of the previous version was made first.",
		"game.failed.title":     "{server}: update failed",
		"game.failed.text":      "{error}",
		"panel.available.title": "LatchAdmin {version} is out",
		"panel.available.text":  "This panel runs {current}. {urgent}",
		"script.updated.title":  "{server}: {script} updated to {version}",
		"script.updated.text":   "Installed from Latch automatically. {applies}",
		"players.record.title":  "{server}: new player record",
		"players.record.text":   "{players} players online at the same time.",
		"latch.lost.title":      "Latch link stopped working",
		"latch.lost.text":       "The account key was revoked on heylatch.app. Scripts keep running, but won't update until the panel is linked again.",
		"test.title":            "LatchAdmin test message",
		"test.text":             "This webhook is set up. Panel events you chose will appear here.",
		"urgent":                "The Latch team marked it urgent. ",
		"restartIn":             "LatchAdmin restarts it in {in}.",
		"noRestart":             "Crash restarts are off.",
		"appliesNow":            "The resource was reloaded.",
		"appliesRestart":        "It runs from the next server restart.",
	},
	"tr": {
		"server.crash.title":    "{server} çöktü",
		"server.crash.text":     "Sunucu beklenmedik şekilde durdu ({exit}). {restart}",
		"server.ready.title":    "{server} açık",
		"server.ready.text":     "Sunucu başladı ve oyuncu kabul ediyor.",
		"server.stop.title":     "{server} durdu",
		"server.stop.text":      "Sunucu durduruldu.",
		"server.restart.title":  "{server}: planlı yeniden başlatma",
		"server.restart.text":   "Her gün saat {time} yapılan yeniden başlatma başlıyor.",
		"game.available.title":  "{game} {version} çıktı",
		"game.available.text":   "{server} şu an {current} sürümünde. {urgent}Sunucunun Güncellemeler sekmesinden güncelleyebilirsin.",
		"game.updated.title":    "{server} {version} sürümüne güncellendi",
		"game.updated.text":     "Önce eski sürümün yedeği alındı.",
		"game.failed.title":     "{server}: güncelleme başarısız",
		"game.failed.text":      "{error}",
		"panel.available.title": "LatchAdmin {version} çıktı",
		"panel.available.text":  "Bu panel {current} sürümünde. {urgent}",
		"script.updated.title":  "{server}: {script} {version} sürümüne güncellendi",
		"script.updated.text":   "Latch'ten otomatik kuruldu. {applies}",
		"players.record.title":  "{server}: yeni oyuncu rekoru",
		"players.record.text":   "Aynı anda {players} oyuncu çevrimiçi.",
		"latch.lost.title":      "Latch bağlantısı çalışmıyor",
		"latch.lost.text":       "Hesap anahtarı heylatch.app'te iptal edilmiş. Script'ler çalışmaya devam eder ama panel yeniden bağlanana kadar güncellenmez.",
		"test.title":            "LatchAdmin deneme mesajı",
		"test.text":             "Bu webhook kuruldu. Seçtiğin panel olayları burada görünecek.",
		"urgent":                "Latch ekibi bu sürümü acil olarak işaretledi. ",
		"restartIn":             "LatchAdmin {in} sonra yeniden başlatacak.",
		"noRestart":             "Çökme sonrası yeniden başlatma kapalı.",
		"appliesNow":            "Resource yeniden yüklendi.",
		"appliesRestart":        "Bir sonraki sunucu yeniden başlatmasında devreye girer.",
	},
}

// Text returns a shared phrase in a language (used to fill {urgent}, {restart}, {applies}).
func Text(lang, key string) string {
	if t, ok := texts[lang]; ok {
		return t[key]
	}
	return texts["en"][key]
}

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
