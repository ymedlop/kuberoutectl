---
title: "kuberoutectl target list"
parent: Command reference
layout: default
description: "List discovered Kubernetes targets"
---

## kuberoutectl target list

List discovered Kubernetes targets

### Synopsis

List discovered Kubernetes targets.

Filter with --provider (azure|aws) and/or --selector (repeatable),
e.g. `--selector env=prod` or `--selector "region in [westeurope]"`.
Hidden targets are omitted by default; pass --all to include them, or
`--selector hidden=true` to list only hidden ones.
The ALIAS column is a short handle you can pass to `target use`,
`target inspect`, and `target label` instead of the full ID.

```
kuberoutectl target list [flags]
```

### Options

```
  -a, --all                    include hidden targets
  -h, --help                   help for list
  -p, --provider string        filter by provider (azure|aws)
  -l, --selector stringArray   filter by label selector (repeatable)
  -w, --wide                   also show the full target ID
```

### Options inherited from parent commands

```
  -o, --output string   output format: text|json (default "text")
```

### SEE ALSO

* [kuberoutectl target](kuberoutectl_target.md)	 - Inspect, label, and use Kubernetes targets

