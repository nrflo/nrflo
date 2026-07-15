#!/usr/bin/env bash
# CLAUDE.md byte-cap gate (Mandatory Rule 1). Finds every CLAUDE.md in the repo,
# picks its cap by location, and prints PASS / WARN / OVER per file.
#
# Blocking semantics: a CLAUDE.md over cap blocks the commit (exit 1) ONLY when
# it is part of the current change set (staged/unstaged/untracked) — you must not
# commit a file you touched while it is over cap. A pre-existing overage in a file
# this work did not touch is reported as WARN and does not block (Rule 1 is
# enforced "in the same commit"). Run with --strict to make EVERY overage block.
#
# Caps (bytes), from the table in the root CLAUDE.md:
#   root CLAUDE.md ................ 10240   (10 KB)
#   be/CLAUDE.md, ui/CLAUDE.md .... 12288   (12 KB)
#   be/internal/spawner/CLAUDE.md . 15360   (15 KB, documented exception)
#   package CLAUDE.md ............. 12288   (12 KB)  — a dir holding Go source
#   sub-package / leaf CLAUDE.md ..  6144   ( 6 KB)  — nested (>=4 deep) doc-only
set -euo pipefail

SKILL_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO="$(cd "$SKILL_DIR/../../.." && pwd)"
cd "$REPO"

STRICT=0
[ "${1:-}" = "--strict" ] && STRICT=1

cap_for() {
  # $1 = repo-relative path to a CLAUDE.md
  local rel="$1" dir slashes
  case "$rel" in
    CLAUDE.md)                     echo 10240; return ;;  # repo root
    be/CLAUDE.md|ui/CLAUDE.md)     echo 12288; return ;;  # backend / frontend top-level
    be/internal/spawner/CLAUDE.md) echo 15360; return ;;  # documented spawner exception
  esac
  # A directory that holds Go source is a code package → package tier (12 KB).
  dir="$(dirname "$rel")"
  if find "$dir" -maxdepth 1 -name '*.go' 2>/dev/null | head -1 | grep -q .; then
    echo 12288; return
  fi
  # No Go source: nested doc (>=4 path segments) is a leaf (6 KB); a top-level
  # area doc (ui/src/<area>, manual_testing, …) gets the package tier.
  slashes="${rel//[^\/]/}"
  if [ "${#slashes}" -ge 4 ]; then echo 6144; else echo 12288; fi
}

# Current change set (new path for renames), one repo-relative path per line.
changed=""
if git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
  changed="$(git -c core.quotepath=false status --porcelain \
    | sed -e 's/^...//' -e 's/^.* -> //')"
else
  STRICT=1
fi

is_changed() {
  [ "$STRICT" -eq 1 ] && return 0
  printf '%s\n' "$changed" | grep -Fxq "$1"
}

fail=0
warned=0
while IFS= read -r f; do
  rel="${f#./}"
  size="$(wc -c < "$f" | tr -d ' ')"
  cap="$(cap_for "$rel")"
  if [ "$size" -le "$cap" ]; then
    printf 'PASS  %-46s %6s / %6s\n' "$rel" "$size" "$cap"
  elif is_changed "$rel"; then
    printf 'OVER  %-46s %6s / %6s  (+%s, in this change set)\n' "$rel" "$size" "$cap" "$((size - cap))"
    fail=1
  else
    printf 'WARN  %-46s %6s / %6s  (+%s, pre-existing — untouched here)\n' "$rel" "$size" "$cap" "$((size - cap))"
    warned=1
  fi
done < <(find . \
  \( -name node_modules -o -name .git -o -name dist -o -name build \) -prune -o \
  -name CLAUDE.md -print | sort)

if [ "$fail" -ne 0 ]; then
  echo "FAIL: a CLAUDE.md you changed is over cap — trim it (prefer deletion, Rule 1) before committing." >&2
  exit 1
fi
[ "$warned" -ne 0 ] && echo "NOTE: pre-existing CLAUDE.md overage(s) above were not introduced by this work; fix opportunistically."
echo "OK: every CLAUDE.md you changed is within cap."
