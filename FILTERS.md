# Authoring cmdshape filters

cmdshape ships useful defaults, but we do not pretend to know your domain. Use
a project-local filter when a repository's scripts, generated logs, or failure
messages need context that a general-purpose default cannot safely infer.

The filter language is ordinary YAML. You can review it like code, replay it
against captured output, commit it with the repository, and keep native
passthrough whenever the shape is uncertain.

## The agent-first path

Give a coding agent the embedded workflow:

```bash
cmdshape filter prompt
cmdshape filter prompt <filter-id>
```

For a standalone filter, `<filter-id>` is the executed command's basename
without its path. When multiple executable names intentionally share one
behavior model, use the canonical target id and map the other names in
`.mappings.yaml`, for example `gradlew: gradle`. The lifecycle commands trim
surrounding whitespace, normalize ids to lowercase, and require
`^[a-z0-9][a-z0-9-]*$`: a lowercase letter or digit followed only by lowercase
letters, digits, or hyphens. Spaces, underscores, and a leading hyphen are
invalid.

The prompt tells the agent to work in `./.cmdshape/filters`, copy an existing
source before editing, capture representative native output, show the proposed
YAML, ask before trusting it, and verify only after the exact current source is
active. It also explains how to read the local performance report. This is the
shortest useful request to paste to an agent:

> Create or improve the `cmdshape` filter with canonical id `<filter-id>`.
> Start by running `cmdshape filter prompt <filter-id>`. Capture representative
> output from the actual command, show me the proposed YAML and replay
> decisions, and ask before trusting the final project filter source.

The agent should:

1. Run `cmdshape filter status` and copy any matching home or shipped filter
   into `./.cmdshape/filters`, or run `cmdshape filter new <filter-id>` for a
   new passthrough-safe scaffold.
2. Capture success, warning, failure, structured, and interactive cases that
   matter to the repository. The native streams remain useful even if the
   capture's initial replay expectation came from a lower-priority filter.
3. Edit the project-local YAML and show the complete project source.
4. Run `cmdshape filter trust` only after the user approves those exact bytes.
5. Run `cmdshape verify --dir <fixture-dir>` and inspect merged output,
   stream-specific output, decisions, and dispatch.
6. If verification leads to an edit, review and trust the changed source again
   before the next replay.

Project filters are inactive in release builds until their exact current bytes
are trusted. Any addition, removal, rename, mapping edit, or content change
invalidates approval. Re-review and trust again after every edit. This is a
deliberate boundary: a filter can remove useful context, so activation should
be an explicit project decision. Untrusted, changed, invalid, or unsafe sources
fall back to the remaining valid source or native passthrough.

## Source locations and precedence

cmdshape has three filter locations with different ownership:

- `./.cmdshape/filters/` contains project-owned filters that can travel with a
  repository.
- `~/.config/cmdshape/filters/` contains home-scoped filters and materialized
  shipped defaults.
- `filters/` in this repository contains the built-in sources embedded into a
  release.

Release builds load a trusted project source before the home source.
Project-local filter ids and mapping keys therefore override matching
home-scoped entries. Invalid definitions, invalid mappings, and aliases whose
targets did not compile are ignored safely. Unresolved tools use passthrough.

`cmdshape init` installs coding-agent integrations. It does not materialize
home filters; startup maintenance and `cmdshape repair` own that state.

Use `cmdshape filter status` whenever the selected filter is surprising. It
shows the project trust state as well as active, overridden, changed, and
broken registrations.

## Complete manual tutorial

The same workflow works without an agent. The tutorial uses a small
`demo-tool` command whose `run` output contains fixed wrapper lines:

```text
demo-tool run v1.0.0
$ demo-tool internal-task
useful-line-1
useful-line-2
Done in 0.01s.
```

The filter will retain the two useful lines, preserve JSON exactly, and leave
unmodeled commands native.

To follow the tutorial, save this as `demo-tool` in the project root:

```bash
#!/usr/bin/env bash
set -euo pipefail

case "${1:-}" in
  run)
    echo "demo-tool run v1.0.0"
    echo '$ demo-tool internal-task'
    echo "useful-line-1"
    echo "useful-line-2"
    echo "Done in 0.01s."
    ;;
  --json)
    printf '{"status":"ok","items":["useful-line-1","useful-line-2"]}\n'
    ;;
  fail)
    echo "demo-tool run v1.0.0"
    echo "error: something went wrong" >&2
    exit 2
    ;;
  *)
    echo "usage: demo-tool {run|--json|fail}" >&2
    exit 64
    ;;
esac
```

Make it executable and inspect each native path before filtering:

```bash
chmod +x demo-tool
./demo-tool run
./demo-tool --json
./demo-tool fail
```

### Scaffold and inspect

Create a project-local scaffold:

```bash
cmdshape filter new demo-tool
cmdshape filter status
```

The scaffold creates:

- `./.cmdshape/filters/demo-tool.yaml`;
- `./.cmdshape/filters/.mappings.yaml` with an identity mapping.

It starts with passthrough behavior so the command remains safe while the
filter is being authored.

### Capture representative native output

Capture each behavior you care about before relying on the new filter:

```bash
cmdshape capture --dir fixture-run -- ./demo-tool run
cmdshape capture --dir fixture-json -- ./demo-tool --json
cmdshape capture --dir fixture-failure -- ./demo-tool fail
```

The capture runs the native command once. Its sequenced streams remain the
source of truth even when the initial replay files were produced by a home
filter or passthrough.

### Author the project filter

Edit `./.cmdshape/filters/demo-tool.yaml`:

```yaml
version: 1
filter: demo-tool
about: Remove fixed run wrappers while preserving structured and unknown modes.
cases:
  - id: structured
    when_arguments:
      have_any: ["--json"]
    passthrough: true

  - id: run-success
    when_arguments:
      have_sequence: [run]
    compress_output:
      stdout:
        lines:
          skip:
            - starts_with: "demo-tool run v"
            - starts_with: "$ "
            - starts_with: "Done in "

  - id: passthrough-default
    passthrough: true
```

Case order matters: the first matching case wins. Put narrow structured or
precision-sensitive cases first and keep a passthrough fallback last while the
filter grows.

### Review, trust, and replay

Review every project YAML file and `.mappings.yaml`. Trust the complete source
only after it matches what you intend to execute:

```bash
cmdshape filter trust
cmdshape filter status
cmdshape verify --dir fixture-run
cmdshape verify --dir fixture-json
cmdshape verify --dir fixture-failure
```

Compare `output.txt` with `verify-output.txt`. Use
`verify-stdout.txt` and `verify-stderr.txt` when destination matters. Read
`verify-decisions.txt` to see why each line was kept, replaced, skipped, or
emitted. Confirm `verify-dispatch.txt` selected the intended `filter|case`
before copying that exact value to `dispatch.txt` in a checked-in fixture.

`output.txt` is the earlier capture expectation.
`verify-output.txt` is the result from the currently trusted filter. If the
result needs another YAML change, approval becomes `changed`; review and trust
again before replaying. Trust is not a test shortcut. Revoke it with
`cmdshape filter untrust` when the project should return to home filters or
native passthrough.

## Captures and verification artifacts

`cmdshape capture -- <command>` runs the command natively once, records the
sequenced streams, and bootstraps replay expectations through the current
runtime. The capture directory and files are private by default, but command
output may contain credentials, source, paths, or customer data. Redact known
literal values before they are persisted:

```bash
cmdshape capture --confidential "$TOKEN" -- tool --flag value
```

Review all files before sharing or committing. `--confidential` is literal
redaction; it is not a secret scanner.

Capture creates:

- `command.yaml`, including the native exit code;
- sequenced `stdout.txt` and `stderr.txt` with `00000|` prefixes;
- merged `output.txt`;
- exact stream expectations in `output.stdout.txt` and `output.stderr.txt`.

Non-zero commands still write artifacts. A record that cannot fit one
newline-terminated fixture line uses the runtime's `@cmdshape/base64:` payload;
do not decode or rewrite it by hand.

Verification reads `command.yaml` and the optional sequenced streams and writes:

- `verify-output.txt`;
- `verify-stdout.txt` and `verify-stderr.txt`;
- `verify-decisions.txt`;
- `verify-dispatch.txt`.

Missing streams mean empty streams. Broken sequence numbering is an error.
Merged output remains useful for a human view; stream-specific files assert
where bytes were written.

## YAML reference

The structural schema is [schemas/cmdshape-filter.schema.json](schemas/cmdshape-filter.schema.json).
Go validation remains authoritative for runtime-only and cross-field rules.
The schema is useful for editor completion, but a successful schema check does
not replace a trusted replay.

### Top-level fields

```yaml
version: 1
filter: tool-name
about: A short reason this filter exists.
flags_consuming_next_arg:
  - "--project"
cases:
  - id: default
    passthrough: true
```

- `version` must be exactly `1`.
- `filter` is the canonical command/filter id.
- `about` documents the intent for reviewers.
- `flags_consuming_next_arg` is an optional filter-wide list used when parsing
  positional arguments.
- `cases` are evaluated in order; the first matching case owns the output.

### Cases and selection

Each case has an `id` and may have `when_arguments`. A case without
`when_arguments` matches as a fallback.

```yaml
cases:
  - id: json
    when_arguments:
      have_any: ["--json"]
    passthrough: true

  - id: test
    when_arguments:
      first_is: test
    compress_output:
      combined:
        lines:
          skip:
            - starts_with: "progress:"

  - id: passthrough-default
    passthrough: true
```

Specific cases belong above general cases. A case may define either
`passthrough: true` or `compress_output`, never both. A passthrough case may
still define `finally`; passthrough means no line filtering, not that the case
cannot emit a documented successful-exit footer.

### `when_arguments`

Argument matching excludes the executable name. For
`cmdshape ./demo-tool run`, `first_is: run` sees `run` as the first argument.
Predicates within one block are combined with AND.

Available predicates:

- `first_is`: the first argument equals one token;
- `first_in`: the first argument equals any listed token;
- `have_any`: argv contains at least one listed token;
- `lack_any`: argv contains none of the listed tokens;
- `have_sequence`: argv contains an exact ordered subsequence;
- `have_short_flag`: at least one listed short flag is present;
- `not_have_short_flag`: every listed short flag is absent;
- `have_all_short_flags`: all listed short flags are present;
- `not_have_all_short_flags`: the complete listed set is not present together;
- `positionals_lack_any`: parsed positionals contain none of the listed tokens;
- `no_positionals`: the command has no positional arguments.

Examples:

```yaml
when_arguments:
  first_is: test
  lack_any: ["--json", "--help"]
```

```yaml
when_arguments:
  first_in: [run, start]
```

```yaml
when_arguments:
  have_any: ["--json", "--format=json"]
```

```yaml
when_arguments:
  have_sequence: [config, list]
```

```yaml
when_arguments:
  have_short_flag: ["-v"]
  not_have_short_flag: ["-q"]
```

```yaml
when_arguments:
  have_all_short_flags: ["-a", "-l"]
```

```yaml
when_arguments:
  positionals_lack_any: [node_modules]
```

```yaml
when_arguments:
  no_positionals: true
```

Prefer literal argv matching over guessing from output. If the invocation is
not modeled confidently, allow it to reach a passthrough case.

### `passthrough`

Use case-level passthrough for JSON, machine-readable output, interactive
modes, precision-sensitive formats, and shapes that are not modeled yet:

```yaml
- id: structured
  when_arguments:
    have_any: ["--json"]
  passthrough: true
```

Unmatched cases and unresolved tools also fall back to native output. Do not
add a `compress_output` block to a passthrough case.

### `compress_output` and stream routing

`compress_output` accepts one of two routing shapes:

- `combined` processes stdout and stderr as one ordered stream;
- `stdout` and/or `stderr` process the native streams independently.

Do not mix `combined` with `stdout` or `stderr`. A stream omitted from a split
definition remains native.

Combined example:

```yaml
compress_output:
  combined:
    lines:
      skip:
        - starts_with: "progress:"
```

Split-stream example:

```yaml
compress_output:
  stdout:
    lines:
      max:
        count: 40
        print: "... {{value}} more stdout lines"
  stderr:
    lines:
      keep:
        - starts_with: "warning:"
        - starts_with: "error:"
```

Because `keep` is present for stderr, it acts as a whitelist in that scope.
If a stream should remain completely native, omit its scope or choose a
case-level passthrough case.

### `lines.skip`

`skip` removes known boilerplate while unmatched lines continue through:

```yaml
lines:
  skip:
    - starts_with: "demo-tool run v"
    - starts_with: "$ "
    - ends_with: " completed"
    - regex: "^Done in [0-9.]+s\\.$"
```

Each skip rule must set exactly one matcher: `starts_with`, `contains`,
`ends_with`, or `regex`.

### `lines.keep`

`keep` protects a known set of useful lines. When at least one keep rule exists
in a scope, unmatched lines in that scope are ignored:

```yaml
lines:
  keep:
    - starts_with: "error:"
    - contains: "warning"
```

Use it only when a real corpus demonstrates that the whitelist retains all
actionable success, warning, and failure information. Prefer `skip` when the
noise is easier to identify than the useful output.

### `lines.replace`

`replace` keeps a line's meaning while making a proven verbose form shorter:

```yaml
lines:
  replace:
    - starts_with: "Loaded cache from "
      to: "Loaded cache"
    - regex: "^Finished in [0-9.]+s$"
      to: "Finished"
```

Replacement rules use exactly one of `starts_with`, `contains`, `ends_with`,
or `regex`, and the replacement field is named `to`. Regex replacements and
group templates must use only captures and variables accepted by runtime
validation.

### `lines.max`

`max` bounds repetitive output and can report the omitted line count:

```yaml
lines:
  max:
    count: 20
    print: "... {{value}} more lines omitted"
```

`count` must be positive. `print` is optional. The ordinary template accepts
`{{value}}`; `{{groups_summary}}` is available only with a valid grouped
summary in the same scope.

### `normalize_command`

Normalization changes argv before native execution, so use it more carefully
than output filtering. Case selection happens first; normalization cannot make
its own case match.

Supported mutations:

```yaml
normalize_command:
  append_if_missing: ["--no-color"]
  append_if_no_positionals: [status]
  add_short_flags: ["-a"]
```

- `append_if_missing` appends tokens not already present.
- `append_if_no_positionals` appends tokens only when parsed positionals are
  empty.
- `add_short_flags` ensures listed short flags are present.

Use normalization only when it preserves the documented meaning of the
command. Without it, cmdshape executes argv as supplied.

### `flags_consuming_next_arg`

Declare split-form flags whose next token is a value:

```yaml
flags_consuming_next_arg:
  - "-C"
  - "--output"
```

For `tool --output report.txt`, this prevents `report.txt` from being mistaken
for a positional. It affects `no_positionals`,
`positionals_lack_any`, and `append_if_no_positionals`. Do not list attached
forms such as `--output=report.txt`, and do not list flags that do not consume
the next token.

Common signs that a declaration is missing:

- a no-positionals case stops matching only when a split flag is used;
- `append_if_no_positionals` does not run;
- the same option behaves differently in attached and split forms.

### Variables, groups, and `finally`

These fields support complex summaries but are not needed for most project
filters:

```yaml
variables:
  - name: warnings
    type: number
    initial_value: "0"
```

- Variables may be numeric counters or captured strings. Replacement and group
  actions can update only variables declared in the enclosing case.
- Groups collect or render related lines using declared matchers and templates.
  Group template variables and references are validated against the group.
- `max.groups_summary` is valid only when the same scope has at least one
  collect group.
- `finally` emits a documented footer only for a successful case; do not rely
  on it for failure diagnostics.

Example successful-exit footer:

```yaml
finally:
  print: "Filtered output complete"
```

Prefer `skip`, `keep`, `replace`, and `max` until a captured corpus proves that
stateful grouping is necessary. Consult the schema for the complete group and
variable shape rather than copying an unrelated built-in filter blindly.

### Mappings

Aliases live in `.mappings.yaml`:

```yaml
version: 1
map:
  gradle: gradle
  gradlew: gradle
```

Project aliases override home aliases with the same key. An alias binds only to
a filter that compiled successfully in the same source. Broken mappings safely
fall back to passthrough and are visible in `cmdshape filter status`.

`cmdshape filter new` creates an identity mapping. Add aliases only when
multiple executable spellings intentionally share one filter.

## Reusable patterns

Preserve structured modes:

```yaml
- id: json
  when_arguments:
    have_any: ["--json", "--format=json"]
  passthrough: true
```

Remove fixed banners and timing:

```yaml
compress_output:
  combined:
    lines:
      skip:
        - starts_with: "tool v"
        - starts_with: "Done in "
```

Keep only known diagnostics:

```yaml
compress_output:
  stderr:
    lines:
      keep:
        - starts_with: "warning:"
        - starts_with: "error:"
```

Bound a repetitive list:

```yaml
compress_output:
  stdout:
    lines:
      max:
        count: 25
        print: "... {{value}} more entries"
```

## Common mistakes

### The filter never becomes active

Run `cmdshape filter status`. Check the project trust state, filter id, mapping,
YAML validation result, and whether the project source changed after approval.

### The wrong case matches

Check case order and the arguments recorded in `command.yaml`. Remember that
all predicates in one `when_arguments` block are required and that split flag
values need `flags_consuming_next_arg`.

### Structured output was compacted

Add a narrow passthrough case above the general compacting case and capture the
structured mode as a regression fixture.

### The fixture still shows old output

`output.txt` is capture-time output. Run `cmdshape verify --dir <fixture-dir>`
after trusting the current source and inspect `verify-output.txt`.

### Sequence numbers are broken

Do not renumber `stdout.txt` or `stderr.txt` by hand. Recapture the command.

### An edit appears to have no effect

Any source or mapping edit invalidates trust. Review the complete source, run
`cmdshape filter trust`, and verify again.

## Performance-guided iteration

Use the local report to decide what to inspect:

```bash
cmdshape filter performance --limit 30
cmdshape filter performance --tool <tool> --limit 30
cmdshape filter performance --global --tool <tool> --limit 30
```

`RUNS` is frequency. `NET REDUCTION` and `REDUCTION %` report exact
routed-output byte reduction. `PASS` is the passthrough rate and `NONZERO`
is the rate of native commands with nonzero exits. `review-case`, `failure-heavy`, and
`passthrough-opportunity` are hints, not proof that a broader filter is safe.
Capture representative output before changing behavior.

## Authoring habits that scale

- Start with passthrough, then add one narrow case at a time.
- Capture warnings and failures, not only clean success output.
- Preserve structured output unless the filter has an explicit, fixture-backed
  contract for it.
- Prefer literal matchers before regex.
- Prefer removing known noise over inventing a new representation.
- Keep paths, identifiers, counts, and diagnostics that an agent can reuse in
  its next command.
- Treat performance reports as prioritization evidence, not permission to
  broaden a matcher.
- Commit project filters only after reviewing captures for source, paths,
  credentials, and customer data.

## Built-in filters and fixtures

Project filters can be committed with the repository. Shipped built-ins belong
in `filters/<tool>.yaml`, aliases in `filters/.mappings.yaml`, and replay
fixtures in `testdata/benchmarks/<tool>/<case>/`.

A checked-in fixture has:

- required `command.yaml` with an explicit `exit_code`, including zero;
- at least one replay input: `stdout.txt`, `stderr.txt`, or legacy
  `output.txt`;
- optional merged and stream-specific output expectations;
- optional `decisions.txt`;
- required `dispatch.txt` containing the reviewed `filter|case`.

Keep `00000|` stream sequence prefixes contiguous. Use `cmdshape verify` to
generate candidate output, decision, and dispatch artifacts; promote them only
after review. Do not hand-edit generated benchmark artifacts or weaken
expansion and exit-code gates.

A built-in change needs the focused fixture and validation workflow described
in [CONTRIBUTING.md](CONTRIBUTING.md). Project-only behavior should remain in
the project instead of being generalized prematurely.

For runtime source precedence and the passthrough boundary, see
[ARCHITECTURE.md](ARCHITECTURE.md). For vulnerability reporting, see
[SECURITY.md](SECURITY.md).
