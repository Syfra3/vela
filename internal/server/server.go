package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Syfra3/vela/internal/app"
	"github.com/Syfra3/vela/internal/query"
	"github.com/Syfra3/vela/pkg/types"
)

// Server exposes read-only graph and query endpoints.
type Server struct {
	provider func() *query.Engine
	port     int
	srv      *http.Server
}

func New(engine *query.Engine, port int) *Server {
	return NewWithProvider(func() *query.Engine { return engine }, port)
}

func NewWithProvider(provider func() *query.Engine, port int) *Server {
	return &Server{provider: provider, port: port}
}

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/graph", s.handleGraph)
	mux.HandleFunc("/query", s.handleQuery)
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/status", s.handleStatus)
	return mux
}

func (s *Server) listen() (net.Listener, error) {
	return net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(s.port)))
}

func (s *Server) Start(ctx context.Context) error {
	listener, err := s.listen()
	if err != nil {
		return err
	}
	s.srv = &http.Server{
		Addr:         listener.Addr().String(),
		Handler:      s.routes(),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() {
		fmt.Printf("[serve] listening on http://%s\n", listener.Addr())
		fmt.Println("[serve] endpoints: /graph  /query  /health  /status")
		errCh <- s.srv.Serve(listener)
	}()
	select {
	case <-ctx.Done():
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return s.srv.Shutdown(shutCtx)
	case err := <-errCh:
		return err
	}
}

func (s *Server) handleGraph(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	engine := s.provider()
	if !graphAvailable(w, engine) {
		return
	}
	writeJSON(w, engine.Graph())
}

func (s *Server) handleQuery(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	req, err := parseQueryRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	engine := s.provider()
	if !graphAvailable(w, engine) {
		return
	}
	output, err := engine.RunRequest(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, map[string]any{"request": req, "output": output, "freshness": engine.Freshness()})
}

func graphAvailable(w http.ResponseWriter, engine *query.Engine) bool {
	if d := engine.UnavailableDiagnostic(); d != nil {
		http.Error(w, d.Message, http.StatusServiceUnavailable)
		return false
	}
	if candidates := engine.AmbiguousCorpora(); len(candidates) > 1 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		writeJSON(w, map[string]any{"status": "ambiguous", "candidates": candidates})
		return false
	}
	return true
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, s.provider().StatusResult())
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]string{"status": "ok"})
}

func parseQueryRequest(r *http.Request) (types.QueryRequest, error) {
	input := app.QueryRequestInput{
		Kind:    types.QueryKind(strings.TrimSpace(r.URL.Query().Get("kind"))),
		Subject: strings.TrimSpace(r.URL.Query().Get("subject")),
		Target:  strings.TrimSpace(r.URL.Query().Get("target")),
	}
	if limitText := strings.TrimSpace(r.URL.Query().Get("limit")); limitText != "" {
		limit, err := strconv.Atoi(limitText)
		if err != nil {
			return types.QueryRequest{}, fmt.Errorf("invalid limit")
		}
		input.Limit = limit
	}
	return app.NormalizeQueryRequest(input)
}

func writeJSON(w http.ResponseWriter, value any) {
	data, err := json.Marshal(value)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(data)
}
