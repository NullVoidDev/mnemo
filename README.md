# Mnemo

> Portable, open, shareable memory layer for small local LLMs. Make weak models smart with a `.mind` file.

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Go Version](https://img.shields.io/badge/Go-1.26+-00ADD8?logo=go)](go.mod)
[![Status](https://img.shields.io/badge/status-early%20development-orange)](#roadmap)
[![PRs Welcome](https://img.shields.io/badge/PRs-welcome-brightgreen.svg)](CONTRIBUTING.md)

🇧🇷 [Leia em português](README.pt-BR.md)

## The Problem

Small local LLMs (1B–3B parameters — LiquidAI LFM2.5-1.2B, small Qwen models, etc.) run on any PC without a good GPU. But they are limited, and they remember **nothing** between sessions. Fine-tuning them locally is impractical for most people.

Every conversation starts from zero. Your corrections, your preferences, your context — all gone.

## The Solution

Mnemo is a local daemon that sits between you and the model — an OpenAI-compatible API proxy that works with Ollama, llama.cpp, LM Studio, and anything else that speaks the same protocol. It automatically injects relevant memories into the context window.

**The model stays the same. The system gets smarter over time.**

```text
┌──────────┐      ┌───────────────────┐      ┌──────────────┐
│  Client  │ ───> │   Mnemo daemon    │ ───> │  Local LLM   │
│ (any UI) │ <─── │  (memory proxy)   │ <─── │ (Ollama etc) │
└──────────┘      └────────┬──────────┘      └──────────────┘
                           │
                     ┌─────┴─────┐
                     │  .mind /  │
                     │  SQLite   │
                     └───────────┘
```

## The `.mind` Format — the heart of the project

An open, portable, model-independent memory file. Think of what `.gguf` did for weights — `.mind` does the same for memory.

With a `.mind` file you can:

- **Switch models** (LFM → Qwen → anything) and keep your entire "mind"
- **Export** your memory as a file and share it with other people
- **Import** community-made thematic knowledge packs
- Control privacy with per-memory public/private flags

See the draft specification: [docs/MIND_FORMAT.md](docs/MIND_FORMAT.md)

## Three Layers of Memory

| Layer | What it stores | Example |
| ----- | -------------- | ------- |
| **Facts** | Information extracted and confirmed by the user | "my name is X", "my project uses Go" |
| **Episodes** | Summaries of past conversations | "on July 10 we debugged the proxy timeout" |
| **Procedures** | Corrections and preferences | "when I ask for X, do Y" |

## Human-in-the-Loop Learning

After each session, Mnemo extracts memory candidates and the user approves or rejects them via CLI. There is **no automatic self-learning in v1** — 1.2B models are not reliable enough for self-evaluation.

## Architecture (v1 / MVP)

- **Language:** Go — single binary, runs anywhere, no dependencies
- **Storage:** SQLite — single file, trivially easy to export
- **Embeddings:** local, on CPU, via a small embedding model
- **Interface:** CLI + HTTP daemon. No GUI in v1.
- **Target hardware:** a weak PC with no GPU. If it works well with LFM2.5-1.2B via Ollama/llama.cpp, it works with anything.

```text
cmd/mnemo/        CLI entry point
internal/memory/  memory layers, extraction, ranking
internal/proxy/   OpenAI-compatible HTTP proxy
internal/store/   SQLite persistence
internal/embed/   local embedding generation
docs/             specifications and design docs
```

## Roadmap

- [x] Project structure and repository
- [ ] `.mind` format specification ([draft](docs/MIND_FORMAT.md))
- [ ] Daemon design: endpoints, memory injection flow, SQLite schema
- [ ] SQLite store
- [ ] Local embeddings on CPU
- [ ] OpenAI-compatible proxy with memory injection
- [ ] CLI: session review, approve/reject memory candidates
- [ ] `.mind` export/import
- [ ] Community knowledge packs

## Contributing

Contributions are welcome! See [CONTRIBUTING.md](CONTRIBUTING.md).

## License

[MIT](LICENSE) — anyone can download, modify, and use.
