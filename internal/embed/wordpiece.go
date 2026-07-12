package embed

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// Tokenizer is a BERT WordPiece tokenizer compatible with the HuggingFace
// reference for all-MiniLM-L6-v2 (uncased: lowercase + strip accents).
// Conformance is enforced against golden files produced by the reference
// implementation (DAEMON_DESIGN.md §6.1): 100% token-ID parity or fallback
// to hugot — no middle ground.
type Tokenizer struct {
	vocab map[string]int64
	clsID int64
	sepID int64
	unkID int64
}

// maxWordChars mirrors the reference max_input_chars_per_word: longer words
// become [UNK] without attempting subword matching.
const maxWordChars = 100

// NewTokenizer loads a WordPiece vocabulary file (one token per line, line
// number = token ID).
func NewTokenizer(vocabPath string) (*Tokenizer, error) {
	f, err := os.Open(vocabPath)
	if err != nil {
		return nil, fmt.Errorf("embed: open vocab: %w", err)
	}
	defer f.Close()
	return newTokenizerFromReader(f)
}

func newTokenizerFromReader(r io.Reader) (*Tokenizer, error) {
	vocab := make(map[string]int64, 31000)
	sc := bufio.NewScanner(r)
	var id int64
	for sc.Scan() {
		// Tokens are stored verbatim; only the trailing newline is removed.
		vocab[strings.TrimRight(sc.Text(), "\r")] = id
		id++
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("embed: read vocab: %w", err)
	}
	t := &Tokenizer{vocab: vocab}
	var ok bool
	for _, s := range []struct {
		name string
		dst  *int64
	}{{"[CLS]", &t.clsID}, {"[SEP]", &t.sepID}, {"[UNK]", &t.unkID}} {
		if *s.dst, ok = vocab[s.name]; !ok {
			return nil, fmt.Errorf("embed: vocab is missing %s", s.name)
		}
	}
	return t, nil
}

// Encode tokenizes text into BERT input IDs: [CLS] pieces... [SEP], with the
// pieces truncated so the total length never exceeds maxLen.
func (t *Tokenizer) Encode(text string, maxLen int) []int64 {
	var pieces []int64
	for _, word := range pretokenize(text) {
		pieces = append(pieces, t.wordpiece(word)...)
	}
	if maxLen >= 2 && len(pieces) > maxLen-2 {
		pieces = pieces[:maxLen-2]
	}
	ids := make([]int64, 0, len(pieces)+2)
	ids = append(ids, t.clsID)
	ids = append(ids, pieces...)
	return append(ids, t.sepID)
}

// wordpiece greedily matches the longest vocabulary subword; any failure
// turns the whole word into [UNK] (reference behavior).
func (t *Tokenizer) wordpiece(word string) []int64 {
	runes := []rune(word)
	if len(runes) > maxWordChars {
		return []int64{t.unkID}
	}
	var out []int64
	start := 0
	for start < len(runes) {
		end := len(runes)
		match := int64(-1)
		for start < end {
			sub := string(runes[start:end])
			if start > 0 {
				sub = "##" + sub
			}
			if id, ok := t.vocab[sub]; ok {
				match = id
				break
			}
			end--
		}
		if match < 0 {
			return []int64{t.unkID}
		}
		out = append(out, match)
		start = end
	}
	return out
}

// pretokenize applies the BertNormalizer (clean text, space out CJK, strip
// accents, lowercase) and the BertPreTokenizer (whitespace split, isolate
// punctuation), mirroring the reference pipeline order exactly.
func pretokenize(text string) []string {
	var b strings.Builder
	b.Grow(len(text))

	// clean_text + handle_chinese_chars in one pass.
	for _, r := range text {
		switch {
		case r == 0 || r == 0xFFFD || isControl(r):
			// dropped
		case isWhitespace(r):
			b.WriteByte(' ')
		case isCJK(r):
			b.WriteByte(' ')
			b.WriteRune(r)
			b.WriteByte(' ')
		default:
			b.WriteRune(r)
		}
	}

	// strip accents (NFD, drop combining marks) then lowercase.
	var c strings.Builder
	c.Grow(b.Len())
	for _, r := range norm.NFD.String(b.String()) {
		if unicode.Is(unicode.Mn, r) {
			continue
		}
		c.WriteRune(unicode.ToLower(r))
	}

	// whitespace split + punctuation isolation.
	var words []string
	for _, w := range strings.Fields(c.String()) {
		words = append(words, splitPunct(w)...)
	}
	return words
}

// splitPunct isolates every punctuation rune as its own token.
func splitPunct(word string) []string {
	var out []string
	var cur strings.Builder
	for _, r := range word {
		if isBertPunct(r) {
			if cur.Len() > 0 {
				out = append(out, cur.String())
				cur.Reset()
			}
			out = append(out, string(r))
			continue
		}
		cur.WriteRune(r)
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}

func isWhitespace(r rune) bool {
	return r == ' ' || r == '\t' || r == '\n' || r == '\r' || unicode.Is(unicode.Zs, r)
}

func isControl(r rune) bool {
	if r == '\t' || r == '\n' || r == '\r' {
		return false // treated as whitespace, not control
	}
	return unicode.In(r, unicode.Cc, unicode.Cf, unicode.Co, unicode.Cs)
}

// isBertPunct matches the reference definition: ASCII punctuation (which
// includes S-category chars like $ + ~ | ^) or any Unicode P category.
func isBertPunct(r rune) bool {
	return (r >= '!' && r <= '/') || (r >= ':' && r <= '@') ||
		(r >= '[' && r <= '`') || (r >= '{' && r <= '~') ||
		unicode.IsPunct(r)
}

// isCJK reports whether r is a CJK ideograph (the ranges BERT spaces out).
func isCJK(r rune) bool {
	switch {
	case r >= 0x4E00 && r <= 0x9FFF,
		r >= 0x3400 && r <= 0x4DBF,
		r >= 0x20000 && r <= 0x2A6DF,
		r >= 0x2A700 && r <= 0x2B73F,
		r >= 0x2B740 && r <= 0x2B81F,
		r >= 0x2B820 && r <= 0x2CEAF,
		r >= 0xF900 && r <= 0xFAFF,
		r >= 0x2F800 && r <= 0x2FA1F:
		return true
	}
	return false
}
