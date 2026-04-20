package setup

import "fmt"

// LocalSearchEmbeddingModel is the local embedding model Vela expects when it
// needs to materialize the retrieval substrate for search.
const LocalSearchEmbeddingModel = "nomic-embed-text"

// RecommendedModel represents a tested Ollama model with compatibility metadata
type RecommendedModel struct {
	Name              string
	DisplayName       string
	Description       string
	Size              string
	ParameterCount    string
	QuantizationLevel string // Q4_K_M, Q5_K_M, etc.

	// Syfra/Ancora ecosystem compatibility
	SyfraCompatible   bool     // Works with Syfra ecosystem
	AncoraEmbedCompat bool     // Compatible with Ancora's embedding approach
	EmbedDimensions   int      // Embedding dimensions (768 for nomic-embed-text)
	RecommendedFor    []string // ["extraction", "reasoning", "embedding"]

	// Performance characteristics
	MinRAM             string // "4GB", "8GB", "16GB"
	AvgTokensPerSecond int    // Approximate on consumer hardware
	ContextWindow      int    // Token context window

	// Model-specific notes
	Notes         string
	SyfraVerified bool // Tested and verified by Syfra team
}

// GetRecommendedModels returns curated list of models for Vela/Syfra ecosystem
func GetRecommendedModels() []RecommendedModel {
	return []RecommendedModel{
		// TIER 1: Syfra-verified models (tested in production)
		{
			Name:               "llama3.2",
			DisplayName:        "Llama 3.2 (3B)",
			Description:        "Fast, efficient model for knowledge extraction",
			Size:               "2GB",
			ParameterCount:     "3B",
			QuantizationLevel:  "Q4_K_M",
			SyfraCompatible:    true,
			AncoraEmbedCompat:  false, // Different embedding approach than nomic
			EmbedDimensions:    4096,  // Llama's internal dim
			RecommendedFor:     []string{"extraction", "classification"},
			MinRAM:             "4GB",
			AvgTokensPerSecond: 40,
			ContextWindow:      8192,
			Notes:              "Best for resource-constrained environments. Fast extraction.",
			SyfraVerified:      true,
		},
		{
			Name:               "llama3",
			DisplayName:        "Llama 3 (8B)",
			Description:        "Balanced performance for graph extraction",
			Size:               "4.7GB",
			ParameterCount:     "8B",
			QuantizationLevel:  "Q4_K_M",
			SyfraCompatible:    true,
			AncoraEmbedCompat:  false,
			EmbedDimensions:    4096,
			RecommendedFor:     []string{"extraction", "reasoning", "summarization"},
			MinRAM:             "8GB",
			AvgTokensPerSecond: 25,
			ContextWindow:      8192,
			Notes:              "Recommended default. Good balance of speed and quality.",
			SyfraVerified:      true,
		},
		{
			Name:               "mistral",
			DisplayName:        "Mistral 7B",
			Description:        "High-quality extraction and reasoning",
			Size:               "4.1GB",
			ParameterCount:     "7B",
			QuantizationLevel:  "Q4_K_M",
			SyfraCompatible:    true,
			AncoraEmbedCompat:  false,
			EmbedDimensions:    4096,
			RecommendedFor:     []string{"extraction", "reasoning"},
			MinRAM:             "8GB",
			AvgTokensPerSecond: 30,
			ContextWindow:      8192,
			Notes:              "Excellent for technical documentation extraction.",
			SyfraVerified:      true,
		},

		// TIER 2: Community-tested (compatible but not Syfra-verified)
		{
			Name:               "phi3",
			DisplayName:        "Phi-3 Mini",
			Description:        "Efficient small model for fast extraction",
			Size:               "2.3GB",
			ParameterCount:     "3.8B",
			QuantizationLevel:  "Q4_K_M",
			SyfraCompatible:    true,
			AncoraEmbedCompat:  false,
			EmbedDimensions:    3072,
			RecommendedFor:     []string{"extraction"},
			MinRAM:             "4GB",
			AvgTokensPerSecond: 50,
			ContextWindow:      4096,
			Notes:              "Very fast, good for large codebases.",
			SyfraVerified:      false,
		},
		{
			Name:               "qwen2.5",
			DisplayName:        "Qwen 2.5 (7B)",
			Description:        "Strong coding and technical reasoning",
			Size:               "4.7GB",
			ParameterCount:     "7B",
			QuantizationLevel:  "Q4_K_M",
			SyfraCompatible:    true,
			AncoraEmbedCompat:  false,
			EmbedDimensions:    3584,
			RecommendedFor:     []string{"extraction", "coding"},
			MinRAM:             "8GB",
			AvgTokensPerSecond: 28,
			ContextWindow:      32768,
			Notes:              "Large context window, excellent for long files.",
			SyfraVerified:      false,
		},

		// TIER 3: Specialized models
		{
			Name:               "codellama",
			DisplayName:        "CodeLlama 7B",
			Description:        "Specialized for code understanding",
			Size:               "3.8GB",
			ParameterCount:     "7B",
			QuantizationLevel:  "Q4_K_M",
			SyfraCompatible:    true,
			AncoraEmbedCompat:  false,
			EmbedDimensions:    4096,
			RecommendedFor:     []string{"extraction"},
			MinRAM:             "8GB",
			AvgTokensPerSecond: 30,
			ContextWindow:      16384,
			Notes:              "Best for pure code extraction (no docs).",
			SyfraVerified:      false,
		},
	}
}

// GetEmbeddingCompatibleModels returns the embedding model Vela expects for
// local retrieval/search setup.
func GetEmbeddingCompatibleModels() []RecommendedModel {
	return []RecommendedModel{
		{
			Name:              LocalSearchEmbeddingModel,
			DisplayName:       "Nomic Embed Text (for Ancora)",
			Description:       "768-dim embeddings for semantic search (Ancora integration)",
			Size:              "270MB",
			ParameterCount:    "137M",
			QuantizationLevel: "Q4_K_M",
			SyfraCompatible:   true,
			AncoraEmbedCompat: true,
			EmbedDimensions:   768,
			RecommendedFor:    []string{"embedding"},
			MinRAM:            "2GB",
			Notes:             "This is NOT an Ollama model. Installed via Ancora setup wizard.",
			SyfraVerified:     true,
		},
	}
}

// FilterByRAM returns models that fit in available RAM
func FilterByRAM(models []RecommendedModel, availableGB int) []RecommendedModel {
	var filtered []RecommendedModel
	for _, m := range models {
		// Parse MinRAM (e.g., "8GB" → 8)
		var required int
		fmt.Sscanf(m.MinRAM, "%dGB", &required)
		if required <= availableGB {
			filtered = append(filtered, m)
		}
	}
	return filtered
}

// FilterBySyfraVerified returns only Syfra-tested models
func FilterBySyfraVerified(models []RecommendedModel) []RecommendedModel {
	var filtered []RecommendedModel
	for _, m := range models {
		if m.SyfraVerified {
			filtered = append(filtered, m)
		}
	}
	return filtered
}

// GetDefaultModel returns the recommended default (llama3)
func GetDefaultModel() RecommendedModel {
	models := GetRecommendedModels()
	for _, m := range models {
		if m.Name == "llama3" {
			return m
		}
	}
	return models[0] // Fallback to first
}

// FormatModelChoice formats a model for display in TUI
func (m RecommendedModel) FormatModelChoice() string {
	verified := ""
	if m.SyfraVerified {
		verified = " ✓"
	}
	return fmt.Sprintf("%s (%s, %s)%s - %s",
		m.DisplayName, m.Size, m.MinRAM, verified, m.Description)
}
