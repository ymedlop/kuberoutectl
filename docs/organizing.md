---
title: "Organizing: labels & collections"
layout: default
nav_order: 3
---

# Organizing clusters: labels & collections

`kuberoutectl` keeps your own organization layer — **labels** and
**collections** — separate from discovered inventory, so it survives every
`sync`. This page covers the everyday workflow: tag clusters, then group them
with live, selector-driven collections that can span clouds.

{: .note }
> The key idea: **a collection is a saved query over labels, not a static
> folder.** Create it first and label clusters into it later — order does not
> matter, and newly matching clusters join automatically.

## Labels

Targets carry two label sets:

- **System labels** — discovered or derived by the tool, under the reserved
  `kuberoutectl.io/` namespace (e.g. `kuberoutectl.io/provider`, `.../region`).
- **User labels** — your own plain `key=value` pairs. These are what you attach
  and select on.

```bash
kuberoutectl target list                          # find the ALIAS to reference
kuberoutectl target label add aks-prod-weu env=prod team=platform
kuberoutectl target label list aks-prod-weu       # show a target's labels
kuberoutectl target label remove aks-prod-weu team
```

`target` also answers to **`clusters`** / `cluster`, so
`kuberoutectl clusters label add …` works identically.

{: .tip }
> User labels are stored separately from discovered inventory, so re-running
> `kuberoutectl sync <provider>` **never erases them**.

## Hiding targets

Some clusters are just noise in day-to-day work — a decommissioned sandbox, a
colleague's account you rarely touch. **Hiding** drops them from the default
`target list` without deleting anything. Like labels, hidden state is user-owned
and **survives every resync**.

```bash
kuberoutectl target hide aks-sandbox            # hide one target
kuberoutectl target hide -l env=staging         # bulk-hide by selector
kuberoutectl target unhide aks-sandbox          # bring it back

kuberoutectl target list                        # hidden targets are gone from here
kuberoutectl target list --all                  # show everything (adds a HIDDEN column)
kuberoutectl target list -l hidden=true         # list only the hidden ones
```

Hiding never affects routing: `target use`, `target inspect`, and collections
still resolve a hidden target by name — it's only filtered out of the default
listing. Visibility is exposed to selectors as the bare keys **`hidden`** and
**`visible`**, so `-l hidden=true` and `-l visible=false` are equivalent.

{: .note }
> **Hide is persistent; delete is not.** Hiding records your intent in
> user-owned state, so it outlasts a `sync`. If you instead want a target gone
> from the cache entirely, see [Curating the cache](#curating-the-cache) — but a
> resync will bring it straight back.

## Curating the cache

`target delete` and `target clear` prune the **discovered cache**, not your
organization layer. They are **ephemeral**: the next `sync <provider>` rediscovers
whatever is still out there and repopulates it.

```bash
kuberoutectl target delete eks-old             # drop one target from the cache
kuberoutectl target clear                      # drop them all (prompts; --yes skips)
```

Reach for these to tidy a stale cache after clusters are torn down cloud-side.
To *keep* a cluster out of your everyday view instead, **hide** it — hiding
survives the resync that `delete` does not.

## Collections

A collection is a saved view over targets, driven primarily by a **label
selector**, with optional **static** members.

### The workflow (create first, label later)

```bash
# 1. Create the collection with a selector — 0 members is fine, nothing matches yet
kuberoutectl collection create production --selector env=prod
# Created collection: production

# 2. Label clusters whenever you like — they join automatically
kuberoutectl target label add aks-prod-weu       env=prod
kuberoutectl target label add eks-prod-frankfurt env=prod

# 3. Membership re-resolves live — no resync needed
kuberoutectl collection show production
# Collection: production
# Members: 2
# aks-prod-weu        aks  westeurope    valid
# eks-prod-frankfurt  eks  eu-central-1  valid

# 4. Point kubectl at the whole set
kuberoutectl collection use production
```

Because membership is recomputed from **current labels** every time, labeling a
new cluster tomorrow adds it to `production` with no extra step.

### Managing collections

```bash
kuberoutectl collection list                      # all saved collections
kuberoutectl collection show production            # members (resolved live)
kuberoutectl collection use production             # activate the whole set
kuberoutectl collection delete production
```

Every inventory command supports `-o json` for scripting, including
`collection show`.

## Selectors

Selectors decide what a collection matches:

| Form | Example |
|------|---------|
| Exact match | `--selector env=prod` |
| Multiple (AND) | `--selector env=prod,team=platform` or repeat `--selector` |
| In-list | `--selector "region in [westeurope, eu-central-1]"` |
| Structured attribute (bare key) | `--selector platform=aks`, `--selector provider=aws` |
| Visibility (bare key) | `--selector hidden=true`, `--selector visible=false` |

Beyond your own labels, you can select on a target's built-in attributes by bare
key: **`region`**, **`platform`**, **`provider`**, **`health`**, **`kind`**, plus
**`hidden`** / **`visible`** for [visibility](#hiding-targets). User labels take
precedence when a key collides.

Because selectors match across every provider, one collection can span clouds:

```bash
kuberoutectl collection create eu \
  --selector "region in [westeurope, eu-central-1, europe-west4]"
```

## Static members

For a one-off that doesn't fit a selector, add explicit target IDs at creation —
they are unioned (and de-duplicated) with the selector matches:

```bash
kuberoutectl collection create critical \
  --selector env=prod \
  --static <some-target-id>
```

## Where this fits

- **Labels** are your organization metadata; they survive discovery.
- **Collections** are saved, live views over that metadata.
- **Hiding** is user-owned too, so it also survives discovery — unlike
  **delete/clear**, which only prune the rediscoverable cache.
- **`current`** shows what you last selected (a target or a collection) and how
  fresh the cache is — see the [provider guides]({{ '/guides/' | relative_url }}).
