package store

import (
	"fmt"
	"sort"
)

// indexEntry is one memory in the in-memory brute-force vector index
// (DAEMON_DESIGN.md §6.3: at personal-store scale a linear scan is noise
// next to LLM generation time).
type indexEntry struct {
	vec []float32
	typ string
}

// loadIndex (re)builds the index from the database. When model is non-empty,
// only embeddings produced by that model are indexed — others are stale
// cache awaiting re-embedding (ADR-002).
func (s *Store) loadIndex(model string) error {
	q := `SELECT id, type, embedding FROM memories WHERE embedding IS NOT NULL`
	var args []any
	if model != "" {
		q += ` AND embedding_model = ?`
		args = append(args, model)
	}
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return fmt.Errorf("store: load index: %w", err)
	}
	defer rows.Close()

	idx := make(map[string]indexEntry)
	for rows.Next() {
		var id, typ string
		var blob []byte
		if err := rows.Scan(&id, &typ, &blob); err != nil {
			return fmt.Errorf("store: load index: %w", err)
		}
		vec, err := DecodeVector(blob)
		if err != nil {
			return fmt.Errorf("store: memory %s: %w", id, err)
		}
		idx[id] = indexEntry{vec: vec, typ: typ}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	s.index = idx
	s.mu.Unlock()
	return nil
}

// RebuildIndex reloads the vector index, keeping only embeddings produced by
// the given model ("" = all).
func (s *Store) RebuildIndex(model string) error { return s.loadIndex(model) }

func (s *Store) indexSet(m *Memory) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if m.Embedding == nil {
		delete(s.index, m.ID)
		return
	}
	s.index[m.ID] = indexEntry{vec: m.Embedding, typ: m.Type}
}

func (s *Store) indexDelete(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.index, id)
}

// IndexSize returns how many memories currently have indexed embeddings.
func (s *Store) IndexSize() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.index)
}

// SearchOptions narrows a vector search.
type SearchOptions struct {
	K             int      // max results (0 = no limit)
	MinSimilarity float32  // drop hits below this cosine similarity
	Types         []string // keep only these memory types (empty = all)
}

// SearchResult is one retrieval hit.
type SearchResult struct {
	Memory *Memory
	Score  float32
}

// Search runs a brute-force cosine-similarity scan over the index. Vectors
// are assumed L2-normalized (cosine = dot). Results are sorted by score
// descending; ties break by memory ID for determinism.
func (s *Store) Search(query []float32, opts SearchOptions) ([]SearchResult, error) {
	typeOK := func(string) bool { return true }
	if len(opts.Types) > 0 {
		set := make(map[string]bool, len(opts.Types))
		for _, t := range opts.Types {
			set[t] = true
		}
		typeOK = func(t string) bool { return set[t] }
	}

	type hit struct {
		id    string
		score float32
	}
	s.mu.RLock()
	hits := make([]hit, 0, len(s.index))
	for id, e := range s.index {
		if !typeOK(e.typ) {
			continue
		}
		if score := dot(query, e.vec); score >= opts.MinSimilarity {
			hits = append(hits, hit{id, score})
		}
	}
	s.mu.RUnlock()

	sort.Slice(hits, func(i, j int) bool {
		if hits[i].score != hits[j].score {
			return hits[i].score > hits[j].score
		}
		return hits[i].id < hits[j].id
	})
	if opts.K > 0 && len(hits) > opts.K {
		hits = hits[:opts.K]
	}

	out := make([]SearchResult, 0, len(hits))
	for _, h := range hits {
		m, err := s.GetMemoryByID(h.id)
		if err != nil {
			return nil, err
		}
		out = append(out, SearchResult{Memory: m, Score: h.score})
	}
	return out, nil
}
