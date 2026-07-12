package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// ErrDuplicateContent is returned when inserting a memory whose normalized
// text already exists (UNIQUE content_hash — ADR-003).
var ErrDuplicateContent = errors.New("store: duplicate memory content")

// ErrNotFound is returned when a record does not exist.
var ErrNotFound = errors.New("store: not found")

// Source is a memory's provenance (MIND_FORMAT.md §4.3).
type Source struct {
	Kind      string  `json:"kind"` // "session" | "manual" | "import"
	SessionID *string `json:"session_id"`
	Detail    *string `json:"detail"`
}

// Memory is one approved memory (MIND_FORMAT.md §4.1) plus the embedding
// cache columns of the working store (DAEMON_DESIGN.md §5).
type Memory struct {
	ID             string
	ContentHash    string
	Type           string
	Text           string
	Tags           []string
	Privacy        string
	Source         Source
	CreatedAt      time.Time
	UpdatedAt      time.Time
	Embedding      []float32 // nil until computed; cache, not truth
	EmbeddingModel string
}

// MergeOutcome reports what an import merge did with one memory
// (MIND_FORMAT.md §7).
type MergeOutcome string

const (
	MergeInserted MergeOutcome = "inserted"
	MergeUpdated  MergeOutcome = "updated"
	MergeSkipped  MergeOutcome = "skipped"
)

func validateMemory(m *Memory) error {
	switch m.Type {
	case TypeFact, TypeEpisode, TypeProcedure:
	default:
		return fmt.Errorf("store: invalid memory type %q", m.Type)
	}
	switch m.Privacy {
	case PrivacyPublic, PrivacyPrivate:
	default:
		return fmt.Errorf("store: invalid privacy %q", m.Privacy)
	}
	if NormalizeText(m.Text) == "" {
		return errors.New("store: memory text is empty")
	}
	return nil
}

// normalizeTags lowercases, trims, dedups, and sorts tags.
func normalizeTags(tags []string) []string {
	seen := make(map[string]bool, len(tags))
	out := make([]string, 0, len(tags))
	for _, t := range tags {
		t = strings.ToLower(strings.TrimSpace(t))
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}

func formatTime(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }

func parseTime(s string) (time.Time, error) { return time.Parse(time.RFC3339Nano, s) }

// prepare fills defaults and derived fields. The content hash is always
// recomputed locally from the text — imported hashes are untrusted input.
func prepare(m *Memory) error {
	if err := validateMemory(m); err != nil {
		return err
	}
	if m.ID == "" {
		m.ID = NewID()
	}
	m.ContentHash = ContentHash(m.Text)
	m.Tags = normalizeTags(m.Tags)
	if m.Source.Kind == "" {
		m.Source.Kind = "manual"
	}
	now := time.Now().UTC()
	if m.CreatedAt.IsZero() {
		m.CreatedAt = now
	}
	if m.UpdatedAt.IsZero() {
		m.UpdatedAt = m.CreatedAt
	}
	return nil
}

func isUniqueHashErr(err error) bool {
	return err != nil && strings.Contains(err.Error(), "memories.content_hash")
}

// InsertMemory inserts a new memory, filling ID, hash, and timestamps as
// needed. Returns ErrDuplicateContent if the normalized text already exists.
func (s *Store) InsertMemory(m *Memory) error {
	if err := prepare(m); err != nil {
		return err
	}
	tags, _ := json.Marshal(m.Tags)
	src, _ := json.Marshal(m.Source)
	var blob []byte
	var model sql.NullString
	if m.Embedding != nil {
		blob = EncodeVector(m.Embedding)
		model = sql.NullString{String: m.EmbeddingModel, Valid: true}
	}
	_, err := s.db.Exec(`
		INSERT INTO memories (id, content_hash, type, text, tags, privacy, source,
		                      created_at, updated_at, embedding, embedding_model)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		m.ID, m.ContentHash, m.Type, m.Text, string(tags), m.Privacy, string(src),
		formatTime(m.CreatedAt), formatTime(m.UpdatedAt), blob, model)
	if isUniqueHashErr(err) {
		return fmt.Errorf("%w: %s", ErrDuplicateContent, m.ContentHash)
	}
	if err != nil {
		return fmt.Errorf("store: insert memory: %w", err)
	}
	s.indexSet(m)
	return nil
}

// UpdateMemory rewrites a memory's mutable fields by ID. The hash is
// recomputed; if the text changed and no new embedding is provided, the
// stale embedding is cleared (text is the source of truth, ADR-002).
func (s *Store) UpdateMemory(m *Memory) error {
	if m.ID == "" {
		return errors.New("store: update requires an ID")
	}
	old, err := s.GetMemoryByID(m.ID)
	if err != nil {
		return err
	}
	if err := validateMemory(m); err != nil {
		return err
	}
	m.ContentHash = ContentHash(m.Text)
	m.Tags = normalizeTags(m.Tags)
	if m.CreatedAt.IsZero() {
		m.CreatedAt = old.CreatedAt
	}
	if m.UpdatedAt.IsZero() || !m.UpdatedAt.After(old.UpdatedAt) {
		m.UpdatedAt = time.Now().UTC()
	}
	if m.ContentHash != old.ContentHash && m.Embedding == nil {
		m.EmbeddingModel = ""
	} else if m.Embedding == nil {
		m.Embedding, m.EmbeddingModel = old.Embedding, old.EmbeddingModel
	}
	tags, _ := json.Marshal(m.Tags)
	src, _ := json.Marshal(m.Source)
	var blob []byte
	var model sql.NullString
	if m.Embedding != nil {
		blob = EncodeVector(m.Embedding)
		model = sql.NullString{String: m.EmbeddingModel, Valid: true}
	}
	_, err = s.db.Exec(`
		UPDATE memories SET content_hash=?, type=?, text=?, tags=?, privacy=?, source=?,
		       created_at=?, updated_at=?, embedding=?, embedding_model=?
		WHERE id=?`,
		m.ContentHash, m.Type, m.Text, string(tags), m.Privacy, string(src),
		formatTime(m.CreatedAt), formatTime(m.UpdatedAt), blob, model, m.ID)
	if isUniqueHashErr(err) {
		return fmt.Errorf("%w: %s", ErrDuplicateContent, m.ContentHash)
	}
	if err != nil {
		return fmt.Errorf("store: update memory: %w", err)
	}
	s.indexSet(m)
	return nil
}

// DeleteMemory removes a memory by ID.
func (s *Store) DeleteMemory(id string) error {
	res, err := s.db.Exec(`DELETE FROM memories WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("store: delete memory: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	s.indexDelete(id)
	return nil
}

// SetEmbedding stores the embedding cache for a memory and updates the index.
func (s *Store) SetEmbedding(id, model string, vec []float32) error {
	res, err := s.db.Exec(`UPDATE memories SET embedding=?, embedding_model=? WHERE id=?`,
		EncodeVector(vec), model, id)
	if err != nil {
		return fmt.Errorf("store: set embedding: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	m, err := s.GetMemoryByID(id)
	if err != nil {
		return err
	}
	s.indexSet(m)
	return nil
}

const memoryCols = `id, content_hash, type, text, tags, privacy, source,
	created_at, updated_at, embedding, embedding_model`

func scanMemory(row interface{ Scan(...any) error }) (*Memory, error) {
	var m Memory
	var tags, src, created, updated string
	var blob []byte
	var model sql.NullString
	err := row.Scan(&m.ID, &m.ContentHash, &m.Type, &m.Text, &tags, &m.Privacy, &src,
		&created, &updated, &blob, &model)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: scan memory: %w", err)
	}
	if err := json.Unmarshal([]byte(tags), &m.Tags); err != nil {
		return nil, fmt.Errorf("store: memory %s: bad tags: %w", m.ID, err)
	}
	if err := json.Unmarshal([]byte(src), &m.Source); err != nil {
		return nil, fmt.Errorf("store: memory %s: bad source: %w", m.ID, err)
	}
	if m.CreatedAt, err = parseTime(created); err != nil {
		return nil, fmt.Errorf("store: memory %s: bad created_at: %w", m.ID, err)
	}
	if m.UpdatedAt, err = parseTime(updated); err != nil {
		return nil, fmt.Errorf("store: memory %s: bad updated_at: %w", m.ID, err)
	}
	if blob != nil {
		if m.Embedding, err = DecodeVector(blob); err != nil {
			return nil, fmt.Errorf("store: memory %s: %w", m.ID, err)
		}
		m.EmbeddingModel = model.String
	}
	return &m, nil
}

// GetMemoryByID fetches one memory. Returns ErrNotFound if absent.
func (s *Store) GetMemoryByID(id string) (*Memory, error) {
	return scanMemory(s.db.QueryRow(`SELECT `+memoryCols+` FROM memories WHERE id=?`, id))
}

// GetMemoryByHash fetches one memory by content hash. Returns ErrNotFound if absent.
func (s *Store) GetMemoryByHash(hash string) (*Memory, error) {
	return scanMemory(s.db.QueryRow(`SELECT `+memoryCols+` FROM memories WHERE content_hash=?`, hash))
}

// ListFilter narrows ListMemories. Zero values mean "no filter".
type ListFilter struct {
	Type    string
	Privacy string
}

// ListMemories returns memories matching the filter, newest first.
func (s *Store) ListMemories(f ListFilter) ([]*Memory, error) {
	q := `SELECT ` + memoryCols + ` FROM memories WHERE 1=1`
	var args []any
	if f.Type != "" {
		q += ` AND type = ?`
		args = append(args, f.Type)
	}
	if f.Privacy != "" {
		q += ` AND privacy = ?`
		args = append(args, f.Privacy)
	}
	q += ` ORDER BY created_at DESC, id DESC`
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("store: list memories: %w", err)
	}
	defer rows.Close()
	var out []*Memory
	for rows.Next() {
		m, err := scanMemory(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// CountMemories returns the number of memories per type.
func (s *Store) CountMemories() (map[string]int, error) {
	rows, err := s.db.Query(`SELECT type, COUNT(*) FROM memories GROUP BY type`)
	if err != nil {
		return nil, fmt.Errorf("store: count memories: %w", err)
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var t string
		var n int
		if err := rows.Scan(&t, &n); err != nil {
			return nil, err
		}
		out[t] = n
	}
	return out, rows.Err()
}

// Merge applies the import merge rules of MIND_FORMAT.md §7 to one incoming
// memory: dedup by content hash first, then match by UUID (newer updated_at
// wins, tie keeps local), else insert.
func (s *Store) Merge(m Memory) (MergeOutcome, error) {
	if err := prepare(&m); err != nil {
		return "", err
	}
	// Rule 1 — DEDUP: identical content never duplicates, regardless of IDs.
	if _, err := s.GetMemoryByHash(m.ContentHash); err == nil {
		return MergeSkipped, nil
	} else if !errors.Is(err, ErrNotFound) {
		return "", err
	}
	// Rule 2 — UPDATE: same memory, edited elsewhere.
	local, err := s.GetMemoryByID(m.ID)
	switch {
	case err == nil:
		if !m.UpdatedAt.After(local.UpdatedAt) { // tie → keep local
			return MergeSkipped, nil
		}
		m.CreatedAt = local.CreatedAt
		if err := s.UpdateMemory(&m); err != nil {
			return "", err
		}
		return MergeUpdated, nil
	case errors.Is(err, ErrNotFound):
		// Rule 3 — INSERT.
		if err := s.InsertMemory(&m); err != nil {
			return "", err
		}
		return MergeInserted, nil
	default:
		return "", err
	}
}
