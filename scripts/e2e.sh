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
KC_FIX="$ROOT/internal/providers/kubeconfig/testdata"
GCP_FIX="$ROOT/internal/providers/gcp/testdata"

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

run() { echo; echo "\$ kuberoutectl $*"; "$BIN" "$@"; }

run doctor
run sync azure
run sync aws
run sync kubeconfig
run sync gcp

echo; echo "==> kubeconfig: unique contexts inventoried; a context duplicating a native EKS cluster (same endpoint) is suppressed"
kc="$("$BIN" target list --provider kubeconfig)"; echo "$kc"
assert_contains "$kc" "homelab"          # a self-hosted context, unique endpoint — survives
assert_contains "$kc" "static"           # homelab client-cert user
# The prod-eks context shares the Frankfurt EKS endpoint with the natively-synced
# aws target (sync aws ran first), so the richer native target wins and the
# kubeconfig duplicate is dropped from inventory.
echo "$kc" | grep -qF "prod-eks" && fail "kubeconfig context duplicating a native EKS cluster must be suppressed"
assert_contains "$("$BIN" target list --provider aws)" "eks-prod-frankfurt"  # native target is the single survivor
# The exec-based user's credential is not suppressed (only its duplicate target
# is) and stays honest: unknown health, never renew.
kc_creds="$("$BIN" credential list --provider kubeconfig)"; echo "$kc_creds"
assert_contains "$kc_creds" "unknown"    # exec-based user (externally managed)
echo "$kc_creds" | grep -qF "renew" && fail "kubeconfig credentials must never suggest renew"
use_kc="$("$BIN" target use homelab 2>&1)"; echo "$use_kc"
assert_contains "$use_kc" "kubeconfig updated"   # kubectl config use-context ran

echo; echo "==> GCP: projects become scopes, GKE clusters become targets"
gcp="$("$BIN" target list --provider gcp)"; echo "$gcp"
assert_contains "$gcp" "gke-prod-euw1"          # regional GKE cluster
assert_contains "$gcp" "europe-west4-a"         # zonal location surfaces as region
use_gcp="$("$BIN" target use gke-lab-euw4 2>&1)"; echo "$use_gcp"
assert_contains "$use_gcp" "kubeconfig updated" # gcloud container clusters get-credentials ran

creds="$("$BIN" credential list)"; echo; echo "$creds"
assert_contains "$creds" "static   none"     # AWS static keys not coerced into renew
assert_contains "$creds" "expired  renew"    # expired SSO session
assert_contains "$creds" "valid    use"      # working identities

echo; echo "==> credential list --provider filters"
az_creds="$("$BIN" credential list --provider azure)"; echo "$az_creds"
assert_contains "$az_creds" "azure"
echo "$az_creds" | grep -qF "aws:" && fail "--provider azure must exclude AWS credentials"

targets="$("$BIN" target list)"; echo; echo "$targets"
assert_contains "$targets" "ALIAS"           # short-handle column, not the giant ID
assert_contains "$targets" "aks-prod-weu"    # Azure AKS (alias == name here)
assert_contains "$targets" "eks-prod-frankfurt"  # AWS EKS
echo "$targets" | grep -qF "$AKS_WEU" && fail "full ID should not appear in default list"

echo; echo "==> filter by provider"
aws_only="$("$BIN" target list --provider aws)"; echo "$aws_only"
assert_contains "$aws_only" "eks-prod-frankfurt"
echo "$aws_only" | grep -qF "aks-prod-weu" && fail "--provider aws must exclude Azure targets"

echo; echo "==> --wide shows the full ID"
assert_contains "$("$BIN" target list --wide)" "$AKS_WEU"

echo; echo "==> label across providers and collect"
run target label add "$AKS_WEU" env=prod
run target label add "$EKS_FRA" env=prod
run collection create production --selector env=prod
show="$("$BIN" collection show production)"; echo; echo "$show"
assert_contains "$show" "Members: 2"

echo; echo "==> target use by short alias fetches credentials into kubeconfig (default)"
use_out="$("$BIN" target use aks-prod-weu 2>&1)"; echo "$use_out"
assert_contains "$use_out" "kubeconfig updated"
assert_contains "$use_out" "aks-prod-weu"
noku_out="$("$BIN" target use "$AKS_WEU" --no-kubeconfig 2>&1)"; echo "$noku_out"
assert_contains "$noku_out" "kubeconfig unchanged"

echo; echo "==> current answers 'what am I pointed at?'"
cur="$("$BIN" current)"; echo "$cur"
assert_contains "$cur" "aks-prod-weu"        # the target just used
assert_contains "$cur" "Last sync"           # cache freshness shown

echo; echo "==> resync both providers (user labels must survive)"
"$BIN" sync azure >/dev/null
"$BIN" sync aws >/dev/null
show2="$("$BIN" collection show production)"
assert_contains "$show2" "Members: 2"
assert_contains "$("$BIN" target inspect "$EKS_FRA")" "user-label    env=prod"

echo; echo "==> inspect reports the Kubernetes server version (unknown for kubeconfig, which has no source)"
eks_inspect="$("$BIN" target inspect "$EKS_FRA")"; echo "$eks_inspect"
echo "$eks_inspect" | grep -Eq '^Version[[:space:]]+1\.29$' || fail "EKS inspect Version should be 1.29 (from discovery, normalized)"
kc_inspect="$("$BIN" target inspect homelab)"; echo "$kc_inspect"
echo "$kc_inspect" | grep -Eq '^Version[[:space:]]+unknown$' || fail "kubeconfig inspect Version should be unknown"

echo; echo "==> consolidated command surface: inventory group, setup, and the clusters alias"
assert_contains "$("$BIN" inventory sources)"    "PROVIDER"     # was: source list
assert_contains "$("$BIN" inventory scopes)"     "KIND"         # was: scope list
assert_contains "$("$BIN" inventory providers)"  "azure"        # was: provider list
assert_contains "$("$BIN" clusters list)"        "aks-prod-weu" # `clusters` is an alias of `target`
assert_contains "$("$BIN" setup aws-sso --help)" "sso-session"  # was: aws sso populate
for gone in "provider list" "source list" "scope list" "aws sso populate"; do
  if "$BIN" $gone >/dev/null 2>&1; then fail "removed command still works: kuberoutectl $gone"; fi
done

echo; echo "==> target hide is persistent: dropped from the default list, kept across a resync, revealed by --all"
"$BIN" target hide eks-prod-frankfurt >/dev/null
echo "$("$BIN" target list --provider aws)" | grep -qF "eks-prod-frankfurt" && fail "hidden target must be absent from the default list"
assert_contains "$("$BIN" target list --provider aws --all)" "eks-prod-frankfurt"   # --all reveals it
assert_contains "$("$BIN" target list -l hidden=true)" "eks-prod-frankfurt"          # isolate hidden ones
"$BIN" sync aws >/dev/null                                                            # a resync rediscovers the cluster
echo "$("$BIN" target list --provider aws)" | grep -qF "eks-prod-frankfurt" && fail "hide must survive a resync (user-owned state)"
"$BIN" target unhide eks-prod-frankfurt >/dev/null
assert_contains "$("$BIN" target list --provider aws)" "eks-prod-frankfurt"           # unhide restores it

echo; echo "==> target delete is ephemeral: removed from the cache, restored by a resync"
assert_contains "$("$BIN" target list --provider aws)" "eks-prod-frankfurt"
del="$("$BIN" target delete eks-prod-frankfurt 2>&1)"; echo "$del"
assert_contains "$del" "Deleted target:"
echo "$("$BIN" target list --provider aws)" | grep -qF "eks-prod-frankfurt" && fail "deleted target must be gone from the list"
"$BIN" sync aws >/dev/null
assert_contains "$("$BIN" target list --provider aws)" "eks-prod-frankfurt"   # resync repopulates

echo; echo "==> target clear wipes targets only; credentials survive; --yes skips the prompt"
cleared="$("$BIN" target clear --yes 2>&1)"; echo "$cleared"
assert_contains "$cleared" "Cleared"
assert_contains "$("$BIN" target list)" "No targets"          # every target gone
assert_contains "$("$BIN" credential list)" "static"          # credentials untouched by clear

echo
echo "E2E OK: cross-provider discovery, health spectrum, and label survival verified."
