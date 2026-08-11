---
title: "kuberoutectl version"
parent: Command reference
layout: default
description: "Print build version information"
---

## kuberoutectl version

Print build version information

### Synopsis

Print build version information.

With --check-update, also ask GitHub whether a newer stable release exists.
That request happens only when you ask for it: `version` on its own — and
every command except `doctor` — makes no network call of its own. Set
KUBEROUTECTL_NO_UPDATE_CHECK to any value to disable the check everywhere.

```
kuberoutectl version [flags]
```

### Options

```
      --check-update   also check whether a newer stable release is available
  -h, --help           help for version
```

### Options inherited from parent commands

```
  -o, --output string   output format: text|json (default "text")
  -v, --verbose         trace external CLI commands, exit codes, and their stderr on failure
```

### SEE ALSO

* [kuberoutectl](kuberoutectl.md)	 - Discover, organize, and route Kubernetes access across providers

