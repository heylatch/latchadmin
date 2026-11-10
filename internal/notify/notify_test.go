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
	e := Event{Kind: "server.crash", Server: "Main", Vars: map[string]string{"exit": "exit code 3", "restart": "@restartIn|in=10s"}, At: time.Now()}
	em := Message(e, "en", "Panel")["embeds"].([]embed)[0]
	if em.Title != "Main crashed" || !strings.Contains(em.Description, "exit code 3") || !strings.Contains(em.Description, "restarts it in 10s") {
		t.Fatalf("got %+v", em)
	}
	tr := Message(e, "tr", "Panel")["embeds"].([]embed)[0]
	if tr.Title != "Main çöktü" || !strings.Contains(tr.Description, "10s sonra") {
		t.Fatalf("got %+v", tr)
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
