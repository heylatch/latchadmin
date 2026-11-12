package supervisor

import (
	"errors"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Quiet queries: LatchAdmin sends "status" (player count) and "help" (command list) itself and reads the answer
// without it showing up in the console or the log. Lines are claimed by a matcher while the query is open; any
// other output passes through as usual.

type capture struct {
	match func(line string) (take, done bool)
	lines []string
	last  time.Time
	done  bool
}

// captured hands a line to an open query. It reports whether the query took it.
func (p *Proc) captured(line string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	c := p.capt
	if c == nil || c.done {
		return false
	}
	take, done := c.match(line)
	if take {
		c.lines = append(c.lines, line)
		c.last = time.Now()
	}
	if done {
		c.done = true
	}
	return take
}

// Query sends a command and collects the lines match takes. It ends when match says done, when no line came for
// quiet after the first one, or after timeout.
func (p *Proc) Query(command string, match func(string) (bool, bool), quiet, timeout time.Duration) ([]string, error) {
	p.cmu.Lock()
	defer p.cmu.Unlock()
	p.mu.Lock()
	stdin := p.stdin
	if stdin == nil || p.st.State != Running {
		p.mu.Unlock()
		return nil, errors.New("server is not running")
	}
	c := &capture{match: match}
	p.capt = c
	p.mu.Unlock()
	defer func() {
		p.mu.Lock()
		p.capt = nil
		p.mu.Unlock()
	}()
	if _, err := io.WriteString(stdin, command+"\n"); err != nil {
		return nil, err
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
		p.mu.Lock()
		finished := c.done || (len(c.lines) > 0 && time.Since(c.last) > quiet)
		p.mu.Unlock()
		if finished {
			break
		}
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), c.lines...), nil
}

var playersRe = regexp.MustCompile(`Players:\s*(\d+)\s*/\s*(\d+)`)

// RefreshPlayers reads the player count with "status".
func (p *Proc) RefreshPlayers() {
	lines, err := p.Query("status", func(l string) (bool, bool) {
		switch {
		case strings.Contains(l, "] Server status:"), strings.Contains(l, "]   Name:"), strings.Contains(l, "]   Host:"):
			return true, false
		case playersRe.MatchString(l):
			return true, true
		}
		return false, false
	}, 500*time.Millisecond, 3*time.Second)
	if err != nil {
		return
	}
	for _, l := range lines {
		if m := playersRe.FindStringSubmatch(l); m != nil {
			n, _ := strconv.Atoi(m[1])
			max, _ := strconv.Atoi(m[2])
			p.mu.Lock()
			p.st.Players, p.st.MaxPlayers = &n, max
			p.mu.Unlock()
		}
	}
}

var timestamped = regexp.MustCompile(`^\[\d{2}:\d{2}:\d{2}\]`)
var commandLine = regexp.MustCompile(`^([a-zA-Z][\w:.-]*) (.+)$`)

// RefreshCommands reads the server's command list with "help". Help lines carry no "[hh:mm:ss]" prefix, which is
// how they're told apart from log output arriving at the same time.
func (p *Proc) RefreshCommands() {
	lines, err := p.Query("help", func(l string) (bool, bool) {
		return !timestamped.MatchString(l) || strings.HasSuffix(strings.TrimSpace(l), "Available commands:"), false
	}, 600*time.Millisecond, 4*time.Second)
	if err != nil {
		return
	}
	var out []Command
	seen := map[string]bool{}
	for i, l := range lines {
		m := commandLine.FindStringSubmatch(strings.TrimSpace(l))
		if m == nil || i+1 >= len(lines) || strings.TrimSpace(lines[i+1]) != "Usage:" || seen[m[1]] {
			continue
		}
		seen[m[1]] = true
		out = append(out, Command{Name: m[1], Description: strings.TrimSpace(m[2])})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	if len(out) > 0 {
		p.mu.Lock()
		p.commands = out
		p.mu.Unlock()
	}
}

// Commands returns the command list read from "help" (empty until the server has run once).
func (p *Proc) Commands() []Command {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]Command(nil), p.commands...)
}

// Sample updates and returns CPU and memory use. Call it from one place only (the metrics collector).
func (p *Proc) Sample() Status {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.st.MemBytes, p.st.CPU = p.sample()
	return p.st
}

// ResourceStat is one row of the server's "resmon" table: script time and memory of a resource over the last minute.
type ResourceStat struct {
	Name      string  `json:"name"`
	MsPerTick float64 `json:"msPerTick"`
	MaxMs     float64 `json:"maxMs"`
	HeapMB    float64 `json:"heapMb"`
	LimitMB   float64 `json:"limitMb"`
	GCMs      float64 `json:"gcMs"` // ms of garbage collection per second
	Slow      int     `json:"slow"` // ticks over the server's slow threshold
}

var resmonRow = regexp.MustCompile(`^([\w.:@/-]+)\s+([\d.]+)\s+([\d.]+)\s+([\d.]+)\s+([\d.]+)\s+([\d.]+)\s+(\d+)$`)

// ResourceMonitor runs "resmon" quietly. supported is false when the server printed no resource table (a game
// version without the command).
func (p *Proc) ResourceMonitor() (rows []ResourceStat, supported bool, err error) {
	started := false
	lines, err := p.Query("resmon", func(l string) (bool, bool) {
		t := strings.TrimSpace(l)
		switch {
		case strings.Contains(l, "Resource monitor"):
			started = true
			return true, false
		case !started:
			return false, false
		case strings.HasPrefix(t, "resource ") && strings.Contains(t, "ms/tick"):
			return true, false
		case resmonRow.MatchString(t):
			return true, false
		case t == "":
			return true, true
		}
		return false, false
	}, 400*time.Millisecond, 3*time.Second)
	if err != nil {
		return nil, false, err
	}
	for _, l := range lines {
		if strings.Contains(l, "Resource monitor") {
			supported = true
		}
		m := resmonRow.FindStringSubmatch(strings.TrimSpace(l))
		if m == nil {
			continue
		}
		num := func(i int) float64 { f, _ := strconv.ParseFloat(m[i], 64); return f }
		slow, _ := strconv.Atoi(m[7])
		rows = append(rows, ResourceStat{Name: m[1], MsPerTick: num(2), MaxMs: num(3), HeapMB: num(4), LimitMB: num(5), GCMs: num(6), Slow: slow})
	}
	return rows, supported, nil
}
