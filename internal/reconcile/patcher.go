package reconcile

import (
	"fmt"
	"sync"

	igraph "github.com/Syfra3/vela/internal/graph"
	"github.com/Syfra3/vela/pkg/types"
	"gonum.org/v1/gonum/graph/simple"
)

// memorySrc is the Source stamped on all observation nodes entering via the
// streaming path (Ancora IPC). Mirrors extract.memorySrc but lives here to
// avoid an import cycle.
var memorySrc = &types.Source{
	Type: types.SourceTypeMemory,
	Name: "ancora",
}

const memoryRootNodeID = "memory:ancora"

func workspaceNodeID(ws string) string   { return fmt.Sprintf("ancora:workspace:%s", ws) }
func visibilityNodeID(vis string) string { return fmt.Sprintf("ancora:visibility:%s", vis) }
func organizationNodeID(org string) string {
	return fmt.Sprintf("ancora:organization:%s", org)
}

// Patcher applies a ChangeSet to the in-memory graph using minimal mutations.
// It maintains indexes so it can quickly locate existing nodes and edges
// without scanning the full graph on every event.
type Patcher struct {
	mu sync.RWMutex

	graph     *igraph.Graph
	nodeIndex map[int64]string // ancora ID -> node ID string ("memory:observation:<id>")

	// llmQueue receives ObservationNodes queued for async LLM extraction.
	// It is buffered so the patcher is never blocked by a slow extractor.
	llmQueue chan types.ObservationNode
}

// NewPatcher creates a Patcher that operates on the given graph.
// llmQueueSize controls the buffer depth of the LLM extraction queue.
func NewPatcher(g *igraph.Graph, llmQueueSize int) *Patcher {
	if llmQueueSize <= 0 {
		llmQueueSize = 64
	}
	return &Patcher{
		graph:     g,
		nodeIndex: make(map[int64]string),
		llmQueue:  make(chan types.ObservationNode, llmQueueSize),
	}
}

// Apply mutates the graph according to the ChangeSet. The graph is locked for
// the duration of the call so callers do not need external synchronization.
func (p *Patcher) Apply(cs ChangeSet) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	for i := range cs.Added {
		if err := p.addNode(&cs.Added[i]); err != nil {
			return fmt.Errorf("add obs %d: %w", cs.Added[i].AncoraID, err)
		}
	}

	for i := range cs.Updated {
		if err := p.updateNode(&cs.Updated[i]); err != nil {
			return fmt.Errorf("update obs %d: %w", cs.Updated[i].AncoraID, err)
		}
	}

	for _, id := range cs.Deleted {
		p.deleteNode(id)
	}

	return nil
}

// LLMQueue returns the channel of nodes waiting for LLM reference extraction.
// The caller should drain this channel with a worker pool.
func (p *Patcher) LLMQueue() <-chan types.ObservationNode {
	return p.llmQueue
}

// addNode inserts a new ObservationNode into the graph.
func (p *Patcher) addNode(obs *types.ObservationNode) error {
	p.ensureMemoryHierarchy(obs)

	obs.Source = memorySrc
	node := obs.ToNode()

	// Upsert: if a node with this ID already exists, treat as update.
	if _, exists := p.graph.NodeIndex[obs.ID]; exists {
		return p.updateNode(obs)
	}

	// Add to graph structures.
	newID := int64(len(p.graph.NodeByID) + 1)
	p.graph.Directed.AddNode(simple.Node(newID))
	p.graph.NodeIndex[obs.ID] = newID
	p.graph.NodeByID[newID] = node
	p.graph.Nodes = append(p.graph.Nodes, node)
	p.nodeIndex[obs.AncoraID] = obs.ID

	// Add edges for explicit references.
	p.addReferenceEdges(obs)

	// Queue for async LLM extraction (non-blocking).
	select {
	case p.llmQueue <- *obs:
	default:
	}

	return nil
}

func (p *Patcher) ensureMemoryHierarchy(obs *types.ObservationNode) {
	p.ensureNode(types.Node{
		ID:          igraph.MemoryRootID,
		Label:       "Ancora Memory",
		NodeType:    string(types.NodeTypeMemorySource),
		Description: "Persistent memory: decisions, bugs, architecture observations",
		Source:      memorySrc,
	})

	if obs.Workspace != "" {
		p.ensureNode(types.Node{
			ID:       igraph.MemoryWorkspaceID(obs.Workspace),
			Label:    obs.Workspace,
			NodeType: string(types.NodeTypeWorkspace),
			Source:   memorySrc,
		})
	}

	if obs.Visibility != "" {
		p.ensureNode(types.Node{
			ID:       igraph.MemoryVisibilityID(obs.Visibility),
			Label:    obs.Visibility,
			NodeType: string(types.NodeTypeVisibility),
			Source:   memorySrc,
		})
	}

	if obs.Organization != "" {
		p.ensureNode(types.Node{
			ID:       igraph.MemoryOrganizationID(obs.Organization),
			Label:    obs.Organization,
			NodeType: string(types.NodeTypeOrganization),
			Source:   memorySrc,
		})
	}
}

func (p *Patcher) ensureNode(node types.Node) {
	if _, exists := p.graph.NodeIndex[node.ID]; exists {
		return
	}

	newID := int64(len(p.graph.NodeByID) + 1)
	p.graph.Directed.AddNode(simple.Node(newID))
	p.graph.NodeIndex[node.ID] = newID
	p.graph.NodeByID[newID] = node
	p.graph.Nodes = append(p.graph.Nodes, node)
}

// updateNode applies a delta update to an existing node.
func (p *Patcher) updateNode(obs *types.ObservationNode) error {
	gonumID, exists := p.graph.NodeIndex[obs.ID]
	if !exists {
		// Node not in graph yet — treat as add.
		return p.addNode(obs)
	}

	// Update in-place in NodeByID.
	updatedNode := obs.ToNode()
	p.graph.NodeByID[gonumID] = updatedNode

	// Sync the Nodes slice (linear scan, acceptable for the typical size).
	for i, n := range p.graph.Nodes {
		if n.ID == obs.ID {
			p.graph.Nodes[i] = updatedNode
			break
		}
	}

	// Rebuild edges: remove stale reference edges then re-add.
	p.removeReferenceEdges(obs.ID)
	p.addReferenceEdges(obs)

	// Queue for LLM re-extraction (non-blocking).
	select {
	case p.llmQueue <- *obs:
	default:
	}

	return nil
}

// deleteNode removes an observation node and all its edges from the graph.
func (p *Patcher) deleteNode(ancoraID int64) {
	nodeIDStr, ok := p.nodeIndex[ancoraID]
	if !ok {
		return
	}

	gonumID, ok := p.graph.NodeIndex[nodeIDStr]
	if !ok {
		return
	}

	// Remove from gonum graph.
	p.graph.Directed.RemoveNode(gonumID)

	// Remove from indexes.
	delete(p.graph.NodeIndex, nodeIDStr)
	delete(p.graph.NodeByID, gonumID)
	delete(p.nodeIndex, ancoraID)

	// Remove from Nodes slice.
	filtered := p.graph.Nodes[:0]
	for _, n := range p.graph.Nodes {
		if n.ID != nodeIDStr {
			filtered = append(filtered, n)
		}
	}
	p.graph.Nodes = filtered

	// Remove from ResolvedEdges.
	p.removeReferenceEdges(nodeIDStr)
}

// addReferenceEdges creates graph edges for the explicit references in an
// ObservationNode. Binder metadata is attached immediately so stale or
// ambiguous references remain visible instead of silently decaying.
func (p *Patcher) addReferenceEdges(obs *types.ObservationNode) {
	for _, ref := range obs.References {
		resolved, ok := igraph.ResolveMemoryReference(ref.Type, ref.Target, obs.ObsType, igraph.MemoryOptions{})
		if !ok {
			continue
		}
		edge := igraph.MemoryReferenceEdge(obs.ID, resolved, fmt.Sprintf("ancora:obs:%d", obs.AncoraID))
		edge = igraph.BindMemoryReferenceEdge(edge, p.graph.Nodes)
		// Only add to ResolvedEdges — the gonum graph is not updated here
		// because the target may be a bare label (file path, concept) that
		// does not correspond to an existing gonum node.
		p.graph.ResolvedEdges = append(p.graph.ResolvedEdges, edge)
	}
}

// removeReferenceEdges drops all ResolvedEdges originating from nodeID.
func (p *Patcher) removeReferenceEdges(nodeID string) {
	filtered := p.graph.ResolvedEdges[:0]
	for _, e := range p.graph.ResolvedEdges {
		if e.Source != nodeID {
			filtered = append(filtered, e)
		}
	}
	p.graph.ResolvedEdges = filtered
}
