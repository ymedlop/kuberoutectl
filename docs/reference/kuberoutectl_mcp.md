---
title: "kuberoutectl mcp"
parent: Command reference
layout: default
description: "Run an MCP server exposing kuberoutectl as tools (stdio)"
---

## kuberoutectl mcp

Run an MCP server exposing kuberoutectl as tools (stdio)

### Synopsis

Serve a Model Context Protocol server over stdio so an MCP-compatible AI
client can list inventory, inspect credential health, route targets, and
manage collections. No secrets and no destructive actions are exposed.
Use --read-only to expose the inspection tools only.

```
kuberoutectl mcp [flags]
```

### Options

```
  -h, --help        help for mcp
      --read-only   expose only read tools (no sync/use/collection writes)
```

### Options inherited from parent commands

```
  -o, --output string   output format: text|json (default "text")
  -v, --verbose         trace external CLI commands, exit codes, and their stderr on failure
```

### SEE ALSO

* [kuberoutectl](kuberoutectl.md)	 - Discover, organize, and route Kubernetes access across providers

