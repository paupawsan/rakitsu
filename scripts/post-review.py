#!/usr/bin/env python3
"""Post a review built from scripts/ci-review.sh's JSON output to a PR,
attaching one inline comment per finding where possible.

GitHub's Reviews API rejects the WHOLE request if even one inline
comment's file/line doesn't match a line that's actually part of the
diff (a real risk since the model can misjudge a line number) — so on
that specific failure (HTTP 422) this retries once as a body-only
review, folding every finding into the body as a fallback rather than
silently dropping them. Any OTHER failure (bad token, rate limit, a
5xx) is left as a failure rather than blindly retried, since a retry
there wouldn't fix anything and could risk a duplicate post if the
original request actually landed server-side despite a network error.

Usage: post-review.py <result-json> <repo> <pr-number> <commit-sha>
"""
import json
import subprocess
import sys

# GitHub's review body cap; the fallback below can inline every finding
# into one body, which a large finding set could exceed.
MAX_BODY_CHARS = 60000


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

    if proc.returncode != 0 and comments and "422" in proc.stderr:
        sys.stderr.write(
            "Inline comment post rejected (HTTP 422 — likely a file/line "
            f"the model named that isn't actually part of the diff): "
            f"retrying as a body-only review:\n{proc.stderr}\n"
        )
        fallback_body = result["body"]
        for c in comments:
            fallback_body += f"\n\n**{c['path']}:{c['line']}** — {c['body']}"
        if len(fallback_body) > MAX_BODY_CHARS:
            fallback_body = fallback_body[:MAX_BODY_CHARS] + "\n\n… (truncated, over GitHub's review body limit)"
        proc = post(repo, pr_number, {"commit_id": commit_sha, "event": "COMMENT", "body": fallback_body})

    sys.stdout.write(proc.stdout)
    if proc.returncode != 0:
        sys.stderr.write(proc.stderr)
        return proc.returncode
    return 0


if __name__ == "__main__":
    sys.exit(main())
