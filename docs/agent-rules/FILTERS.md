# Agent Rule – Filters

cmdshape filters are ordinary YAML files. You can inspect shipped filters, override them locally, and iterate on new filters
without rebuilding cmdshape.

## Authoring Model

- Built-in filters belong in `filters/<tool>.yaml`.
- Wrapper or alias spellings belong in `filters/.mappings.yaml`.
- Local iteration happens in `./.cmdshape/filters`.
- Project-local filters are committable; generated repo-local cmdshape state is ignored through cmdshape-owned `./.cmdshape/.gitignore`.
- Managed home-scoped filters live in `~/.config/cmdshape/filters`.
- Release builds load an explicitly trusted `./.cmdshape/filters` first and
  `~/.config/cmdshape/filters` second.
- `cmdshape filter trust` approves the exact current project source. Any path,
  mapping, addition, removal, rename, or content change requires approval
  again; `cmdshape filter untrust` removes approval.
- Project-local filter definitions override home-scoped definitions with the same canonical filter id.
- Project-local `.mappings.yaml` aliases override home-scoped aliases with the same key.
- Shipped filters are embedded from `filters/` and materialized into `~/.config/cmdshape/filters` by `cmdshape repair` and startup
  maintenance. `cmdshape init` installs integrations; it does not own home filter materialization.

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
cmdshape filter new my-tool
```

That writes:

- `./.cmdshape/filters/my-tool.yaml`
- `./.cmdshape/filters/.mappings.yaml`

The scaffold includes:

- commented authoring guidance
- a `yaml-language-server` schema directive
- a valid passthrough-safe initial case

Use project-local scaffolds first. Promote the finished result into `filters/<tool>.yaml` and `filters/.mappings.yaml`
only when the behavior belongs in the shipped built-in set.

When creating or improving an existing filter, first check the active sources with:

```bash
cmdshape filter status
```

If a matching home-scoped or shipped/global filter exists, copy it into `./.cmdshape/filters` and edit the project-local
copy. Agents MUST NOT edit `~/.config/cmdshape/filters` or shipped `filters/` directly unless the user explicitly asks for a
global or built-in filter change.

Agents can ask cmdshape for the embedded self-service workflow with:

```bash
cmdshape filter prompt
cmdshape filter prompt <filter-id>
```

The current schema lives at [schemas/cmdshape-filter.schema.json](../../schemas/cmdshape-filter.schema.json).

Schema notes:

- the JSON Schema is structural
- Go validation remains authoritative for runtime-only rules
- examples include cross-field semantic checks, regex capture references, template references, and unsupported declared
  shapes

Mappings live in:

- `./.cmdshape/filters/.mappings.yaml`
- `~/.config/cmdshape/filters/.mappings.yaml`

Typical use:

```yaml
gradle: gradle
gradlew: gradle
```

Mapping rules:

- keep mappings small and explicit
- use them when one filter should serve multiple command spellings
- a project-local alias can only bind to a filter that compiled successfully in `./.cmdshape/filters`
- a home-scoped alias can only bind to a filter that compiled successfully in `~/.config/cmdshape/filters`
- lower-priority aliases do not replace aliases that were already registered from a higher-priority source
- broken mappings fall back safely to passthrough and are recorded in the audit log

## Workflow

Recommended iteration loop:

1. `cmdshape filter prompt <filter-id>`
2. `cmdshape filter status`
3. copy any matching global/home filter into `./.cmdshape/filters` before editing, or run `cmdshape filter new <filter-id>`
4. `cmdshape capture -- <tool> ...`
5. edit `./.cmdshape/filters/<filter-id>.yaml`
6. `cmdshape verify`
7. compare `output.txt` with `verify-output.txt` and the stream-specific
   `output.stdout.txt` / `output.stderr.txt` expectations with
   `verify-stdout.txt` / `verify-stderr.txt`
8. inspect `verify-decisions.txt` when behavior is unclear
9. run `cmdshape filter trust` only after reviewing the complete project filter
   source that should become active

For brand-new filters with no existing source, the short loop is:

1. `cmdshape filter prompt <filter-id>`
2. `cmdshape filter new <filter-id>`
3. `cmdshape capture -- <tool> ...`
4. edit `./.cmdshape/filters/<filter-id>.yaml`
5. `cmdshape verify`
6. compare merged and stream-specific output expectations with the matching
   `verify-*` artifacts
7. inspect `verify-decisions.txt` when behavior is unclear
8. run `cmdshape filter trust` only after reviewing the complete project filter
   source that should become active

If a matching home-scoped filter already exists, copy or refresh a project-local version first so the project-local
filter acts as the active override.

### Performance

Use performance metrics to prioritize improvements before authoring broad changes:

```bash
cmdshape filter performance --limit 30
cmdshape filter performance --tool <tool> --limit 30
cmdshape filter performance --global --tool <tool> --limit 30
```

`--tool` filters by the invoked command name. Treat `review-case`, `failure-heavy`, and `passthrough-opportunity` hints
as starting points for inspection, then capture representative output and verify the project-local filter change.

### Capture

Capture a real command with:

```bash
cmdshape capture -- my-tool --flag value
```

Capture creates a private directory (`0700`) and private files (`0600`).
Because native output may include secrets, use
`cmdshape capture --confidential value1,value2 -- my-tool --flag value` for known
literal values and review the fixture before committing it. Redacted captures
set `redacted: true` in `command.yaml`.

Capture writes:

- `command.yaml`
- `stdout.txt`
- `stderr.txt`
- `output.txt`
- `output.stdout.txt`
- `output.stderr.txt`

Capture rules:

- `stdout.txt` and `stderr.txt` use `00000|` sequence prefixes to preserve cross-stream ordering
- records that cannot be represented as one newline-terminated fixture line
  use the runtime's `@cmdshape/base64:` payload encoding; do not decode or rewrite
  them by hand
- capture runs the command natively once, then replays the captured streams through the current cmdshape runtime to bootstrap
  merged and stream-specific output expectations
- `command.yaml` records `exit_code` even when it is zero
- non-zero exits still write artifacts so failures can be iterated locally

### Verify

Replay a captured fixture with:

```bash
cmdshape verify
```

or:

```bash
cmdshape verify --dir path/to/fixture
```

Verify reads:

- `command.yaml`
- optional `stdout.txt`
- optional `stderr.txt`

Verify writes:

- `verify-output.txt`
- `verify-stdout.txt`
- `verify-stderr.txt`
- `verify-decisions.txt`
- `verify-dispatch.txt`

Promote the exact `filter|case` value in `verify-dispatch.txt` to
`dispatch.txt` only after checking that argument matching selected the
intended case.
- `verify-stdout.txt`
- `verify-stderr.txt`
- `verify-decisions.txt`

Verification rules:

- missing `stdout.txt` or `stderr.txt` means that stream is empty
- broken sequence numbering is an error
- merged `output.txt` remains supported, while `output.stdout.txt` and
  `output.stderr.txt` assert exact destinations
- `verify-decisions.txt` is always generated and shows why lines were kept, replaced, skipped, or emitted synthetically

### Repair

Restore the managed home-level filter state with:

```bash
cmdshape repair
```

or:

```bash
cmdshape repair --yes
```

`cmdshape repair` rewrites the managed `~/.config/cmdshape` state, including shipped filters and the home-level `.mappings.yaml`.
It does not touch project-local `./.cmdshape/filters`. Rewrite repair may refresh cmdshape-owned repo-local ignore state in
`./.cmdshape/.gitignore` so generated metrics remain ignored without hiding project-local filters.

When the interactive prompt is declined, `cmdshape repair` falls back to additive sync: it adds only missing shipped filters
and missing shipped `.mappings.yaml` entries without mutating repository files.

## Authoring Guardrails

- Start with corpus expansion before filter redesign when the current benchmark set is toy-sized, stale, or obviously
  unrepresentative.
- Prefer one hypothesis at a time.
- Use real command output whenever possible, ideally from `cmdshape capture` or an existing research corpus that reflects
  native output.
- Treat warning-bearing success paths as first-class behavior. Clean success fixtures alone are often misleading.
- Treat machine-oriented, structured, or precision modes as explicit passthrough boundaries unless the current filter
  contract already defines normalization.
- Declare value-taking options in `flags_consuming_next_arg`. Matching then
  treats `--flag=value` and `--flag value` equivalently, stops at `--`, and
  leaves the argv sent to the child untouched. Short-option clusters are not
  split generically.
- Be skeptical of table rewrites. Many native tables are already near the token floor.
- Be skeptical of log compression. Tool-defined build/test output is often compressible; user application logs usually
  are not.
- Preserve shell-usable output identity. If a rewrite makes follow-up commands harder to form, the savings are probably
  not worth it.
- Prefer promoting verified output from `verify-output.txt` or fresh `cmdshape capture` output instead of hand-editing
  expectations by guesswork.

## Benchmark And Test Alignment

- Follow `docs/agent-rules/BENCHMARKS.md` for benchmark workflow and expectations.
- Follow `docs/agent-rules/TESTING.md` for test-layer selection and placement rules.
- New filters MUST add benchmark fixtures under `testdata/benchmarks/<tool>/`.
- Existing filters with behavior changes MUST update benchmark coverage for the changed behavior.
- Family filters should keep sibling replay fixture directories under the family benchmark root, each with
  `command.yaml` and any required replay artifacts.
- Benchmark verification is exercised through the benchmark-related Ginkgo/Gomega suites rather than a separate manual
  `cmdshape-ci` workflow.
- New runtime/filter features MUST add command-specific or runtime-specific coverage in the narrowest relevant package.
