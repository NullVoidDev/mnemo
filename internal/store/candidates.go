package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Candidate is an extracted memory candidate awaiting human review
// (DAEMON_DESIGN.md §7). Candidates never become memories without explicit
// approval, and are never exported to .mind files.
type Candidate struct {
	ID        string
	Type      string
	Text      string
	Tags      []string
	SessionID string // originating session ("" if none)
	CreatedAt time.Time
	Status    string
}

// InsertCandidate stores a new pending candidate, filling ID and timestamp
// as needed.
func (s *Store) InsertCandidate(c *Candidate) error {
	switch c.Type {
	case TypeFact, TypeEpisode, TypeProcedure:
	default:
		return fmt.Errorf("store: invalid candidate type %q", c.Type)
	}
	if NormalizeText(c.Text) == "" {
		return errors.New("store: candidate text is empty")
	}
	if c.ID == "" {
		c.ID = NewID()
	}
	if c.CreatedAt.IsZero() {
		c.CreatedAt = time.Now().UTC()
	}
	if c.Status == "" {
		c.Status = StatusPending
	}
	c.Tags = normalizeTags(c.Tags)
	tags, _ := json.Marshal(c.Tags)
	var sess sql.NullString
	if c.SessionID != "" {
		sess = sql.NullString{String: c.SessionID, Valid: true}
	}
	_, err := s.db.Exec(`
		INSERT INTO candidates (id, type, text, tags, session_id, created_at, status)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		c.ID, c.Type, c.Text, string(tags), sess, formatTime(c.CreatedAt), c.Status)
	if err != nil {
		return fmt.Errorf("store: insert candidate: %w", err)
	}
	return nil
}

func scanCandidate(row interface{ Scan(...any) error }) (*Candidate, error) {
	var c Candidate
	var tags, created string
	var sess sql.NullString
	err := row.Scan(&c.ID, &c.Type, &c.Text, &tags, &sess, &created, &c.Status)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: scan candidate: %w", err)
	}
	if err := json.Unmarshal([]byte(tags), &c.Tags); err != nil {
		return nil, fmt.Errorf("store: candidate %s: bad tags: %w", c.ID, err)
	}
	c.SessionID = sess.String
	if c.CreatedAt, err = parseTime(created); err != nil {
		return nil, fmt.Errorf("store: candidate %s: bad created_at: %w", c.ID, err)
	}
	return &c, nil
}

const candidateCols = `id, type, text, tags, session_id, created_at, status`

// GetCandidate fetches one candidate. Returns ErrNotFound if absent.
func (s *Store) GetCandidate(id string) (*Candidate, error) {
	return scanCandidate(s.db.QueryRow(`SELECT `+candidateCols+` FROM candidates WHERE id=?`, id))
}

// ListCandidates returns candidates with the given status ("" = all),
// oldest first (review order).
func (s *Store) ListCandidates(status string) ([]*Candidate, error) {
	q := `SELECT ` + candidateCols + ` FROM candidates`
	var args []any
	if status != "" {
		q += ` WHERE status = ?`
		args = append(args, status)
	}
	q += ` ORDER BY created_at ASC, id ASC`
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("store: list candidates: %w", err)
	}
	defer rows.Close()
	var out []*Candidate
	for rows.Next() {
		c, err := scanCandidate(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// SetCandidateStatus marks a candidate approved or rejected.
func (s *Store) SetCandidateStatus(id, status string) error {
	switch status {
	case StatusPending, StatusApproved, StatusRejected:
	default:
		return fmt.Errorf("store: invalid candidate status %q", status)
	}
	res, err := s.db.Exec(`UPDATE candidates SET status=? WHERE id=?`, status, id)
	if err != nil {
		return fmt.Errorf("store: set candidate status: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// HasPendingCandidateWithText reports whether a pending candidate with the
// same normalized text already exists (extraction dedup, DAEMON_DESIGN.md §7).
func (s *Store) HasPendingCandidateWithText(text string) (bool, error) {
	target := ContentHash(text)
	pending, err := s.ListCandidates(StatusPending)
	if err != nil {
		return false, err
	}
	for _, c := range pending {
		if ContentHash(c.Text) == target {
			return true, nil
		}
	}
	return false, nil
}
