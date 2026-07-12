# Tokenizer conformance golden files

Mandated by DAEMON_DESIGN.md §6.1: the in-tree WordPiece tokenizer is approved
only while `go test` matches these golden files **100% on token IDs**. The
expected IDs are produced by the reference implementation (HuggingFace
`tokenizers`), never by our own code. If conformance cannot be kept at 100%,
the fallback is `hugot`.

## Files

- `golden.json` — test cases: input text, expected token IDs, and (for
  debuggability) the expected token strings.
- `vocab.txt` — the `all-MiniLM-L6-v2` WordPiece vocabulary the Go tokenizer
  loads in tests (30,522 entries, Apache-2.0, from
  `sentence-transformers/all-MiniLM-L6-v2`).
  SHA-256: `07eced375cec144d27c900241f3e339478dec958f92fddbc551f295c992038a3`
- `generate_golden.py` — the exact script that produced `golden.json`.

## How golden.json was produced (reproducible)

Reference: HuggingFace `tokenizers` **0.23.1** on Python 3.14, using the
`tokenizer.json` of `sentence-transformers/all-MiniLM-L6-v2`
(SHA-256 `be50c3628f2bf5bb5e3a7f17b1f74611b2561a3a27eeab05e5aa30f411572037`),
no padding, truncation at 256 tokens (the model's `max_seq_length`):

```sh
python3 -m venv venv
./venv/bin/pip install tokenizers==0.23.1
curl -sLO https://huggingface.co/sentence-transformers/all-MiniLM-L6-v2/resolve/main/tokenizer.json
./venv/bin/python generate_golden.py tokenizer.json > golden.json
```

## Corpus coverage (per spec)

Plain English · Portuguese with accents (ç, ã, é) · emoji · out-of-vocabulary
words · long strings past the 256-token truncation · empty string · multiple
spaces/tabs/newlines · mixed case · NFD-decomposed accents · punctuation and
ASCII symbols (`$ + ~ | ^`) · CJK · digits.
