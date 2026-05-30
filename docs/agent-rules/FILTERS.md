# Agent Rule – Filters

CCP filters are ordinary YAML files. You can inspect shipped filters, override them locally, and iterate on new filters
without rebuilding CCP.

## Authoring Model

- Built-in filters belong in `filters/<tool>.yaml`.
- Wrapper or alias spellings belong in `filters/.mappings.yaml`.
- Local iteration happens in `./.ccp/filters`.
- Project-local filters are committable; generated repo-local CCP state is ignored through CCP-owned `./.ccp/.gitignore`.
- Managed home-scoped filters live in `~/.config/ccp/filters`.
- Release builds load `./.ccp/filters` first and `~/.config/ccp/filters` second.
- Project-local filter definitions override home-scoped definitions with the same canonical filter id.
- Project-local `.mappings.yaml` aliases override home-scoped aliases with the same key.
- Shipped filters are embedded from `filters/` and materialized into `~/.config/ccp/filters` by `ccp repair` and startup
  maintenance. `ccp init` installs integrations; it does not own home filter materialization.

Use the existing YAML DSL as-is whenever possible.

- Be extremely hesitant to change the DSL.
- If a concise implementation is not possible without a DSL change, stop and get explicit confirmation before changing
  it.
- Keep authored behavior in YAML whenever the current runtime vocabulary can express it.
- Put shared runtime behavior in `internal/filters`, `internal/engine`, or `internal/contracts`, not in bespoke per-tool
  Go filters.

Family-level guidance:

- Single-entity tools typically use one YAML file such as `filters/npm.yaml`.
- Family tools should prefer one YAML file when the commands share one logical behavior surface, for example
  `filters/git.yaml`.
- Direct-tool and wrapper-tool pairs that intentionally share one logical behavior should prefer one canonical filter
  plus mappings.

## Scaffolds, Schema, And Mappings

Create a project-local scaffold with:

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

Use project-local scaffolds first. Promote the finished result into `filters/<tool>.yaml` and `filters/.mappings.yaml`
only when the behavior belongs in the shipped built-in set.

The current schema lives at [schemas/ccp-filter.schema.json](../../schemas/ccp-filter.schema.json).

Schema notes:

- the JSON Schema is structural
- Go validation remains authoritative for runtime-only rules
- examples include cross-field semantic checks, regex capture references, template references, and unsupported declared
  shapes

Mappings live in:

- `./.ccp/filters/.mappings.yaml`
- `~/.config/ccp/filters/.mappings.yaml`

Typical use:

```yaml
gradle: gradle
gradlew: gradle
```

Mapping rules:

- keep mappings small and explicit
- use them when one filter should serve multiple command spellings
- a project-local alias can only bind to a filter that compiled successfully in `./.ccp/filters`
- a home-scoped alias can only bind to a filter that compiled successfully in `~/.config/ccp/filters`
- lower-priority aliases do not replace aliases that were already registered from a higher-priority source
- broken mappings fall back safely to passthrough and are recorded in the audit log

## Workflow

Recommended iteration loop:

1. `ccp filter new my-tool`
2. `ccp capture -- my-tool ...`
3. edit `./.ccp/filters/my-tool.yaml`
4. `ccp verify`
5. compare `output.txt` with `verify-output.txt`
6. inspect `verify-decisions.txt` when behavior is unclear

If a matching home-scoped filter already exists, copy or refresh a project-local version first so the project-local
filter acts as the active override.

### Capture

Capture a real command with:

```bash
ccp capture -- my-tool --flag value
```

Capture writes:

- `command.yaml`
- `stdout.txt`
- `stderr.txt`
- `output.txt`

Capture rules:

- `stdout.txt` and `stderr.txt` use `00000|` sequence prefixes to preserve cross-stream ordering
- capture runs the command natively once, then replays the captured streams through the current CCP runtime to bootstrap
  `output.txt`
- non-zero exits still write artifacts so failures can be iterated locally

### Verify

Replay a captured fixture with:

```bash
ccp verify
```

or:

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

Verification rules:

- missing `stdout.txt` or `stderr.txt` means that stream is empty
- broken sequence numbering is an error
- `verify-decisions.txt` is always generated and shows why lines were kept, replaced, skipped, or emitted synthetically

### Repair

Restore the managed home-level filter state with:

```bash
ccp repair
```

or:

```bash
ccp repair --yes
```

`ccp repair` rewrites the managed `~/.config/ccp` state, including shipped filters and the home-level `.mappings.yaml`.
It does not touch project-local `./.ccp/filters`. Rewrite repair may refresh CCP-owned repo-local ignore state in
`./.ccp/.gitignore` so generated metrics remain ignored without hiding project-local filters.

When the interactive prompt is declined, `ccp repair` falls back to additive sync: it adds only missing shipped filters
and missing shipped `.mappings.yaml` entries without mutating repository files.

## Authoring Guardrails

- Start with corpus expansion before filter redesign when the current benchmark set is toy-sized, stale, or obviously
  unrepresentative.
- Prefer one hypothesis at a time.
- Use real command output whenever possible, ideally from `ccp capture` or an existing research corpus that reflects
  native output.
- Treat warning-bearing success paths as first-class behavior. Clean success fixtures alone are often misleading.
- Treat machine-oriented, structured, or precision modes as explicit passthrough boundaries unless the current filter
  contract already defines normalization.
- Be skeptical of table rewrites. Many native tables are already near the token floor.
- Be skeptical of log compression. Tool-defined build/test output is often compressible; user application logs usually
  are not.
- Preserve shell-usable output identity. If a rewrite makes follow-up commands harder to form, the savings are probably
  not worth it.
- Prefer promoting verified output from `verify-output.txt` or fresh `ccp capture` output instead of hand-editing
  expectations by guesswork.

## Benchmark And Test Alignment

- Follow `docs/agent-rules/BENCHMARKS.md` for benchmark workflow and expectations.
- Follow `docs/agent-rules/TESTING.md` for test-layer selection and placement rules.
- New filters MUST add benchmark fixtures under `testdata/benchmarks/<tool>/`.
- Existing filters with behavior changes MUST update benchmark coverage for the changed behavior.
- Family filters should keep sibling replay fixture directories under the family benchmark root, each with
  `command.yaml` and any required replay artifacts.
- Benchmark verification is exercised through the benchmark-related Ginkgo/Gomega suites rather than a separate manual
  `ccp-ci` workflow.
- New runtime/filter features MUST add command-specific or runtime-specific coverage in the narrowest relevant package.
