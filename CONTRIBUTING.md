# Contributing to cmdshape

Thanks for taking a look. cmdshape is deliberately conservative: a smaller
answer is useful only when an agent can still trust it. Start with a focused
issue or pull request, explain the command shape you are changing, and keep
the smallest safe patch.

## Project principles

cmdshape is a command proxy, not a replacement implementation of native tools.
Changes should:

- preserve the native executable, exit status, critical diagnostics, exact
  `--raw` path, and zero-byte semantics;
- prefer shape-preserving filtering over a new output language;
- fall back to passthrough for ambiguity, structured or precision-sensitive
  output, and interactive or TTY-dependent behavior;
- keep command-aware behavior deterministic and isolated from other tools;
- keep tool-specific behavior in YAML when the current DSL can express it;
- use real captured output and representative failure cases as evidence.

Avoid reimplementing behavior already owned by the native executable. A
smaller output is not an improvement when it becomes harder to use in a
follow-up command.

## Development environment

- Use Go 1.26 or newer, as declared by `go.mod`.
- Run commands from the repository root.
- Treat CI as the canonical definition of release builds, supported targets,
  and validation gates.
- Keep repository versions in exact `X.Y.Z` form without a leading `v`.
- Preserve unrelated working-tree changes; do not rewrite generated artifacts
  or benchmark results manually.

## Where changes belong

- Runtime behavior belongs in the owning `internal/` package.
- Filter behavior belongs in YAML. Read [FILTERS.md](FILTERS.md) before adding
  or changing a filter, mapping, capture, or fixture.
- Agent integrations belong under `internal/lifecycle/agents` and their
  managed artifacts. Keep adapter-specific behavior out of shared runtime
  packages.
- Documentation should describe observed behavior, not aspirational savings or
  undocumented integrations.

Preserve these invariants:

- native execution and exit-code parity;
- critical diagnostics, warnings, and failure context;
- exact `--raw` behavior and zero-byte output semantics;
- deterministic command-shape isolation;
- passthrough for ambiguous, structured, interactive, or unsafe cases;
- shell-usable output where the native format supports it.

Do not edit generated benchmark artifacts by hand or bypass benchmark gates.
Existing tests are part of the contract; behavioral changes need coverage, but
tests must not be weakened to make a change pass.

## Scope expectations

Keep a pull request focused on one behavior or documentation goal. Do not mix a
filter improvement with an unrelated runtime refactor, adapter cleanup, or
formatting sweep.

Use the narrowest owning package:

- parser and planner decisions belong near command parsing and dispatch;
- streaming and ordered-output behavior belongs in `internal/engine`;
- YAML schema, validation, loading, and compilation belong in
  `internal/filters/yaml`;
- metrics, reporting, and retention belong in `internal/metrics`;
- lifecycle commands and integrations belong in `internal/lifecycle`;
- tool-specific line behavior belongs in `filters/<tool>.yaml`.

Discuss a DSL change before implementing it. The existing vocabulary should be
preferred unless a concrete, representative command cannot be expressed
safely.

## Filter development

Read [FILTERS.md](FILTERS.md) for the complete authoring and trust workflow.
For a built-in change:

1. Inspect the current source and mappings.
2. Start from a real capture, not an invented success-only example.
3. Model structured and precision-sensitive modes as passthrough first.
4. Change one hypothesis at a time.
5. Verify success, warning, failure, empty, structured, and unmodeled shapes
   that are relevant to the tool.
6. Preserve paths, identifiers, diagnostics, and shell-usable lines.
7. Add or update deterministic replay fixtures.

Single tools normally use one YAML file. Tool families should share a
canonical filter when their command surface is genuinely one behavior model;
wrapper spellings belong in `.mappings.yaml`.

Do not add bespoke Go filters. Shared capabilities belong in the canonical
runtime only when existing YAML cannot express a justified cross-tool need.

## Adapter development

Coding-agent integrations are managed adapters, not free-form installer
scripts. Preserve each adapter's ownership boundary:

- hooks and plugins must route commands only where the agent contract permits;
- instruction, rule, and context adapters must update their canonical target
  without erasing user-owned content;
- initialization and uninstall must be idempotent;
- detection must not imply that every adapter uses automatic interception;
- adapters must register in the runtime-owned inventory so generated CLI facts
  remain authoritative.

Prefer family conformance coverage for adapters that share recipes. Keep
bespoke tests only for behavior that is actually unique.

## Validation by change type

For Go or runtime changes, use Go 1.26 or newer and run the repository check
from the root:

```bash
./scripts/validate.sh
```

The script is the canonical full check and includes formatting, tests, race,
coverage, and benchmark gates. Treat any non-zero exit as a failure.

For filter or fixture changes, run a focused loop first:

```bash
cmdshape filter status
cmdshape verify --dir path/to/fixture
go test -count=1 ./internal/filters/... ./internal/benchmark ./cmd/cmdshape-ci
```

Built-in filter changes also need representative success, warning, failure, and
passthrough fixtures under `testdata/benchmarks/<tool>/`. Capture them with
`cmdshape capture`, review sensitive output, and assert the selected
`filter|case` in `dispatch.txt`.

For lifecycle or adapter changes, run the owning package tests and regenerate
runtime-owned facts when the command or integration inventory changes:

```bash
go run ./cmd/cmdshape-docgen
go test -count=1 ./cmd/cmdshape-docgen ./internal/lifecycle/...
```

Docs-only changes do not need the full Go validation script. Markdown-only
changes are excluded from the Go validation workflows; site files still follow
the Pages workflow. Check links, code blocks, generated facts, and the rendered
site when site copy changes. Never modify existing tests for an editorial pass.

## Testing expectations

Behavioral changes require coverage at the layer that owns the decision:

- parser or planner changes need command-shape, flag, ambiguity, normalization,
  dispatch, and passthrough cases;
- runner changes need native execution, exit-code, raw-mode, stdin, and
  stdout/stderr integration coverage;
- engine changes need action ordering, buffering, stream identity, empty
  output, and lifecycle emission coverage;
- loader changes need source precedence, trust, invalid definitions, mappings,
  and passthrough fallback coverage;
- lifecycle changes need command help, flag validation, file ownership,
  failure, and idempotency coverage;
- filter changes need replay fixtures rather than wrapper-only unit tests.

Parent-command tests should verify routing and broad passthrough only. Detailed
behavior belongs in the command family or package that owns it. Prefer shared
table or conformance coverage over repeated one-off tests.

Existing tests express the current contract. Do not rewrite, weaken, or remove
them merely to accommodate an implementation. New tests may be added for new
behavior, and obsolete tests may be removed only when the behavior they cover
is deliberately removed.

### Planning changes versus runtime changes

Planning changes decide what will run:

- tool and filter selection;
- argv and positional parsing;
- normalization;
- dispatch;
- ambiguity and passthrough selection.

Runtime changes decide how the selected command executes:

- process lifecycle and exit status;
- stdin, stdout, and stderr handling;
- filtering, buffering, raw mode, and confidential redaction;
- metrics and recovery side effects.

Cover both layers when a change crosses the boundary.

## Filter and fixture checklist

Before opening a filter pull request:

1. Work in `./.cmdshape/filters`, not the home directory or shipped `filters/`,
   unless the change is explicitly a built-in update.
2. Capture real output and inspect all streams for secrets.
3. Verify merged output, stream-specific output, decisions, dispatch, exit
   code, warnings, failures, and passthrough cases.
4. Review the complete project source before running `cmdshape filter trust`.
5. Re-approve after any source, mapping, path, or filename change.

Benchmark reports compare exact native and shaped bytes. They are fixture
evidence at the `cmdshape` command-output boundary, not evidence about total
agent context, model tokens, billing, turns, task cost, or result quality.

## Benchmark fixtures

Replay assets live under `testdata/benchmarks/<tool>/<case>/`:

- `command.yaml` is required and records an explicit `exit_code`;
- `stdout.txt` and `stderr.txt` are optional sequenced native streams;
- legacy merged `output.txt` remains a valid replay input when streams are
  absent;
- `output.txt`, `output.stdout.txt`, and `output.stderr.txt` may assert merged
  and exact stream results;
- `decisions.txt` may assert filtering decisions;
- `dispatch.txt` must assert the selected `filter|case`.

At least one native replay input must exist. Sequence prefixes must start at
`00000|` and remain contiguous. Assertions must be deterministic. Exact-byte
expansion, exit-code mismatch, and a wrong dispatch remain failures.

Benchmark metrics are written to artifact-local stores and must not enter
normal project or global gain reports. Track shaped-output overhead and treat
byte reduction as fixture evidence rather than a universal outcome promise.
Real captures are preferable to toy output when evaluating safety boundaries.

## Pull requests

Keep the description concrete:

- what user-visible behavior changed;
- which native and fallback paths remain untouched;
- how the change was validated;
- whether docs, fixtures, generated facts, or integrations changed.

Use the repository pull request template. Small, reviewable commits are easier
to validate than a broad cleanup mixed with behavior changes.

## Collaboration

Prefer clarity over cleverness. Raise ambiguity early, especially around
native semantics, output precision, or integration ownership. Large DSL,
architecture, installer, or release changes should be discussed before
implementation.

## License

By contributing, you agree that your work is released under the repository's
[MIT license](LICENSE).
