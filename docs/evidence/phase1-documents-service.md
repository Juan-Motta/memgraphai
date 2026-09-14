# P1.4a document shared-service contract and evidence

P1.4a delivers only the transport-neutral document service, SQLite persistence, and descriptor-bound verified reads. CLI and MCP adapter exposure remains a later P1.4 cut, so the parent P1.4 checkbox remains unchecked.

## Delivery forecast

The pre-write forecast is 1,290 product/evidence changed lines: application service/tests 450, SQLite schema/store/tests 560, descriptor read/tests 140, test fixture/evidence 140. This is below the authorized 1,500-line cut and 2,000-line human budget. No chain or size exception is selected.

## Exact operations and JSON fields

All operations use the settled `memgraphai.experimental/v1alpha1` envelope. Inputs are strict JSON objects and all document bytes use unpadded base64 in `content_base64`; this preserves exact Markdown bytes without response JSON escaping expansion.

| Operation | Scope | Required input fields | Success result fields |
| --- | --- | --- | --- |
| `document.create` | `project` or `workstream` | `document_id`, `revision_id`, `expected_revision_id: null`, `content_base64`, `provenance.origin`; optional `provenance.client`, `provenance.model` | `document_id`, `revision_id`, `project_id`, optional `workstream_id`, `created_at` |
| `document.update` | matching `project` or `workstream` | `document_id`, `revision_id`, nonempty `expected_revision_id`, `content_base64`, `provenance.origin`; optional client/model | `document_id`, `revision_id`, `project_id`, optional `workstream_id`, `created_at` |
| `document.list` | `project` or `workstream` | `{}` | `documents` metadata only: IDs, current revision, scope, and revision provenance; never content |
| `document.read` | matching `project` or `workstream` | `document_id`; optional `revision_id` | IDs, scope, exact selected revision, `content_base64`, checksum, byte count, and provenance |
| `document.history` | matching `project` or `workstream` | `document_id` | bounded revision metadata, newest first, never content |

`document.create` has an explicitly present JSON `null` expected revision; omitted or a non-null initial expectation is `invalid`. `document.update` requires a nonempty exact expected revision and a different nonempty revision ID. Create registration does not expose an empty precommit document: list/read only select a committed current pointer, and same-operation replay reaches the durable operation ledger rather than failing early on a registered row.

## Scope, provenance, and outcomes

Project scope selects only rows with `workstream_id IS NULL`; workstream scope selects exactly the declared project/workstream. The service never reads a session, current context, recency, or checkpoint. The store proves document/project/workstream/revision ownership before filesystem access. Document ID and scope are immutable; revision IDs belong to exactly one document.

Each revision stores `origin`, client-reported `client`, client-reported `model`, and server `created_at` atomically with the revision pointer and terminal operation result. Additive v4 migration preserves valid v3 rows with `origin = "unknown"` and NULL client/model/time; it invents no legacy provenance or timestamp.

Stable outcomes are `invalid` for malformed fields, bounds, and tokens; `not_found` for absent exact IDs; `scope_denied` for a declared scope that cannot select the target; `binding_mismatch` for cross-project/workstream/revision relationships; `conflict` for stale expected revisions or stale views; `integrity_discrepancy` for missing, tampered, symlinked, or nonregular selected files; and `idempotency_mismatch` for any changed immutable request field, including provenance. Replays revalidate scope before returning a durable terminal result and retain the original result timestamp. An after-commit response loss is recovered from the operation ledger; it never assumes rollback. Orphans remain report-only through the existing targeted storage evidence.

## Bounds and pagination

Decoded input Markdown is limited to 1 MiB before preparation. Document reads encode exact verified bytes as base64 and reject a result payload over 2 MiB before returning it. List and history enforce `1..200`, default 50, before allocation even for direct service calls; their opaque tokens bind contract version, operation, full scope, document filter where applicable, deterministic order, limit, cursor, and store view generation. Malformed or rebound tokens are `invalid`; a changed view is `conflict`.

The document result payload is bounded independently of JSON escaping by base64 encoding. This cut does not claim a global envelope cap for arbitrary pre-existing unbounded operation or scope IDs; enforcing that general envelope limit would require a separate settled change to `internal/app/contract.go`, which is outside this cut.

## Intended verification

Strict TDD starts with executable service/store/read failures, then covers project-general/workstream isolation, stale concurrent updates, committed response-loss replay, provenance mismatch, failed precommit invisibility/orphan reporting, foreign historical revisions, current/history corruption and nonregular leaves, descriptor-bound reads, token rebinding/staleness, and valid-v3 reopen.

## TDD cycle evidence

| Cycle | RED | GREEN | Triangulate / refactor |
| --- | --- | --- | --- |
| Shared create contract | `go test ./internal/app -run '^TestDocumentServiceCreatesProjectGeneralRevisionWithExplicitNullExpectation$' -count=1 -timeout=120s` failed with `outcome = "internal", want "ok"`. | The same focused command passed after strict input, explicit-null creation, base64 decoding, scoped write, and metadata response behavior. | Project-general/workstream isolation, stale update retention, history, direct page limits, token rebound, and stale token checks pass in `documents_test.go`. |
| Additive persistence | `go test ./internal/store/sqlite -run '^TestOpenMigratesValidV3DocumentRowsWithoutInventingRevisionProvenance$' -count=1 -timeout=120s` failed with `no such column: d.workstream_id`. | The focused command passed after additive v4 documents/revisions/view schema and triggers. | Valid v3 rows reopen with NULL scope/client/model/time and `unknown` origin; existing v1-v3 migration and recovery tests remain green. |
| Descriptor-bound read | `go test ./internal/revisionfs -run '^TestReadVerifiedReturnsExactBytesFromTheCheckedDescriptor$' -count=1 -timeout=120s` failed with `revision integrity discrepancy`. | The same command passed after `ReadVerified` returned bytes from its checked descriptor. | Existing symlink, nonregular, altered, missing, and hard-link-policy tests stay green; store tests add foreign historical, altered historical, missing/nonregular current checks. |

## Actual verification

| Command | Result |
| --- | --- |
| `go test ./internal/app ./internal/store/sqlite ./internal/revisionfs -count=1 -timeout=120s` | PASS after GREEN and again before final suite. |
| `go test ./... -count=1 -timeout=120s` | PASS once before repeated verification and PASS five additional independent runs. |
| `go test ./internal/app ./internal/store/sqlite ./internal/revisionfs -count=5 -timeout=120s` | PASS five focused repeats. |
| `git diff --check a8f098a` | PASS. |

No user library, CLI/MCP implementation, daemon, dependency change, commit, push, process helper, or global installation was used. Test fixtures use `t.TempDir()` and leave no persistent process. This evidence is limited to P1.4a shared services/store/verified reads; adapter parity and final P1.4 task closure remain pending.

---

## Gatekeeper correction: legacy compatibility and bounds

This correction retains the preceding P1.4a evidence unchanged and fixes only the three observed regressions.

| Defect | Genuine RED | GREEN / triangulation |
| --- | --- | --- |
| V3 operation digest and generation-zero views | A valid v3 fixture, built from the real foundation/project/continuity schema at versions 1–3, first returned list replay `ok`, want `conflict`; after the token correction it exposed `ExecuteOperation(op-committed legacy replay) ... idempotency_mismatch`, want committed. | `operationDigestV3` exactly preserves the `a8f098a:internal/store/sqlite/recovery.go` canonical digest when every added field is absent. Seeded committed and conflict rows replay after v4 migration; nonempty changed provenance remains `idempotency_mismatch`. The same migrated generation-zero view rejects both list and history continuations after `CreateDocument` advances it. |
| Base64 allocation order | The initial RED named the missing decoder. The actual prepatch behavior RED then failed `oversized decode allocations = 1, want zero before DecodeString`. | `RawStdEncoding.EncodedLen(1 MiB)` is checked before `DecodeString`; exact raw-unpadded 1 MiB input succeeds, one byte over and invalid base64 fail, and the oversized isolated decoder path allocates zero times. |

| Command | Result |
| --- | --- |
| `go test ./internal/app -run '^TestDecodeDocumentContentBoundsBeforeAllocation$' -count=1 -timeout=120s` | Initial missing-decoder RED; then actual prepatch allocation RED; PASS GREEN. |
| `go test ./internal/store/sqlite -run '^TestOpenMigratesValidV3DocumentRowsWithoutInventingRevisionProvenance$' -count=1 -timeout=120s` | RED for generation-zero stale replay, then RED for legacy digest mismatch, then PASS GREEN. |
| Both focused commands with `-count=5` | PASS. |
| `go test ./... -count=1 -timeout=120s` | PASS. |
| `git diff --check a8f098a` | PASS. |

Only `t.TempDir()` databases and filesystem roots were used; Go test processes exited normally, no helper process or persistent artifact required cleanup, and no user library was accessed.
