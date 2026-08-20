# `internal/llm/format/testdata/`

Sanitized fixtures captured from real session JSONLs. Used by golden tests in `format_test.go`.

## Sanitization rules

Real session JSONLs contain repo-specific identifiers. Before checking a fixture in here, run the regex sweep below over the raw blob:

| Pattern | Replacement | Reason |
|---|---|---|
| `#\d+` (issue/PR numbers) | `#NN` | Stable across repo activity |
| `\b[0-9a-f]{7,40}\b` (commit SHAs) | `0000000` | Stable across history |
| `(?i)paupawsan/[a-z0-9-]+` (repo names) | `org/repo` | Independent of org rename |
| `(?i)/Users/[^/]+/` and `/home/[^/]+/` | `/Users/USER/` | Path independence |
| `(?:\d{4})-(?:\d{2})-(?:\d{2})` (ISO dates) | `YYYY-MM-DD` | Stable across calendar |
| API keys, env values, service account paths | `REDACTED` | Security |

Tests check extraction *shape* — bullet structure, marker presence, length distribution — not specific identifiers, so sanitization preserves test value.

## Fixture file naming

```
{strategy_n}_{shape}_{outcome}.txt
```

- `strategy_n`: which extraction strategy this fixture exercises (`strategy_5`, `strategy_3`, `false_positive_guard`, etc.)
- `shape`: shape descriptor (`thus_imperative_short`, `trailing_bullet_list`, `csv_label_prefix`)
- `outcome`: `extract` (should succeed) or `reject` (should fail / fall through to next strategy)

Examples:
- `strategy_5_thus_imperative_short_extract.txt`
- `strategy_5_meta_commentary_reject.txt`
- `strategy_3_trailing_bullet_list_extract.txt` (added in a later PR)

## Adding a new fixture

1. Capture a real session blob: `grep -l "REASONING-ONLY" ~/.rakitsu/sessions/*.jsonl`, then pull the relevant `final_answer` field.
2. Run the sanitization sweep (script TBD; for now, do it by hand using the table above).
3. Add to `testdata/` with the naming convention.
4. Add a test in `format_test.go` that loads the fixture via `os.ReadFile` and asserts extract / reject behavior.

Strategies 3 and 4 (trailing bullet list, CSV label prefix) aren't implemented yet. Strategy 5 lands first with smaller hand-crafted fixtures focused on shape rather than full real captures.
