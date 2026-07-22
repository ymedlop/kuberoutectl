---
title: kuberoutectl
layout: default
nav_order: 1
description: >-
  kuberoutectl is an open-source CLI that discovers, organizes, and routes
  Kubernetes access across Azure (AKS), AWS (EKS), GCP (GKE), and kubeconfig —
  one local inventory of your clusters and their credential health.
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

<div class="demo" markdown="0" style="margin: 2.25rem 0; text-align: center;">
  <img src="{{ '/assets/demo.gif' | relative_url }}"
       alt="kuberoutectl in action — sync four providers, list targets, inspect credential health, label and collect, then route kubectl"
       loading="lazy"
       style="max-width: 100%; height: auto; border-radius: 10px; box-shadow: 0 6px 28px rgba(0, 0, 0, 0.18);" />
  <span style="display: block; margin-top: 0.6rem; font-size: 0.85rem; opacity: 0.7;">Discover → organize → route, across all four providers.</span>
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

## Why kuberoutectl

Every extra cloud means another CLI, another identity, and a few more unreadable
`~/.kube/config` contexts. `kuberoutectl` collapses that into one local inventory
of what you can reach and whether it's healthy — so you route `kubectl` to the
right cluster in seconds.

## Quick Start

```bash
brew install ymedlop/tap/kuberoutectl   # macOS — all install methods in the guide below
kuberoutectl doctor                      # is the provider CLI reachable?
kuberoutectl sync azure                  # discover clusters + credential health (also: aws | gcp | kubeconfig)
kuberoutectl target list                 # what can I reach, and is it healthy?
kuberoutectl target use <alias>          # route kubectl at the right cluster
```

## Learn more

- **[Installation]({{ '/installation/' | relative_url }})** — every platform: Homebrew, apt, Scoop, packages, manual.
- **[Provider guides]({{ '/guides/' | relative_url }})** — Azure · AWS · GCP · kubeconfig, step by step.
- **[Organizing: labels & collections]({{ '/organizing/' | relative_url }})** — tag clusters and build selector-driven views.
- **[Command reference]({{ '/reference/' | relative_url }})** — every command and flag, generated from the CLI.
- **[Concepts & architecture](https://github.com/ymedlop/kuberoutectl#core-concepts)** — the domain model and design principles ([ARCHITECTURE.md](https://github.com/ymedlop/kuberoutectl/blob/main/ARCHITECTURE.md) for depth).
- **[Contributing](https://github.com/ymedlop/kuberoutectl)** — source, building, and development workflow.
