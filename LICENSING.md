# Rakitsu Licensing — Public Rationale

**TL;DR**: **Rakitsu** (part of the Rakitsu brand family) is under the **Business Source License 1.1** with a narrow non-compete clause. You can use it freely for personal, internal, educational, and research purposes. The output of your agent runs, your agent configuration YAML files, and artefacts produced by `rakitsu export` are yours — entirely unencumbered by this License. The only restriction is that you can't offer Rakitsu itself as a hosted or embedded service competing with our commercial offering. Converts automatically to **Apache 2.0** four years after each release.

If you're in doubt about whether your intended use is covered, read on — and if still unsure, email us.

---

## What license is Rakitsu under?

- **Business Source License 1.1** (BSL 1.1) — a source-available license originally authored by MariaDB, now widely used (CockroachDB, Sentry, MariaDB MaxScale, Couchbase, HashiCorp Terraform).
- **Change Date**: four years from each version's first public release, at which point that version automatically converts to Apache License 2.0.
- **Licensors**: Sakaki Natan (primary author) & Paulus Ery Wasito Adhi (paupawsan, co-author).

See [`LICENSE`](./LICENSE) for the full legal text.

## What can I do with Rakitsu under the current license?

### Yes, freely — no commercial license needed

- **Personal use**: any use on your own projects, hobbies, or open-source contributions.
- **Internal use at your organisation**: building, running, and debugging agents for your own team's workflows, even commercially — on any infrastructure you control, on-premises or your own cloud servers. This includes running Rakitsu to automate work behind a commercial product, where your customers receive the results but never operate Rakitsu themselves.
- **Educational use**: teaching, learning, courses, bootcamps, tutorials, student projects.
- **Research use**: academic or industrial research, including research that leads to publications or commercial products.
- **Modification and redistribution**: fork rakitsu, modify it, redistribute it — as long as you retain the license and respect the non-compete clause.
- **Using your agents' output however you want**: agent configuration YAML files you author, agent run outputs (LLM responses, tool call results, generated artefacts), session logs, event streams, and anything produced by `rakitsu export` or a RuntimeTarget export are **not** covered by the BSL. Ship them under MIT, Apache, proprietary, sell them, serve them — they're yours. This is the most important point for users building production systems with Rakitsu, and it's explicit in the Additional Use Grant.

### Requires a commercial license (email us)

- **Offering Rakitsu as a hosted service** that competes with our commercial offering — e.g. a SaaS "Rakitsu Cloud" you run and charge for.
- **Embedding Rakitsu as a competing feature inside your own paid product** — e.g. bundling Rakitsu as a "visual agent builder" or "agent-run inspector" feature that directly competes with what we sell.

Everything else is free. The clause is deliberately narrow.

**The test is who uses Rakitsu, not whether you name it.** If only your own team operates Rakitsu and third parties receive the products or outputs of that work, no commercial license is needed — regardless of where it runs or whether Rakitsu is ever mentioned. If third parties themselves use Rakitsu's functionality — as a hosted service or as a feature embedded in your product — that is the territory above, branded or not.

## Why BSL and not MIT / Apache 2.0?

Three reasons, stated honestly:

1. **We plan to build a commercial product on top of Rakitsu** (hosted agent builder, team features, managed enterprise integrations). The BSL's non-compete clause protects that plan from a well-funded competitor taking Rakitsu's code and offering a hosted service before we can.
2. **We want to be open to buyout offers**. The BSL doesn't affect this directly, but it does keep the commercial value of the code concentrated rather than diluted across clones and hosted-service competitors.
3. **We want to sell commercial licenses** to organisations whose use falls outside the Additional Use Grant. This is a revenue path that pure permissive licenses don't offer.

We seriously considered MIT and Apache 2.0. What made us choose BSL:

- MIT / Apache give away the right to compete against us using our own code. For a tool with a clear commercial product path, that's a material cost.
- BSL preserves all community-adoption benefits (source visible, forkable, modifiable, free for almost everyone) while adding a narrow anti-compete clause.
- The automatic 4-year conversion to Apache 2.0 means every version of Rakitsu becomes fully permissive eventually — this is not a license lock-in.

## Why not AGPL or SSPL?

- AGPL 3.0 would force any hosted service to open-source its entire stack. Too aggressive for our goal; penalises honest users who happen to run Rakitsu server-side.
- SSPL is even broader and widely disliked. It's the license that sparked the Elastic / AWS fork and the MongoDB ecosystem pushback. We wanted commercial protection without starting that kind of fight.

## What if I want to use Rakitsu in a way not covered above?

Email the Licensors via the project repository. Commercial license terms are negotiable; we're friendly and we want this to work for you. Typical scenarios we accommodate:

- Hosted commercial service (we'll discuss terms)
- Embedded inside your paid product (we'll discuss terms)
- OEM / redistribution with custom terms
- Removing the Change Date restriction early (rare but possible)

## How does this interact with contributions?

Contributions are accepted under a **DCO sign-off** (`git commit -s`), described in [`CONTRIBUTING.md`](./CONTRIBUTING.md) — it certifies you wrote the contribution (or have the right to submit it) and agree to license it under Rakitsu's license, same as the rest of the codebase. You keep copyright on your contribution; there's no separate Contributor License Agreement, and no royalties, compensation, or support obligations attach in either direction.

One consequence worth stating plainly: because there's no CLA, contributions are licensed to the project strictly under the BSL 1.1 terms above (including its automatic 4-year Apache 2.0 conversion) — nothing broader. If the project ever wanted to do something with a contribution outside those terms, that would need the contributor's separate agreement, same as any DCO-only project.

## The output of Rakitsu (your agent configs, run outputs, exports) — clarification

This is worth saying twice because it's the part most easily misread:

> The output produced by Rakitsu at your direction is **not** subject to the BSL. You own it and can license it however you want.

If you write an agent configuration YAML, run it through Rakitsu, and collect its output (LLM responses, tool call results, generated files), every one of those artefacts is yours. Ship them, sell them, open-source them — none of it is restricted by Rakitsu's license. Same for anything produced by `rakitsu export` or a RuntimeTarget export. The BSL governs *the tool*, not *what you build with the tool*.

This is explicit in the Additional Use Grant of the [`LICENSE`](./LICENSE) file.

## Comparison at a glance

| Use case | MIT / Apache 2.0 | Rakitsu's BSL | AGPL | Proprietary |
|---|---|---|---|---|
| Personal projects | yes | yes | yes | no |
| Internal commercial use | yes | yes | caveat (network copyleft) | no |
| Academic research | yes | yes | yes | no |
| Fork and redistribute | yes | yes | yes | no |
| Ship agent configs and run outputs commercially | yes | yes (explicit in clause) | yes | no |
| Offer the tool itself as a competing hosted service | yes | no (needs commercial license) | caveat (must open-source your stack) | no |
| Eventually becomes fully permissive? | already is | yes (Apache 2.0 after 4y) | no | no |

## Trademark

"Rakitsu" (and any associated logos, once stable) is a trademark of the Licensors. The BSL and the eventual Apache 2.0 conversion **do not grant you any right in any trademark or logo**. You can fork the code and release your own version — you just can't call it "Rakitsu" or use our logos in a way that implies endorsement or affiliation.

## Questions?

- Specific license questions (is my use allowed?) — open a GitHub Discussion or email the Licensors.
- Commercial licensing — contact the Licensors via the project repository.
- Contribution questions — see [`CONTRIBUTING.md`](./CONTRIBUTING.md).

---

*This document is a public rationale, not legal advice. For binding terms, the [`LICENSE`](./LICENSE) file is authoritative.*
