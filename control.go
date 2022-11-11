package copr

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/mazzegi/log"
	"github.com/pkg/errors"
)

type controllerUnit struct {
	unit   Unit
	guard  *Guard
	cancel func()
}

func NewController(dir string, secs *Secrets, glbEnv map[string]string) (*Controller, error) {
	us, err := LoadUnits(dir, secs)
	if err != nil {
		return nil, errors.Wrapf(err, "load-units in %q", dir)
	}

	c := &Controller{
		unitConfigs: us,
		glbEnv:      glbEnv,
		commandC:    make(chan Command),
		statCache:   NewUnitStatsCache(),
	}
	for _, u := range us.units {
		u := u
		env := u.Config.Env
		for k, v := range glbEnv {
			env = append(env, fmt.Sprintf("%s=%s", k, v))
		}

		log.Debugf("controller: new-guard: prg=%q; args=%v; env=%v", u.Config.Program, u.Config.Args, env)
		guard, err := NewGuard(
			filepath.Join(u.Dir, u.Config.Program),
			WithArgs(u.Config.Args...),
			WithEnv(env...),
			WithWd(u.Dir),
			WithRestartAfter(time.Second*time.Duration(u.Config.RestartAfterSec)),
			WithOnChange(func(rs GuardRunningState, pid int) {
				switch rs {
				case GuardStatusRunningStarted:
					c.statCache.started(u.Name, pid)
				case GuardStatusRunningStopped:
					c.statCache.stopped(u.Name)
				}
			}),
		)
		if err != nil {
			return nil, errors.Wrapf(err, "new-guard for unit %q", u.Name)
		}

		c.units = append(c.units, &controllerUnit{
			unit:  u,
			guard: guard,
		})
		c.statCache.add(u.Name, u.Config.Enabled)
	}

	return c, nil
}

type Controller struct {
	sync.RWMutex
	unitConfigs *Units
	glbEnv      map[string]string
	units       []*controllerUnit
	commandC    chan Command
	statCache   *UnitStatsCache
}

func (c *Controller) RunCtx(ctx context.Context) {
	log.Infof("controller: run")
	c.Lock()
	wg := sync.WaitGroup{}
	for _, cu := range c.units {
		log.Infof("controller: run %q", cu.unit.Name)
		wg.Add(1)
		gctx, cancel := context.WithCancel(ctx)
		cu.cancel = cancel
		go func(g *Guard) {
			defer wg.Done()
			g.RunCtx(gctx)
		}(cu.guard)
	}
	c.Unlock()

	wg.Add(1)
	go func() {
		defer wg.Done()
		timer := time.NewTimer(5 * time.Second)
		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
				c.statCache.collect()
				timer.Reset(5 * time.Second)
			}
		}
	}()

	allDoneC := make(chan struct{})
	go func() {
		defer close(allDoneC)
		wg.Wait()
	}()
	log.Infof("controller: loop")

loop:
	for {
		select {
		case <-ctx.Done():
			break loop
		case cmd := <-c.commandC:
			switch cmd := cmd.(type) {
			case *CommandStartAll:
				log.Debugf("start-all-command")
				cmd.resultC <- c.startAll()
			case *CommandStopAll:
				cmd.resultC <- c.stopAll()
			case *CommandStart:
				cmd.resultC <- c.start(cmd.unit)
			case *CommandStop:
				cmd.resultC <- c.stop(cmd.unit)
			case *CommandEnable:
				cmd.resultC <- c.enable(cmd.unit)
			case *CommandDisable:
				cmd.resultC <- c.disable(cmd.unit)
			case *CommandDeploy:
				var resp CommandResponse
				cu, ok := c.findUnit(cmd.unit)
				if ok {
					resp = c.deployUpdate(cu, cmd.dir)
					cmd.resultC <- resp
					continue loop
				}
				cu, resp = c.deployCreate(cmd.unit, cmd.dir)
				if !resp.HasErrors() {
					log.Infof("controller: run %q", cu.unit.Name)
					wg.Add(1)
					gctx, cancel := context.WithCancel(ctx)
					cu.cancel = cancel
					go func(g *Guard) {
						defer wg.Done()
						g.RunCtx(gctx)
					}(cu.guard)
					sresp := c.start(cmd.unit)
					resp.merge(sresp)
				}
				cmd.resultC <- resp
			default:
				log.Warnf("invalid command of type %T", cmd)
			}
		}
	}

	select {
	case <-time.After(5 * time.Second):
		log.Warnf("controller: timeout in wait for all guards done")
		return
	case <-allDoneC:
		log.Infof("controller: all guards are done")
		return
	}
}

func (c *Controller) startAll() (resp CommandResponse) {
	for _, cu := range c.units {
		uresp := c.start(cu.unit.Name)
		resp.merge(uresp)
	}
	//resp.log()
	return
}

func (c *Controller) stopAll() (resp CommandResponse) {
	for _, cu := range c.units {
		uresp := c.stop(cu.unit.Name)
		resp.merge(uresp)
	}
	//resp.log()
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

func (c *Controller) unitDo(unit string, do func(cu *controllerUnit, resp *CommandResponse)) (resp CommandResponse) {
	if cu, ok := c.findUnit(unit); ok {
		do(cu, &resp)
		resp.log()
		return
	}
	resp.Errorf("no such unit %q", unit)
	resp.log()
	return
}

func (c *Controller) start(unit string) (resp CommandResponse) {
	return c.unitDo(unit, func(cu *controllerUnit, resp *CommandResponse) {
		if !cu.unit.Config.Enabled {
			resp.AddMsg("unit %q is disabled", unit)
			return
		}
		if cu.guard.IsStarted() {
			resp.AddMsg("guard %q is already started with PID %d", cu.unit.Name, cu.guard.PID())
			return
		}

		pid, err := cu.guard.Start()
		if err != nil {
			resp.Errorf("starting unit %q: %v", unit, err)
			return
		}
		//c.statCache.started(cu.unit.Name, pid)
		resp.AddMsg("started %q with PID %d", cu.unit.Name, pid)
	})
}

func (c *Controller) stop(unit string) (resp CommandResponse) {
	return c.unitDo(unit, func(cu *controllerUnit, resp *CommandResponse) {
		if !cu.guard.IsStarted() {
			resp.AddMsg("guard %q is not started", cu.unit.Name)
			return
		}

		err := cu.guard.Stop()
		if err != nil {
			resp.Errorf("ERROR: stopping %q with PID %d: %v", cu.unit.Name, cu.guard.PID(), err)
			return
		}
		//c.statCache.stopped(cu.unit.Name)
		resp.AddMsg("stopped %q", cu.unit.Name)
	})
}

func (c *Controller) enable(unit string) (resp CommandResponse) {
	return c.unitDo(unit, func(cu *controllerUnit, resp *CommandResponse) {
		if cu.unit.Config.Enabled {
			resp.AddMsg("unit %q is already enabled", unit)
			return
		}
		cu.unit.Config.Enabled = true
		err := c.unitConfigs.SaveUnit(cu.unit)
		if err != nil {
			resp.Errorf("enable unit %q: save: %v", unit, err)
			return
		}
		c.statCache.enabled(cu.unit.Name)
		resp.AddMsg("enable unit %q", unit)
	})
}

func (c *Controller) disable(unit string) (resp CommandResponse) {
	return c.unitDo(unit, func(cu *controllerUnit, resp *CommandResponse) {
		if !cu.unit.Config.Enabled {
			resp.AddMsg("unit %q is already disabled", unit)
			return
		}
		if cu.guard.IsStarted() {
			err := cu.guard.Stop()
			if err != nil {
				resp.Errorf("ERROR: stopping %q with PID %d: %v", cu.unit.Name, cu.guard.PID(), err)
				return
			}
			//c.statCache.stopped(cu.unit.Name)
			resp.AddMsg("stopped %q", cu.unit.Name)
		}

		cu.unit.Config.Enabled = false
		err := c.unitConfigs.SaveUnit(cu.unit)
		if err != nil {
			resp.Errorf("disable unit %q: save: %v", unit, err)
			return
		}
		c.statCache.disabled(cu.unit.Name)
		resp.AddMsg("disable unit %q", unit)
	})
}

func (c *Controller) deployCreate(unit string, dir string) (newUnit *controllerUnit, resp CommandResponse) {
	u, err := c.unitConfigs.Create(unit, dir)
	if err != nil {
		resp.AddError(errors.Wrapf(err, "create unit-config %q in %q", unit, dir))
		return nil, resp
	}
	resp.AddMsg("unit %q: created", unit)

	env := u.Config.Env
	for k, v := range c.glbEnv {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}

	guard, err := NewGuard(
		filepath.Join(u.Dir, u.Config.Program),
		WithArgs(u.Config.Args...),
		WithEnv(env...),
		WithWd(u.Dir),
		WithRestartAfter(time.Second*time.Duration(u.Config.RestartAfterSec)),
	)
	if err != nil {
		resp.AddError(errors.Wrapf(err, "new-guard for unit %q", u.Name))
		return nil, resp
	}

	newUnit = &controllerUnit{
		unit:  u,
		guard: guard,
	}
	c.units = append(c.units, newUnit)
	c.statCache.add(u.Name, u.Config.Enabled)
	return newUnit, resp
}

func (c *Controller) deployUpdate(cu *controllerUnit, dir string) (resp CommandResponse) {
	wasRunning := false
	if cu.guard.IsStarted() {
		wasRunning = true
		cu.guard.Stop()
	}

	//
	u, err := c.unitConfigs.Update(cu.unit.Name, dir)
	if err != nil {
		resp.AddError(errors.Wrapf(err, "%q: update-unit-config", cu.unit.Name))
		return resp
	}
	cu.unit = u

	//update guard
	env := u.Config.Env
	for k, v := range c.glbEnv {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}
	err = cu.guard.UpdateOpts(
		WithArgs(u.Config.Args...),
		WithEnv(env...),
		WithWd(u.Dir),
		WithRestartAfter(time.Second*time.Duration(u.Config.RestartAfterSec)),
		WithOnChange(func(rs GuardRunningState, pid int) {
			switch rs {
			case GuardStatusRunningStarted:
				c.statCache.started(u.Name, pid)
			case GuardStatusRunningStopped:
				c.statCache.stopped(u.Name)
			}
		}),
	)
	if err != nil {
		resp.AddError(errors.Wrapf(err, "%q: update-guard-options", cu.unit.Name))
		return resp
	}
	resp.AddMsg("unit %q: updated", cu.unit.Name)

	//
	if !cu.unit.Config.Enabled {
		resp.AddMsg("unit %q: disabled", cu.unit.Name)
		return
	}
	if !wasRunning {
		resp.AddMsg("unit %q: not started (was not running before)", cu.unit.Name)
		return
	}

	pid, err := cu.guard.Start()
	if err != nil {
		resp.Errorf("starting unit %q: %v", cu.unit.Name, err)
	} else {
		resp.AddMsg("started %q with PID %d", cu.unit.Name, pid)
	}
	return
}
