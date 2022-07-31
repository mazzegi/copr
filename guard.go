package copr

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"

	"github.com/pkg/errors"
)

type GuardOption func(g *Guard) error

func WithArgs(args ...string) GuardOption {
	return func(g *Guard) error {
		g.args = args
		return nil
	}
}

func WithEnv(env ...string) GuardOption {
	return func(g *Guard) error {
		g.env = env
		return nil
	}
}

func WithWd(wd string) GuardOption {
	return func(g *Guard) error {
		g.wd = wd
		return nil
	}
}

func WithStdOut(w io.Writer) GuardOption {
	return func(g *Guard) error {
		g.stdOut = w
		return nil
	}
}

func WithStdErr(w io.Writer) GuardOption {
	return func(g *Guard) error {
		g.stdErr = w
		return nil
	}
}

func NewGuard(programm string, opts ...GuardOption) (*Guard, error) {
	wd, err := os.Getwd()
	if err != nil {
		return nil, errors.Wrap(err, "getwd")
	}
	g := &Guard{
		programm: programm,
		wd:       wd,
		stdOut:   os.Stdout,
		stdErr:   os.Stderr,
		actionC:  make(chan action),
	}
	for _, o := range opts {
		err := o(g)
		if err != nil {
			return nil, err
		}
	}
	return g, nil
}

//

type actionType string

const (
	actionTypeStart actionType = "start"
	actionTypeStop  actionType = "stop"
)

type action struct {
	typ  actionType
	errC chan error
}

type Guard struct {
	programm string
	args     []string
	env      []string
	wd       string
	stdOut   io.Writer
	stdErr   io.Writer
	actionC  chan action
}

const (
	killTimeout  = 5 * time.Second
	restartAfter = 5 * time.Second
)

func (g *Guard) RunCtx(ctx context.Context) {
	var pid int = -1
	exitC := make(chan struct{})

	isRunning := func() bool {
		return pid > -1
	}

	kill := func() error {
		if !isRunning() {
			return errors.Errorf("not running")
		}
		err := killProcess(pid)
		if err != nil {
			return errors.Wrap(err, "kill-process")
		}
		pid = -1
		timer := time.NewTimer(killTimeout)
		defer timer.Stop()
		select {
		case <-exitC:
			return nil
		case <-timer.C:
			return errors.Errorf("kill: timeout in waiting for exit")
		}
	}

	start := func() error {
		if isRunning() {
			return errors.Errorf("already running")
		}
		spid, err := g.start(exitC)
		if err != nil {
			return errors.Wrap(err, "start")
		}
		pid = spid
		return nil
	}

	restart := time.NewTimer(0)
	restart.Stop()
	for {
		select {
		case <-ctx.Done():
			kill()
			return
		case <-exitC:
			restart.Reset(restartAfter)
		case <-restart.C:
			err := start()
			if err != nil {
				io.WriteString(g.stdErr, fmt.Sprintf("restart: %v", err))
			}
		case a := <-g.actionC:
			switch a.typ {
			case actionTypeStart:
				err := start()
				a.errC <- err
			case actionTypeStop:
				err := kill()
				a.errC <- err
			default:
				a.errC <- errors.Errorf("invalid action type %q", a.typ)
			}
		}
	}
}

func (g *Guard) start(exitC chan struct{}) (pid int, err error) {
	cmd := exec.Command(g.programm, g.args...)
	env := os.Environ()
	cmd.Env = append(env, g.env...)
	cmd.Dir = g.wd
	cmd.Stdout = g.stdOut
	cmd.Stderr = g.stdErr
	cmd.SysProcAttr = sysProcAttrChildProc()

	err = cmd.Start()
	if err != nil {
		return -1, errors.Wrap(err, "start-command")
	}
	pid = cmd.Process.Pid

	go func() {
		defer func() {
			exitC <- struct{}{}
		}()
		err := cmd.Wait()
		if err != nil {
			io.WriteString(g.stdErr, fmt.Sprintf("error in cmd-wait: %v", err))
		}
	}()

	return pid, nil
}
