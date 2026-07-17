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

## macOS (Homebrew)

The simplest path on macOS — installs and updates via `brew`, and handles the
unsigned-binary quarantine for you (no manual `xattr` step):

```bash
brew install ymedlop/tap/kuberoutectl
kuberoutectl version
```

`brew upgrade kuberoutectl` picks up new releases. Prefer a manual download?
Use the cross-platform instructions below.

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

## Windows

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
