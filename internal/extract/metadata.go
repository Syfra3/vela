package extract

import "github.com/Syfra3/vela/pkg/types"

// Metadata keys stamped onto extracted nodes and edges so that downstream
// layers (graph insertion, chunking, retrieval, evidence attribution) have a
// uniform, layer-aware contract independent of which extractor produced them.
//
// Canonical identity is still assigned by ExtractAll (project prefixing, file
// node IDs); extractors only produce layer-local metadata here — they must not
// invent graph-level identity.
const (
	MetaLayer                  = "layer"
	MetaEvidenceType           = "evidence_type"
	MetaEvidenceConfidence     = "evidence_confidence"
	MetaEvidenceSourceArtifact = "evidence_source_artifact"
)

// Evidence type tags used when stamping extraction output. These correspond to
// the Evidence.Type field in pkg/types and let the orchestrator attribute
// retrieval hits back to the extractor that produced them.
const (
	EvidenceTypeAST        = "ast"
	EvidenceTypeDocLLM     = "doc-llm"
	EvidenceTypePDFLLM     = "pdf-llm"
	EvidenceTypeFilesystem = "filesystem"
	EvidenceTypeProject    = "project"
)

// stampRepoNode writes repo-layer + evidence metadata onto a node in place.
// It preserves any existing Metadata entries.
func stampRepoNode(n *types.Node, evType string, confidence types.Confidence, sourceArtifact string) {
	if n.Metadata == nil {
		n.Metadata = map[string]interface{}{}
	}
	n.Metadata[MetaLayer] = string(types.LayerRepo)
	n.Metadata[MetaEvidenceType] = evType
	n.Metadata[MetaEvidenceConfidence] = string(confidence)
	if sourceArtifact != "" {
		n.Metadata[MetaEvidenceSourceArtifact] = sourceArtifact
	}
}

// stampRepoEdge writes repo-layer + evidence metadata onto an edge in place.
// It preserves any existing Metadata entries and does not override the edge's
// own Confidence string, which is part of the legacy wire format.
func stampRepoEdge(e *types.Edge, evType string, confidence types.Confidence, sourceArtifact string) {
	if e.Metadata == nil {
		e.Metadata = map[string]interface{}{}
	}
	e.Metadata[MetaLayer] = string(types.LayerRepo)
	e.Metadata[MetaEvidenceType] = evType
	e.Metadata[MetaEvidenceConfidence] = string(confidence)
	if sourceArtifact != "" {
		e.Metadata[MetaEvidenceSourceArtifact] = sourceArtifact
	}
}

// evidenceForExt returns the evidence type + confidence that should be stamped
// onto output derived from a given file extension. Unknown extensions fall
// back to filesystem-level evidence.
func evidenceForExt(ext string) (string, types.Confidence) {
	switch ext {
	case ".go", ".py", ".ts", ".tsx", ".js", ".jsx":
		return EvidenceTypeAST, types.ConfidenceExtracted
	case ".md", ".txt", ".pdf":
		return EvidenceTypeFilesystem, types.ConfidenceInferred
	}
	return EvidenceTypeFilesystem, types.ConfidenceInferred
}
