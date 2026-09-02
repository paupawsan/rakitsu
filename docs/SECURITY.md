# Rakitsu security & trust model

Read this before running Rakitsu with configs you did not write, or before
exposing `rakitsu serve` to anything beyond your own machine.

Rakitsu runs LLM-driven agents that execute tools — shell commands and file
operations — on your behalf. The security posture depends entirely on **which
of two modes** you are in.

## Mode 1 — local CLI (`rakitsu run`)

Trust model: **the config is yours, and tools run as you.**

When you run `rakitsu run config.yaml "…"`, any `cli` or `fs` tool the config
declares executes with **your user's full permissions**, in your working
directory. This is by design — it is what makes a local coding agent useful.

### The cli tool allowlist is a footgun fence, not a sandbox

The `cli` tool has a built-in command allowlist (`ls`, `cat`, `git`, `go`,
`python3`, `node`, `bash`, …) and a denylist (`rm`, `sudo`, `dd`, …). **Treat
this as a guard rail against obvious mistakes, not as a security boundary.**

The allowlist includes general-purpose interpreters — `python3`, `node`, `go`,
`bash`, `sh` — and `find`. Any of these can execute arbitrary code or delete
files, so the denylist does **not** actually contain a hostile config:

```yaml
# All of these run despite `rm` being "blocked":
python3 -c "import shutil; shutil.rmtree('...')"
find . -delete
bash -c "$(printf 'r''m -rf x')"
```

The shell-payload lint (`lintShellPayload`) is best-effort and trivially
bypassable — the source says so explicitly.

**Consequence:** only run configs you trust, the same way you only run shell
scripts you trust. Do not rely on the allowlist to make a config from an
untrusted source safe.

### Reducing blast radius

- **`fs` tools:** always set `allowed_paths` explicitly. The default is `["."]`
  — the *entire* launch directory — so a config that omits it and is launched
  near secrets can read `.env`, `.git`, credentials, etc. Point it at the
  narrowest directory the task needs.
- **`cli` tools:** keep `settings.allowed_commands` minimal. Every command you
  add is a new capability. `docker` in particular is effectively root on most
  dev machines (it can mount the host and run privileged containers).
- **Untrusted work:** use `sandbox: { type: docker }` (see below).

### Docker sandbox

`sandbox: { type: docker }` gives stronger isolation than `local_restricted`:
the container's root filesystem is read-only (with a small writable `/tmp`
for scratch space) and setuid/setgid privilege escalation is blocked
(`--security-opt=no-new-privileges`). The mounted working directory stays
read-write, and networking is only disabled when `network_isolated: true`
is set. It does not yet drop Linux capabilities, run as non-root, or set a
pids limit. Prefer it over local execution for untrusted configs, but do
not treat it as a hardened jail.

## Mode 2 — the hub (`rakitsu serve`)

Trust model: **loopback-only and single-user by default.**

`rakitsu serve` exposes a control plane that can **upload a config and start a
run** — i.e. it can execute code as the server's user. That is safe on
`localhost` (only you can reach it) and unsafe on any routable interface
without authentication.

### Network exposure requires an API token

Binding a non-loopback host (`--host 0.0.0.0`, a LAN IP, a Tailscale address)
is **refused** unless you set an API token:

```bash
export RAKITSU_API_TOKEN="$(openssl rand -hex 32 | tr -d '\n')"
rakitsu serve --host 0.0.0.0
```

When the token is set, the code-execution and state-changing endpoints
(`/api/run`, `/api/configs/upload*`, `/api/browse`, `/api/workdir`,
`/api/chat/start`, `/api/debug/*`, config deletion, and cross-session
messaging) require `Authorization: Bearer <token>`. Read-only UI endpoints and
the static web UI remain open so the page can load.

The same token also satisfies cross-session messaging
(`RAKITSU_SESSION_MSG_TOKEN` is still accepted for that feature specifically).

### Known limitations (Phase 2)

- The **web UI over the network is not supported yet**: the browser cannot
  attach the bearer token to page navigations, so use an SSH tunnel to reach
  the UI remotely, or call the API directly with the token.
- The **MCP (`/mcp`) and A2A (`/a2a`) endpoints require the same bearer
  token as everything else** (`requiresAuth` in `internal/server/auth.go`
  matches both by path). The Agent Card discovery route
  (`/.well-known/agent-card.json`) is deliberately left open — the A2A spec
  expects it to be publicly fetchable so a client can learn what auth is
  required before authenticating. The non-loopback bind refusal remains the
  backstop for all of it — keep it behind a tunnel or a trusted network.
- WebSocket upgrades are restricted to localhost/same-origin (so a random web
  page cannot drive your agent), but there is no CSRF token on same-origin
  POSTs yet.

## Reporting

This is pre-release (alpha) software. If you find a security issue, please open
an issue tagged `security` rather than a public disclosure.

## Summary

| You are… | Safe? | What protects you |
|----------|-------|-------------------|
| Running your own config locally | Yes | You trust your own config |
| Running an **untrusted** config locally | **No** | Allowlist is not a boundary — use Docker sandbox + narrow `allowed_paths` |
| `serve` on localhost | Yes | Only you can reach it |
| `serve` on the network, no token | **Refused** | Won't start |
| `serve` on the network, with token | Mostly | Token-gated API; UI/MCP/A2A caveats above |
