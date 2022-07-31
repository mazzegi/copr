//go:build linux
// +build linux

package copr

import "syscall"

func sysProcAttrChildProc() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		Pdeathsig: syscall.SIGKILL,
		Setpgid:   true,
	}
}

func killProcess(pid int) error {
	return syscall.Kill(pid, syscall.SIGINT)
}
