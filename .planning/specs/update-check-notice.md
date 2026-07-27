# Spec: update-available notice

**Created**: 2026-07-26
**Status**: draft
**Author**: Yeray Medina López
**Epic**: none
**Target release**: v1.2.0 — explicitly **not** v1.1.0, which is mid-flight

---

## Problem

A user who installed `kuberoutectl` months ago has no way to learn that a newer
version exists short of visiting the repo. The AWS `sso_session` fix shipping in
1.1.0 is exactly the case that hurts: users on 1.0.0 see `unknown` credential
health with no suggested action, conclude the tool is broken for their setup, and
have no signal that the fix is one upgrade away.

## Goal

Someone running any normal `kuberoutectl` command on an outdated release sees a
one-line notice on **stderr** telling them a newer version exists, and can turn
it off permanently in one step.

Success is verifiable:

- Running a build stamped `1.0.0` with a cached "latest = 1.1.0" prints the
  notice to stderr; stdout is byte-identical to a run without the notice.
- `KUBEROUTECTL_NO_UPDATE_CHECK=1` produces no notice and **no network call**.
- `kuberoutectl target list -o json` emits parseable JSON with the notice
  suppressed entirely.
- `kuberoutectl mcp` never emits the notice on any stream that the MCP client
  reads.
- No command is measurably slower — the notice never waits on the network.

## User Stories

- **As a user on an outdated release**, I want to be told a newer version exists,
  so that I stop debugging a bug that is already fixed upstream.
- **As someone running kuberoutectl in CI or a script**, I want no unsolicited
  network calls and no extra output, so that my logs stay clean and my
  air-gapped runner does not stall.
- **As a privacy-conscious user**, I want a documented, single-variable opt-out,
  so that I can make the tool network-silent again.

## Requirements

### Must have

1. **stderr only, never stdout.** `internal/cli/mcp.go:14-17` reserves stdout for
   the MCP JSON-RPC transport; `AGENTS.md` requires deterministic,
   machine-readable output. Writing the notice to stdout would corrupt MCP frames
   and break `-o json` parsing.
2. **Hard suppression contexts** — no notice *and* no check when any of:
   - the command is `mcp`,
   - any `-o json` / machine-readable output flag is set,
   - stderr is not a terminal (piped, redirected, CI),
   - `KUBEROUTECTL_NO_UPDATE_CHECK` is set to any non-empty value,
   - the config field disables it,
   - `buildinfo.Version` is not a stable release (`dev`, `0.0.0-snapshot-*`).
3. **Never blocks.** The printed notice always comes from a **cached** result
   written by a previous run. A user never waits on the network to see it.
4. **Minimal message, no upgrade instructions.** The tool does not know how it
   was installed (brew / scoop / apt / manual tarball), so it must not guess:
   wrong advice can leave a user with two conflicting binaries. Message shape:
   ```
   A newer kuberoutectl is available: 1.1.0 (you have 1.0.0)
   https://github.com/ymedlop/kuberoutectl/releases/latest
   ```
5. **Silent failure.** Network unreachable, DNS failure, timeout, HTTP 403
   (GitHub's unauthenticated rate limit is 60/h per IP, and corporate NAT shares
   one IP across many users) — all produce no output, no error, no non-zero exit.
6. **Backoff on failure.** Record last *attempt* separately from last *success*,
   so a permanently offline machine does not attempt a fetch on every single
   command.
7. **Stable releases only.** A pre-release tag (`v1.2.0-rc.1`) must never be
   offered to a user on a stable version.
8. **Opt-out documented** in README and the docs site, alongside a plain
   statement that this is the only outbound request kuberoutectl makes.

### Nice to have

- `kuberoutectl version --check-update` to force a synchronous check on demand,
  which also gives the opt-out crowd a way to check deliberately.
- Suppress the notice for N days after it has been shown once, rather than on
  every command, if it proves annoying in practice.

### Out of scope

- **Install-method detection and upgrade commands** — explicitly rejected: a
  heuristic on the binary path can produce actively harmful advice.
- **Self-update / in-place binary replacement.** Not building an updater.
- **Telemetry of any kind.** The request sends nothing but a plain GET; no
  version, no user identifier, no usage data.
- Notifying about the rolling `development-snapshot` pre-release.
- Any new third-party dependency — stdlib `net/http` only.

## Data Model

New file, `CacheDir()/update-check.json` (cache, not state: it is derived and
refetchable, so losing it to a cache wipe is harmless — unlike user labels,
which is what `StateDir()` protects).

```json
{
  "latest_version": "1.1.0",
  "last_success":   "2026-07-26T10:00:00Z",
  "last_attempt":   "2026-07-26T10:00:00Z"
}
```

Written through the existing `internal/cache/jsonstore` temp-file + rename path,
so a concurrent reader never sees a torn file and two simultaneous invocations
cannot corrupt it.

## API Changes

No CLI surface change beyond the environment variable and a config field. New
internal package `internal/updatecheck` — the network call and the semver
comparison live there, not in Cobra handlers (`AGENTS.md`: business logic stays
out of command handlers).

Outbound request — the **first and only** one the core makes:

```
GET https://api.github.com/repos/ymedlop/kuberoutectl/releases/latest
Accept: application/vnd.github+json
```

Unauthenticated. Timeout: 3s. Only `tag_name` and `prerelease` are read.

## UI Changes

Two lines on stderr, before the command's own output. Nothing else.

## Edge Cases

1. **MCP transport corruption.** A notice on stdout during `kuberoutectl mcp`
   breaks JSON-RPC framing for every client. → Suppressed by command name, and
   asserted in `scripts/e2e.sh` alongside the existing handshake check.
2. **`-o json` corrupted.** Even on stderr this is safe, but a future refactor
   could route it wrongly. → Test asserts stdout is byte-identical with and
   without a pending notice.
3. **Non-release build.** `dev` and `0.0.0-snapshot-<sha>` do not parse as
   comparable semver; a naive compare either crashes or nags forever. → Skip
   before any parsing.
4. **Pre-release newer than stable.** `v1.2.0-rc.1` published while the user is
   on `1.1.0` must not trigger a notice. → The API's `latest` endpoint already
   excludes pre-releases, but the `prerelease` field is checked anyway rather
   than trusted.
5. **Offline / air-gapped machine.** Must never stall a command or print an
   error. → 3s timeout on a goroutine whose result is abandoned if the command
   finishes first, plus attempt-based backoff.
6. **Rate limited (HTTP 403).** Many users behind one corporate IP exhaust 60
   req/h. → Treated as a normal failure: silent, backed off.
7. **Corrupt or hand-edited state file.** → Treated as empty; never fatal.
8. **Clock skew.** A `last_check` timestamp in the future (VM snapshot restore,
   bad NTP) must not disable checking forever. → A future timestamp is treated
   as "check due".
9. **Concurrent invocations.** Two shells running commands at once both refresh.
   → Atomic rename makes the last writer win harmlessly.

## Testing Criteria

### Happy path

- Cached `latest_version` newer than `buildinfo.Version` → notice on stderr,
  exact expected text.
- Cached version equal or older → no output.
- Fetch parses a fixture GitHub release payload into the right version.

### Edge cases

- Each suppression context (`mcp`, `-o json`, non-TTY stderr, env var, config
  off, `dev` version, snapshot version) individually produces **no notice and no
  HTTP call** — asserted with a fake HTTP client that fails the test if called.
- **stdout purity**: for `target list` and `target list -o json`, stdout is
  byte-identical with a pending notice and without it.
- Fetch failures — timeout, connection refused, HTTP 403, HTTP 500, malformed
  JSON — each produce no output and a zero exit code.
- `prerelease: true` payload → no notice.
- Corrupt state file → no crash, treated as empty.
- Future-dated `last_success` → check still runs.
- Semver comparison table: `1.0.0 < 1.1.0`, `1.10.0 > 1.9.0` (string compare
  would get this wrong), `1.1.0 == 1.1.0`, `v`-prefix normalization.
- e2e: `kuberoutectl mcp` handshake still clean with a pending update notice in
  the cache.

## Dependencies

- `internal/buildinfo` for the current version (already exists; `-ldflags`).
- `internal/cache/jsonstore` for atomic read/write (already exists).
- `internal/config` for the opt-out field and `CacheDir()`.
- Go stdlib `net/http` — **the first network client in the core**; today all
  external access is delegated to cloud CLIs via `execx`. This is an
  architectural first and must be called out in the PR and the docs.
- No new module dependency.

---

## Gate 1 — checklist

**Completeness**
- [x] Problem clearly stated, with a concrete instance (1.0.0 users and the AWS SSO fix)
- [x] Goal measurable (5 verifiable assertions)
- [x] User stories present (3)
- [x] Requirements split must / nice-to-have / out of scope
- [x] Out of scope section exists, and names the rejected option explicitly

**Data model**
- [x] Storage location chosen with a stated reason (cache vs state)
- [x] Concurrency/atomicity addressed (jsonstore rename)
- [x] Corrupt-file behaviour defined

**API design**
- [x] Exact endpoint, headers, timeout, and fields consumed
- [x] Stated that nothing is transmitted outbound
- [x] Business logic placed outside Cobra handlers per AGENTS.md

**Quality**
- [x] Edge cases listed (9)
- [x] Happy-path testing criteria
- [x] Edge-case testing criteria, including a no-HTTP-call assertion
- [x] Dependencies listed, with the architectural first flagged

**Gate 1: PASSED**
