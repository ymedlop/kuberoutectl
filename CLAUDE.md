# CLAUDE.md

@AGENTS.md

## Claude Code

Use this repository-level file for Claude-specific guidance.

### Behavior
- Prefer concise, testable changes.
- Follow the repo architecture and prompt files.
- Keep generated implementation aligned with the latest prompt in `prompts/claude-code/`.
- When instructions conflict, follow `AGENTS.md` first, then this file.

### Workflow
- Read `README.md` and `ARCHITECTURE.md` before major changes.
- Read the latest prompt in `prompts/claude-code/` before implementing.
- Keep `prompts/claude-code/CHANGELOG.md` updated when prompt intent changes.

### Scope
- This file is for Claude Code only.
- `AGENTS.md` remains the canonical repo-wide agent instruction file.
