# Archived agent rules

These files came from the Spartan AI toolkit template and describe a
**Kotlin / Micronaut / PostgreSQL / React** stack:

- `TIMEZONE.md` — UTC handling for a JVM backend + SQL + browser frontend.
- `NAMING_CONVENTIONS.md` — Jackson/Axios snake_case↔camelCase, Micronaut
  `@QueryValue`, Exposed tables, etc.

`kuberoutectl` is a **Go CLI** with no database and no frontend, so none of
this applies. They were moved out of `.claude/rules/` (which is auto-loaded
into every agent turn) to keep the working context lean and avoid misleading
guidance. They are kept here only for reference.

The Go-specific guidance that *does* apply lives in:

- `AGENTS.md` — repo-wide rules (domain model, architecture, providers).
- `.claude/skills/go-development` — loaded on demand when writing Go.
- `.claude/rules/core/SKILL_AUTHORING.md` — still applies when authoring skills.

If this project ever grows a service with a database or web UI, adapt and move
the relevant rule back under `.claude/rules/`.
