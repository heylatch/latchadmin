package app

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/heylatch/latchadmin/internal/store"
)

// A server folder has to be a plain local path, and must not be (another spelling of) a folder the panel already
// uses. Folders are compared by file identity, not by text: on Windows "D:\srv.", "\\?\D:\srv", "\\localhost\D$\srv"
// and "D:\SRV~1" are all the same folder, and a symlink can point anywhere.

var shortName = regexp.MustCompile(`~\d`)

func cleanDir(dir string) (string, error) {
	dir = strings.TrimSpace(dir)
	if dir == "" || !filepath.IsAbs(dir) {
		return "", errors.New("enter a full folder path")
	}
	if runtime.GOOS == "windows" {
		if strings.HasPrefix(dir, `\\`) || strings.HasPrefix(dir, "//") {
			return "", errors.New(`use a folder on this machine, like D:\servers\main (network and \\?\ paths aren't accepted)`)
		}
		for _, part := range strings.FieldsFunc(dir, func(r rune) bool { return r == '\\' || r == '/' }) {
			if part == "." || part == ".." {
				continue
			}
			if part != strings.TrimRight(part, ". ") || shortName.MatchString(part) {
				return "", errors.New("write the folder's full name (no short names, no dots or spaces at the end)")
			}
		}
	}
	dir = filepath.Clean(dir)
	if real, err := filepath.EvalSymlinks(dir); err == nil {
		dir = real
	}
	return dir, nil
}

// lineage is the identity of dir and of every existing folder above it.
func lineage(dir string) []os.FileInfo {
	var out []os.FileInfo
	for d := dir; ; d = filepath.Dir(d) {
		if fi, err := os.Stat(d); err == nil {
			out = append(out, fi)
		}
		if filepath.Dir(d) == d {
			return out
		}
	}
}

func under(line []os.FileInfo, folder os.FileInfo) bool {
	for _, fi := range line {
		if os.SameFile(fi, folder) {
			return true
		}
	}
	return false
}

// overlap reports whether dir is other, is inside other, or (when contains is set) holds other inside it.
func overlap(dir string, line []os.FileInfo, other string, contains bool) bool {
	oi, err := os.Stat(other)
	if err != nil {
		return strings.EqualFold(filepath.Clean(other), dir)
	}
	if under(line, oi) {
		return true
	}
	if !contains {
		return false
	}
	di, err := os.Stat(dir)
	return err == nil && under(lineage(other), di)
}

// folderClash explains why dir can't hold a new server, or returns nil.
func (a *App) folderClash(dir string) error {
	var taken []string
	a.Store.Read(func(st *store.State) {
		for _, sv := range st.Servers {
			taken = append(taken, sv.Dir)
		}
	})
	line := lineage(dir)
	for _, other := range taken {
		if overlap(dir, line, other, true) {
			return errors.New("this folder is, holds, or is inside another server's folder")
		}
	}
	// The panel's data (keys, users) and the folder of its own binary are off limits.
	if overlap(dir, line, a.DataDir, true) {
		return errors.New("choose a folder outside LatchAdmin's data folder")
	}
	if exe, err := os.Executable(); err == nil {
		if real, err := filepath.EvalSymlinks(exe); err == nil {
			exe = real
		}
		if di, err := os.Stat(dir); err == nil && under(lineage(filepath.Dir(exe)), di) {
			return errors.New("choose a folder that doesn't hold LatchAdmin itself")
		}
	}
	return nil
}
