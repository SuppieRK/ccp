## Purpose
Define the `git worktree` subfilter contract for safe listing compaction and passthrough handling of stateful or precision-oriented shapes.

## Requirements

### Requirement: Git Worktree Planning Safety
The `git worktree` phase SHALL compact only listing-oriented invocation shapes.

#### Scenario: worktree listing dispatch
- **WHEN** `git worktree` is invoked in listing-oriented form such as `git worktree list`
- **THEN** the `git` parent dispatches to the `git worktree` subfilter.

#### Scenario: worktree management passthrough
- **WHEN** `git worktree` is invoked with management subcommands such as `add`, `remove`, `move`, `repair`, or `prune`
- **THEN** the `git worktree` phase returns passthrough behavior.

#### Scenario: precision-oriented list passthrough
- **WHEN** `git worktree list` is invoked with precision-oriented flags such as `--porcelain`, `--verbose`, or `-v`
- **THEN** the `git worktree` phase returns passthrough behavior.

### Requirement: Git Worktree Runtime Compaction
The `git worktree` phase SHALL preserve worktree path and branch identity while reducing listing noise.

#### Scenario: compact worktree list
- **WHEN** supported `git worktree` output is compacted
- **THEN** output preserves each worktree path and associated branch or detached-head identity in a bounded line-oriented form
- **AND** keeps the native field ordering of path first, then hash, then branch identity.

#### Scenario: shorter relative paths may be used
- **WHEN** a listed worktree path can be expressed relative to the current working directory, using the resolved working-directory path when symlinks are involved, with a shorter shell-usable path
- **THEN** the compacted output may use that relative path instead of the longer absolute path
- **AND** relative paths are rendered in slash-normalized form for stable shell-friendly output across platforms.

#### Scenario: diagnostics remain visible
- **WHEN** `git worktree` emits actionable diagnostics
- **THEN** those diagnostics remain visible to the caller.
