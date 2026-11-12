// Package bridge holds the small "latchadmin" resource LatchAdmin puts into a server's resources/ folder, so the
// panel can show announcements and restart warnings on players' screens. The files are built into the panel (no
// download), written only into a folder that is LatchAdmin's own, and replaced when their version changes.
package bridge

import (
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/heylatch/latchadmin/internal/archive"
)

//go:embed resource
var files embed.FS

// Folder is the resource's folder name inside resources/.
const Folder = "latchadmin"

type manifest struct {
	Version    string `json:"version"`
	LatchAdmin struct {
		Managed bool `json:"managed"`
	} `json:"latchadmin"`
}

func readManifest(raw []byte) (manifest, error) {
	var m manifest
	err := json.Unmarshal(raw, &m)
	return m, err
}

// Version is the version of the built-in resource.
func Version() string {
	raw, _ := files.ReadFile("resource/package.json")
	m, _ := readManifest(raw)
	return m.Version
}

// ErrNotOurs means resources/latchadmin exists but wasn't put there by LatchAdmin.
var ErrNotOurs = errors.New("resources/latchadmin exists and isn't LatchAdmin's own resource; rename it to use in-game notices")

// Install writes the resource into serverDir/resources/latchadmin when it's missing or older. It reports whether
// anything changed.
func Install(serverDir string) (bool, error) {
	target := filepath.Join(serverDir, "resources", Folder)
	if info, err := os.Lstat(target); err == nil {
		if archive.IsLink(info) || !info.IsDir() {
			return false, ErrNotOurs
		}
		raw, err := os.ReadFile(filepath.Join(target, "package.json"))
		if err != nil {
			return false, ErrNotOurs
		}
		cur, err := readManifest(raw)
		if err != nil || !cur.LatchAdmin.Managed {
			return false, ErrNotOurs
		}
		if cur.Version == Version() && complete(target) {
			return false, nil
		}
	}
	// Write next to it and swap, so the server never sees a half-written resource.
	tmp := target + ".latchadmin-new"
	_ = os.RemoveAll(tmp)
	err := fs.WalkDir(files, "resource", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel := strings.TrimPrefix(strings.TrimPrefix(p, "resource"), "/")
		out := filepath.Join(tmp, filepath.FromSlash(rel))
		if d.IsDir() {
			return os.MkdirAll(out, 0o755)
		}
		raw, err := files.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(out, raw, 0o644)
	})
	if err != nil {
		_ = os.RemoveAll(tmp)
		return false, err
	}
	if err := os.RemoveAll(target); err != nil {
		_ = os.RemoveAll(tmp)
		return false, err
	}
	if err := os.Rename(tmp, target); err != nil {
		return false, err
	}
	return true, nil
}

// complete reports whether every built-in file is present and unchanged.
func complete(target string) bool {
	ok := true
	_ = fs.WalkDir(files, "resource", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !ok {
			return err
		}
		want, _ := files.ReadFile(p)
		got, err := os.ReadFile(filepath.Join(target, filepath.FromSlash(strings.TrimPrefix(p, "resource/"))))
		ok = err == nil && bytes.Equal(got, want)
		return nil
	})
	return ok
}

// Remove deletes the resource, but only LatchAdmin's own.
func Remove(serverDir string) error {
	target := filepath.Join(serverDir, "resources", Folder)
	info, err := os.Lstat(target)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if archive.IsLink(info) || !info.IsDir() {
		return ErrNotOurs
	}
	raw, err := os.ReadFile(filepath.Join(target, "package.json"))
	if err != nil {
		return ErrNotOurs
	}
	if m, err := readManifest(raw); err != nil || !m.LatchAdmin.Managed {
		return ErrNotOurs
	}
	return os.RemoveAll(target)
}

// Notice is what a player's screen shows.
type Notice struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"` // announce | restart
	Label   string `json:"label"`
	Title   string `json:"title,omitempty"`
	Text    string `json:"text,omitempty"`
	Seconds int    `json:"seconds,omitempty"` // restart: countdown
	Now     string `json:"now,omitempty"`     // restart: shown when the countdown ends
}

var texts = map[string]map[string]string{
	"en": {
		"announce":     "Announcement",
		"announceFrom": "Announcement from %s",
		"restartLabel": "Scheduled restart",
		"restartTitle": "The server restarts in %s",
		"restartText":  "Finish what you're doing and find a safe spot.",
		"restartNow":   "Restarting now",
		"minute":       "1 minute",
		"minutes":      "%d minutes",
	},
	"tr": {
		"announce":     "Duyuru",
		"announceFrom": "%s tarafından duyuru",
		"restartLabel": "Planlı yeniden başlatma",
		"restartTitle": "Sunucu %s sonra yeniden başlıyor",
		"restartText":  "Yaptığın işi bitir ve güvenli bir yere geç.",
		"restartNow":   "Şimdi yeniden başlıyor",
		"minute":       "1 dakika",
		"minutes":      "%d dakika",
	},
}

func text(lang, key string) string {
	if t, ok := texts[lang]; ok {
		return t[key]
	}
	return texts["en"][key]
}

// Announcement builds an announcement notice. author is shown only when given.
func Announcement(lang, title, body, author string) Notice {
	label := text(lang, "announce")
	if author != "" {
		label = fmt.Sprintf(text(lang, "announceFrom"), author)
	}
	return Notice{Kind: "announce", Label: label, Title: title, Text: body}
}

// RestartWarning builds a countdown notice for a restart in `seconds`.
func RestartWarning(lang string, seconds int) Notice {
	mins := (seconds + 59) / 60
	when := text(lang, "minute")
	if mins != 1 {
		when = fmt.Sprintf(text(lang, "minutes"), mins)
	}
	return Notice{Kind: "restart", Label: text(lang, "restartLabel"), Title: fmt.Sprintf(text(lang, "restartTitle"), when),
		Text: text(lang, "restartText"), Seconds: seconds, Now: text(lang, "restartNow")}
}
