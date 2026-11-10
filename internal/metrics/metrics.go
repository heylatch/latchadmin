// Package metrics keeps performance history for the panel's statistics pages: one sample every 15 seconds for
// the last 24 hours, one-minute summaries for the last 7 days, and server events (start, ready, stop, crash) for
// 30 days. Everything stays on this machine (metrics.json.gz in the data folder).
package metrics

import (
	"compress/gzip"
	"encoding/json"
	"math"
	"os"
	"sync"
	"time"
)

const (
	Interval   = 15 * time.Second
	rawKeep    = 24 * time.Hour
	minuteKeep = 7 * 24 * time.Hour
	eventKeep  = 30 * 24 * time.Hour
	maxEvents  = 2000
)

// Sample is one measurement. Players is -1 when unknown. Mem is bytes in use.
type Sample struct {
	T       int64   `json:"t"`
	Up      bool    `json:"u"`
	CPU     float32 `json:"c"`
	Mem     uint64  `json:"m"`
	Players int16   `json:"p"`
}

type series struct {
	Raw    []Sample `json:"raw"`
	Minute []Sample `json:"min"`
	acc    []Sample // samples of the current minute, not yet summarised
}

// Event is something that happened to a server.
type Event struct {
	T      int64  `json:"t"`
	Kind   string `json:"kind"` // start, ready, stop, crash, update
	Detail string `json:"detail,omitempty"`
}

type Store struct {
	mu     sync.Mutex
	path   string
	Series map[string]*series `json:"series"`
	Events map[string][]Event `json:"events"`
	Host   HostInfo           `json:"host"`
}

// HostInfo holds the latest totals for the machine.
type HostInfo struct {
	MemTotal  uint64 `json:"memTotal"`
	CPUs      int    `json:"cpus"`
	DiskFree  uint64 `json:"diskFree"`
	DiskTotal uint64 `json:"diskTotal"`
}

// Open loads saved history (a missing or damaged file starts empty).
func Open(path string) *Store {
	s := &Store{path: path, Series: map[string]*series{}, Events: map[string][]Event{}}
	f, err := os.Open(path)
	if err != nil {
		return s
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		return s
	}
	var loaded Store
	if json.NewDecoder(zr).Decode(&loaded) == nil {
		if loaded.Series != nil {
			s.Series = loaded.Series
		}
		if loaded.Events != nil {
			s.Events = loaded.Events
		}
		s.Host = loaded.Host
	}
	return s
}

// Save writes the history to disk.
func (s *Store) Save() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	tmp := s.path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	zw := gzip.NewWriter(f)
	err = json.NewEncoder(zw).Encode(s)
	if cerr := zw.Close(); err == nil {
		err = cerr
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, s.path)
}

// Add records a sample for key (a server id or "host").
func (s *Store) Add(key string, smp Sample) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sr := s.Series[key]
	if sr == nil {
		sr = &series{}
		s.Series[key] = sr
	}
	sr.Raw = append(sr.Raw, smp)
	cut := smp.T - int64(rawKeep/time.Second)
	i := 0
	for i < len(sr.Raw) && sr.Raw[i].T < cut {
		i++
	}
	sr.Raw = sr.Raw[i:]
	// One-minute summary once the minute is over.
	if len(sr.acc) > 0 && sr.acc[0].T/60 != smp.T/60 {
		sr.Minute = append(sr.Minute, summarise(sr.acc, sr.acc[0].T/60*60))
		sr.acc = sr.acc[:0]
		cut := smp.T - int64(minuteKeep/time.Second)
		j := 0
		for j < len(sr.Minute) && sr.Minute[j].T < cut {
			j++
		}
		sr.Minute = sr.Minute[j:]
	}
	sr.acc = append(sr.acc, smp)
}

// summarise: CPU is the average while up; memory and players are the peak (a short spike stays visible).
func summarise(list []Sample, t int64) Sample {
	out := Sample{T: t, Players: -1}
	var cpu float64
	up := 0
	for _, x := range list {
		if !x.Up {
			continue
		}
		up++
		out.Up = true
		cpu += float64(x.CPU)
		out.Mem = max(out.Mem, x.Mem)
		if x.Players > out.Players {
			out.Players = x.Players
		}
	}
	if up > 0 {
		out.CPU = float32(cpu / float64(up))
	}
	return out
}

// Forget drops a removed server's history.
func (s *Store) Forget(key string) {
	s.mu.Lock()
	delete(s.Series, key)
	delete(s.Events, key)
	s.mu.Unlock()
}

// SetHost stores the machine totals.
func (s *Store) SetHost(h HostInfo) {
	s.mu.Lock()
	s.Host = h
	s.mu.Unlock()
}

func (s *Store) HostInfo() HostInfo {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.Host
}

// AddEvent records a server event.
func (s *Store) AddEvent(key, kind, detail string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().Unix()
	list := append(s.Events[key], Event{T: now, Kind: kind, Detail: detail})
	cut := now - int64(eventKeep/time.Second)
	i := 0
	for i < len(list) && list[i].T < cut {
		i++
	}
	list = list[i:]
	if len(list) > maxEvents {
		list = list[len(list)-maxEvents:]
	}
	s.Events[key] = list
}

// Point is one chart point. Nil fields are gaps (server down or value unknown).
type Point struct {
	T       int64    `json:"t"`
	CPU     *float64 `json:"cpu"`
	Mem     *uint64  `json:"mem"`
	Players *int     `json:"players"`
}

// Range returns up to maxPoints points for key since from (unix seconds).
func (s *Store) Range(key string, from int64, maxPoints int) []Point {
	s.mu.Lock()
	defer s.mu.Unlock()
	sr := s.Series[key]
	if sr == nil {
		return []Point{}
	}
	src := sr.Raw
	step := int64(Interval / time.Second)
	if time.Now().Unix()-from > int64(rawKeep/time.Second) {
		src = append(append([]Sample{}, sr.Minute...), summarise(sr.acc, time.Now().Unix()/60*60))
		step = 60
	}
	now := time.Now().Unix()
	span := now - from
	bucket := max(step, int64(math.Ceil(float64(span)/float64(maxPoints))))
	out := make([]Point, 0, maxPoints)
	var cur []Sample
	curStart := from
	flush := func() {
		if len(cur) == 0 {
			return
		}
		sm := summarise(cur, curStart)
		p := Point{T: curStart}
		if sm.Up {
			c := math.Round(float64(sm.CPU)*10) / 10
			m := sm.Mem
			p.CPU, p.Mem = &c, &m
			if sm.Players >= 0 {
				n := int(sm.Players)
				p.Players = &n
			}
		}
		out = append(out, p)
		cur = cur[:0]
	}
	for _, x := range src {
		if x.T < from {
			continue
		}
		b := from + (x.T-from)/bucket*bucket
		if b != curStart {
			flush()
			curStart = b
		}
		cur = append(cur, x)
	}
	flush()
	return out
}

// Summary describes a server (or the host) over a range.
type Summary struct {
	Uptime      *float64 `json:"uptime"` // share of samples the server was up, 0..1; nil without data
	AvgCPU      *float64 `json:"avgCpu"`
	PeakCPU     *float64 `json:"peakCpu"`
	AvgMem      *uint64  `json:"avgMem"`
	PeakMem     *uint64  `json:"peakMem"`
	AvgPlayers  *float64 `json:"avgPlayers"`
	PeakPlayers *int     `json:"peakPlayers"`
	Starts      int      `json:"starts"`
	Crashes     int      `json:"crashes"`
	Covered     int64    `json:"covered"` // seconds of history in the range
}

func (s *Store) Summary(key string, from int64) Summary {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out Summary
	for _, e := range s.Events[key] {
		if e.T >= from {
			switch e.Kind {
			case "start":
				out.Starts++
			case "crash":
				out.Crashes++
			}
		}
	}
	sr := s.Series[key]
	if sr == nil {
		return out
	}
	src := sr.Raw
	step := int64(Interval / time.Second)
	if time.Now().Unix()-from > int64(rawKeep/time.Second) {
		src = append(append([]Sample{}, sr.Minute...), sr.acc...)
		step = 60
	}
	var n, up, pn int
	var cpu, mem, players, peakCPU float64
	var peakMem uint64
	peakPlayers := -1
	for _, x := range src {
		if x.T < from {
			continue
		}
		n++
		if !x.Up {
			continue
		}
		up++
		cpu += float64(x.CPU)
		peakCPU = math.Max(peakCPU, float64(x.CPU))
		mem += float64(x.Mem)
		peakMem = max(peakMem, x.Mem)
		if x.Players >= 0 {
			pn++
			players += float64(x.Players)
			peakPlayers = max(peakPlayers, int(x.Players))
		}
	}
	out.Covered = int64(n) * step
	if n > 0 {
		u := float64(up) / float64(n)
		out.Uptime = &u
	}
	if up > 0 {
		a, pc := math.Round(cpu/float64(up)*10)/10, math.Round(peakCPU*10)/10
		am := uint64(mem / float64(up))
		out.AvgCPU, out.PeakCPU, out.AvgMem, out.PeakMem = &a, &pc, &am, &peakMem
	}
	if pn > 0 {
		ap := math.Round(players/float64(pn)*10) / 10
		out.AvgPlayers, out.PeakPlayers = &ap, &peakPlayers
	}
	return out
}

// EventsSince returns a server's events since from, newest first.
func (s *Store) EventsSince(key string, from int64) []Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []Event{}
	list := s.Events[key]
	for i := len(list) - 1; i >= 0; i-- {
		if list[i].T >= from {
			out = append(out, list[i])
		}
	}
	return out
}
