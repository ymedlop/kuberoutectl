---
title: "kuberoutectl target unhide"
parent: Command reference
layout: default
description: "Reveal previously hidden targets"
---

## kuberoutectl target unhide

Reveal previously hidden targets

### Synopsis

Reveal one target by ref, or many with --selector (e.g.
`--selector hidden=true` to reveal everything currently hidden).

```
kuberoutectl target unhide [<alias|id|name>] [flags]
```

### Options

```
  -h, --help                   help for unhide
  -l, --selector stringArray   reveal all targets matching this selector (repeatable)
```

### Options inherited from parent commands

```
  -o, --output string   output format: text|json (default "text")
```

### SEE ALSO

* [kuberoutectl target](kuberoutectl_target.md)	 - Inspect, label, and use Kubernetes targets

