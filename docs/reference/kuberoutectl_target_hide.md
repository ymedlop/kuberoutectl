---
title: "kuberoutectl target hide"
parent: Command reference
layout: default
description: "Hide targets from the default list (persists across resyncs)"
---

## kuberoutectl target hide

Hide targets from the default list (persists across resyncs)

### Synopsis

Hide one target by ref, or many with --selector. Hidden targets are
remembered in user state and stay hidden across resyncs. They still
appear under `target list --all` (and `--selector hidden=true`), and can
be revealed again with `target unhide`.

```
kuberoutectl target hide [<alias|id|name>] [flags]
```

### Options

```
  -h, --help                   help for hide
  -l, --selector stringArray   hide all targets matching this selector (repeatable)
```

### Options inherited from parent commands

```
  -o, --output string   output format: text|json (default "text")
```

### SEE ALSO

* [kuberoutectl target](kuberoutectl_target.md)	 - Inspect, label, and use Kubernetes targets

