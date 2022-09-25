package copr

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/mazzegi/log"
	"github.com/pkg/errors"
	"github.com/shirou/gopsutil/process"
)

type controllerUnit struct {
	unit  Unit
	guard *Guard
	stats Stats
	proc  *process.Process
}

func NewController(dir string) (*Controller, error) {
	us, err := LoadUnits(dir)
	if err != nil {
		return nil, errors.Wrapf(err, "load-units in %q", dir)
	}

	c := &Controller{
		unitConfigs: us,
		runC:        make(chan *controllerUnit),
	}
	for _, u := range us.units {
		guard, err := NewGuard(
			filepath.Join(u.Dir, u.Config.Program),
			WithArgs(u.Config.Args...),
			WithEnv(u.Config.Env...),
			WithWd(u.Dir),
			WithRestartAfter(time.Second*time.Duration(u.Config.RestartAfterSec)),
		)
		if err != nil {
			return nil, errors.Wrapf(err, "new-guard for unit %q", u.Name)
		}

		c.units = append(c.units, &controllerUnit{
			unit:  u,
			guard: guard,
		})
	}

	return c, nil
}

type Controller struct {
	sync.RWMutex
	unitConfigs *Units
	units       []*controllerUnit
	runC        chan *controllerUnit
}

// TODO: route all requests and action over an extra channel and handle them in run
func (c *Controller) RunCtx(ctx context.Context, guardsRunningC chan struct{}) {
	log.Infof("controller: run")
	c.Lock()
	wg := sync.WaitGroup{}
	for _, cu := range c.units {
		log.Infof("controller: run %q", cu.unit.Name)
		wg.Add(1)
		go func(g *Guard) {
			defer wg.Done()
			g.RunCtx(ctx)
		}(cu.guard)
	}
	c.Unlock()

	allDoneC := make(chan struct{})
	go func() {
		defer close(allDoneC)
		wg.Wait()
	}()
	log.Infof("controller: loop")
	close(guardsRunningC)

	func() {
		timer := time.NewTimer(0)
		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
				c.collectStats()
				timer.Reset(5 * time.Second)
			case cu := <-c.runC:
				log.Infof("controller: run %q", cu.unit.Name)
				wg.Add(1)
				go func(g *Guard) {
					defer wg.Done()
					g.RunCtx(ctx)
				}(cu.guard)
			}
		}
	}()

	select {
	case <-time.After(5 * time.Second):
		log.Warnf("controller: timeout in wait for all guards done")
		return
	case <-allDoneC:
		log.Infof("controller: all guards are done")
		return
	}
}

type ControllerResponse struct {
	Messages []string
	Errors   []string
}

func (cr *ControllerResponse) Msgf(pattern string, args ...any) {
	cr.Messages = append(cr.Messages, fmt.Sprintf(pattern, args...))
}

func (cr *ControllerResponse) Errf(pattern string, args ...any) {
	cr.Messages = append(cr.Messages, fmt.Sprintf(pattern, args...))
}

func (cr ControllerResponse) log() {
	for _, m := range cr.Messages {
		log.Infof("controller: %s", m)
	}
	for _, m := range cr.Errors {
		log.Errorf("controller: %s", m)
	}
}

func (c *Controller) collectStats() {
	c.Lock()
	defer c.Unlock()
	for _, cu := range c.units {
		if !cu.guard.IsStarted() {
			cu.stats = Stats{}
			continue
		}
		pid := int32(cu.guard.PID())
		if cu.proc == nil || pid != cu.proc.Pid {
			proc, err := process.NewProcess(pid)
			if err != nil {
				log.Warnf("controller-stats: new-process for PID %d: %v", pid, err)
				continue
			}
			cu.proc = proc
		}

		err := CollectStats(cu.proc, &cu.stats)
		if err != nil {
			log.Warnf("collect stats for unit %q, pid %d", cu.unit.Name, cu.guard.PID())
			continue
		}
	}
}

//

func (c *Controller) StartAll() (resp ControllerResponse) {
	c.Lock()
	defer c.Unlock()
	for _, cu := range c.units {
		if cu.guard.IsStarted() {
			resp.Msgf("guard %q is already started with PID %d", cu.unit.Name, cu.guard.PID())
			continue
		}
		if !cu.unit.Config.Enabled {
			resp.Msgf("guard %q is disabled", cu.unit.Name)
			continue
		}

		pid, err := cu.guard.Start()
		if err != nil {
			resp.Errf("ERROR: starting %q: %v", cu.unit.Name, err)
			continue
		}
		resp.Msgf("started %q with PID %d", cu.unit.Name, pid)
	}
	resp.log()
	return
}

func (c *Controller) StopAll() (resp ControllerResponse) {
	c.Lock()
	defer c.Unlock()
	for _, cu := range c.units {
		if !cu.guard.IsStarted() {
			resp.Msgf("guard %q is not started", cu.unit.Name)
			continue
		}

		err := cu.guard.Stop()
		if err != nil {
			resp.Errf("ERROR: stopping %q with PID %d: %v", cu.unit.Name, cu.guard.PID(), err)
			continue
		}
		resp.Msgf("stopped %q", cu.unit.Name)
	}
	resp.log()
	return
}

func (c *Controller) findUnit(unit string) (*controllerUnit, bool) {
	for _, cu := range c.units {
		if cu.unit.Name == unit {
			return cu, true
		}
	}
	return nil, false
}

func (c *Controller) unitDo(unit string, do func(cu *controllerUnit, resp *ControllerResponse)) (resp ControllerResponse) {
	c.Lock()
	defer c.Unlock()
	if cu, ok := c.findUnit(unit); ok {
		do(cu, &resp)
		return
	}
	resp.Errf("no such unit %q", unit)
	resp.log()
	return
}

func (c *Controller) Start(unit string) (resp ControllerResponse) {
	return c.unitDo(unit, func(cu *controllerUnit, resp *ControllerResponse) {
		if !cu.unit.Config.Enabled {
			resp.Msgf("unit %q is disabled", unit)
			return
		}
		if cu.guard.IsStarted() {
			resp.Msgf("guard %q is already started with PID %d", cu.unit.Name, cu.guard.PID())
			return
		}

		pid, err := cu.guard.Start()
		if err != nil {
			resp.Errf("starting unit %q: %v", unit, err)
		} else {
			resp.Msgf("started %q with PID %d", cu.unit.Name, pid)
		}
	})
}

func (c *Controller) Stop(unit string) (resp ControllerResponse) {
	return c.unitDo(unit, func(cu *controllerUnit, resp *ControllerResponse) {
		if !cu.guard.IsStarted() {
			resp.Msgf("guard %q is not started", cu.unit.Name)
			return
		}

		err := cu.guard.Stop()
		if err != nil {
			resp.Errf("ERROR: stopping %q with PID %d: %v", cu.unit.Name, cu.guard.PID(), err)
			return
		}
		resp.Msgf("stopped %q", cu.unit.Name)
	})
}

func (c *Controller) Enable(unit string) (resp ControllerResponse) {
	return c.unitDo(unit, func(cu *controllerUnit, resp *ControllerResponse) {
		if cu.unit.Config.Enabled {
			resp.Msgf("unit %q is already enabled", unit)
			return
		}
		cu.unit.Config.Enabled = true
		err := c.unitConfigs.SaveUnit(cu.unit)
		if err != nil {
			resp.Errf("enable unit %q: save: %v", unit, err)
		} else {
			resp.Msgf("enable unit %q", unit)
		}
	})
}

func (c *Controller) Disable(unit string) (resp ControllerResponse) {
	return c.unitDo(unit, func(cu *controllerUnit, resp *ControllerResponse) {
		if !cu.unit.Config.Enabled {
			resp.Msgf("unit %q is already disabled", unit)
			return
		}
		if cu.guard.IsStarted() {
			err := cu.guard.Stop()
			if err != nil {
				resp.Errf("ERROR: stopping %q with PID %d: %v", cu.unit.Name, cu.guard.PID(), err)
				return
			}
			resp.Msgf("stopped %q", cu.unit.Name)
		}

		cu.unit.Config.Enabled = false
		err := c.unitConfigs.SaveUnit(cu.unit)
		if err != nil {
			resp.Errf("disable unit %q: save: %v", unit, err)
		} else {
			resp.Msgf("disable unit %q", unit)
		}
	})
}

func (c *Controller) Stat() (resp ControllerResponse) {
	c.Lock()
	defer c.Unlock()
	for _, cu := range c.units {
		if !cu.unit.Config.Enabled {
			resp.Msgf("unit %q: disabled", cu.unit.Name)
			continue
		}
		if cu.guard.IsStarted() {
			resp.Msgf("unit %q: enabled, started with PID %d: %s", cu.unit.Name, cu.guard.PID(), cu.stats.String())
		} else {
			resp.Msgf("unit %q: enabled, not-started", cu.unit.Name)
		}
	}
	return
}

func (c *Controller) Deploy(name string, dir string) (resp ControllerResponse, err error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return resp, errors.Errorf("empty unit name")
	}
	err = ValidateUnitDir(dir)
	if err != nil {
		return resp, errors.Wrapf(err, "validate unit-dir %q", dir)
	}

	//check if unit exists
	c.Lock()
	defer c.Unlock()
	if cu, ok := c.findUnit(name); ok {
		return c.deployUpdate(cu, dir)
	} else {
		return c.deployCreate(name, dir)
	}
}

func (c *Controller) deployCreate(unit string, dir string) (resp ControllerResponse, err error) {
	u, err := c.unitConfigs.Create(unit, dir)
	if err != nil {
		return resp, errors.Wrapf(err, "create unit-config %q in %q", unit, dir)
	}
	resp.Msgf("unit %q: created", unit)

	guard, err := NewGuard(
		filepath.Join(u.Dir, u.Config.Program),
		WithArgs(u.Config.Args...),
		WithEnv(u.Config.Env...),
		WithWd(u.Dir),
		WithRestartAfter(time.Second*time.Duration(u.Config.RestartAfterSec)),
	)
	if err != nil {
		return resp, errors.Wrapf(err, "new-guard for unit %q", u.Name)
	}

	cu := &controllerUnit{
		unit:  u,
		guard: guard,
	}
	c.units = append(c.units, cu)
	c.runC <- cu
	resp.Msgf("unit %q: guard is running", unit)

	if !u.Config.Enabled {
		resp.Msgf("unit %q: disabled", unit)
		return
	}

	pid, err := guard.Start()
	if err != nil {
		resp.Errf("starting unit %q: %v", unit, err)
	} else {
		resp.Msgf("started %q with PID %d", unit, pid)
	}

	return
}

func (c *Controller) deployUpdate(cu *controllerUnit, dir string) (resp ControllerResponse, err error) {
	wasRunning := false
	if cu.guard.IsStarted() {
		wasRunning = true
		cu.guard.Stop()
	}

	//
	u, err := c.unitConfigs.Update(cu.unit.Name, dir)
	if err != nil {
		return resp, errors.Wrapf(err, "%q: update-unit-config", cu.unit.Name)
	}
	cu.unit = u

	//update guard
	err = cu.guard.UpdateOpts(
		WithArgs(u.Config.Args...),
		WithEnv(u.Config.Env...),
		WithWd(u.Dir),
		WithRestartAfter(time.Second*time.Duration(u.Config.RestartAfterSec)),
	)
	if err != nil {
		return resp, errors.Wrapf(err, "%q: update-guard-options", cu.unit.Name)
	}
	resp.Msgf("unit %q: updated", cu.unit.Name)

	//
	if !cu.unit.Config.Enabled {
		resp.Msgf("unit %q: disabled", cu.unit.Name)
		return
	}
	if !wasRunning {
		resp.Msgf("unit %q: not started (was not running before)", cu.unit.Name)
		return
	}

	pid, err := cu.guard.Start()
	if err != nil {
		resp.Errf("starting unit %q: %v", cu.unit.Name, err)
	} else {
		resp.Msgf("started %q with PID %d", cu.unit.Name, pid)
	}
	return
}
