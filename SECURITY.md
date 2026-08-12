# Security Policy

## Supported versions

**Only the most recent stable release is supported.** Fixes land in the next
release cut from `main`; there is no backport branch, so the remedy for a
vulnerability is to upgrade.

| Version | Supported |
| ------- | --------- |
| 1.2.x   | :white_check_mark: |
| < 1.2   | :x: |

Releases before 1.0.0 do not exist — the project's first tag was `v1.0.0`.

## Reporting a vulnerability

**Use GitHub's private vulnerability reporting:** open the repository's
[Security tab](https://github.com/ymedlop/kuberoutectl/security/advisories/new)
and choose *Report a vulnerability*. The report stays private to you and the
maintainers until a fix is published.

Please do **not** open a public issue for a suspected vulnerability.

Include what you have:

- What the issue is, and which version (`kuberoutectl version`) you saw it on.
- Steps to reproduce, or the command sequence that triggers it.
- What an attacker gains — the impact matters more than the severity label.

Expect an acknowledgement within a week. This is a small project; if a report
goes unanswered longer than that, it has been missed rather than ignored, and a
nudge is welcome.

## What is in scope

`kuberoutectl` is a local CLI. It has no server, no account, and no network
service of its own — it drives the provider CLIs you already have installed.
That shapes what a vulnerability in it looks like:

**In scope**

- Anything that leaks credentials or tokens into the local cache, logs,
  `--verbose` output, or an error message.
- Command injection or unintended argument construction in the calls made to
  `az`, `aws`, `gcloud`, or `kubectl`.
- Binary resolution being tricked into executing something other than the
  intended provider CLI.
- Local state written with permissions that expose it to other users on the
  machine.
- Anything in the MCP server (`kuberoutectl mcp`) that lets a connected client
  reach beyond the tools it was given, or that exposes a write tool on a
  connection started read-only.
- Supply-chain problems in the release artifacts or the workflows that build
  them.

**Out of scope**

- Vulnerabilities in `az`, `aws`, `gcloud`, or `kubectl` themselves — report
  those to the respective vendors.
- Cloud permissions that are too broad. `kuberoutectl` reports what your
  credentials can reach; it does not grant access.
- The local cache not encrypting its contents. It stores discovered inventory,
  labels, collections, selection, and hidden targets — deliberately no
  credentials. Authentication stays with the provider CLIs and your kubeconfig,
  which is where the secrets live and where they are meant to stay.

## Release artifact integrity

Release binaries are **not signed**. Integrity is provided by `checksums.txt`
(SHA256), published with every release. Verify a download before running it —
see [Verifying a download](RELEASING.md#verifying-a-download).

The Homebrew cask clears the macOS Gatekeeper quarantine attribute on install,
because the binary is unsigned. If that trade is not acceptable in your
environment, build from source instead — the build is reproducible, and a
CI job verifies that the same commit produces byte-identical artifacts.

## What runs automatically

These reduce the odds of a vulnerability reaching a release; they do not
replace a report:

- **CodeQL** (`security-extended`) on every push and pull request to `main` and
  `development`, plus a weekly scheduled scan.
- **Dependabot** version and security updates across Go modules, GitHub Actions,
  and the docs site's gems.
- **Secret scanning** with push protection, so a committed credential is blocked
  rather than discovered later.
- **Reproducible-build verification** in CI, so a tampered or non-deterministic
  build is visible as a checksum mismatch.
