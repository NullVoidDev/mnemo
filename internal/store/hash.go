package store

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"golang.org/x/text/unicode/norm"
)

// NormalizeText implements the normative text normalization of
// MIND_FORMAT.md §4.2: NFC, trim, collapse whitespace runs to a single
// ASCII space. Nothing else — no case folding.
func NormalizeText(s string) string {
	// strings.Fields splits on Unicode White_Space, which trims and
	// collapses in one pass.
	return strings.Join(strings.Fields(norm.NFC.String(s)), " ")
}

// ContentHash returns "sha256:" + lowercase hex SHA-256 of the normalized
// text (MIND_FORMAT.md §4.2). Two implementations MUST agree on this value
// for dedup to work across .mind files.
func ContentHash(text string) string {
	sum := sha256.Sum256([]byte(NormalizeText(text)))
	return "sha256:" + hex.EncodeToString(sum[:])
}
