package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/mazzegi/copr"
	"github.com/pkg/errors"
)

func logf(pattern string, args ...any) {
	fmt.Printf(pattern+"\n", args...)
}

func errf(pattern string, args ...any) {
	fmt.Printf("ERROR: "+pattern+"\n", args...)
}

func main() {
	if len(os.Args) < 2 {
		errf("need at least a subcommand")
		os.Exit(1)
	}

	host := os.Getenv("COPRD_HOST")
	if host == "" {
		host = "127.0.0.1:21001"
	}
	logf("connecting to host %s", host)

	var err error
	var msg string
	sub := strings.ToLower(strings.TrimSpace(os.Args[1]))

	switch sub {
	case "probe":
		msg, err = probe(host)
	default:
		err = errors.Errorf("invalid subcommand %q", sub)
	}

	if err != nil {
		errf(err.Error())
	} else {
		logf(msg)
	}
}

func probe(host string) (string, error) {
	url := fmt.Sprintf("http://%s/probe", host)
	resp, err := http.Get(url)
	if err != nil {
		return "", errors.Wrapf(err, "get %q", url)
	}
	var ctlRes copr.CTLResponse
	err = json.NewDecoder(resp.Body).Decode(&ctlRes)
	if err != nil {
		return "", errors.Wrap(err, "decode-json")
	}
	if resp.StatusCode != http.StatusOK {
		return "", errors.Errorf("status %s: %s", resp.Status, ctlRes.Message)
	}
	return ctlRes.Message, nil
}
