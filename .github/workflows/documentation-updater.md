---
name: Documentation Updater
description: Detect docs drift and propose or apply documentation improvements.
engine: copilot
---

# Documentation Updater

## Goal
Keep repository documentation aligned with the current codebase, workflows, and contributor experience.

## What to include
- Missing or outdated README sections.
- Broken setup instructions.
- Incomplete developer workflow docs.
- Gaps between implementation and documentation.
- Places where examples no longer match current behavior.

## Style
- Prefer small doc fixes over large rewrites.
- Keep language clear and direct.
- Preserve existing tone unless it causes confusion.
- Focus on practical contributor value.

## Process
1. Review repository docs and recent code changes.
2. Detect mismatches or missing guidance.
3. Suggest the smallest useful documentation update.
4. If safe, prepare the doc change directly.
5. Keep the result easy to review in a PR.

## Output
Open a pull request or prepare a documentation issue with concrete edits.
