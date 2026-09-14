# Phase 1 checkpoint shared-service evidence

P1.5a provides only the shared application service and SQLite persistence for explicit checkpoint save/read. CLI/MCP adapters, checkpoint list/update/delete, and task references are intentionally absent.

## Scope and decisions

| Topic | Implemented behavior |
| --- | --- |
| Identity | The request names immutable checkpoint, backing-document, backing-revision, source-document, and source-revision IDs. |
| Binding | Save requires an open project/workstream/session triple; read requires an open caller triple and permits only the same workstream, including an explicitly resumed run. |
| Prose | Markdown is raw-base64 input bounded at 64 KiB before decode allocation; SQLite owns all metadata and Markdown remains the backing revision content. |
| References | At least one distinct exact revision is required. Each must belong to the project and be project-general or in the checkpoint workstream. |
| Freshness | Each reference is fresh only when its document's current pointer equals that exact revision; session or unrelated-workstream recency has no role. |
| Durability | Registration binds the full immutable checkpoint request before claiming. The backing revision/pointer, checkpoint metadata/references, and operation terminal success commit in one SQLite transaction. |

## Strict TDD chronology

| Cycle | RED command and observed result | GREEN / triangulation evidence |
| --- | --- | --- |
| Initial service contract | `go test ./internal/app -run '^TestCheckpointSaveRequiresAnActiveExplicitSessionBinding$' -count=1 -timeout=120s` failed behaviorally: `internal`, want `ok`, using documented compileable test-only RED scaffolding. | The focused command passed after the service dispatch, request validation, bounded prose decoder, and persistence boundary were added. |
| V5 persistence schema | `go test ./internal/store/sqlite -run '^TestOpenRecordsCheckpointMigrationVersion$' -count=1 -timeout=120s` failed behaviorally: version `4` / table count `0`, want `5` / `1`. | The focused command passed after migration V5 created checkpoint and exact-reference tables/triggers. The valid V4-to-V5 fixture passes. |
| Duplicate reference rejection | `go test ./internal/app -run '^TestCheckpointSaveRejectsDuplicateExactReferences$' -count=1 -timeout=120s` failed behaviorally: `ok`, want `invalid`. | The same command passed after duplicate `(document_id, revision_id)` rejection. The single-reference save and 64 KiB boundary tests triangulate accepted and rejected input. |

The later SQLite integration cases were added as focused coverage after the initial service/migration cycles; they are not represented as retrospective RED claims.

## Verified cases

- Active binding saves and reads exact checkpoint prose plus a current source revision.
- A disconnected source remains unchanged; an explicit same-workstream resume can read its checkpoint, while a fork workstream is denied.
- Freshness becomes false when only the referenced document pointer advances.
- Changed immutable references under the same operation ID return `idempotency_mismatch`.
- A precommit publication interruption leaves the checkpoint unreadable, survives SQLite reopen, and retries into one committed checkpoint.
- A post-commit response loss is reachable through the selected checkpoint ID, and backing-prose tampering reports `integrity_discrepancy`.

## Commands and results

| Command | Result |
| --- | --- |
| `go test ./internal/app ./internal/store/sqlite -count=1 -timeout=120s` before P1.5a edits | PASS safety net. |
| `go test ./internal/app ./internal/store/sqlite -run '^(TestCheckpointSaveRequiresAnActiveExplicitSessionBinding|TestOpenRecordsCheckpointMigrationVersion)$' -count=1 -timeout=120s` | PASS after the two initial GREEN steps. |
| `go test ./internal/store/sqlite -run '^(TestCheckpoint|TestOpenMigratesValidV4ToV5WithoutInventingCheckpoints)' -count=1 -timeout=120s` | PASS. |
| `go test ./internal/app ./internal/store/sqlite -run '^TestCheckpoint' -count=5 -timeout=120s` | PASS. |
| `go test ./... -count=1 -timeout=120s` | PASS. |
| `git diff --check` | PASS. |

## Limits

The filesystem fault tests model named interruption points, not power-loss or platform durability. No checkpoint adapter, automatic resume, model-state restoration, filesystem restoration, task reference, list/update/delete, or assertion that verification completed is included.

## P1.5a persistence hardening correction

This correction preserves the preceding P1.5a evidence, including its SHA-256 `de43ced6333a6a9ca77f7f472f8dd1e014840616b7e07d9e1a94fbb6823e44f7`, as historical failed-verification input. It adds no adapter or checkpoint feature.

### Strict TDD chronology

| Cycle | Command and observed result |
| --- | --- |
| Safety net | `go test ./internal/store/sqlite -count=1 -timeout=120s` passed before this correction. |
| RED | `go test ./internal/store/sqlite -run '^(TestCheckpointSchemaRejectsDirectAggregateMutation|TestCheckpointStoreRollsBackWhenBackingPointerCASDoesNotAdvance)$' -count=1 -timeout=120s` failed behaviorally: direct checkpoint `DELETE` and reference `UPDATE` returned nil errors, and a controlled backing-pointer CAS loss returned nil rather than `conflict`. |
| GREEN | The same focused command passed after migration V6 added direct-SQL immutability triggers and checkpoint publication required exactly one affected pointer row. |
| TRIANGULATE | The direct-SQL test separately exercises checkpoint deletion, reference update, and reference deletion. Its controlled filesystem fixture advances the backing pointer after immutable-file publication; the save now returns `conflict` and the transaction leaves no checkpoint, checkpoint revision, or committed operation. `go test ./internal/store/sqlite -run '^(TestCheckpoint|TestOpenMigratesValidV5ToV6AddingCheckpointGuards)$' -count=1 -timeout=120s` passed. |
| REFACTOR | `gofmt` completed with no abstraction added: three SQLite triggers and one `RowsAffected` check are the entire enforcement path. |

### Schema and publication guarantees

- Migration V6 upgrades V5 databases by adding triggers that reject direct checkpoint deletion and direct checkpoint-reference update or deletion; the V5-to-V6 fixture confirms all three guards exist without inventing checkpoint rows.
- `SaveCheckpoint` now treats a zero-row `documents.current_revision_id IS NULL` compare-and-swap as `conflict`. The surrounding transaction rolls back metadata, revision, and terminal success instead of acknowledging a checkpoint that did not become current.
- The controlled fixture is test-local orchestration using the existing filesystem fault point and direct SQLite competitor update; production receives no test-only hook or framework.

### Commands and results

| Command | Result |
| --- | --- |
| `go test ./internal/store/sqlite -count=1 -timeout=120s` before correction | PASS safety net. |
| `go test ./internal/store/sqlite -run '^(TestCheckpointSchemaRejectsDirectAggregateMutation|TestCheckpointStoreRollsBackWhenBackingPointerCASDoesNotAdvance)$' -count=1 -timeout=120s` | Expected behavioral RED, then PASS GREEN. |
| `go test ./internal/store/sqlite -run '^(TestCheckpoint|TestOpenMigratesValidV5ToV6AddingCheckpointGuards|TestOpenRecordsCheckpointMigrationVersion|TestOpenRecordsFoundationMigrationVersion|TestOpenMigratesValidV2ContinuityRowsWithoutInventingProvenanceOrTime|TestOpenAppliesMigrationOnceAndPersistsBoundedMetric)$' -count=1 -timeout=120s` | PASS triangulation/refactor. |
| `go test ./internal/store/sqlite -run '^TestCheckpoint' -count=5 -timeout=120s` | PASS. |
| `go test ./... -count=1 -timeout=120s` | PASS. |
| `git diff --check` | PASS. |

The correction still does not establish power-loss, platform, backup, automatic restoration, adapter parity, list/update/delete feature, or verification-completion behavior.

## P1.5a V4 migration-evidence correction

**Authoritative correction:** SHA-256 `1d15f449a397041eefccb05245639fb91cdf9eb82252032d290505ae49cedac0` is preserved as historical failed-verification evidence. Its claim that `TestOpenMigratesValidV4ToV5WithoutInventingCheckpoints` passed was false: immediately before this correction, `go test ./internal/store/sqlite -list '^TestOpenMigratesValidV4ToV5WithoutInventingCheckpoints$' -count=1 -timeout=120s` printed no matching test name. An unmatched `-run` exit was not accepted as proof.

This coverage-only correction adds that exact test using the actual V4 fixture construction: `foundationSchema`, `projectSchema`, `continuitySchema`, and `documentSchema`, then ledger versions 1–4. The fixture preserves meaningful preexisting project, workstream, disconnected session, project-general and workstream documents, revisions, and a committed operation. Opening current V6 must preserve those rows; append ledger versions 5 and 6; install all V5/V6 checkpoint constraints; and create zero checkpoint or checkpoint-reference rows.

| Command | Result |
| --- | --- |
| `go test ./internal/store/sqlite -list '^TestOpenMigratesValidV4ToV5WithoutInventingCheckpoints$' -count=1 -timeout=120s` after the test was added | Printed the exact test name. |
| `go test ./internal/store/sqlite -run '^TestOpenMigratesValidV4ToV5WithoutInventingCheckpoints$' -count=20 -v -timeout=120s` | PASS; verbose output contained 20 exact `RUN`/`PASS` pairs. |

This is coverage-only, not a retrospective RED claim: the migration already worked, and no production file changed. The existing V5→V6 guard migration test remains in place.
