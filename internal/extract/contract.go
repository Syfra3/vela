package extract

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/Syfra3/vela/pkg/types"
)

// Contract-layer ingestion. This extractor turns declared artifacts (OpenAPI
// specs and protobuf service definitions) into contract-layer nodes and edges
// that live alongside — not inside — the repo graph.
//
// Everything emitted here carries `layer=contract` and
// `evidence_confidence=declared` so that later fusion can preserve declared
// truth over weaker derived signals.

// Evidence type tags specific to the contract layer.
const (
	EvidenceTypeOpenAPI = "openapi"
	EvidenceTypeProto   = "proto"
)

// Contract-layer node ID helpers. Contract identity lives in its own namespace
// (prefix "contract:") so it never collides with the repo-local "<project>:"
// scheme — declared truth must remain addressable independently of any repo.
func contractServiceID(name string) string {
	return "contract:service:" + strings.ToLower(name)
}

func contractEndpointID(service, method, path string) string {
	return "contract:endpoint:" + strings.ToLower(service) + ":" + strings.ToLower(method) + ":" + path
}

func contractRPCID(service, method string) string {
	return "contract:rpc:" + strings.ToLower(service) + ":" + strings.ToLower(method)
}

// stampContractNode writes contract-layer + declared-evidence metadata.
func stampContractNode(n *types.Node, evType string, sourceArtifact string) {
	if n.Metadata == nil {
		n.Metadata = map[string]interface{}{}
	}
	n.Metadata[MetaLayer] = string(types.LayerContract)
	n.Metadata[MetaEvidenceType] = evType
	n.Metadata[MetaEvidenceConfidence] = string(types.ConfidenceDeclared)
	if sourceArtifact != "" {
		n.Metadata[MetaEvidenceSourceArtifact] = sourceArtifact
	}
}

// stampContractEdge writes contract-layer + declared-evidence metadata.
func stampContractEdge(e *types.Edge, evType string, sourceArtifact string) {
	if e.Metadata == nil {
		e.Metadata = map[string]interface{}{}
	}
	e.Metadata[MetaLayer] = string(types.LayerContract)
	e.Metadata[MetaEvidenceType] = evType
	e.Metadata[MetaEvidenceConfidence] = string(types.ConfidenceDeclared)
	if sourceArtifact != "" {
		e.Metadata[MetaEvidenceSourceArtifact] = sourceArtifact
	}
	// The Edge.Confidence legacy wire field also gets the declared marker so
	// pre-layer consumers (Obsidian export, existing rankers) still see the
	// strongest signal.
	e.Confidence = "DECLARED"
}

// IsContractFile returns true when a path looks like a declared contract
// artifact supported by this extractor.
func IsContractFile(path string) bool {
	base := strings.ToLower(filepath.Base(path))
	ext := strings.ToLower(filepath.Ext(path))
	if ext == ".proto" {
		return true
	}
	if ext == ".yaml" || ext == ".yml" || ext == ".json" {
		// Only pick files that look like OpenAPI: filename contains
		// "openapi" or "swagger". Avoid sniffing arbitrary yaml — that
		// would blur the contract boundary and let random manifests
		// declare services.
		if strings.Contains(base, "openapi") || strings.Contains(base, "swagger") {
			return true
		}
	}
	return false
}

// ExtractContract parses the supplied set of files, keeping only those
// recognised as contract artifacts, and returns contract-layer nodes + edges
// stamped with declared evidence.
//
// src is the repo/workspace context the artifacts were discovered in — the
// extractor emits a workspace-consumable binding edge (`declared_in`) from
// each service to the project node so workspace routing can pick up declared
// services without re-reading the artifact. If src is nil no binding edges are
// emitted (the contract graph is still produced).
func ExtractContract(root string, files []string, src *types.Source) ([]types.Node, []types.Edge, error) {
	var nodes []types.Node
	var edges []types.Edge

	seenService := map[string]bool{}
	seenNode := map[string]bool{}

	addNode := func(n types.Node) {
		if seenNode[n.ID] {
			return
		}
		seenNode[n.ID] = true
		nodes = append(nodes, n)
	}

	for _, path := range files {
		if !IsContractFile(path) {
			continue
		}
		rel := RelPath(root, path)
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		var fileNodes []types.Node
		var fileEdges []types.Edge
		var evType string
		ext := strings.ToLower(filepath.Ext(path))
		switch ext {
		case ".proto":
			evType = EvidenceTypeProto
			fileNodes, fileEdges = parseProto(string(data), rel)
		default:
			evType = EvidenceTypeOpenAPI
			fileNodes, fileEdges = parseOpenAPI(data, rel)
		}

		for _, n := range fileNodes {
			stampContractNode(&n, evType, rel)
			addNode(n)
			if n.NodeType == string(types.NodeTypeService) && !seenService[n.ID] && src != nil {
				seenService[n.ID] = true
				binding := types.Edge{
					Source:     n.ID,
					Target:     ProjectNodeID(src.Name),
					Relation:   "declared_in",
					SourceFile: rel,
				}
				stampContractEdge(&binding, evType, rel)
				edges = append(edges, binding)
			}
		}
		for _, e := range fileEdges {
			stampContractEdge(&e, evType, rel)
			edges = append(edges, e)
		}
	}

	// Stable ordering so snapshots and merge semantics are deterministic.
	sort.SliceStable(nodes, func(i, j int) bool { return nodes[i].ID < nodes[j].ID })
	sort.SliceStable(edges, func(i, j int) bool {
		if edges[i].Source != edges[j].Source {
			return edges[i].Source < edges[j].Source
		}
		if edges[i].Target != edges[j].Target {
			return edges[i].Target < edges[j].Target
		}
		return edges[i].Relation < edges[j].Relation
	})

	return nodes, edges, nil
}

// ── OpenAPI parser ──────────────────────────────────────────────────────────
//
// We only extract the structure we need for the contract graph: the service
// identity (info.title) and each operation (method + path). Schema bodies are
// intentionally ignored — contract-layer granularity is service / endpoint.

type openAPIDoc struct {
	OpenAPI string                               `yaml:"openapi" json:"openapi"`
	Swagger string                               `yaml:"swagger" json:"swagger"`
	Info    struct{ Title string }               `yaml:"info" json:"info"`
	Paths   map[string]map[string]map[string]any `yaml:"paths" json:"paths"`
}

var httpMethods = map[string]bool{
	"get": true, "post": true, "put": true, "patch": true,
	"delete": true, "head": true, "options": true, "trace": true,
}

func parseOpenAPI(data []byte, rel string) ([]types.Node, []types.Edge) {
	var doc openAPIDoc
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, nil
	}
	if doc.OpenAPI == "" && doc.Swagger == "" {
		return nil, nil
	}

	title := strings.TrimSpace(doc.Info.Title)
	if title == "" {
		// Fall back to the filename so every contract still gets a service
		// identity. Declared-but-unnamed services are still declared.
		title = strings.TrimSuffix(filepath.Base(rel), filepath.Ext(rel))
	}
	serviceID := contractServiceID(title)
	service := types.Node{
		ID:          serviceID,
		Label:       title,
		NodeType:    string(types.NodeTypeService),
		SourceFile:  rel,
		Description: fmt.Sprintf("Declared service %q (OpenAPI)", title),
	}

	nodes := []types.Node{service}
	var edges []types.Edge

	// Stable iteration over paths + methods.
	paths := make([]string, 0, len(doc.Paths))
	for p := range doc.Paths {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	for _, p := range paths {
		ops := doc.Paths[p]
		methods := make([]string, 0, len(ops))
		for m := range ops {
			if httpMethods[strings.ToLower(m)] {
				methods = append(methods, m)
			}
		}
		sort.Strings(methods)

		for _, m := range methods {
			label := strings.ToUpper(m) + " " + p
			epID := contractEndpointID(title, m, p)
			nodes = append(nodes, types.Node{
				ID:          epID,
				Label:       label,
				NodeType:    string(types.NodeTypeContract),
				SourceFile:  rel,
				Description: "Declared endpoint",
			})
			edges = append(edges, types.Edge{
				Source:     serviceID,
				Target:     epID,
				Relation:   "declares",
				SourceFile: rel,
			})
		}
	}

	return nodes, edges
}

// ── Proto parser ────────────────────────────────────────────────────────────
//
// A full protobuf parser is overkill for contract-layer identity — we only
// need service + rpc declarations. This tiny regex-based scanner handles the
// canonical `service Foo { rpc Bar(Req) returns (Resp); }` shape. Comments
// and nested structures are ignored on purpose.

var (
	reProtoService = regexp.MustCompile(`(?m)^\s*service\s+([A-Za-z_][A-Za-z0-9_]*)\s*\{`)
	reProtoRPC     = regexp.MustCompile(`(?m)^\s*rpc\s+([A-Za-z_][A-Za-z0-9_]*)\s*\(`)
)

func parseProto(src, rel string) ([]types.Node, []types.Edge) {
	var nodes []types.Node
	var edges []types.Edge

	// Find each service block and scan its body for rpc definitions. Splitting
	// on `{` / matching `}` would be more correct, but a simple forward scan
	// to the next `service` keyword is enough for the declared-identity use
	// case and avoids bringing in a proto parser dep.
	matches := reProtoService.FindAllStringSubmatchIndex(src, -1)
	for i, m := range matches {
		svcName := src[m[2]:m[3]]
		start := m[1]
		end := len(src)
		if i+1 < len(matches) {
			end = matches[i+1][0]
		}
		body := src[start:end]

		serviceID := contractServiceID(svcName)
		nodes = append(nodes, types.Node{
			ID:          serviceID,
			Label:       svcName,
			NodeType:    string(types.NodeTypeService),
			SourceFile:  rel,
			Description: fmt.Sprintf("Declared service %q (proto)", svcName),
		})

		rpcs := reProtoRPC.FindAllStringSubmatch(body, -1)
		for _, r := range rpcs {
			rpcName := r[1]
			rpcID := contractRPCID(svcName, rpcName)
			nodes = append(nodes, types.Node{
				ID:          rpcID,
				Label:       svcName + "." + rpcName,
				NodeType:    string(types.NodeTypeContract),
				SourceFile:  rel,
				Description: "Declared RPC",
			})
			edges = append(edges, types.Edge{
				Source:     serviceID,
				Target:     rpcID,
				Relation:   "declares",
				SourceFile: rel,
			})
		}
	}

	return nodes, edges
}
