package copr

import (
	"fmt"
	"sort"
	"sync"

	"github.com/mazzegi/log"
	"github.com/pkg/errors"
	"github.com/shirou/gopsutil/mem"
	"github.com/shirou/gopsutil/process"
)

// Stats is a collection of typical process stats
type StatsDescriptor struct {
	Name         string
	Enabled      bool
	Started      bool
	PID          int
	RSS          uint64
	VM           uint64
	CPUPerc      float64
	MEMPerc      float64
	RLimitSoftFD uint64
	RLimitHardFD uint64
	NumFD        uint64
}

const (
	kB = 1024
	mB = 1024 * kB
	gB = 1024 * mB
)

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
func (s StatsDescriptor) RSSH() string {
	return memH(float64(s.RSS))
}

// VMH returns VM in a human-readable format
func (s StatsDescriptor) VMH() string {
	return memH(float64(s.VM))
}

// String outputs the stats as string
func (s StatsDescriptor) String() string {
	return fmt.Sprintf("%q: enabled=%t, started=%t, pid=%d, rss=%s, vm=%s, cpu=%.1f, mem=%.1f sl=%d, hl=%d, fds=%d",
		s.Name, s.Enabled, s.Started,
		s.PID, s.RSSH(), s.VMH(), s.CPUPerc, s.MEMPerc, s.RLimitSoftFD, s.RLimitHardFD, s.NumFD)
}

type stats struct {
	name         string
	enabled      bool
	pid          int
	rss          uint64
	vm           uint64
	cpuperc      float64
	memperc      float64
	rlimitsoftfd uint64
	rlimithardfd uint64
	numfd        uint64
	_lastCPUPerc float64
	proc         *process.Process
}

func (s stats) descriptor() StatsDescriptor {
	return StatsDescriptor{
		Name:         s.name,
		Enabled:      s.enabled,
		Started:      s.pid > -1,
		PID:          s.pid,
		RSS:          s.rss,
		VM:           s.vm,
		CPUPerc:      s.cpuperc,
		MEMPerc:      s.memperc,
		RLimitSoftFD: s.rlimitsoftfd,
		RLimitHardFD: s.rlimithardfd,
		NumFD:        s.numfd,
	}
}

func (s *stats) collect() error {
	pmi, err := s.proc.MemoryInfo()
	if err != nil {
		return errors.Wrap(err, "memory-info")
	}
	vm, err := mem.VirtualMemory()
	if err != nil {
		return errors.Wrap(err, "virtual memory")
	}
	rls, _ := s.proc.Rlimit()
	numFDs, err := s.proc.NumFDs()
	if err != nil {
		numFDs = 0
	}

	s.pid = int(s.proc.Pid)
	s.rss = pmi.RSS
	s.vm = vm.Total
	s.memperc = float64(pmi.RSS) / float64(vm.Total) * 100.0
	s.numfd = uint64(numFDs)

	for _, rl := range rls {
		if rl.Resource == process.RLIMIT_NOFILE {
			s.rlimitsoftfd = uint64(rl.Soft)
			s.rlimithardfd = uint64(rl.Hard)
		}
	}
	//
	currCPUPerc, err := s.proc.Percent(0)
	if err != nil {
		log.Warnf("PID %d, proc-percent (CPU): %v", s.proc.Pid, err)
		return nil
	}
	s.cpuperc = (currCPUPerc + s._lastCPUPerc) / 2.0
	s._lastCPUPerc = currCPUPerc

	return nil
}

func NewUnitStatsCache() *UnitStatsCache {
	return &UnitStatsCache{
		unitStats: make(map[string]*stats),
	}
}

type UnitStatsCache struct {
	sync.RWMutex
	unitStats map[string]*stats
}

func (c *UnitStatsCache) collect() error {
	c.Lock()
	defer c.Unlock()
	for _, s := range c.unitStats {
		if s.proc == nil {
			continue
		}
		err := s.collect()
		if err != nil {
			log.Errorf("%q collect: %v", s.name, err)
		}
	}
	return nil
}

func (c *UnitStatsCache) statsDescriptor(unit string) (StatsDescriptor, error) {
	c.RLock()
	defer c.RUnlock()
	s, ok := c.unitStats[unit]
	if !ok {
		return StatsDescriptor{}, errors.Errorf("no such unit %q", unit)
	}
	return s.descriptor(), nil
}

func (c *UnitStatsCache) allStatsDescriptors() []StatsDescriptor {
	c.RLock()
	defer c.RUnlock()
	var sds []StatsDescriptor
	for _, s := range c.unitStats {
		sds = append(sds, s.descriptor())
	}
	sort.Slice(sds, func(i, j int) bool {
		return sds[i].Name < sds[j].Name
	})
	return sds
}

func (c *UnitStatsCache) add(name string, enabled bool) {
	c.Lock()
	defer c.Unlock()
	c.unitStats[name] = &stats{
		name:    name,
		enabled: enabled,
	}
}

func (c *UnitStatsCache) started(name string, pid int) {
	c.Lock()
	defer c.Unlock()
	if us, ok := c.unitStats[name]; ok {
		log.Debugf("started %q %d", name, pid)
		proc, err := process.NewProcess(int32(pid))
		if err != nil {
			log.Errorf("new process with pid %d", pid)
			return
		}
		us.pid = pid
		us.proc = proc
	}
}

func (c *UnitStatsCache) stopped(name string) {
	c.Lock()
	defer c.Unlock()
	if us, ok := c.unitStats[name]; ok {
		log.Debugf("stopped %q", name)
		us.pid = -1
		us.proc = nil
	}
}

func (c *UnitStatsCache) enabled(name string) {
	c.Lock()
	defer c.Unlock()
	if us, ok := c.unitStats[name]; ok {
		us.enabled = true
	}
}

func (c *UnitStatsCache) disabled(name string) {
	c.Lock()
	defer c.Unlock()
	if us, ok := c.unitStats[name]; ok {
		us.enabled = false
	}
}
