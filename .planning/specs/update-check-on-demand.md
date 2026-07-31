# Spec: on-demand update check

**Created**: 2026-07-26 (rewritten 2026-07-28)
**Status**: shipped in #114
**Author**: Yeray Medina López
**Epic**: none
**Target release**: v1.2.0

**Supersedes** the ambient stderr-notice design in this file's earlier revision
(`update-check-notice.md`, see git history). That design is preserved as a
rejected alternative at the end, with the reasoning — it was not wrong, it was
disproportionate.

---

## Problem

A user who installed `kuberoutectl` months ago has no way to learn that a newer
version exists short of visiting the repo. The concrete cost is not missing a
feature — it is **debugging a bug that is already fixed upstream**. The AWS
`sso_session` fix in 1.1.0 is the case that hurt: users on 1.0.0 see `unknown`
credential health with no suggested action, conclude the tool is broken for their
setup, and have no signal that the fix is one upgrade away.

Note what that story actually contains: the user is **already troubleshooting**.
They are not idly running `target list`; something is wrong and they are looking
for why. That is the moment worth intercepting, and it already has a command —
`doctor`, whose entire job is "is my environment set up correctly".

Package managers are a partial answer, not a full one. `brew`, `scoop` and the
Cloudsmith `apt` repo can all deliver the upgrade, but none of them *tell* the
user to run it, and people go months without `brew upgrade`.

## Goal

A user troubleshooting `kuberoutectl` learns, without leaving the command they
already ran, that their binary is out of date — and no network request is ever
made by a command that is not about diagnostics.

Verifiable:

1. `kuberoutectl doctor` on an outdated stable build reports a `warn` check naming
   the current and latest versions, alongside the existing provider checks.
2. `kuberoutectl version --check-update` reports the same, and plain
   `kuberoutectl version` makes **no** network call.
3. `KUBEROUTECTL_NO_UPDATE_CHECK=1` produces no check row and **no** network call.
4. A `dev` or snapshot build never performs the check and never reports a version
   verdict.
5. Every other command — `sync`, `target`, `collection`, `current`, `mcp` — makes
   no outbound request, asserted with a fake HTTP client that fails the test if
   called.

## User stories

- **As a user whose setup misbehaves**, I want `doctor` to tell me my binary is
  months old, so I stop debugging something that is already fixed.
- **As someone scripting kuberoutectl or driving it from an AI agent**, I want no
  unsolicited network calls from the commands I actually run, so my logs stay
  clean and an air-gapped runner never stalls.
- **As a privacy-conscious or corporate user**, I want the outbound request
  confined to two clearly-named diagnostic commands and disableable with one
  variable, so I can answer "does this tool phone home" with a flat no.

## Requirements

### Must have

1. **Two entry points, both explicitly diagnostic**: the `doctor` command, and
   `version --check-update`. No other command performs the check, and there is no
   ambient notice on any command's output.

2. **`doctor` checks by default.** Behind an opt-in flag the check would only
   reach users who already suspected the answer, which is precisely the
   population that does not need it.

   *Amended 2026-07-28 (built)*: this originally said the feature "deliberately
   revises `DoctorService`'s contract", whose doc comment states it "does not
   attempt discovery or network calls". **It does not, and that turned out to be
   unnecessary.** `DoctorService` is untouched: the `doctor` command appends the
   row to what `Run()` returns, gated by `updatecheck.Enabled` before any client
   is constructed. The service's promise stays true, `Run()` keeps its signature,
   and a future consumer — the MCP server, per the Gate 1 open question below —
   inherits no network call by construction rather than by remembering to opt
   out. Breaking that contract was a cost the design did not have to pay.

3. **The result is a `services.Check`, not a banner.** It joins the existing rows
   and inherits their rendering and their `-o json` shape. This is what removes
   the entire class of problems the rejected design had to solve: there is no
   stdout/stderr split to get right, no MCP framing to protect, and `-o json`
   stays valid because the verdict is part of the structured payload rather than
   text printed beside it.

   | Situation | Status | Detail |
   |---|---|---|
   | up to date | `ok` | `1.2.0 is the latest release` |
   | newer available | `warn` | `1.2.0 is available (you have 1.0.0)` + releases URL |
   | check could not run | `ok` | names why: offline, rate limited, malformed response |

4. **A failed check is reported, never silently dropped.** Offline, DNS failure,
   timeout, HTTP 403 (GitHub allows 60 unauthenticated requests/hour per IP, and a
   corporate NAT shares one IP across everyone) and a malformed body all produce
   an `ok` row saying the check could not run — `ok` because nothing about the
   *environment* is wrong, and present because a check that vanishes when it fails
   is indistinguishable from one that was never wired up. Same principle as the
   provider error convention: resilient, but never silent.

   The single exception is an explicit opt-out, where the row is omitted entirely
   — the user asked for silence, which is consent rather than a silent skip.

5. **No upgrade instructions.** The tool cannot know whether it arrived via brew,
   scoop, apt, an rpm or a downloaded tarball, and a heuristic on the binary path
   can leave someone with two conflicting binaries. It names the version and links
   the releases page; the user knows how they installed it.

6. **Stable releases only, in both directions.** A build whose version is not a
   stable semver release (`dev`, `0.0.0-snapshot-<sha>`) skips the check entirely
   rather than comparing. A pre-release upstream (`v1.2.0-rc.1`) is never offered
   to a user on a stable version — the `latest` endpoint already excludes
   pre-releases, and the `prerelease` field is checked anyway rather than trusted.

7. **Opt-out**: `KUBEROUTECTL_NO_UPDATE_CHECK` set to any non-empty value
   suppresses the row and the request. Documented in the README and the docs site
   next to a plain statement that this is the only outbound request
   `kuberoutectl` makes and that it transmits nothing.

   *Amended 2026-07-28 (plan D1)*: this originally read "or a config field".
   There is no config file — `internal/config` resolves paths only, and
   `config.Default()` is its sole construction path — so a config-file loader
   would have to be built to host one boolean. The environment variable is the
   whole opt-out until a config file exists for other reasons.

8. **Bounded cost.** 3-second timeout. The command may take that long in the worst
   case; that is acceptable for a diagnostic the user invoked on purpose, and is
   exactly why no other command performs the check.

### Nice to have

- `doctor --offline` to skip the check for one invocation without setting the
  environment variable.
- Reporting how old the installed version is ("released 4 months ago"), which is
  more actionable than a version number for someone who does not track releases.

### Out of scope

- **An ambient notice on every command.** Rejected — see the section at the end.
- **Install-method detection and upgrade commands.** A heuristic on the binary
  path can produce actively harmful advice.
- **Self-update / in-place binary replacement.** Not building an updater.
- **Telemetry of any kind.** The request is a plain GET carrying no version, no
  identifier and no usage data.
- **A cached result.** The rejected design needed one because it printed on every
  command; here the user invoked a diagnostic and can wait for the answer.
  Dropping it removes the backoff policy, the clock-skew handling, the
  corrupt-file path and the concurrent-writer question along with it.
- Notifying about the rolling `development-snapshot` pre-release.
- Any new module dependency — stdlib `net/http` only.

## Data model

**None.** Nothing is persisted. This is the largest simplification over the
rejected design, which required a `CacheDir()/update-check.json` holding
`latest_version`, `last_success` and `last_attempt` — a file that then needed
atomic writes, a backoff policy, future-timestamp handling and corrupt-file
tolerance, all so that an ambient notice could avoid blocking.

## API changes

New internal package `internal/updatecheck`, so the HTTP call and the version
comparison stay out of Cobra handlers (AGENTS.md):

```go
// Latest fetches the newest stable release tag. ok=false when the check could
// not run, with a reason suitable for display.
func Latest(ctx context.Context, c *http.Client) (version string, ok bool, reason string)

// Newer reports whether latest is a newer stable release than current. ok=false
// when current is not a comparable stable version (dev, snapshot).
func Newer(current, latest string) (newer bool, ok bool)
```

`Newer` is pure and gets its own table test. String comparison is wrong here —
`"1.10.0" < "1.9.0"` lexically — and that is the kind of bug that stays invisible
until the tenth minor release.

Outbound request, the **first and only** one the core makes:

```
GET https://api.github.com/repos/ymedlop/kuberoutectl/releases/latest
Accept: application/vnd.github+json
```

Unauthenticated, 3s timeout. Only `tag_name` and `prerelease` are read.

CLI surface:

```
kuberoutectl doctor                    # + an update check row
kuberoutectl version --check-update    # + the same verdict
```

`--check-update` is a new flag, so **`make docs-reference` must run and its output
be committed** — the ladder's fourth rung, and the one that failed CI in #110.

`DoctorService` must take the fetcher as an injected dependency rather than
constructing it. The central assertions in this spec are about calls that are
*not* made, and those cannot be written against a client the service builds
itself.

## UI changes

```console
$ kuberoutectl doctor
CHECK             STATUS  DETAIL
provider:aws      ok      resolved at /usr/local/bin/aws
provider:azure    ok      resolved at /opt/homebrew/bin/az
provider:gcp      fail    gcloud not found in PATH
version           warn    1.2.0 is available (you have 1.0.0) — https://github.com/ymedlop/kuberoutectl/releases/latest

$ kuberoutectl version --check-update
kuberoutectl 1.0.0 (commit abc1234, built 2026-03-02T10:00:00Z)
1.2.0 is available — https://github.com/ymedlop/kuberoutectl/releases/latest
```

Offline, the row stays and says so:

```
version           ok      could not reach the releases API; skipped
```

## Edge cases

1. **Non-release build** — `dev` and `0.0.0-snapshot-<sha>` do not parse as
   comparable semver. Skip before any parsing and before any request; a naive
   compare either nags forever or crashes.
2. **Pre-release upstream** — `prerelease: true` yields no verdict, even though
   the endpoint should already have excluded it.
3. **Offline / air-gapped** — no error, no non-zero exit, a row explaining it, and
   at most a 3s wait.
4. **Rate limited (HTTP 403)** — indistinguishable from any other failure to the
   user, and treated identically. Worth naming separately in the test matrix
   because a shared corporate IP makes it the *likely* failure, not an exotic one.
5. **Malformed JSON from a 200 response** — the check could not run, and is
   reported as such. It must never read as "you are up to date": that is the
   update-check equivalent of the parse-failure bug fixed in #111.
6. **`-o json`** — the check is a `Check` object like any other, so the output
   stays valid JSON with no special handling. Asserted rather than assumed.
7. **Opt-out set** — no row, no request. The one case where silence is correct.
8. **Every non-diagnostic command** — `sync`, `target`, `collection`, `current`,
   `mcp` make no request, asserted with a client that fails the test if called.
9. **`doctor` with no network *and* no providers installed** — two independent
   failures must both be reported; neither may mask the other.

## Testing criteria

**Happy path**

- Outdated stable version + a fixture release payload → `warn` row naming both
  versions.
- Current version equal to latest → `ok`, no upgrade text.
- `version --check-update` prints the verdict; plain `version` does not, and makes
  no call.

**Edge cases**

- `Newer` table test: `1.0.0 < 1.1.0`; `1.10.0 > 1.9.0` (the case a string compare
  fails); equal versions; a `v` prefix on either side; `dev`; `0.0.0-snapshot-abc`;
  an empty string; a non-semver tag.
- Each suppression context (env var, config off, `dev` build, snapshot build)
  individually produces **no row and no HTTP call**, with a fake client that fails
  the test when invoked.
- Each failure mode (timeout, connection refused, 403, 500, malformed body,
  `prerelease: true`) produces a row saying the check could not run, and a zero
  exit code.
- `doctor -o json` stays parseable with the row present, absent, and failed.
- e2e: `doctor` against the fake environment, plus an assertion that
  `kuberoutectl mcp`'s stdio handshake is untouched — cheap insurance that the
  check never migrates onto a path where it would corrupt JSON-RPC framing.

**Explicitly not covered**: no test hits the real GitHub API. The payload shape is
fixture-driven from the documented `releases/latest` response, and that gap
belongs in the PR rather than papered over.

## Dependencies

- `internal/buildinfo` for the current version (exists; injected via `-ldflags`).
- `internal/services.DoctorService` — **unmodified** (see the amendment on
  requirement 2). The `doctor` command composes the extra row onto its result;
  the service itself never learns about HTTP or release versions.
- Go stdlib `net/http` — **the first network client in the core**. Today every
  external call is delegated to a cloud CLI through `execx`. This is an
  architectural first and must be called out in the PR and in the docs, not
  slipped in.
- No new module dependency, no persisted state, no new cache file.

---

## Rejected alternative: an ambient notice on every command

The earlier revision specified a one-line stderr notice printed by any command
when a cached check showed a newer version. It was well specified — stderr only,
cached so it never blocks, opt-out documented, no telemetry. It is rejected on
proportionality, not on correctness.

- **The audience it reaches is the narrowest slice of users.** The notice required
  stderr to be a TTY, which is right. But `kuberoutectl` ships an MCP server for
  AI agents, and agent invocations are not TTYs; neither are scripts or CI. After
  the suppression rules the audience is an interactive human — and most of those
  installed through brew, scoop or apt, which can already deliver the upgrade.
- **The cost sat in machinery, not in the feature.** Nine edge cases, a cache file
  with attempt/success backoff, TTY detection, a six-condition suppression matrix,
  stdout-purity assertions and MCP framing protection — all so one line could be
  printed safely beside output the tool must not disturb. Moving the verdict
  *into* a diagnostic command's structured output makes most of that machinery
  unnecessary rather than merely smaller: the hard problems (stdout purity, MCP
  framing, never blocking) stop existing instead of being solved.
- **The precedent is expensive for this tool specifically.** `kuberoutectl`
  handles cloud credentials in corporate environments. "The core makes an
  unsolicited outbound request on every invocation" is a sentence that has to be
  defended in every security review the tool ever faces. Confining the request to
  two commands whose names say "diagnostic" is a far easier sentence to defend.

**What is genuinely lost**: the user who never runs `doctor` and never finds out.
That is a real regression against the rejected design, accepted knowingly. The bet
is that the population hurt by an old binary and the population who run `doctor`
overlap heavily — because being hurt by an old binary is what sends people to
`doctor` in the first place.

If that bet proves wrong, the escalation path is additive and does not invalidate
this work: `internal/updatecheck` and `Newer` are exactly what an ambient notice
would need, so the rejected design remains buildable on top of this one.

---

## Gate 1 — checklist

**Completeness**
- [x] Problem clearly stated, with the concrete instance (1.0.0 users and the AWS
      SSO fix) and the observation that the user is already troubleshooting.
- [x] Goal measurable — five verifiable outcomes, including a negative one (no
      request from non-diagnostic commands).
- [x] User stories present (3): the human, the script/agent, the security-conscious
      operator.
- [x] Requirements split must / nice / out of scope.
- [x] Out of scope names the rejected option and links to its full reasoning.

**Data model**
- [x] Nothing persisted, with the reason stated and the machinery it removes
      enumerated.

**API design**
- [x] Exact endpoint, headers, timeout and fields consumed.
- [x] Stated that nothing is transmitted outbound.
- [x] Business logic outside Cobra handlers, in `internal/updatecheck`.
- [x] The fetcher is injectable, because the central assertions are about calls
      *not* made and cannot be written otherwise.
- [x] New flag noted as requiring `make docs-reference`.

**Quality**
- [x] Edge cases listed (9).
- [x] Happy-path and edge-case testing criteria, including a no-HTTP-call
      assertion per suppression context.
- [x] Dependencies listed, with the architectural first flagged.
- [x] The unproven area (no test hits the real API) named.
- [x] A contract this feature deliberately breaks (`DoctorService` "no network
      calls") named rather than quietly violated.

**Open question for Gate 2**: whether the revised `DoctorService` behaviour should
be a constructor option (`WithUpdateCheck`) rather than unconditional, so a future
caller — the MCP server, if it ever exposes a doctor tool — can reuse the service
without inheriting a network call. Deciding this needs the plan's view of how the
CLI and MCP layers each build the service, which is why it is deferred rather than
guessed here.

**Gate 1: PASSED**
