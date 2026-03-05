# Command Compression Proxy (`ccp`)

[![Go Version](https://img.shields.io/badge/go-1.24%2B-blue)](#development-environment)
[![License](https://img.shields.io/badge/license-MIT-green)](LICENSE)
[![Status](https://img.shields.io/badge/status-incubating-orange)](#security--stability)
[![Reliability Rating](https://sonarcloud.io/api/project_badges/measure?project=SuppieRK_ccp&metric=reliability_rating)](https://sonarcloud.io/summary/new_code?id=SuppieRK_ccp)
[![Security Rating](https://sonarcloud.io/api/project_badges/measure?project=SuppieRK_ccp&metric=security_rating)](https://sonarcloud.io/summary/new_code?id=SuppieRK_ccp)
[![Maintainability Rating](https://sonarcloud.io/api/project_badges/measure?project=SuppieRK_ccp&metric=sqale_rating)](https://sonarcloud.io/summary/new_code?id=SuppieRK_ccp)

`ccp` is an agent-first command proxy written in Go.

It executes native system commands and compacts their output to reduce tokens consumed by coding agents while preserving execution correctness, exit codes, and critical diagnostics.

This project prioritizes deterministic, machine-consumable correctness over terminal-faithful human presentation.

---

## What This Project Does

`ccp` sits in front of native CLI tools and:

- Executes the real command.
- Processes `stdout` and `stderr`.
- Applies tool-specific compaction logic.
- Preserves exit code parity and critical diagnostics.
- Falls back to passthrough on ambiguity or unsafe shapes.

Supported command surfaces include:

- Files/search: `ls`, `find`, `grep`
- Source control: `git`
- Containers/cluster: `docker`, `kubectl`
- Java/build: `gradle`, `maven`
- JavaScript/TypeScript: `npm`, `pnpm`, `yarn`, `npx`, `node`, `deno`
- Python/Go/Rust: `pip`, `python`, `pytest`, `go`, `cargo`

Excluded by design: abstracted meta-commands like `read`, `run`, `shell`, `build`, `test`, `sql`, `logs`, `discover`.

---

## Why It’s Useful

### Primary Optimization Target

The core KPI is output reduction as a proxy for reduced token consumption in agent workflows.

`ccp`:

- Removes repetitive or low-signal output when safe.
- Preserves actionable diagnostics.
- Keeps deterministic behavior for identical inputs.
- Emits zero bytes if native output is zero bytes.
- Preserves exact byte stream in `--raw` mode.

### Deterministic & Safe by Design

The proxy guarantees:

- Exit code parity.
- Preservation of critical errors.
- Passthrough on ambiguity.
- Stable deterministic output.
- Isolation of command contexts.

For coding agents operating in large repositories, this materially reduces context size without sacrificing correctness.

---

## Execution modes

- **Semantic mode (default)** — compaction enabled.
- **`--raw`** — byte-for-byte passthrough.
- **`--strict`** — reject ambiguous plans.
- **`--capture-raw`** — preserve raw artifacts while executing normally.

---

## Getting Started

### Installer Script

```bash
curl -fsSL https://raw.githubusercontent.com/SuppieRK/ccp/main/scripts/install.sh | sh
```

### Build and Install from Source

> Requires Go 1.24+

```bash
go build -o ccp ./cmd/ccp
./ccp --version
```

Install system-wide:

```bash
install -m 0755 ./ccp /usr/local/bin/ccp
ccp --version
```

---

## Basic Usage

Wrap native commands directly:

```bash
ccp ls -la
ccp git status
ccp grep -R "TODO" .
```

Shell-style chains:

```bash
ccp echo a && echo b
ccp "echo a && echo b"
```

Raw passthrough:

```bash
ccp --raw git status
```

Strict mode:

```bash
ccp --strict git log
```

---

## Support & Documentation

Primary documentation:

- [ARCHITECTURE.md](./ARCHITECTURE.md)
- [CONTRIBUTING.md](./CONTRIBUTING.md)
- [SECURITY.md](./SECURITY.md)

For issues:

- Open a GitHub issue for bugs or feature requests.
- For vulnerabilities, follow the process in [SECURITY.md](./SECURITY.md).

---

## Security & Stability

Project status: incubating (0.y.z zerover).

- Only the latest release is supported.
- No backports or LTS branches.
- Breaking changes may occur prior to 1.0.0.
- Users must upgrade to receive security fixes.

See [SECURITY.md](SECURITY.md) for reporting and scope details.

---

## License

See [LICENSE](LICENSE).