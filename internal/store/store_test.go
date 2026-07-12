package store

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func mem(typ, text string, tags ...string) *Memory {
	return &Memory{Type: typ, Text: text, Tags: tags, Privacy: PrivacyPrivate}
}

// --- normalization and content hash (MIND_FORMAT.md §4.2) ---

func TestNormalizeText(t *testing.T) {
	cases := []struct{ in, want string }{
		{"  My   name is\n Soares ", "My name is Soares"},
		{"", ""},
		{" \t\n ", ""},
		{"café", "café"},                       // NFC composed stays
		{"café", "café"},                      // NFD decomposed → NFC
		{"UPPER Case Kept", "UPPER Case Kept"}, // no case folding
		{"tabs\tand\r\nnewlines", "tabs and newlines"},
	}
	for _, c := range cases {
		if got := NormalizeText(c.in); got != c.want {
			t.Errorf("NormalizeText(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestContentHash(t *testing.T) {
	// Equivalent under normalization → identical hash.
	a := ContentHash("  My   name is\n Soares ")
	b := ContentHash("My name is Soares")
	if a != b {
		t.Errorf("normalized-equal texts hash differently: %s vs %s", a, b)
	}
	// NFC vs NFD forms of the same text → identical hash.
	if ContentHash("café") != ContentHash("café") {
		t.Error("NFC and NFD forms hash differently")
	}
	// Case is semantic → different hash.
	if ContentHash("go") == ContentHash("Go") {
		t.Error("case-differing texts must hash differently")
	}
	if !strings.HasPrefix(a, "sha256:") || len(a) != len("sha256:")+64 {
		t.Errorf("bad hash format: %s", a)
	}
	if a != strings.ToLower(a) {
		t.Errorf("hash must be lowercase hex: %s", a)
	}
}

// --- IDs (ADR-003) ---

func TestNewIDIsUUIDv7(t *testing.T) {
	id := NewID()
	if len(id) != 36 {
		t.Fatalf("unexpected UUID string %q", id)
	}
	if id[14] != '7' {
		t.Errorf("version nibble = %c, want 7 (%s)", id[14], id)
	}
	switch id[19] {
	case '8', '9', 'a', 'b':
	default:
		t.Errorf("variant nibble = %c, want RFC 4122 (%s)", id[19], id)
	}
	// Time-ordered: sequential IDs sort ascending.
	prev := NewID()
	time.Sleep(2 * time.Millisecond)
	next := NewID()
	if !(prev < next) {
		t.Errorf("UUIDv7 not time-ordered: %s !< %s", prev, next)
	}
}

// --- vectors ---

func TestVectorRoundtrip(t *testing.T) {
	v := []float32{0.1, -2.5, 0, 3.14159, -0.0001}
	got, err := DecodeVector(EncodeVector(v))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(v) {
		t.Fatalf("len %d != %d", len(got), len(v))
	}
	for i := range v {
		if got[i] != v[i] {
			t.Errorf("v[%d] = %v, want %v", i, got[i], v[i])
		}
	}
	if _, err := DecodeVector([]byte{1, 2, 3}); err == nil {
		t.Error("DecodeVector accepted a non-multiple-of-4 blob")
	}
}

// --- memories: CRUD and dedup ---

func TestInsertMemoryFillsDerivedFields(t *testing.T) {
	s := newTestStore(t)
	m := mem(TypeFact, "The user's name is Soares.", "Identity", "identity", " ")
	if err := s.InsertMemory(m); err != nil {
		t.Fatal(err)
	}
	if m.ID == "" || m.ContentHash == "" || m.CreatedAt.IsZero() {
		t.Errorf("derived fields not filled: %+v", m)
	}
	got, err := s.GetMemoryByID(m.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Text != m.Text || got.Type != TypeFact || got.Privacy != PrivacyPrivate {
		t.Errorf("roundtrip mismatch: %+v", got)
	}
	if len(got.Tags) != 1 || got.Tags[0] != "identity" {
		t.Errorf("tags not normalized/deduped: %v", got.Tags)
	}
	if got.Source.Kind != "manual" {
		t.Errorf("default source kind = %q, want manual", got.Source.Kind)
	}
}

func TestInsertDuplicateContent(t *testing.T) {
	s := newTestStore(t)
	if err := s.InsertMemory(mem(TypeFact, "My project uses Go.")); err != nil {
		t.Fatal(err)
	}
	// Same text modulo normalization → ErrDuplicateContent (UNIQUE content_hash).
	err := s.InsertMemory(mem(TypeFact, "  My project   uses Go. "))
	if !errors.Is(err, ErrDuplicateContent) {
		t.Fatalf("want ErrDuplicateContent, got %v", err)
	}
}

func TestInsertValidation(t *testing.T) {
	s := newTestStore(t)
	if err := s.InsertMemory(mem("opinion", "x")); err == nil {
		t.Error("accepted invalid type")
	}
	if err := s.InsertMemory(mem(TypeFact, "   ")); err == nil {
		t.Error("accepted whitespace-only text")
	}
	bad := mem(TypeFact, "x")
	bad.Privacy = "secret"
	if err := s.InsertMemory(bad); err == nil {
		t.Error("accepted invalid privacy")
	}
}

func TestUpdateMemoryClearsStaleEmbedding(t *testing.T) {
	s := newTestStore(t)
	m := mem(TypeFact, "original text")
	m.Embedding = []float32{1, 0}
	m.EmbeddingModel = "test-model"
	if err := s.InsertMemory(m); err != nil {
		t.Fatal(err)
	}
	m.Text = "edited text"
	m.Embedding = nil
	if err := s.UpdateMemory(m); err != nil {
		t.Fatal(err)
	}
	got, _ := s.GetMemoryByID(m.ID)
	if got.Embedding != nil || got.EmbeddingModel != "" {
		t.Error("stale embedding survived a text edit")
	}
	if got.ContentHash != ContentHash("edited text") {
		t.Error("content hash not recomputed on update")
	}
	if s.IndexSize() != 0 {
		t.Error("index still holds the cleared embedding")
	}
}

func TestDeleteMemory(t *testing.T) {
	s := newTestStore(t)
	m := mem(TypeFact, "to be deleted")
	if err := s.InsertMemory(m); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteMemory(m.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetMemoryByID(m.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
	if err := s.DeleteMemory(m.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("second delete: want ErrNotFound, got %v", err)
	}
}

// --- merge rules (MIND_FORMAT.md §7) ---

func TestMergeDedupByContentHash(t *testing.T) {
	s := newTestStore(t)
	local := mem(TypeFact, "Shared knowledge line.")
	if err := s.InsertMemory(local); err != nil {
		t.Fatal(err)
	}
	// Same content under a different UUID → rule 1: skip, keep local.
	incoming := *mem(TypeFact, "Shared   knowledge line.")
	incoming.ID = NewID()
	incoming.UpdatedAt = time.Now().Add(time.Hour)
	out, err := s.Merge(incoming)
	if err != nil {
		t.Fatal(err)
	}
	if out != MergeSkipped {
		t.Errorf("outcome = %s, want skipped", out)
	}
	all, _ := s.ListMemories(ListFilter{})
	if len(all) != 1 || all[0].ID != local.ID {
		t.Errorf("local memory not preserved: %v", all)
	}
}

func TestMergeSameIDNewerWins(t *testing.T) {
	s := newTestStore(t)
	base := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	local := mem(TypeProcedure, "old preference")
	local.CreatedAt, local.UpdatedAt = base, base
	if err := s.InsertMemory(local); err != nil {
		t.Fatal(err)
	}

	// Incoming edit with newer updated_at → rule 2: update.
	incoming := *mem(TypeProcedure, "new preference")
	incoming.ID = local.ID
	incoming.CreatedAt = base
	incoming.UpdatedAt = base.Add(time.Hour)
	out, err := s.Merge(incoming)
	if err != nil {
		t.Fatal(err)
	}
	if out != MergeUpdated {
		t.Fatalf("outcome = %s, want updated", out)
	}
	got, _ := s.GetMemoryByID(local.ID)
	if got.Text != "new preference" {
		t.Errorf("text = %q, want the newer edit", got.Text)
	}
	if !got.CreatedAt.Equal(base) {
		t.Errorf("created_at changed on update: %v", got.CreatedAt)
	}

	// Incoming with older updated_at → keep local.
	older := *mem(TypeProcedure, "stale preference")
	older.ID = local.ID
	older.UpdatedAt = base.Add(-time.Hour)
	if out, _ = s.Merge(older); out != MergeSkipped {
		t.Errorf("older incoming: outcome = %s, want skipped", out)
	}

	// Tie → keep local.
	tie := *mem(TypeProcedure, "tie preference")
	tie.ID = local.ID
	tie.UpdatedAt = base.Add(time.Hour) // equals current updated_at
	if out, _ = s.Merge(tie); out != MergeSkipped {
		t.Errorf("tie: outcome = %s, want skipped", out)
	}
	got, _ = s.GetMemoryByID(local.ID)
	if got.Text != "new preference" {
		t.Errorf("tie/older overwrote local: %q", got.Text)
	}
}

func TestMergeInsertsNew(t *testing.T) {
	s := newTestStore(t)
	incoming := *mem(TypeEpisode, "brand new episode")
	incoming.ID = NewID()
	out, err := s.Merge(incoming)
	if err != nil {
		t.Fatal(err)
	}
	if out != MergeInserted {
		t.Errorf("outcome = %s, want inserted", out)
	}
}

// --- vector search (DAEMON_DESIGN.md §6.3) ---

func TestSearch(t *testing.T) {
	s := newTestStore(t)
	insert := func(typ, text string, vec []float32) *Memory {
		t.Helper()
		m := mem(typ, text)
		m.Embedding = vec
		m.EmbeddingModel = "test-model"
		if err := s.InsertMemory(m); err != nil {
			t.Fatal(err)
		}
		return m
	}
	m1 := insert(TypeFact, "exact match", []float32{1, 0})
	m2 := insert(TypeEpisode, "orthogonal", []float32{0, 1})
	m3 := insert(TypeProcedure, "diagonal", []float32{0.70710678, 0.70710678})

	query := []float32{1, 0}
	got, err := s.Search(query, SearchOptions{K: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d results, want 3", len(got))
	}
	wantOrder := []string{m1.ID, m3.ID, m2.ID}
	for i, w := range wantOrder {
		if got[i].Memory.ID != w {
			t.Errorf("result[%d] = %s (score %f), want %s", i, got[i].Memory.ID, got[i].Score, w)
		}
	}
	if got[0].Score < 0.999 || got[1].Score < 0.70 || got[1].Score > 0.71 {
		t.Errorf("unexpected scores: %f, %f", got[0].Score, got[1].Score)
	}

	// min similarity filters the orthogonal memory.
	got, _ = s.Search(query, SearchOptions{K: 10, MinSimilarity: 0.5})
	if len(got) != 2 {
		t.Errorf("minSim: got %d results, want 2", len(got))
	}

	// K limits.
	got, _ = s.Search(query, SearchOptions{K: 1})
	if len(got) != 1 || got[0].Memory.ID != m1.ID {
		t.Errorf("K=1: got %v", got)
	}

	// Type filter.
	got, _ = s.Search(query, SearchOptions{K: 10, Types: []string{TypeProcedure}})
	if len(got) != 1 || got[0].Memory.ID != m3.ID {
		t.Errorf("type filter: got %v", got)
	}
	_ = m2
}

func TestSearchIndexSurvivesReopen(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	m := mem(TypeFact, "persisted vector")
	m.Embedding = []float32{0, 1}
	m.EmbeddingModel = "test-model"
	if err := s.InsertMemory(m); err != nil {
		t.Fatal(err)
	}
	s.Close()

	s2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	got, err := s2.Search([]float32{0, 1}, SearchOptions{K: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Memory.ID != m.ID {
		t.Fatalf("index not rebuilt on reopen: %v", got)
	}
}

func TestRebuildIndexFiltersByModel(t *testing.T) {
	s := newTestStore(t)
	m := mem(TypeFact, "embedded with old model")
	m.Embedding = []float32{1, 0}
	m.EmbeddingModel = "old-model"
	if err := s.InsertMemory(m); err != nil {
		t.Fatal(err)
	}
	if err := s.RebuildIndex("new-model"); err != nil {
		t.Fatal(err)
	}
	if s.IndexSize() != 0 {
		t.Error("stale-model embedding stayed in the index")
	}
	if err := s.RebuildIndex("old-model"); err != nil {
		t.Fatal(err)
	}
	if s.IndexSize() != 1 {
		t.Error("matching-model embedding missing from the index")
	}
}

// --- candidates ---

func TestCandidateLifecycle(t *testing.T) {
	s := newTestStore(t)
	c := &Candidate{Type: TypeFact, Text: "The user prefers dark mode.", Tags: []string{"UI"}}
	if err := s.InsertCandidate(c); err != nil {
		t.Fatal(err)
	}
	pending, err := s.ListCandidates(StatusPending)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 || pending[0].Status != StatusPending || pending[0].Tags[0] != "ui" {
		t.Fatalf("pending list wrong: %+v", pending)
	}

	dup, err := s.HasPendingCandidateWithText("The user   prefers dark mode.")
	if err != nil || !dup {
		t.Errorf("pending dedup check failed: %v %v", dup, err)
	}

	if err := s.SetCandidateStatus(c.ID, StatusApproved); err != nil {
		t.Fatal(err)
	}
	pending, _ = s.ListCandidates(StatusPending)
	if len(pending) != 0 {
		t.Error("approved candidate still pending")
	}
	if err := s.SetCandidateStatus("nope", StatusRejected); !errors.Is(err, ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
	if err := s.SetCandidateStatus(c.ID, "meh"); err == nil {
		t.Error("accepted invalid status")
	}
}

// --- sessions ---

func TestSessionLifecycle(t *testing.T) {
	s := newTestStore(t)
	id, err := s.TouchSession("chat-1")
	if err != nil {
		t.Fatal(err)
	}
	if id != "chat-1" {
		t.Fatalf("id = %s", id)
	}
	if err := s.AppendMessage(id, "user", "hello"); err != nil {
		t.Fatal(err)
	}
	if err := s.AppendMessage(id, "assistant", "hi!"); err != nil {
		t.Fatal(err)
	}
	msgs, err := s.SessionMessages(id)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 2 || msgs[0].Role != "user" || msgs[1].Content != "hi!" {
		t.Fatalf("messages wrong: %+v", msgs)
	}

	// Not extractable while open.
	todo, _ := s.SessionsToExtract()
	if len(todo) != 0 {
		t.Error("open session listed as extractable")
	}

	if err := s.CloseSession(id); err != nil {
		t.Fatal(err)
	}
	if _, err := s.TouchSession(id); err == nil {
		t.Error("touching a closed session must fail")
	}
	todo, _ = s.SessionsToExtract()
	if len(todo) != 1 || todo[0].ID != id {
		t.Fatalf("extractable = %+v", todo)
	}
	if err := s.MarkSessionExtracted(id); err != nil {
		t.Fatal(err)
	}
	todo, _ = s.SessionsToExtract()
	if len(todo) != 0 {
		t.Error("extracted session still listed")
	}
}

func TestCloseIdleSessions(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.TouchSession("idle"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond)
	cutoffAfter := time.Now()
	if _, err := s.TouchSession("active"); err != nil {
		t.Fatal(err)
	}
	n, err := s.CloseIdleSessions(cutoffAfter)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("closed %d sessions, want 1", n)
	}
	idle, _ := s.GetSession("idle")
	active, _ := s.GetSession("active")
	if idle.ClosedAt == nil || active.ClosedAt != nil {
		t.Error("wrong session closed")
	}
	open, total, err := s.CountSessions()
	if err != nil {
		t.Fatal(err)
	}
	if open != 1 || total != 2 {
		t.Errorf("counts: open=%d total=%d", open, total)
	}
}
