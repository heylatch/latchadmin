// Package netx is the only way LatchAdmin talks to the internet.
//
// Every outgoing request goes to a host on a fixed allow-list, over HTTPS, and every redirect is checked against
// the same list. Nothing a server response says can make the panel contact another host. This is what lets you
// verify, by reading this one file, where LatchAdmin can send data and download code from.
package netx

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/heylatch/latchadmin/internal/buildinfo"
)

// Hosts LatchAdmin may contact.
var allowedHosts = map[string]bool{
	"api.heylatch.app": true, // Latch: account link, Library, version manifest, panel report
	"api.mafiahub.dev": true, // MafiaHub: game server releases
	"api.github.com":   true, // LatchAdmin releases (fallback when the Latch manifest is off)
	"github.com":       true, // LatchAdmin release downloads
	// Discord webhooks the owner sets up for notifications (package notify only accepts /api/webhooks/ URLs).
	"discord.com":        true,
	"discordapp.com":     true,
	"canary.discord.com": true,
	"ptb.discord.com":    true,
}

// Download hosts that the hosts above redirect to.
var allowedSuffixes = []string{
	".r2.cloudflarestorage.com", // MafiaHub release zips and Latch packages
	".githubusercontent.com",    // GitHub release assets
}

// ErrNotAllowed is returned for any URL outside the allow-list.
var ErrNotAllowed = errors.New("host not allowed")

// testHosts lets tests point the client at a local server. It is never set outside tests.
var testHosts = map[string]bool{}

// AllowTestHost permits plain-HTTP requests to host (e.g. "127.0.0.1:1234"). For tests only.
func AllowTestHost(host string) { testHosts[host] = true }

// Allowed reports whether u may be requested.
func Allowed(u *url.URL) bool {
	if testHosts[u.Host] {
		return true
	}
	if u.Scheme != "https" || u.User != nil || u.Port() != "" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	if allowedHosts[host] {
		return true
	}
	for _, s := range allowedSuffixes {
		if strings.HasSuffix(host, s) && len(host) > len(s) {
			return true
		}
	}
	return false
}

var client = &http.Client{
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("too many redirects")
		}
		if !Allowed(req.URL) {
			return fmt.Errorf("%w: redirect to %s", ErrNotAllowed, req.URL.Host)
		}
		return nil
	},
	Transport: &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		TLSHandshakeTimeout:   15 * time.Second,
		ResponseHeaderTimeout: 60 * time.Second,
		IdleConnTimeout:       90 * time.Second,
		MaxIdleConnsPerHost:   4,
	},
}

// UserAgent identifies the panel version to the hosts above.
func UserAgent() string { return "LatchAdmin/" + buildinfo.Version }

// Do sends req after checking its URL.
func Do(req *http.Request) (*http.Response, error) {
	if !Allowed(req.URL) {
		return nil, fmt.Errorf("%w: %s", ErrNotAllowed, req.URL.Host)
	}
	req.Header.Set("User-Agent", UserAgent())
	return client.Do(req)
}

// Get downloads url. The caller closes the body.
func Get(ctx context.Context, rawURL string, header http.Header) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	for k, v := range header {
		req.Header[k] = v
	}
	return Do(req)
}

// HTTPError is a non-2xx answer. Body holds up to 4 KB of it.
type HTTPError struct {
	Status int
	Body   []byte
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("HTTP %d: %s", e.Status, strings.TrimSpace(string(e.Body)))
}

// JSON sends body (if not nil) as JSON and decodes the answer into out (if not nil).
func JSON(ctx context.Context, method, rawURL string, header http.Header, body, out any) error {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, rawURL, rd)
	if err != nil {
		return err
	}
	for k, v := range header {
		req.Header[k] = v
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode > 299 {
		b, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
		return &HTTPError{Status: res.StatusCode, Body: b}
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(io.LimitReader(res.Body, 8<<20)).Decode(out)
}

var noRedirect = &http.Client{
	Transport:     client.Transport,
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
}

// DoNoRedirect sends req and returns a redirect response as is, so the caller can read Location. The Location is
// still only usable through Do/Get, which check it against the allow-list.
func DoNoRedirect(req *http.Request) (*http.Response, error) {
	if !Allowed(req.URL) {
		return nil, fmt.Errorf("%w: %s", ErrNotAllowed, req.URL.Host)
	}
	req.Header.Set("User-Agent", UserAgent())
	return noRedirect.Do(req)
}
