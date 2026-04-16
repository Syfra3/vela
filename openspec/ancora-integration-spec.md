# Spec: Vela-Ancora Real-Time Integration

**Project**: Vela  
**Feature**: Ancora Integration  
**Author**: G33N / Syfra3  
**Date**: 2026-04-15  
**Status**: Draft  

---

## 1. Overview

Enable Vela to listen to Ancora (and future Syfra ecosystem modules) via IPC sockets, receiving real-time change events and updating the knowledge graph incrementally using a React-style reconciliation algorithm.

### Goals

1. **Event-driven architecture** — No polling; Ancora emits events, Vela listens
2. **Minimal patches** — Update only affected nodes/edges, not full re-index
3. **Cross-platform IPC** — Unix sockets (Linux/macOS) + Named pipes (Windows)
4. **Secure communication** — Shared secret authentication for Syfra ecosystem only
5. **Pluggable listeners** — Ancora is first source; architecture supports future modules
6. **User-controlled daemon** — Start/stop via CLI/TUI, optional system service install

---

## 2. Architecture

```
┌──────────────────────────────────────────────────────────────┐
│                      Syfra Ecosystem                         │
│                                                              │
│  ~/.syfra/                                                   │
│    ipc-secret              # 32-byte shared auth token       │
│                                                              │
│  ┌─────────────┐                 ┌─────────────────────────┐ │
│  │   Ancora    │                 │         Vela            │ │
│  │  (memory)   │                 │   (knowledge graph)     │ │
│  │             │     events      │                         │ │
│  │ ~/.syfra/   │                 │  Listeners:             │ │
│  │ ancora.sock ●────────────────►│   - AncoraListener      │ │
│  │             │                 │   - FutureModListener   │ │
│  └─────────────┘                 │                         │ │
│                                  │  Reconciler:            │ │
│  ┌─────────────┐                 │   - Differ              │ │
│  │ Future Mod  │     events      │   - Patcher             │ │
│  │  (plugin)   │                 │                         │ │
│  │             │                 │  LLM Extractor:         │ │
│  │ ~/.syfra/   │                 │   - Async worker pool   │ │
│  │ futuremod.sock ●─────────────►│   - Write-back to       │ │
│  │             │                 │     Ancora              │ │
│  └─────────────┘                 │                         │ │
│                                  │  Daemon:                │ │
│                                  │   - PID file            │ │
│                                  │   - Start/Stop/Status   │ │
│                                  │   - Service install     │ │
│                                  └─────────────────────────┘ │
└──────────────────────────────────────────────────────────────┘
```

### Event Flow

```
1. User saves observation via Ancora MCP
2. Ancora Store.AddObservation() executes
3. Ancora emits event to ~/.syfra/ancora.sock
4. Vela daemon receives event (full payload)
5. Vela Reconciler:
   a. Parses explicit references from observation
   b. Creates/updates node in graph
   c. Creates edges to referenced nodes
   d. Queues observation for LLM extraction (async)
6. Vela LLM Extractor (background):
   a. Extracts entities/references via Ollama
   b. Patches graph with discovered edges
   c. Calls Ancora MCP `update` to persist references
7. Ancora emits update event (circular, but idempotent)
8. Vela receives update, no-ops if references unchanged
```

---

## 3. IPC Transport

### 3.1 Cross-Platform Abstraction

```go
// Package: github.com/Syfra3/ancora/internal/ipc (shared)

type Transport interface {
    // Server side
    Listen() (net.Listener, error)
    
    // Client side
    Dial() (net.Conn, error)
    
    // Path to socket/pipe
    Path() string
    
    // Cleanup (remove socket file, etc.)
    Close() error
}

// Implementations:
// - UnixTransport (Linux/macOS): ~/.syfra/<name>.sock
// - NamedPipeTransport (Windows): \\.\pipe\syfra-<name>
```

### 3.2 Socket Paths

| Module | Linux/macOS | Windows |
|--------|-------------|---------|
| Ancora | `~/.syfra/ancora.sock` | `\\.\pipe\syfra-ancora` |
| Future | `~/.syfra/<name>.sock` | `\\.\pipe\syfra-<name>` |

### 3.3 Authentication Handshake

Shared secret stored in `~/.syfra/ipc-secret`:
- Generated on first Syfra tool run (32 random bytes, hex-encoded)
- Readable only by owner (`chmod 600`)

Protocol:
```
Client connects
Client sends: AUTH <hex-encoded-secret>\n
Server validates:
  - If valid: sends OK\n, connection established
  - If invalid: sends ERR unauthorized\n, closes connection
```

### 3.4 Socket Lifecycle

Each module owns its socket:
- **Ancora**: Creates socket when MCP server starts (or dedicated event server)
- **Vela**: Connects as client to configured sockets
- **Graceful shutdown**: Remove socket file on clean exit
- **Stale socket detection**: If socket exists but no listener, remove and retry

---

## 4. Event Schema

### 4.1 Event Types

```go
// Package: github.com/Syfra3/ancora/internal/ipc

type EventType string

const (
    EventObservationCreated EventType = "observation.created"
    EventObservationUpdated EventType = "observation.updated"
    EventObservationDeleted EventType = "observation.deleted"
    EventSessionCreated     EventType = "session.created"
    EventSessionEnded       EventType = "session.ended"
)

type Event struct {
    Type      EventType       `json:"type"`
    Timestamp time.Time       `json:"timestamp"`
    Payload   json.RawMessage `json:"payload"`
}
```

### 4.2 Observation Payloads

**Created/Updated** (full payload):
```go
type ObservationPayload struct {
    ID           int64    `json:"id"`
    SyncID       string   `json:"sync_id"`
    SessionID    string   `json:"session_id"`
    Type         string   `json:"type"`
    Title        string   `json:"title"`
    Content      string   `json:"content"`
    Workspace    string   `json:"workspace,omitempty"`
    Visibility   string   `json:"visibility"`
    TopicKey     string   `json:"topic_key,omitempty"`
    References   []Reference `json:"references,omitempty"`
    CreatedAt    time.Time `json:"created_at"`
    UpdatedAt    time.Time `json:"updated_at"`
}

type Reference struct {
    Type   string `json:"type"`   // "file", "observation", "concept", "function"
    Target string `json:"target"` // "internal/store/store.go", "45", "auth-architecture"
}
```

**Deleted** (ID only):
```go
type ObservationDeletedPayload struct {
    ID     int64  `json:"id"`
    SyncID string `json:"sync_id"`
}
```

### 4.3 Wire Protocol

Events are newline-delimited JSON (NDJSON):
```
{"type":"observation.created","timestamp":"2026-04-15T10:00:00Z","payload":{...}}\n
{"type":"observation.updated","timestamp":"2026-04-15T10:00:01Z","payload":{...}}\n
```

---

## 5. Ancora Changes

### 5.1 Schema Addition

```sql
ALTER TABLE observations ADD COLUMN references TEXT; -- JSON array
```

### 5.2 MCP Tool Updates

**`save` tool** — Add optional `references` parameter:
```go
mcp.WithString("references",
    mcp.Description("JSON array of references: [{\"type\":\"file\",\"target\":\"path/to/file.go\"}]"),
),
```

**`update` tool** — Add optional `references` parameter (same format)

**`get` tool** — Include `references` in response

### 5.3 Event Emission

Add to `Store` struct:
```go
type Store struct {
    db          *sql.DB
    cfg         Config
    hooks       storeHooks
    eventServer *ipc.Server  // NEW: IPC event server
}
```

Emit after successful writes:
```go
func (s *Store) AddObservation(p AddObservationParams) (int64, error) {
    // ... existing logic ...
    
    if s.eventServer != nil {
        s.eventServer.Emit(ipc.Event{
            Type:      ipc.EventObservationCreated,
            Timestamp: time.Now(),
            Payload:   marshalObservationPayload(obs),
        })
    }
    
    return observationID, nil
}
```

### 5.4 Socket Server Lifecycle

Options for when socket is created:
- **A) With MCP server**: `ancora mcp` starts socket alongside MCP
- **B) Separate command**: `ancora events` starts dedicated event server
- **C) Always-on daemon**: `ancora daemon` runs both MCP + events

Recommendation: **A** — Socket starts with `ancora mcp`, controlled by flag:
```bash
ancora mcp --events          # Start MCP + event socket (default)
ancora mcp --no-events       # MCP only, no socket
```

---

## 6. Vela Changes

### 6.1 New Packages

```
vela/
  internal/
    listener/
      interface.go      # EventSource interface
      ancora.go         # Ancora socket listener
      registry.go       # Manages multiple listeners
    reconcile/
      differ.go         # Computes ChangeSet from events
      patcher.go        # Applies changes to graph
      queue.go          # Event queue with deduplication
    extract/
      llm_refs.go       # LLM-based reference extraction
      parser.go         # Explicit reference parsing
    daemon/
      daemon.go         # Daemon lifecycle
      service.go        # Systemd/launchd installer
      pid.go            # PID file management
```

### 6.2 EventSource Interface

```go
// Package: vela/internal/listener

type Event struct {
    Source    string          // "ancora", "future-mod"
    Type      string          // "observation.created", etc.
    Payload   json.RawMessage
    Timestamp time.Time
}

type EventSource interface {
    // Name returns the source identifier
    Name() string
    
    // Connect establishes connection to the source
    Connect(ctx context.Context) error
    
    // Events returns channel of incoming events
    Events() <-chan Event
    
    // Close disconnects from the source
    Close() error
    
    // Status returns connection status
    Status() SourceStatus
}

type SourceStatus struct {
    Connected   bool
    LastEvent   time.Time
    EventCount  int64
    LastError   error
}
```

### 6.3 Reconciler (React-Style Diff)

```go
// Package: vela/internal/reconcile

type ChangeSet struct {
    Added   []ObservationNode
    Updated []ObservationNode
    Deleted []int64
}

type Reconciler struct {
    graph       *graph.Graph
    nodeIndex   map[int64]*graph.Node  // observation ID -> node
    edgeIndex   map[string]*graph.Edge // edge key -> edge
    llmQueue    chan ObservationNode   // async extraction queue
}

func (r *Reconciler) Apply(event Event) error {
    switch event.Type {
    case "observation.created":
        return r.handleCreated(event.Payload)
    case "observation.updated":
        return r.handleUpdated(event.Payload)
    case "observation.deleted":
        return r.handleDeleted(event.Payload)
    }
    return nil
}

func (r *Reconciler) handleUpdated(payload json.RawMessage) error {
    var obs ObservationPayload
    json.Unmarshal(payload, &obs)
    
    // 1. Find existing node
    existing := r.nodeIndex[obs.ID]
    
    // 2. Compute new edges from explicit references
    newEdges := r.parseReferences(obs.References)
    
    // 3. Diff edges
    toAdd, toRemove := r.diffEdges(existing.Edges, newEdges)
    
    // 4. Apply patches
    for _, e := range toRemove {
        r.graph.RemoveEdge(e)
    }
    for _, e := range toAdd {
        r.graph.AddEdge(e)
    }
    
    // 5. Update node content
    existing.Content = obs.Content
    existing.UpdatedAt = obs.UpdatedAt
    
    // 6. Queue for LLM extraction (async)
    r.llmQueue <- obs
    
    return nil
}
```

### 6.4 LLM Reference Extraction

```go
// Package: vela/internal/extract

type LLMExtractor struct {
    client      *llm.Client
    ancoraAddr  string          // MCP address to write back
    workerCount int
}

func (e *LLMExtractor) Extract(obs ObservationPayload) ([]Reference, error) {
    prompt := fmt.Sprintf(`
Extract references from this observation. Return JSON array.

Observation:
Title: %s
Content: %s

Extract:
- File paths (type: "file")
- Function/struct names (type: "function")  
- Concept names (type: "concept")
- Related observation IDs if mentioned (type: "observation")

Response format:
[{"type": "file", "target": "internal/store/store.go"}, ...]
`, obs.Title, obs.Content)

    response, err := e.client.Generate(prompt)
    if err != nil {
        return nil, err
    }
    
    var refs []Reference
    json.Unmarshal([]byte(response), &refs)
    return refs, nil
}

func (e *LLMExtractor) WriteBack(obsID int64, refs []Reference) error {
    // Call Ancora MCP update tool
    refsJSON, _ := json.Marshal(refs)
    return e.ancoraClient.Update(obsID, map[string]any{
        "references": string(refsJSON),
    })
}
```

### 6.5 Daemon Lifecycle

```go
// Package: vela/internal/daemon

type Daemon struct {
    pidFile     string          // ~/.vela/watch.pid
    listeners   []EventSource
    reconciler  *Reconciler
    extractor   *LLMExtractor
    stopCh      chan struct{}
}

func (d *Daemon) Start() error {
    // 1. Check if already running
    if d.isRunning() {
        return ErrAlreadyRunning
    }
    
    // 2. Write PID file
    if err := d.writePID(); err != nil {
        return err
    }
    
    // 3. Connect to all sources
    for _, l := range d.listeners {
        if err := l.Connect(context.Background()); err != nil {
            log.Printf("WARN: failed to connect to %s: %v", l.Name(), err)
            // Continue — source may become available later
        }
    }
    
    // 4. Start event loop
    go d.eventLoop()
    
    // 5. Start LLM worker pool
    go d.extractorLoop()
    
    return nil
}

func (d *Daemon) Stop() error {
    close(d.stopCh)
    for _, l := range d.listeners {
        l.Close()
    }
    return d.removePID()
}
```

### 6.6 CLI Commands

```bash
# Daemon management
vela watch start              # Start daemon in background
vela watch stop               # Stop running daemon
vela watch status             # Show daemon status + connected sources
vela watch logs               # Tail daemon logs

# Source management  
vela watch add ancora         # Add Ancora as event source
vela watch add <name> <socket># Add custom source
vela watch remove <name>      # Remove source
vela watch list               # List configured sources

# Service installation
vela watch install            # Install as system service
vela watch uninstall          # Remove system service
```

### 6.7 TUI Integration

Add "Watch" option to main menu:
```
┌─────────────────────────────────────────┐
│  VELA - Knowledge Graph Explorer        │
├─────────────────────────────────────────┤
│  [E] Extract                            │
│  [Q] Query                              │
│  [W] Watch (daemon)         ● Running   │  <- NEW
│  [C] Config                             │
│  [D] Doctor                             │
│  [X] Exit                               │
└─────────────────────────────────────────┘
```

Watch submenu:
```
┌─────────────────────────────────────────┐
│  WATCH DAEMON                           │
├─────────────────────────────────────────┤
│  Status: Running (PID 12345)            │
│  Uptime: 2h 15m                         │
│  Events processed: 1,234                │
│                                         │
│  Sources:                               │
│    ● ancora    connected   847 events   │
│    ○ gitlab    disconnected             │
│                                         │
│  [S] Start/Stop daemon                  │
│  [A] Add source                         │
│  [I] Install service                    │
│  [L] View logs                          │
│  [B] Back                               │
└─────────────────────────────────────────┘
```

---

## 7. Observation as Graph Node

### 7.1 Node Type

```go
type NodeType string

const (
    NodeTypeFunction    NodeType = "function"
    NodeTypeStruct      NodeType = "struct"
    NodeTypeInterface   NodeType = "interface"
    NodeTypeFile        NodeType = "file"
    NodeTypePackage     NodeType = "package"
    // NEW: Knowledge nodes from Ancora
    NodeTypeObservation NodeType = "observation"
    NodeTypeConcept     NodeType = "concept"
)
```

### 7.2 Observation Node

```go
type ObservationNode struct {
    ID          string    // "ancora:obs:123"
    Type        NodeType  // NodeTypeObservation
    AncoraID    int64     // Original Ancora ID
    Title       string
    Content     string
    ObsType     string    // "decision", "bugfix", "architecture"
    Workspace   string
    References  []Reference
    CreatedAt   time.Time
    UpdatedAt   time.Time
}
```

### 7.3 Edge Types

```go
type EdgeType string

const (
    EdgeTypeCalls       EdgeType = "calls"
    EdgeTypeImports     EdgeType = "imports"
    EdgeTypeImplements  EdgeType = "implements"
    // NEW: Knowledge edges
    EdgeTypeReferences  EdgeType = "references"   // obs -> file/function
    EdgeTypeRelatedTo   EdgeType = "related_to"   // obs -> obs
    EdgeTypeDefines     EdgeType = "defines"      // obs -> concept
    EdgeTypeBelongsTo   EdgeType = "belongs_to"   // obs -> concept
)
```

---

## 8. Configuration

### 8.1 Vela Config Addition

`~/.vela/config.yaml`:
```yaml
watch:
  enabled: true
  sources:
    - name: ancora
      type: syfra
      socket: ~/.syfra/ancora.sock
    # Future sources:
    # - name: gitlab
    #   type: webhook
    #   url: http://localhost:8080/gitlab
  
  reconciler:
    debounce_ms: 100          # Batch events within this window
    max_batch_size: 50        # Max events per reconcile cycle
  
  extractor:
    enabled: true
    workers: 2                # Parallel LLM extraction workers
    write_back: true          # Write discovered refs back to Ancora
    provider: local           # LLM provider for extraction
    model: llama3

daemon:
  pid_file: ~/.vela/watch.pid
  log_file: ~/.vela/watch.log
  log_level: info
```

### 8.2 Syfra Config

`~/.syfra/config.yaml`:
```yaml
ipc:
  secret_file: ~/.syfra/ipc-secret
  socket_dir: ~/.syfra/
```

---

## 9. Task Breakdown

| # | Task | Package | Est | Dep |
|---|------|---------|-----|-----|
| 1 | IPC transport abstraction (Unix + Windows) | ancora/internal/ipc | 2d | - |
| 2 | Auth handshake with shared secret | ancora/internal/ipc | 1d | 1 |
| 3 | Event types and NDJSON serialization | ancora/internal/ipc | 0.5d | 1 |
| 4 | References column in Ancora schema | ancora/internal/store | 0.5d | - |
| 5 | MCP tools update (save/update/get refs) | ancora/internal/mcp | 1d | 4 |
| 6 | Store event emission hooks | ancora/internal/store | 1d | 3 |
| 7 | Socket server in Ancora (--events flag) | ancora/cmd | 1d | 1,2,6 |
| 8 | EventSource interface in Vela | vela/internal/listener | 0.5d | - |
| 9 | Ancora listener implementation | vela/internal/listener | 1.5d | 7,8 |
| 10 | Listener registry (multi-source) | vela/internal/listener | 1d | 9 |
| 11 | Reconciler: event queue + deduplication | vela/internal/reconcile | 1d | - |
| 12 | Reconciler: differ (compute ChangeSet) | vela/internal/reconcile | 1.5d | 11 |
| 13 | Reconciler: patcher (graph updates) | vela/internal/reconcile | 1.5d | 12 |
| 14 | Explicit reference parser | vela/internal/extract | 0.5d | - |
| 15 | LLM reference extractor | vela/internal/extract | 2d | 14 |
| 16 | Write-back to Ancora MCP | vela/internal/extract | 1d | 15 |
| 17 | Daemon lifecycle (start/stop/status/PID) | vela/internal/daemon | 1.5d | 9,11,15 |
| 18 | Daemon auto-reconnect logic | vela/internal/daemon | 1d | 17 |
| 19 | Service installer (systemd/launchd) | vela/internal/daemon | 1d | 17 |
| 20 | CLI commands (vela watch *) | vela/cmd | 1d | 17,19 |
| 21 | TUI watch menu | vela/internal/tui | 1d | 17 |
| 22 | Integration tests (Ancora + Vela e2e) | both | 2d | all |
| 23 | Documentation | both | 1d | all |

**Total: ~24 days** (~4-5 weeks with buffer)

---

## 10. Test Scenarios

### 10.1 Unit Tests

- IPC transport: socket creation, auth handshake, reconnection
- Event serialization: marshal/unmarshal roundtrip
- Reconciler: diff algorithm correctness
- Reference parser: regex extraction accuracy

### 10.2 Integration Tests

1. **Happy path**: Ancora save -> Vela receives -> Graph updated
2. **Reconnection**: Ancora restarts -> Vela reconnects -> No event loss
3. **Auth failure**: Wrong secret -> Connection rejected
4. **LLM extraction**: Observation -> LLM -> References extracted -> Written back
5. **Circular update**: LLM writes back -> Ancora emits -> Vela no-ops (idempotent)

### 10.3 Performance Tests

- 100 rapid saves -> All processed within 5s
- 1000 observations -> Graph patch time < 1s
- Memory usage stable over 24h daemon run

---

## 11. Risks & Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| Socket permission issues on multi-user systems | Connection failures | Document `~/.syfra/` permissions, add doctor check |
| Windows named pipe reliability | Platform-specific bugs | Extensive Windows CI testing |
| LLM extraction rate limiting | Slow write-back | Configurable worker count, backoff |
| Circular event loops | Infinite processing | Idempotency check on references field |
| Daemon crashes losing state | Missed events | Persist sync cursor, replay on restart |

---

## 12. Future Extensions

- **Webhook sources**: GitLab, GitHub events as additional listeners
- **Concept auto-detection**: LLM identifies recurring concepts across observations
- **Graph queries on observations**: "What decisions affected auth module?"
- **Cross-workspace linking**: Observations from workspace A reference code in workspace B
- **Real-time TUI updates**: Graph visualization updates live as events arrive
