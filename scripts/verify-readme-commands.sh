#!/usr/bin/env bash
#
# Drift guard: every `kuberoutectl …` command and long flag shown in README.md
# and driven by scripts/demo.sh must still exist in the CLI. Prevents the README
# example block and the demo GIF from silently going stale after a command rename.
#
#   scripts/verify-readme-commands.sh      # builds the CLI, then checks
#   make verify-readme
#
# Soundness note: Cobra's `--help` ALWAYS exits 0 and falls back to the nearest
# resolvable ancestor, so an exit-code check cannot detect a renamed subcommand
# (`kuberoutectl sync bogus --help` prints `sync`'s help and exits 0). Instead we
# walk the real command tree: for each referenced command we descend only through
# tokens that appear in the parent's "Available Commands" list. A token that
# should be a subcommand but isn't (a rename/removal) is flagged; a positional
# value after a leaf command is correctly left alone.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT
BIN="$WORK/kuberoutectl"

echo "==> building kuberoutectl"
( cd "$ROOT" && go build -o "$BIN" ./cmd/kuberoutectl )

fail=0
declare -A seen

# Names under "Available Commands:" of `kuberoutectl <path>` — empty if it's a
# leaf (takes args, not subcommands).
avail_subcommands() {
  "$BIN" "$@" --help 2>/dev/null | awk '
    /^Available Commands:/ { f = 1; next }
    f && /^[[:space:]]*$/  { f = 0 }
    f && /^[[:space:]]+[a-zA-Z]/ { print $1 }
  '
}

check() {
  read -ra toks <<< "$*"
  [ "${toks[0]:-}" = kuberoutectl ] || return 0

  # Candidate command tokens = leading run of subcommand-like words; long flags
  # collected separately from anywhere on the line.
  local -a cand=() flags=()
  local t
  for t in "${toks[@]:1}"; do
    [[ "$t" =~ ^[a-z][a-z-]*$ ]] || break
    cand+=("$t")
  done
  for t in "${toks[@]:1}"; do
    [[ "$t" =~ ^--[a-z][a-z-]*$ ]] && flags+=("$t")
  done
  [ ${#cand[@]} -gt 0 ] || return 0

  # `clusters`/`cluster` are aliases of `target` (not listed under a parent's
  # Available Commands), so normalize them to walk the canonical tree.
  case "${cand[0]}" in clusters | cluster) cand[0]=target ;; esac

  # Descend the real command tree.
  local -a path=()
  local subs
  for t in "${cand[@]}"; do
    subs="$(avail_subcommands "${path[@]}")"
    [ -n "$subs" ] || break                       # leaf: remaining tokens are args
    if grep -qxF "$t" <<< "$subs"; then
      path+=("$t")
    else
      echo "DRIFT: '$t' is not a subcommand of 'kuberoutectl ${path[*]}' (renamed/removed?)" >&2
      fail=1
      return 0
    fi
  done
  [ ${#path[@]} -gt 0 ] || return 0

  local key="${path[*]} :: ${flags[*]:-}"
  [[ -n "${seen[$key]:-}" ]] && return 0
  seen[$key]=1

  local help f
  help="$("$BIN" "${path[@]}" --help 2>&1)"
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
