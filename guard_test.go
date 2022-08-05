package copr

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/pkg/errors"
)

const prgSrc = `
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

func logf(pattern string, args ...any) {
	fmt.Printf(pattern+"\n", args...)
}

func main() {
	logf("start")
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM, syscall.SIGKILL)
	defer cancel()
	<-ctx.Done()
	logf("done")
}
`

func buildPrg(dir string, name string) error {
	tmpSrc := filepath.Join(dir, "src.go")
	err := os.WriteFile(tmpSrc, []byte(prgSrc), os.ModePerm)
	if err != nil {
		return errors.Wrapf(err, "write file tmp-src: %q", tmpSrc)
	}
	binPath := filepath.Join(dir, name)

	cmd := exec.Command("go", "build", "-v", "-o", binPath, tmpSrc)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err = cmd.Run()

	return err
}

func TestGuard(t *testing.T) {
	dir := "tmp"
	name := "dummy"

	err := os.MkdirAll(dir, os.ModePerm)
	if err != nil {
		t.Fatalf("mkdirall %q", dir)
		return
	}
	defer os.RemoveAll(dir)
	err = buildPrg(dir, name)
	if err != nil {
		t.Fatalf("build-prg")
		return
	}

	binPath := filepath.Join(dir, name)
	guard, err := NewGuard(binPath)
	if err != nil {
		t.Fatalf("new-guard")
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		guard.RunCtx(ctx)
	}()

	//start
	pid, err := guard.Start()
	if err != nil {
		t.Fatalf("guard-start failed: %v", err)
	}
	fmt.Printf("started with pid=%d\n", pid)

	<-time.After(5 * time.Second)
	cancel()
	wg.Wait()

	fmt.Printf("done\n")
}
