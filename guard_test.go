package copr

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/mazzegi/copr/coprtest"
	"github.com/pkg/errors"
)

func buildPrg(dir string, name string) error {
	tmpSrc := "coprtest/cmd/test_dummy/main.go"
	binPath := filepath.Join(dir, name)
	cmd := exec.Command("go", "build", "-v", "-o", binPath, tmpSrc)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	return err
}

func sendRequest(url string, cmd coprtest.TestCommand) ([]byte, error) {
	var buf bytes.Buffer
	err := json.NewEncoder(&buf).Encode(cmd)
	if err != nil {
		return nil, errors.Wrap(err, "encode-json")
	}

	resp, err := http.Post(url, "application/json", &buf)
	if err != nil {
		return nil, errors.Wrap(err, "post-request")
	}
	bs, _ := io.ReadAll(resp.Body)
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, errors.Errorf("request-error with status %s: %q", resp.Status, string(bs))
	}
	return bs, nil
}

func assertEqual[T comparable](t *testing.T, want T, have T, msg string, args ...any) {
	if want != have {
		t.Fatalf(fmt.Sprintf("assert-equal: want=%v, have=%v: ", want, have)+msg, args...)
	}
}

func assertNoErr(t *testing.T, err error, msg string, args ...any) {
	if err != nil {
		t.Fatalf(fmt.Sprintf("assert-no-error: want=no-error, have=%v: ", err)+msg, args...)
	}
}

func assertErr(t *testing.T, err error, msg string, args ...any) {
	if err == nil {
		t.Fatalf("assert-error: want=error, have=no-error: "+msg, args...)
	}
}

func TestGuard(t *testing.T) {
	bindir := "tmp_test_guard"
	name := "dummy"
	dummyBindAddr := "127.0.0.1:21001"
	dummyBindArg := "-bind=" + dummyBindAddr

	crash := func() {
		url := fmt.Sprintf("http://%s", dummyBindAddr)
		sendRequest(url, coprtest.TestCommand{
			Action: coprtest.TestActionCrash,
		})
	}

	err := os.MkdirAll(bindir, os.ModePerm)
	assertNoErr(t, err, "mkdirall %q", bindir)
	defer os.RemoveAll(bindir)

	err = buildPrg(bindir, name)
	assertNoErr(t, err, "build-prg")

	binPath := filepath.Join(bindir, name)
	guard, err := NewGuard(
		binPath,
		WithArgs(dummyBindArg),
		WithKillTimeout(500*time.Millisecond),
		WithRestartAfter(500*time.Millisecond),
	)
	assertNoErr(t, err, "new-guard")

	ctx, cancel := context.WithCancel(context.Background())
	wc := make(chan struct{})
	go func() {
		defer close(wc)
		guard.RunCtx(ctx)
	}()

	//start
	pid, err := guard.Start()
	assertNoErr(t, err, "guard-start")

	fmt.Printf("started with pid=%d\n", pid)
	assertEqual(t, GuardStatusRunningStarted, guard.Status().RunningState, "status after started")

	<-time.After(100 * time.Millisecond)
	crash()
	<-time.After(100 * time.Millisecond)
	assertEqual(t, GuardStatusRunningStopped, guard.Status().RunningState, "status after crash")

	<-time.After(500 * time.Millisecond)
	assertEqual(t, GuardStatusRunningStarted, guard.Status().RunningState, "status after restart")

	//
	err = guard.Stop()
	assertNoErr(t, err, "guard-stop")
	<-time.After(500 * time.Millisecond)
	assertEqual(t, GuardStatusRunningStopped, guard.Status().RunningState, "status after stop")

	pid, err = guard.Start()
	assertNoErr(t, err, "guard-start")
	fmt.Printf("started with pid=%d\n", pid)
	assertEqual(t, GuardStatusRunningStarted, guard.Status().RunningState, "status after started")

	cancel()
	select {
	case <-wc:
	case <-time.After(1 * time.Second):
		t.Fatalf("guard-exit via ctx failed")
	}

	fmt.Printf("done\n")
}
