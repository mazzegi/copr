package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

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
	apiKey := os.Getenv("COPRD_APIKEY")
	if apiKey == "" {
		apiKey = "foo"
	}
	clt := newClient(host, apiKey)

	sub := strings.ToLower(strings.TrimSpace(os.Args[1]))
	t0 := time.Now()
	resp, err := clt.exec(sub, os.Args[2:])
	d := time.Since(t0)

	if err != nil {
		errf("REQUEST: %v", err.Error())
	}
	for _, em := range resp.CtrlErrors {
		errf("COPR: %v", em)
	}
	for _, m := range resp.CtrlMessages {
		logf("%s", m)
	}
	logf("%s", d)
}

func newClient(host string, apiKEy string) *client {
	return &client{
		httpClient: &http.Client{},
		host:       host,
		apiKey:     apiKEy,
	}
}

type client struct {
	httpClient *http.Client
	host       string
	apiKey     string
}

func (clt *client) req(r *http.Request) (copr.CTLResponse, error) {
	r.Header.Add("Authorization", fmt.Sprintf("Bearer %s", clt.apiKey))
	resp, err := clt.httpClient.Do(r)
	if err != nil {
		return copr.CTLResponse{}, errors.Wrapf(err, "get %q", r.URL.String())
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

func (clt *client) get(urlPath string) (copr.CTLResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	url := fmt.Sprintf("http://%s/%s", clt.host, urlPath)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return copr.CTLResponse{}, errors.Wrapf(err, "new-get-request to %q", url)
	}
	return clt.req(req)
}

func (clt *client) post(urlPath string, body io.Reader) (copr.CTLResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	url := fmt.Sprintf("http://%s/%s", clt.host, urlPath)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, body)
	if err != nil {
		return copr.CTLResponse{}, errors.Wrapf(err, "new-post-request to %q", url)
	}
	return clt.req(req)
}

func (clt *client) exec(cmd string, args []string) (copr.CTLResponse, error) {
	switch cmd {
	case "stat":
		if len(args) > 0 {
			return clt.get(fmt.Sprintf("stat?unit=%s", args[0]))
		} else {
			return clt.get("stat")
		}
	case "start-all":
		return clt.post("start-all", nil)
	case "stop-all":
		return clt.post("stop-all", nil)
	case "start":
		if len(args) != 1 {
			return copr.CTLResponse{}, errors.Errorf("usage: start <unit-name>")
		}
		return clt.post(fmt.Sprintf("start?unit=%s", args[0]), nil)
	case "stop":
		if len(args) != 1 {
			return copr.CTLResponse{}, errors.Errorf("usage: stop <unit-name>")
		}
		return clt.post(fmt.Sprintf("stop?unit=%s", args[0]), nil)
	case "enable":
		if len(args) != 1 {
			return copr.CTLResponse{}, errors.Errorf("usage: enable <unit-name>")
		}
		return clt.post(fmt.Sprintf("enable?unit=%s", args[0]), nil)
	case "disable":
		if len(args) != 1 {
			return copr.CTLResponse{}, errors.Errorf("usage: disable <unit-name>")
		}
		return clt.post(fmt.Sprintf("disable?unit=%s", args[0]), nil)
	case "deploy":
		return clt.deploy(args)
	default:
		return copr.CTLResponse{}, errors.Errorf("invalid subcommand %q", cmd)
	}
}

func (clt *client) deploy(args []string) (copr.CTLResponse, error) {
	if len(args) < 2 {
		return copr.CTLResponse{}, errors.Errorf("usage: deploy <unit> <folder>")
	}
	dir := args[1]
	buf := &bytes.Buffer{}
	err := copr.ZipDir(buf, dir)
	if err != nil {
		return copr.CTLResponse{}, errors.Wrapf(err, "zip-dir %q", dir)
	}
	return clt.post(fmt.Sprintf("deploy?unit=%s", args[0]), buf)
}
