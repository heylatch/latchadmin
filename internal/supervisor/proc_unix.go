//go:build !windows

package supervisor

import (
	"errors"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type group struct{ pid int }

func track(cmd *exec.Cmd) (killer, error) { return &group{pid: cmd.Process.Pid}, nil }

// Kill ends the whole process group the server leads.
func (g *group) Kill() error { return syscall.Kill(-g.pid, syscall.SIGKILL) }

func (g *group) Close() {}

const clockTicks = 100 // USER_HZ on every Linux build LatchAdmin targets

func readSample(pid int, _ killer) (cpuSample, error) {
	stat, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return cpuSample{}, err
	}
	// Fields after the command name (which may contain spaces) start after the last ')'.
	s := string(stat)
	i := strings.LastIndexByte(s, ')')
	if i < 0 {
		return cpuSample{}, errors.New("bad /proc stat")
	}
	f := strings.Fields(s[i+1:])
	if len(f) < 13 {
		return cpuSample{}, errors.New("bad /proc stat")
	}
	ut, _ := strconv.ParseInt(f[11], 10, 64)
	st, _ := strconv.ParseInt(f[12], 10, 64)
	statm, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/statm")
	if err != nil {
		return cpuSample{}, err
	}
	sf := strings.Fields(string(statm))
	var rss int64
	if len(sf) > 1 {
		rss, _ = strconv.ParseInt(sf[1], 10, 64)
	}
	return cpuSample{
		at:  time.Now(),
		cpu: time.Duration(ut+st) * time.Second / clockTicks,
		mem: uint64(rss) * uint64(os.Getpagesize()),
	}, nil
}
