package embed

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"math"
	"sync/atomic"
)

// Mock is a deterministic, dependency-free Embedder for tests: the vector is
// derived from a hash of the text, so equal texts embed equally and
// different texts (almost surely) differ. It counts Embed calls so tests can
// assert whether regeneration happened (Phase 3 import tests).
type Mock struct {
	Name  string // reported by Model()
	Dims  int    // reported by Dimensions()
	calls atomic.Int64
}

// NewMock returns a Mock embedder with the given model name and dimensions.
func NewMock(name string, dims int) *Mock { return &Mock{Name: name, Dims: dims} }

// Embed derives one pseudo-random unit vector per text.
func (m *Mock) Embed(_ context.Context, texts []string) ([][]float32, error) {
	m.calls.Add(1)
	out := make([][]float32, len(texts))
	for i, t := range texts {
		out[i] = m.vector(t)
	}
	return out, nil
}

func (m *Mock) vector(text string) []float32 {
	v := make([]float32, m.Dims)
	seed := sha256.Sum256([]byte(text))
	var norm float64
	for i := range v {
		// Stretch the seed by hashing a counter alongside it.
		var buf [40]byte
		copy(buf[:32], seed[:])
		binary.LittleEndian.PutUint64(buf[32:], uint64(i))
		h := sha256.Sum256(buf[:])
		u := binary.LittleEndian.Uint32(h[:4])
		f := float64(u)/float64(math.MaxUint32)*2 - 1 // [-1, 1]
		v[i] = float32(f)
		norm += f * f
	}
	n := float32(math.Sqrt(norm))
	for i := range v {
		v[i] /= n
	}
	return v
}

// Model implements Embedder.
func (m *Mock) Model() string { return m.Name }

// Dimensions implements Embedder.
func (m *Mock) Dimensions() int { return m.Dims }

// Calls reports how many times Embed was invoked.
func (m *Mock) Calls() int { return int(m.calls.Load()) }
