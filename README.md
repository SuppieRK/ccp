# Command Compression Proxy (`ccp`)

[![Go Version](https://img.shields.io/badge/go-1.26%2B-blue)](#development-environment)
[![License](https://img.shields.io/badge/license-MIT-green)](LICENSE)
[![Status](https://img.shields.io/badge/status-incubating-orange)](#security--stability)
[![Reliability Rating](https://sonarcloud.io/api/project_badges/measure?project=SuppieRK_ccp&metric=reliability_rating)](https://sonarcloud.io/summary/new_code?id=SuppieRK_ccp)
[![Security Rating](https://sonarcloud.io/api/project_badges/measure?project=SuppieRK_ccp&metric=security_rating)](https://sonarcloud.io/summary/new_code?id=SuppieRK_ccp)
[![Maintainability Rating](https://sonarcloud.io/api/project_badges/measure?project=SuppieRK_ccp&metric=sqale_rating)](https://sonarcloud.io/summary/new_code?id=SuppieRK_ccp)

`ccp` is an agent-first command proxy written in Go.

It executes native system commands and compacts their output to reduce tokens consumed by coding agents while preserving execution correctness, exit codes, critical diagnostics, and downstream usability.

It stays close to native commands. The goal is smaller output, not a new command language.

## Why It’s Useful

`ccp` reduces tokens consumed by command output, which leaves more room in the model context window for actual code, requirements, and reasoning. In practice, that gives coding agents more usable context to work with, improves the odds of better follow-up results, and lowers usage cost at the same time.

- Removes repetitive or low-signal output when safe.
- Favors filtering and omission over denser non-native output encodings.
- Preserves actionable diagnostics and line-oriented output affordances when possible.
- Keeps deterministic behavior for identical inputs.
- Emits zero bytes if native output is zero bytes.
- Preserves exact byte stream in `--raw` mode unless explicit redaction is enabled with `--confidential`.

## Example Gains

Two real `ccp gain` snapshots from day-to-day work:

- Research task across 4 repositories (Claude Code):

```
- 96 commands proxied, 944007 estimated input tokens -> 59195 output tokens, ~93.73% saved
- Biggest gains: find ~93.98% (57 cmds), grep ~42.56% (28 cmds), ls ~79.67% (2 cmds)
- Savings held down by: wc ~0.00% (5 cmds)
- Bottom line: 884812 estimated tokens saved while preserving native execution semantics
```

- Refactoring tests in a Java project with Gradle (Claude Code):

```
- 88 commands proxied, 5330571 estimated input tokens -> 90127 output tokens, ~98.31% saved
- Biggest gains: find ~98.66% (24 cmds), gradle ~87.07% (5 cmds), grep ~1.44% (4 cmds)
- Savings held down by: cd ~0.00% (23 cmds), jar ~0.00% (21 cmds), grep ~1.44% (4 cmds)
- Bottom line: 5240444 estimated tokens saved while preserving native execution semantics
```

Short benchmark receipts from CI:

| Command | Scenario | Native tokens | CCP tokens | Savings |
|---|---|---:|---:|---:|
| `./gradlew build` | large generated-build failure | 22,917 | 1,132 | 95.06% |
| `docker logs <container>` | noisy container logs | 1009 | 19 | 98.12% |
| `cargo build` | failing build | 280 | 32 | 88.57% |

## Where It Helps Less

`ccp` is conservative by design. Some commands are already compact, structured, or not worth rewriting.

Current repository snapshot from `ccp gain` (Codex):

```
- 1243 commands proxied, 1627069 estimated input tokens -> 1287154 output tokens, ~20.89% saved
- Biggest gains: grep ~55.35% (151 cmds), go ~93.25% (43 cmds), git ~62.91% (30 cmds)
- Savings held down by: sed ~0.00% (536 cmds), openspec ~0.00% (226 cmds)
- Bottom line: 339915 estimated tokens saved while preserving native execution semantics
```

Short benchmark receipts where savings are limited on purpose:

| Command | Scenario | Native tokens | CCP tokens | Savings |
|---|---|---:|---:|---:|
| `docker ps --format {{json .}}` | structured passthrough safety | 169 | 167 | 0.00% |
| `find . -name *.go -type f` | small recursive code search | 202 | 171 | 15.35% |
| `go test -count=1 ./...` | failing test run | 130 | 80 | 38.46% |

Results depend on command mix. Run `ccp gain` after real work to see both the wins and the weak spots in your own repo.

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
ccp init --tools claude,codex,cursor,gemini,github-copilot
```

Check what `ccp` is saving on your own work:

```bash
ccp gain
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

Then verify the effect on real commands:

```bash
ccp gain
ccp gain --table
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

If compressed output seems corrupted, malformed, or unusable for the task, retry with `ccp --raw` as an escape hatch before falling back to the unwrapped command.

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
| Agent integrations | `aider`, `amazon-q`, `cline`, `claude`, `codex`, `continue`, `cursor`, `gemini`, `github-copilot`, `kiro`, `opencode`, `roocode`, `trae`, `windsurf` |

Excluded by design: abstracted meta-commands like `read`, `run`, `shell`, `build`, `test`, `sql`, `logs`, `discover`.

---

## Inspired By

`ccp` was inspired in part by the [rtk](https://github.com/rtk-ai/rtk) project, which explores agent-oriented command ergonomics from a higher-level task and helper CLI perspective.

You might prefer `ccp` if you want to keep existing shell habits, CI commands, and agent-generated command lines mostly unchanged while still reducing noisy output rather than introducing a broad meta-command layer.. The tradeoff is deliberate: less abstraction and fewer helper commands, in exchange for lower migration cost, clearer fallback behavior, and better composability with standard terminal workflows.

That same design also makes `ccp` easier to use across a wider range of coding agents: because it wraps ordinary shell commands instead of introducing an agent-specific task layer, it fits agents that already know how to operate through standard terminal workflows.

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

If the command output contains internal package names or other sensitive strings, add `--confidential value1,value2,...` to redact those substrings from `ccp` output before sharing it. When combined with `--capture-raw`, the same redactions are also applied to the capture files.

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
