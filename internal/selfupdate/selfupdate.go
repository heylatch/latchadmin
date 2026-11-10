// Package selfupdate updates the LatchAdmin binary from GitHub releases.
//
// Every release carries SHA256SUMS and SHA256SUMS.sig, an Ed25519 signature made in GitHub Actions with a key that
// exists nowhere else. The public half is below. An update is installed only if the signature checks out and the
// binary's hash is listed. Latch's manifest can say an update exists and whether it's urgent; it can't supply
// the file: binaries come from github.com and must carry this signature.
package selfupdate

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/heylatch/latchadmin/internal/buildinfo"
	"github.com/heylatch/latchadmin/internal/netx"
)

// PublicKey verifies release signatures.
const PublicKey = "L+to6Jag4F1Wm7EHh8Wqh8GG5JttiOZe59ZPMI9Cbd8="

// AssetName is the release file for this OS and CPU.
func AssetName() string {
	name := "latchadmin-" + runtime.GOOS + "-" + runtime.GOARCH
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return name
}

// Compare compares two x.y.z versions (pre-release suffixes ignored).
func Compare(a, b string) int {
	pa, pb := parts(a), parts(b)
	for i := 0; i < 3; i++ {
		if pa[i] != pb[i] {
			if pa[i] < pb[i] {
				return -1
			}
			return 1
		}
	}
	return 0
}

func parts(v string) [3]int {
	var out [3]int
	v = strings.TrimPrefix(v, "v")
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	for i, p := range strings.SplitN(v, ".", 3) {
		out[i], _ = strconv.Atoi(p)
	}
	return out
}

// LatestFromGitHub reads the latest release tag from the GitHub API.
func LatestFromGitHub(ctx context.Context) (string, error) {
	var r struct {
		Tag string `json:"tag_name"`
	}
	if err := netx.JSON(ctx, http.MethodGet, "https://api.github.com/repos/"+buildinfo.Repo+"/releases/latest", nil, nil, &r); err != nil {
		return "", err
	}
	return strings.TrimPrefix(r.Tag, "v"), nil
}

func fetch(ctx context.Context, url string, limit int64) ([]byte, error) {
	res, err := netx.Get(ctx, url, nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: HTTP %d", url, res.StatusCode)
	}
	return io.ReadAll(io.LimitReader(res.Body, limit))
}

// VerifySums checks the signature over SHA256SUMS and returns the hash listed for asset.
func VerifySums(sums, sig []byte, asset string) ([]byte, error) {
	pub, err := base64.StdEncoding.DecodeString(PublicKey)
	if err != nil || len(pub) != ed25519.PublicKeySize {
		return nil, errors.New("bad built-in public key")
	}
	rawSig, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(sig)))
	if err != nil {
		return nil, errors.New("signature is not base64")
	}
	if !ed25519.Verify(pub, sums, rawSig) {
		return nil, errors.New("release signature is invalid; the update was refused")
	}
	sc := bufio.NewScanner(bytes.NewReader(sums))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) == 2 && strings.TrimPrefix(f[1], "*") == asset {
			return hex.DecodeString(f[0])
		}
	}
	return nil, fmt.Errorf("%s is not in this release", asset)
}

// Download fetches and verifies version for this platform. It returns the path of the new binary, next to the
// running one.
func Download(ctx context.Context, version string) (string, error) {
	if Compare(version, buildinfo.Version) <= 0 {
		return "", errors.New("that version isn't newer than this one")
	}
	base := "https://github.com/" + buildinfo.Repo + "/releases/download/v" + version + "/"
	sums, err := fetch(ctx, base+"SHA256SUMS", 64<<10)
	if err != nil {
		return "", err
	}
	sig, err := fetch(ctx, base+"SHA256SUMS.sig", 4<<10)
	if err != nil {
		return "", err
	}
	want, err := VerifySums(sums, sig, AssetName())
	if err != nil {
		return "", err
	}
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if exe, err = filepath.EvalSymlinks(exe); err != nil {
		return "", err
	}
	res, err := netx.Get(ctx, base+AssetName(), nil)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download: HTTP %d", res.StatusCode)
	}
	next := exe + ".new"
	f, err := os.OpenFile(next, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	_, err = io.Copy(io.MultiWriter(f, h), io.LimitReader(res.Body, 200<<20))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil && !bytes.Equal(h.Sum(nil), want) {
		err = errors.New("downloaded file doesn't match the signed hash; the update was refused")
	}
	if err != nil {
		os.Remove(next)
		return "", err
	}
	return next, nil
}

// Swap puts the downloaded binary in place of the running one. The old one is kept as <exe>.old.
func Swap(next string) (string, error) {
	exe := strings.TrimSuffix(next, ".new")
	old := exe + ".old"
	_ = os.Remove(old)
	// Windows can rename a running executable but not overwrite it.
	if err := os.Rename(exe, old); err != nil {
		return "", err
	}
	if err := os.Rename(next, exe); err != nil {
		_ = os.Rename(old, exe)
		return "", err
	}
	return exe, nil
}

// CleanOld removes the previous binary left by a past update.
func CleanOld() {
	if exe, err := os.Executable(); err == nil {
		_ = os.Remove(exe + ".old")
		_ = os.Remove(exe + ".new")
	}
}
