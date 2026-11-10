package notify

import (
	"strings"
	"testing"
	"time"
)

func TestValidURL(t *testing.T) {
	ok := []string{
		"https://discord.com/api/webhooks/123456789012345678/abcdefghijklmnopqrstuvwxyz_ABC-123",
		"https://canary.discord.com/api/webhooks/123456789012345678/abcdefghijklmnopqrstuvwxyz",
		"https://discordapp.com/api/webhooks/123456789012345678/abcdefghijklmnopqrstuvwxyz",
	}
	bad := []string{
		"http://discord.com/api/webhooks/123456789012345678/abcdefghijklmnopqrstuvwxyz",
		"https://discord.com.evil.com/api/webhooks/123456789012345678/abcdefghijklmnopqrstuvwxyz",
		"https://discord.com/api/users/123456789012345678/abcdefghijklmnopqrstuvwxyz",
		"https://example.com/api/webhooks/123456789012345678/abcdefghijklmnopqrstuvwxyz",
		"https://discord.com/api/webhooks/123/abc?wait=true",
	}
	for _, u := range ok {
		if !ValidURL(u) {
			t.Errorf("should accept %s", u)
		}
	}
	for _, u := range bad {
		if ValidURL(u) {
			t.Errorf("should refuse %s", u)
		}
	}
}

func TestMessage(t *testing.T) {
	e := Event{Kind: "server.crash", Server: "Main*", Vars: map[string]string{"restart": "@restartIn|in=10s"}, Fields: []Field{{Label: "f.exit", Value: "exit code 3"}}, Link: "/servers/x", At: time.Now()}
	msg := Message(e, "en", "Panel", "https://panel.example.com")
	if msg["flags"] != isComponentsV2 {
		t.Fatal("not a Components V2 message")
	}
	box := msg["components"].([]map[string]any)[0]["components"].([]map[string]any)
	head := box[0]["content"].(string)
	if !strings.Contains(head, "### Main\\* crashed") || !strings.Contains(head, "restarts it in 10s") {
		t.Fatalf("head %q", head)
	}
	if !strings.Contains(box[1]["content"].(string), "**Exit:** exit code 3") {
		t.Fatalf("fields %q", box[1]["content"])
	}
	btn := box[len(box)-1]["components"].([]map[string]any)[0]
	if btn["url"] != "https://panel.example.com/servers/x" {
		t.Fatalf("button %v", btn)
	}
	tr := Message(e, "tr", "Panel", "")["components"].([]map[string]any)[0]["components"].([]map[string]any)
	if !strings.Contains(tr[0]["content"].(string), "çöktü") || !strings.Contains(tr[0]["content"].(string), "10s sonra") {
		t.Fatalf("tr %q", tr[0]["content"])
	}
	for _, g := range Groups {
		for _, ev := range g.Events {
			for _, lang := range []string{"en", "tr"} {
				if texts[lang][ev+".title"] == "" || texts[lang][ev+".text"] == "" {
					t.Errorf("%s: missing %s text", lang, ev)
				}
			}
		}
	}
}

func TestStatusMessage(t *testing.T) {
	n := 12
	b := Board{Title: "Our *servers*", Note: "Open the launcher", At: time.Unix(1700000000, 0), Buttons: []Button{{Label: "Discord", URL: "https://discord.gg/x"}},
		Servers: []BoardEntry{
			{Name: "Main", Game: "Arcane MP 1.12.2", State: "online", Players: &n, MaxPlayers: 128, Address: "play.example.com:27015", Since: time.Unix(1699990000, 0)},
			{Name: "Test", Game: "Arcane MP 1.12.2", State: "offline"},
		}}
	msg := StatusMessage(b, "en")
	parts := msg["components"].([]map[string]any)
	if len(parts) != 5 { // header, 2 servers, footer, buttons
		t.Fatalf("parts %d", len(parts))
	}
	head := parts[0]["components"].([]map[string]any)[0]["content"].(string)
	if !strings.Contains(head, `## Our \*servers\*`) || !strings.Contains(head, "**1** of 2 servers online · **12** players") {
		t.Fatalf("head %q", head)
	}
	main := parts[1]["components"].([]map[string]any)[0]["content"].(string)
	if parts[1]["accent_color"] != colorOnline || !strings.Contains(main, "**Players:** 12 / 128") || !strings.Contains(main, "`play.example.com:27015`") {
		t.Fatalf("main %q", main)
	}
	if parts[2]["accent_color"] != colorOffline || strings.Contains(parts[2]["components"].([]map[string]any)[0]["content"].(string), "Players") {
		t.Fatal("offline server shows players")
	}
	// The "updated" time alone doesn't count as a change.
	later := b
	later.At = b.At.Add(time.Hour)
	if BoardHash(b, "en") != BoardHash(later, "en") {
		t.Fatal("hash changes with time")
	}
	m := 13
	later.Servers = append([]BoardEntry{}, b.Servers...)
	later.Servers[0].Players = &m
	if BoardHash(b, "en") == BoardHash(later, "en") {
		t.Fatal("hash ignores players")
	}
	for _, k := range []string{"board.summary", "board.state.online", "board.state.restarting", "board.f.address", "board.updated"} {
		if texts["tr"][k] == "" {
			t.Errorf("tr: missing %s", k)
		}
	}
	if !ValidAddress("[2001:db8::1]:27015") || ValidAddress("play.example.com:27015`") || ValidAddress("a b") {
		t.Fatal("address check")
	}
	if ValidButtonURL("http://example.com") || !ValidButtonURL("https://discord.gg/abc") {
		t.Fatal("button check")
	}
}
