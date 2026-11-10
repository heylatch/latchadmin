package metrics

import "time"

// HostSampler measures the whole machine: CPU busy share across all cores, and memory in use.
type HostSampler struct {
	busy, total uint64
}

// Sample returns CPU percent (0..100 for the whole machine) and memory used/total. The first call has no CPU.
func (h *HostSampler) Sample(dir string) (Sample, HostInfo) {
	s := Sample{T: time.Now().Unix(), Up: true, Players: -1}
	info := HostInfo{CPUs: cpuCount()}
	if busy, total, ok := cpuTimes(); ok {
		if h.total > 0 && total > h.total {
			s.CPU = float32(float64(busy-h.busy) / float64(total-h.total) * 100)
		}
		h.busy, h.total = busy, total
	}
	if used, total, ok := memory(); ok {
		s.Mem, info.MemTotal = used, total
	}
	if free, total, ok := disk(dir); ok {
		info.DiskFree, info.DiskTotal = free, total
	}
	return s, info
}
