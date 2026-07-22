---
title: "kuberoutectl collection create"
parent: Command reference
layout: default
description: "Create a collection from a selector and/or static targets"
---

## kuberoutectl collection create

Create a collection from a selector and/or static targets

### Synopsis

Create a saved view over targets.

Selectors accept key=value equalities (comma-joined or repeated) and
`key in [a, b]` in-lists, e.g.:
  --selector env=prod
  --selector env=prod,team=platform
  --selector "region in [westeurope, eu-west-1]"

```
kuberoutectl collection create <name> --selector <expr> [flags]
```

### Options

```
      --description string      human-readable description
  -h, --help                    help for create
      --selector key in [a,b]   selector clause (repeatable): key=value or key in [a,b]
      --static stringArray      explicit target ID to include (repeatable)
```

### Options inherited from parent commands

```
  -o, --output string   output format: text|json (default "text")
```

### SEE ALSO

* [kuberoutectl collection](kuberoutectl_collection.md)	 - Manage saved views over targets

