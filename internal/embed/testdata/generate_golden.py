#!/usr/bin/env python3
"""Generate golden token IDs for the WordPiece conformance test.

The expected IDs come from the REFERENCE implementation (HuggingFace
`tokenizers`), never from our own Go code — see DAEMON_DESIGN.md §6.1.

Usage (documented in README.md):

    python3 -m venv venv && ./venv/bin/pip install tokenizers==0.23.1
    curl -sLO https://huggingface.co/sentence-transformers/all-MiniLM-L6-v2/resolve/main/tokenizer.json
    ./venv/bin/python generate_golden.py tokenizer.json > golden.json
"""

import json
import sys

from tokenizers import Tokenizer

# max_seq_length of all-MiniLM-L6-v2 (sentence-transformers config); the Go
# embedder truncates at the same limit.
MAX_LEN = 256

CASES = [
    # plain English
    "Hello, world!",
    "The quick brown fox jumps over the lazy dog.",
    "Mnemo is a portable memory layer for small local LLMs.",
    # Portuguese with accents (ç, ã, é, ...)
    "O coração da solução é a memória portátil.",
    "Amanhã à noite vou à reunião com o João em São Paulo.",
    "ação informação você português caçador",
    # emoji
    "I love this project 😀🚀",
    "🦄",
    "emoji inside w😀rd",
    # out-of-vocabulary words
    "asdkfjqwer zxqvbnmtp",
    "supercalifragilisticexpialidocious antidisestablishmentarianism",
    "xylophonisticness",
    # long string (past the 256-token truncation limit)
    "memory " * 300,
    # empty string
    "",
    # multiple spaces and whitespace soup
    "multiple    spaces   between     words",
    "  leading and trailing  ",
    "tabs\tand\nnewlines\r\nmixed   in",
    # case handling (uncased model lowercases)
    "MiXeD CaSe TEXT",
    # accents in NFD (decomposed) form: "café" with combining acute
    "café decomposed",
    # punctuation and ASCII symbols (S-category chars like $ + ~ | ^)
    "price: $5.99 + tax ~= $6.50 (approx.) | a^b `q`",
    "don't stop-motion e-mail user@example.com https://example.com/path?q=1",
    # CJK (BERT spaces out ideographs)
    "深度学习模型 mixed with English",
    # digits
    "in 2026 there were 1234567 memories",
]


def main() -> None:
    tok = Tokenizer.from_file(sys.argv[1])
    tok.no_padding()
    tok.enable_truncation(max_length=MAX_LEN)
    out = []
    for text in CASES:
        enc = tok.encode(text)
        out.append({"text": text, "ids": enc.ids, "tokens": enc.tokens})
    json.dump(out, sys.stdout, ensure_ascii=False, indent=1)
    sys.stdout.write("\n")


if __name__ == "__main__":
    main()
