package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"syscall"
	"time"

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
			case coprtest.TestActionStress:
				go makeSomeStress(1 * time.Minute)
				w.WriteHeader(http.StatusOK)
			case coprtest.TestActionProbe:
				w.WriteHeader(http.StatusOK)
			case coprtest.TestActionGetEnv:
				env := os.Getenv(cmd.Param)
				w.WriteHeader(http.StatusOK)
				fmt.Fprint(w, env)
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

func makeSomeStress(dur time.Duration) {
	logf("make stress for %s", dur)
	defer logf("make stress done")
	timer := time.NewTimer(dur)
	var pis []float64
	for {
		select {
		case <-timer.C:
			for i := 0; i < 5; i++ {
				fmt.Println(pis[i])
			}
			return
		default:
			var pi float64
			for k := float64(0); k < float64(10000); k += 1.0 {
				pi += 1 / math.Pow(16, k) * (4/(8*k+1) - 2/(8*k+4) - 1/(8*k+5) - 1/(8*k+6))
			}
			pis = append(pis, pi)
			sort.Float64s(pis)
		}
	}
}
