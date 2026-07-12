package embed

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"testing"
)

// TestONNXEndToEnd runs real inference. It needs the model files, so it is
// gated on MNEMO_TEST_MODEL_DIR (a directory containing model.onnx,
// vocab.txt, and the ONNX Runtime shared library) and skips otherwise —
// CI without the files stays green, local runs verify the full pipeline.
func TestONNXEndToEnd(t *testing.T) {
	dir := os.Getenv("MNEMO_TEST_MODEL_DIR")
	if dir == "" {
		t.Skip("MNEMO_TEST_MODEL_DIR not set")
	}
	e, err := NewONNX(ONNXConfig{
		ModelPath:   filepath.Join(dir, "model.onnx"),
		VocabPath:   filepath.Join(dir, "vocab.txt"),
		LibraryPath: filepath.Join(dir, LibraryName()),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()

	texts := []string{
		"A cat sits on the mat.",
		"A kitten rests on the rug.",
		"The stock market crashed in 1929.",
	}
	vecs, err := e.Embed(context.Background(), texts)
	if err != nil {
		t.Fatal(err)
	}
	if len(vecs) != 3 || len(vecs[0]) != ModelDims {
		t.Fatalf("shape: %d x %d, want 3 x %d", len(vecs), len(vecs[0]), ModelDims)
	}
	for i, v := range vecs {
		var norm float64
		for _, f := range v {
			norm += float64(f) * float64(f)
		}
		if math.Abs(norm-1) > 1e-3 {
			t.Errorf("vec[%d] norm² = %f, want 1", i, norm)
		}
	}
	dotp := func(a, b []float32) float64 {
		var s float64
		for i := range a {
			s += float64(a[i]) * float64(b[i])
		}
		return s
	}
	catKitten := dotp(vecs[0], vecs[1])
	catMarket := dotp(vecs[0], vecs[2])
	t.Logf("sim(cat,kitten)=%.3f sim(cat,market)=%.3f", catKitten, catMarket)
	if catKitten <= catMarket {
		t.Errorf("semantic ordering broken: sim(cat,kitten)=%.3f <= sim(cat,market)=%.3f",
			catKitten, catMarket)
	}
	if catKitten < 0.5 {
		t.Errorf("similar sentences score only %.3f", catKitten)
	}

	// Batch result must match single-text result (padding must not leak).
	single, err := e.Embed(context.Background(), []string{texts[0]})
	if err != nil {
		t.Fatal(err)
	}
	if sim := dotp(vecs[0], single[0]); sim < 0.9999 {
		t.Errorf("batched vs single mismatch: sim = %f", sim)
	}
}
