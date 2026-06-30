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
  <a href="https://github.com/SuppieRK/ccp/releases"><img alt="Release" src="https://img.shields.io/github/v/release/SuppieRK/ccp?style=flat-square" /></a>
  <a href="https://github.com/SuppieRK/ccp/actions/workflows/main-validation.yml"><img alt="CI" src="https://github.com/SuppieRK/ccp/actions/workflows/main-validation.yml/badge.svg" /></a>
  <a href="https://sonarcloud.io/summary/new_code?id=SuppieRK_ccp"><img alt="Maintainability Rating" src="https://sonarcloud.io/api/project_badges/measure?project=SuppieRK_ccp&metric=sqale_rating" /></a>
</p>

## Command Compression Proxy for coding agents.

Keep command truth intact while shaping noisy output to fit your workflow. Own deterministic YAML filters, verify them against real output, and keep passthrough when exactness matters.

Use it directly as `ccp <command>`, or install integrations and keep your usual command shape.

Validated on every build across `52` tool families and `382` replay cases.

## See It Work

If you only read one thing, read this.

<table>
  <tr>
    <td valign="top">
      <strong><code>git status</code></strong>
      <pre lang="text"><code>On branch main
Your branch is up to date with 'origin/main'.

Changes not staged for commit:
  (use "git add &lt;file&gt;..." to update what will be committed)
  ...
        modified:   README.md
        modified:   internal/lifecycle/gain.go
        ...

Untracked files:
  ...
        internal/lifecycle/report_text.go</code></pre>
    </td>
    <td valign="top">
      <strong><code>ccp git status</code></strong>
      <pre lang="text"><code>## main...origin/main
 M README.md
 M internal/lifecycle/gain.go
 M internal/lifecycle/gain_global.go
 M internal/lifecycle/gain_test.go
?? internal/lifecycle/report_text.go</code></pre>
    </td>
  </tr>
</table>

Same command. Same result. Less ceremony.

## Output Contract

If output gets compacted too aggressively, the model ends up reconstructing missing context from hints. That is where confusion, extra follow-up commands, and wasted tokens tend to come from. CCP is designed to avoid that failure mode.

- Preserve exit codes and critical diagnostics.
- Compact output into a shorter view that still looks like the command it came from.
- Fall back to native output for ambiguous, structured, precision-sensitive, or machine-oriented cases.
- Leave `--raw` available when exact output matters.
- Let teams tune domain-specific logs with YAML instead of relying on guessed summaries.

The goal is not maximum compression. The goal is a smaller output you can still use.

## Quick Start

Install CCP:

```bash
curl -fsSL https://raw.githubusercontent.com/SuppieRK/ccp/main/scripts/install.sh | sh
```

Try it on a normal command:

```bash
ccp git status
```

Want it wired into supported tools too?

```bash
ccp init --tools claude,codex,opencode
# or: ccp init
```

`ccp init` installs or refreshes integrations in their normal locations. You do not need to run it in every repo. Use `--tools` when you want an explicit one-time setup; omit it when you want CCP to auto-detect supported tools from the current repository.

After `ccp init`, supported tools can keep the normal command shape for you.

Want a quick check after using it for a bit?

```bash
ccp gain
```

Need exact native output for a comparison or edge case?

```bash
ccp --raw git status
```

## Reporting And Real-World Proof

Two real `ccp gain` snapshots from day-to-day work:

- Refactoring tests in a Java project with Gradle (Claude Code):

> Here Claude heavily uses `find` in a suboptimal way and runs Gradle builds to implement the task - good noise reduction opportunity

```text
88 cmds · 5,330,571 → 90,127 tokens (98.3% saved)

Wins  : find (4.8m / 99%) · gradle (367k / 87%) · grep (6k / 1%)
Drag  : cd (23 cmds) · jar (21 cmds) · grep (4 cmds)
Trend : ↑ +12.4 pts week over week (85.9% → 98.3%) · on a roll
```

- Current repository snapshot from `ccp gain` (Codex):

> Here Codex heavily uses `sed` to read files and runs `grep`/`go`/`git` - reading files is passthrough by default, only some outputs were safe to reduce

```text
1,825 cmds · 2,461,959 → 2,160,427 tokens (12.3% saved)

Wins  : grep (67k / 60%) · go (54k / 90%) · git (16k / 59%)
Drag  : sed (765 cmds) · openspec (245 cmds)
Trend : ↓ -2.1 pts week over week (14.4% → 12.3%) · slipping
```

Detailed views stay bounded by default, so they do not take over the terminal:

```bash
ccp gain --table
ccp history
```

Use `--limit 0` when you want the full-text table. JSON and CSV exports ignore `--limit`.

That mix is the point. CCP can save a lot, and it also knows when not to fake it.

Useful commands:

```bash
ccp gain --global
ccp history --global
ccp repair
ccp uninstall --tools codex
ccp uninstall
```

Successful pre-1.0 upgrades run rewrite repair through the newly installed binary, including guarded current-repository CCP migrations. Downgrades are rejected by `ccp upgrade`; uninstall and reinstall explicitly if you need an older version.

## Own The Compression Rules

Own your compression logic: author filters in YAML, ship overridden behavior with your repo, share filters across your team, and fix your edge case today instead of waiting for upstream.

Two filter scopes are built in:

- project-local filters in `./.ccp/filters`
- home-scoped filters in `~/.config/ccp/filters`

Project-local filters are meant to be committed when they describe repo behavior. CCP-generated repo-local state, such as `./.ccp/gain.db`, is ignored by a CCP-owned `./.ccp/.gitignore`; CCP does not need to append broad `.ccp` rules to your repository root `.gitignore` for metrics.

Use `ccp filter prompt` to print an embedded, agent-ready workflow for creating or improving filters. The workflow starts by copying any matching global/home filter into `./.ccp/filters` before editing; global filters are not edited unless you explicitly ask for a global change.

Project scope overrides home scope. That gives you a clean model:

- experiment in one repo without touching anything else
- ship shared defaults across a team
- keep repo-specific overrides without forks
- handle domain-specific logs without pretending one generic filter fits every project

Read more details in [FILTERS.md](FILTERS.md)

## Capability Matrix

| Area              | Tools                                                                                                                                                        |    Representative Savings |
|-------------------|--------------------------------------------------------------------------------------------------------------------------------------------------------------|--------------------------:|
| Files/search      | `find`, `grep`, `ls`                                                                                                                                         |              up to `95%+` |
| Source control    | `git`                                                                                                                                                        |                 `10-60%+` |
| Build systems     | `maven`, `gradle`, `sbt`, `bazel`                                                                                                                            |                 `50-95%+` |
| JS/TS             | `biome`, `bun`, `eslint`, `jest`, `npm`, `nx`, `oxlint`, `pnpm`, `turbo`, `yarn`, `npx`, `node`, `next`, `prettier`, `playwright`, `prisma`, `tsc`, `vitest` |                 `15-85%+` |
| PHP               | `composer`                                                                                                                                                   |                 `10-30%+` |
| Python            | `basedpyright`, `pip`, `poetry`, `pytest`, `mypy`, `ruff`, `ty`, `uv`                                                                                        |                 `25-85%+` |
| Config lint       | `yamllint`, `shellcheck`, `hadolint`                                                                                                                         |                 `10-40%+` |
| Infra and IaC     | `helm`, `terraform`, `tofu`, `tflint`                                                                                                                        |                  `5-90%+` |
| Go/Rust           | `go`, `golangci-lint`, `cargo`, `trunk`                                                                                                                      |                 `35-90%+` |
| Dart/Flutter      | `dart`, `flutter`                                                                                                                                            |                 `35-85%+` |
| Containers        | `docker`                                                                                                                                                     | `95%+` for build surfaces |
| Embedded/Firmware | `pio`                                                                                                                                                        |                 `10-40%+` |
| Elixir            | `mix`                                                                                                                                                        |                  `5-80%+` |
| Other runtimes    | `deno`                                                                                                                                                       |                  `5-80%+` |

Structured, precision, and already-compact modes are intentionally left native when compression would reduce trust.

CCP also integrates with `aider`, `amazon-q`, `antigravity`, `auggie`, `claude`, `cline`, `codebuddy`, `codex`, `crush`, `cursor`, `factory`, `gemini`, `github-copilot`, `kiro`, `kilocode`, `opencode`, `pi`, `qoder`, `qwen`, `roocode`, `trae`, `windsurf`, plus `costrict` as an alias for `roocode`.

## License

See [LICENSE](LICENSE).
