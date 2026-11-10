//go:build windows

package metrics

import (
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	kernel32             = windows.NewLazySystemDLL("kernel32.dll")
	getSystemTimes       = kernel32.NewProc("GetSystemTimes")
	globalMemoryStatusEx = kernel32.NewProc("GlobalMemoryStatusEx")
)

type memoryStatusEx struct {
	Length               uint32
	MemoryLoad           uint32
	TotalPhys            uint64
	AvailPhys            uint64
	TotalPageFile        uint64
	AvailPageFile        uint64
	TotalVirtual         uint64
	AvailVirtual         uint64
	AvailExtendedVirtual uint64
}

func ft(f windows.Filetime) uint64 { return uint64(f.HighDateTime)<<32 | uint64(f.LowDateTime) }

// cpuTimes returns busy and total CPU time across all cores.
func cpuTimes() (busy, total uint64, ok bool) {
	var idle, kernel, user windows.Filetime
	r, _, _ := getSystemTimes.Call(uintptr(unsafe.Pointer(&idle)), uintptr(unsafe.Pointer(&kernel)), uintptr(unsafe.Pointer(&user)))
	if r == 0 {
		return 0, 0, false
	}
	// Kernel time includes idle time.
	total = ft(kernel) + ft(user)
	return total - ft(idle), total, true
}

func memory() (used, total uint64, ok bool) {
	var m memoryStatusEx
	m.Length = uint32(unsafe.Sizeof(m))
	r, _, _ := globalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&m)))
	if r == 0 {
		return 0, 0, false
	}
	return m.TotalPhys - m.AvailPhys, m.TotalPhys, true
}

func disk(path string) (free, total uint64, ok bool) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, 0, false
	}
	var avail, tot, totalFree uint64
	if err := windows.GetDiskFreeSpaceEx(p, &avail, &tot, &totalFree); err != nil {
		return 0, 0, false
	}
	return avail, tot, true
}

func cpuCount() int { return runtime.NumCPU() }
