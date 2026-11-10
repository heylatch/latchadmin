// Package archive extracts zip files without letting an entry escape its target folder.
package archive

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// ErrUnsafe is returned for an entry with an absolute path, "..", a drive letter or a link.
var ErrUnsafe = errors.New("unsafe path in archive")

// CleanName validates a zip entry name and returns it with forward slashes.
func CleanName(name string) (string, error) {
	if name == "" || strings.Contains(name, "\\") || strings.HasPrefix(name, "/") || strings.ContainsRune(name, 0) {
		return "", fmt.Errorf("%w: %q", ErrUnsafe, name)
	}
	if len(name) >= 2 && name[1] == ':' {
		return "", fmt.Errorf("%w: %q", ErrUnsafe, name)
	}
	for _, part := range strings.Split(strings.TrimSuffix(name, "/"), "/") {
		if part == ".." || part == "." || part == "" {
			return "", fmt.Errorf("%w: %q", ErrUnsafe, name)
		}
	}
	return name, nil
}

// CommonRoot returns "dir/" when every file in the archive is inside one top folder, else "".
func CommonRoot(files []*zip.File) string {
	root := ""
	for _, f := range files {
		if strings.HasSuffix(f.Name, "/") {
			continue
		}
		i := strings.IndexByte(f.Name, '/')
		if i < 0 {
			return ""
		}
		top := f.Name[:i+1]
		if root == "" {
			root = top
		} else if root != top {
			return ""
		}
	}
	return root
}

// Options controls Extract.
type Options struct {
	// Prefix: only entries under this path are extracted, with the prefix removed ("" = all).
	Prefix string
	// Keep: existing files whose relative path matches are left as they are.
	Keep func(rel string) bool
	// MaxBytes caps the total uncompressed size (0 = 4 GB).
	MaxBytes int64
	// Progress is called with the bytes written so far.
	Progress func(written int64)
}

// Result lists what Extract did.
type Result struct {
	Files int
	Kept  []string
	Bytes int64
}

// Extract writes the archive into dest. dest must already exist and must not be a link.
func Extract(r *zip.Reader, dest string, opt Options) (Result, error) {
	var res Result
	limit := opt.MaxBytes
	if limit == 0 {
		limit = 4 << 30
	}
	realDest, err := filepath.EvalSymlinks(dest)
	if err != nil {
		return res, err
	}
	for _, f := range r.File {
		name, err := CleanName(f.Name)
		if err != nil {
			return res, err
		}
		if !strings.HasPrefix(name, opt.Prefix) || strings.HasSuffix(name, "/") {
			continue
		}
		rel := strings.TrimPrefix(name, opt.Prefix)
		if rel == "" {
			continue
		}
		mode := f.Mode()
		if mode&os.ModeSymlink != 0 || !(mode.IsRegular() || mode&os.ModeType == 0) {
			return res, fmt.Errorf("%w: %q is not a regular file", ErrUnsafe, f.Name)
		}
		out := filepath.Join(dest, filepath.FromSlash(rel))
		if info, err := os.Lstat(out); err == nil {
			if info.Mode()&os.ModeSymlink != 0 {
				return res, fmt.Errorf("%w: %s is a link", ErrUnsafe, out)
			}
			if opt.Keep != nil && opt.Keep(rel) {
				res.Kept = append(res.Kept, rel)
				continue
			}
		}
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			return res, err
		}
		// The parent folder, with links resolved, must still be inside dest.
		parent, err := filepath.EvalSymlinks(filepath.Dir(out))
		if err != nil {
			return res, err
		}
		if parent != realDest && !strings.HasPrefix(parent, realDest+string(filepath.Separator)) {
			return res, fmt.Errorf("%w: %s goes through a link", ErrUnsafe, out)
		}
		n, err := writeEntry(f, out, limit-res.Bytes)
		res.Bytes += n
		if err != nil {
			return res, err
		}
		res.Files++
		if opt.Progress != nil {
			opt.Progress(res.Bytes)
		}
	}
	return res, nil
}

func writeEntry(f *zip.File, out string, budget int64) (int64, error) {
	rc, err := f.Open()
	if err != nil {
		return 0, err
	}
	defer rc.Close()
	perm := os.FileMode(0o644)
	if f.Mode()&0o111 != 0 {
		perm = 0o755
	}
	tmp := out + ".latchadmin-tmp"
	w, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, perm)
	if err != nil {
		return 0, err
	}
	n, err := io.Copy(w, io.LimitReader(rc, budget+1))
	if cerr := w.Close(); err == nil {
		err = cerr
	}
	if err == nil && n > budget {
		err = errors.New("archive is larger than allowed")
	}
	if err != nil {
		os.Remove(tmp)
		return n, err
	}
	if err := os.Rename(tmp, out); err != nil {
		os.Remove(tmp)
		return n, err
	}
	return n, nil
}

// GlobMatch matches rel against simple globs: exact, "dir/**", "dir/*", or "*" wildcards.
func GlobMatch(globs []string, rel string) bool {
	for _, g := range globs {
		switch {
		case g == rel:
			return true
		case strings.HasSuffix(g, "/**"):
			if strings.HasPrefix(rel, strings.TrimSuffix(g, "**")) {
				return true
			}
		case strings.Contains(g, "*"):
			if ok, _ := path.Match(g, rel); ok {
				return true
			}
		}
	}
	return false
}
