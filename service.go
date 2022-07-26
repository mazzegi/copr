package copr

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/mazzegi/log"
	"github.com/pkg/errors"
)

type CTLResponse struct {
	CtrlMessages []string `json:"ctrl-message,omitempty"`
	CtrlErrors   []string `json:"ctrl-errors,omitempty"`
}

func NewService(bind string, controller *Controller, apiKey string) (*Service, error) {
	l, err := net.Listen("tcp", bind)
	if err != nil {
		return nil, errors.Wrapf(err, "listen-tcp on %q", bind)
	}

	s := &Service{
		listener:   l,
		server:     &http.Server{},
		apiKey:     apiKey,
		controller: controller,
	}
	return s, nil
}

type Service struct {
	listener   net.Listener
	server     *http.Server
	apiKey     string
	controller *Controller
}

func (s *Service) RunCtx(ctx context.Context) error {
	s.server.Handler = http.HandlerFunc(s.handleHttp)
	go s.server.Serve(s.listener)
	log.Infof("serving on %q", s.listener.Addr())

	//activate controller
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		s.controller.RunCtx(ctx)
	}()

	s.controller.StartAll()

	<-ctx.Done()

	log.Infof("wait for controller done")
	wg.Wait()
	log.Infof("controller is done")

	sctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	s.server.Shutdown(sctx)
	log.Infof("server is done")
	return nil
}

func (s *Service) replyMsg(w http.ResponseWriter, status int, resp CommandResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(CTLResponse{
		CtrlMessages: resp.Messages,
		CtrlErrors:   resp.ErrorStrings(),
	})
}

func (s *Service) handleHttp(w http.ResponseWriter, r *http.Request) {
	authHeader := strings.TrimSpace(r.Header.Get("Authorization"))
	apiKey := strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer"))
	if apiKey != s.apiKey {
		resp := CommandResponse{}
		resp.Errorf("unauthorized")
		s.replyMsg(w, http.StatusUnauthorized, resp)
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.handleGET(w, r)
	case http.MethodPost:
		s.handlePOST(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Service) handleGET(w http.ResponseWriter, r *http.Request) {
	log.Debugf("handle-GET: %q", r.URL.Path)

	elt, tail, _ := strings.Cut(strings.TrimPrefix(r.URL.Path, "/"), "/")
	switch elt {
	case "stat":
		var resp CommandResponse
		unit := r.URL.Query().Get("unit")
		if unit == "" {
			resp = s.controller.StatAll()
		} else {
			resp = s.controller.Stat(unit)
		}
		s.replyMsg(w, http.StatusOK, resp)
	default:
		resp := CommandResponse{}
		resp.Errorf("no such resource %q", elt)
		s.replyMsg(w, http.StatusNotFound, resp)
	}
	_ = tail
}

func (s *Service) handlePOST(w http.ResponseWriter, r *http.Request) {
	log.Debugf("handle-POST: %q", r.URL.Path)
	elt, tail, _ := strings.Cut(strings.TrimPrefix(r.URL.Path, "/"), "/")
	switch elt {
	case "start-all":
		resp := s.controller.StartAll()
		s.replyMsg(w, http.StatusOK, resp)
	case "stop-all":
		resp := s.controller.StopAll()
		s.replyMsg(w, http.StatusOK, resp)
	case "start":
		resp := s.controller.Start(r.URL.Query().Get("unit"))
		s.replyMsg(w, http.StatusOK, resp)
	case "stop":
		resp := s.controller.Stop(r.URL.Query().Get("unit"))
		s.replyMsg(w, http.StatusOK, resp)
	case "enable":
		resp := s.controller.Enable(r.URL.Query().Get("unit"))
		s.replyMsg(w, http.StatusOK, resp)
	case "disable":
		resp := s.controller.Disable(r.URL.Query().Get("unit"))
		s.replyMsg(w, http.StatusOK, resp)
	case "deploy":
		resp, err := s.deploy(r)
		if err != nil {
			resp.AddError(err)
			s.replyMsg(w, http.StatusInternalServerError, resp)
		} else {
			s.replyMsg(w, http.StatusOK, resp)
		}
	default:
		resp := CommandResponse{}
		resp.Errorf("no such command %q", elt)
		s.replyMsg(w, http.StatusNotFound, resp)
	}
	_ = tail
}

//

func (s *Service) deploy(r *http.Request) (CommandResponse, error) {
	//copy content to temp file
	name := fmt.Sprintf("deploy_%s_%d", time.Now().Format("20060102150405"), rand.Intn(1000))
	tmpFile := fmt.Sprintf(".tmp/%s.zip", name)

	err := os.MkdirAll(".tmp", os.ModePerm)
	if err != nil {
		return CommandResponse{}, errors.Wrapf(err, "mkdirall %q", ".tmp")
	}
	defer os.RemoveAll(".tmp")
	tf, err := os.Create(tmpFile)
	if err != nil {
		return CommandResponse{}, errors.Wrapf(err, "create temp-file %q", tmpFile)
	}
	defer tf.Close()

	_, err = io.Copy(tf, r.Body)
	if err != nil {
		return CommandResponse{}, errors.Wrapf(err, "deploy copy to tmp-file %q", tmpFile)
	}

	// unzip folder
	tmpDir := fmt.Sprintf(".tmp/%s", name)
	err = UnzipTo(tmpFile, tmpDir)
	if err != nil {
		return CommandResponse{}, errors.Wrapf(err, "unzip %q to %q", tmpFile, tmpDir)
	}
	return s.controller.Deploy(r.URL.Query().Get("unit"), tmpDir), nil
}
