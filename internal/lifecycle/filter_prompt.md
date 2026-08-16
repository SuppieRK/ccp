# cmdshape Filter Authoring Prompt

You are helping create or improve a cmdshape YAML filter for `{{FILTER_ID}}`.

When using the generic prompt, `{{FILTER_ID}}` is a placeholder.

`{{FILTER_ID}}` is the canonical filter id. For a standalone filter, use the
executed command's basename without its path, for example `demo-tool` for
`./demo-tool`. When several executable names intentionally share one behavior
model, use the canonical target id and add the other names as aliases in
`.mappings.yaml`, for example `gradlew: gradle`.

`cmdshape filter prompt` and `cmdshape filter new` trim surrounding whitespace,
normalize the id to lowercase, then require it to match
`^[a-z0-9][a-z0-9-]*$`. It must start with a lowercase letter or digit and may
then contain only lowercase letters, digits, or hyphens. Spaces, underscores,
and a leading hyphen are invalid. Use the same canonical id in the YAML
`filter` field, the `{{FILTER_ID}}.yaml` filename, and mapping targets.

cmdshape ships useful defaults, but we do not pretend to know your domain. A
project-local filter is a good fit for repository scripts, generated logs, and
failure output that needs local context.

The project root is the nearest enclosing Git worktree root. Outside a Git
repository, it is the current directory. Every project-local `./.cmdshape`
path below is relative to that resolved root, even when cmdshape is invoked
from a subdirectory.

## Safety rules

- Start by working in the project-local filter directory: `./.cmdshape/filters`.
- If a matching global, home-scoped, or shipped filter exists, copy it into `./.cmdshape/filters` first and edit that project-local copy.
- Do not edit global/home filters under `~/.config/cmdshape/filters` unless the user directly asks.
- Do not edit shipped built-in filters under `filters/` unless the user directly asks.
- Preserve native execution semantics, exit codes, actionable diagnostics,
  exact `--raw` output, and empty-output behavior.
- Prefer passthrough for structured, interactive, precision-sensitive,
  ambiguous, or unsafe shapes. Warning-bearing success and failure output are
  first-class behavior.
- Never auto-trust a project filter. Trust only after the user has reviewed the
  complete current source.

## Review and authoring workflow

1. Inspect active sources with `cmdshape filter status`.
2. Copy a matching source into `./.cmdshape/filters`, or create a safe scaffold
   with `cmdshape filter new {{FILTER_ID}}`.
3. Use performance data to choose representative cases, not to prove that a
   filter is safe:

   ```bash
   cmdshape filter performance --limit 30
   cmdshape filter performance --tool <tool> --limit 30
   ```

   The `--tool` value is the invoked tool name, not necessarily the resolved
   filter id. Read `RUNS` as frequency, `NET REDUCTION` and `REDUCTION %` as
   exact routed-output byte reduction, and `PASS` as passthrough rate. Treat
   `review-case` hints, `failure-heavy` hints, and
   `passthrough-opportunity` hints as starting points for inspection.

4. Capture real output, including a clean success, warnings, failures, and any
   structured or interactive mode that should remain native:

   ```bash
   cmdshape capture -- {{COMMAND_EXAMPLE}}
   ```

5. Edit only `./.cmdshape/filters/{{FILTER_ID}}.yaml` and, when needed,
   `./.cmdshape/filters/.mappings.yaml`. Keep the behavior in the existing YAML
   DSL. Make one hypothesis at a time.
6. Show the complete proposed project source to the user. Only after the user
   approves the exact current bytes run `cmdshape filter trust`.
7. Replay the fixture:

   ```bash
   cmdshape verify --dir <fixture-dir>
   ```

   Compare merged and stream-specific output, inspect
   `verify-decisions.txt`, and confirm the selected `filter|case` in
   `verify-dispatch.txt` before promoting it to `dispatch.txt`.
8. Show the replay output and decisions. If verification leads to any source
   or mapping edit, review the complete source and run `cmdshape filter trust`
   again before replaying the changed filter.

Project-local approval is exact and short-lived by design. Any path, mapping,
addition, removal, rename, or content change makes the source `changed` and
requires review and approval again. Until then, release builds safely ignore
the project filter and use the remaining valid sources or passthrough.

## Authoring guidance

- Match the command and arguments narrowly before matching output.
- Keep failure details, warnings, paths, counts, and next-step hints that an
  agent can act on.
- Prefer `keep` and small `replace` rules over a rewritten table.
- Preserve shell-usable line-oriented output.
- Use `max` only when the bounded result remains useful; otherwise pass through.
- Do not change the Go runtime or the YAML vocabulary unless the user confirms
  that the existing DSL cannot express the behavior.
- Add or update replay fixtures when changing a shipped built-in filter.

Capture directories are private (`0700` directory, `0600` files), but native
output may still contain credentials, paths, or source. Use
`cmdshape capture --confidential value1,value2 -- <tool> <args...>` for known
literal values, inspect every file, and never commit secrets. Sequence prefixes
preserve cross-stream order; keep them intact.

Capture writes `command.yaml`, sequenced `stdout.txt` and `stderr.txt`, merged
`output.txt`, and exact stream expectations in `output.stdout.txt` and
`output.stderr.txt`. Verify writes `verify-output.txt`, `verify-stdout.txt`,
`verify-stderr.txt`, `verify-decisions.txt`, and `verify-dispatch.txt`.
