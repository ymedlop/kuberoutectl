---
name: Daily Repo Status
description: Generate a daily summary of repository activity and maintenance opportunities.
engine: copilot
---

# Daily Repo Status

## Goal
Create a concise daily report for maintainers that summarizes recent activity and highlights the next most useful actions.

## What to include
- New and updated issues.
- Open pull requests.
- Recent commits that changed important parts of the repo.
- Documentation gaps or stale docs.
- Testing, CI, or release process problems.
- Suggestions for the next small maintainer action.

## Style
- Be short and practical.
- Prioritize actionable items.
- Avoid repeating noise.
- Use bullet points with clear labels.

## Process
1. Inspect recent repository activity.
2. Group related items together.
3. Identify the highest-value maintenance task.
4. Write the result as a new issue or update an existing one.
5. Keep the report readable in under 1 minute.

## Output
Create an issue titled `Daily Repo Report`.
