package app

import (
	"context"
	"sync"
	"time"

	"github.com/heylatch/latchadmin/internal/buildinfo"
	"github.com/heylatch/latchadmin/internal/games"
	"github.com/heylatch/latchadmin/internal/latch"
	"github.com/heylatch/latchadmin/internal/release"
	"github.com/heylatch/latchadmin/internal/selfupdate"
	"github.com/heylatch/latchadmin/internal/store"
)

func compare(a, b string) int { return selfupdate.Compare(a, b) }

// ProductInfo is what the panel knows about the newest version of the panel or a game.
type ProductInfo struct {
	Latest   string          `json:"latest"`
	Urgent   *latch.Release  `json:"urgent,omitempty"` // newest urgent release newer than what we run, if any
	Releases []latch.Release `json:"releases,omitempty"`
}

type versionCache struct {
	mu        sync.Mutex
	products  map[string]latch.Product
	source    string // "latch" or "direct"
	checkedAt time.Time
	err       string
}

// setManifest stores a manifest from Latch (anonymous GET or the report answer).
func (a *App) setManifest(m latch.Manifest) {
	a.versions.mu.Lock()
	a.versions.products, a.versions.source, a.versions.checkedAt, a.versions.err = m.Products, "latch", time.Now(), ""
	a.versions.mu.Unlock()
}

// checkVersions refreshes the latest versions. With the Latch check off it asks MafiaHub and GitHub directly.
func (a *App) checkVersions(ctx context.Context) {
	var useLatch, linked bool
	a.Store.Read(func(st *store.State) {
		useLatch, linked = st.Settings.CheckLatch, st.Latch.Key != "" && !st.Latch.Disconnected
	})
	if useLatch && !linked {
		if m, err := latch.GetManifest(ctx); err == nil {
			a.setManifest(m)
		} else {
			a.versionError(err)
		}
	} else if !useLatch {
		products := map[string]latch.Product{}
		for _, g := range games.All {
			if v, err := release.Latest(ctx, g); err == nil {
				products[g.ID] = latch.Product{Latest: &v, Releases: []latch.Release{{Version: v}}}
			}
		}
		if v, err := selfupdate.LatestFromGitHub(ctx); err == nil && v != "" {
			products["latchadmin"] = latch.Product{Latest: &v, Releases: []latch.Release{{Version: v}}}
		}
		a.versions.mu.Lock()
		a.versions.products, a.versions.source, a.versions.checkedAt, a.versions.err = products, "direct", time.Now(), ""
		a.versions.mu.Unlock()
	}
	a.considerSelfUpdate(ctx)
}

func (a *App) versionError(err error) {
	a.versions.mu.Lock()
	a.versions.err = err.Error()
	a.versions.mu.Unlock()
}

// CheckNow refreshes versions immediately (the "Check for updates" button).
func (a *App) CheckNow(ctx context.Context) {
	linked := false
	a.Store.Read(func(st *store.State) { linked = st.Latch.Key != "" && !st.Latch.Disconnected })
	if linked {
		a.report(ctx)
	}
	a.checkVersions(ctx)
}

// Product returns version info for a product ("latchadmin" or a game id) relative to the version we run.
func (a *App) Product(id, current string) ProductInfo {
	a.versions.mu.Lock()
	p, ok := a.versions.products[id]
	a.versions.mu.Unlock()
	if !ok || p.Latest == nil {
		return ProductInfo{}
	}
	info := ProductInfo{Latest: *p.Latest, Releases: p.Releases}
	for _, r := range p.Releases {
		if r.Urgent && (current == "" || compare(r.Version, current) > 0) {
			rr := r
			info.Urgent = &rr
			break
		}
	}
	return info
}

func (a *App) latestGame(id string) string { return a.Product(id, "").Latest }

// VersionsMeta tells the UI where versions came from and when.
func (a *App) VersionsMeta() (string, time.Time, string) {
	a.versions.mu.Lock()
	defer a.versions.mu.Unlock()
	return a.versions.source, a.versions.checkedAt, a.versions.err
}

// ---------- Self-update ----------

// PanelUpdate describes the panel's own update state.
type PanelUpdate struct {
	Current string      `json:"current"`
	Info    ProductInfo `json:"info"`
	Pending string      `json:"pending,omitempty"` // downloaded and verified, waiting for a quiet moment
	Policy  string      `json:"policy"`
	Dev     bool        `json:"dev"`
}

func (a *App) PanelUpdate() PanelUpdate {
	var policy string
	a.Store.Read(func(st *store.State) { policy = st.Settings.SelfUpdate })
	return PanelUpdate{Current: buildinfo.Version, Info: a.Product("latchadmin", buildinfo.Version), Pending: a.pendingSelfUpdate(), Policy: policy, Dev: buildinfo.IsDev()}
}

func (a *App) pendingSelfUpdate() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.pendingSelf
}

// considerSelfUpdate downloads an update the policy allows and queues it.
func (a *App) considerSelfUpdate(ctx context.Context) {
	if buildinfo.IsDev() || a.pendingSelfUpdate() != "" {
		return
	}
	u := a.PanelUpdate()
	if u.Info.Latest == "" || compare(u.Info.Latest, buildinfo.Version) <= 0 {
		return
	}
	if u.Policy == "off" || (u.Policy == "urgent" && u.Info.Urgent == nil) {
		return
	}
	if err := a.PrepareSelfUpdate(ctx, u.Info.Latest); err != nil {
		a.Logf("panel update %s: %v", u.Info.Latest, err)
	}
}

// PrepareSelfUpdate downloads and verifies a release and queues it. It is applied when no server is running or at
// the next scheduled restart, or right away with ApplyNow.
func (a *App) PrepareSelfUpdate(ctx context.Context, version string) error {
	a.selfMu.Lock()
	defer a.selfMu.Unlock()
	if a.pendingSelfUpdate() != "" {
		return nil
	}
	path, hash, err := selfupdate.Download(ctx, version)
	if err != nil {
		return err
	}
	a.mu.Lock()
	a.pendingSelf, a.pendingHash = path, hash
	a.mu.Unlock()
	a.Store.Audit("system", "panel.updateReady", version, nil)
	return nil
}

// applySelfUpdate stops every server (they restart with the panel), swaps the binary and restarts.
func (a *App) applySelfUpdate(_ context.Context) {
	a.selfMu.Lock()
	defer a.selfMu.Unlock()
	a.mu.Lock()
	path, hash := a.pendingSelf, a.pendingHash
	a.pendingSelf = "" // taken: a second apply does nothing
	a.mu.Unlock()
	if path == "" {
		return
	}
	a.Logf("installing panel update")
	a.Shutdown()
	exe, err := selfupdate.Swap(path, hash)
	if err != nil {
		a.Logf("panel update failed: %v", err)
		a.mu.Lock()
		a.pendingSelf, a.shuttingDown = "", false
		a.mu.Unlock()
		a.Boot()
		return
	}
	a.Store.Audit("system", "panel.updated", "", nil)
	if a.Restart != nil {
		a.Restart(exe)
	}
}

// ApplyNow installs a prepared update immediately, restarting running servers.
func (a *App) ApplyNow(ctx context.Context) {
	go a.applySelfUpdate(ctx)
}

// Compare compares two x.y.z versions.
func Compare(a, b string) int { return compare(a, b) }
