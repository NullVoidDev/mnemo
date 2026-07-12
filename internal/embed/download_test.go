package embed

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func serve(t *testing.T, body []byte) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func sum(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func TestFetchPlainFile(t *testing.T) {
	body := []byte("model bytes")
	srv := serve(t, body)
	dir := t.TempDir()
	a := Asset{URL: srv.URL, SHA256: sum(body), Target: "model.onnx"}
	if err := Fetch(context.Background(), nil, a, dir); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "model.onnx"))
	if err != nil || !bytes.Equal(got, body) {
		t.Fatalf("installed file wrong: %q %v", got, err)
	}
	if !a.Installed(dir) {
		t.Error("Installed() = false after install")
	}
}

func TestFetchRejectsBadHash(t *testing.T) {
	srv := serve(t, []byte("tampered bytes"))
	dir := t.TempDir()
	a := Asset{URL: srv.URL, SHA256: strings.Repeat("0", 64), Target: "model.onnx"}
	err := Fetch(context.Background(), nil, a, dir)
	if err == nil || !strings.Contains(err.Error(), "SHA-256 mismatch") {
		t.Fatalf("want SHA-256 mismatch error, got %v", err)
	}
	// Nothing installed, no temp litter.
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Errorf("directory not clean after failure: %v", entries)
	}
}

func TestFetchExtractsTgzMember(t *testing.T) {
	lib := []byte("shared library bytes")
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	// A decoy entry plus the real member.
	tw.WriteHeader(&tar.Header{Name: "pkg/README", Mode: 0o644, Size: 5})
	tw.Write([]byte("hello"))
	tw.WriteHeader(&tar.Header{Name: "pkg/lib/libonnxruntime.1.dylib", Mode: 0o755, Size: int64(len(lib))})
	tw.Write(lib)
	tw.Close()
	gz.Close()

	srv := serve(t, buf.Bytes())
	dir := t.TempDir()
	a := Asset{
		URL:    srv.URL,
		SHA256: sum(buf.Bytes()),
		Member: "pkg/lib/libonnxruntime.1.dylib",
		Target: "libonnxruntime.dylib",
	}
	if err := Fetch(context.Background(), nil, a, dir); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "libonnxruntime.dylib"))
	if err != nil || !bytes.Equal(got, lib) {
		t.Fatalf("extracted member wrong: %q %v", got, err)
	}

	// Missing member is a clear error.
	a.Member = "pkg/lib/nope.dylib"
	a.Target = "other.dylib"
	if err := Fetch(context.Background(), nil, a, dir); err == nil ||
		!strings.Contains(err.Error(), "not found in archive") {
		t.Fatalf("want member-not-found error, got %v", err)
	}
}

func TestRuntimeAssetPinnedForCurrentPlatform(t *testing.T) {
	a, err := RuntimeAsset()
	if err != nil {
		t.Skipf("platform not pinned: %v", err)
	}
	if a.SHA256 == "" || a.Member == "" || a.Target == "" {
		t.Errorf("incomplete asset: %+v", a)
	}
}
