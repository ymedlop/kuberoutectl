---
title: "kuberoutectl target delete"
parent: Command reference
layout: default
description: "Delete a target from the local cache"
---

## kuberoutectl target delete

Delete a target from the local cache

### Synopsis

Delete a target from the local cache.

This is a cache cleanup, not a permanent exclusion: a later
`kuberoutectl sync <provider>` re-adds the target if the cluster still
exists. Scopes, credentials, and sources are left untouched.

```
kuberoutectl target delete <alias|id|name> [flags]
```

### Options

```
  -h, --help   help for delete
```

### Options inherited from parent commands

```
  -o, --output string   output format: text|json (default "text")
```

### SEE ALSO

* [kuberoutectl target](kuberoutectl_target.md)	 - Inspect, label, and use Kubernetes targets

