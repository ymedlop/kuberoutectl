---
title: Home
layout: default
nav_order: 1
description: >-
  Discover, organize, and route Kubernetes access across Azure, AWS, GCP,
  and kubeconfig — from one provider-agnostic CLI.
---

<div class="hero" markdown="0">
  <span class="hero__eyebrow">Open source · Go · Apache-2.0</span>
  <h1 class="hero__title">kuberoutectl</h1>
  <p class="hero__tagline">
    Discover, organize, and route Kubernetes access across Azure, AWS, GCP,
    and kubeconfig — from one provider-agnostic CLI that keeps a local
    inventory of your clusters and credential health.
  </p>
  <div class="hero__actions">
    <a class="hero__btn hero__btn--primary" href="{{ '/installation/' | relative_url }}">Install</a>
    <a class="hero__btn hero__btn--ghost" href="#quick-start">Quick start</a>
    <a class="hero__btn hero__btn--ghost" href="{{ '/guides/' | relative_url }}">Provider guides</a>
    <a class="hero__btn hero__btn--ghost" href="{{ '/organizing/' | relative_url }}">Organizing</a>
  </div>
  <div class="provider-strip">
    <span class="provider-strip__label">Works with</span>
    <span class="provider-strip__item">Azure AKS</span>
    <span class="provider-strip__item">AWS EKS</span>
    <span class="provider-strip__item">GCP GKE</span>
    <span class="provider-strip__item">kubeconfig</span>
  </div>
</div>

<div class="feature-grid" markdown="0">
  <div class="feature-card">
    <div class="feature-card__icon">🔎</div>
    <h3>Discover</h3>
    <p>One <code>sync</code> per provider populates a local inventory of clusters, scopes, and credentials — no manual bookkeeping.</p>
  </div>
  <div class="feature-card">
    <div class="feature-card__icon">🫀</div>
    <h3>Health-aware</h3>
    <p>Every credential carries a health state — valid, expiring, expired, static — and a suggested next action.</p>
  </div>
  <div class="feature-card">
    <div class="feature-card__icon">🏷️</div>
    <h3>Organize</h3>
    <p>Label targets and save selector-driven collections that span clouds and survive every resync.</p>
  </div>
  <div class="feature-card">
    <div class="feature-card__icon">🧭</div>
    <h3>Route</h3>
    <p><code>target use</code> writes kubeconfig and points <code>kubectl</code> at the right cluster in one step.</p>
  </div>
</div>

<div class="demo" markdown="0" style="margin: 2.25rem 0; text-align: center;">
  <img src="{{ '/assets/demo.gif' | relative_url }}"
       alt="kuberoutectl in action — sync four providers, list targets, inspect credential health, label and collect, then route kubectl"
       loading="lazy"
       style="max-width: 100%; height: auto; border-radius: 10px; box-shadow: 0 6px 28px rgba(0, 0, 0, 0.18);" />
  <span style="display: block; margin-top: 0.6rem; font-size: 0.85rem; opacity: 0.7;">Discover → organize → route, across all four providers.</span>
</div>

## Why kuberoutectl

`kuberoutectl` is built to solve a real operational problem: **managing Kubernetes access across multiple cloud providers is fragmented**.

### The Problem

Operators often need to move between:
- Multiple cloud providers (Azure, AWS, GCP, self-hosted)
- Multiple identities or subscriptions/accounts
- Multiple clusters per environment
- Different local access methods

The current toolchain gives you pieces — one CLI for auth, another for context switching, another for inspection — but no single operator-focused layer that keeps an organized local inventory of access and lets you route to the right cluster quickly.

### The Solution

`kuberoutectl` fills that gap by:

- **Discovering** Kubernetes access targets from supported providers (Azure, AWS, GCP, kubeconfig)
- **Caching** discovered inventory locally for quick access
- **Detecting** credential health — valid, expiring, expired, static, or unknown
- **Helping** users renew or re-authenticate credentials when supported
- **Organizing** targets with user-defined labels and collections
- **Keeping** provider logic behind a provider-agnostic core

## Quick Start

If you're already familiar with `kuberoutectl`, here's the universal workflow:

```bash
kuberoutectl doctor              # 1. is the provider CLI reachable?
kuberoutectl sync <provider>     # 2. discover clusters + credential health
kuberoutectl credential list     # 3. what's valid / expiring / expired?
kuberoutectl target list         # 4. what can I reach?
kuberoutectl target use <id>     # 5. route kubectl at one cluster
```

## Core Concepts

The CLI is built around a stable domain model that works identically across all providers:

- **Provider**: source of access such as `azure`, `aws`, `gcp`, or `kubeconfig`
- **AccessSource**: concrete source of access data (Azure CLI profile, AWS profile, kubeconfig file)
- **Credential**: usable identity inside a provider
- **Scope**: administrative or logical boundary (subscription, account, project)
- **Target**: selectable Kubernetes destination (AKS, EKS, GKE, or kubeconfig context)
- **Labels**: key/value metadata used to organize targets
- **Collections**: saved logical views over targets, driven by label selectors

## Documentation Structure

### [Organizing: labels & collections](organizing.md)

How to tag clusters with labels and group them into live, selector-driven
collections that span clouds — including the create-first, label-later workflow.

### [Provider Guides](guides/index.md)

Step-by-step manuals for using `kuberoutectl` with each supported cloud:

- **[Azure (AKS)](guides/azure.md)** — managing AKS clusters and credentials with Azure CLI
- **[AWS (EKS)](guides/aws.md)** — managing EKS clusters across profiles and accounts
- **[GCP (GKE)](guides/gcp.md)** — managing GKE clusters with gcloud
- **[kubeconfig](guides/kubeconfig.md)** — self-hosted, local, and handed-to-you contexts

Each guide covers:
1. **Setting up the provider** — ensuring your CLI is configured and authenticated
2. **Discovering clusters** — using `sync` to populate the local cache
3. **Checking credential health** — understanding what's valid, expiring, or expired
4. **Managing clusters** — inspecting, selecting, and routing to targets
5. **Organizing with labels** — tagging clusters for easy filtering
6. **Creating collections** — saving views with selectors

### [Shared Model](guides/index.md)

The guides reference a shared domain model that lets the same commands work identically across all providers. This section explains:
- How each cloud provider maps to the universal model
- The credential health spectrum
- The universal workflow loop

## Commands

Every inventory command supports `--output json` (`-o json`). The full
command reference lives in the [README](https://github.com/ymedlop/kuberoutectl#commands), and per-cloud
walkthroughs are in the [provider guides](guides/index.md).

## Architecture & Design Principles

- **Provider-agnostic core**: provider-specific logic stays behind interfaces
- **User-owned organization**: labels and collections survive discovery resyncs
- **Cache first**: local inventory for fast access and organization
- **No secret vault**: the cache stores inventory, not credentials
- **Operator-focused UX**: answers practical questions quickly

For deeper architectural details, see the main [README.md](https://github.com/ymedlop/kuberoutectl/blob/main/README.md) or [ARCHITECTURE.md](https://github.com/ymedlop/kuberoutectl/blob/main/ARCHITECTURE.md).


## Getting Help

- **New to kuberoutectl?** Start with the [Quick Start](#quick-start) and a provider guide for your cloud.
- **Setting up a specific cloud?** Jump to [Azure](guides/azure.md), [AWS](guides/aws.md), [GCP](guides/gcp.md), or [kubeconfig](guides/kubeconfig.md).
- **Understanding credential health?** See [Credential Health, Once](guides/index.md#credential-health-once).
- **Advanced workflows?** See the [command reference](https://github.com/ymedlop/kuberoutectl#commands) in the README.

## Contributing

`kuberoutectl` is open source. For source code, building, and development workflow, see the main [README.md](https://github.com/ymedlop/kuberoutectl/blob/main/README.md) and [ARCHITECTURE.md](https://github.com/ymedlop/kuberoutectl/blob/main/ARCHITECTURE.md).

## License

Apache License 2.0. See [LICENSE](https://github.com/ymedlop/kuberoutectl/blob/main/LICENSE) for details.
