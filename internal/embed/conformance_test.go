package embed

import (
	"encoding/json"
	"os"
	"testing"
)

// goldenCase is one entry of testdata/golden.json, produced by the reference
// HuggingFace tokenizer (see testdata/README.md).
type goldenCase struct {
	Text   string   `json:"text"`
	IDs    []int64  `json:"ids"`
	Tokens []string `json:"tokens"`
}

// TestTokenizerConformance enforces DAEMON_DESIGN.md §6.1: 100% exact token-ID
// parity with the reference implementation. Any mismatch is a hard failure —
// an "almost right" tokenizer silently degrades retrieval.
func TestTokenizerConformance(t *testing.T) {
	raw, err := os.ReadFile("testdata/golden.json")
	if err != nil {
		t.Fatalf("golden file: %v", err)
	}
	var cases []goldenCase
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatalf("golden file: %v", err)
	}
	if len(cases) == 0 {
		t.Fatal("golden file is empty")
	}
	tok, err := NewTokenizer("testdata/vocab.txt")
	if err != nil {
		t.Fatal(err)
	}

	for _, c := range cases {
		got := tok.Encode(c.Text, 256)
		if len(got) != len(c.IDs) {
			t.Errorf("%q: got %d ids, want %d\n got: %v\nwant: %v (tokens %v)",
				truncate(c.Text), len(got), len(c.IDs), got, c.IDs, c.Tokens)
			continue
		}
		for i := range got {
			if got[i] != c.IDs[i] {
				t.Errorf("%q: id[%d] = %d, want %d (token %q)\n got: %v\nwant: %v",
					truncate(c.Text), i, got[i], c.IDs[i], c.Tokens[i], got, c.IDs)
				break
			}
		}
	}
}

func truncate(s string) string {
	if len(s) > 60 {
		return s[:60] + "…"
	}
	return s
}
