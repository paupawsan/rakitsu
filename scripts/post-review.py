#!/usr/bin/env python3
"""Post a review built from scripts/ci-review.sh's JSON output to a PR,
attaching one inline comment per finding where possible.

GitHub's Reviews API rejects the WHOLE request if even one inline
comment's file/line doesn't match a line that's actually part of the
diff (a real risk since the model can misjudge a line number) — so on
that failure this retries once as a body-only review, folding every
finding into the body as a fallback rather than silently dropping them.

Usage: post-review.py <result-json> <repo> <pr-number> <commit-sha>
"""
import json
import subprocess
import sys


def post(repo: str, pr_number: str, payload: dict) -> subprocess.CompletedProcess:
    return subprocess.run(
        ["gh", "api", "--method", "POST", f"repos/{repo}/pulls/{pr_number}/reviews", "--input", "-"],
        input=json.dumps(payload),
        text=True,
        capture_output=True,
    )


def main() -> int:
    result_path, repo, pr_number, commit_sha = sys.argv[1:5]
    result = json.load(open(result_path, encoding="utf-8"))
    comments = result.get("comments", [])

    payload = {
        "commit_id": commit_sha,
        "event": "COMMENT",
        "body": result["body"],
        "comments": [{"path": c["path"], "line": c["line"], "body": c["body"]} for c in comments],
    }
    proc = post(repo, pr_number, payload)

    if proc.returncode != 0 and comments:
        sys.stderr.write(
            "Inline comment post failed (likely a file/line the model named "
            "that isn't actually part of the diff) — retrying as a "
            f"body-only review:\n{proc.stderr}\n"
        )
        fallback_body = result["body"]
        for c in comments:
            fallback_body += f"\n\n**{c['path']}:{c['line']}** — {c['body']}"
        proc = post(repo, pr_number, {"commit_id": commit_sha, "event": "COMMENT", "body": fallback_body})

    sys.stdout.write(proc.stdout)
    if proc.returncode != 0:
        sys.stderr.write(proc.stderr)
        return proc.returncode
    return 0


if __name__ == "__main__":
    sys.exit(main())
