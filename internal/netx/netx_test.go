package netx

import (
	"net/url"
	"testing"
)

func TestAllowed(t *testing.T) {
	ok := []string{
		"https://api.heylatch.app/public/v1/me",
		"https://api.mafiahub.dev/mods/kcdc/releases/latest",
		"https://abc123.r2.cloudflarestorage.com/bucket/x.zip?sig=1",
		"https://github.com/heylatch/latchadmin/releases/download/v1.0.0/SHA256SUMS",
		"https://objects.githubusercontent.com/x",
	}
	bad := []string{
		"http://api.heylatch.app/public/v1/me",           // not HTTPS
		"https://api.heylatch.app.evil.com/",              // suffix trick
		"https://evilr2.cloudflarestorage.com.evil.com/",  // suffix trick
		"https://r2.cloudflarestorage.com/",               // bare suffix
		"https://user@api.heylatch.app/",                  // userinfo
		"https://api.heylatch.app:8443/",                  // other port
		"https://heylatch.app/",                           // not an API host
		"https://127.0.0.1/",
		"file:///etc/passwd",
	}
	for _, s := range ok {
		u, _ := url.Parse(s)
		if !Allowed(u) {
			t.Errorf("should allow %s", s)
		}
	}
	for _, s := range bad {
		u, _ := url.Parse(s)
		if Allowed(u) {
			t.Errorf("should refuse %s", s)
		}
	}
}
