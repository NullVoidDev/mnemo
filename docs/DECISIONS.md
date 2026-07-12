# Architecture Decision Records

Decisions are numbered, dated, and never deleted — superseded ADRs are marked as such.
Each ADR may have a **Concerns** section: technical objections recorded at decision time.
The decision stands as written; concerns exist so future revisions start from an honest record.

---

## ADR-001 — `.mind` container: ZIP archive with JSON + binary embeddings

**Date:** 2026-07-11 · **Status:** Accepted

### Context

The `.mind` interchange format needs a container. Options considered:

1. Raw SQLite file (the daemon's working store doubles as the format)
2. ZIP archive with JSON text + binary embeddings
3. Custom binary format

### Decision

`.mind` is a **ZIP archive** with its own extension, containing:

| Entry | Content |
| ----- | ------- |
| `manifest.json` | Format version, export date, generator tool, embedding model (name + dimensions), memory count |
| `memories.jsonl` | One memory per line — inspectable with any text editor |
| `embeddings.bin` | Vectors as float32 little-endian, in the same order as the JSONL (optional, see ADR-002) |
| `README.txt` | Human-readable explanation of what the file is |

SQLite remains the daemon's **working** storage. `.mind` is export/import only.

### Rationale

An interchange format must be specifiable independently of any implementation. Tying the
format to SQLite ties it to one library's file layout. A custom binary format is
over-engineering for v1. ZIP + JSONL is implementable in any language with a standard
library, and a curious user can unzip it and read their own memories.

### Concerns

- **Index-based coupling between `memories.jsonl` and `embeddings.bin` is fragile**: a
  tool that reorders or filters lines without rewriting the binary file corrupts every
  subsequent vector silently. Mitigation adopted in the spec: the manifest carries SHA-256
  checksums of both entries and importers MUST verify count and size invariants before
  trusting vectors (see MIND_FORMAT.md §5). A future revision could store per-record
  vector offsets or move vectors into the JSONL as base64, trading size for robustness.

---

## ADR-002 — Embeddings travel inside the file but are disposable

**Date:** 2026-07-11 · **Status:** Accepted

### Decision

- The manifest declares the embedding model (name + dimensions).
- If the importer uses the **same** model, it reuses the vectors (instant import).
- If the model differs or vectors are absent, the importer regenerates embeddings
  locally from the text.
- **Text is always the source of truth; embeddings are cache.**

### Rationale

Model independence is a pillar of the project. Shipping vectors makes the common case
(same default model, `all-MiniLM-L6-v2`) instant on weak CPUs; making them disposable
keeps the format honest about what actually matters — the text.

### Concerns

None. This is the right call.

---

## ADR-003 — Stable IDs: UUIDv7 + content hash

**Date:** 2026-07-11 · **Status:** Accepted

### Decision

- Every memory gets a **UUIDv7** at creation (time-ordered, no central coordination).
- Every memory also carries a **`content_hash`**: SHA-256 of the normalized text, used
  for deduplication on merge — two `.mind` files containing the same memory do not
  produce a duplicate.
- **Merge order:** first dedup by `content_hash`; then match by UUID (same memory,
  edited → the record with the most recent `updated_at` wins).

### Rationale

UUIDv7 gives globally unique, time-sortable IDs with zero coordination. Content hashing
catches the common sharing case: the same knowledge-pack memory arriving twice via
different paths.

### Concerns

- **"Latest `updated_at` wins" trusts wall clocks.** Two machines with skewed clocks can
  resolve an edit conflict the wrong way. Acceptable for v1 (human reviews imports
  anyway); a future revision could add a logical version counter.
- **Text normalization must be specified exactly**, or two implementations will compute
  different hashes for the same memory and dedup breaks. The normalization algorithm is
  therefore normative in MIND_FORMAT.md §4.2 — deliberately minimal (NFC, trim, collapse
  whitespace) and **case-preserving**, because case can be semantically meaningful.

---

## ADR-004 — Local embeddings: ONNX in-process by default

**Date:** 2026-07-11 · **Status:** Accepted

### Decision

- Default embedder: **`all-MiniLM-L6-v2`** (384 dimensions) running via ONNX Runtime
  inside the Go binary. Library evaluation and recommendation: DAEMON_DESIGN.md §6
  (recommendation: `onnxruntime_go` + a minimal in-tree WordPiece tokenizer).
- The `Embedder` interface in `internal/embed` allows swapping backends; an **optional**
  Python microservice backend is configurable for users who want a different model.
- The ONNX model is **downloaded on first use** (`mnemo init`), never embedded in the
  binary.

### Rationale

Single binary, CPU-only, weak-PC-first. In-process inference avoids a mandatory second
process; the interface keeps the door open without making anyone run Python.

### Concerns

- **ONNX Runtime is a native shared library, not Go code.** Any Go binding (cgo or
  runtime dynamic loading) means the deployment is really "binary + `libonnxruntime` +
  model file", which weakens — but does not break — the single-binary story. Mitigation:
  `mnemo init` downloads both the model and the matching ONNX Runtime library into the
  data directory, so the user experience stays "one command". A pure-Go inference path
  does not exist today at acceptable quality/speed for MiniLM.
- **cgo complicates cross-compilation.** Accepted cost for v1; the SQLite driver is
  chosen pure-Go (see DAEMON_DESIGN.md §5) precisely so ONNX remains the only native
  dependency.
