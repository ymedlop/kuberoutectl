#!/usr/bin/env bash
#
# Regenerate the demo GIF (assets/demo.gif for the README + docs/assets/demo.gif
# for the Pages site) deterministically from the
# committed provider fixtures — no real cloud, no credentials, no secrets.
#
#   make demo            # or: bash scripts/demo.sh
#   DRY=1 bash scripts/demo.sh   # run the flow only, skip recording (for debugging)
#
# It mirrors scripts/e2e.sh: it builds the CLI and puts fake `az`/`aws`/`gcloud`/
# `kubectl` on PATH that serve internal/providers/*/testdata, all inside a
# throwaway HOME. Then it records the operator flow with asciinema and renders it
# to a GIF with agg.
#
# Rendering uses asciinema + agg (not vhs): agg rasterizes with an embedded font
# renderer, so it needs no headless browser and runs anywhere CI does. The command
# sequence in the `driver` below is the human-readable source of truth for the GIF
# — keep it in sync with the CLI (scripts/verify-readme-commands.sh checks it).
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

# Required tools (unless DRY=1, which only needs the Go toolchain).
need() { command -v "$1" >/dev/null 2>&1 || { echo "missing tool: $1 — $2" >&2; return 1; }; }
missing=0
need go "install Go (https://go.dev/dl)" || missing=1
if [ "${DRY:-0}" != "1" ]; then
  need asciinema "apt install asciinema  (or: pipx install asciinema)" || missing=1
  need agg "cargo install --git https://github.com/asciinema/agg, or grab a release binary from https://github.com/asciinema/agg/releases" || missing=1
fi
[ "$missing" = "0" ] || { echo "install the tools above and re-run." >&2; exit 1; }

AZ_FIX="$ROOT/internal/providers/azure/testdata"
AWS_FIX="$ROOT/internal/providers/aws/testdata"
KC_FIX="$ROOT/internal/providers/kubeconfig/testdata"
GCP_FIX="$ROOT/internal/providers/gcp/testdata"

WORK="$(mktemp -d)"
trap 'chmod -R u+w "$WORK" 2>/dev/null || true; rm -rf "$WORK"' EXIT
BIN="$WORK/kuberoutectl"
mkdir -p "$WORK/bin" "$WORK/home"

echo "==> building kuberoutectl"
( cd "$ROOT" && go build -o "$BIN" ./cmd/kuberoutectl )
ln -sf "$BIN" "$WORK/bin/kuberoutectl"

echo "==> installing fake az/aws/gcloud/kubectl on PATH"
cat > "$WORK/bin/az" <<EOF
#!/usr/bin/env bash
case "\$*" in
  "account list --output json") cat "$AZ_FIX/account-list.json" ;;
  "account get-access-token --output json") cat "$AZ_FIX/access-token.json" ;;
  "aks list --subscription aaaaaaaa-0000-0000-0000-000000000001 --output json") cat "$AZ_FIX/aks-list-prod.json" ;;
  "aks list --subscription aaaaaaaa-0000-0000-0000-000000000002 --output json") cat "$AZ_FIX/aks-list-lab.json" ;;
  "aks get-credentials"*) echo "Merged cluster as current context." ;;
  *) echo "[]" ;;
esac
EOF

cat > "$WORK/bin/aws" <<EOF
#!/usr/bin/env bash
SSO="https://my-sso.awsapps.com/start"
case "\$*" in
  "configure list-profiles") printf 'default\nprod-sso\nlegacy-static\n' ;;
  "sts get-caller-identity --profile default --output json") exit 1 ;;
  "configure get sso_start_url --profile default") echo "\$SSO" ;;
  "sts get-caller-identity --profile legacy-static --output json") cat "$AWS_FIX/identity-static.json" ;;
  "configure get sso_start_url --profile legacy-static") exit 1 ;;
  "configure get region --profile legacy-static") echo "us-east-1" ;;
  "eks list-clusters --profile legacy-static --region us-east-1 --output json") echo '{"clusters":[]}' ;;
  "sts get-caller-identity --profile prod-sso --output json") cat "$AWS_FIX/identity-prod-sso.json" ;;
  "configure get sso_start_url --profile prod-sso") echo "\$SSO" ;;
  "configure get region --profile prod-sso") echo "eu-central-1" ;;
  "eks list-clusters --profile prod-sso --region eu-central-1 --output json") cat "$AWS_FIX/eks-list-prod.json" ;;
  "eks describe-cluster --profile prod-sso --region eu-central-1 --name eks-prod-frankfurt --output json") cat "$AWS_FIX/eks-describe-frankfurt.json" ;;
  "eks describe-cluster --profile prod-sso --region eu-central-1 --name eks-prod-ireland --output json") cat "$AWS_FIX/eks-describe-ireland.json" ;;
  *) exit 1 ;;
esac
EOF

cat > "$WORK/bin/kubectl" <<EOF
#!/usr/bin/env bash
case "\$*" in
  "config view --raw -o json") cat "$KC_FIX/config-view.json" ;;
  "config use-context "*) echo "Switched to context \"\${*##* }\"." ;;
  *) exit 1 ;;
esac
EOF

cat > "$WORK/bin/gcloud" <<EOF
#!/usr/bin/env bash
case "\$*" in
  "config list --format=json") cat "$GCP_FIX/config-list.json" ;;
  "auth list --format=json") cat "$GCP_FIX/auth-list.json" ;;
  "projects list --format=json") cat "$GCP_FIX/projects-list.json" ;;
  "container clusters list --project platform-prod-123 --format=json") cat "$GCP_FIX/clusters-list-prod.json" ;;
  "container clusters list --project platform-lab-456 --format=json") cat "$GCP_FIX/clusters-list-lab.json" ;;
  "container clusters get-credentials"*) echo "Fetching cluster endpoint and auth data." ;;
  *) exit 1 ;;
esac
EOF
chmod +x "$WORK/bin/az" "$WORK/bin/aws" "$WORK/bin/kubectl" "$WORK/bin/gcloud"

# Isolate the environment: throwaway HOME, fake bins FIRST on PATH.
export HOME="$WORK/home"
export PATH="$WORK/bin:$PATH"

# Safety guard (do not remove): every provider CLI the demo touches MUST resolve
# to the fake bin dir. If any resolves to a real system CLI, abort rather than
# risk recording a real account's output into a committed, public GIF.
for c in kuberoutectl az aws gcloud kubectl; do
  resolved="$(command -v "$c" || true)"
  case "$resolved" in
    "$WORK/bin/"*) ;;
    *) echo "REFUSING: '$c' resolves to '${resolved:-<not found>}', not the fake bin dir ($WORK/bin)." >&2
       echo "Aborting so the recording cannot capture real-account output." >&2
       exit 1 ;;
  esac
done

# The demo flow. This is the readable source of truth for what the GIF shows.
cat > "$WORK/driver.sh" <<'DRV'
#!/usr/bin/env bash
prompt=$'\033[38;5;114m❯\033[0m '
type_cmd() {
  printf '%s' "$prompt"
  local c="$1" i
  for ((i = 0; i < ${#c}; i++)); do printf '%s' "${c:i:1}"; sleep 0.018; done
  printf '\n'
  eval "$c" || true
}
beat() { sleep "${1:-1.1}"; }

clear
beat 0.4
type_cmd "kuberoutectl doctor";                                   beat
type_cmd "kuberoutectl sync azure";                               beat 0.8
type_cmd "kuberoutectl sync aws";                                 beat 0.8
type_cmd "kuberoutectl sync gcp";                                 beat 0.8
type_cmd "kuberoutectl sync kubeconfig";                          beat
type_cmd "kuberoutectl target list";                              beat 1.6
type_cmd "kuberoutectl credential list --provider aws";           beat 1.6
type_cmd "kuberoutectl target label add aks-prod-weu env=prod";   beat
type_cmd "kuberoutectl collection create prod --selector env=prod"; beat 1.2
type_cmd "kuberoutectl target use aks-prod-weu";                  beat
type_cmd "kuberoutectl current";                                  beat 1.8
DRV
chmod +x "$WORK/driver.sh"

if [ "${DRY:-0}" = "1" ]; then
  echo "==> DRY run (no recording)"
  bash "$WORK/driver.sh"
  exit 0
fi

echo "==> recording with asciinema"
CAST="$WORK/demo.cast"
asciinema rec --overwrite --cols 116 --rows 32 -c "bash $WORK/driver.sh" "$CAST"

echo "==> rendering GIF with agg"
mkdir -p "$ROOT/assets"
agg \
  --theme monokai \
  --font-size 16 \
  --line-height 1.3 \
  --speed 1.0 \
  --idle-time-limit 1 \
  --last-frame-duration 3 \
  "$CAST" "$ROOT/assets/demo.gif"

# Second copy for the docs site: GitHub Pages serves from docs/, so the root
# assets/ copy (used by README.md) isn't visible to the site. Keep them in sync
# by emitting both from this one recording.
mkdir -p "$ROOT/docs/assets"
cp "$ROOT/assets/demo.gif" "$ROOT/docs/assets/demo.gif"

echo "==> wrote $ROOT/assets/demo.gif and docs/assets/demo.gif ($(stat -c%s "$ROOT/assets/demo.gif") bytes)"
