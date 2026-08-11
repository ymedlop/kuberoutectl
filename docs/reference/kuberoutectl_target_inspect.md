---
title: "kuberoutectl target inspect"
parent: Command reference
layout: default
description: "Show a single target in detail, including labels"
---

## kuberoutectl target inspect

Show a single target in detail, including labels

### Synopsis

Show a single target in detail, including labels.

Operability comes from the last `sync`. Pass --refresh to re-establish it
against the provider instead — one extra API call, for the cluster named.

```
kuberoutectl target inspect <alias|id|name> [flags]
```

### Options

```
  -h, --help      help for inspect
      --refresh   re-check operability against the provider instead of using the last sync
```

### Options inherited from parent commands

```
  -o, --output string   output format: text|json (default "text")
  -v, --verbose         trace external CLI commands, exit codes, and their stderr on failure
```

### SEE ALSO

* [kuberoutectl target](kuberoutectl_target.md)	 - Inspect, label, and use Kubernetes targets

