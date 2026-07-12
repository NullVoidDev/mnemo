# The `.mind` Format — Draft Specification

> **Status:** DRAFT v0. Nothing here is final. This document exists to be discussed, torn apart, and rewritten before any code is written.

## What is a `.mind` file?

A `.mind` file is a portable, open, model-independent memory interchange format for LLM memory systems. What `.gguf` is for model weights, `.mind` aims to be for memory: a single file you can move between machines, models, and tools.

## Design Goals

1. **Model-independent** — memories survive switching from LFM to Qwen to anything else. Text is the source of truth; embeddings are a derived, replaceable cache.
2. **Portable** — a single file. Copy it, back it up, send it to a friend.
3. **Shareable** — per-memory privacy flags make it safe to export a public subset ("knowledge packs").
4. **Open** — fully specified, no proprietary blobs, implementable by anyone.
5. **Inspectable** — a human should be able to open a `.mind` file and understand what the system knows about them.

## Open Questions (to resolve before v1)

These must be decided together before freezing the spec:

- **Container:** single SQLite file vs. zip/tar archive with JSON + binary embeddings vs. custom binary format?
- **Embeddings:** embedded in the file or always regenerated on import? (Regeneration is slow on weak CPUs; embedding models differ across users.)
- **Versioning:** how do we evolve the format without breaking old files?
- **Identity:** should memories have stable IDs (UUIDs? content hashes?) to support merging two `.mind` files?
- **Compression:** worth it for large memory sets?

## Tentative Structure

A `.mind` file contains:

### Header / Metadata

| Field | Description |
| ----- | ----------- |
| `format_version` | Version of the `.mind` spec (semver) |
| `created_at` / `updated_at` | Timestamps (UTC, RFC 3339) |
| `embedding_model` | Identifier of the model used for embeddings (e.g. `all-MiniLM-L6-v2`), so importers know whether they can reuse them |
| `embedding_dimensions` | Vector size |
| `generator` | Tool that produced the file (e.g. `mnemo/0.1.0`) |

### Memories

Each memory record has:

| Field | Description |
| ----- | ----------- |
| `id` | Stable identifier |
| `type` | `fact` \| `episode` \| `procedure` |
| `content` | The memory text (source of truth) |
| `privacy` | `private` \| `public` — public memories are included in shared exports |
| `created_at` | When the memory was created |
| `confirmed` | Whether the user explicitly approved this memory |
| `embedding` | Optional cached vector (regenerable from `content`) |
| `metadata` | Free-form key/value (source session, tags, etc.) |

### Memory Types

- **`fact`** — user-confirmed information: "my name is X", "my project uses Go"
- **`episode`** — summary of a past conversation
- **`procedure`** — correction or preference: "when I ask for X, do Y"

## Sharing Semantics

- Exporting a full `.mind` file includes everything (personal backup).
- Exporting a **pack** includes only `public` memories.
- Importing merges memories into the local store; conflicts and duplicates are surfaced to the user for review (human in the loop, consistent with Mnemo's learning cycle).

## Non-Goals (v1)

- No encryption inside the format (use OS/file-level encryption).
- No sync protocol — `.mind` is a file format, not a service.
- No model weights or fine-tuning data.
