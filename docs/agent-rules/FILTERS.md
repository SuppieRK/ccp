# Agent Rule – Filters

CCP filters are ordinary YAML files. You can inspect shipped filters, override them locally, and iterate on new filters without rebuilding CCP.

## Architecture

- Runtime behavior is defined by authored YAML filters under `filters/` plus the invariant-enforcing Go runtime in `internal/`.
- Local overrides and in-progress authoring live under `./.ccp/filters`, which takes precedence over `~/.config/ccp/filters` at runtime.
- Single-entity tools typically use one YAML file such as `filters/npm.yaml`.
- Family tools should prefer one family YAML file such as `filters/git.yaml` when the commands share one logical behavior surface.
- Direct-tool and wrapper-tool pairs with one logical behavior should prefer one canonical YAML filter and reuse it through `filters/.mappings.yaml`.
- Shared runtime behavior belongs in the Go core under `internal/filters`, `internal/engine`, and `internal/contracts`, not in bespoke per-tool Go filters.

## Layout Guidance

- Keep authored behavior in YAML whenever the existing runtime vocabulary can express it.
- Keep Go files responsibility-scoped, but do not force a strict one-file-per-tool rule where the runtime is clearer split across multiple files.
- Put broadly reusable runtime behavior in `internal/filters/`, `internal/engine/`, or `internal/contracts/` based on scope.
- Reuse existing shared helper files where they already exist, but avoid introducing new generic helper files unless the scope is clearly distinct and justified.

## Active Filter Locations

CCP uses two active filter scopes:

- project scope: `./.ccp/filters`
- home scope: `~/.config/ccp/filters`

Precedence is:

1. project scope
2. home scope

If both scopes define the same filter identity or mapping target, the project version wins.

Shipped filters are materialized into `~/.config/ccp/filters` by `ccp init`, `ccp upgrade`, and `ccp repair`. CCP does not recreate or overwrite project-local filters during normal command execution.

## Create A New Filter

Generate a scaffold in the current project:

```bash
ccp filter new my-tool
```

That writes:

- `./.ccp/filters/my-tool.yaml`
- `./.ccp/filters/.mappings.yaml`

The scaffold includes:

- commented authoring guidance
- a `yaml-language-server` schema directive
- a valid passthrough-safe initial case

## Schema Support

The current authoring schema lives at [schemas/ccp-filter.schema.json](../../schemas/ccp-filter.schema.json).

`ccp filter new` adds a `yaml-language-server` comment pointing at that schema so editors can offer validation and completion immediately.

The JSON Schema is intentionally structural. Go validation remains authoritative for runtime-only rules such as:

- some cross-field semantic constraints
- regex named-capture references
- template variable references
- runtime support gaps for declared-but-not-generically-supported fields

## Mappings

`./.ccp/filters/.mappings.yaml` and `~/.config/ccp/filters/.mappings.yaml` map command spellings to canonical filter ids.

Typical use:

```yaml
gradle: gradle
gradlew: gradle
```

Use mappings when:

- one YAML filter should serve wrapper and direct-tool spellings
- the executable name does not match the canonical filter filename

Keep mappings small and explicit. Ambiguous or broken mappings fall back safely to passthrough and are recorded in the audit log.

## Capture Workflow

Capture a real command into the current directory:

```bash
ccp capture -- my-tool --flag value
```

Capture writes:

- `command.yaml`
- `stdout.txt`
- `stderr.txt`
- `output.txt`

Notes:

- `stdout.txt` and `stderr.txt` use `00000|` sequence prefixes so replay preserves cross-stream ordering.
- capture runs the command natively once, then replays the captured streams through the current CCP runtime to bootstrap `output.txt`.
- non-zero exits still write artifacts so you can iterate on failures locally.

## Verify Workflow

Replay a captured fixture directory:

```bash
ccp verify
```

Or:

```bash
ccp verify --dir path/to/fixture
```

Verify reads:

- `command.yaml`
- optional `stdout.txt`
- optional `stderr.txt`

Verify writes:

- `verify-output.txt`
- `verify-decisions.txt`

`verify-decisions.txt` is always generated and shows fixed-width replay decisions so you can see why lines were kept, replaced, skipped, or emitted synthetically.

Missing `stdout.txt` or `stderr.txt` means that stream is empty. Broken sequence numbering is treated as an error.

## Restore Shipped Defaults

To restore the managed home-level filter state:

```bash
ccp repair
```

Or for automation:

```bash
ccp repair --yes
```

`ccp repair` rewrites the managed `~/.config/ccp` state, including shipped filters and the home-level `.mappings.yaml`.

It does not touch project-local `./.ccp/filters`.

## Suggested Iteration Loop

1. `ccp filter new my-tool`
2. `ccp capture -- my-tool ...`
3. edit `./.ccp/filters/my-tool.yaml`
4. `ccp verify`
5. compare `output.txt` with `verify-output.txt`
6. inspect `verify-decisions.txt` when behavior is unclear

## Spec Alignment

- Family tools require one spec that covers the shared entity behavior.
- Specs under: `openspec/specs/`
- Spec-fixture directory names MUST match spec IDs.
- When specs change, update the matching YAML benchmark coverage under `testdata/benchmarks/`.

## Benchmark Coverage

- Follow `docs/agent-rules/BENCHMARKS.md` for benchmark workflow and expectations.
- New filters MUST add benchmark fixtures under `testdata/benchmarks/<tool>/`.
- Existing filters with behavior changes MUST update benchmark coverage for the changed behavior.
- Family filters should keep sibling replay fixture directories under the family benchmark root, each with `command.yaml` and any required replay artifacts.
- For built-in filters contributed to the repository, keep authored YAML under `filters/` and replay fixtures under `testdata/benchmarks/<tool>/`.

## Runner Test Coverage

- Follow `docs/agent-rules/TESTING.md` for test-layer selection and generic placement rules.
- New runtime/filter features MUST add command-specific or runtime-specific coverage in the narrowest relevant package, and shared planner/runtime helpers should live in shared test helper files instead of one helper per tool.
- Existing filters with planning or runtime behavior changes MUST update those tests alongside YAML fixtures and OpenSpec.
