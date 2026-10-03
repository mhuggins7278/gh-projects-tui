# Remaining issue audit — 2026-09-30

Starting commit: `0400201`. The working tree was clean. This pass checked all
nine open issues against their acceptance criteria; existing implementations
were verified before treating their still-open tracking issues as completed.

## Completed work

| Issue | Existing implementation and audit result |
| --- | --- |
| [#14](https://github.com/mhuggins7278/gh-projects-tui/issues/14) — broader saved filters | The bounded grammar and evidence matrix landed in `ecbba77`. This audit additionally rejects unquoted grouping parentheses, preserves quoted literal punctuation, and expands the reproducible read-only membership/pagination probe. See the [fresh filter evidence](filter-audit-2026-09-30.md) and [authoritative grammar matrix](api-contract.md#saved-filter-grammar-and-evidence); forms without supporting evidence remain explicitly gated. |
| [#23](https://github.com/mhuggins7278/gh-projects-tui/issues/23) — parent-issue filters | Implemented in `f3cc1b0`. New `TestParentIssueSavedViewOpensAndPagesExactFilter` exercises the original quoted filter through the actual view-loading gate and both item pages, across board/table layouts and user/organization owners. A fresh read-only sandbox probe matched parent metadata: positive quoted/unquoted queries returned two children, and negations returned the other 24 items across 12 pages. The original private view's reopening is fixture-backed, not a fresh live comparison. |
| [#24](https://github.com/mhuggins7278/gh-projects-tui/issues/24) — wider table titles | Implemented in `a768247`. New `TestTableTitleWidthRespectsSavedOrderAndUnicode` covers eight fields, Title first/middle/last, widths 40/80/120/240, saved column order, readable allocation, and rows bounded by terminal cells. It covers issue numbers, CJK, joined emoji, and combining characters. No production width change was needed. |
| [#22](https://github.com/mhuggins7278/gh-projects-tui/issues/22) — roadmap investigation | Investigation, safe decoding/rejection fixtures, and the bounded representation proposal landed in `b1000f3`. The fixture-only renderer is now complete in #27. All saved roadmaps remain unsupported. |
| [#25](https://github.com/mhuggins7278/gh-projects-tui/issues/25) — endpoint-mapping decision | The explicit no-go decision landed in `b230d1d`. The 2026-09-30 recheck still found only `visibleFields` in GraphQL view configuration and no documented REST view read for endpoint mapping. Missing, stale, ambiguous, and unsupported-definition rejection fixtures pass. Completing this investigation records the unavailable API capability; it does not supply a usable mapping. |

Closing keywords in the audit/fix commit close these five tracking issues when
it reaches `main`. Existing implementation commits are preserved.

## Still blocked

| Open issue | Prerequisite to resume |
| --- | --- |
| [#26](https://github.com/mhuggins7278/gh-projects-tui/issues/26) — date placement/filter/order verification | A supported source for selected endpoint field IDs, followed by existing populated roadmap web/API comparisons. The earlier date-empty samples and synthetic bars/points do not establish placement parity. |
| [#28](https://github.com/mhuggins7278/gh-projects-tui/issues/28) — live read-only integration | Usable endpoint mapping and #26's verified semantics. Renderer #27 is complete; the picker still prevents roadmap item loading and writes. |
| [#29](https://github.com/mhuggins7278/gh-projects-tui/issues/29) — single-select grouping | The initial verified timeline and existing grouped-view comparisons for option order, unset values, and saved row sorting. |
| [#30](https://github.com/mhuggins7278/gh-projects-tui/issues/30) — iteration endpoints | The initial verified timeline, explicit endpoint mapping, and existing populated comparisons for active/completed/missing/mixed endpoints and date conversion. |

These feature issues stay open. Their issue descriptions and the
[roadmap investigation](api-contract.md#roadmap-layout-investigation) record the actual
missing prerequisites. No project data or authentication was changed, and no
undocumented endpoint, browser credential, inferred field role, or local
substitute was used to bypass the roadmap gate.

## Validation

The full `go test -race ./...` suite, `go vet ./...`, and local command build
passed. The live-tagged package compiles with its opt-in probes disabled. The
expanded read-only filter contract passed all 29 cases; the parser fuzz run
completed 32,877 executions without failures. The existing roadmap metadata,
picker rejection, and timeline fixtures also pass. No release tag was created.

## Subsequent endpoint-source decision

The user subsequently decided to keep roadmap views unsupported. No endpoint
configuration workaround is exposed by the application.

The [populated sample probe](roadmap-date-verification-2026-09-30.md) returned
540 unique items over six pages and matched 21 visible web rows to project
position order. Browser access ended before bar/point geometry comparisons;
the evidence does not complete #26 or enable #28.
