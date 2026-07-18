---
title: Installation
layout: default
nav_order: 2
---

# Installation

Pre-built binaries come in two flavors: **stable** releases cut from `vX.Y.Z`
tags, and a rolling **`development-snapshot`** pre-release rebuilt on every push
to `development`. Each build ships **Windows, Linux, and macOS in both `amd64`
and `arm64`**. See [RELEASING.md](https://github.com/ymedlop/kuberoutectl/blob/main/RELEASING.md)
for how releases are produced and verified.

[Download from the releases page](https://github.com/ymedlop/kuberoutectl/releases){: .btn .btn-primary }

Assets are named `kuberoutectl_<version>_<os>_<arch>.<ext>` — `.tar.gz` for
Linux and macOS, `.zip` for Windows.

{: .note }
> Not sure which architecture you need? Run `uname -m` — `x86_64` → `amd64`,
> `aarch64`/`arm64` → `arm64`. On Windows, check **Settings → System → About →
> System type**.

Prefer to build it yourself? See
[Building from source](https://github.com/ymedlop/kuberoutectl#building-from-source).

## Choosing an install method

| Method | Best when | Trade-off |
|--------|-----------|-----------|
| **Package manager** (Homebrew, Scoop, `.deb`/`.rpm`/`.apk`) | you want install + upgrade handled for you | Homebrew/Scoop need a **published stable release** (see [Troubleshooting](#troubleshooting)) |
| **Direct download** (`.tar.gz` / `.zip`) | you want a single binary with no package manager, or you're on a pre-release | you upgrade manually |

Both install the same binaries from the same tagged release. If a package manager
command fails, jump to [Troubleshooting](#troubleshooting).

## macOS (Homebrew)

The simplest path on macOS — installs and updates via `brew`, and handles the
unsigned-binary quarantine for you (no manual `xattr` step):

```bash
brew install ymedlop/tap/kuberoutectl
kuberoutectl version
```

`brew upgrade kuberoutectl` picks up new releases. Prefer a manual download?
Use the cross-platform instructions below.

## Linux (packages)

Each release ships `.deb`, `.rpm`, and `.apk` packages (amd64 + arm64) as release
assets. Download the one for your distro and arch, then:

```bash
sudo dpkg -i kuberoutectl_*_amd64.deb          # Debian / Ubuntu
sudo rpm -i  kuberoutectl_*_amd64.rpm           # Fedora / RHEL / openSUSE
sudo apk add --allow-untrusted kuberoutectl_*_amd64.apk   # Alpine
kuberoutectl version
```

The binary lands on your `PATH` at `/usr/bin/kuberoutectl`. Packages are unsigned,
so `apk` needs `--allow-untrusted`.

## Linux and macOS (manual)

Download the asset matching your OS (`linux` | `darwin`) and arch
(`amd64` | `arm64`) from the releases page, then, from the folder where it
landed:

```bash
tar -xzf kuberoutectl_*_linux_amd64.tar.gz      # adjust os/arch to match
chmod +x kuberoutectl
sudo mv kuberoutectl /usr/local/bin/             # or any dir on your PATH
kuberoutectl version
```

{: .warning }
> On **macOS** the binary is unsigned, so Gatekeeper quarantines it on first
> run. Clear the quarantine flag once after extracting:
>
> ```bash
> xattr -d com.apple.quarantine ./kuberoutectl    # or: right-click → Open
> ```

## Windows (Scoop)

The simplest path on Windows — installs and updates via [Scoop](https://scoop.sh):

```powershell
scoop bucket add ymedlop https://github.com/ymedlop/scoop-bucket
scoop install kuberoutectl
kuberoutectl version
```

`scoop update kuberoutectl` picks up new releases. Prefer a manual download? See
below.

## Windows (manual)

Download the `..._windows_<arch>.zip` asset, extract it, and run from
PowerShell:

```powershell
Expand-Archive kuberoutectl_*_windows_amd64.zip -DestinationPath kuberoutectl
.\kuberoutectl\kuberoutectl.exe version
```

Move `kuberoutectl.exe` somewhere on your `PATH` to call it from anywhere.

{: .warning }
> SmartScreen may warn about the unsigned binary — choose **More info → Run
> anyway**.

## Verify the download (optional)

Each release includes `checksums.txt`:

```bash
sha256sum -c checksums.txt          # Linux
shasum -a 256 -c checksums.txt      # macOS
```

```powershell
Get-FileHash .\kuberoutectl_*.zip -Algorithm SHA256   # Windows (compare to checksums.txt)
```

## Verify the installed version

Confirm you're running the build you intended — `kuberoutectl version` prints the
version baked into the binary at release time, which matches the release tag:

```bash
kuberoutectl version
# kuberoutectl v1.2.3 (commit abc1234, built 2026-01-01T00:00:00Z)
```

If it reports an older version than you installed, an earlier binary is shadowing
it on your `PATH` — see [Troubleshooting](#troubleshooting).

## Troubleshooting

{: .note }
> **Homebrew and Scoop come online with the first stable `vX.Y.Z` release.** They
> publish to a tap/bucket only on stable tags, so until then `brew install` /
> `scoop install` won't find a formula/manifest — use a **direct download** or the
> **Linux packages** (both work from any release, including pre-releases).

**Homebrew — `Error: ... tap not found` or no formula**
: The tap isn't published yet (see the note above), or it's stale. If it exists,
  run `brew update` first, then `brew install ymedlop/tap/kuberoutectl`.

**Scoop — `Couldn't find manifest` / empty bucket**
: Same as above. After `scoop bucket add ymedlop https://github.com/ymedlop/scoop-bucket`,
  run `scoop update` so the new manifest is seen, then `scoop install kuberoutectl`.

**Linux `.apk` — `UNTRUSTED signature`**
: The packages are unsigned; add `--allow-untrusted`:
  `sudo apk add --allow-untrusted kuberoutectl_*_amd64.apk`.

**Linux `.rpm` — signature warning**
: Expected — the packages are unsigned. `sudo rpm -i` still installs.

**macOS (manual) — "kuberoutectl is damaged and can't be opened"**
: Gatekeeper quarantined the unsigned binary. Clear it:
  `xattr -d com.apple.quarantine ./kuberoutectl`. (Homebrew does this for you.)

**Windows (manual) — SmartScreen warning**
: Choose **More info → Run anyway** (the binary is unsigned).

**`command not found` after install**
: The install directory isn't on your `PATH`. Packages install to
  `/usr/bin/kuberoutectl`; for a manual install, move the binary somewhere on
  your `PATH`.

**`kuberoutectl version` shows an old version**
: An older binary earlier on your `PATH` is shadowing the new one. Find them with
  `which -a kuberoutectl` (Linux/macOS) or `where kuberoutectl` (Windows) and
  remove the stale one.

## Next steps

Once `kuberoutectl version` works, head to a
[provider guide]({{ '/guides/' | relative_url }}) for your cloud, or run the
universal loop:

```bash
kuberoutectl doctor              # is the provider CLI reachable?
kuberoutectl sync <provider>     # discover clusters + credential health
kuberoutectl target list         # what can I reach?
kuberoutectl target use <id>     # route kubectl at one cluster
```
