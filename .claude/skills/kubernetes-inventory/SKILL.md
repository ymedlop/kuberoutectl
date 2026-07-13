---
name: kubernetes-inventory
description: Use when working on targets, labels, collections, selectors, kubeconfig semantics, or inventory persistence in kuberoutectl.
---

# Kubernetes Inventory

Use this skill when working on the inventory and organization model in `kuberoutectl`.

## Goals

- Keep targets, scopes, and credentials distinct.
- Preserve user labels across discovery syncs.
- Make collections selector-driven and predictable.
- Keep inventory persistence simple and JSON-based for the MVP.

## Workflow

1. Read the current target and collection model.
2. Decide whether the change affects discovered state or user-owned state.
3. Update the domain model first.
4. Update selector or label logic next.
5. Persist user-owned state separately from discovery state.
6. Add tests for selection and persistence behavior.

## When to use

Use this skill for:
- targets,
- scopes,
- labels,
- collections,
- selectors,
- kubeconfig context mapping,
- inventory persistence,
- sync behavior.

## References

- `references/targets.md`
- `references/labels.md`
- `references/collections.md`
- `references/persistence.md`
