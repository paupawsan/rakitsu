#!/usr/bin/env python3
"""Parse the Reviewer agent's raw output (either the literal text
NO FINDINGS, or one or more '### FINDING ... ### END' blocks, per the
template in scripts/review-config-openai.yaml's system_prompt) into the
JSON review-result shape scripts/post-review.py posts to GitHub:

  {"body": "<preamble + short summary line>",
   "comments": [{"path": ..., "line": ..., "body": ...}, ...]}

Never trusts the model's output to be well-formed: anything that doesn't
match the expected block shape falls back to putting the whole raw text
in "body" with an empty "comments" list, so a parsing miss degrades to
"a real review body, no inline comments" rather than losing content.

Usage: parse-findings.py <preamble-file> <findings-file>
"""
import json
import re
import sys

FINDING_RE = re.compile(
    r"### FINDING\s*\n"
    r"FILE:\s*(?P<file>.+?)\s*\n"
    r"LINE:\s*(?P<line>\d+)\s*\n"
    r"SEVERITY:\s*(?P<severity>CRITICAL|MAJOR|MINOR)\s*\n"
    r"BODY:\s*(?P<body>.*?)\n"
    r"### END",
    re.DOTALL,
)


def main() -> int:
    preamble_path, findings_path = sys.argv[1], sys.argv[2]
    preamble = open(preamble_path, encoding="utf-8").read().rstrip("\n")
    raw = open(findings_path, encoding="utf-8").read()
    text = raw.strip()

    if text == "NO FINDINGS":
        result = {"body": preamble + "\n\nNo issues found in this diff.", "comments": []}
        json.dump(result, sys.stdout)
        return 0

    matches = list(FINDING_RE.finditer(raw))
    if not matches:
        # Didn't match the expected template at all — degrade to a plain
        # body so the content still reaches the PR, just without inline
        # anchoring.
        result = {"body": preamble + "\n\n" + raw.strip(), "comments": []}
        json.dump(result, sys.stdout)
        return 0

    comments = [
        {
            "path": m.group("file").strip(),
            "line": int(m.group("line")),
            "body": f"**{m.group('severity')}**: {m.group('body').strip()}",
        }
        for m in matches
    ]

    plural = "" if len(comments) == 1 else "s"
    summary = f"Found {len(comments)} issue{plural} — see inline comment{plural} below."
    result = {"body": preamble + "\n\n" + summary, "comments": comments}
    json.dump(result, sys.stdout)
    return 0


if __name__ == "__main__":
    sys.exit(main())
