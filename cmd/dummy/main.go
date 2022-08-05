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
