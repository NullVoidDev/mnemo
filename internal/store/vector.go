package store

import (
	"encoding/binary"
	"fmt"
	"math"
)

// EncodeVector serializes a float32 vector as little-endian bytes — the
// layout shared by the embedding BLOB column and embeddings.bin in .mind
// files (MIND_FORMAT.md §5).
func EncodeVector(v []float32) []byte {
	b := make([]byte, 4*len(v))
	for i, f := range v {
		binary.LittleEndian.PutUint32(b[4*i:], math.Float32bits(f))
	}
	return b
}

// DecodeVector parses little-endian float32 bytes back into a vector.
func DecodeVector(b []byte) ([]float32, error) {
	if len(b)%4 != 0 {
		return nil, fmt.Errorf("store: vector blob length %d is not a multiple of 4", len(b))
	}
	v := make([]float32, len(b)/4)
	for i := range v {
		v[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[4*i:]))
	}
	return v, nil
}

// dot returns the dot product of two vectors. Embeddings are L2-normalized
// by the embedder, so this equals cosine similarity (DAEMON_DESIGN.md §6.3).
// Mismatched lengths score 0 (never comparable).
func dot(a, b []float32) float32 {
	if len(a) != len(b) {
		return 0
	}
	var sum float32
	for i := range a {
		sum += a[i] * b[i]
	}
	return sum
}
