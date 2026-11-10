package app

import (
	"context"
	"time"

	"github.com/heylatch/latchadmin/internal/metrics"
	"github.com/heylatch/latchadmin/internal/store"
	"github.com/heylatch/latchadmin/internal/supervisor"
)

// collect takes one sample of the machine and of every server. It's light on purpose: a few system calls, and
// a quiet "status" to each running server once a minute for the player count.
func (a *App) collect(_ context.Context) {
	now := time.Now()
	dir := a.DataDir
	var servers []store.Server
	a.Store.Read(func(st *store.State) { servers = append(servers, st.Servers...) })
	if len(servers) > 0 {
		dir = servers[0].Dir
	}
	hs, info := a.host.Sample(dir)
	a.Metrics.Add("host", hs)
	a.Metrics.SetHost(info)
	for _, sv := range servers {
		p := a.Proc(sv.ID)
		st := p.Status()
		smp := metrics.Sample{T: now.Unix(), Players: -1}
		if st.State == supervisor.Running || st.State == supervisor.Starting || st.State == supervisor.Stopping {
			if st.State == supervisor.Running && now.Unix()/int64(metrics.Interval/time.Second)%4 == 0 {
				p.RefreshPlayers()
			}
			st = p.Sample()
			smp.Up, smp.CPU, smp.Mem = true, float32(st.CPU), st.MemBytes
			if st.Players != nil {
				smp.Players = int16(*st.Players)
			}
		}
		a.Metrics.Add(sv.ID, smp)
	}
}
