---
title: "kuberoutectl target clear"
parent: Command reference
layout: default
description: "Delete all targets from the local cache"
---

## kuberoutectl target clear

Delete all targets from the local cache

### Synopsis

Delete all targets from the local cache. Scopes, credentials, and sources
are kept, and a resync repopulates targets. Prompts for confirmation
unless --yes is given.

```
kuberoutectl target clear [flags]
```

### Options

```
  -h, --help   help for clear
  -y, --yes    skip the confirmation prompt
```

### Options inherited from parent commands

```
  -o, --output string   output format: text|json (default "text")
```

### SEE ALSO

* [kuberoutectl target](kuberoutectl_target.md)	 - Inspect, label, and use Kubernetes targets

