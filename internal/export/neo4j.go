package export

import (
	"context"
	"fmt"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"

	"github.com/Syfra3/vela/pkg/types"
)

// PushNeo4j connects to the Neo4j instance at boltURL and writes all nodes and
// edges from g. Existing data is NOT cleared — nodes and edges are merged using
// MERGE to make the operation idempotent.
//
// Returns a clear error if Neo4j is unreachable (not a panic).
func PushNeo4j(g *types.Graph, boltURL, username, password string) error {
	ctx := context.Background()

	driver, err := neo4j.NewDriverWithContext(boltURL, neo4j.BasicAuth(username, password, ""))
	if err != nil {
		return fmt.Errorf("creating neo4j driver: %w", err)
	}
	defer driver.Close(ctx)

	if err := driver.VerifyConnectivity(ctx); err != nil {
		return fmt.Errorf("neo4j unreachable at %s: %w", boltURL, err)
	}

	session := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	// Write nodes
	if err := writeNeo4jNodes(ctx, session, g.Nodes); err != nil {
		return fmt.Errorf("writing nodes: %w", err)
	}

	// Write edges
	if err := writeNeo4jEdges(ctx, session, g.Edges); err != nil {
		return fmt.Errorf("writing edges: %w", err)
	}

	return nil
}

func writeNeo4jNodes(ctx context.Context, session neo4j.SessionWithContext, nodes []types.Node) error {
	for _, n := range nodes {
		// Use MERGE so re-runs are idempotent
		cypher := fmt.Sprintf(
			`MERGE (n:%s {id: $id}) SET n.label = $label, n.file = $file, n.community = $community`,
			neo4jLabel(n.NodeType),
		)
		params := map[string]any{
			"id":        n.ID,
			"label":     n.Label,
			"file":      n.SourceFile,
			"community": n.Community,
		}
		_, err := session.Run(ctx, cypher, params)
		if err != nil {
			return fmt.Errorf("node %s: %w", n.ID, err)
		}
	}
	return nil
}

func writeNeo4jEdges(ctx context.Context, session neo4j.SessionWithContext, edges []types.Edge) error {
	for _, e := range edges {
		relType := neo4jRelType(e.Relation)
		cypher := fmt.Sprintf(
			`MATCH (a {id: $from}), (b {id: $to})
			 MERGE (a)-[r:%s]->(b)
			 SET r.confidence = $confidence, r.file = $file`,
			relType,
		)
		params := map[string]any{
			"from":       e.Source,
			"to":         e.Target,
			"confidence": e.Confidence,
			"file":       e.SourceFile,
		}
		_, err := session.Run(ctx, cypher, params)
		if err != nil {
			// Edge endpoints may not exist (dangling refs) — skip, don't abort
			continue
		}
	}
	return nil
}

// neo4jLabel converts a node type string to a valid Neo4j node label.
// Neo4j labels must start with a letter and contain only alphanumeric + underscore.
func neo4jLabel(nodeType string) string {
	switch nodeType {
	case "function":
		return "Function"
	case "method":
		return "Method"
	case "struct":
		return "Struct"
	case "interface":
		return "Interface"
	case "class":
		return "Class"
	case "concept":
		return "Concept"
	case "module":
		return "Module"
	case "constant":
		return "Constant"
	default:
		return "Node"
	}
}

// neo4jRelType converts an edge relation string to a valid Neo4j relationship type.
func neo4jRelType(relation string) string {
	switch relation {
	case "calls":
		return "CALLS"
	case "imports":
		return "IMPORTS"
	case "uses":
		return "USES"
	case "implements":
		return "IMPLEMENTS"
	case "extends":
		return "EXTENDS"
	case "describes":
		return "DESCRIBES"
	case "related_to":
		return "RELATED_TO"
	default:
		return "RELATED_TO"
	}
}
