package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Syfra3/vela/internal/ancora"
	"github.com/Syfra3/vela/internal/query"
	"github.com/Syfra3/vela/internal/retrieval"
)

// Server exposes the knowledge graph via legacy HTTP endpoints.
type Server struct {
	engine   *query.Engine
	searcher *query.Searcher
	port     int
	srv      *http.Server
}

// New creates a Server that serves the graph at the given port.
func New(engine *query.Engine, ancoraDBPath string, port int) *Server {
	if ancoraDBPath == "" {
		if resolved, err := ancora.DefaultDBPath(); err == nil {
			ancoraDBPath = resolved
		}
	}
	return &Server{engine: engine, searcher: query.NewSearcher(engine, ancoraDBPath), port: port}
}

// Start registers routes and begins listening. It blocks until the context
// is cancelled.
func (s *Server) Start(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/graph", s.handleGraph)
	mux.HandleFunc("/node/", s.handleNode)
	mux.HandleFunc("/path", s.handlePath)
	mux.HandleFunc("/search", s.handleSearch)
	mux.HandleFunc("/health", s.handleHealth)

	s.srv = &http.Server{
		Addr:         fmt.Sprintf(":%d", s.port),
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		fmt.Printf("[serve] listening on http://localhost:%d\n", s.port)
		fmt.Println("[serve] endpoints: /graph  /node/<id>  /path?from=A&to=B  /search?q=term  /health")
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

// handleGraph returns the full graph as JSON.
func (s *Server) handleGraph(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	result := s.engine.Query("nodes") // side-effect free — just reads graph
	_ = result

	// Return the raw graph data via engine introspection
	data, err := json.Marshal(s.engine.Graph())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(data)
}

// handleNode returns a single node by its ID.
func (s *Server) handleNode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := r.URL.Path[len("/node/"):]
	if id == "" {
		http.Error(w, "missing node id", http.StatusBadRequest)
		return
	}
	node, ok := s.engine.NodeByID(id)
	if !ok {
		http.Error(w, fmt.Sprintf("node %q not found", id), http.StatusNotFound)
		return
	}
	data, _ := json.Marshal(node)
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(data)
}

// handlePath returns the shortest path between two nodes.
func (s *Server) handlePath(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")
	if from == "" || to == "" {
		http.Error(w, "missing from or to query parameters", http.StatusBadRequest)
		return
	}
	result := s.engine.Path(from, to)
	data, _ := json.Marshal(map[string]string{"path": result})
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(data)
}

// handleHealth returns a simple health check.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	data, _ := json.Marshal(map[string]string{"status": "ok"})
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(data)
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		http.Error(w, "missing q query parameter", http.StatusBadRequest)
		return
	}
	limit := parseIntOrDefault(r.URL.Query().Get("limit"), 5)
	profile := query.SearchProfile(strings.ToLower(strings.TrimSpace(r.URL.Query().Get("profile"))))
	maxHops := parseIntOrDefault(r.URL.Query().Get("max_hops"), 2)
	maxExpansions := parseIntOrDefault(r.URL.Query().Get("max_expansions"), 24)
	relations := splitCSV(r.URL.Query().Get("relations"))

	searcher := s.searcher.WithTraversal(retrieval.TraversalOptions{MaxHops: maxHops, MaxExpansions: maxExpansions, AllowedRelations: relations})
	if profile == "" || profile == query.SearchProfileFederated {
		resp, err := searcher.Search(q, limit)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, resp)
		return
	}
	run, err := searcher.RunProfile(q, limit, profile)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, run)
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

func parseIntOrDefault(input string, fallback int) int {
	if strings.TrimSpace(input) == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(input)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func splitCSV(input string) []string {
	if strings.TrimSpace(input) == "" {
		return nil
	}
	parts := strings.Split(input, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		values = append(values, part)
	}
	if len(values) == 0 {
		return nil
	}
	return values
}
