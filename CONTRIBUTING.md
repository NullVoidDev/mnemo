# Contributing to Mnemo

Thanks for your interest in contributing! Mnemo is in early development, so things move fast and the design is still taking shape.

## Ground Rules

- **Code and comments in English.**
- **Conventional commits** — small, frequent commits with messages like `feat: add memory ranking`, `fix: handle empty embeddings`, `docs: clarify .mind spec`.
- **Tests from day one** — every package should have `go test` coverage. No heavy dependencies.
- **CPU-first** — everything must run on a weak PC without a GPU. That is the target audience. Reference test: if it works well with LFM2.5-1.2B via Ollama/llama.cpp, it works with anything.
- **Spec before code** — significant design changes (especially to the `.mind` format) should be discussed in an issue and reflected in `docs/` before implementation.

## Getting Started

1. Fork and clone the repository
2. Make sure you have Go 1.26+ installed
3. Run the tests: `go test ./...`
4. Create a branch, make your changes, open a pull request

## Reporting Issues

Open an issue with a clear description. For bugs, include your OS, Go version, the model and runtime you were using (e.g. LFM2.5-1.2B on Ollama), and steps to reproduce.

## The `.mind` Format

The `.mind` specification ([docs/MIND_FORMAT.md](docs/MIND_FORMAT.md)) is the project's most important asset. Changes to it require an issue and discussion before any PR — backward compatibility and portability come first.

## License

By contributing, you agree that your contributions will be licensed under the [MIT License](LICENSE).
