---
title: MCP server
description: "Operate kuberoutectl from an AI client via the Model Context Protocol — an optional, opt-in stdio server that exposes inventory, credential health, target routing, and collections as safe structured tools with no secrets and no destructive actions."
layout: default
nav_order: 5
---

# MCP server

`kuberoutectl mcp` runs a [Model Context Protocol](https://modelcontextprotocol.io)
(MCP) server over **stdio**, so an MCP-compatible AI client can operate the safe
core workflows as structured tools instead of shelling out to the CLI and
scraping text.

It is a thin adapter over the same services the CLI uses — the tools carry no
new behavior of their own.

## Design principles

- **Opt-in.** Nothing listens until you run `kuberoutectl mcp`. There is no
  daemon and no background listener.
- **No secrets.** Credential tools return health, action hints, and identifiers
  only — never tokens, keys, or kubeconfig contents.
- **No destructive actions.** Deleting targets, clearing the cache, renewing
  credentials, editing labels, and hiding targets are **not** exposed. Pass
  `--read-only` to drop even the safe writes.
- **Provider-agnostic.** Tool names and schemas never assume a specific cloud.

## Running it

```bash
kuberoutectl mcp              # read + safe-write tools
kuberoutectl mcp --read-only  # inspection tools only
```

The server speaks JSON-RPC over stdin/stdout; **stdout is the protocol channel**,
so don't expect human-readable output there — point an MCP client at the command.

### Example client configuration

Most clients launch an MCP server as a subprocess. A typical entry:

```json
{
  "mcpServers": {
    "kuberoutectl": {
      "command": "kuberoutectl",
      "args": ["mcp"]
    }
  }
}
```

For an inspection-only connection, use `"args": ["mcp", "--read-only"]`.

## Tools

### Read (always available)

| Tool | What it returns |
|------|-----------------|
| `list_providers` | Registered providers and their capabilities. |
| `list_targets` | Targets, optionally filtered by `provider` and/or `selector`. |
| `get_target` | One target resolved by id, alias, or name. |
| `list_sources` | Discovered access sources. |
| `list_scopes` | Discovered scopes (subscriptions/accounts). |
| `list_credentials` | Credential health + action hints (no secrets). |
| `get_status` | Current selection (target/collection) and cache freshness. |
| `list_collections` | Saved collections. |
| `get_collection` | A collection resolved to its current member targets. |

### Safe writes (omitted with `--read-only`)

| Tool | Effect |
|------|--------|
| `sync_provider` | Discover a provider's inventory into the local cache. |
| `use_target` | Select a target as current; with `activate`, also merge its context into `~/.kube/config`. |
| `create_or_update_collection` | Create a collection, or update it in place if the name exists. |

## Notes and limitations

- Transport is **stdio only** in this release; a network transport may come
  later.
- The server is long-lived, so mutating tools are serialized internally — two
  overlapping writes can't lose an update.
- `use_target` with `activate` writes your kubeconfig (additive, like
  `target use`); call it with `activate` unset to record the selection only.
