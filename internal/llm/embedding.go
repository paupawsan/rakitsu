package llm

import "context"

// EmbeddingProvider generates vector embeddings for text.
// Returned by createEmbeddingProvider(); passed to agent.NewRetriever().
type EmbeddingProvider interface {
	Embed(ctx context.Context, text string) ([]float32, error)
	GetName() string
}
