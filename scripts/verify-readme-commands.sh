#!/usr/bin/env bash
#
# Drift guard: every `kuberoutectl …` command and long flag shown in README.md
# and driven by scripts/demo.sh must still exist in the CLI. Prevents the README
# example block and the demo GIF from silently going stale after a command rename.
#
#   scripts/verify-readme-commands.sh      # builds the CLI, then checks
#
# Per referenced command it takes the leading run of subcommand-like tokens as the
# command path (stopping at the first placeholder / flag / value), asserts
# `kuberoutectl <path> --help` exits 0, and asserts every long flag on that line
# appears in that help text.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT
BIN="$WORK/kuberoutectl"

echo "==> building kuberoutectl"
( cd "$ROOT" && go build -o "$BIN" ./cmd/kuberoutectl )

fail=0
declare -A seen

check() {
  # $* is a raw command line beginning with "kuberoutectl"
  read -ra toks <<< "$*"
  [ "${toks[0]:-}" = "kuberoutectl" ] || return 0

  local path=() flags=() t
  for t in "${toks[@]:1}"; do
    [[ "$t" =~ ^[a-z][a-z-]*$ ]] || break
    path+=("$t")
  done
  [ ${#path[@]} -gt 0 ] || return 0
  for t in "${toks[@]:1}"; do
    [[ "$t" =~ ^--[a-z][a-z-]*$ ]] && flags+=("$t")
  done

  local key="${path[*]} :: ${flags[*]:-}"
  [[ -n "${seen[$key]:-}" ]] && return 0
  seen[$key]=1

  local help
  if ! help="$("$BIN" "${path[@]}" --help 2>&1)"; then
    echo "DRIFT: 'kuberoutectl ${path[*]}' no longer exists" >&2
    fail=1
    return 0
  fi
  local f
  for f in "${flags[@]:-}"; do
    [ -n "$f" ] || continue
    grep -q -- "$f" <<< "$help" || { echo "DRIFT: flag '$f' not on 'kuberoutectl ${path[*]}'" >&2; fail=1; }
  done
  echo "ok: kuberoutectl ${path[*]} ${flags[*]:-}"
}

# 1. README.md — command lines inside fenced code blocks.
while IFS= read -r line; do
  line="${line#"${line%%[![:space:]]*}"}"
  check "$line"
done < <(awk '/^[[:space:]]*```/ { inf = !inf; next } inf' "$ROOT/README.md" | grep -E '^[[:space:]]*kuberoutectl ')

# 2. scripts/demo.sh — the recorded flow: type_cmd "kuberoutectl …".
while IFS= read -r cmd; do
  check "$cmd"
done < <(grep -oE 'type_cmd "kuberoutectl [^"]+"' "$ROOT/scripts/demo.sh" | sed -E 's/^type_cmd "//; s/"$//')

if [ "$fail" -ne 0 ]; then
  echo "" >&2
  echo "Command drift detected — update README.md / scripts/demo.sh to match the CLI." >&2
  exit 1
fi
echo ""
echo "All README + demo commands and flags exist in the current CLI."
