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

func WithStdIn(r io.Reader) GuardOption {
	return func(g *Guard) error {
		g.stdIn = r
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
		stdIn:    os.Stdin,
		stdOut:   os.Stdout,
		stdErr:   os.Stderr,
		actionC:  make(chan any),
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
type actionStartResult struct {
	err error
	pid int
}

type actionStopResult struct {
	err error
}

type actionStart struct {
	resC chan actionStartResult
}

type actionStop struct {
	resC chan actionStopResult
}

// type actionType string

// const (
// 	actionTypeStart actionType = "start"
// 	actionTypeStop  actionType = "stop"
// )

// type action struct {
// 	typ  actionType
// 	errC chan error
// }

type Guard struct {
	programm string
	args     []string
	env      []string
	wd       string
	stdIn    io.Reader
	stdOut   io.Writer
	stdErr   io.Writer
	actionC  chan any
}

const (
	killTimeout  = 5 * time.Second
	restartAfter = 5 * time.Second
)

func (g *Guard) Start() (pid int, err error) {
	resC := make(chan actionStartResult)
	g.actionC <- &actionStart{
		resC: resC,
	}
	res := <-resC
	return res.pid, res.err
}

func (g *Guard) Stop() error {
	resC := make(chan actionStopResult)
	g.actionC <- &actionStop{
		resC: resC,
	}
	res := <-resC
	return res.err
}

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
		cmd := exec.Command(g.programm, g.args...)
		env := os.Environ()
		cmd.Env = append(env, g.env...)
		cmd.Dir = g.wd
		cmd.Stdin = g.stdIn
		cmd.Stdout = g.stdOut
		cmd.Stderr = g.stdErr
		cmd.SysProcAttr = sysProcAttrChildProc()
		err := cmd.Start()
		if err != nil {
			return errors.Wrap(err, "start-command")
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
			switch a := a.(type) {
			case *actionStart:
				err := start()
				a.resC <- actionStartResult{
					err: err,
					pid: pid,
				}
			case *actionStop:
				err := kill()
				a.resC <- actionStopResult{
					err: err,
				}
			default:
				io.WriteString(g.stdErr, fmt.Sprintf("invalid action type %T", a))
			}
		}
	}
}