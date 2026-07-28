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
  "configure list-profiles") printf 'default\nops\nprod-sso\nlegacy-static\n' ;;
  "sts get-caller-identity --profile default --output json") echo "Error loading SSO Token: Token for default does not exist" >&2; exit 1 ;;
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
  # ops: a second SSO profile into the SAME account. It sees frankfurt (so the
  # two profiles' targets share an ARN and must fold into one), but IAM denies
  # it describe on ireland — the no-pattern case this feature exists for.
  "sts get-caller-identity --profile ops --output json") cat "$AWS_FIX/identity-ops.json" ;;
  "configure get sso_start_url --profile ops") echo "\$SSO" ;;
  "configure get region --profile ops") echo "eu-central-1" ;;
  "eks list-clusters --profile ops --region eu-central-1 --output json") cat "$AWS_FIX/eks-list-prod.json" ;;
  "eks describe-cluster --profile ops --region eu-central-1 --name eks-prod-frankfurt --output json") cat "$AWS_FIX/eks-describe-frankfurt.json" ;;
  "eks describe-cluster --profile ops --region eu-central-1 --name eks-prod-ireland --output json") exit 1 ;;
  # Frankfurt is in API authentication mode, so its access-entry list is
  # authoritative in both directions. The list is asked for once for the whole
  # cluster (through the group's first profile, ops) and spans two pages: ops'
  # own entry is on page 2, so a check that ignored nextToken would report a real
  # entry as absent — which under API mode reads as a confirmed refusal.
  # prod-sso appears on neither page.
  "eks list-access-entries --cluster-name eks-prod-frankfurt --profile ops --region eu-central-1 --output json") cat "$AWS_FIX/access-entries-page1.json" ;;
  "eks list-access-entries --cluster-name eks-prod-frankfurt --profile ops --region eu-central-1 --output json --starting-token eyJwYWdlIjogMn0=") cat "$AWS_FIX/access-entries-page2.json" ;;
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

echo; echo "==> aws: an expired-token profile is surfaced on sync (diagnostic), not silently dropped"
sync_aws_diag="$("$BIN" sync aws 2>&1)"; echo "$sync_aws_diag"
assert_contains "$sync_aws_diag" 'profile "default": identity check failed'  # expired SSO named, not swallowed
assert_contains "$sync_aws_diag" 'aws sso login --profile default'           # actionable remedy

echo; echo "==> aws --verbose: the raw cloud-CLI command and its stderr are traced"
sync_aws_verbose="$("$BIN" sync aws --verbose 2>&1)"; echo "$sync_aws_verbose"
assert_contains "$sync_aws_verbose" "[exec] "                                                    # trace format present
assert_contains "$sync_aws_verbose" "sts get-caller-identity --profile default --output json"    # raw command traced
assert_contains "$sync_aws_verbose" "Error loading SSO Token"                                    # underlying CLI stderr shown

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

echo; echo "==> two AWS profiles into one account fold into a single target"
# eks-prod-frankfurt is visible to both prod-sso and ops. Before the fold this
# produced two targets sharing an ARN, indistinguishable in the listing and with
# the second unreachable by any printed reference.
aws_rows="$("$BIN" target list --provider aws)"; echo "$aws_rows"
fra_count="$(echo "$aws_rows" | grep -c "eks-prod-frankfurt")"
[ "$fra_count" -eq 1 ] || fail "expected exactly 1 frankfurt row, got $fra_count"
assert_contains "$aws_rows" "PROFILES"
assert_contains "$aws_rows" "ops"

echo; echo "==> a cluster only one profile can describe records only that profile"
# ops is denied describe on ireland, so ireland must not offer it as a way in.
ireland="$("$BIN" target inspect eks-prod-ireland)"; echo "$ireland"
echo "$ireland" | grep -qE '^profile[[:space:]]+ops' && fail "ireland must not list the denied profile"

echo; echo "==> sync reports which profile was denied on which cluster"
denials="$("$BIN" sync aws 2>&1)"; echo "$denials" | grep -F "cannot describe" || true
assert_contains "$denials" "eks-prod-ireland"

echo; echo "==> target use --profile picks the access path, and it is remembered"
default_use="$("$BIN" target use eks-prod-frankfurt --no-kubeconfig 2>&1)"; echo "$default_use"
assert_contains "$default_use" "default"          # an unprompted default says so
chosen="$("$BIN" target use eks-prod-frankfurt --profile prod-sso --no-kubeconfig 2>&1)"; echo "$chosen"
assert_contains "$chosen" "via prod-sso"
echo "$chosen" | grep -qF "default" && fail "an explicit choice must not read as a default"
assert_contains "$("$BIN" current)" "prod-sso"    # persisted, and reported
again="$("$BIN" target use eks-prod-frankfurt --no-kubeconfig 2>&1)"; echo "$again"
assert_contains "$again" "remembered"

echo; echo "==> access entries decide who can actually operate, not just authenticate"
# Both profiles can describe frankfurt — that is IAM. Only ops holds an EKS
# access entry, which is the Kubernetes-side layer that decides whether kubectl
# works at all. The OPERABLE column is the difference between the two.
assert_contains "$aws_rows" "OPERABLE"
fra_row="$(echo "$aws_rows" | grep 'eks-prod-frankfurt')"
echo "$fra_row"
assert_contains "$fra_row" "ops"
# Ireland was never checked (only one profile reaches it), and an unchecked row
# must read `unknown` rather than blank — a blank cell reads as "no", and "we
# did not look" is not "no".
ire_row="$(echo "$aws_rows" | grep 'eks-prod-ireland')"
echo "$ire_row"
assert_contains "$ire_row" "unknown"

fra_inspect="$("$BIN" target inspect eks-prod-frankfurt)"; echo "$fra_inspect"
assert_contains "$fra_inspect" "Access check"
assert_contains "$fra_inspect" "not operable"     # prod-sso, confirmed absent under API mode

echo; echo "==> choosing a profile the cluster refuses warns on stderr and still proceeds"
warn="$("$BIN" target use eks-prod-frankfurt --profile prod-sso --no-kubeconfig 2>&1 >/dev/null)"; echo "$warn"
assert_contains "$warn" "no access entry"
assert_contains "$warn" "ops"                     # names one that would work
stdout_only="$("$BIN" target use eks-prod-frankfurt --profile prod-sso --no-kubeconfig 2>/dev/null)"
assert_contains "$stdout_only" "Recorded selection"  # reports, does not block
echo "$stdout_only" | grep -qF "no access entry" && fail "the warning belongs on stderr, not stdout"

echo; echo "==> an unreachable profile is rejected, naming the ones that work"
bad="$("$BIN" target use eks-prod-frankfurt --profile nope --no-kubeconfig 2>&1)" && fail "expected failure"
echo "$bad"
assert_contains "$bad" "ops"

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

echo; echo "==> mcp: the stdio server answers a real initialize + tools/list handshake"
# Keep stdin open briefly (sleep) so the server flushes its responses before EOF.
# The full typed round-trip is covered by the Go in-memory test; this proves the
# shipped binary speaks MCP end to end.
mcp_out="$( { printf '%s\n' \
  '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"e2e","version":"0"}}}' \
  '{"jsonrpc":"2.0","method":"notifications/initialized"}' \
  '{"jsonrpc":"2.0","id":2,"method":"tools/list"}'; sleep 0.5; } | "$BIN" mcp 2>/dev/null )"
assert_contains "$mcp_out" '"list_targets"'                 # a read tool is registered
assert_contains "$mcp_out" '"create_or_update_collection"'  # a safe-write tool is registered
mcp_ro="$( { printf '%s\n' \
  '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"e2e","version":"0"}}}' \
  '{"jsonrpc":"2.0","method":"notifications/initialized"}' \
  '{"jsonrpc":"2.0","id":2,"method":"tools/list"}'; sleep 0.5; } | "$BIN" mcp --read-only 2>/dev/null )"
echo "$mcp_ro" | grep -qF '"create_or_update_collection"' && fail "--read-only must not expose write tools"
assert_contains "$mcp_ro" '"list_targets"'                  # read tools still present

echo
echo "E2E OK: cross-provider discovery, health spectrum, label survival, and MCP handshake verified."
