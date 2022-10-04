package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"path/filepath"

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
	sec := flag.String("sec", "", "copr secret password")
	flag.Parse()

	secPath := filepath.Join(*dir, copr.SecretFile)
	secs, err := copr.NewSecrets(secPath, *sec)
	if err != nil {
		return errors.Wrapf(err, "copr-new-secrets at %q", secPath)
	}

	apiKey, ok := secs.Find("copr.apikey")
	if !ok {
		return errors.Errorf("found no apikey")
	}

	controller, err := copr.NewController(*dir, secs)
	if err != nil {
		return errors.Wrapf(err, "new controller in %q", *dir)
	}

	s, err := copr.NewService(*bind, controller, apiKey)
	if err != nil {
		return errors.Wrap(err, "new-service")
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)
	defer cancel()
	err = s.RunCtx(ctx)
	return err
}
