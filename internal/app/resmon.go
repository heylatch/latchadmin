package app

import (
	"sort"
	"sync"
	"time"

	"github.com/heylatch/latchadmin/internal/supervisor"
)

// Resource performance: once a minute LatchAdmin runs the server's own "resmon" command quietly and keeps six hours
// of it in memory (nothing is written to disk). Servers whose game has no resmon are tried once a start and then
// left alone.

const resmonKeep = 6 * 60 // one sample a minute

type resmonPoint struct {
	T    int64
	Rows []supervisor.ResourceStat
}

type resmonStore struct {
	mu          sync.Mutex
	points      map[string][]resmonPoint
	unsupported map[string]time.Time // server id -> start time it was found unsupported in
}

func (r *resmonStore) add(id string, p resmonPoint) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.points == nil {
		r.points = map[string][]resmonPoint{}
	}
	pts := append(r.points[id], p)
	if len(pts) > resmonKeep {
		pts = pts[len(pts)-resmonKeep:]
	}
	r.points[id] = pts
}

// sampleResources runs resmon on a running server, unless its game doesn't have the command.
func (a *App) sampleResources(id string, st supervisor.Status) {
	a.resmon.mu.Lock()
	skip := !st.StartedAt.IsZero() && a.resmon.unsupported[id].Equal(st.StartedAt)
	a.resmon.mu.Unlock()
	if skip {
		return
	}
	rows, supported, err := a.Proc(id).ResourceMonitor()
	if err != nil {
		return
	}
	if !supported {
		a.resmon.mu.Lock()
		if a.resmon.unsupported == nil {
			a.resmon.unsupported = map[string]time.Time{}
		}
		a.resmon.unsupported[id] = st.StartedAt
		a.resmon.mu.Unlock()
		return
	}
	a.resmon.add(id, resmonPoint{T: time.Now().Unix(), Rows: rows})
}

// ResourcePerf is what the Performance tab shows: the latest table and, for the heaviest resources, script time
// over the chosen window.
type ResourcePerf struct {
	Supported bool                      `json:"supported"`
	At        int64                     `json:"at"`
	Latest    []supervisor.ResourceStat `json:"latest"`
	Top       []string                  `json:"top"`    // resources in the chart, heaviest first
	Points    []map[string]any          `json:"points"` // {t, <resource>: ms/tick}
}

// ResourcePerformance returns resource stats since `from`, with the `top` heaviest resources charted.
func (a *App) ResourcePerformance(id string, from time.Time, top int) ResourcePerf {
	a.resmon.mu.Lock()
	pts := append([]resmonPoint(nil), a.resmon.points[id]...)
	_, unsupported := a.resmon.unsupported[id]
	a.resmon.mu.Unlock()
	out := ResourcePerf{Supported: !unsupported || len(pts) > 0, Latest: []supervisor.ResourceStat{}, Top: []string{}, Points: []map[string]any{}}
	if len(pts) == 0 {
		return out
	}
	last := pts[len(pts)-1]
	out.At, out.Latest = last.T, append(out.Latest, last.Rows...)
	sort.Slice(out.Latest, func(i, j int) bool { return out.Latest[i].MsPerTick > out.Latest[j].MsPerTick })

	// Heaviest by average script time over the window.
	total := map[string]float64{}
	var window []resmonPoint
	for _, p := range pts {
		if p.T >= from.Unix() {
			window = append(window, p)
			for _, r := range p.Rows {
				total[r.Name] += r.MsPerTick
			}
		}
	}
	for name := range total {
		out.Top = append(out.Top, name)
	}
	sort.Slice(out.Top, func(i, j int) bool { return total[out.Top[i]] > total[out.Top[j]] })
	if len(out.Top) > top {
		out.Top = out.Top[:top]
	}
	for _, p := range window {
		row := map[string]any{"t": p.T}
		for _, r := range p.Rows {
			for _, name := range out.Top {
				if r.Name == name {
					row[name] = r.MsPerTick
				}
			}
		}
		out.Points = append(out.Points, row)
	}
	return out
}
