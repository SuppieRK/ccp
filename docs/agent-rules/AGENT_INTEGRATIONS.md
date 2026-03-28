# Agent Rule – Agent Integrations

CCP agent integrations are lifecycle-managed adapters under `internal/lifecycle/agents`.

Use this guide when changing supported integrations, adapter registration, managed files, plugin outputs, or
install/verify/uninstall behavior.

## Canonical Registration

- New built-in lifecycle agent adapters MUST register through `internal/lifecycle/agents/agent_specs.go`.
- Prefer the existing adapter families before creating a bespoke adapter:
    - managed context files
    - managed context links
    - managed rule files
    - managed hook/settings adapters
    - managed JS plugin adapters
- Keep bespoke adapters only when `init`, `verify`, `uninstall`, or artifact content is materially different from the
  shared families.

## Scope And Placement

- Shared lifecycle-agent helpers belong in `internal/lifecycle/agents`.
- Shared runtime behavior does not belong in adapter packages; keep core execution/filter semantics in
  `internal/contracts`, `internal/engine`, `internal/filters`, or other runtime packages.
- Home-scoped and repo-scoped install targets should follow the adapter's actual integration surface, not a forced
  one-size-fits-all convention.

## Managed Artifact Principles

- Adapters must be installed into their canonical managed targets only.
- `init` installs or updates supported integrations.
- `uninstall` removes managed artifacts from the same canonical targets.
- `repair` owns the managed CCP home state under `~/.config/ccp`; adapter-specific managed files remain adapter-owned.

## Safety And Product Boundaries

- Prefer explicit, inspectable managed content over opaque mutation where possible.
- Do not treat hook-based command rewriting as a freestanding security feature.
- If an integration surface cannot preserve the right trust boundary, prefer a simpler or more conservative integration
  model.
- Keep agent integrations as convenience layers. The product should still make sense with explicit `ccp <command>`
  usage.

## Testing Expectations

- Prefer family conformance tests for shared adapter families.
- Keep bespoke adapter tests only for bespoke behavior.
- Update lifecycle tests when `init`, detect, `verify`, or `uninstall` behavior changes.
- Verify managed artifact paths and content precisely; adapter work is path-sensitive by nature.

## Documentation Expectations

- Update `README.md` when the supported integration surface changes in a user-visible way.
- Update `CONTRIBUTING.md` or this document when the contributor workflow for integrations changes.
- If an integration is removed or intentionally unsupported, document the boundary clearly rather than leaving stale
  claims in place.
