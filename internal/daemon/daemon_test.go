package daemon

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Syfra3/vela/internal/export"
	igraph "github.com/Syfra3/vela/internal/graph"
	"github.com/Syfra3/vela/internal/listener"
	"github.com/Syfra3/vela/internal/retrieval"
	"github.com/Syfra3/vela/pkg/types"
)

func withDaemonStubEmbeddings(t *testing.T) {
	t.Helper()
	restore := retrieval.SetEmbedTextsForTesting(func(texts []string) ([][]float32, error) {
		out := make([][]float32, 0, len(texts))
		for range texts {
			out = append(out, []float32{1, 0})
		}
		return out, nil
	})
	t.Cleanup(restore)
}

func TestDaemonStartStopLifecycle(t *testing.T) {
	t.Parallel()

	d := newTestDaemon(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := d.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if status := d.Status(); !strings.Contains(status, "running") {
		t.Fatalf("Status() = %q, want running", status)
	}

	if err := d.Stop(); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}

	if status := d.Status(); status != "stopped" {
		t.Fatalf("Status() after stop = %q, want stopped", status)
	}
}

func TestDaemonStartReturnsAlreadyRunning(t *testing.T) {
	t.Parallel()

	d := newTestDaemon(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := d.Start(ctx); err != nil {
		t.Fatalf("first Start() error = %v", err)
	}
	defer func() { _ = d.Stop() }()

	err := d.Start(ctx)
	if !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("second Start() error = %v, want ErrAlreadyRunning", err)
	}
}

func TestDaemonStopWhenNotRunning(t *testing.T) {
	t.Parallel()

	d := newTestDaemon(t)
	err := d.Stop()
	if !errors.Is(err, ErrNotRunning) {
		t.Fatalf("Stop() error = %v, want ErrNotRunning", err)
	}
}

func TestDaemonAncoraSocketReconcileSyncsObsidian(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	socketPath := filepath.Join(tmp, "ancora.sock")

	ready := make(chan struct{})
	serverErr := make(chan error, 1)
	go serveAncoraEvent(t, socketPath, ready, serverErr, listener.ObservationPayload{
		ID:           42,
		SyncID:       "sync-42",
		SessionID:    "sess-42",
		Type:         "decision",
		Title:        "Realtime memory",
		Content:      "Observation content from socket",
		Workspace:    "vela",
		Visibility:   "work",
		Organization: "glim",
		TopicKey:     "architecture/e2e",
		References:   []listener.Reference{{Type: "file", Target: "internal/store/store.go"}},
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	})
	<-ready

	g, err := igraph.Build(nil, nil)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	cfg := &types.Config{
		LLM: types.LLMConfig{},
		Watch: types.WatchConfig{
			Enabled: true,
			Sources: []types.WatchSourceConfig{{
				Name:   "ancora",
				Type:   "syfra",
				Socket: socketPath,
			}},
			Reconciler: types.ReconcilerConfig{DebounceMs: 0, MaxBatchSize: 10},
			Extractor:  types.ExtractorConfig{Enabled: false},
		},
		Daemon: types.DaemonConfig{
			PIDFile:    filepath.Join(tmp, "watch.pid"),
			LogFile:    "",
			StatusFile: filepath.Join(tmp, "watch-status.json"),
		},
		Obsidian: types.ObsidianConfig{AutoSync: true, VaultDir: tmp},
	}

	d, err := New(cfg, g)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := d.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer func() { _ = d.Stop() }()

	obsNote := filepath.Join(tmp, "obsidian", "Memories", "vela", "work", "Realtime memory.md")
	workspaceIndex := filepath.Join(tmp, "obsidian", "Memories", "_index", "workspace-vela.md")
	visibilityIndex := filepath.Join(tmp, "obsidian", "Memories", "_index", "visibility-work.md")
	organizationIndex := filepath.Join(tmp, "obsidian", "Memories", "_index", "organization-glim.md")

	deadline := time.Now().Add(3 * time.Second)
	for {
		if _, err := os.Stat(obsNote); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for obsidian note at %s", obsNote)
		}
		time.Sleep(25 * time.Millisecond)
	}

	if err := <-serverErr; err != nil {
		t.Fatalf("socket server error = %v", err)
	}

	data, err := os.ReadFile(obsNote)
	if err != nil {
		t.Fatalf("read obsidian note: %v", err)
	}
	content := string(data)
	for _, want := range []string{
		`obs_type: "decision"`,
		`workspace: "vela"`,
		`visibility: "work"`,
		`organization: "glim"`,
		`topic_key: "architecture/e2e"`,
		"Observation content from socket",
		"[[workspace-vela]]",
		"[[visibility-work]]",
		"[[organization-glim]]",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("obsidian note missing %q:\n%s", want, content)
		}
	}

	for _, path := range []string{workspaceIndex, visibilityIndex, organizationIndex} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected hierarchy note %s: %v", path, err)
		}
	}

	if _, ok := d.graph.NodeIndex["memory:observation:42"]; !ok {
		t.Fatal("expected observation node in in-memory graph")
	}
	if _, ok := d.graph.NodeIndex["memory:workspace:vela"]; !ok {
		t.Fatal("expected workspace node in in-memory graph")
	}
	if _, ok := d.graph.NodeIndex["memory:visibility:work"]; !ok {
		t.Fatal("expected visibility node in in-memory graph")
	}
	if _, ok := d.graph.NodeIndex["memory:organization:glim"]; !ok {
		t.Fatal("expected organization node in in-memory graph")
	}
}

func TestFlushGraphPreservesExistingProjects(t *testing.T) {
	t.Parallel()
	withDaemonStubEmbeddings(t)

	tmp := t.TempDir()
	graphPath := filepath.Join(tmp, "graph.json")

	existing := &types.Graph{
		Nodes: []types.Node{
			{
				ID:       "project:vela",
				Label:    "vela",
				NodeType: string(types.NodeTypeProject),
				Source:   &types.Source{Type: types.SourceTypeCodebase, Name: "vela", Path: "/work/vela"},
			},
			{
				ID:       "workspace:repo:vela",
				Label:    "vela",
				NodeType: string(types.NodeTypeWorkspace),
				Source:   &types.Source{Type: types.SourceTypeCodebase, Name: "vela", Path: "/work/vela"},
			},
		},
		Edges: []types.Edge{{Source: "workspace:repo:vela", Target: "project:vela", Relation: "documents"}},
	}
	if err := export.WriteJSON(existing, tmp); err != nil {
		t.Fatalf("WriteJSON(existing) error = %v", err)
	}

	memoryGraph, err := igraph.Build([]types.Node{
		{
			ID:       "memory:observation:1",
			Label:    "Preserve projects",
			NodeType: string(types.NodeTypeObservation),
			Source:   &types.Source{Type: types.SourceTypeMemory, Name: "ancora"},
		},
	}, []types.Edge{})
	if err != nil {
		t.Fatalf("Build(memory graph) error = %v", err)
	}

	d := &Daemon{graph: memoryGraph, graphPath: graphPath}
	d.flushGraph()

	loaded, err := export.LoadJSON(graphPath)
	if err != nil {
		t.Fatalf("LoadJSON() error = %v", err)
	}

	seenProject := false
	seenMemory := false
	for _, node := range loaded.Nodes {
		switch node.ID {
		case "project:vela":
			seenProject = true
		case "memory:observation:1":
			seenMemory = true
		}
	}
	if !seenProject {
		t.Fatal("expected existing project node to be preserved")
	}
	if !seenMemory {
		t.Fatal("expected memory node to be persisted")
	}
}

func serveAncoraEvent(t *testing.T, socketPath string, ready chan<- struct{}, errs chan<- error, payload listener.ObservationPayload) {
	t.Helper()

	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		errs <- err
		close(ready)
		return
	}
	defer func() {
		_ = ln.Close()
		_ = os.Remove(socketPath)
	}()
	close(ready)

	conn, err := ln.Accept()
	if err != nil {
		errs <- err
		return
	}
	defer conn.Close()

	reader := bufio.NewReader(conn)
	_ = conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	if line, err := reader.ReadString('\n'); err == nil && strings.HasPrefix(line, "AUTH ") {
		if _, err := conn.Write([]byte("OK\n")); err != nil {
			errs <- err
			return
		}
	}
	_ = conn.SetReadDeadline(time.Time{})

	framePayload, err := json.Marshal(payload)
	if err != nil {
		errs <- err
		return
	}
	frame, err := json.Marshal(struct {
		Type      listener.EventType `json:"type"`
		Timestamp time.Time          `json:"timestamp"`
		Payload   json.RawMessage    `json:"payload"`
	}{
		Type:      listener.EventObservationCreated,
		Timestamp: time.Now().UTC(),
		Payload:   framePayload,
	})
	if err != nil {
		errs <- err
		return
	}

	writer := bufio.NewWriter(conn)
	if _, err := writer.Write(append(frame, '\n')); err != nil {
		errs <- err
		return
	}
	if err := writer.Flush(); err != nil {
		errs <- err
		return
	}

	errs <- nil
}

func newTestDaemon(t *testing.T) *Daemon {
	t.Helper()

	g, err := igraph.Build(nil, nil)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	cfg := &types.Config{
		LLM: types.LLMConfig{},
		Watch: types.WatchConfig{
			Enabled: false,
			Sources: nil,
			Reconciler: types.ReconcilerConfig{
				DebounceMs:   0,
				MaxBatchSize: 10,
			},
			Extractor: types.ExtractorConfig{Enabled: false},
		},
		Daemon: types.DaemonConfig{
			PIDFile: filepath.Join(t.TempDir(), "watch.pid"),
			LogFile: "",
		},
	}

	d, err := New(cfg, g)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return d
}
