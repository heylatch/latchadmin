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
