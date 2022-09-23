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
	unit  Unit
	guard *Guard
	pid   int
}

func NewController(dir string) (*Controller, error) {
	us, err := LoadUnits(dir)
	if err != nil {
		return nil, errors.Wrapf(err, "load-units in %q", dir)
	}

	c := &Controller{}
	for _, u := range us.units {
		guard, err := NewGuard(
			filepath.Join(u.Dir, u.Config.Program),
			WithArgs(u.Config.Args...),
			WithEnv(u.Config.Env...),
			WithWd(u.Dir),
			WithRestartAfter(time.Second*time.Duration(u.Config.RestartAfterSec)),
		)
		if err != nil {
			return nil, errors.Wrapf(err, "new-guard for unit %q", u.Config.Name)
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
	units []*controllerUnit
}

func (c *Controller) RunCtx(ctx context.Context) {
	c.Lock()
	wg := sync.WaitGroup{}
	for _, cu := range c.units {
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
	<-ctx.Done()

	select {
	case <-time.After(5 * time.Second):
		log.Warnf("controller: timeout in wait for all guards done")
		return
	case <-allDoneC:
		log.Infof("controller: all guards are done")
		return
	}
}

func (c *Controller) StartAll() (messages []string, errors []string) {
	c.Lock()
	defer c.Unlock()
	for _, cu := range c.units {
		if cu.pid > -1 {
			messages = append(messages, fmt.Sprintf("guard %q is already started with PID %d", cu.unit.Config.Name, cu.pid))
			continue
		}

		pid, err := cu.guard.Start()
		if err != nil {
			errors = append(errors, fmt.Sprintf("ERROR: starting %q: %v", cu.unit.Config.Name, err))
			continue
		}
		cu.pid = pid
		messages = append(messages, fmt.Sprintf("started %q with PID %d", cu.unit.Config.Name, pid))
	}
	for _, m := range messages {
		log.Infof("controller: %s", m)
	}
	for _, m := range errors {
		log.Errorf("controller: %s", m)
	}
	return
}

func (c *Controller) StopAll() (messages []string, errors []string) {
	c.Lock()
	defer c.Unlock()
	for _, cu := range c.units {
		if cu.pid < 0 {
			messages = append(messages, fmt.Sprintf("guard %q is not started", cu.unit.Config.Name))
			continue
		}

		err := cu.guard.Stop()
		if err != nil {
			errors = append(errors, fmt.Sprintf("ERROR: stopping %q with PID %d: %v", cu.unit.Config.Name, cu.pid, err))
			continue
		}
		cu.pid = -1
		messages = append(messages, fmt.Sprintf("stopped %q", cu.unit.Config.Name))
	}
	for _, m := range messages {
		log.Infof("controller: %s", m)
	}
	for _, m := range errors {
		log.Errorf("controller: %s", m)
	}
	return
}
