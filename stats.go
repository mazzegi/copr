package copr

import (
	"fmt"

	"github.com/mazzegi/log"
	"github.com/pkg/errors"
	"github.com/shirou/gopsutil/mem"
	"github.com/shirou/gopsutil/process"
)

const (
	kB = 1024
	mB = 1024 * kB
	gB = 1024 * mB
)

// Stats is a collection of typical process stats
type Stats struct {
	PID          int
	RSS          uint64
	VM           uint64
	CPUPerc      float64
	MEMPerc      float64
	RLimitSoftFD uint64
	RLimitHardFD uint64
	NumFD        uint64
	_lastCPUPerc float64
}

func memH(v float64) string {
	switch {
	case v < kB:
		return fmt.Sprintf("%.0f B", v)
	case v < mB:
		return fmt.Sprintf("%.1f KB", float64(v)/float64(kB))
	case v < gB:
		return fmt.Sprintf("%.1f MB", float64(v)/float64(mB))
	default:
		return fmt.Sprintf("%.1f GB", float64(v)/float64(gB))
	}
}

// RSSH returns RSS in a human-readable format
func (s Stats) RSSH() string {
	return memH(float64(s.RSS))
}

// VMH returns VM in a human-readable format
func (s Stats) VMH() string {
	return memH(float64(s.VM))
}

// String outputs the stats as string
func (s Stats) String() string {
	return fmt.Sprintf("pid=%d, rss=%s, vm=%s, cpu=%.1f, mem=%.1f sl=%d, hl=%d, fds=%d",
		s.PID, s.RSSH(), s.VMH(), s.CPUPerc, s.MEMPerc, s.RLimitSoftFD, s.RLimitHardFD, s.NumFD)
}

// GetStats collects the stats for the passed process
func CollectStats(proc *process.Process, stats *Stats) error {
	pmi, err := proc.MemoryInfo()
	if err != nil {
		return errors.Wrap(err, "memory-info")
	}
	vm, err := mem.VirtualMemory()
	if err != nil {
		return errors.Wrap(err, "virtual memory")
	}
	rls, _ := proc.Rlimit()
	numFDs, err := proc.NumFDs()
	if err != nil {
		numFDs = 0
	}

	stats.PID = int(proc.Pid)
	stats.RSS = pmi.RSS
	stats.VM = vm.Total
	stats.MEMPerc = float64(pmi.RSS) / float64(vm.Total) * 100.0
	stats.NumFD = uint64(numFDs)

	for _, rl := range rls {
		if rl.Resource == process.RLIMIT_NOFILE {
			stats.RLimitSoftFD = uint64(rl.Soft)
			stats.RLimitHardFD = uint64(rl.Hard)
		}
	}
	//
	currCPUPerc, err := proc.Percent(0)
	if err != nil {
		log.Warnf("PID %d, proc-percent (CPU): %v", proc.Pid, err)
		return nil
	}
	stats.CPUPerc = (currCPUPerc + stats._lastCPUPerc) / 2.0
	stats._lastCPUPerc = currCPUPerc

	return nil
}
