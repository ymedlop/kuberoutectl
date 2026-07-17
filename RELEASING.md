# Releasing kuberoutectl

`kuberoutectl` ships two kinds of pre-built artifacts, both produced by
[GoReleaser](https://goreleaser.com) from a single [`.goreleaser.yaml`](.goreleaser.yaml).
Every release covers Windows, Linux, and macOS on both amd64 and arm64.

| Kind | Trigger | Workflow | GitHub release |
|------|---------|----------|----------------|
| **Snapshot** | every push to `development` | `snapshot-release.yml` | a single rolling `development-snapshot` pre-release, replaced each push |
| **Stable** | pushing a `vX.Y.Z` tag | `release.yml` | a **draft** release a maintainer reviews and publishes |

Neither kind is signed; integrity is provided by `checksums.txt` (SHA256). See
[Verifying a download](#verifying-a-download).

## Artifacts

Each release contains, per OS/arch:

- `kuberoutectl_<version>_<os>_<arch>.tar.gz` (Linux, macOS)
- `kuberoutectl_<version>_<os>_<arch>.zip` (Windows)

plus a single `checksums.txt` with the SHA256 of every archive.

Build metadata (`Version`, `Commit`, `Date`) is embedded into the binary via
`-ldflags -X` and shown by `kuberoutectl version`.

## Cutting a stable release

1. **Pick a green commit.** Tag only a commit that has already passed CI on
   `main` — `ci.yml` runs the full gate there, including `scripts/e2e.sh`. The
   release workflow's own gate is `make check` (fmt + vet + test) and does
   **not** re-run the e2e flow, so a tag off an unverified commit could ship
   something e2e would have caught.

2. **Tag and push** using a semver tag:

   ```bash
   git tag v1.2.3            # or a pre-release: v1.2.3-rc.1
   git push origin v1.2.3
   ```

   `release.yml` runs on the tag: it verifies with `make check`, then builds all
   OS/arch archives + `checksums.txt` and uploads them to a **draft** GitHub
   release. `prerelease` is auto-detected — `v1.2.3` is a full release,
   `v1.2.3-rc.1` is marked pre-release. Both land as drafts.

3. **Review and publish.** Open the draft release on GitHub, confirm the
   artifacts and generated notes look right, and click **Publish**. Nothing is
   public until you do. **Publish promptly** — the Homebrew cask (below) points at
   the release's archive URLs, which only resolve once the draft is published.

## Homebrew tap

On a stable release, GoReleaser also generates a Homebrew **cask** and pushes it
to a separate tap repo, so macOS users can
`brew install ymedlop/tap/kuberoutectl`.

### One-time setup (before the first stable tag)

The tap push needs a repo and a token that the default `GITHUB_TOKEN` can't
provide (it can't write to another repository):

1. **Create the tap repo** — `ymedlop/homebrew-tap`, public. Homebrew requires
   the `homebrew-` prefix; it's referenced as `ymedlop/tap`.
   ```bash
   gh repo create ymedlop/homebrew-tap --public \
     --description "Homebrew tap for kuberoutectl"
   ```
2. **Create a fine-grained PAT** with **Contents: write** on `ymedlop/homebrew-tap`
   only.
3. **Store it as a secret on THIS repo** (not the tap — the workflow runs here):
   ```bash
   gh secret set HOMEBREW_TAP_GITHUB_TOKEN --repo ymedlop/kuberoutectl
   ```

If the secret is missing when you tag, the release still creates the draft and
uploads all archives — only the tap push fails. So a **red `release.yml` run may
still have shipped a draft**: check the Releases page before assuming nothing
happened, and delete the stray draft if you're re-cutting.

### Behavior

- **Stable tags only.** `skip_upload: auto` skips the cask push for pre-releases
  (`vX.Y.Z-rc.N`) and snapshots, so the tap always points at a real stable
  version. A `v0.0.1-rc.1` therefore exercises `release.yml` but **not** the tap
  — the tap is first updated by your first stable `vX.Y.Z`.
- **Version alignment is automatic** — the cask version and the binary's embedded
  version both come from the git tag.
- **No `xattr` step for users.** The cask clears the Gatekeeper quarantine on
  install (unsigned binary), so `brew install` gives a runnable binary directly.

## Snapshots

Snapshots are automatic: every push to `development` rebuilds the artifacts and
replaces the `development-snapshot` pre-release, so testers always have the
latest build without cutting a formal release. No manual step is needed.

## Verifying a download

Download the archive for your platform plus `checksums.txt` from the release,
then verify:

```bash
# Linux
sha256sum -c checksums.txt

# macOS
shasum -a 256 -c checksums.txt
```

```powershell
# Windows (PowerShell)
Get-FileHash .\kuberoutectl_*.zip -Algorithm SHA256
```

`-c` checks every listed file present in the current directory and reports `OK`.

## Validating the packaging config

The GoReleaser config is validated on every pull request by the
`goreleaser-check` job in `ci.yml` (`goreleaser check`), so a broken release
setup fails the PR rather than the tag push. To validate locally:

```bash
goreleaser check
```
