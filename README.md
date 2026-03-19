<p align="center">
  <a href="https://github.com/SuppieRK/ccp">
    <picture>
      <source srcset="assets/readme-banner.png">
      <img src="assets/readme-banner.png" alt="CCP banner">
    </picture>
  </a>
</p>

<p align="center">
  <a href="./LICENSE"><img alt="License: MIT" src="https://img.shields.io/badge/License-MIT-blue.svg?style=flat-square" /></a>
  <a href="#security--stability"><img alt="Status: incubating" src="https://img.shields.io/badge/status-incubating-orange?style=flat-square" /></a>
  <a href="https://github.com/SuppieRK/ccp/releases"><img alt="Release" src="https://img.shields.io/github/v/release/SuppieRK/ccp?style=flat-square" /></a>
</p>

<p align="center">
  <a href="https://github.com/SuppieRK/ccp/actions/workflows/main-validation.yml"><img alt="CI" src="https://github.com/SuppieRK/ccp/actions/workflows/main-validation.yml/badge.svg" /></a>
  <a href="https://sonarcloud.io/summary/new_code?id=SuppieRK_ccp"><img alt="Reliability Rating" src="https://sonarcloud.io/api/project_badges/measure?project=SuppieRK_ccp&metric=reliability_rating" /></a>
  <a href="https://sonarcloud.io/summary/new_code?id=SuppieRK_ccp"><img alt="Security Rating" src="https://sonarcloud.io/api/project_badges/measure?project=SuppieRK_ccp&metric=security_rating" /></a>
  <a href="https://sonarcloud.io/summary/new_code?id=SuppieRK_ccp"><img alt="Maintainability Rating" src="https://sonarcloud.io/api/project_badges/measure?project=SuppieRK_ccp&metric=sqale_rating" /></a>
</p>

<p align="center">
  <a href="#quick-start">Quick Start</a> &bull;
  <a href="#example-gains">Example Gains</a> &bull;
  <a href="#where-it-helps-less">Where It Helps Less</a> &bull;
  <a href="#usage">Usage</a> &bull;
  <a href="./ARCHITECTURE.md">Architecture</a> &bull;
  <a href="#support--documentation">Support</a>
</p>

## Why It’s Useful

Use `ccp` when coding agents are burning too much context on terminal output.

- More room for code, requirements, and reasoning.
- Same shell commands, exit codes, and critical diagnostics.
- Safe fallback with `--raw` when exact output matters.

Typical fit:
- build failures
- search/listing output
- container and cluster logs

`ccp` is conservative by design. It trims noisy output when safe and leaves structured, compact, or ambiguous cases closer to native output.

## Quick Start

Install:

```bash
curl -fsSL https://raw.githubusercontent.com/SuppieRK/ccp/main/scripts/install.sh | sh
```

Verify the installation:

```bash
ccp --help
```

Initialize supported coding-agent integrations (automatically or with `--tools`):

```bash
ccp init
```

See what it saved in your own work:

```bash
ccp gain
```

Want the detailed table instead of the short summary?

```bash
ccp gain --table
```

## Example Gains

Two real `ccp gain` snapshots from day-to-day work:

- Refactoring tests in a Java project with Gradle (Claude Code):

```
- 88 commands proxied, 5,330,571 estimated input tokens -> 90,127 output tokens, ~98.31% saved
- Biggest gains: find ~98.66% (24 cmds), gradle ~87.07% (5 cmds), grep ~1.44% (4 cmds)
- Savings held down by: cd (23 cmds, no savings), jar (21 cmds, no savings), grep ~1.44% (4 cmds)
- Bottom line: 5,240,444 estimated tokens saved. Breathtaking results, with plenty of context back.
```

- Research task across 4 repositories (Claude Code):

```
- 96 commands proxied, 944,007 estimated input tokens -> 59,195 output tokens, ~93.73% saved
- Biggest gains: find ~93.98% (57 cmds), grep ~42.56% (28 cmds), ls ~79.67% (2 cmds)
- Savings held down by: wc (5 cmds, no savings)
- Bottom line: 884,812 estimated tokens saved. Breathtaking results, with plenty of context back.
```

Short benchmark receipts from CI:

| Command | Scenario | Native tokens | CCP tokens | Savings |
|---|---|---:|---:|---:|
| `./gradlew build` | large generated-build failure | 236 | 47 | 80.08% |
| `./mvnw test` | successful test | 590 | 29 | 95.08% |
| `cargo build` | failing build | 130 | 22 | 83.08% |

## Where It Helps Less

`ccp` is conservative by design. Some commands are already compact, structured, or not worth rewriting.

Current repository snapshot from `ccp gain` (Codex):

```
- 1,825 commands proxied, 2,461,959 estimated input tokens -> 2,160,427 output tokens, ~12.25% saved
- Biggest gains: grep ~59.48% (210 cmds), go ~90.25% (92 cmds), git ~59.25% (40 cmds)
- Savings held down by: sed (765 cmds, no savings), openspec (245 cmds, no savings)
- Bottom line: 301,532 estimated tokens saved. It ain't much, but it's honest work.
```

Short benchmark receipts where savings are limited on purpose:

| Command | Scenario | Native tokens | CCP tokens | Savings |
|---|---|---:|---:|---:|
| `go test -count=1 -json ./...` | structured passthrough safety | 2114 | 2114 | 0.00% |
| `find . -name *.go -type f` | small recursive code search | 176 | 147 | 16.48% |
| `go test -count=1 ./...` | failing test run | 109 | 63 | 42.20% |

Results depend on command mix. Run `ccp gain` after real work to see both the wins and the weak spots in your own repo.

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
| Agent integrations | `aider`, `antigravity`, `amazon-q`, `auggie`, `claude`, `codebuddy`, `codex`, `crush`, `cursor`, `factory`, `gemini`, `github-copilot`, `iflow`, `kiro`, `kilocode`, `opencode`, `pi`, `qoder`, `qwen`, `roocode`, `costrict` (alias of `roocode`), `trae` |

Excluded by design: abstracted meta-commands like `read`, `run`, `shell`, `build`, `test`, `sql`, `logs`, `discover`.

---

## Support & Documentation

Primary documentation:

- [ARCHITECTURE.md](./ARCHITECTURE.md)
- [FILTERS.md](./docs/agent-rules/FILTERS.md)
- [CONTRIBUTING.md](./CONTRIBUTING.md)
- [SECURITY.md](./SECURITY.md)

For issues:

- Open a GitHub issue for bugs or feature requests.
- For vulnerabilities, follow the process in [SECURITY.md](./SECURITY.md).
- For command-output bugs, include the exact command, native `stdout`, native `stderr`, CCP output, `ccp --version`, and OS/shell details.
- One way to collect that information is:

```bash
ccp --version
mkdir -p .artifacts/issue
ccp capture --dir .artifacts/issue -- <command>
ccp verify --dir .artifacts/issue
```

If the command output contains internal package names or other sensitive strings, redact them before sharing the artifacts.

Attach or paste:
- the exact command you ran
- `stdout.txt`
- `stderr.txt`
- `output.txt`

---

## Security & Stability

Project status: incubating (0.y.z zerover).

- Only the latest release is supported.
- No backports or LTS branches.
- Breaking changes may occur prior to 1.0.0.
- Users must upgrade to receive security fixes.

See [SECURITY.md](SECURITY.md) for reporting and scope details.

---

## Inspired By

`ccp` was inspired in part by the [rtk](https://github.com/rtk-ai/rtk) project, which explores agent-oriented command ergonomics from a higher-level task and helper CLI perspective.

You might prefer `ccp` if you want to keep existing shell habits, CI commands, and agent-generated command lines mostly unchanged while still reducing noisy output rather than introducing a broad meta-command layer. The tradeoff is deliberate: less abstraction and fewer helper commands, in exchange for lower migration cost, clearer fallback behavior, and better composability with standard terminal workflows.

That same design also makes `ccp` easier to use across a wider range of coding agents: because it wraps ordinary shell commands instead of introducing an agent-specific task layer, it fits agents that already know how to operate through standard terminal workflows.

---

## License

See [LICENSE](LICENSE).
