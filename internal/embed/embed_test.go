package embed

import (
	"context"
	"math"
	"strings"
	"testing"
)

func testTokenizer(t *testing.T) *Tokenizer {
	t.Helper()
	tok, err := NewTokenizer("testdata/vocab.txt")
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

func TestEncodeEmptyString(t *testing.T) {
	tok := testTokenizer(t)
	ids := tok.Encode("", 256)
	if len(ids) != 2 || ids[0] != tok.clsID || ids[1] != tok.sepID {
		t.Errorf("empty string = %v, want [CLS SEP]", ids)
	}
}

func TestEncodeTruncationReservesSpecials(t *testing.T) {
	tok := testTokenizer(t)
	ids := tok.Encode(strings.Repeat("memory ", 500), 256)
	if len(ids) != 256 {
		t.Fatalf("len = %d, want 256", len(ids))
	}
	if ids[0] != tok.clsID || ids[255] != tok.sepID {
		t.Error("[CLS]/[SEP] not preserved under truncation")
	}
}

func TestOverlongWordBecomesUNK(t *testing.T) {
	tok := testTokenizer(t)
	ids := tok.Encode(strings.Repeat("a", 101), 256)
	if len(ids) != 3 || ids[1] != tok.unkID {
		t.Errorf("101-char word = %v, want [CLS UNK SEP]", ids)
	}
}

func TestMockEmbedder(t *testing.T) {
	m := NewMock("mock-model", 8)
	v1, err := m.Embed(context.Background(), []string{"hello", "hello", "world"})
	if err != nil {
		t.Fatal(err)
	}
	if m.Calls() != 1 {
		t.Errorf("calls = %d, want 1", m.Calls())
	}
	if len(v1) != 3 || len(v1[0]) != 8 {
		t.Fatalf("shape wrong: %d x %d", len(v1), len(v1[0]))
	}
	// Deterministic and text-sensitive.
	for i := range v1[0] {
		if v1[0][i] != v1[1][i] {
			t.Fatal("same text embedded differently")
		}
	}
	same := true
	for i := range v1[0] {
		if v1[0][i] != v1[2][i] {
			same = false
			break
		}
	}
	if same {
		t.Error("different texts embedded identically")
	}
	// L2-normalized.
	var norm float64
	for _, f := range v1[0] {
		norm += float64(f) * float64(f)
	}
	if math.Abs(norm-1) > 1e-5 {
		t.Errorf("norm² = %f, want 1", norm)
	}
}
