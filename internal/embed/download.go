package embed

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// ortVersion is the pinned ONNX Runtime release.
const ortVersion = "1.27.1"

// Asset is one downloadable file with a pinned SHA-256 (DAEMON_DESIGN.md
// §6.1: `mnemo init` verifies every download against hashes pinned here).
type Asset struct {
	URL    string
	SHA256 string // lowercase hex of the downloaded file (archive when Member != "")
	Member string // path inside the archive to extract ("" = plain file)
	Target string // filename to install into the destination directory
}

// ModelAssets returns the embedding model files (model + vocabulary) from
// sentence-transformers/all-MiniLM-L6-v2 (Apache-2.0).
func ModelAssets() []Asset {
	const base = "https://huggingface.co/sentence-transformers/all-MiniLM-L6-v2/resolve/main/"
	return []Asset{
		{
			URL:    base + "onnx/model.onnx",
			SHA256: "6fd5d72fe4589f189f8ebc006442dbb529bb7ce38f8082112682524616046452",
			Target: "model.onnx",
		},
		{
			URL:    base + "vocab.txt",
			SHA256: "07eced375cec144d27c900241f3e339478dec958f92fddbc551f295c992038a3",
			Target: "vocab.txt",
		},
	}
}

// LibraryName is the canonical ONNX Runtime file name inside the models
// directory for the current platform.
func LibraryName() string {
	switch runtime.GOOS {
	case "darwin":
		return "libonnxruntime.dylib"
	case "windows":
		return "onnxruntime.dll"
	default:
		return "libonnxruntime.so"
	}
}

// RuntimeAsset returns the pinned ONNX Runtime shared library for the
// current platform. Unpinned platforms get a clear error instead of an
// unverified download.
func RuntimeAsset() (Asset, error) {
	const relBase = "https://github.com/microsoft/onnxruntime/releases/download/v" + ortVersion + "/"
	type pin struct{ archive, sha, member string }
	pins := map[string]pin{
		"darwin/arm64": {
			archive: "onnxruntime-osx-arm64-" + ortVersion + ".tgz",
			sha:     "e42b77a7281cc6e55141bf44fcfbac2c782b823a491bbb6ac33c781dd991f8a6",
			member:  "onnxruntime-osx-arm64-" + ortVersion + "/lib/libonnxruntime." + ortVersion + ".dylib",
		},
		"darwin/amd64": {
			archive: "onnxruntime-osx-x86_64-" + ortVersion + ".tgz",
			sha:     "0019dfc4b32d63c1392aa264aed2253c1e0c2fb09216f8e2cc269bbfb8bb49b5",
			member:  "onnxruntime-osx-x86_64-" + ortVersion + "/lib/libonnxruntime." + ortVersion + ".dylib",
		},
		"linux/amd64": {
			archive: "onnxruntime-linux-x64-" + ortVersion + ".tgz",
			sha:     "25b1ef1fea1acd210d63f8f24dc870ad6e077795ce1f54876252c6d3803c15af",
			member:  "onnxruntime-linux-x64-" + ortVersion + "/lib/libonnxruntime.so." + ortVersion,
		},
		"linux/arm64": {
			archive: "onnxruntime-linux-aarch64-" + ortVersion + ".tgz",
			sha:     "33c67e33d1e25b816878366ea276589a024f71f000e7ff955c4b33224d639edd",
			member:  "onnxruntime-linux-aarch64-" + ortVersion + "/lib/libonnxruntime.so." + ortVersion,
		},
		"windows/amd64": {
			archive: "onnxruntime-win-x64-" + ortVersion + ".zip",
			sha:     "2e00414a63fdef0914cd5a5ede6c707844878e0c08e1b6693842f0451b2df2a1",
			member:  "onnxruntime-win-x64-" + ortVersion + "/lib/onnxruntime.dll",
		},
	}
	key := runtime.GOOS + "/" + runtime.GOARCH
	p, ok := pins[key]
	if !ok {
		return Asset{}, fmt.Errorf(
			"embed: no pinned ONNX Runtime for %s — download it manually from "+
				"https://github.com/microsoft/onnxruntime/releases and place the shared "+
				"library in the models directory as %s", key, LibraryName())
	}
	return Asset{URL: relBase + p.archive, SHA256: p.sha, Member: p.member, Target: LibraryName()}, nil
}

// Installed reports whether the asset's target already exists in dir.
func (a Asset) Installed(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, a.Target))
	return err == nil
}

// Fetch downloads the asset into dir, verifies its pinned SHA-256, extracts
// the archive member when needed, and installs the target atomically. A
// failed verification leaves nothing behind.
func Fetch(ctx context.Context, client *http.Client, a Asset, dir string) error {
	if client == nil {
		client = http.DefaultClient
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("embed: create %s: %w", dir, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.URL, nil)
	if err != nil {
		return fmt.Errorf("embed: request %s: %w", a.URL, err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("embed: download %s: %w", a.URL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("embed: download %s: HTTP %s", a.URL, resp.Status)
	}

	tmp, err := os.CreateTemp(dir, "download-*")
	if err != nil {
		return fmt.Errorf("embed: temp file: %w", err)
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()

	hash := sha256.New()
	if _, err := io.Copy(io.MultiWriter(tmp, hash), resp.Body); err != nil {
		return fmt.Errorf("embed: download %s: %w", a.URL, err)
	}
	if got := hex.EncodeToString(hash.Sum(nil)); got != a.SHA256 {
		return fmt.Errorf("embed: %s: SHA-256 mismatch: got %s, want %s — refusing to install",
			a.URL, got, a.SHA256)
	}

	dest := filepath.Join(dir, a.Target)
	if a.Member == "" {
		tmp.Close()
		if err := os.Rename(tmp.Name(), dest); err != nil {
			return fmt.Errorf("embed: install %s: %w", dest, err)
		}
		return nil
	}
	return extractMember(tmp.Name(), a.Member, dest)
}

// extractMember pulls one regular file out of a .tgz or .zip archive and
// installs it atomically at dest.
func extractMember(archivePath, member, dest string) error {
	out, err := os.CreateTemp(filepath.Dir(dest), "extract-*")
	if err != nil {
		return fmt.Errorf("embed: temp file: %w", err)
	}
	defer os.Remove(out.Name())
	defer out.Close()

	var found bool
	switch {
	case strings.HasSuffix(archivePath, ".zip") || strings.HasSuffix(member, ".dll"):
		found, err = extractZipMember(archivePath, member, out)
	default:
		found, err = extractTgzMember(archivePath, member, out)
	}
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("embed: member %s not found in archive", member)
	}
	if err := out.Close(); err != nil {
		return err
	}
	if err := os.Chmod(out.Name(), 0o755); err != nil {
		return err
	}
	if err := os.Rename(out.Name(), dest); err != nil {
		return fmt.Errorf("embed: install %s: %w", dest, err)
	}
	return nil
}

func extractTgzMember(archivePath, member string, out io.Writer) (bool, error) {
	f, err := os.Open(archivePath)
	if err != nil {
		return false, err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return false, fmt.Errorf("embed: gzip: %w", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return false, nil
		}
		if err != nil {
			return false, fmt.Errorf("embed: tar: %w", err)
		}
		if hdr.Name == member && hdr.Typeflag == tar.TypeReg {
			if _, err := io.Copy(out, tr); err != nil {
				return false, fmt.Errorf("embed: extract %s: %w", member, err)
			}
			return true, nil
		}
	}
}

func extractZipMember(archivePath, member string, out io.Writer) (bool, error) {
	zr, err := zip.OpenReader(archivePath)
	if err != nil {
		return false, fmt.Errorf("embed: zip: %w", err)
	}
	defer zr.Close()
	for _, f := range zr.File {
		if f.Name != member {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return false, err
		}
		defer rc.Close()
		if _, err := io.Copy(out, rc); err != nil {
			return false, fmt.Errorf("embed: extract %s: %w", member, err)
		}
		return true, nil
	}
	return false, nil
}
