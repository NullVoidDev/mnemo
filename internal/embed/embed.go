// Package embed generates local, CPU-only text embeddings used for memory
// retrieval (DAEMON_DESIGN.md §6).
package embed

import "context"

// Embedder produces L2-normalized embedding vectors for texts
// (DAEMON_DESIGN.md §6.2). Implementations: ONNX (default, in-process),
// Service (optional HTTP microservice, ADR-004), Mock (tests).
type Embedder interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
	Model() string   // e.g. "all-MiniLM-L6-v2"
	Dimensions() int // e.g. 384
}
