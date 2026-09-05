#!/usr/bin/env bash
# scripts/ci-review.sh — whole-diff PR review via rakitsu against direct
# OpenAI, for .github/workflows/ai-review.yml.
#
# Writes a JSON review result to <output-file>:
#   {"body": "<preamble + summary>", "comments": [{"path","line","body"}, ...]}
# scripts/post-review.py turns this into a GitHub PR review — one inline
# comment per finding, falling back to a body-only review if GitHub
# rejects an inline anchor. This script never posts anything itself, and
# nothing in the diff/findings text (untrusted PR content) is ever
# shell-interpolated — it only ever flows through files into
# scripts/parse-findings.py's own string/JSON handling.
#
# Usage: scripts/ci-review.sh <full|incremental> <base-ref> <head-ref> <output-file>
set -euo pipefail

MAX_DIFF_BYTES=200000  # matches scripts/review-pr-diff.sh's existing cap

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CONFIG="$SCRIPT_DIR/review-config-openai.yaml"
RAKITSU_BIN="${RAKITSU_BIN:-rakitsu}"

if [ $# -ne 4 ]; then
  echo "Usage: $0 <full|incremental> <base-ref> <head-ref> <output-file>" >&2
  exit 1
fi
MODE="$1"; BASE="$2"; HEAD="$3"; OUTPUT="$4"

case "$MODE" in
  full) SCOPE_TEXT="the full PR diff" ;;
  incremental) SCOPE_TEXT="the diff since the last commit" ;;
  *) echo "FATAL: mode must be 'full' or 'incremental', got: $MODE" >&2; exit 1 ;;
esac

git rev-parse --verify --quiet "$BASE" >/dev/null || { echo "FATAL: bad ref: $BASE" >&2; exit 1; }
git rev-parse --verify --quiet "$HEAD" >/dev/null || { echo "FATAL: bad ref: $HEAD" >&2; exit 1; }
: "${OPENAI_API_KEY:?OPENAI_API_KEY not set}"

if [ "$MODE" = "full" ]; then
  diff_content=$(git diff "$BASE...$HEAD")
else
  diff_content=$(git diff "$BASE" "$HEAD")
fi
diff_bytes=$(LC_ALL=C printf '%s' "$diff_content" | wc -c)

preamble() {
  cat <<EOF
## Automated review

This review was generated automatically by this repo's automated review
pipeline, not a human — it ran rakitsu against openai/gpt-5-nano over
$SCOPE_TEXT, reading file content and diffs as text only, no code
execution. Treat findings as a starting point to verify, not a final word.

---
EOF
}

# Body-only JSON result, no inline comments. $1 here is built only from
# trusted local text (SCOPE_TEXT/BASE/HEAD, which are validated git SHAs,
# never raw diff/PR content), so passing it through argv is safe.
write_body_only() {
  local msg
  msg="$(preamble)

$1"
  python3 -c '
import json, sys
json.dump({"body": sys.argv[1], "comments": []}, sys.stdout)
' "$msg"
}

if [ "$diff_bytes" -eq 0 ]; then
  write_body_only "No changes found between $BASE and $HEAD." > "$OUTPUT"
  exit 0
fi

if [ "$diff_bytes" -gt "$MAX_DIFF_BYTES" ]; then
  write_body_only "Diff is $diff_bytes bytes, over the $MAX_DIFF_BYTES-byte auto-review cap — skipped. Review this one by hand, or split it into smaller PRs." > "$OUTPUT"
  exit 0
fi

query="DIFF ($SCOPE_TEXT, base=$BASE, head=$HEAD):

$diff_content"

TMP_STDERR=$(mktemp)
FINDINGS_TXT=$(mktemp)
PREAMBLE_TXT=$(mktemp)
trap 'rm -f "$TMP_STDERR" "$FINDINGS_TXT" "$PREAMBLE_TXT"' EXIT

set +e
raw_output=$("$RAKITSU_BIN" run "$CONFIG" "$query" --no-hub 2>"$TMP_STDERR")
rc=$?
set -e

if [ "$rc" -ne 0 ]; then
  echo "FATAL: rakitsu run failed (exit $rc):" >&2
  cat "$TMP_STDERR" >&2
  exit 1
fi

# rakitsu run's stdout wraps the actual answer between a "Result (took...)"
# header and a closing ruler of only U+2501 (━) characters — everything
# else (config path, session ID, "Executing agent...") is CLI scaffolding
# that must not reach a public PR comment.
findings=$(printf '%s\n' "$raw_output" | awk '
  /^Result \(took/ { in_result=1; next }
  in_result && /^━+$/ { if (started==1) exit; started=1; next }
  in_result && started==1 { print }
')

if [ -z "$findings" ]; then
  echo "FATAL: could not extract Result content from rakitsu output:" >&2
  echo "$raw_output" >&2
  exit 1
fi

preamble > "$PREAMBLE_TXT"
printf '%s\n' "$findings" > "$FINDINGS_TXT"
python3 "$SCRIPT_DIR/parse-findings.py" "$PREAMBLE_TXT" "$FINDINGS_TXT" > "$OUTPUT"
