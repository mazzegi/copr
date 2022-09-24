package main

import (
	"context"
	"flag"
	"os"
	"os/signal"

	"github.com/mazzegi/copr"
	"github.com/mazzegi/log"
	"github.com/pkg/errors"
)

func main() {
	err := run()
	if err != nil {
		log.Errorf("run: %v", err)
	}
}

func run() error {
	bind := flag.String("bind", ":21001", "http-bind-address")
	dir := flag.String("dir", "../../_demo", "workspace directory")
	flag.Parse()

	controller, err := copr.NewController(*dir)
	if err != nil {
		return errors.Wrapf(err, "new controller in %q", *dir)
	}

	s, err := copr.NewService(*bind, controller)
	if err != nil {
		return errors.Wrap(err, "new-service")
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)
	defer cancel()
	err = s.RunCtx(ctx)
	return err
}