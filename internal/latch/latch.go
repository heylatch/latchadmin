// Package latch talks to Latch (api.heylatch.app) for the panel: linking an account, listing and installing the
// scripts in the owner's Library, reading the version manifest and sending the panel report.
//
// Direction of trust: the panel calls Latch; Latch never calls the panel. Answers from Latch are data (versions,
// flags, a download link checked against the netx allow-list). Nothing in them is run, and no path in them is used
// to write files: install locations come from the package's own folder name, validated like any zip entry.
package latch

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/heylatch/latchadmin/internal/archive"
	"github.com/heylatch/latchadmin/internal/netx"
	"github.com/heylatch/latchadmin/internal/resources"
)

const base = "https://api.heylatch.app/public/v1"

// baseURL can be changed by tests only.
var baseURL = base

// SetBaseForTest points the client at a test server.
func SetBaseForTest(u string) { baseURL = u }

// ErrUnauthorized means the account key no longer works (revoked on heylatch.app).
var ErrUnauthorized = errors.New("the Latch key was revoked or is invalid")

func call(ctx context.Context, key, method, path string, body, out any) error {
	h := http.Header{}
	if key != "" {
		h.Set("Authorization", "Bearer "+key)
	}
	err := netx.JSON(ctx, method, baseURL+path, h, body, out)
	var he *netx.HTTPError
	if errors.As(err, &he) {
		var e struct {
			Error struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		_ = json.Unmarshal(he.Body, &e)
		if he.Status == http.StatusUnauthorized {
			return ErrUnauthorized
		}
		if e.Error.Message != "" {
			return &APIError{Status: he.Status, Code: e.Error.Code, Message: e.Error.Message}
		}
	}
	return err
}

// APIError is an error answer from Latch, with its English message.
type APIError struct {
	Status  int
	Code    string
	Message string
}

func (e *APIError) Error() string { return e.Message }

// ---------- Linking ----------

type DeviceStart struct {
	DeviceCode        string `json:"deviceCode"`
	UserCode          string `json:"userCode"`
	VerifyURL         string `json:"verifyUrl"`
	VerifyURLComplete string `json:"verifyUrlComplete"`
	Interval          int    `json:"interval"`
	ExpiresIn         int    `json:"expiresIn"`
}

// StartLink begins linking; the owner approves userCode on heylatch.app.
func StartLink(ctx context.Context, deviceName string) (DeviceStart, error) {
	var out DeviceStart
	err := call(ctx, "", http.MethodPost, "/device/start", map[string]string{"deviceName": deviceName}, &out)
	return out, err
}

// PollLink returns "pending", "approved" (with the key), "denied" or "expired".
func PollLink(ctx context.Context, deviceCode string) (string, string, error) {
	var out struct {
		Status string `json:"status"`
		Key    string `json:"key"`
	}
	err := call(ctx, "", http.MethodPost, "/device/poll", map[string]string{"deviceCode": deviceCode}, &out)
	return out.Status, out.Key, err
}

type Account struct {
	AccountID   string `json:"accountId"`
	Username    string `json:"username"`
	DisplayName string `json:"displayName"`
}

func Me(ctx context.Context, key string) (Account, error) {
	var out Account
	err := call(ctx, key, http.MethodGet, "/me", nil, &out)
	return out, err
}

// ---------- Library ----------

type LibraryItem struct {
	Script struct {
		ID            string  `json:"id"`
		Slug          string  `json:"slug"`
		Name          string  `json:"name"`
		Ecosystem     string  `json:"ecosystem"`
		LatestVersion *string `json:"latestVersion"`
	} `json:"script"`
	Workspace struct {
		Slug string `json:"slug"`
		Name string `json:"name"`
	} `json:"workspace"`
	Status string `json:"status"`
	Own    bool   `json:"own"`
}

// Usable reports whether the item can be installed (access active, published, a MafiaHub script).
func (i LibraryItem) Usable() bool {
	return (i.Status == "active" || i.Own) && i.Script.LatestVersion != nil && i.Script.Ecosystem == "mafiahub"
}

func Library(ctx context.Context, key string) ([]LibraryItem, error) {
	var out struct {
		Data []LibraryItem `json:"data"`
	}
	err := call(ctx, key, http.MethodGet, "/me/library", nil, &out)
	return out.Data, err
}

// ---------- Installing ----------

// Installed maps script id → the resource that holds it, from each resource's latch.json.
func Installed(serverDir string) map[string]resources.Resource {
	list, _ := resources.List(serverDir)
	out := map[string]resources.Resource{}
	for _, r := range list {
		if r.Latch != nil {
			out[r.Latch.ScriptID] = r
		}
	}
	return out
}

// InstallResult says what Install did.
type InstallResult struct {
	Script   string   `json:"script"`
	Folder   string   `json:"folder"`
	Version  string   `json:"version"`
	Files    int      `json:"files"`
	Kept     []string `json:"kept"`
	UpToDate bool     `json:"upToDate"`
	Updated  bool     `json:"updated"`
}

var linkRe = regexp.MustCompile(`https?://\S+`)

// checkPackage makes sure the zip is the script and version that was asked for: its latch.json (written by Latch
// into every package) must name them. A package for another script can't replace this one.
func checkPackage(zr *zip.Reader, root, scriptID, version string) error {
	for _, f := range zr.File {
		if f.Name != root+"latch.json" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		defer rc.Close()
		var meta resources.Latch
		if err := json.NewDecoder(io.LimitReader(rc, 64<<10)).Decode(&meta); err != nil {
			return fmt.Errorf("package: latch.json is unreadable: %w", err)
		}
		if meta.ScriptID != scriptID || meta.Version != version {
			return fmt.Errorf("package is for %s %s, not the version asked for", meta.ScriptID, meta.Version)
		}
		return nil
	}
	return errors.New("package has no latch.json; it doesn't look like a Latch package")
}

// DefaultKeep: files a server owner edits inside a resource; an update leaves them alone.
var DefaultKeep = []string{"config.json", "config/**", "data/**", "*.config.json"}

// Install downloads the owner's copy of a script version and writes it into serverDir/resources.
// Same rules as `latch pull`: it never writes through a link and never over a folder that isn't a Latch package.
func Install(ctx context.Context, key, serverDir string, item LibraryItem, version string) (InstallResult, error) {
	res := InstallResult{Script: item.Script.Slug, Version: version}
	if cur, ok := Installed(serverDir)[item.Script.ID]; ok && cur.Latch.Version == version {
		res.Folder, res.UpToDate = cur.Folder, true
		return res, nil
	}
	var dl struct {
		FileName string `json:"fileName"`
		URL      string `json:"url"`
	}
	path := "/me/library/" + url.PathEscape(item.Script.Slug) + "/versions/" + url.PathEscape(version) + "/download"
	if err := call(ctx, key, http.MethodPost, path, nil, &dl); err != nil {
		return res, err
	}
	if strings.HasSuffix(dl.FileName, ".jar") {
		return res, errors.New("this is a Hytale script; LatchAdmin runs MafiaHub servers")
	}
	resp, err := netx.Get(ctx, dl.URL, nil)
	if err != nil {
		// The link is signed: it would let anyone who reads the console log download the package.
		return res, errors.New("package download failed: " + linkRe.ReplaceAllString(err.Error(), "[download link]"))
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return res, fmt.Errorf("package download: HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 512<<20))
	if err != nil {
		return res, err
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return res, fmt.Errorf("package: %w", err)
	}
	root := archive.CommonRoot(zr.File)
	if err := checkPackage(zr, root, item.Script.ID, version); err != nil {
		return res, err
	}
	folder := strings.TrimSuffix(root, "/")
	if folder == "" {
		folder = item.Script.Slug
	}
	if !resources.ValidFolder(folder) {
		return res, fmt.Errorf("package folder %q isn't a valid resource name", folder)
	}
	// An installed copy may sit under another folder name (e.g. renamed, or disabled): update it in place.
	resDir := filepath.Join(serverDir, "resources")
	if cur, ok := Installed(serverDir)[item.Script.ID]; ok {
		folder = cur.Folder
		if !cur.Enabled {
			resDir = filepath.Join(serverDir, "resources-disabled")
		}
	}
	target := filepath.Join(resDir, folder)
	if info, err := os.Lstat(target); err == nil {
		if archive.IsLink(info) {
			return res, fmt.Errorf("%s is a link to another folder (probably source code); LatchAdmin won't write through it", folder)
		}
		if _, err := os.Stat(filepath.Join(target, "latch.json")); err != nil {
			entries, _ := os.ReadDir(target)
			if len(entries) > 0 {
				return res, fmt.Errorf("resources/%s exists and isn't a Latch package; move it away first", folder)
			}
		}
		res.Updated = true
	}
	if err := os.MkdirAll(target, 0o755); err != nil {
		return res, err
	}
	out, err := archive.Extract(zr, target, archive.Options{Prefix: root, MaxBytes: 1 << 30, Keep: func(rel string) bool { return archive.GlobMatch(DefaultKeep, rel) }})
	res.Folder, res.Files, res.Kept = folder, out.Files, out.Kept
	return res, err
}

// ---------- Manifest and report ----------

type Release struct {
	Version     string  `json:"version"`
	PublishedAt *string `json:"publishedAt"`
	Urgent      bool    `json:"urgent"`
	Note        *string `json:"note"`
}

type Product struct {
	Latest   *string   `json:"latest"`
	Releases []Release `json:"releases"`
}

type Manifest struct {
	Products map[string]Product `json:"products"`
}

// GetManifest reads versions and urgent flags. No key is sent.
func GetManifest(ctx context.Context) (Manifest, error) {
	var m Manifest
	err := call(ctx, "", http.MethodGet, "/latchadmin/manifest", nil, &m)
	return m, err
}

// ReportServer is one game server in the panel report.
type ReportServer struct {
	Game        string         `json:"game"`
	GameVersion *string        `json:"gameVersion"`
	State       string         `json:"state"`
	Scripts     []ReportScript `json:"scripts"`
}

type ReportScript struct {
	ScriptID string `json:"scriptId"`
	Version  string `json:"version"`
}

// Report is everything LatchAdmin ever sends about itself. See docs/TELEMETRY.md.
type Report struct {
	InstanceID   string         `json:"instanceId"`
	Name         string         `json:"name"`
	PanelVersion string         `json:"panelVersion"`
	OS           string         `json:"os"`
	Servers      []ReportServer `json:"servers"`
}

// OSName is the value reported for this build.
func OSName() string {
	switch runtime.GOOS {
	case "windows", "linux":
		return runtime.GOOS
	}
	return "other"
}

// SendReport posts the report and returns the manifest Latch answers with.
func SendReport(ctx context.Context, key string, r Report) (Manifest, error) {
	var m Manifest
	err := call(ctx, key, http.MethodPost, "/me/panel", r, &m)
	return m, err
}
