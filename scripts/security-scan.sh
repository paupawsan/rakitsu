#!/bin/sh
# Scan for private fingerprints and leaked credentials. Run before any
# public release sync. Exits non-zero on any leak.
#
# Generic patterns (credential prefixes, tracked credential files) are
# checked unconditionally. Project-specific private strings (hostnames,
# internal IPs, PC names) are loaded from .security-patterns.local if
# present — that file is gitignored so its contents never ship publicly.
#
# Add project-specific patterns to .security-patterns.local, one per line.
# Example:
#   my-server-hostname
#   10\.0\.0\.1
#   my-pc-name
#
# Scope: by default this scans the whole working tree. Set SCAN_CACHED=1 to
# scan the git INDEX instead (git grep --cached) -- useful in CI or a
# pre-commit hook, where you only want to check what's actually staged.

set -eu

ROOT="$(git rev-parse --show-toplevel)"
cd "$ROOT"

EXCLUDE_PATHS=':!web/package-lock.json :!web/node_modules :!.git :!scripts/security-scan.sh :!.security-patterns.local'
GREP_SCOPE=""
[ "${SCAN_CACHED:-0}" = "1" ] && GREP_SCOPE="--cached"

fail=0

# --- 1. Credential prefixes (always checked) ---------------------------

echo "== credential prefixes =="
for p in '\bsk-[A-Za-z0-9]{10,}' '\bnvapi-[A-Za-z0-9]+' '\bAKIA[A-Z0-9]{16}' '\bghp_[A-Za-z0-9]+' '\bglpat-[A-Za-z0-9_-]+'; do
  matches=$(git grep $GREP_SCOPE -l -E "$p" -- $EXCLUDE_PATHS 2>/dev/null || true)
  if [ -n "$matches" ]; then
    for f in $matches; do
      # Filter out obvious placeholders (sk-..., sk-ant-..., YOUR_KEY, <your-key>, etc.)
      if git grep $GREP_SCOPE -E "$p" -- "$f" | grep -vE '\.\.\.|placeholder|example|YOUR_|<.*>' >/dev/null 2>&1; then
        echo "LEAK: credential pattern found in $f"
        fail=1
      fi
    done
  fi
done

# --- 2. Tracked credential files (always checked) ---------------------

echo "== tracked credential files =="
bad_files=$(git ls-files | grep -E '\.env$|\.mcp\.json$|gemini-keys/|(^|/)api-key$' || true)
if [ -n "$bad_files" ]; then
  echo "LEAK: credential files tracked in git:"
  echo "$bad_files" | sed 's/^/  /'
  fail=1
fi

# --- 3. Project-specific patterns (from local file, if present) -------

if [ -f .security-patterns.local ]; then
  echo "== project-specific patterns =="
  while IFS= read -r pattern; do
    [ -z "$pattern" ] && continue
    case "$pattern" in '#'*) continue ;; esac
    matches=$(git grep $GREP_SCOPE -l -E "$pattern" -- $EXCLUDE_PATHS 2>/dev/null || true)
    if [ -n "$matches" ]; then
      echo "LEAK: private pattern matched in:"
      echo "$matches" | sed 's/^/  /'
      fail=1
    fi
  done < .security-patterns.local
else
  echo "== project-specific patterns == (no .security-patterns.local found — generic scan only)"
fi

if [ $fail -eq 0 ]; then
  echo ""
  echo "PASS — no leaks detected"
else
  echo ""
  echo "FAIL — fix leaks above before syncing to public"
  exit 1
fi
