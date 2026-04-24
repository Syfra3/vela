package server

import (
	"context"
	"encoding/json"
	"fmt"
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
	engine *query.Engine
	port   int
	srv    *http.Server
}

func New(engine *query.Engine, port int) *Server {
	return &Server{engine: engine, port: port}
}

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/graph", s.handleGraph)
	mux.HandleFunc("/query", s.handleQuery)
	mux.HandleFunc("/health", s.handleHealth)
	return mux
}

func (s *Server) Start(ctx context.Context) error {
	s.srv = &http.Server{
		Addr:         fmt.Sprintf(":%d", s.port),
		Handler:      s.routes(),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() {
		fmt.Printf("[serve] listening on http://localhost:%d\n", s.port)
		fmt.Println("[serve] endpoints: /graph  /query  /health")
		errCh <- s.srv.ListenAndServe()
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
	writeJSON(w, s.engine.Graph())
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
	output, err := s.engine.RunRequest(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, map[string]any{"request": req, "output": output})
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
