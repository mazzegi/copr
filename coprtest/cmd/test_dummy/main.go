package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/mazzegi/copr/coprtest"
)

func logf(pattern string, args ...any) {
	fmt.Printf("test-dummy: "+pattern+"\n", args...)
}

func main() {
	bind := flag.String("bind", "127.0.0.1:21000", "http bind address")
	flag.Parse()

	logf("start")
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM, syscall.SIGKILL)
	defer cancel()

	server := &http.Server{
		Addr: *bind,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var cmd coprtest.TestCommand
			err := json.NewDecoder(r.Body).Decode(&cmd)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			switch cmd.Action {
			case coprtest.TestActionCrash:
				w.WriteHeader(http.StatusOK)
				os.Exit(1)
			default:
				http.Error(w, fmt.Sprintf("unknown action %q", cmd.Action), http.StatusBadRequest)
				return
			}
		}),
	}
	go server.ListenAndServe()
	logf("listen-and-serve-http on %q", *bind)

	<-ctx.Done()
	server.Shutdown(context.Background())
	logf("done")
}
