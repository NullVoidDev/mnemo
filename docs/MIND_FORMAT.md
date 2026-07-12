# The `.mind` Format — Specification v0.1

> **Status:** v0.1 — awaiting approval before implementation.
> Decisions behind this spec: [DECISIONS.md](DECISIONS.md) (ADR-001, ADR-002, ADR-003).

The key words MUST, MUST NOT, SHOULD, and MAY are to be interpreted as described in
RFC 2119.

## 1. Overview

A `.mind` file is a portable, open, model-independent memory interchange file for LLM
memory systems. It is a **ZIP archive** (per ADR-001) with the `.mind` extension.

`.mind` is an **interchange** format: export, import, sharing, backup. It is not the
daemon's working storage (that is SQLite, see DAEMON_DESIGN.md §5).

Guarantees the format makes:

1. **Text is the source of truth.** A `.mind` file with no embeddings at all is still a
   complete, valid memory export.
2. **Model independence.** Embeddings are a disposable cache (ADR-002); switching
   embedding models never loses information.
3. **Privacy by default.** Exports include only `public` memories unless the user
   explicitly opts in (§6).
4. **Deterministic dedup.** Two implementations MUST compute identical `content_hash`
   values for the same text (§4.2).

## 2. Archive Layout

```text
example.mind (ZIP)
├── manifest.json      REQUIRED  format metadata
├── memories.jsonl     REQUIRED  one memory per line
├── embeddings.bin     OPTIONAL  float32 LE vectors, same order as memories.jsonl
└── README.txt         REQUIRED  human-readable explanation
```

- Entry names are case-sensitive and MUST appear at the archive root (no directories).
- Writers MUST use UTF-8 without BOM for all text entries.
- Writers SHOULD use the `deflate` ZIP compression method; readers MUST accept both
  `stored` and `deflate`.
- Readers MUST ignore unknown extra entries (forward compatibility).

## 3. `manifest.json`

A single JSON object:

```json
{
  "format_version": "0.1.0",
  "exported_at": "2026-07-11T21:30:00Z",
  "generator": "mnemo/0.1.0",
  "embedding": {
    "model": "all-MiniLM-L6-v2",
    "dimensions": 384,
    "normalized": true
  },
  "memory_count": 42,
  "includes_private": false,
  "checksums": {
    "memories.jsonl": "sha256:9f2c...e1a0",
    "embeddings.bin": "sha256:71bd...044c"
  }
}
```

| Field | Type | Required | Description |
| ----- | ---- | -------- | ----------- |
| `format_version` | string | yes | Semver of this spec the file conforms to |
| `exported_at` | string | yes | RFC 3339 UTC timestamp of export |
| `generator` | string | yes | Producing tool, `name/version` |
| `embedding` | object \| null | yes | `null` when `embeddings.bin` is absent |
| `embedding.model` | string | yes* | Embedding model identifier |
| `embedding.dimensions` | int | yes* | Vector size |
| `embedding.normalized` | bool | yes* | Whether vectors are L2-normalized |
| `memory_count` | int | yes | Number of lines in `memories.jsonl` |
| `includes_private` | bool | yes | Whether any `privacy: "private"` memory is included |
| `checksums` | object | yes | SHA-256 (lowercase hex, `sha256:` prefix) per entry; `embeddings.bin` key present only when the entry exists |

\* required when `embedding` is not `null`.

Readers MUST ignore unknown manifest fields. Writers MUST NOT remove required fields
within the same major version.

## 4. `memories.jsonl`

One JSON object per line (JSON Lines). Line order is significant only as the index into
`embeddings.bin`.

### 4.1 Memory schema

| Field | Type | Required | Description |
| ----- | ---- | -------- | ----------- |
| `id` | string | yes | UUIDv7, assigned at creation, stable for the memory's lifetime |
| `content_hash` | string | yes | `sha256:` + lowercase hex SHA-256 of the normalized `text` (§4.2) |
| `type` | string | yes | `fact` \| `episode` \| `procedure` |
| `text` | string | yes | The memory content. Source of truth. Non-empty. |
| `tags` | array of string | yes | May be empty. Lowercase, no duplicates. |
| `privacy` | string | yes | `public` \| `private` |
| `source` | object | yes | Provenance (§4.3) |
| `created_at` | string | yes | RFC 3339 UTC |
| `updated_at` | string | yes | RFC 3339 UTC, ≥ `created_at` |

Readers MUST ignore unknown fields and SHOULD preserve them on re-export.

### 4.2 Text normalization and `content_hash` (normative)

`content_hash` = SHA-256 over the UTF-8 bytes of `normalize(text)`, where `normalize` is,
in order:

1. Unicode normalization form **NFC**
2. Trim leading and trailing whitespace (Unicode `White_Space`)
3. Collapse every internal run of whitespace to a single ASCII space (U+0020)

Nothing else. No case folding (case can be semantically meaningful), no punctuation
stripping. Two implementations MUST produce byte-identical hashes for the same `text`.

Example: `"  My   name is\n Soares "` → `"My name is Soares"` →
`sha256:b0e1…` (hash of those exact UTF-8 bytes).

### 4.3 `source` object

```json
{ "kind": "session", "session_id": "0197f2a1-...", "detail": null }
```

| Field | Type | Description |
| ----- | ---- | ----------- |
| `kind` | string | `session` (extracted from a conversation) \| `manual` (user-typed) \| `import` (came from another `.mind`) |
| `session_id` | string \| null | Originating session UUID when `kind` = `session` |
| `detail` | string \| null | Free text; for `import`, SHOULD name the original pack (e.g. `"go-stdlib-pack.mind"`) |

### 4.4 Examples — one per memory type

**`fact`** — user-confirmed information:

```json
{"id":"0197f2a1-7c3e-7d90-b1a2-3c4d5e6f7a8b","content_hash":"sha256:1f0e2d3c4b5a69788796a5b4c3d2e1f00112233445566778899aabbccddeeff0","type":"fact","text":"The user's name is Soares and their main project is written in Go.","tags":["identity","projects"],"privacy":"private","source":{"kind":"session","session_id":"0197f2a1-0000-7000-8000-000000000001","detail":null},"created_at":"2026-07-10T14:02:11Z","updated_at":"2026-07-10T14:02:11Z"}
```

**`episode`** — summary of a past conversation:

```json
{"id":"0197f2b4-9a1c-7e21-8f00-aabbccddeeff","content_hash":"sha256:aa11bb22cc33dd44ee55ff660718293a4b5c6d7e8f9012345678901234567890","type":"episode","text":"On 2026-07-10 the user debugged a proxy timeout: the cause was a missing context deadline on the upstream HTTP client; fixed by adding a 60s timeout.","tags":["debugging","proxy"],"privacy":"private","source":{"kind":"session","session_id":"0197f2b4-0000-7000-8000-000000000002","detail":null},"created_at":"2026-07-10T18:45:00Z","updated_at":"2026-07-10T18:45:00Z"}
```

**`procedure`** — correction or preference:

```json
{"id":"0197f2c8-1b2d-7f43-9e10-112233445566","content_hash":"sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef","type":"procedure","text":"When the user asks for a commit message, always use conventional commits format (feat:, fix:, docs:, chore:).","tags":["git","preferences"],"privacy":"public","source":{"kind":"manual","session_id":null,"detail":null},"created_at":"2026-07-09T10:00:00Z","updated_at":"2026-07-11T09:30:00Z"}
```

## 5. `embeddings.bin`

Raw concatenated vectors, no header:

- One vector per memory, in the **same order** as `memories.jsonl` lines.
- Each vector is `embedding.dimensions` × float32, little-endian.
- Total size MUST equal `memory_count × dimensions × 4` bytes.

**Validation (normative).** Before reusing vectors, a reader MUST verify all of:

1. `manifest.embedding` is not `null`
2. `len(embeddings.bin) == memory_count × dimensions × 4`
3. Line count of `memories.jsonl` == `memory_count`
4. Checksums in the manifest match both entries

If any check fails, or the local embedding model differs from `manifest.embedding.model`,
the reader MUST discard the vectors and regenerate from `text` (ADR-002). A `.mind` file
without `embeddings.bin` is valid.

## 6. Privacy rules

- Every memory carries `privacy: "public" | "private"`.
- **Default export includes only `public` memories.** Including private memories
  requires an explicit `--include-private` flag; the manifest then sets
  `includes_private: true` so the file is self-describing about its sensitivity.
- Tools SHOULD warn loudly when sharing/exporting with `includes_private: true`.
- Memory **candidates** (not yet human-approved, see DAEMON_DESIGN.md §7) MUST NOT be
  exported: `.mind` files contain only approved memories.
- On import, memories keep their original `privacy` flag.

## 7. Merge and deduplication (import semantics)

Importing a `.mind` file into a local store processes each incoming memory in file
order. Per ADR-003:

```text
for each incoming memory M:
  1. DEDUP    if local store has a memory with M.content_hash → skip M (keep local)
  2. UPDATE   else if local store has a memory with M.id:
                same memory, edited → keep whichever has the newer updated_at
                (tie → keep local)
  3. INSERT   else → insert M, with source.kind = "import" recorded
```

- Rule 1 before rule 2: identical content never duplicates, regardless of IDs.
- The importer MUST report a summary (inserted / updated / skipped) so the human stays
  in the loop.
- Importers SHOULD offer a dry-run mode that prints the summary without writing.

## 8. Versioning and compatibility policy

- `format_version` follows **semver**: `MAJOR.MINOR.PATCH`.
- **MINOR/PATCH** = backward compatible. Readers MUST accept any file whose major
  version they support, ignoring unknown fields/entries (§2, §3, §4.1).
- **MAJOR** = breaking. Readers MUST refuse files with a newer major version, with a
  clear error naming both versions.
- Writers always write the newest version they know.
- Field semantics are never redefined within a major version — breaking meaning requires
  a new field name or a major bump.

## 9. `README.txt` (informative)

Writers MUST include a short plain-text explanation so a person who receives a `.mind`
file and unzips it understands what they are holding. Suggested content:

```text
This is a .mind file — a portable memory export for local LLM assistants.
It was generated by Mnemo (https://github.com/NullVoidDev/mnemo).

- memories.jsonl  contains the memories (one JSON object per line)
- manifest.json   describes the format version and embedding model
- embeddings.bin  optional cached vectors (safe to delete; regenerable)

You can inspect memories.jsonl with any text editor.
```

## 10. Non-goals (v0.1)

- No encryption inside the format (use OS/file-level encryption).
- No sync protocol — `.mind` is a file format, not a service.
- No model weights or fine-tuning data.
- No partial/differential exports (a `.mind` file is always self-contained).
