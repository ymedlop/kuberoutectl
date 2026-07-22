---
title: "kuberoutectl target use"
parent: Command reference
layout: default
description: "Select a target and fetch its credentials into ~/.kube/config"
---

## kuberoutectl target use

Select a target and fetch its credentials into ~/.kube/config

### Synopsis

Select a target as current. The target can be given by its short alias
(see `target list`), its full ID, or its name. By default this also fetches
the cluster's credentials into ~/.kube/config and sets it as the current
kubectl context (via the provider's native flow, e.g. az aks get-credentials /
aws eks update-kubeconfig). Use --no-kubeconfig to only record the selection.

```
kuberoutectl target use <alias|id|name> [flags]
```

### Options

```
  -h, --help            help for use
      --no-kubeconfig   record the selection only; do not modify ~/.kube/config
```

### Options inherited from parent commands

```
  -o, --output string   output format: text|json (default "text")
```

### SEE ALSO

* [kuberoutectl target](kuberoutectl_target.md)	 - Inspect, label, and use Kubernetes targets

