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
