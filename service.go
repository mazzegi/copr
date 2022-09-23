package copr

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/mazzegi/log"
	"github.com/pkg/errors"
)

type CTLResponse struct {
	Message string `json:"message"`
}

func NewService(bind string) (*Service, error) {
	l, err := net.Listen("tcp", bind)
	if err != nil {
		return nil, errors.Wrapf(err, "listen-tcp on %q", bind)
	}
	s := &Service{
		listener: l,
		server:   &http.Server{},
	}
	return s, nil
}

type Service struct {
	listener net.Listener
	server   *http.Server
}

func (s *Service) RunCtx(ctx context.Context) error {
	s.server.Handler = http.HandlerFunc(s.handleHttp)
	go s.server.Serve(s.listener)
	log.Infof("serving on %q", s.listener.Addr())
	<-ctx.Done()

	sctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	s.server.Shutdown(sctx)
	return nil
}

func (s *Service) handleHttp(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleGET(w, r)
	case http.MethodPost:
		s.handlePOST(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Service) replyMsg(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(CTLResponse{Message: msg})
}

func (s *Service) handleGET(w http.ResponseWriter, r *http.Request) {
	log.Debugf("handle-GET: %q", r.URL.Path)

	elt, tail, _ := strings.Cut(strings.TrimPrefix(r.URL.Path, "/"), "/")
	switch elt {
	case "probe":
		s.replyMsg(w, "probe ok")
	}
	_ = tail
}

func (s *Service) handlePOST(w http.ResponseWriter, r *http.Request) {
	log.Debugf("handle-POST: %q", r.URL.Path)
}
