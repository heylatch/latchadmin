// Package foundations installs and updates HMP Foundations (github.com/arcane-mp/foundations), the open-source
// gameplay framework for Arcane MP servers: thirty hmp-* resources released together, plus example config files.
//
// Releases come from GitHub through the panel's allow-list. An update replaces every hmp-* resource folder as a
// unit (they must share one version); the old folders are kept under backups/, and files in data/ are never
// overwritten.
package foundations

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/heylatch/latchadmin/internal/archive"
	"github.com/heylatch/latchadmin/internal/netx"
)

const Repo = "arcane-mp/foundations"

// Game is the only game Foundations is for.
const Game = "arcane-mp"

const root = "hmp-foundations/"

var versionRe = regexp.MustCompile(`^\d{1,4}\.\d{1,4}\.\d{1,4}$`)

// configName is the only shape of example config file that is copied into data/ (no separators, no "..").
var configName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,80}\.json$`)

// Release is the latest published version.
type Release struct {
	Version  string `json:"version"`
	URL      string `json:"url"` // release page
	assetURL string // zip download
}

// Latest asks GitHub for the newest release.
func Latest(ctx context.Context) (Release, error) {
	var gh struct {
		TagName string `json:"tag_name"`
		HTMLURL string `json:"html_url"`
		Assets  []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
		} `json:"assets"`
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	if err := netx.JSON(ctx, http.MethodGet, "https://api.github.com/repos/"+Repo+"/releases/latest", http.Header{"Accept": {"application/vnd.github+json"}}, nil, &gh); err != nil {
		return Release{}, fmt.Errorf("couldn't check Foundations releases: %w", err)
	}
	v := strings.TrimPrefix(gh.TagName, "v")
	if !versionRe.MatchString(v) {
		return Release{}, fmt.Errorf("unexpected Foundations release tag %q", gh.TagName)
	}
	r := Release{Version: v, URL: "https://github.com/" + Repo + "/releases/tag/v" + v}
	for _, a := range gh.Assets {
		if a.Name == "hmp-foundations-v"+v+".zip" {
			r.assetURL = a.URL
		}
	}
	if r.assetURL == "" {
		return Release{}, fmt.Errorf("Foundations %s has no hmp-foundations-v%s.zip", v, v)
	}
	return r, nil
}

// Installed returns the Foundations version in a server folder ("" when it isn't installed).
func Installed(serverDir string) string {
	raw, err := os.ReadFile(filepath.Join(serverDir, "resources", "hmp-core", "package.json"))
	if err != nil {
		return ""
	}
	var p struct {
		Version string `json:"version"`
	}
	if json.Unmarshal(raw, &p) != nil || !versionRe.MatchString(p.Version) {
		return ""
	}
	return p.Version
}

// overlap lists resources that must not run next to Foundations (from its INSTALL.md).
var overlap = []string{"hmp-discord", "charselect", "interactables", "breakables", "roles", "house", "housepoints",
	"audio", "blips", "doors", "emotes", "spells", "progression", "spellupgrades", "duels", "pvp", "blips-dev"}

// Conflicts lists enabled resources that clash with Foundations.
func Conflicts(serverDir string) []string {
	var out []string
	entries, _ := os.ReadDir(filepath.Join(serverDir, "resources"))
	for _, e := range entries {
		name := e.Name()
		if !e.IsDir() {
			continue
		}
		for _, o := range overlap {
			if strings.EqualFold(name, o) {
				out = append(out, name)
			}
		}
		if strings.HasPrefix(strings.ToLower(name), "rp-") {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

// Result says what an install changed.
type Result struct {
	Version   string   `json:"version"`
	Previous  string   `json:"previous,omitempty"`
	Resources int      `json:"resources"`
	Configs   []string `json:"configs"` // example config files copied into data/ (existing ones are left alone)
	Backup    string   `json:"backup,omitempty"`
}

// Install downloads rel and puts its resources into serverDir. The server must be stopped.
func Install(ctx context.Context, serverDir string, rel Release, progress func(stage string)) (Result, error) {
	res := Result{Version: rel.Version, Previous: Installed(serverDir)}
	progress("download")
	resp, err := netx.Get(ctx, rel.assetURL, nil)
	if err != nil {
		return res, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return res, fmt.Errorf("Foundations download: HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 128<<20))
	if err != nil {
		return res, err
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return res, fmt.Errorf("Foundations package: %w", err)
	}
	if err := checkPackage(zr, rel.Version); err != nil {
		return res, err
	}

	progress("install")
	resDir := filepath.Join(serverDir, "resources")
	staging := filepath.Join(serverDir, ".foundations-staging")
	_ = os.RemoveAll(staging)
	if err := os.MkdirAll(staging, 0o755); err != nil {
		return res, err
	}
	defer os.RemoveAll(staging)
	if _, err := archive.Extract(zr, staging, archive.Options{Prefix: root + "resources/", MaxBytes: 1 << 30}); err != nil {
		return res, err
	}
	news, err := os.ReadDir(staging)
	if err != nil {
		return res, err
	}
	if err := os.MkdirAll(resDir, 0o755); err != nil {
		return res, err
	}
	// Old folders move to backups/ first, so a failed swap can be put back by hand and nothing is deleted.
	stamp := time.Now().Format("20060102-150405")
	backup := filepath.Join(serverDir, "backups", "foundations-"+orNone(res.Previous)+"-"+stamp)
	for _, e := range news {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), "hmp-") {
			continue
		}
		target := filepath.Join(resDir, e.Name())
		if info, err := os.Lstat(target); err == nil {
			if archive.IsLink(info) {
				return res, fmt.Errorf("resources/%s is a link; LatchAdmin won't replace it", e.Name())
			}
			if err := os.MkdirAll(backup, 0o755); err != nil {
				return res, err
			}
			if err := os.Rename(target, filepath.Join(backup, e.Name())); err != nil {
				return res, err
			}
			res.Backup = filepath.Base(backup)
		}
		if err := os.Rename(filepath.Join(staging, e.Name()), target); err != nil {
			return res, err
		}
		res.Resources++
	}

	// Example configs only fill gaps: a server's own data/*.json is never replaced.
	dataDir := filepath.Join(serverDir, "data")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return res, err
	}
	for _, f := range zr.File {
		name, ok := strings.CutPrefix(f.Name, root+"examples/config/data/")
		if !ok || !configName.MatchString(name) || strings.Contains(name, "..") {
			continue
		}
		out := filepath.Join(dataDir, name)
		if filepath.Dir(out) != filepath.Clean(dataDir) {
			continue
		}
		if _, err := os.Lstat(out); err == nil {
			continue
		}
		if err := copyEntry(f, out); err != nil {
			return res, err
		}
		res.Configs = append(res.Configs, name)
	}
	return res, nil
}

// checkPackage makes sure the zip is Foundations at the expected version before anything is written.
func checkPackage(zr *zip.Reader, version string) error {
	resources := map[string]bool{}
	var meta []byte
	for _, f := range zr.File {
		if f.Name == root+"foundations.json" {
			rc, err := f.Open()
			if err != nil {
				return err
			}
			meta, _ = io.ReadAll(io.LimitReader(rc, 1<<20))
			rc.Close()
		}
		if rest, ok := strings.CutPrefix(f.Name, root+"resources/"); ok {
			if name, _, ok := strings.Cut(rest, "/"); ok && strings.HasPrefix(name, "hmp-") {
				resources[name] = true
			}
		}
	}
	if meta == nil || !resources["hmp-core"] {
		return errors.New("this doesn't look like a Foundations release (no foundations.json or hmp-core)")
	}
	if !bytes.Contains(meta, []byte(`"`+version+`"`)) {
		return fmt.Errorf("the package isn't Foundations %s", version)
	}
	return nil
}

func copyEntry(f *zip.File, out string) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	w, err := os.OpenFile(out, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	_, err = io.Copy(w, io.LimitReader(rc, 8<<20))
	if cerr := w.Close(); err == nil {
		err = cerr
	}
	return err
}

func orNone(v string) string {
	if v == "" {
		return "none"
	}
	return v
}
