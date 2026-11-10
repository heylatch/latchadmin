// Package supervisor runs game server processes: start, graceful stop ("quit" on stdin, then kill after 30 s),
// console input, live and daily log files, crash restarts with backoff and a health check on the game port.
package supervisor

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

type State string

const (
	Stopped  State = "stopped"
	Starting State = "starting"
	Running  State = "running"
	Stopping State = "stopping"
	Crashed  State = "crashed"
	Updating State = "updating" // set by the app while a game update runs
)

// Spec is how to launch a server. It is resolved again for every (re)start, so edited env takes effect.
type Spec struct {
	Exe         string
	Dir         string
	Env         []string
	Port        int
	AutoRestart bool
}

type Line struct {
	N    int64     `json:"n"`
	At   time.Time `json:"at"`
	Text string    `json:"text"`
	Src  string    `json:"src"` // "out", "err", "panel" (lines written by LatchAdmin itself)
}

type Health struct {
	At         time.Time `json:"at"`
	OK         bool      `json:"ok"`
	ModName    string    `json:"modName,omitempty"`
	ModVersion string    `json:"modVersion,omitempty"`
	MaxPlayers int       `json:"maxPlayers,omitempty"`
	Password   bool      `json:"passwordRequired,omitempty"`
}

type Status struct {
	State     State     `json:"state"`
	PID       int       `json:"pid,omitempty"`
	StartedAt time.Time `json:"startedAt,omitempty"`
	ReadyAt   time.Time `json:"readyAt,omitempty"`
	Crashes   int       `json:"crashes"`
	LastExit  string    `json:"lastExit,omitempty"`
	NextStart time.Time `json:"nextStart,omitempty"` // crash restart scheduled
	Health    *Health   `json:"health,omitempty"`
	MemBytes  uint64    `json:"memBytes,omitempty"`
	CPU       float64   `json:"cpu,omitempty"` // percent of one core, averaged since the last sample
	// Players and MaxPlayers come from the console "status" command; nil until read.
	Players    *int `json:"players,omitempty"`
	MaxPlayers int  `json:"maxPlayers,omitempty"`
}

// Command is a console command the server listed in "help".
type Command struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// Proc is one game server.
type Proc struct {
	ID      string
	logDir  string
	resolve func() (Spec, error)
	// OnState is called (outside locks) after every state change.
	OnState func(State)

	mu         sync.Mutex
	cmd        *exec.Cmd
	stdin      io.WriteCloser
	killer     killer
	st         Status
	stopping   bool
	done       chan struct{} // closed when the process has exited
	fin        chan struct{} // closed after exit handling, when the state is final
	restarting bool
	restartT   *time.Timer
	backoff    time.Duration
	cpuPrev    cpuSample

	cmu      sync.Mutex // one quiet query at a time
	capt     *capture
	commands []Command

	lmu    sync.Mutex
	lines  []Line
	next   int64
	subs   map[chan Line]struct{}
	logF   *os.File
	logDay string
}

const keepLines = 3000

var readyRe = regexp.MustCompile(`(?i)server successfully started`)

// New creates a stopped server. resolve builds its launch spec.
func New(id, logDir string, resolve func() (Spec, error)) *Proc {
	return &Proc{ID: id, logDir: logDir, resolve: resolve, st: Status{State: Stopped}, subs: map[chan Line]struct{}{}}
}

func (p *Proc) Status() Status {
	p.mu.Lock()
	defer p.mu.Unlock()
	s := p.st
	if s.Health != nil {
		h := *s.Health
		s.Health = &h
	}
	return s
}

func (p *Proc) setState(s State) {
	p.mu.Lock()
	p.st.State = s
	cb := p.OnState
	p.mu.Unlock()
	if cb != nil {
		cb(s)
	}
}

// SetUpdating marks the server busy with a game update (it must be stopped).
func (p *Proc) SetUpdating(on bool) error {
	p.mu.Lock()
	if on {
		if p.st.State != Stopped && p.st.State != Crashed {
			p.mu.Unlock()
			return errors.New("stop the server first")
		}
		p.cancelRestart()
		p.st.State = Updating
	} else {
		// Only leave Updating: the server may already have been started again.
		if p.st.State != Updating {
			p.mu.Unlock()
			return nil
		}
		p.st.State = Stopped
	}
	state, cb := p.st.State, p.OnState
	p.mu.Unlock()
	if cb != nil {
		cb(state)
	}
	return nil
}

// Start launches the server.
func (p *Proc) Start() error {
	p.mu.Lock()
	if p.st.State != Stopped && p.st.State != Crashed {
		state := p.st.State
		p.mu.Unlock()
		return fmt.Errorf("server is %s", state)
	}
	p.cancelRestart()
	// Claim the start under the lock, so a crash-restart timer or a second click can't launch a second process.
	p.st.State = Starting
	p.mu.Unlock()
	if err := p.launch(); err != nil {
		p.setState(Stopped)
		return err
	}
	return nil
}

// cancelRestart stops a pending crash restart; p.mu must be held.
func (p *Proc) cancelRestart() {
	if p.restartT != nil {
		p.restartT.Stop()
		p.restartT = nil
	}
	p.st.NextStart = time.Time{}
}

func (p *Proc) launch() error {
	spec, err := p.resolve()
	if err != nil {
		p.Panelf("Can't start: %v", err)
		return err
	}
	cmd := exec.Command(spec.Exe)
	cmd.Dir = spec.Dir
	cmd.Env = spec.Env
	setProcAttr(cmd)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		p.Panelf("Can't start: %v", err)
		return err
	}
	k, err := track(cmd)
	if err != nil {
		_ = cmd.Process.Kill()
		return err
	}
	done := make(chan struct{})
	fin := make(chan struct{})
	p.mu.Lock()
	p.cmd, p.stdin, p.killer, p.done, p.fin, p.stopping = cmd, stdin, k, done, fin, false
	p.st = Status{State: Starting, PID: cmd.Process.Pid, StartedAt: time.Now(), Crashes: p.st.Crashes}
	p.cpuPrev = cpuSample{}
	cb := p.OnState
	p.mu.Unlock()
	if cb != nil {
		cb(Starting)
	}
	p.Panelf("Started %s (PID %d)", filepath.Base(spec.Exe), cmd.Process.Pid)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); p.pump(stdout, "out") }()
	go func() { defer wg.Done(); p.pump(stderr, "err") }()
	go p.healthLoop(spec.Port, done)
	go func() {
		wg.Wait()
		err := cmd.Wait()
		close(done)
		p.exited(err, spec.AutoRestart)
		close(fin) // the state is final (Stopped or Crashed) once fin is closed
	}()
	return nil
}

func (p *Proc) exited(err error, autoRestart bool) {
	code := 0
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		code = ee.ExitCode()
	}
	p.mu.Lock()
	wasStopping := p.stopping
	uptime := time.Since(p.st.StartedAt)
	if p.killer != nil {
		p.killer.Close()
	}
	p.cmd, p.stdin, p.killer = nil, nil, nil
	p.st.PID = 0
	p.st.Health = nil
	p.st.Players = nil
	p.st.LastExit = fmt.Sprintf("exit code %d", code)
	p.mu.Unlock()

	// A requested stop is a stop whatever the exit code (on Linux the engine exits 139 after a clean "quit").
	if wasStopping {
		p.Panelf("Server stopped")
		p.setState(Stopped)
		return
	}
	p.Panelf("Server exited unexpectedly (exit code %d)", code)
	p.mu.Lock()
	p.st.Crashes++
	if uptime > 10*time.Minute || p.backoff == 0 {
		p.backoff = 10 * time.Second
	} else {
		p.backoff = min(p.backoff*2, 5*time.Minute)
	}
	wait := p.backoff
	p.st.State = Crashed
	if autoRestart {
		p.scheduleRestart(wait)
	}
	cb := p.OnState
	p.mu.Unlock()
	if autoRestart {
		p.Panelf("Restarting in %s", wait.Round(time.Second))
	}
	if cb != nil {
		cb(Crashed)
	}
}

// scheduleRestart starts the server again after wait unless something else changed its state; p.mu must be held.
func (p *Proc) scheduleRestart(wait time.Duration) {
	p.st.NextStart = time.Now().Add(wait)
	p.restartT = time.AfterFunc(wait, func() {
		p.mu.Lock()
		p.restartT = nil
		p.st.NextStart = time.Time{}
		if p.st.State != Crashed {
			p.mu.Unlock()
			return
		}
		p.st.State = Starting // claimed, as in Start
		p.mu.Unlock()
		if err := p.launch(); err != nil {
			// The binary may be missing or locked for a moment: try again later instead of staying down.
			p.mu.Lock()
			p.st.State = Crashed
			p.backoff = min(p.backoff*2, 5*time.Minute)
			next := p.backoff
			p.scheduleRestart(next)
			p.mu.Unlock()
			p.Panelf("Restart failed; trying again in %s", next.Round(time.Second))
		}
	})
}

// Stop asks the server to quit and kills it if it hasn't after 30 seconds.
func (p *Proc) Stop() error {
	p.mu.Lock()
	p.cancelRestart()
	if p.cmd == nil {
		crashed := p.st.State == Crashed
		p.mu.Unlock()
		if crashed {
			p.setState(Stopped) // through setState, so "was running" and the stop event are recorded
		}
		return nil
	}
	p.stopping = true
	p.st.State = Stopping
	stdin, fin, k, cb := p.stdin, p.fin, p.killer, p.OnState
	p.mu.Unlock()
	if cb != nil {
		cb(Stopping)
	}
	p.Panelf("Stopping: sent \"quit\"")
	_, _ = io.WriteString(stdin, "quit\n")
	select {
	case <-fin:
	case <-time.After(30 * time.Second):
		p.Panelf("Server didn't stop within 30 seconds; killing it")
		_ = k.Kill()
		<-fin
	}
	return nil
}

// Restart stops and starts the server.
func (p *Proc) Restart() error {
	p.mu.Lock()
	p.restarting = true
	p.mu.Unlock()
	defer func() {
		p.mu.Lock()
		p.restarting = false
		p.mu.Unlock()
	}()
	if err := p.Stop(); err != nil {
		return err
	}
	return p.Start()
}

// Restarting reports whether a restart (stop then start) is under way.
func (p *Proc) Restarting() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.restarting
}

// Send writes one console command to the server.
func (p *Proc) Send(command string) error {
	command = strings.TrimSpace(command)
	if command == "" || strings.ContainsAny(command, "\r\n\x00") || len(command) > 1000 {
		return errors.New("invalid command")
	}
	p.mu.Lock()
	stdin := p.stdin
	p.mu.Unlock()
	if stdin == nil {
		return errors.New("server is not running")
	}
	p.emit(Line{Text: "> " + command, Src: "panel"})
	_, err := io.WriteString(stdin, command+"\n")
	return err
}

func (p *Proc) pump(r io.Reader, src string) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		text := strings.TrimRight(sc.Text(), "\r")
		if readyRe.MatchString(text) {
			p.mu.Lock()
			wasStarting := p.st.State == Starting
			if wasStarting {
				p.st.ReadyAt = time.Now()
			}
			p.mu.Unlock()
			if wasStarting {
				p.setState(Running)
			}
		}
		if p.captured(text) {
			continue
		}
		p.emit(Line{Text: text, Src: src})
	}
	if sc.Err() != nil {
		p.emit(Line{Text: "[LatchAdmin] Output line too long; the rest of this output is skipped", Src: "panel"})
		_, _ = io.Copy(io.Discard, r)
	}
}

// Panelf writes a line from LatchAdmin into the server's console.
func (p *Proc) Panelf(format string, a ...any) {
	p.emit(Line{Text: "[LatchAdmin] " + fmt.Sprintf(format, a...), Src: "panel"})
}

func (p *Proc) emit(l Line) {
	p.lmu.Lock()
	defer p.lmu.Unlock()
	p.next++
	l.N, l.At = p.next, time.Now()
	p.lines = append(p.lines, l)
	if len(p.lines) > keepLines {
		p.lines = p.lines[len(p.lines)-keepLines:]
	}
	p.writeLog(l)
	for ch := range p.subs {
		select {
		case ch <- l:
		default: // slow reader: it will see a gap ("n" jumps) and can reload
		}
	}
}

// Subscribe returns the recent lines and a channel of new ones. Call cancel when done.
func (p *Proc) Subscribe() ([]Line, <-chan Line, func()) {
	ch := make(chan Line, 512)
	p.lmu.Lock()
	recent := append([]Line(nil), p.lines...)
	p.subs[ch] = struct{}{}
	p.lmu.Unlock()
	return recent, ch, func() {
		p.lmu.Lock()
		delete(p.subs, ch)
		p.lmu.Unlock()
	}
}

func (p *Proc) writeLog(l Line) {
	day := l.At.Format("2006-01-02")
	if p.logF == nil || p.logDay != day {
		if p.logF != nil {
			p.logF.Close()
		}
		_ = os.MkdirAll(p.logDir, 0o755)
		f, err := os.OpenFile(filepath.Join(p.logDir, day+".log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			p.logF = nil
			return
		}
		p.logF, p.logDay = f, day
		go pruneLogs(p.logDir, 14)
	}
	fmt.Fprintf(p.logF, "%s %s\n", l.At.Format("15:04:05.000"), l.Text)
}

func pruneLogs(dir string, days int) {
	entries, _ := os.ReadDir(dir)
	cut := time.Now().AddDate(0, 0, -days).Format("2006-01-02")
	for _, e := range entries {
		if n := e.Name(); strings.HasSuffix(n, ".log") && n[:len(n)-4] < cut {
			_ = os.Remove(filepath.Join(dir, n))
		}
	}
}

// LogDir is where daily console logs are written.
func (p *Proc) LogDir() string { return p.logDir }

var healthClient = &http.Client{Timeout: 5 * time.Second}

// healthLoop asks the game port for its info JSON (MafiaHub servers answer GET / on the game port over TCP).
func (p *Proc) healthLoop(port int, done <-chan struct{}) {
	if port <= 0 {
		return
	}
	t := time.NewTicker(20 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-done:
			return
		case <-t.C:
		}
		h := probe(port)
		p.mu.Lock()
		p.st.Health = &h
		if h.MaxPlayers > 0 && p.st.MaxPlayers == 0 {
			p.st.MaxPlayers = h.MaxPlayers
		}
		promote := h.OK && p.st.State == Starting
		if promote {
			p.st.ReadyAt = time.Now()
		}
		p.mu.Unlock()
		if promote {
			p.setState(Running)
		}
	}
}

func probe(port int) Health {
	h := Health{At: time.Now()}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("http://127.0.0.1:%d/", port), nil)
	res, err := healthClient.Do(req)
	if err != nil {
		return h
	}
	defer res.Body.Close()
	var body struct {
		ModName    string `json:"mod_name"`
		ModVersion string `json:"mod_version"`
		MaxPlayers int    `json:"max_players"`
		Password   bool   `json:"password_required"`
	}
	if res.StatusCode == 200 && json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&body) == nil {
		h.OK, h.ModName, h.ModVersion, h.MaxPlayers, h.Password = true, body.ModName, body.ModVersion, body.MaxPlayers, body.Password
	}
	return h
}

// sample returns memory and CPU use; p.mu must be held.
func (p *Proc) sample() (uint64, float64) {
	if p.cmd == nil || p.cmd.Process == nil {
		return 0, 0
	}
	s, err := readSample(p.cmd.Process.Pid, p.killer)
	if err != nil {
		return 0, 0
	}
	cpu := 0.0
	if !p.cpuPrev.at.IsZero() {
		wall := s.at.Sub(p.cpuPrev.at).Seconds()
		if wall > 0 {
			cpu = (s.cpu - p.cpuPrev.cpu).Seconds() / wall * 100
		}
	}
	p.cpuPrev = s
	return s.mem, cpu
}

type cpuSample struct {
	at  time.Time
	cpu time.Duration
	mem uint64
}

// killer ends a server and everything it started.
type killer interface {
	Kill() error
	Close()
}
