## Purpose
Define safe planning and runtime compaction behavior for default human-readable `git show` commit inspection output.

## Requirements

### Requirement: Git Show Planning Safety
The `git show` phase SHALL compact only safe default commit-inspection shapes and SHALL preserve passthrough for precision-sensitive variants.

#### Scenario: default commit show dispatch
- **WHEN** `git show` is invoked without explicit formatting, blob display, or raw/stat-only precision modes
- **THEN** the `git` parent dispatches to the `git show` subfilter.

#### Scenario: explicit formatting passthrough
- **WHEN** `git show` args include explicit formatting or precision-sensitive display flags such as `--format`, `--pretty`, `--raw`, `--stat`, or `--numstat`
- **THEN** the `git show` phase returns passthrough behavior.

#### Scenario: blob show passthrough
- **WHEN** `git show` targets blob-style output such as `<rev>:<path>`
- **THEN** the `git show` phase returns passthrough behavior.

### Requirement: Git Show Runtime Compaction
The `git show` phase SHALL preserve commit-identifying signal while reducing repetitive diff and stat verbosity.

#### Scenario: compact default show output
- **WHEN** `git show` emits default human-readable commit output
- **THEN** compacted output includes commit-identifying summary lines and bounded file or hunk context.

#### Scenario: stderr diagnostics remain visible
- **WHEN** `git show` emits diagnostics on stderr
- **THEN** those diagnostics remain visible to the caller.

#### Scenario: low-confidence fallback
- **WHEN** `git show` output cannot be compacted with sufficient confidence
- **THEN** the phase falls back to passthrough output.
