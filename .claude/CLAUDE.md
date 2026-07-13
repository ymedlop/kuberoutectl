# Spartan AI Toolkit — Engineering Manager Workflow

## Why Spartan?

Spartan commands are **pre-built, high-quality prompts** for workflows where free-form chat leads to missed steps. They don't replace Claude — they make Claude more reliable for structured work.

Without Spartan: "Create a PR" → Claude pushes code. Forgets to rebase, skips tests, no PR description.
With `/spartan:pr-ready`: 6-step checklist — rebase, tests, lint, architecture check, security scan, PR description generated. Devs usually forget 3 of these.

**When commands add value:** Structured workflows with multiple steps, checklists, or scaffolding that must follow specific conventions.
**When commands don't add value:** Questions, explanations, small code changes, brainstorming — just talk to Claude.

---

## Command or Chat? (Decision Rule)

```
What do you need?
│
├─ Question / explanation / brainstorm → Just ask Claude
├─ Small code change (< 30 min, ≤ 3 files) → Just ask Claude (Superpowers handles TDD/review)
├─ Structured workflow with checklist → Use a /spartan: command
└─ Don't know which command → Type /spartan (smart router asks what you need)
```

**Superpowers is always active.** When you say "review this" or "debug this" in normal chat, Claude auto-triggers the right skill. You don't need a command for that.

**Commands are for when the workflow matters more than the answer** — deploying, creating PRs, scaffolding new services, planning multi-day work.

---

## Task Size → Tool Routing

| Size | Use |
|---|---|
| < 30 min, ≤ 3 files | Just ask Claude (no command needed) |
| < 1 day | `/spartan:spec` → `/spartan:build` |
| 1–3 days | `/spartan:spec` → `/spartan:plan` → `/spartan:build` |
| Multi-feature work | `/spartan:epic` → then spec/plan/build each feature |


---

## Core Principles (Always Enforce)

### 1. Match the User's Language
**Detect the language of the user's message and respond entirely in that same language.** This is not optional — it overrides the default English behavior of all commands. If the user writes in Vietnamese, ALL output must be in Vietnamese. If in French, respond in French. If in English, respond in English. This applies to everything: explanations, questions, gate prompts, debug reports, summaries, and PR descriptions. Only code syntax, variable names, file paths, and command names (e.g., `/spartan:debug`) stay in their original form.

### 2. Spec Before Code
- Task < 1 day → `/spartan:spec` + `/spartan:plan` + `/spartan:build`
- Multi-feature work → `/spartan:epic` then spec/plan/build each feature
- Never write production code without a written spec or plan

### 3. TDD is Non-Negotiable
- Red → Green → Refactor, always
- Write tests first, then the code that makes them pass

### 4. Context Hygiene (Auto-Managed)
Claude proactively manages its own context window:
- When detecting context pressure (slow responses, forgetting earlier context, long conversation) → auto-run `/compact` to summarize and free space
- If compaction isn't enough → auto-save critical state to `.handoff/` and `.memory/`, then tell user to start a fresh session
- User can also manually trigger `/spartan:context-save` at any time
- Session > 60% → hard stop, no exceptions
- State is in `.planning/` (specs/plans), `.memory/` (permanent), or `.handoff/` (session), never in chat history

**Self-monitoring signals** (Claude watches for these in its own behavior):
- Starting to lose track of earlier decisions → compact NOW
- Repeating questions already answered → compact NOW
- Response quality dropping → warn user + compact
- Multi-step command taking unusually long → consider compacting between steps

### 5. Auto Mode
When user says **"auto on"** or **"auto mode"**, all Spartan commands skip confirmation prompts and execute straight through. Claude will:
- Show the spec/plan/output but NOT pause to ask "does this match?" or "shall I proceed?"
- Continue to the next step automatically after each step completes
- Still STOP for destructive actions (git force push, dropping tables, deleting files)
- Still SHOW output at each step (user can interrupt with "stop" or "wait")

Turn off with **"auto off"**. Default is **auto off** (commands ask for confirmation).

Auto mode is ideal for experienced users who trust the workflow and want maximum velocity.

### 6. Safety Guardrails

| Command | What it does |
|---|---|
| `/spartan:careful` | Warn before destructive ops (rm -rf, DROP, force-push) |
| `/spartan:freeze <dir>` | Lock edits to one directory only |
| `/spartan:guard <dir>` | Both combined. Deactivate with `off` or `/spartan:unfreeze` |

### 7. Intellectual Honesty
- **Push back** when the user's approach has clear problems — agreeing to avoid conflict is a failure mode. Say what's wrong, suggest alternatives, then let the user decide.
- **When confused:** STOP → name exactly what's unclear → present 2-3 options → wait. Never guess silently.
- **When wrong:** Admit it immediately. Don't quietly patch over a mistake — say "I was wrong about X, here's the correction."

---

## Core Commands (always available)

### Feature Workflow
```
/spartan:epic → /spartan:spec → [/spartan:ux] → /spartan:plan → /spartan:build → /spartan:pr-ready
                     ↑              ↑                 ↑              ↑ + 3.5           ↑
                   Gate 1      Design Gate          Gate 2         Gate 3            Gate 4
```

| Size | Path |
|---|---|
| Single feature (backend) | `/spartan:spec` → `/spartan:plan` → `/spartan:build` |
| Single feature (with UI) | `/spartan:spec` → `/spartan:ux prototype` → `/spartan:plan` → `/spartan:build` |
| Batch of features (1-2 weeks) | `/spartan:epic` → then spec/plan/build each feature |

### Workflows (start here)
| Command | Purpose |
|---|---|
| `/spartan` | **Smart router** — routes to the right workflow or command |
| `/spartan:build [backend\|frontend] "feature"` | Full feature workflow: understand → plan → TDD → review → PR |
| `/spartan:debug "symptom"` | Bug workflow: reproduce → investigate → fix → review → PR |
| `/spartan:onboard` | Codebase understanding: scan → map → setup |

### Spec & Plan (saved artifacts)
| Command | Purpose |
|---|---|
| `/spartan:spec "feature"` | Write a feature spec → saves to `.planning/specs/` → Gate 1 |
| `/spartan:plan "feature"` | Write implementation plan from spec → saves to `.planning/plans/` → Gate 2 |
| `/spartan:epic "name"` | Break big work into ordered features → saves to `.planning/epics/` |

### Quality Gates
| Command | Purpose |
|---|---|
| `/spartan:gate-review [phase]` | Dual-agent review (Gate 3.5) — builder + reviewer must both accept |

### Individual Commands
| Command | Purpose |
|---|---|
| `/spartan:pr-ready` | Pre-PR checklist + auto PR description |
| `/spartan:codex [sub]` | Second-opinion review via Codex CLI (review/ship/security/uncommitted/setup) |
| `/spartan:daily` | Standup summary from git log |
| `/spartan:init-project` | Auto-generate CLAUDE.md from codebase |
| `/spartan:context-save` | Manage context: compact first, full save if needed |
| `/spartan:update` | Upgrade Spartan to latest version |


---

## Git Branching

- `main` — production only, protected
- `develop` — integration branch
- `feature/{ticket}-{slug}` — new features
- `fix/{ticket}-{slug}` — bug fixes

---

## What NOT to Do
- Don't write code without a spec
- Don't skip tests
- Don't continue a session past 60% context
- Don't manually edit `.planning/` files — let the spec/plan commands handle them
- Don't commit secrets or hardcoded credentials
- Don't force a command when a simple chat answer is enough
