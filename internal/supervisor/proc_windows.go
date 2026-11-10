//go:build windows

package supervisor

import (
	"os/exec"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// The server gets its own process group (Ctrl+C in the panel's console doesn't reach it; the panel stops it with
// "quit") and runs in a job object that kills it and its children if the panel itself dies.
func setProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NEW_PROCESS_GROUP, HideWindow: true}
}

type job struct {
	h    windows.Handle
	proc windows.Handle
}

func track(cmd *exec.Cmd) (killer, error) {
	h, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, err
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(h, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		windows.CloseHandle(h)
		return nil, err
	}
	ph, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE|windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.PROCESS_VM_READ, false, uint32(cmd.Process.Pid))
	if err != nil {
		windows.CloseHandle(h)
		return nil, err
	}
	if err := windows.AssignProcessToJobObject(h, ph); err != nil {
		windows.CloseHandle(ph)
		windows.CloseHandle(h)
		return nil, err
	}
	return &job{h: h, proc: ph}, nil
}

func (j *job) Kill() error { return windows.TerminateJobObject(j.h, 1) }

func (j *job) Close() {
	windows.CloseHandle(j.proc)
	windows.CloseHandle(j.h)
}

var (
	psapi                = windows.NewLazySystemDLL("psapi.dll")
	getProcessMemoryInfo = psapi.NewProc("GetProcessMemoryInfo")
)

type processMemoryCounters struct {
	Cb                         uint32
	PageFaultCount             uint32
	PeakWorkingSetSize         uintptr
	WorkingSetSize             uintptr
	QuotaPeakPagedPoolUsage    uintptr
	QuotaPagedPoolUsage        uintptr
	QuotaPeakNonPagedPoolUsage uintptr
	QuotaNonPagedPoolUsage     uintptr
	PagefileUsage              uintptr
	PeakPagefileUsage          uintptr
}

func readSample(_ int, k killer) (cpuSample, error) {
	j, ok := k.(*job)
	if !ok || j == nil {
		return cpuSample{}, windows.ERROR_INVALID_HANDLE
	}
	var c, e, kt, ut windows.Filetime
	if err := windows.GetProcessTimes(j.proc, &c, &e, &kt, &ut); err != nil {
		return cpuSample{}, err
	}
	ticks := func(f windows.Filetime) time.Duration {
		return time.Duration((int64(f.HighDateTime)<<32 | int64(f.LowDateTime)) * 100)
	}
	var m processMemoryCounters
	m.Cb = uint32(unsafe.Sizeof(m))
	r, _, err := getProcessMemoryInfo.Call(uintptr(j.proc), uintptr(unsafe.Pointer(&m)), uintptr(m.Cb))
	if r == 0 {
		return cpuSample{}, err
	}
	return cpuSample{at: time.Now(), cpu: ticks(kt) + ticks(ut), mem: uint64(m.WorkingSetSize)}, nil
}
