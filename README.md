# Command Compression Proxy (`ccp`)

[![Go Version](https://img.shields.io/badge/go-1.26%2B-blue)](#development-environment)
[![License](https://img.shields.io/badge/license-MIT-green)](LICENSE)
[![Status](https://img.shields.io/badge/status-incubating-orange)](#security--stability)
[![Reliability Rating](https://sonarcloud.io/api/project_badges/measure?project=SuppieRK_ccp&metric=reliability_rating)](https://sonarcloud.io/summary/new_code?id=SuppieRK_ccp)
[![Security Rating](https://sonarcloud.io/api/project_badges/measure?project=SuppieRK_ccp&metric=security_rating)](https://sonarcloud.io/summary/new_code?id=SuppieRK_ccp)
[![Maintainability Rating](https://sonarcloud.io/api/project_badges/measure?project=SuppieRK_ccp&metric=sqale_rating)](https://sonarcloud.io/summary/new_code?id=SuppieRK_ccp)

`ccp` is an agent-first command proxy written in Go.

It executes native system commands and compacts their output to reduce tokens consumed by coding agents while preserving execution correctness, exit codes, critical diagnostics, and downstream usability.

This project prioritizes deterministic, machine-consumable correctness over terminal-faithful human presentation and favors shape-preserving compaction so coding agents can still interpret output and compose follow-up shell commands naturally.

## Why It’s Useful

`ccp` optimizes for useful compression, not maximal compression.

- Removes repetitive or low-signal output when safe.
- Favors filtering and omission over denser non-native output encodings.
- Preserves actionable diagnostics and line-oriented output affordances when possible.
- Keeps deterministic behavior for identical inputs.
- Emits zero bytes if native output is zero bytes.
- Preserves exact byte stream in `--raw` mode.

For coding agents operating in large repositories, this reduces context size without sacrificing correctness or downstream operability.

---

## Getting Started

Install with the provided script:

```bash
curl -fsSL https://raw.githubusercontent.com/SuppieRK/ccp/main/scripts/install.sh | sh
```

Or build from source (`Go 1.26+`):

```bash
go build -o ccp ./cmd/ccp
install -m 0755 ./ccp /usr/local/bin/ccp
```

Verify the installation:

```bash
ccp --version
```

Initialize supported agent integrations:

```bash
ccp init
```

Or select tools explicitly:

```bash
ccp init --tools claude,codex
```

Uninstall:

```bash
ccp uninstall
```

---

## Usage

Primary use case: initialize supported coding-agent integrations so agent shell commands are routed through `ccp`.

```bash
ccp init
```

You can also wrap commands directly in local workflows or CI when you want smaller logs without changing the underlying command behavior:

```bash
ccp ls -la
ccp git status
ccp grep -R "TODO" .

# byte-for-byte passthrough
ccp --raw git status
```

Chained and piped commands should prefix each executable:

```bash
ccp echo chain-ok && ccp echo chain-done
ccp false || ccp echo chain-recovered
ccp nl -ba spec.md | ccp sed -n '1,260p'
```

---

## Capability Matrix

| Area | Supported |
|---|---|
| Files/search | `ls`, `find`, `grep` |
| Source control | `git` |
| Containers/cluster | `docker`, `kubectl` |
| Java/build | `gradle`, `maven` |
| JavaScript/TypeScript | `npm`, `pnpm`, `yarn`, `npx`, `node`, `deno` |
| Python/Go/Rust | `pip`, `python`, `pytest`, `go`, `cargo` |

Excluded by design: abstracted meta-commands like `read`, `run`, `shell`, `build`, `test`, `sql`, `logs`, `discover`.

---

## Support & Documentation

Primary documentation:

- [ARCHITECTURE.md](./ARCHITECTURE.md)
- [CONTRIBUTING.md](./CONTRIBUTING.md)
- [SECURITY.md](./SECURITY.md)

For issues:

- Open a GitHub issue for bugs or feature requests.
- For vulnerabilities, follow the process in [SECURITY.md](./SECURITY.md).
- For command-output bugs, include the exact command, native `stdout`, native `stderr`, CCP output, `ccp --version`, and OS/shell details.
- One way to collect that information is:

```bash
# version
ccp --version

# native stdout/stderr in timestamped capture files
mkdir -p .artifacts/issue
ccp --capture-raw --capture-raw-dir .artifacts/issue <command>

# CCP output shown on the terminal
ccp <command> > .artifacts/issue/ccp.stdout 2> .artifacts/issue/ccp.stderr
```

If the raw captures contain internal package names or other sensitive strings, add `--confidential value1,value2,...` to the `--capture-raw` command to redact those substrings in the capture files before sharing them. This does not redact the separate `ccp <command>` output file, so review that file manually before posting.

Attach or paste:
- the exact command you ran
- `.artifacts/issue/ccp-capture-*-input-stdout.txt`
- `.artifacts/issue/ccp-capture-*-input-stderr.txt`
- `.artifacts/issue/ccp.stdout`
- `.artifacts/issue/ccp.stderr` if relevant

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
