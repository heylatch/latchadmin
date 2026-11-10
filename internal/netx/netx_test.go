package netx

import (
	"net/url"
	"testing"
)

func TestAllowed(t *testing.T) {
	ok := []string{
		"https://api.heylatch.app/public/v1/me",
		"https://api.mafiahub.dev/mods/kcdc/releases/latest",
		"https://2a847ef49aad6a6bad3e183d42e129eb.r2.cloudflarestorage.com/mafiahub-artifacts/x.zip?sig=1",
		"https://4dc16afe8ffbae7eee8ad9ba2e789ac9.r2.cloudflarestorage.com/private/x.zip?sig=1",
		"https://bucket.4dc16afe8ffbae7eee8ad9ba2e789ac9.r2.cloudflarestorage.com/x.zip",
		"https://release-assets.githubusercontent.com/github-production-release-asset/1/2",
		"https://github.com/heylatch/latchadmin/releases/download/v1.0.0/SHA256SUMS",
		"https://objects.githubusercontent.com/x",
		"https://discord.com/api/webhooks/1/abc",
	}
	bad := []string{
		"http://api.heylatch.app/public/v1/me",           // not HTTPS
		"https://api.heylatch.app.evil.com/",             // suffix trick
		"https://evilr2.cloudflarestorage.com.evil.com/", // suffix trick
		"https://r2.cloudflarestorage.com/",              // bare suffix
		"https://abc123.r2.cloudflarestorage.com/x.zip",  // someone else's R2 account
		"https://a.b.4dc16afe8ffbae7eee8ad9ba2e789ac9.r2.cloudflarestorage.com.evil.com/",
		"https://raw.githubusercontent.com/anyone/repo/main/x", // anyone's files
		"https://user@api.heylatch.app/",                       // userinfo
		"https://api.heylatch.app:8443/",                       // other port
		"https://heylatch.app/",                                // not an API host
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
