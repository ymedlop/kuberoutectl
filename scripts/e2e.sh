#!/usr/bin/env bash
#
# End-to-end smoke test for kuberoutectl with no cloud access.
#
# It builds the CLI and puts fake `az` and `aws` executables on PATH that serve
# the committed provider fixtures (internal/providers/*/testdata). It then runs
# a representative operator flow — discover both clouds, inspect the health
# spectrum, label across providers, build a collection, and resync — asserting
# the key guarantees along the way. Everything runs in a throwaway HOME so it
# never touches your real ~/.kuberoutectl.
#
# Usage:  scripts/e2e.sh          # run the flow, print output, assert
#         KEEP=1 scripts/e2e.sh   # keep the temp workdir for inspection
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
AZ_FIX="$ROOT/internal/providers/azure/testdata"
AWS_FIX="$ROOT/internal/providers/aws/testdata"

WORK="$(mktemp -d)"
if [ "${KEEP:-0}" = "1" ]; then
  echo "workdir: $WORK (kept)"
else
  # chmod first: a read-only file cannot always be removed by rm alone.
  trap 'chmod -R u+w "$WORK" 2>/dev/null || true; rm -rf "$WORK"' EXIT
fi

BIN="$WORK/kuberoutectl"
mkdir -p "$WORK/bin"

# Stable IDs from the fixtures.
AKS_WEU="/subscriptions/aaaaaaaa-0000-0000-0000-000000000001/resourcegroups/rg-platform/providers/Microsoft.ContainerService/managedClusters/aks-prod-weu"
EKS_FRA="arn:aws:eks:eu-central-1:111111111111:cluster/eks-prod-frankfurt"

fail() { echo "ASSERTION FAILED: $1" >&2; exit 1; }
assert_contains() { echo "$1" | grep -qF "$2" || fail "expected to find '$2'"; }

# Build with the real HOME so the Go module cache is reused. Only the CLI runs
# below get an isolated HOME, so discovery writes to a throwaway ~/.kuberoutectl
# rather than your real one (and cleanup never touches the module cache).
echo "==> building kuberoutectl"
( cd "$ROOT" && go build -o "$BIN" ./cmd/kuberoutectl )

export HOME="$WORK/home"
export PATH="$WORK/bin:$PATH"
mkdir -p "$HOME"

echo "==> installing fake az and aws on PATH"
cat > "$WORK/bin/az" <<EOF
#!/usr/bin/env bash
case "\$*" in
  "account list --output json") cat "$AZ_FIX/account-list.json" ;;
  "account get-access-token --output json") cat "$AZ_FIX/access-token.json" ;;
  "aks list --subscription aaaaaaaa-0000-0000-0000-000000000001 --output json") cat "$AZ_FIX/aks-list-prod.json" ;;
  "aks list --subscription aaaaaaaa-0000-0000-0000-000000000002 --output json") cat "$AZ_FIX/aks-list-lab.json" ;;
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
chmod +x "$WORK/bin/az" "$WORK/bin/aws"

run() { echo; echo "\$ kuberoutectl $*"; "$BIN" "$@"; }

run doctor
run sync azure
run sync aws

creds="$("$BIN" credential list)"; echo; echo "$creds"
assert_contains "$creds" "static   none"     # AWS static keys not coerced into renew
assert_contains "$creds" "expired  renew"    # expired SSO session
assert_contains "$creds" "valid    use"      # working identities

targets="$("$BIN" target list)"; echo; echo "$targets"
assert_contains "$targets" "aks-prod-weu"    # Azure AKS
assert_contains "$targets" "eks-prod-frankfurt"  # AWS EKS

echo; echo "==> label across providers and collect"
run target label add "$AKS_WEU" env=prod
run target label add "$EKS_FRA" env=prod
run collection create production --selector env=prod
show="$("$BIN" collection show production)"; echo; echo "$show"
assert_contains "$show" "Members: 2"

echo; echo "==> resync both providers (user labels must survive)"
"$BIN" sync azure >/dev/null
"$BIN" sync aws >/dev/null
show2="$("$BIN" collection show production)"
assert_contains "$show2" "Members: 2"
assert_contains "$("$BIN" target inspect "$EKS_FRA")" "user-label    env=prod"

echo
echo "E2E OK: cross-provider discovery, health spectrum, and label survival verified."
