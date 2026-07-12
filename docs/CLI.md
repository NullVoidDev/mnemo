# Mnemo CLI — v1 Specification

> **Status:** v0.1 — awaiting approval before implementation.
> Related: [DAEMON_DESIGN.md](DAEMON_DESIGN.md) · [MIND_FORMAT.md](MIND_FORMAT.md)

One binary, `mnemo`, with subcommands. Every command except `init` and `serve` is a
thin client over the daemon's admin API (DAEMON_DESIGN.md §4); if the daemon is not
running, commands that need it fail fast with a hint to run `mnemo serve`.

## Global flags

| Flag | Default | Description |
| ---- | ------- | ----------- |
| `--data-dir` | `~/.mnemo` | Data directory (config, SQLite, models) |
| `--json` | off | Machine-readable output (for scripts) |

Exit codes: `0` success · `1` error · `2` usage error.

## `mnemo init`

Prepare the data directory. Idempotent — safe to re-run.

1. Create `<data-dir>/`, default `config.json` (printed for review), empty SQLite store.
2. Download the embedding model (`all-MiniLM-L6-v2` ONNX + `vocab.txt`) and the
   platform's ONNX Runtime library into `<data-dir>/models/`, verifying pinned SHA-256s.
3. Print backend instructions ("point your client at `http://127.0.0.1:8737/v1`").

Flags: `--backend-url <url>` (written into config, default Ollama's
`http://127.0.0.1:11434/v1`), `--skip-download` (offline; embedding unavailable until
models are in place).

## `mnemo serve`

Run the daemon in the foreground (logs to stderr). Flags: `--listen <addr>`.
Stop with Ctrl-C (graceful: closes open sessions, flushes SQLite).

## `mnemo review`

The human-in-the-loop command. First triggers extraction over closed, unextracted
sessions (`POST /mnemo/extract`), then walks through pending candidates interactively:

```text
$ mnemo review
Extracting from 2 closed sessions... 3 new candidates.

[1/3] fact · from session 2026-07-11 18:02
  "The user's project mnemo is written in Go and targets weak PCs."
  (a)pprove  (e)dit then approve  (r)eject  (s)kip  (q)uit → a
  privacy? (P)rivate / p(u)blic → P
  ✓ saved as memory 0197f3ab-...
```

- `edit` opens `$EDITOR` with the candidate text; the edited text is what gets approved.
- Approve prompts for privacy (default **private**) and optional tags.
- Non-interactive flags: `--list` (print pending and exit), `--approve <id>`,
  `--reject <id>`, `--extract-only`.

## `mnemo export`

Write a `.mind` file (MIND_FORMAT.md). Public memories only by default (§6).

```text
$ mnemo export -o my-go-knowledge.mind --tags go,testing
Exported 17 memories (17 public, 0 private) → my-go-knowledge.mind
```

Flags: `-o <path>` (default `mnemo-export-<date>.mind`), `--include-private` (prints a
loud warning), `--tags <a,b>` (filter), `--type <fact|episode|procedure>` (filter).

## `mnemo import`

Merge a `.mind` file into the local store (MIND_FORMAT.md §7).

```text
$ mnemo import go-stdlib-pack.mind --dry-run
go-stdlib-pack.mind: format 0.1.0, 240 memories, embeddings: all-MiniLM-L6-v2 (reusable)
Would insert 232 · update 1 · skip 7 (duplicates)

$ mnemo import go-stdlib-pack.mind
Inserted 232 · updated 1 · skipped 7. Embeddings reused (same model).
```

Flags: `--dry-run`. Incompatible major format version → clear error, exit 1.

## `mnemo search`

Semantic search over memories — for humans, and the honest way to debug retrieval
("why did/didn't X get injected?").

```text
$ mnemo search "how do I like my tests" -k 3
0.71 procedure  Prefer table-driven tests in Go.            [go, testing]
0.55 fact       The user's project mnemo is written in Go.  [projects]
0.41 episode    On 2026-07-10 the user debugged a proxy...  [debugging]
```

Flags: `-k <n>` (default 8), `--type <t>`, `--min <similarity>` (default 0, i.e. show
everything the daemon would see before its threshold).

## `mnemo status`

```text
$ mnemo status
mnemo 0.1.0 · daemon: running (127.0.0.1:8737)
backend: http://127.0.0.1:11434/v1 (reachable, 14 models)
embedding: all-MiniLM-L6-v2 (onnx, 384 dims)
memories: 312 (facts 190 · episodes 87 · procedures 35) · candidates pending: 3
sessions: 1 open · 41 total
```

Exit code reflects health: `0` daemon up, `1` daemon down or backend unreachable.

## Deferred (not in v1)

`mnemo memories rm/edit` beyond the admin API, shell completions, `mnemo doctor`,
pack signing/verification.
