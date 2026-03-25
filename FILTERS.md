# Tutorial: Create Your Own CCP Filter

This guide shows how to create, edit, and test your own CCP filters.

It is for user-authored filters, not built-in repository filters.

By the end, you will know how to:

- scaffold a new filter in `./.ccp/filters/`
- edit the YAML safely
- test the filter live against a command
- capture a real fixture with `ccp capture`
- replay that fixture with `ccp verify`

## The Quick Mental Model

CCP has two user-facing filter locations:

- `./.ccp/filters/` for project-local filters that travel with one repo
- `~/.config/ccp/filters/` for home-scoped filters you want across repos

If you are just getting started, use `./.ccp/filters/`.

That is what this tutorial covers.

## What We Will Build

We will create a tiny fake command called `demo-tool` so you can follow the tutorial end to end without touching a real tool first.

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

CCP writes the scaffold file and ensures the mappings file contains an identity mapping:

- `./.ccp/filters/demo-tool.yaml`
- `./.ccp/filters/.mappings.yaml`

The scaffold starts safe. It contains a passthrough case so nothing breaks while you are still authoring the real behavior.

The generated mapping file also includes an identity mapping:

```yaml
version: 1
map:
  demo-tool: demo-tool
```

That means commands such as `./demo-tool run` still resolve to the `demo-tool` filter id.

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
argv: ["./demo-tool", "run"]
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

If the new behavior is correct and you want a checked-in expectation, copy `verify-output.txt` over `output.txt` instead of rewriting it by hand.

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
- keep one hypothesis per edit so it is obvious what improved or broke

## Common Mistakes

### My filter never matches

Check these first:

- the filter id in `demo-tool.yaml`
- the command shape in `command.yaml`
- the order of your `cases`
- whether an earlier case is catching the command first

### JSON got compacted when I wanted passthrough

Add an explicit passthrough case above the more general case.

### I changed the YAML but the fixture still looks old

Run `ccp verify --dir ...` again. `output.txt` is what capture wrote earlier; `verify-output.txt` is what the current filter produces now.

### The replay files have broken sequence numbers

Do not renumber them by hand. Recapture the fixture.

## What To Learn Next

Once you are comfortable with this tutorial, explore these next:

- `schemas/ccp-filter.schema.json` for the full field structure
- `./.ccp/filters/.mappings.yaml` for aliasing multiple command names to one filter
- `~/.config/ccp/filters/` when you want the same custom filter across many repos

The shortest useful loop to remember is:

```bash
ccp filter new my-tool
# edit ./.ccp/filters/my-tool.yaml
ccp ./my-tool some-command
ccp capture --dir fixture -- ./my-tool some-command
ccp verify --dir fixture
```
