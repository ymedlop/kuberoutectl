# Changelog

All notable changes to `kuberoutectl` are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project aims to
follow [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

This file is maintained by hand (GoReleaser's changelog generation is disabled).

## [1.0.0] — first stable release

_Date is set when the `v1.0.0` tag is cut; see [RELEASING.md](RELEASING.md)._

The first **stable** public release. 1.0.0 is a **stability milestone**, not a
feature milestone: the core discover → organize → route workflow is complete and
the command surface is not expected to change in breaking ways.

### Discover
- Provider-agnostic core with a provider registry and capability flags.
- **Azure (AKS)**, **AWS (EKS)**, **GCP (GKE)**, and **kubeconfig** discovery.
- Kubeconfig contexts that duplicate a natively-discovered cluster (matched by
  API-server endpoint) are suppressed, so a cluster isn't inventoried twice.
- Normalized Kubernetes server version persisted per target (`unknown` when a
  provider has no source, e.g. kubeconfig).

### Organize
- User **labels** and selector-driven **collections** over targets; both survive
  every discovery resync (JSON `cache/` vs `state/` separation).
- Target **visibility** (`hide` / `unhide`, persistent) and ephemeral cache
  curation (`delete` / `clear`).
- Credential **health** awareness (valid / expiring / expired / static / unknown)
  with suggested actions; static credentials are never coerced into a renew flow.

### Route
- `target use` writes kubeconfig and points `kubectl` at the selected cluster;
  `current` reports what you're pointed at and cache freshness.
- Deterministic, `-o json`-capable inventory output.

### Distribute
- Cross-platform release artifacts for Windows, Linux, and macOS (amd64 + arm64):
  `.tar.gz` / `.zip` archives plus `checksums.txt`.
- Package-manager install paths: **Homebrew** (cask), **Scoop**, and Linux
  **`.deb` / `.rpm` / `.apk`** packages.
- **Reproducible builds** — the same commit produces byte-identical artifacts.
- Documented, repeatable release automation (draft GitHub release on a `vX.Y.Z`
  tag) with the packaging config validated in CI.

### Docs
- Provider guides, an installation guide with a troubleshooting section, and a
  labels & collections guide, published to GitHub Pages.

[1.0.0]: https://github.com/ymedlop/kuberoutectl/releases/tag/v1.0.0
