package store

import (
	"database/sql"
	"fmt"
	"sync"

	"github.com/google/uuid"
	_ "modernc.org/sqlite" // pure-Go SQLite driver
)

// Memory types (MIND_FORMAT.md §4.1).
const (
	TypeFact      = "fact"
	TypeEpisode   = "episode"
	TypeProcedure = "procedure"
)

// Privacy levels (MIND_FORMAT.md §6).
const (
	PrivacyPublic  = "public"
	PrivacyPrivate = "private"
)

// Candidate statuses (DAEMON_DESIGN.md §5).
const (
	StatusPending  = "pending"
	StatusApproved = "approved"
	StatusRejected = "rejected"
)

// Store is the daemon's working storage: SQLite for persistence plus an
// in-memory brute-force vector index for retrieval (DAEMON_DESIGN.md §6.3).
type Store struct {
	db *sql.DB

	mu    sync.RWMutex
	index map[string]indexEntry // memory id -> vector + type
}

// Open opens (or creates) the SQLite database at path, runs migrations, and
// loads the vector index into memory.
func Open(path string) (*Store, error) {
	dsn := "file:" + path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("store: open %s: %w", path, err)
	}
	// The daemon is the single writer; one connection avoids SQLITE_BUSY
	// surprises with the in-process driver.
	db.SetMaxOpenConns(1)

	if err := migrate(db); err != nil {
		db.Close()
		return nil, err
	}
	s := &Store{db: db, index: make(map[string]indexEntry)}
	if err := s.loadIndex(""); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// Close closes the underlying database.
func (s *Store) Close() error { return s.db.Close() }

// NewID returns a new UUIDv7 (time-ordered, no coordination — ADR-003).
func NewID() string { return uuid.Must(uuid.NewV7()).String() }

const schemaV1 = `
CREATE TABLE meta (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL
);

CREATE TABLE memories (
  id              TEXT PRIMARY KEY,
  content_hash    TEXT NOT NULL UNIQUE,
  type            TEXT NOT NULL CHECK (type IN ('fact','episode','procedure')),
  text            TEXT NOT NULL,
  tags            TEXT NOT NULL DEFAULT '[]',
  privacy         TEXT NOT NULL DEFAULT 'private' CHECK (privacy IN ('public','private')),
  source          TEXT NOT NULL,
  created_at      TEXT NOT NULL,
  updated_at      TEXT NOT NULL,
  embedding       BLOB,
  embedding_model TEXT
);
CREATE INDEX idx_memories_type    ON memories(type);
CREATE INDEX idx_memories_privacy ON memories(privacy);

CREATE TABLE candidates (
  id         TEXT PRIMARY KEY,
  type       TEXT NOT NULL CHECK (type IN ('fact','episode','procedure')),
  text       TEXT NOT NULL,
  tags       TEXT NOT NULL DEFAULT '[]',
  session_id TEXT,
  created_at TEXT NOT NULL,
  status     TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','approved','rejected'))
);
CREATE INDEX idx_candidates_status ON candidates(status);

CREATE TABLE sessions (
  id               TEXT PRIMARY KEY,
  started_at       TEXT NOT NULL,
  last_activity_at TEXT NOT NULL,
  closed_at        TEXT,
  extracted        INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE messages (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  session_id TEXT NOT NULL REFERENCES sessions(id),
  role       TEXT NOT NULL CHECK (role IN ('user','assistant','system')),
  content    TEXT NOT NULL,
  created_at TEXT NOT NULL
);
CREATE INDEX idx_messages_session ON messages(session_id);
`

// migrations[i] upgrades the schema from version i to i+1.
var migrations = []string{schemaV1}

func migrate(db *sql.DB) error {
	var version int
	err := db.QueryRow(`SELECT value FROM meta WHERE key = 'schema_version'`).Scan(&version)
	if err != nil {
		version = 0 // meta table missing: fresh database
	}
	for v := version; v < len(migrations); v++ {
		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("store: begin migration %d: %w", v+1, err)
		}
		if _, err := tx.Exec(migrations[v]); err != nil {
			tx.Rollback()
			return fmt.Errorf("store: migration %d: %w", v+1, err)
		}
		if _, err := tx.Exec(
			`INSERT INTO meta(key, value) VALUES ('schema_version', ?)
			 ON CONFLICT(key) DO UPDATE SET value = excluded.value`, v+1,
		); err != nil {
			tx.Rollback()
			return fmt.Errorf("store: migration %d: %w", v+1, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("store: commit migration %d: %w", v+1, err)
		}
	}
	return nil
}

// GetMeta returns the value for key from the meta table ("" if absent).
func (s *Store) GetMeta(key string) (string, error) {
	var v string
	err := s.db.QueryRow(`SELECT value FROM meta WHERE key = ?`, key).Scan(&v)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return v, err
}

// SetMeta sets a key in the meta table.
func (s *Store) SetMeta(key, value string) error {
	_, err := s.db.Exec(
		`INSERT INTO meta(key, value) VALUES (?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}
