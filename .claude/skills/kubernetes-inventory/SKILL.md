---
name: kubernetes-inventory
description: "Inventory and organization model for kuberoutectl. Use when working on targets, aliases, labels, collections, selectors, selection/current, or cache/state persistence."
allowed_tools:
  - Read
  - Write
  - Edit
  - Glob
  - Grep
  - Bash
---

# Kubernetes Inventory

Use this skill when working on the inventory and organization model in
`kuberoutectl`.

## Goals

- Keep Scope, Target, and Credential distinct — never collapse them.
- User organization (labels, collections, selection) must survive every resync.
- Collections are selector-driven live views, not folders.
- Persistence stays JSON: `cache/` (discovered, replaceable) vs `state/`
  (user-owned, never touched by sync). That split *is* the survival guarantee.

## Workflow

1. Decide first: does the change touch discovered state or user-owned state?
   They have different files, services, and rules.
2. Update the domain model, then services, then CLI rendering — in that order.
3. Add tests for the survival guarantees, not just the happy path
   (resync keeps labels; collections re-resolve; stale selection surfaces).

## Gotchas

- **Discovery never writes UserLabels.** Providers return targets with empty
  UserLabels; `DiscoveryService.applyUserLabels` re-attaches them by ID — the
  single place they're populated. A provider setting them would silently break
  resync survival.
- **Merging is per-provider replace-only.** Syncing one provider drops and
  re-adds only that provider's entities; everything else in the snapshot must
  pass through untouched. A "rebuild the whole snapshot" shortcut destroys the
  other clouds' inventory.
- **Selectors evaluate `SelectionLabels()`, not raw labels.** Bare keys
  (`region`, `platform`, `provider`, `health`, `kind`) are structured-attribute
  aliases; user labels override them. Before aliases existed, `region=x`
  matched nothing because the system label is namespaced
  (`kuberoutectl.io/region`) — don't regress to matching only EffectiveLabels.
- **Aliases are computed on read, never persisted.** `AssignAliases` runs in
  every read path (list/inspect/resolve/use) so aliases can't drift from the
  snapshot. Persisting them would break on the next name collision. Colliding
  names get a deterministic `-<hash6>` suffix; resolution order is
  ID → alias → name, with ambiguous names erroring out.
- **A stale selection is information, not an error.** If a resync removed the
  selected target, `current` reports the selection with a "no longer in the
  cache" note — erroring would hide what the operator was pointed at.
- **The reserved namespace is `kuberoutectl.io/`.** User label writes must go
  through `ValidateUserLabel`, which rejects it; system labels are tool-owned
  and rewritten every sync.

## References

- `references/targets.md`
- `references/labels.md`
- `references/collections.md`
- `references/persistence.md`
