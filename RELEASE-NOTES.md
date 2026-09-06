# Release Notes

Rakitsu is licensed under the [Business Source License 1.1](LICENSE). Per the
license text, the **Change Date** — the point at which a given version's
source converts to the Change License (Apache 2.0) — is four years from the
date *that specific version* was first publicly distributed, not a single
fixed date for the whole project:

> "Change Date: Four years from the date each version of the Licensed Work
> is first publicly distributed."
>
> "This License applies separately for each version of the Licensed Work and
> the Change Date may vary for each version..."

This file is the durable, git-tracked record of that date for each release —
independent of GitHub's own release metadata, which is useful but mutable
(a release can be deleted and recreated, changing its timestamps).

Timestamps are recorded to the second, in UTC, not just a calendar date — a
bare date is ambiguous once a reader is in a different timezone than the one
that first comes to mind (this project's own maintainers are UTC+9, where a
release published late in the UTC day already falls on the next calendar
day locally).

| Version | First published (UTC) | Converts to Apache 2.0 (UTC) |
|---|---|---|
| [v0.2.0-alpha.1](https://github.com/paupawsan/rakitsu/releases/tag/v0.2.0-alpha.1) | 2026-09-05T23:26:49Z | 2030-09-05T23:26:49Z |
| [v0.2.0-alpha.2](https://github.com/paupawsan/rakitsu/releases/tag/v0.2.0-alpha.2) | 2026-09-06T02:07:06Z | 2030-09-06T02:07:06Z |
| [v0.2.0-alpha.3](https://github.com/paupawsan/rakitsu/releases/tag/v0.2.0-alpha.3) | 2026-09-06T06:36:17Z | 2030-09-06T06:36:17Z |

Add a row here whenever a new tag is released. Use the release's own
`published_at` timestamp — `gh release view <tag> --json publishedAt`, or
the GitHub UI — as the "First published" value, not the day this file
happens to be edited, which may not be the same day. If a release is ever
deleted and recreated (as happened once during `v0.2.0-alpha.1`'s own
initial setup), update the row to the *recreated* release's timestamp —
that's the one that actually counts as "first publicly distributed".
