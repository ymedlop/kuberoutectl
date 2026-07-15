# Prompt changelog

This file tracks the evolution of prompt files used to guide `kuberoutectl` design and implementation.

## Why this exists

Prompts are part of the engineering workflow for this repository.

They influence:
- architecture,
- code generation,
- implementation order,
- naming,
- scope,
- and delivery expectations.

Keeping prompt history makes it easier to:
- understand why the generated output changed,
- roll back to a previous prompt shape,
- compare architectural decisions over time,
- and keep a stable record of intent.

## Changelog format

Each entry should include:
- date,
- prompt filename,
- summary of changes,
- reason for the change,
- expected implementation impact.

***

## 2026-07-15

### `kuberoutectl-v4-consolidated-cli.md`

#### Summary of changes
- Reframed the prompt from a greenfield milestone-1 build to maintaining a
  shipped, multi-provider CLI (Azure, AWS, GCP, kubeconfig all implemented).
- Replaced v3's flat per-entity commands with the **consolidated command
  surface**: `inventory sources|scopes|providers`, `setup aws-sso`, and the
  `target` → `clusters`/`cluster` aliases; removed the top-level
  `provider`/`source`/`scope`/`aws` commands.
- Documented that no provider is special at root — providers are a dimension
  (`sync <provider>`, `--provider`) so the cross-cloud view is preserved.
- Updated delivery to the full OS/arch matrix (incl. arm64), install docs, and
  the GitHub Pages site.
- Recorded the AWS Organizations account-discovery backlog item.
- Kept the domain model, health/action spectrum, labels, and collections
  unchanged (Scope and Target stay distinct).

#### Reason for the change
- v3's "Required CLI commands" and provider-status sections no longer matched
  the shipped tool after the CLI consolidation and the GCP/kubeconfig providers.
- The command surface is a design decision worth recording as the new source of
  truth, with the rationale for each grouping.

#### Expected implementation impact
- Future work should target the consolidated command tree and the four-provider
  package template.
- Provider-specific setup goes under `setup`, not a per-provider root command.
- `v3` is retained as the historical milestone-1 record.

***

## 2026-07-13

### `kuberoutectl-mvp-v3-azure-aws.md`

#### Summary of changes
- Set Azure as the first provider for the MVP.
- Set AWS as the second provider for the MVP.
- Kept the architecture provider-agnostic.
- Preserved future support for kubeconfig and GCP.
- Added labels and collections as first-class concepts.
- Reinforced separation between discovered inventory and user-owned metadata.
- Added snapshot-release-friendly delivery expectations for the `development` branch.

#### Reason for the change
- Azure and AWS are the most relevant real-world environments for early testing.
- The project needs practical operator value from the start.
- Labels and collections are required to organize clusters by environment, project, or team.
- Prompt evolution needs to track design intent in a stable way.

#### Expected implementation impact
- Azure provider should be implemented first.
- AWS provider should follow without changing the core domain model.
- User labels and collections need dedicated persistence and service boundaries.
- The repository should support prompt versioning as part of the normal engineering workflow.

***

## Template for future entries

```md
## YYYY-MM-DD

### `prompt-file-name.md`

#### Summary of changes
- ...
- ...

#### Reason for the change
- ...
- ...

#### Expected implementation impact
- ...
- ...
```
