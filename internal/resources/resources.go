// Package resources manages a server's resources/ folder. MafiaHub loads every folder directly under resources/
// that has a package.json; there is no on/off list, so "disabled" means moved to resources-disabled/.
package resources

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/heylatch/latchadmin/internal/archive"
)

const (
	enabledDir  = "resources"
	disabledDir = "resources-disabled"
	removedDir  = "backups/removed-resources"
)

// Latch is what latch.json says about a Latch-protected resource.
type Latch struct {
	ScriptID string `json:"scriptId"`
	Version  string `json:"version"`
	Slug     string `json:"slug"`
	Name     string `json:"name"`
}

type Resource struct {
	Folder       string   `json:"folder"`
	Name         string   `json:"name"`
	Enabled      bool     `json:"enabled"`
	Version      string   `json:"version,omitempty"`
	Author       string   `json:"author,omitempty"`
	Dependencies []string `json:"dependencies,omitempty"`
	// Runs is false when package.json has no "mafiahub" block (the server loads nothing from it).
	Runs  bool   `json:"runs"`
	Latch *Latch `json:"latch,omitempty"`
}

// ValidFolder rejects names that could point outside resources/.
func ValidFolder(name string) bool {
	return name != "" && name != "." && name != ".." && !strings.HasPrefix(name, ".") && !strings.ContainsAny(name, `/\:*?"<>|`+"\x00") && len(name) <= 100
}

// List returns enabled and disabled resources, sorted by folder name.
func List(serverDir string) ([]Resource, error) {
	var out []Resource
	for _, d := range []struct {
		dir string
		on  bool
	}{{enabledDir, true}, {disabledDir, false}} {
		entries, err := os.ReadDir(filepath.Join(serverDir, d.dir))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			if !e.IsDir() || !ValidFolder(e.Name()) {
				continue
			}
			r, ok := read(filepath.Join(serverDir, d.dir, e.Name()))
			if !ok {
				continue
			}
			r.Folder, r.Enabled = e.Name(), d.on
			out = append(out, r)
		}
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Folder) < strings.ToLower(out[j].Folder) })
	return out, nil
}

func read(dir string) (Resource, bool) {
	raw, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		return Resource{}, false
	}
	var pkg struct {
		Name     string          `json:"name"`
		Version  string          `json:"version"`
		Author   json.RawMessage `json:"author"`
		Mafiahub *struct {
			Deps []struct {
				Name string `json:"name"`
			} `json:"resourceDependencies"`
		} `json:"mafiahub"`
	}
	_ = json.Unmarshal(raw, &pkg)
	r := Resource{Name: pkg.Name, Version: pkg.Version, Runs: pkg.Mafiahub != nil}
	var author string
	if json.Unmarshal(pkg.Author, &author) == nil {
		r.Author = author
	} else {
		var a struct{ Name string }
		if json.Unmarshal(pkg.Author, &a) == nil {
			r.Author = a.Name
		}
	}
	if pkg.Mafiahub != nil {
		for _, d := range pkg.Mafiahub.Deps {
			if d.Name != "" {
				r.Dependencies = append(r.Dependencies, d.Name)
			}
		}
	}
	if lr, err := os.ReadFile(filepath.Join(dir, "latch.json")); err == nil {
		var l Latch
		if json.Unmarshal(lr, &l) == nil && l.ScriptID != "" {
			r.Latch = &l
		}
	}
	return r, true
}

func noLink(p string) error {
	info, err := os.Lstat(p)
	if err != nil {
		return err
	}
	if archive.IsLink(info) {
		return fmt.Errorf("%s is a link; LatchAdmin won't move it", filepath.Base(p))
	}
	return nil
}

// SetEnabled moves a resource between resources/ and resources-disabled/.
func SetEnabled(serverDir, folder string, on bool) error {
	if !ValidFolder(folder) {
		return errors.New("bad resource name")
	}
	from, to := disabledDir, enabledDir
	if !on {
		from, to = enabledDir, disabledDir
	}
	src := filepath.Join(serverDir, from, folder)
	dst := filepath.Join(serverDir, to, folder)
	if err := noLink(src); err != nil {
		return err
	}
	if _, err := os.Stat(dst); err == nil {
		return fmt.Errorf("%s/%s already exists", to, folder)
	}
	if err := os.MkdirAll(filepath.Join(serverDir, to), 0o755); err != nil {
		return err
	}
	return os.Rename(src, dst)
}

// Remove moves a resource to backups/removed-resources (nothing is deleted). Returns the backup path.
func Remove(serverDir, folder string) (string, error) {
	if !ValidFolder(folder) {
		return "", errors.New("bad resource name")
	}
	var src string
	for _, d := range []string{enabledDir, disabledDir} {
		p := filepath.Join(serverDir, d, folder)
		if _, err := os.Lstat(p); err == nil {
			src = p
			break
		}
	}
	if src == "" {
		return "", os.ErrNotExist
	}
	if err := noLink(src); err != nil {
		return "", err
	}
	dst := filepath.Join(serverDir, filepath.FromSlash(removedDir), folder+"-"+time.Now().UTC().Format("20060102-150405"))
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return "", err
	}
	return dst, os.Rename(src, dst)
}

// Upload installs a zipped resource into resources/. The zip must hold one folder with a package.json, or the
// package.json at its top (then fallback names the folder). Existing folders are never overwritten.
func Upload(serverDir string, zr *zip.Reader, fallback string) (string, error) {
	root := archive.CommonRoot(zr.File)
	folder := strings.TrimSuffix(root, "/")
	if folder == "" {
		folder = strings.TrimSuffix(fallback, filepath.Ext(fallback))
	}
	if !ValidFolder(folder) {
		return "", errors.New("the zip's folder name isn't a valid resource name")
	}
	hasManifest := false
	for _, f := range zr.File {
		if f.Name == root+"package.json" {
			hasManifest = true
		}
	}
	if !hasManifest {
		return "", errors.New("the zip has no package.json at its top")
	}
	for _, d := range []string{enabledDir, disabledDir} {
		if _, err := os.Lstat(filepath.Join(serverDir, d, folder)); err == nil {
			return "", fmt.Errorf("a resource named %s already exists; remove it first", folder)
		}
	}
	dst := filepath.Join(serverDir, enabledDir, folder)
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return "", err
	}
	if _, err := archive.Extract(zr, dst, archive.Options{Prefix: root, MaxBytes: 1 << 30}); err != nil {
		_ = os.RemoveAll(dst)
		return "", err
	}
	return folder, nil
}
