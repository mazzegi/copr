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

	sub := strings.ToLower(strings.TrimSpace(os.Args[1]))
	resp, err := exec(host, sub, os.Args[2:])

	if err != nil {
		errf("REQUEST: %v", err.Error())
	}
	for _, em := range resp.Errors {
		errf("COPR: %s", em)
	}
	for _, m := range resp.Messages {
		logf("%s", m)
	}
}

func exec(host string, cmd string, args []string) (copr.CTLResponse, error) {
	switch cmd {
	case "stat":
		return get(host, "stat")
	case "start-all":
		return postCommand(host, "start-all")
	case "stop-all":
		return postCommand(host, "stop-all")
	case "start":
		if len(args) != 1 {
			return copr.CTLResponse{}, errors.Errorf("usage: start <unit-name>")
		}
		return postCommand(host, fmt.Sprintf("start?unit=%s", args[0]))
	case "stop":
		if len(args) != 1 {
			return copr.CTLResponse{}, errors.Errorf("usage: stop <unit-name>")
		}
		return postCommand(host, fmt.Sprintf("stop?unit=%s", args[0]))
	case "enable":
		if len(args) != 1 {
			return copr.CTLResponse{}, errors.Errorf("usage: enable <unit-name>")
		}
		return postCommand(host, fmt.Sprintf("enable?unit=%s", args[0]))
	case "disable":
		if len(args) != 1 {
			return copr.CTLResponse{}, errors.Errorf("usage: disable <unit-name>")
		}
		return postCommand(host, fmt.Sprintf("disable?unit=%s", args[0]))
	default:
		return copr.CTLResponse{}, errors.Errorf("invalid subcommand %q", cmd)
	}
}

func get(host string, urlPath string) (copr.CTLResponse, error) {
	url := fmt.Sprintf("http://%s/%s", host, urlPath)
	resp, err := http.Get(url)
	if err != nil {
		return copr.CTLResponse{}, errors.Wrapf(err, "get %q", url)
	}
	var ctlRes copr.CTLResponse
	err = json.NewDecoder(resp.Body).Decode(&ctlRes)
	if err != nil {
		return copr.CTLResponse{}, errors.Wrap(err, "decode-json")
	}
	if resp.StatusCode != http.StatusOK {
		return ctlRes, errors.Errorf("status %s", resp.Status)
	}
	return ctlRes, nil
}

func postCommand(host string, urlPath string) (copr.CTLResponse, error) {
	url := fmt.Sprintf("http://%s/%s", host, urlPath)
	resp, err := http.Post(url, "application/json", nil)
	if err != nil {
		return copr.CTLResponse{}, errors.Wrapf(err, "post %q", url)
	}
	var ctlRes copr.CTLResponse
	err = json.NewDecoder(resp.Body).Decode(&ctlRes)
	if err != nil {
		return copr.CTLResponse{}, errors.Wrap(err, "decode-json")
	}
	if resp.StatusCode != http.StatusOK {
		return ctlRes, errors.Errorf("status %s", resp.Status)
	}
	return ctlRes, nil
}
