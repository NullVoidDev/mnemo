package store

import (
	"database/sql"
	"fmt"
	"time"
)

// Session groups proxy turns for later candidate extraction
// (DAEMON_DESIGN.md §3.4, §7).
type Session struct {
	ID             string
	StartedAt      time.Time
	LastActivityAt time.Time
	ClosedAt       *time.Time // nil while open
	Extracted      bool
}

// TouchSession creates the session if missing, or bumps last_activity_at if
// open. Returns the session ID (generated when id is ""). Touching a closed
// session is an error — the caller decides how to key a new one.
func (s *Store) TouchSession(id string) (string, error) {
	if id == "" {
		id = NewID()
	}
	now := formatTime(time.Now().UTC())
	sess, err := s.GetSession(id)
	switch err {
	case nil:
		if sess.ClosedAt != nil {
			return "", fmt.Errorf("store: session %s is closed", id)
		}
		if _, err := s.db.Exec(`UPDATE sessions SET last_activity_at=? WHERE id=?`, now, id); err != nil {
			return "", fmt.Errorf("store: touch session: %w", err)
		}
		return id, nil
	case ErrNotFound:
		if _, err := s.db.Exec(
			`INSERT INTO sessions (id, started_at, last_activity_at) VALUES (?, ?, ?)`,
			id, now, now); err != nil {
			return "", fmt.Errorf("store: create session: %w", err)
		}
		return id, nil
	default:
		return "", err
	}
}

func scanSession(row interface{ Scan(...any) error }) (*Session, error) {
	var sess Session
	var started, activity string
	var closed sql.NullString
	var extracted int
	err := row.Scan(&sess.ID, &started, &activity, &closed, &extracted)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: scan session: %w", err)
	}
	if sess.StartedAt, err = parseTime(started); err != nil {
		return nil, fmt.Errorf("store: session %s: bad started_at: %w", sess.ID, err)
	}
	if sess.LastActivityAt, err = parseTime(activity); err != nil {
		return nil, fmt.Errorf("store: session %s: bad last_activity_at: %w", sess.ID, err)
	}
	if closed.Valid {
		t, err := parseTime(closed.String)
		if err != nil {
			return nil, fmt.Errorf("store: session %s: bad closed_at: %w", sess.ID, err)
		}
		sess.ClosedAt = &t
	}
	sess.Extracted = extracted != 0
	return &sess, nil
}

const sessionCols = `id, started_at, last_activity_at, closed_at, extracted`

// GetSession fetches one session. Returns ErrNotFound if absent.
func (s *Store) GetSession(id string) (*Session, error) {
	return scanSession(s.db.QueryRow(`SELECT `+sessionCols+` FROM sessions WHERE id=?`, id))
}

// CloseSession marks an open session closed (idempotent on already-closed).
func (s *Store) CloseSession(id string) error {
	res, err := s.db.Exec(
		`UPDATE sessions SET closed_at=? WHERE id=? AND closed_at IS NULL`,
		formatTime(time.Now().UTC()), id)
	if err != nil {
		return fmt.Errorf("store: close session: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		if _, err := s.GetSession(id); err != nil {
			return err
		}
	}
	return nil
}

// CloseIdleSessions closes every open session whose last activity is before
// the cutoff. Returns how many were closed.
func (s *Store) CloseIdleSessions(cutoff time.Time) (int, error) {
	res, err := s.db.Exec(
		`UPDATE sessions SET closed_at=? WHERE closed_at IS NULL AND last_activity_at < ?`,
		formatTime(time.Now().UTC()), formatTime(cutoff))
	if err != nil {
		return 0, fmt.Errorf("store: close idle sessions: %w", err)
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// SessionsToExtract returns closed, not-yet-extracted sessions, oldest first.
func (s *Store) SessionsToExtract() ([]*Session, error) {
	rows, err := s.db.Query(`SELECT ` + sessionCols + ` FROM sessions
		WHERE closed_at IS NOT NULL AND extracted = 0 ORDER BY closed_at ASC`)
	if err != nil {
		return nil, fmt.Errorf("store: sessions to extract: %w", err)
	}
	defer rows.Close()
	var out []*Session
	for rows.Next() {
		sess, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sess)
	}
	return out, rows.Err()
}

// MarkSessionExtracted flags a session as processed by extraction.
func (s *Store) MarkSessionExtracted(id string) error {
	res, err := s.db.Exec(`UPDATE sessions SET extracted=1 WHERE id=?`, id)
	if err != nil {
		return fmt.Errorf("store: mark extracted: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// CountSessions returns (open, total).
func (s *Store) CountSessions() (open, total int, err error) {
	err = s.db.QueryRow(`SELECT COUNT(*), COUNT(*) FILTER (WHERE closed_at IS NULL) FROM sessions`).
		Scan(&total, &open)
	if err != nil {
		return 0, 0, fmt.Errorf("store: count sessions: %w", err)
	}
	return open, total, nil
}

// Message is one logged proxy turn.
type Message struct {
	ID        int64
	SessionID string
	Role      string // "user" | "assistant" | "system"
	Content   string
	CreatedAt time.Time
}

// AppendMessage logs one turn into a session.
func (s *Store) AppendMessage(sessionID, role, content string) error {
	_, err := s.db.Exec(
		`INSERT INTO messages (session_id, role, content, created_at) VALUES (?, ?, ?, ?)`,
		sessionID, role, content, formatTime(time.Now().UTC()))
	if err != nil {
		return fmt.Errorf("store: append message: %w", err)
	}
	return nil
}

// SessionMessages returns a session's messages in insertion order.
func (s *Store) SessionMessages(sessionID string) ([]*Message, error) {
	rows, err := s.db.Query(
		`SELECT id, session_id, role, content, created_at FROM messages
		 WHERE session_id = ? ORDER BY id ASC`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("store: session messages: %w", err)
	}
	defer rows.Close()
	var out []*Message
	for rows.Next() {
		var m Message
		var created string
		if err := rows.Scan(&m.ID, &m.SessionID, &m.Role, &m.Content, &created); err != nil {
			return nil, fmt.Errorf("store: scan message: %w", err)
		}
		if m.CreatedAt, err = parseTime(created); err != nil {
			return nil, fmt.Errorf("store: message %d: bad created_at: %w", m.ID, err)
		}
		out = append(out, &m)
	}
	return out, rows.Err()
}
