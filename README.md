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
  <a href="#design-choices"><img alt="Status: incubating" src="https://img.shields.io/badge/status-incubating-orange?style=flat-square" /></a>
  <a href="https://github.com/SuppieRK/ccp/releases"><img alt="Release" src="https://img.shields.io/github/v/release/SuppieRK/ccp?style=flat-square" /></a>
</p>

<p align="center">
  <a href="https://github.com/SuppieRK/ccp/actions/workflows/main-validation.yml"><img alt="CI" src="https://github.com/SuppieRK/ccp/actions/workflows/main-validation.yml/badge.svg" /></a>
  <a href="https://sonarcloud.io/summary/new_code?id=SuppieRK_ccp"><img alt="Reliability Rating" src="https://sonarcloud.io/api/project_badges/measure?project=SuppieRK_ccp&metric=reliability_rating" /></a>
  <a href="https://sonarcloud.io/summary/new_code?id=SuppieRK_ccp"><img alt="Security Rating" src="https://sonarcloud.io/api/project_badges/measure?project=SuppieRK_ccp&metric=security_rating" /></a>
  <a href="https://sonarcloud.io/summary/new_code?id=SuppieRK_ccp"><img alt="Maintainability Rating" src="https://sonarcloud.io/api/project_badges/measure?project=SuppieRK_ccp&metric=sqale_rating" /></a>
</p>

<p align="center">
  <a href="#bring-your-own-filter">Bring Your Own Filter</a> &bull;
  <a href="#early-proof">Early Proof</a> &bull;
  <a href="#capability-matrix">Capability Matrix</a> &bull;
  <a href="#quick-start">Quick Start</a> &bull;
  <a href="#design-choices">Design Choices</a>
</p>

## Command Compression Proxy for coding agents

- Run the same shell commands. Keep native exit codes and critical diagnostics.
- Save context when output is genuinely compressible. More room for code, requirements, and reasoning.
- Native output when trust matters more than squeezing tokens. Drop back to `--raw` when exact output matters.

## Bring Your Own Filter

Own your compression logic: author filters in YAML, ship overridden behavior with your repo, share filters across your team, and fix your edge case today instead of waiting for upstream.

Two filter scopes are built in:

- project-local filters in `./.ccp/filters`
- home-scoped filters in `~/.config/ccp/filters`

Project scope overrides home scope. That gives you a clean model:

- experiment in one repo without touching anything else
- ship shared defaults across a team
- keep repo-specific overrides without forks

## Early Proof

Two real `ccp gain` snapshots from day-to-day work:

- Refactoring tests in a Java project with Gradle (Claude Code):

```text
- 88 commands proxied, 5,330,571 estimated input tokens -> 90,127 output tokens, ~98.31% saved
- Biggest gains: find ~98.66% (24 cmds), gradle ~87.07% (5 cmds), grep ~1.44% (4 cmds)
- Savings held down by: cd (23 cmds, no savings), jar (21 cmds, no savings), grep ~1.44% (4 cmds)
- Bottom line: 5,240,444 estimated tokens saved. Breathtaking results, with plenty of context back.
```

- Current repository snapshot from `ccp gain` (Codex):

```text
- 1,825 commands proxied, 2,461,959 estimated input tokens -> 2,160,427 output tokens, ~12.25% saved
- Biggest gains: grep ~59.48% (210 cmds), go ~90.25% (92 cmds), git ~59.25% (40 cmds)
- Savings held down by: sed (765 cmds, no savings), openspec (245 cmds, no savings)
- Bottom line: 301,532 estimated tokens saved. It ain't much, but it's honest work.
```

That mix is the point. CCP can save a lot. It also knows when not to fake it.

## Capability Matrix

Benchmarked with each build across `24` tool families and `201` replay cases.

| Area | Tools | Representative Savings |
|---|---|---:|
| Files/search | `find`, `grep`, `ls` | up to `95%+` |
| Source control | `git` | `10–60%+` |
| Java builds | `maven`, `gradle` | `50–95%+` |
| JS/TS | `npm`, `pnpm`, `yarn`, `npx`, `node`, `next`, `prettier`, `playwright`, `tsc` | `15–85%+` |
| Python | `pip`, `pytest`, `mypy`, `ruff` | `25–85%+` |
| Go/Rust | `go`, `golangci-lint`, `cargo` | `35–90%+` |
| Containers | `docker` | `95%+` for build surfaces |
| Other runtimes | `deno` | targeted, conservative wins |

Structured, precision, and already-compact modes are intentionally left native when compression would reduce trust.

CCP also integrates with these coding agents and editors: `aider`, `amazon-q`, `antigravity`, `auggie`, `claude`, `codebuddy`, `codex`, `crush`, `cursor`, `factory`, `gemini`, `github-copilot`, `iflow`, `kiro`, `kilocode`, `opencode`, `pi`, `qoder`, `qwen`, `roocode`, `trae`, plus `costrict` as an alias for `roocode`.

## Quick Start

Install:

```bash
curl -fsSL https://raw.githubusercontent.com/SuppieRK/ccp/main/scripts/install.sh | sh
```

Check the install:

```bash
ccp --help
```

Initialize supported coding-agent integrations:

```bash
ccp init # or: --tools claude,codex,opencode
```

See what CCP saved in your own work:

```bash
ccp gain
```

Want the detailed table instead of the short summary?

```bash
ccp gain --table
```

## Design Choices

CCP takes a few strong stances on purpose.

- Native commands stay native. CCP adds only a small set of helper commands for filter authoring and gain inspection.
- Exit codes and critical diagnostics matter. By default, CCP preserves them.
- `--raw` is always there when exact output matters more than compression.
- Ambiguous, structured, unsupported, and machine-oriented modes fall back to passthrough.
- Generic log filters are intentionally not shipped. Your logs are domain-specific. Pretending otherwise is how tools become untrustworthy.
- Agent integrations are convenience layers, not the center of the product. The clearest mental model is still explicit `ccp <command>`.

## Inspired By RTK

`ccp` was inspired in part by the [rtk](https://github.com/rtk-ai/rtk) project, but chose a different direction: explicit proxying over a broad helper-command layer, user-owned YAML filters over a mostly built-in command catalog, and easy overrides when needed.

## License

See [LICENSE](LICENSE).
