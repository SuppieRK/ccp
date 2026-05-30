# Tutorial: Create Your Own CCP Filter

This guide shows how to create, edit, and test your own CCP filters.

It is for user-authored filters, not built-in repository filters.

By the end, you will know how to:

- scaffold a new filter in `./.ccp/filters/`
- inspect which filters are active, overridden, or broken with `ccp filter status`
- edit the YAML safely
- test the filter live against a command
- capture a real fixture with `ccp capture`
- replay that fixture with `ccp verify`

## The Quick Mental Model

CCP has two user-facing filter locations:

- `./.ccp/filters/` for project-local filters that travel with one repo
- `~/.config/ccp/filters/` for home-scoped filters you want across repos

If you are just getting started, use `./.ccp/filters/`.

Project-local filters are intended to stay visible to Git so they can travel with a repository. CCP-generated repo-local state, such as `./.ccp/gain.db`, is ignored through a CCP-owned `./.ccp/.gitignore`; do not customize that nested ignore file because CCP rewrites it authoritatively.

That is what this tutorial covers.

When you are unsure which filter CCP will actually use, run:

```bash
ccp filter status
```

It shows all discovered rows from the active filter scopes and helps answer:

- which filters are active now
- which home-scoped filters are overridden by project-local ones
- which filters or mappings are broken and therefore unavailable

## What We Will Build

We will create a tiny fake command called `demo-tool` so you can follow the tutorial end to end without touching a real
tool first.

Its raw `run` output looks like this:

```text
demo-tool run v1.0.0
$ demo-tool internal-task
useful-line-1
useful-line-2
Done in 0.01s.
```

Our filter will keep the useful lines and remove the boilerplate so CCP prints:

```text
useful-line-1
useful-line-2
```

We will also keep JSON output untouched.

## Before You Start

- examples in this guide assume Bash on macOS or Linux
- make sure `ccp` is installed and on your `PATH`
- work from the root of a project directory
- make sure your `ccp` build includes `ccp verify`; if `ccp verify --help` says it is dev-only, upgrade first
- the scaffold already points at the public schema URL, so you do not need a local schema file to follow this tutorial

Useful commands in this tutorial:

```bash
ccp filter new demo-tool
ccp filter status
ccp capture --dir fixture-run -- ./demo-tool run
ccp verify --dir fixture-run
```

## Step 1: Create A Tiny Command To Filter

Create a file named `demo-tool` in your project root:

```bash
cat > demo-tool <<'EOF'
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
  json)
    printf '{"status":"ok","items":["useful-line-1","useful-line-2"]}\n'
    ;;
  fail)
    echo "demo-tool run v1.0.0"
    echo "error: something went wrong" >&2
    exit 2
    ;;
  *)
    echo "usage: demo-tool {run|json|fail}" >&2
    exit 64
    ;;
esac
EOF

chmod +x demo-tool
```

Check the raw command first:

```bash
./demo-tool run
```

You should see:

```text
demo-tool run v1.0.0
$ demo-tool internal-task
useful-line-1
useful-line-2
Done in 0.01s.
```

## Step 2: Scaffold A New Filter

Run:

```bash
ccp filter new demo-tool
```

CCP writes the scaffold file and ensures the `.mappings.yaml` file contains an identity mapping:

- `./.ccp/filters/demo-tool.yaml`
- `./.ccp/filters/.mappings.yaml`

The scaffold starts safe. It contains a passthrough case, so nothing breaks while you are still authoring the real
behavior.

The generated mapping file also includes an identity mapping:

```yaml
version: 1
map:
  demo-tool: demo-tool
```

That means commands such as `./demo-tool run` still resolve to the `demo-tool` filter id.

## Step 2.5: Check Which Filter CCP Will Use

Before you start editing behavior, confirm that CCP can see your new project-local filter:

```bash
ccp filter status
```

You should see a row for `demo-tool` coming from the project scope:

```text
ccp filter status

showing 1 rows

+-----------+------------------------------+---------+--------+
| TOOL      | FILTER                       | SOURCE  | STATUS |
+-----------+------------------------------+---------+--------+
| demo-tool | ./.ccp/filters/demo-tool.yaml | project | ok     |
+-----------+------------------------------+---------+--------+
```

As you add more filters over time, this command also shows:

- overridden rows when a project-local filter shadows a home-scoped one
- broken filter files with parse or validation errors
- broken `.mappings.yaml` rows and missing mapping targets

Use `ccp filter status` first whenever the runtime behavior is surprising. It is the quickest way to confirm whether CCP
is using the filter you think it is.

## Step 3: Start With A Minimal Real Filter

Edit `./.ccp/filters/demo-tool.yaml` so it matches this:

```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/SuppieRK/ccp/refs/heads/main/schemas/ccp-filter.schema.json
version: 1
filter: demo-tool
about: Compact demo-tool run boilerplate while preserving JSON output and any unmodeled commands.

cases:
  - id: json-passthrough
    when_arguments:
      first_is: json
    passthrough: true

  - id: run
    when_arguments:
      first_is: run
    compress_output:
      combined:
        lines:
          skip:
            - starts_with: "demo-tool run v"
            - starts_with: "$ demo-tool internal-task"
            - starts_with: "Done in "

  - id: passthrough-default
    passthrough: true
```

Why this is a good first filter:

- it matches one narrow command shape: `run`
- it explicitly preserves structured output with a `json-passthrough` case
- it keeps any unmodeled command in passthrough mode
- it removes boilerplate instead of inventing a custom summary format

Remember that case order matters. The first matching case wins.

## Step 4: Test The Filter Live

Run the command through CCP:

```bash
ccp ./demo-tool run
```

You should now see:

```text
useful-line-1
useful-line-2
```

Now test the structured mode:

```bash
ccp ./demo-tool json
```

You should still get the original JSON unchanged:

```text
{"status":"ok","items":["useful-line-1","useful-line-2"]}
```

And a failure path:

```bash
ccp ./demo-tool fail
```

Expected output:

```text
demo-tool run v1.0.0
error: something went wrong
```

That happens because the `fail` command does not match the `run` case, so it falls back to `passthrough-default`.

This is the behavior you usually want while a filter is still growing.

## Step 5: Capture A Real Fixture

Once the filter behaves the way you want, capture a fixture from a real command run:

```bash
ccp capture --dir fixture-run -- ./demo-tool run
```

CCP writes these files into `fixture-run/`:

- `command.yaml`
- `stdout.txt`
- `stderr.txt`
- `output.txt`

In this tutorial, `command.yaml` looks like this:

```yaml
argv: [ "./demo-tool", "run" ]
```

And `stdout.txt` contains the native sequenced output:

```text
00000|demo-tool run v1.0.0
00001|$ demo-tool internal-task
00002|useful-line-1
00003|useful-line-2
00004|Done in 0.01s.
```

The generated `output.txt` contains the current filtered result:

```text
useful-line-1
useful-line-2
```

Two important rules:

- do not hand-edit the `00000|` sequence prefixes in `stdout.txt` or `stderr.txt`
- if the capture is wrong, regenerate it with `ccp capture`

## Step 6: Replay The Fixture With `ccp verify`

Now replay that fixture through the current filter:

```bash
ccp verify --dir fixture-run
```

This writes two more files:

- `verify-output.txt`
- `verify-decisions.txt`

In this tutorial, `verify-output.txt` is:

```text
useful-line-1
useful-line-2
```

And `verify-decisions.txt` explains what happened to each line:

```text
<skip>    | demo-tool run v1.0.0
<skip>    | $ demo-tool internal-task
<keep>    | useful-line-1
<keep>    | useful-line-2
<skip>    | Done in 0.01s.
```

This is the basic authoring loop:

1. run a real command
2. adjust the YAML
3. capture a fixture
4. verify the fixture
5. compare `output.txt` with `verify-output.txt`

If the new behavior is correct and you want a checked-in expectation, copy `verify-output.txt` over `output.txt` instead
of rewriting it by hand.

## Step 7: Capture A Passthrough Boundary Too

You should test the cases you want to preserve, not just the cases you want to compress.

For the JSON path:

```bash
ccp capture --dir fixture-json -- ./demo-tool json
ccp verify --dir fixture-json
```

The generated `verify-output.txt` should still be:

```text
{"status":"ok","items":["useful-line-1","useful-line-2"]}
```

And `verify-decisions.txt` should show that the line was kept:

```text
<keep>    | {"status":"ok","items":["useful-line-1","useful-line-2"]}
```

This is how you make passthrough behavior explicit and testable.

## Step 8: Add More Cases Gradually

Once one case works, grow the filter one small step at a time.

A safe order is:

1. add passthrough cases for structured or precision-sensitive output
2. add one new narrow case
3. capture one new fixture
4. verify again

Avoid trying to model an entire tool in one pass.

## Field Reference For Personal Filters

Use this section as a quick lookup while authoring.

You do not need the whole schema on day one. Most personal filters start with a small set of fields and only grow when
the simple version stops being enough.

### The Top-Level Fields You Start With

This is the smallest useful filter shape:

```yaml
version: 1
filter: demo-tool
about: Compact demo-tool boilerplate while preserving safe passthrough behavior.

cases:
  - id: passthrough-default
    passthrough: true
```

What each field does:

- `version`: must be exactly `1`
- `filter`: the canonical filter id; this is the name mappings point to
- `about`: a short note explaining what the filter is trying to do
- `cases`: the ordered list of matching rules

### `cases`: The Real Authoring Unit

Most of your work happens inside `cases`.

```yaml
cases:
  - id: json-passthrough
    when_arguments:
      first_is: json
    passthrough: true

  - id: run
    when_arguments:
      first_is: run
    compress_output:
      combined:
        lines:
          skip:
            - starts_with: "Done in "

  - id: passthrough-default
    passthrough: true
```

Rules to remember:

- `id` is the case name you will see in diagnostics and maintain later
- case order matters; the first matching case wins
- specific cases should go above general cases
- a fallback passthrough case is usually the safest last case while a filter is still growing

### `when_arguments`: Match The Command Shape

`when_arguments` decides when a case should apply.

Important matching rules:

- matching happens against the command arguments after the executable name
- for `ccp ./demo-tool run`, `first_is: run` matches because CCP matches `run`, not `./demo-tool`
- if you specify multiple predicates in one `when_arguments` block, they are all required; CCP treats them as AND, not
  OR

The most useful matchers are:

- `first_is`: exact first argument match
- `first_in`: first argument is one of several values
- `have_any`: argv contains at least one listed token anywhere
- `lack_any`: argv contains none of the listed tokens
- `have_sequence`: argv contains an exact ordered subsequence
- `have_short_flag`: at least one listed short flag is present
- `have_all_short_flags`: all listed short flags are present
- `positionals_lack_any`: listed tokens are absent from positional arguments
- `no_positionals`: command has no positional arguments

Examples:

```yaml
when_arguments:
  first_is: run
```

```yaml
when_arguments:
  first_in: [ "run", "start" ]
```

```yaml
when_arguments:
  have_any: [ "--json", "--format=json" ]
```

```yaml
when_arguments:
  lack_any: [ "--help", "--verbose" ]
```

```yaml
when_arguments:
  have_sequence: [ "config", "list" ]
```

```yaml
when_arguments:
  have_short_flag: [ "-v" ]
```

```yaml
when_arguments:
  positionals_lack_any: [ "node_modules" ]
```

```yaml
when_arguments:
  no_positionals: true
```

Good practice:

- prefer matching argv shape before reaching for output rules
- keep the match as narrow and literal as you can

### `passthrough`: Preserve Output Exactly

`passthrough: true` tells CCP not to apply `compress_output` rules for that case.

```yaml
- id: json-passthrough
  when_arguments:
    have_any: [ "--json" ]
  passthrough: true
```

Use passthrough for:

- JSON and machine-readable output
- precision-sensitive or structured modes
- any command shape you have not modeled safely yet

Important:

- a passthrough case cannot also define `compress_output`
- a passthrough case can still define `finally`, so it is more accurate to think of passthrough as "no filtering" rather
  than "absolutely untouched bytes"

### `compress_output`: Choose Which Stream To Transform

When you do want compaction, use `compress_output`.

You can shape either:

- `combined`: stdout and stderr merged in original order
- `stdout`: stdout only
- `stderr`: stderr only

Start with `combined` unless you have a strong reason to treat the streams differently.

Simple example:

```yaml
compress_output:
  combined:
    lines:
      skip:
        - starts_with: "Done in "
```

Separate stream example:

```yaml
compress_output:
  stdout:
    lines:
      skip:
        - starts_with: "progress:"
  stderr:
    lines:
      keep:
        - starts_with: "error:"
```

### `lines.skip`: Remove Boilerplate

`skip` is the most common first tool.

```yaml
lines:
  skip:
    - starts_with: "demo-tool run v"
    - starts_with: "$ demo-tool internal-task"
    - starts_with: "Done in "
```

Use it for:

- banners
- wrapper command echoes
- timing/footer noise
- repetitive hints you do not need in agent workflows

### `lines.keep`: Protect Important Lines

`keep` is useful when a stream contains some lines that must always survive filtering.

```yaml
lines:
  keep:
    - starts_with: "error:"
    - contains: "warning"
```

Use it when:

- diagnostics matter more than compression
- a command mixes noise with a small number of critical lines

Important behavior:

- once you define `keep` rules, they act like a whitelist for lines in that scope
- lines that do not match `keep` are dropped unless another rule such as `replace` handles them first
- use `keep` carefully; it is stricter than `skip`

### `lines.replace`: Keep Meaning, Simplify Noise

Use `replace` when a line is useful but too verbose to keep as-is.

Literal example:

```yaml
lines:
  replace:
    - starts_with: "Loaded cache from "
      to: "Loaded cache"
```

Regex example:

```yaml
lines:
  replace:
    - regex: '^Finished in [0-9.]+s$'
      to: "Finished"
```

You can also use `contains` or `ends_with` instead of `starts_with` or `regex`:

```yaml
lines:
  replace:
    - ends_with: " files checked"
      to: "check completed"
```

For `skip`, `keep`, and `replace`, each rule should use exactly one matcher field:

- `starts_with`
- `contains`
- `ends_with`
- `regex`

Prefer literal matchers first. Reach for `regex` only when the simpler matchers cannot describe the pattern cleanly.

### `lines.max`: Cap Long Output Safely

If the output is useful but too repetitive, cap it instead of rewriting everything.

```yaml
compress_output:
  combined:
    lines:
      max:
        count: 20
        print: "... {{value}} more lines omitted"
```

Here, `{{value}}` is the number of omitted lines.

If you omit `print`, CCP still truncates at the requested count; you just do not get a summary line.

Use `max` when:

- the first few lines are useful, but the tail is repetitive
- you want predictable bounded output without inventing a synthetic summary format

### `normalize_command`: Add Safe Defaults When Needed

`normalize_command` lets CCP add arguments before execution.

This is useful, but it is more advanced than simple filtering. Use it sparingly.

Important:

- `when_arguments` selects the case first
- `normalize_command` runs only after the case already matched
- normalization does not help a case match; it only stabilizes the final executed command shape

Available mutations:

- `append_if_missing`
- `append_if_no_positionals`
- `add_short_flags`

Examples:

```yaml
normalize_command:
  append_if_missing: [ "--no-color" ]
```

```yaml
normalize_command:
  append_if_no_positionals: [ "status" ]
```

```yaml
normalize_command:
  add_short_flags: [ "-a" ]
```

Only use normalization when it preserves the expected meaning of the command and gives you a more stable output shape.

### `flags_consuming_next_arg`: Fix Tricky Argument Parsing

Some flags consume the next argv token. If you do not declare them, positional matching can become wrong.

What this field means:

- it is a top-level list shared by the whole filter
- each listed flag tells CCP: "the next token belongs to this flag"
- this only matters for split-form arguments such as `--output report.txt`
- do not list attached forms such as `--output=report.txt`; the value is already attached to the same token

Example:

```yaml
flags_consuming_next_arg:
  - "-o"
  - "--output"
```

Why this matters:

- CCP needs to know which tokens are real positionals and which tokens are just flag values
- without this list, a flag value can be mistaken for a positional argument
- that can break predicates like `no_positionals` and `positionals_lack_any`
- it can also break `normalize_command.append_if_no_positionals`

Concrete example:

```text
my-tool --output report.txt
```

If `--output` is not listed in `flags_consuming_next_arg`, CCP may incorrectly treat `report.txt` as a positional
argument.

That means this case can fail to match when you expected it to match:

```yaml
when_arguments:
  no_positionals: true
```

And this normalization can fail to run when you expect it to run:

```yaml
normalize_command:
  append_if_no_positionals: [ "status" ]
```

Another useful example:

```text
my-tool -C packages/app run test
```

If `-C` consumes the next token, then `packages/app` is not a real positional argument. Without
`flags_consuming_next_arg`, CCP can misread that command shape too.

Example filter snippet:

```yaml
flags_consuming_next_arg:
  - "-C"
  - "--output"

cases:
  - id: default-status
    when_arguments:
      no_positionals: true
    normalize_command:
      append_if_no_positionals: [ "status" ]
    compress_output:
      combined:
        lines:
          skip:
            - starts_with: "Done in "
```

When to add this field:

- a flag takes its value as the next separate token
- your filter uses positional-sensitive matching or normalization
- the filter behaves as if a flag value were an extra argument

When not to add it:

- the option is already attached, like `--format=json`
- the token does not consume the next token
- you are only matching simple flags and never reason about positionals

Common signs you forgot it:

- `no_positionals: true` stops matching unexpectedly
- `positionals_lack_any` behaves strangely
- `append_if_no_positionals` does not fire when it should
- the same command matches when written one way, but not when written with a split flag value

Most first filters do not need this field. Add it when argument matching starts behaving strangely because a flag value
is being mistaken for a real positional argument.

### Advanced Fields You Can Ignore At First

These fields are powerful, but most personal filters do not need them on day one:

- `variables`: counters or captured values you want to reuse later
- `finally`: text emitted after a successful case finishes
- `groups`: grouped output sections for more complex commands

Tiny examples:

```yaml
variables:
  - name: warnings
    type: number
```

```yaml
finally:
  print: "Filtered output complete"
```

```yaml
compress_output:
  combined:
    groups:
      - id: warning-group
        matches_regex: '^warning:'
        group_by: "warning"
        finally:
          print: "Warnings were grouped"
```

If simple `skip`, `keep`, `replace`, and `max` are enough, stay there for as long as possible.

One more important detail:

- top-level `finally` is emitted only on exit code `0`
- do not rely on it for failure summaries

### Common Patterns To Reuse

Preserve structured output:

```yaml
- id: json
  when_arguments:
    have_any: [ "--json" ]
  passthrough: true
```

Remove banners and timing lines:

```yaml
compress_output:
  combined:
    lines:
      skip:
        - starts_with: "tool v"
        - starts_with: "Done in "
```

Keep errors visible:

```yaml
compress_output:
  combined:
    lines:
      keep:
        - starts_with: "error:"
```

Simplify noisy status lines:

```yaml
lines:
  replace:
    - contains: "/tmp/"
      to: "temporary file created"
```

Cap repetitive output:

```yaml
lines:
  max:
    count: 25
    print: "... {{value}} more lines omitted"
```

## When To Edit `.mappings.yaml`

Most first-time filters do not need extra mappings beyond the identity mapping that `ccp filter new` already adds.

Add more mappings only when multiple command spellings should share one filter.

Example:

```yaml
version: 1
map:
  demo-tool: demo-tool
  demo-tool-alt: demo-tool
```

Keep mappings small and explicit.

## Good Filter Authoring Habits

- start with passthrough, then add narrow cases above it
- preserve structured output such as JSON unless you have a very strong reason not to
- prefer small line filtering over aggressive rewrites
- capture real command output instead of inventing fixtures by hand
- use `verify-decisions.txt` when you are unsure why a line changed
- keep one hypothesis per edit, so it is clear what improved or broke

## Common Mistakes

### My filter never matches

Check these first:

- the filter id in `demo-tool.yaml`
- the command shape in `command.yaml`
- the order of your `cases`
- whether an earlier case is catching the command first
- `ccp filter status` to confirm the filter is active instead of overridden or broken

### JSON got compacted when I wanted passthrough

Add an explicit passthrough case above the more general case.

### I changed the YAML, but the fixture still looks old

Run `ccp verify --dir ...` again. `output.txt` is what capture wrote earlier; `verify-output.txt` is what the current
filter produces now.

### The replay files have broken sequence numbers

Do not renumber them by hand. Recapture the fixture.

## What To Learn Next

Once you are comfortable with this tutorial, explore these next:

- `schemas/ccp-filter.schema.json` for the full field structure
- `./.ccp/filters/.mappings.yaml` for aliasing multiple command names to one filter
- `~/.config/ccp/filters/` when you want the same custom filter across many repos
- `ccp filter status` when you need to inspect active, overridden, or broken filter rows across scopes

The shortest useful loop to remember is:

```bash
ccp filter new my-tool
# edit ./.ccp/filters/my-tool.yaml
ccp ./my-tool some-command
ccp capture --dir fixture -- ./my-tool some-command
ccp verify --dir fixture
```
