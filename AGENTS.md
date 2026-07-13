# AGENTS.md

## Purpose

This repository builds `kuberoutectl`, an open source Go CLI for discovering, organizing, and using Kubernetes access across multiple providers.

This file defines standing instructions for AI coding agents working in this repo. Keep it short, stable, and focused on rules that remain true as the code evolves.

## Product direction

- `kuberoutectl` is a provider-agnostic Kubernetes access routing CLI.
- The first real MVP starts with **Azure** and then **AWS**.
- Future providers include **kubeconfig** and **GCP**.
- The product is not just a context switcher or just a wrapper around cloud CLIs.
- The product includes local inventory, credential health awareness, labels, and collections.

## Core domain model

Always preserve these domain distinctions:

- **Provider**: backend such as `azure`, `aws`, `gcp`, `kubeconfig`
- **AccessSource**: concrete source of local/provider access data
- **Credential**: usable identity inside a provider
- **Scope**: administrative or logical boundary such as subscription, account, profile scope, or future project
- **Target**: selectable Kubernetes destination such as AKS, EKS, future GKE, or future kubeconfig context
- **Collection**: saved logical grouping of targets
- **Labels**: metadata used to organize targets

Do **not** collapse `Scope` and `Target` into one type, even if a provider seems simple.

## Architecture rules

- Keep the core provider-agnostic.
- Do not spread provider-specific conditionals across services.
- Put provider behavior behind explicit interfaces and a provider registry.
- Keep business logic out of Cobra command handlers.
- Prefer explicit code over reflection or plugin-style indirection.
- Use JSON persistence first; do not introduce SQLite unless the task explicitly requires it.
- Do not turn the local cache into a secret vault.

## Provider priorities

Current implementation order:
1. Azure
2. AWS
3. kubeconfig
4. GCP

When implementing Azure or AWS, keep the abstractions suitable for later kubeconfig support, where some credentials may be static and not renewable.

## Labels and collections

- Targets support both **system labels** and **user labels**.
- User labels must survive discovery resyncs.
- Collections are first-class saved views over targets, primarily driven by selectors, with optional static target membership.
- Do not model collections as simple folders.

## Binary resolution

When external CLIs are needed, resolve binaries in this order:
1. explicit path from config
2. managed runtime installed by `kuberoutectl`
3. PATH lookup
4. clear diagnostic error

Managed runtime support is optional and must not be the default assumption.

## Repository workflow

- Use `README.md` for project overview.
- Use `ARCHITECTURE.md` for the technical design.
- Keep evolving implementation prompts under `prompts/claude-code/`.
- Prompt files are versioned project assets; do not overwrite historical prompts unless explicitly asked.
- Snapshot builds are expected from the `development` branch through GitHub Actions draft releases.

## Coding expectations

- Prefer small, testable services.
- Add or update tests when changing domain logic, persistence, selector behavior, or provider parsing.
- Keep outputs deterministic where possible.
- Prefer machine-readable JSON output support for CLI commands that expose inventory.
- Use clear naming over clever abstractions.

## What agents should do before major implementation

Before making large architectural changes:
1. read `README.md`
2. read `ARCHITECTURE.md`
3. read the latest prompt in `prompts/claude-code/`
4. preserve the domain model unless the task explicitly asks to revise it

## What does not belong here

Do not put temporary task prompts, long research notes, or implementation transcripts in this file. Put evolving prompts in `prompts/claude-code/` instead.
