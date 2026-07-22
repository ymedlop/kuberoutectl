---
title: "kuberoutectl current"
parent: Command reference
layout: default
description: "Show the currently selected target or collection"
---

## kuberoutectl current

Show the currently selected target or collection

### Synopsis

Show what you are pointed at: the target or collection recorded by the
last `target use` / `collection use`, its health as of the last sync, and
how fresh that information is.

```
kuberoutectl current [flags]
```

### Options

```
  -h, --help   help for current
```

### Options inherited from parent commands

```
  -o, --output string   output format: text|json (default "text")
```

### SEE ALSO

* [kuberoutectl](kuberoutectl.md)	 - Discover, organize, and route Kubernetes access across providers

