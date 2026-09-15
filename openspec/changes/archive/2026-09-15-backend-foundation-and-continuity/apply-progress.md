# Apply progress: backend-foundation-and-continuity

## Status consumed

- Native SDD status was `apply: ready` for this OpenSpec change, with repository-local edit authority limited to `/Users/juanmotta/Desktop/personal/memgraphai`; `actionContext.warnings` was empty.
- The parent authorized only work unit `p0-1-build-sqlite-probe`: P0.1 stepwise Phase 0 implementation and project Go dependency downloads. No Phase 1 implementation, commit, PR, global install, paid/live integration, or user-library access was performed.
- Strict TDD was active. The runner prerequisite was confirmed before behavioral RED with `go test ./...` exiting successfully after root module setup.

## Completed implementation task

- [x] P0.1 — Establish root `go.mod`/`go.sum`, verify the runner, compare executable SQLite candidates, implement the smallest probe/tests, and record build evidence.
  - Persisted task update: the P0.1 implementation-owned row in `tasks.md` is visibly `- [x]` and its superseded `cmd/memgraph/` spelling is now `cmd/memgraphai/`.
  - Evidence digest: `sha256:c088a8c29545b4aab83f8edc29005bdc81c19535cb76525eb7de49647d653496` for `docs/evidence/phase0-build.md`.

## Files changed

- `go.mod`, `go.sum`
- `cmd/memgraphai/doc.go`, `cmd/memgraphai/main.go`, `cmd/memgraphai/main_test.go`
- `internal/store/sqlite/probe.go`, `internal/store/sqlite/probe_test.go`
- `internal/testkit/tempdb.go`, `internal/testkit/tempdb_test.go`
- `docs/evidence/phase0-build.md`
- `openspec/config.yaml`
- `openspec/changes/backend-foundation-and-continuity/tasks.md`
- `openspec/changes/backend-foundation-and-continuity/apply-progress.md`

## Evidence and verification

`modernc.org/sqlite v1.58.0` was selected only after executable probes compared it with `github.com/mattn/go-sqlite3 v1.14.52`. Both reported `sqlite_compileoption_used('ENABLE_FTS5') = 1`, created and queried a real FTS5 table, and committed a transaction. The selected path was additionally verified under `CGO_ENABLED=0`.

| Command | Result |
| --- | --- |
| `go test ./internal/testkit ./internal/store/sqlite ./cmd/memgraphai` | PASS |
| `go test ./...` | PASS |
| `go build -o /tmp/memgraphai-p0-1-build/memgraphai ./cmd/memgraphai && /tmp/memgraphai-p0-1-build/memgraphai probe` | PASS; printed successful FTS5/transaction probe |
| `CGO_ENABLED=0 go test ./...` | PASS |
| `CGO_ENABLED=0 go build -o /tmp/memgraphai-p0-1-cgo-disabled/memgraphai ./cmd/memgraphai && /tmp/memgraphai-p0-1-cgo-disabled/memgraphai probe` | PASS |

Temporary candidate modules, test databases, and `/tmp/memgraphai-p0-1-*` build directories were removed and their absence was verified. Tested platform facts and observed dynamic links are recorded in `docs/evidence/phase0-build.md`; no support promise is made.

## TDD Cycle Evidence

| Task | Test file | Layer | Safety net | RED | GREEN | TRIANGULATE | REFACTOR |
| --- | --- | --- | --- | --- | --- | --- | --- |
| P0.1 isolated test artifacts | `internal/testkit/tempdb_test.go` | Unit | N/A (new) | `undefined: TempSQLitePath` | Focused test passed after smallest helper | Distinct names produce distinct artifact paths | `gofmt`; focused suite passed |
| P0.1 SQLite probe | `internal/store/sqlite/probe_test.go` | SQLite integration | N/A (new) | `undefined: Probe` | FTS5/transaction test passed | Requested present and absent FTS terms returned 1 and 0 | No speculative abstraction; formatted suite passed |
| P0.1 executable | `cmd/memgraphai/main_test.go` | Process integration | N/A (new) | `main` undeclared when executing `go run . probe` | Probe command test passed | Unknown argument returns its usage path | No source refactor required; formatted suite passed |

The initial testkit green assertion and the `go run` nonzero-exit wrapper assertion were corrected as test-harness observations without changing correct production behavior; full details and commands are in the build evidence.

## Design deviations

- The design's historical `cmd/memgraph` path was superseded by explicit approval; P0.1 uses `cmd/memgraphai` and root module `memgraphai`.
- No schema, migration, filesystem publication protocol, FTS retrieval behavior, adapter, or Phase 1 source was added.

## Workload and boundary

- Work-unit boundary: P0.1 only (`p0-1-build-sqlite-probe`). No PR, chain, commit, or size exception was selected or created.
- New module/probe/test/evidence files total 353 physical lines before this progress record, including the 50-line generated `go.sum`; the focused OpenSpec edits are additional. This is well below the active 2,000-line review budget, so no `ask-on-risk` delivery decision is needed for this completed work unit.

## Remaining implementation tasks

- [ ] Add identity constraint tests and the smallest concrete domain/SQLite behavior in `internal/domain/`, `internal/store/sqlite/`, and `internal/testkit/`; run RED → GREEN → TRIANGULATE → REFACTOR for aliases, symlinks, nested/unavailable/moved paths, same-named projects, cross-project workstreams, mismatched sessions, and association-versus-authorization, recording results in `docs/evidence/phase0-identity.md`. <!-- sdd-owner: implementation -->
- [ ] Add fault-injection tests and the narrow filesystem/publication implementation in `internal/testkit/`, `internal/revisionfs/`, and `internal/store/sqlite/`; run RED → GREEN → TRIANGULATE → REFACTOR across prepare/sync/no-clobber, expected-revision rejection, pointer commit, response loss, first publication, replacement, and preparation failure, recording evidence in `docs/evidence/phase0-publication.md`. <!-- sdd-owner: implementation -->
- [ ] Add restart, checksum, orphan, takeover, and multi-process tests with the minimum recovery/ownership/retry implementation in `internal/testkit/`, `internal/store/sqlite/`, and `internal/revisionfs/`; run RED → GREEN → TRIANGULATE → REFACTOR for unknown commits, lost responses, altered/missing files, active staging, stale writers, simultaneous retries, idempotency mismatch, authorization recheck, committed replay after later revision advancement, and retry exhaustion, then record recovery, concurrency, and Phase 0 exit evidence. <!-- sdd-owner: implementation -->
- [ ] Resolve the permission, exact envelope/versioning, MCP library/build, pagination-token, and session-interruption contracts in `docs/evidence/phase1-contracts.md` only, documenting the future responsibilities of `internal/app/contract.go` and both adapters without writing source code; do not add ceremonial tests of prose completeness, and hand real failing contract-behavior cases to P1.1 after concrete decisions while preserving noninteractive errors and semantic parity. <!-- sdd-owner: implementation -->
- [ ] Add tests and concrete wiring in `go.mod`, `cmd/memgraph/main.go`, `internal/app/contract.go`, `internal/store/sqlite/`, `internal/telemetry/`, and `internal/testkit/`; run RED → GREEN → TRIANGULATE → REFACTOR for scope, outcomes, migration versioning, limits, required metrics, response bytes, and isolated metric failure. <!-- sdd-owner: implementation -->
- [ ] Add shared-service, CLI, and MCP tests plus concrete project/path operations in `internal/app/projects.go`, `internal/adapter/cli/`, `internal/adapter/mcp/`, and `internal/testkit/`; run RED → GREEN → TRIANGULATE → REFACTOR for identity, aliases, ambiguity/unavailability, nested/symlink paths, and noninteractive errors. <!-- sdd-owner: implementation -->
- [ ] Add continuity tests and concrete workstream/session persistence in `internal/app/continuity.go`, `internal/store/sqlite/`, `internal/adapter/cli/`, `internal/adapter/mcp/`, and `internal/testkit/`; run RED → GREEN → TRIANGULATE → REFACTOR for mismatches, missing sessions, explicit close, disconnect, interleaved logical MCP sessions, resume, and fork. <!-- sdd-owner: implementation -->
- [ ] Add document tests and concrete services in `internal/app/documents.go`, `internal/revisionfs/`, `internal/store/sqlite/`, `internal/adapter/cli/`, `internal/adapter/mcp/`, and `internal/testkit/`; run RED → GREEN → TRIANGULATE → REFACTOR for scope-only reads, history, stale writes, precommit failure, response loss, altered files, idempotent replay, and orphan reporting, without adding FTS retrieval. <!-- sdd-owner: implementation -->
- [ ] Add checkpoint tests and concrete persistence/adapters in `internal/app/checkpoints.go`, `internal/store/sqlite/`, `internal/adapter/cli/`, `internal/adapter/mcp/`, and `internal/testkit/`; run RED → GREEN → TRIANGULATE → REFACTOR for validated session binding, limits, exact revision references, freshness, explicit resume, disconnect, and fork distinction. <!-- sdd-owner: implementation -->
- [ ] Add cross-cutting tests and close only observed gaps in `internal/testkit/`, `internal/adapter/cli/`, `internal/adapter/mcp/`, `internal/store/sqlite/`, and `internal/revisionfs/`; run RED → GREEN → TRIANGULATE → REFACTOR, then record actual commands, results, and limitations in `docs/evidence/phase1-verification.md` without adding deferred features. <!-- sdd-owner: implementation -->

Phase 1 remains blocked by the Phase 0 exit record and separate approval. The next eligible implementation work is P0.2, subject to a new scoped parent instruction.

---

## P0.1 transaction rollback evidence correction

### Status consumed

- Native SDD status was authoritative and `apply: ready`; repository-local authority remained limited to `/Users/juanmotta/Desktop/personal/memgraphai`, with no `actionContext` warnings.
- The parent limited this correction to `p0-1-build-sqlite-probe` and the stated P0.1 files. No P0.2 schema/identity behavior, Phase 1 behavior, new dependency, commit, review lifecycle action, global tool, user-library access, or persistent process was introduced.
- Strict TDD was active with `go test ./...` available. The task workload forecast is an `ask-on-risk` concern only for the broader combined scope; this bounded P0.1 correction is within the explicit 2,000-line approval and has no chain or size exception.

### Completed implementation task and persisted checkbox

- [x] P0.1 correction — added newly executed behavioral transaction rollback evidence without changing or relabeling the original compile-only RED evidence.
  - The P0.1 implementation-owned row in `tasks.md` remains visibly checked `- [x]` after the fresh focused and full evidence below. It is the sole P0.1 task checkbox; no other task checkbox was changed.
  - Fresh evidence digest: `sha256:14dcf91760481fbdb353e789c728751b43b5c0b77330fa5c005b77e705709763` for `docs/evidence/phase0-build.md`.

### Files changed

- `internal/store/sqlite/probe.go`
- `internal/store/sqlite/probe_test.go`
- `docs/evidence/phase0-build.md`
- `openspec/changes/backend-foundation-and-continuity/apply-progress.md`

### Verification and strict TDD evidence

| Task | Test file | Layer | Safety net | RED | GREEN | TRIANGULATE | REFACTOR |
| --- | --- | --- | --- | --- | --- | --- | --- |
| P0.1 rollback correction | `internal/store/sqlite/probe_test.go` | SQLite integration | `go test ./internal/store/sqlite ./cmd/memgraphai` passed | `go test ./internal/store/sqlite -run '^TestProbeRollbackLeavesNoRowVisible$'` failed with actual assertion `rows visible after rollback = -1, want 0` | The same focused test passed after a real transaction inserted `rolled back`, called `Rollback`, and counted that value | `go test ./internal/store/sqlite -run '^TestProbe(ReportsFTS5AndCommittedTransaction|RollbackLeavesNoRowVisible|UsesTheRequestedFTS5Query)$'` passed with `TransactionValue == "committed"` and `RolledBackRows == 0` | `gofmt` completed; no further production refactor was necessary and the focused suite remained green |

The new executable assertion is exactly `result.RolledBackRows == 0`; it rejects a visible `rolled back` row. The contrasting committed assertion is exactly `result.TransactionValue == "committed"`. These are legitimate follow-up RED/GREEN/TRIANGULATE results, not a retroactive reinterpretation of P0.1's original undefined-symbol compilation failures.

| Command | Result |
| --- | --- |
| `go test ./internal/store/sqlite ./cmd/memgraphai` | PASS safety net before follow-up edits |
| `go test ./internal/store/sqlite -run '^TestProbeRollbackLeavesNoRowVisible$'` | expected RED failure: `Probe() rows visible after rollback = -1, want 0` |
| `gofmt -w internal/store/sqlite/probe.go internal/store/sqlite/probe_test.go && go test ./internal/store/sqlite -run '^TestProbeRollbackLeavesNoRowVisible$'` | PASS GREEN |
| `go test ./internal/store/sqlite -run '^TestProbe(ReportsFTS5AndCommittedTransaction|RollbackLeavesNoRowVisible|UsesTheRequestedFTS5Query)$'` | PASS TRIANGULATE |
| `go test ./...` | PASS: `cmd/memgraphai`, `internal/store/sqlite`, and `internal/testkit` |
| `go build -o /tmp/memgraphai-p0-1-rollback/memgraphai ./cmd/memgraphai && /tmp/memgraphai-p0-1-rollback/memgraphai probe` | PASS; printed `memgraphai: SQLite FTS5 probe succeeded (matches=1, transaction=committed)` |

### Original limitation, cleanup, and boundary

- The original evidence remains explicit: its `undefined: Probe` and `main undeclared` RED entries are compilation/setup failures, not behavioral RED evidence. This correction does not fabricate a prior behavioral failure or alter that history.
- `/tmp/memgraphai-p0-1-rollback` was removed after the process check and its absence was verified. Test databases were created through `t.TempDir()`; no user library was opened. No global tools were installed and no long-running process remained.
- No design deviation was made: the added probe is only a complementary P0.1 transaction behavior check. It does not assert filesystem durability, recovery, publication, schema, identity, authorization, FTS retrieval, or Phase 1 behavior.

### Workload and remaining work

- Boundary: one P0.1 correction only; no PR, commit, chained delivery, or size exception was created. The correction touches four artifact/source files and is well below the active 2,000-line review budget.
- The exact unchecked implementation task lines remain the unchanged list under `## Remaining implementation tasks` above. Phase 1 remains blocked by P0.4's Phase 0 exit evidence and separate approval.

---

## P0.2 identity constraints

### Status and authorization consumed

- The authoritative native status was `gentle-ai.sdd-status@2`, `apply: ready`, for `backend-foundation-and-continuity`; repository-local edits were authorized only under `/Users/juanmotta/Desktop/personal/memgraphai`, with no action-context warnings.
- The parent supplied the P0.2 proceed token `sha256:32e894ac50ea0618e137e4063f9257f2667221eed73e96046aab68ccfcb6767b`. This worker did not acquire, settle, or persist that token.
- The user authorized Phase 0 stepwise work only. No Phase 1 implementation, adapters, publication/recovery work, dependency, global install, commit, paid integration, or user-library access was added.
- Strict TDD was active. `go test ./...` was an existing passing safety net, and `openspec/config.yaml` now records the P0.1-confirmed Go runner rather than `unverified`.

### Completed task and persisted checkbox

- [x] P0.2 — Add identity constraint tests and the smallest concrete domain/SQLite behavior in `internal/domain/`, `internal/store/sqlite/`, and `internal/testkit/`, with evidence in `docs/evidence/phase0-identity.md`.
  - The persisted P0.2 implementation-owned row in `tasks.md` was changed from `- [ ]` to `- [x]` immediately after completion and re-read visibly checked.
  - Identity evidence SHA-256: `87e4a1a1fece4561d9d67f434057544a68f3940f24a409966413deed7e828219`.

### Files changed

- `internal/domain/identity.go`, `internal/domain/identity_test.go`
- `internal/store/sqlite/identity.go`, `internal/store/sqlite/identity_test.go`
- `internal/testkit/paths.go`
- `docs/evidence/phase0-identity.md`
- `openspec/config.yaml`
- `openspec/changes/backend-foundation-and-continuity/tasks.md`
- `openspec/changes/backend-foundation-and-continuity/apply-progress.md`

### TDD Cycle Evidence

| Task | Test file | Layer | Safety net | RED | GREEN | TRIANGULATE | REFACTOR |
| --- | --- | --- | --- | --- | --- | --- | --- |
| Explicit authorization | `internal/domain/identity_test.go` | Unit | `go test ./...` PASS before P0.2 | Executable failure: `AuthorizeProject() error = <nil>, want "scope_denied"` | Focused domain test PASS after explicit-grant check | The same test proves an unrelated grant is denied and a matching explicit grant succeeds | Typed stable outcome helper; focused test PASS |
| SQLite identity and binding | `internal/store/sqlite/identity_test.go` | SQLite integration | `go test ./...` PASS before P0.2 | Executable failures: immutable rename returned `<nil>`; alias resolved `none`; cross-project session creation returned `<nil>` | Focused identity suite PASS after schema and behavior | Covers exact alias/symlink one, nested no-prefix `none`, same-target many, moved unavailable/none, same names, cross-project, mismatched and valid bindings | Private workstream-project validator; focused suite PASS |

### Verification and cleanup

| Command | Result |
| --- | --- |
| `go test ./...` before P0.2 edits | PASS safety net |
| `go test ./internal/domain -run '^TestAuthorizeProjectDoesNotTreatAssociationAsPermission$'` | expected behavioral RED, then PASS GREEN |
| `go test ./internal/store/sqlite -run '^TestStorePreservesProjectIDsAndCase$'` | expected behavioral RED: immutable rename returned `<nil>` |
| `go test ./internal/store/sqlite -run '^(TestStorePreservesProjectIDsAndCase|TestResolvePathReportsOneManyNoneAndUnavailableWithoutGuessing|TestStoreRejectsCrossProjectAndMismatchedBindings)$'` | expected behavioral RED with three concrete assertions, then PASS GREEN |
| `go test ./internal/domain ./internal/store/sqlite -run '^(TestAuthorizeProjectDoesNotTreatAssociationAsPermission|TestStorePreservesProjectIDsAndCase|TestResolvePathReportsOneManyNoneAndUnavailableWithoutGuessing|TestStoreRejectsCrossProjectAndMismatchedBindings)$'` | PASS triangulation/refactor |
| `go test ./...` | PASS final suite |

Test databases and paths were allocated with `t.TempDir()` and cleaned by the Go test framework. The Go test commands created only short-lived test processes; no persistent process remained. No build directory, global installation, dependency download, or user library access occurred. The P0.2 implementation/evidence files contain 433 physical lines; with P0.1's reported 353 baseline lines, the observed Phase 0 subtotal remains below the user-authorized 2,000-line budget. No chain, PR, commit, or size exception was needed.

### Remaining implementation tasks

- [ ] Add fault-injection tests and the narrow filesystem/publication implementation in `internal/testkit/`, `internal/revisionfs/`, and `internal/store/sqlite/`; run RED → GREEN → TRIANGULATE → REFACTOR across prepare/sync/no-clobber, expected-revision rejection, pointer commit, response loss, first publication, replacement, and preparation failure, recording evidence in `docs/evidence/phase0-publication.md`. <!-- sdd-owner: implementation -->
- [ ] Add restart, checksum, orphan, takeover, and multi-process tests with the minimum recovery/ownership/retry implementation in `internal/testkit/`, `internal/store/sqlite/`, and `internal/revisionfs/`; run RED → GREEN → TRIANGULATE → REFACTOR for unknown commits, lost responses, altered/missing files, active staging, stale writers, simultaneous retries, idempotency mismatch, authorization recheck, committed replay after later revision advancement, and retry exhaustion, then record recovery, concurrency, and Phase 0 exit evidence. <!-- sdd-owner: implementation -->
- [ ] Resolve the permission, exact envelope/versioning, MCP library/build, pagination-token, and session-interruption contracts in `docs/evidence/phase1-contracts.md` only, documenting the future responsibilities of `internal/app/contract.go` and both adapters without writing source code; do not add ceremonial tests of prose completeness, and hand real failing contract-behavior cases to P1.1 after concrete decisions while preserving noninteractive errors and semantic parity. <!-- sdd-owner: implementation -->
- [ ] Add tests and concrete wiring in `go.mod`, `cmd/memgraph/main.go`, `internal/app/contract.go`, `internal/store/sqlite/`, `internal/telemetry/`, and `internal/testkit/`; run RED → GREEN → TRIANGULATE → REFACTOR for scope, outcomes, migration versioning, limits, required metrics, response bytes, and isolated metric failure. <!-- sdd-owner: implementation -->
- [ ] Add shared-service, CLI, and MCP tests plus concrete project/path operations in `internal/app/projects.go`, `internal/adapter/cli/`, `internal/adapter/mcp/`, and `internal/testkit/`; run RED → GREEN → TRIANGULATE → REFACTOR for identity, aliases, ambiguity/unavailability, nested/symlink paths, and noninteractive errors. <!-- sdd-owner: implementation -->
- [ ] Add continuity tests and concrete workstream/session persistence in `internal/app/continuity.go`, `internal/store/sqlite/`, `internal/adapter/cli/`, `internal/adapter/mcp/`, and `internal/testkit/`; run RED → GREEN → TRIANGULATE → REFACTOR for mismatches, missing sessions, explicit close, disconnect, interleaved logical MCP sessions, resume, and fork. <!-- sdd-owner: implementation -->
- [ ] Add document tests and concrete services in `internal/app/documents.go`, `internal/revisionfs/`, `internal/store/sqlite/`, `internal/adapter/cli/`, `internal/adapter/mcp/`, and `internal/testkit/`; run RED → GREEN → TRIANGULATE → REFACTOR for scope-only reads, history, stale writes, precommit failure, response loss, altered files, idempotent replay, and orphan reporting, without adding FTS retrieval. <!-- sdd-owner: implementation -->
- [ ] Add checkpoint tests and concrete persistence/adapters in `internal/app/checkpoints.go`, `internal/store/sqlite/`, `internal/adapter/cli/`, `internal/adapter/mcp/`, and `internal/testkit/`; run RED → GREEN → TRIANGULATE → REFACTOR for validated session binding, limits, exact revision references, freshness, explicit resume, disconnect, and fork distinction. <!-- sdd-owner: implementation -->
- [ ] Add cross-cutting tests and close only observed gaps in `internal/testkit/`, `internal/adapter/cli/`, `internal/adapter/mcp/`, `internal/store/sqlite/`, and `internal/revisionfs/`; run RED → GREEN → TRIANGULATE → REFACTOR, then record actual commands, results, and limitations in `docs/evidence/phase1-verification.md` without adding deferred features. <!-- sdd-owner: implementation -->

Phase 1 remains unapproved and blocked by P0.3/P0.4 evidence and the Phase 0 exit record. The next eligible implementation work is P0.3 only after a separately scoped parent instruction.

---

## P0.3 durable publication

### Status, authorization, and boundary

- Consumed the authoritative `gentle-ai.sdd-status@2` context: `apply: ready`, repository-local mode, workspace `/Users/juanmotta/Desktop/personal/memgraphai`, and that same allowed edit root, with no action-context warnings.
- The parent authorized only sequential Phase 0 work unit `p0-3-durable-publication`, supplied proceed-attempt `sha256:9535ab3229498f35a185bccadf4a8f8ec301cdb1d7d962d0ffe350ce80e6342d`, and retained all acquire/settle/token persistence ownership. No Phase 1 code, CLI/MCP behavior, commit, global installation, paid call, user library, or persistent process was used.
- Strict TDD was active with the verified `go test ./...` runner. The safety net `go test ./internal/domain ./internal/store/sqlite ./internal/testkit` passed before the P0.3 edits.

### Completed task and persisted checkbox

- [x] P0.3 — Add fault-injection tests and the narrow filesystem/publication implementation in `internal/testkit/`, `internal/revisionfs/`, and `internal/store/sqlite/`, with evidence in `docs/evidence/phase0-publication.md`.
  - The exact P0.3 implementation-owned row in `tasks.md` was changed to `- [x]` immediately after its coverage was complete; it must remain visibly checked when this record is read.
  - Publication evidence SHA-256: `d81ed40332afd1e2ddd1acd9f557c591b74b0a7e632a3c655e917f78e0958686`.

### Files changed

- `internal/revisionfs/publication.go`, `internal/revisionfs/publication_test.go`
- `internal/store/sqlite/identity.go`, `internal/store/sqlite/identity_test.go`, `internal/store/sqlite/publication.go`, `internal/store/sqlite/publication_test.go`
- `internal/domain/identity.go`, `internal/testkit/faults.go`
- `docs/evidence/phase0-publication.md`
- `openspec/changes/backend-foundation-and-continuity/tasks.md`
- `openspec/changes/backend-foundation-and-continuity/apply-progress.md`

### TDD Cycle Evidence

| Task | Test file | Layer | Safety net | RED | GREEN | TRIANGULATE | REFACTOR |
| --- | --- | --- | --- | --- | --- | --- | --- |
| Durable Markdown preparation | `internal/revisionfs/publication_test.go` | Filesystem integration | N/A (new) | `TestPreparePublishesImmutableMarkdown` failed with the executable `revision preparation is not implemented` error | Focused test passed after staged sync, parent/revision directory sync, immutable link publication, and metadata creation | Missing root, checksum/bytes, no-clobber original preservation, unsafe IDs, and pre-publication fault all pass | Narrow `safeComponent`, `ensureDir`, and `syncDir`; focused suite remains green |
| SQLite foreign-key coverage | `internal/store/sqlite/identity_test.go` | SQLite integration | Existing focused suite passed | Direct invalid SQL on a second usable connection returned nil, failing the explicit constraint assertion | DSN connection pragma made the focused direct-SQL constraint test pass | Two concurrently acquired pool connections prove SQLite, not Go prechecks, rejects the invalid foreign key | `foreignKeyDSN` centralizes the connection setting; focused suite remains green |
| Pointer publication | `internal/store/sqlite/publication_test.go` | SQLite/filesystem integration | Existing focused suite passed | First call failed with executable `CreateDocument() error = document publication is not implemented` | First prepared revision became current after the transaction committed | Valid replacement, stale conflict, pre-commit interruption retaining prior current, and post-commit response loss returning `unknown` all pass | `matchesExpectedCurrent` isolates null-versus-exact expected revision handling; focused suite remains green |

### Verification, process, and cleanup evidence

| Command | Result |
| --- | --- |
| `go test ./internal/revisionfs -run '^TestPreparePublishesImmutableMarkdown$'` | expected RED, then PASS GREEN |
| `go test ./internal/store/sqlite -run '^TestStoreRejectsForeignKeyViolationsOnEveryUsableConnection$'` | expected RED, then PASS GREEN |
| `go test ./internal/store/sqlite -run '^TestPublishRevisionMakesFirstPreparedRevisionCurrent$'` | expected RED, then PASS GREEN |
| `go test ./internal/revisionfs -run '^TestPrepare'` | PASS triangulation/refactor |
| `go test ./internal/store/sqlite -run '^(TestPublishRevision|TestStoreRejectsForeignKeyViolationsOnEveryUsableConnection$)'` | PASS triangulation/refactor |
| `go test ./...` | PASS final suite |

All SQLite and Markdown fixtures used `t.TempDir()` and were cleaned by the test framework. Only short-lived `go test` processes ran; no user library was opened, no persistent process remained, and no external build directory was created.

### Design limits and workload

- The P0.3 prepared path intentionally has an operation-ID staging directory but no ownership-generation or operation ledger. That fencing, replay, recovery, orphan, checksum-read, and contention work remains exclusively P0.4; the P0.3 `unknown` response-loss result does not claim safe replay or rollback.
- Returned-error fault injection exercises code-path ordering only. It is not power-loss, filesystem-crash-consistency, platform-support, or production-durability evidence.
- Live source/test lines for completed Phase 0 work are 1,108; completed evidence files total 175 lines, for 1,283 lines before this progress artifact. Applying P0.4's task estimate of 230–430 changed lines forecasts 1,513–1,713 total Phase 0 implementation/evidence lines. This remains below the authorized 2,000-line Phase 0 budget, although it leaves a 287–487 line buffer and should be reforecast before P0.4 expands scope. No chain, PR, commit, or size exception was selected.

### Remaining implementation tasks

- [ ] Add restart, checksum, orphan, takeover, and multi-process tests with the minimum recovery/ownership/retry implementation in `internal/testkit/`, `internal/store/sqlite/`, and `internal/revisionfs/`; run RED → GREEN → TRIANGULATE → REFACTOR for unknown commits, lost responses, altered/missing files, active staging, stale writers, simultaneous retries, idempotency mismatch, authorization recheck, committed replay after later revision advancement, and retry exhaustion, then record recovery, concurrency, and Phase 0 exit evidence. <!-- sdd-owner: implementation -->
- [ ] Resolve the permission, exact envelope/versioning, MCP library/build, pagination-token, and session-interruption contracts in `docs/evidence/phase1-contracts.md` only, documenting the future responsibilities of `internal/app/contract.go` and both adapters without writing source code; do not add ceremonial tests of prose completeness, and hand real failing contract-behavior cases to P1.1 after concrete decisions while preserving noninteractive errors and semantic parity. <!-- sdd-owner: implementation -->
- [ ] Add tests and concrete wiring in `go.mod`, `cmd/memgraph/main.go`, `internal/app/contract.go`, `internal/store/sqlite/`, `internal/telemetry/`, and `internal/testkit/`; run RED → GREEN → TRIANGULATE → REFACTOR for scope, outcomes, migration versioning, limits, required metrics, response bytes, and isolated metric failure. <!-- sdd-owner: implementation -->
- [ ] Add shared-service, CLI, and MCP tests plus concrete project/path operations in `internal/app/projects.go`, `internal/adapter/cli/`, `internal/adapter/mcp/`, and `internal/testkit/`; run RED → GREEN → TRIANGULATE → REFACTOR for identity, aliases, ambiguity/unavailability, nested/symlink paths, and noninteractive errors. <!-- sdd-owner: implementation -->
- [ ] Add continuity tests and concrete workstream/session persistence in `internal/app/continuity.go`, `internal/store/sqlite/`, `internal/adapter/cli/`, `internal/adapter/mcp/`, and `internal/testkit/`; run RED → GREEN → TRIANGULATE → REFACTOR for mismatches, missing sessions, explicit close, disconnect, interleaved logical MCP sessions, resume, and fork. <!-- sdd-owner: implementation -->
- [ ] Add document tests and concrete services in `internal/app/documents.go`, `internal/revisionfs/`, `internal/store/sqlite/`, `internal/adapter/cli/`, `internal/adapter/mcp/`, and `internal/testkit/`; run RED → GREEN → TRIANGULATE → REFACTOR for scope-only reads, history, stale writes, precommit failure, response loss, altered files, idempotent replay, and orphan reporting, without adding FTS retrieval. <!-- sdd-owner: implementation -->
- [ ] Add checkpoint tests and concrete persistence/adapters in `internal/app/checkpoints.go`, `internal/store/sqlite/`, `internal/adapter/cli/`, `internal/adapter/mcp/`, and `internal/testkit/`; run RED → GREEN → TRIANGULATE → REFACTOR for validated session binding, limits, exact revision references, freshness, explicit resume, disconnect, and fork distinction. <!-- sdd-owner: implementation -->
- [ ] Add cross-cutting tests and close only observed gaps in `internal/testkit/`, `internal/adapter/cli/`, `internal/adapter/mcp/`, `internal/store/sqlite/`, and `internal/revisionfs/`; run RED → GREEN → TRIANGULATE → REFACTOR, then record actual commands, results, and limitations in `docs/evidence/phase1-verification.md` without adding deferred features. <!-- sdd-owner: implementation -->

P0.4 is the sole next Phase 0 work unit. Phase 1 remains unapproved and blocked by P0.4's Phase 0 exit record.

---

## P0.4 Phase 0 exit — blocked before implementation

### Status and action context consumed

- Consumed the authoritative native `gentle-ai.sdd-status@2` context: `apply: ready`, repository-local mode, workspace `/Users/juanmotta/Desktop/personal/memgraphai`, that same allowed edit root, and no `actionContext` warnings.
- The parent scoped work to `p0-4-recovery-concurrency` and holds the proceed token `sha256:c5fea2bc950c885fc48607ecc0b2db825f7ec42ff8e41f1b710c82f756dfcbba`; this worker did not acquire, settle, or persist it. Phase 1 remains unapproved and untouched.
- Strict TDD was active with the verified `go test ./...` runner. The completed-suite safety net passed before any P0.4 source or test edit.

### Budget decision and persisted task state

- No P0.4 implementation was started. The exact P0.4 implementation-owned task remains visibly unchecked in `tasks.md`; no checkbox was changed because no behavior completed.
- The measured completed Phase 0 source/evidence baseline is 1,283 physical lines. A conservative minimum P0.4 forecast is 890 additional lines: 250 for ledger/fingerprint/outcome/fencing, 190 for generation staging/integrity/orphan behavior, 300 for real reopen/subprocess and separate-process tests, and 150 for recovery/concurrency/exit evidence. That is a 2,173-line candidate before this progress update, above the explicit 2,000-line limit.
- Under the configured `ask-on-risk` delivery policy, implementation stopped rather than omit process evidence, compress tests, or weaken the corrected idempotency and fencing invariants. `docs/evidence/phase0-exit.md` records the blocking exit; it is not a viability claim.

### Files changed

- `docs/evidence/phase0-exit.md`
- `openspec/changes/backend-foundation-and-continuity/apply-progress.md`

### Verification, TDD, process, and cleanup facts

| Command | Result |
| --- | --- |
| `go test ./...` | PASS safety net: `cmd/memgraphai`, `internal/domain`, `internal/revisionfs`, `internal/store/sqlite`, and `internal/testkit` |
| Phase 0 physical-line count | 1,283 before P0.4 implementation |
| `find /tmp -maxdepth 1 -type d -name 'memgraphai-p0-4-*'` | no matching directory |

| Task | Test file | Layer | Safety net | RED | GREEN | TRIANGULATE | REFACTOR |
| --- | --- | --- | --- | --- | --- | --- | --- |
| P0.4 recovery/concurrency | None; budget gate before behavior work | N/A | `go test ./...` PASS | Not started: source/test edits would exceed the forecast gate | Not started | Not started | Not started |

Only short-lived Go test processes ran. No P0.4 subprocess, forced termination, reopen fixture, fault injection, temporary P0.4 directory, dependency download, user-library access, persistent process, global tool, commit, paid/live call, or Phase 1 source change occurred. No design deviation was made; the block preserves the task's required same-transaction operation result, reauthorization, fencing, integrity, orphan, and cross-process requirements.

### Remaining implementation task

- [ ] Add restart, checksum, orphan, takeover, and multi-process tests with the minimum recovery/ownership/retry implementation in `internal/testkit/`, `internal/store/sqlite/`, and `internal/revisionfs/`; run RED → GREEN → TRIANGULATE → REFACTOR for unknown commits, lost responses, altered/missing files, active staging, stale writers, simultaneous retries, idempotency mismatch, authorization recheck, committed replay after later revision advancement, and retry exhaustion, then record recovery, concurrency, and Phase 0 exit evidence. <!-- sdd-owner: implementation -->

A maintainer must choose a delivery path that admits this cohesive work unit (such as a chained work-unit authorization) or explicitly accept `size:exception`; until then Phase 1 remains blocked.

---

## P0.4 recovery, integrity, and concurrency

### Status, authorization, and scope

- Consumed native `gentle-ai.sdd-status@2`: `apply: ready`, repository-local workspace `/Users/juanmotta/Desktop/personal/memgraphai`, the same allowed edit root, and no action-context warnings.
- The user explicitly authorized only P0.4 work unit `p0-4-recovery-concurrency` on baseline `39d8bee`, with a 2,000-line **incremental** P0.4 budget. The parent owns proceed token `sha256:0c5abcfe9f7907877c2994bb0c42253bae96e09e991deb33dfc025e3b5ffde79` and remediation/settlement; this worker neither acquired, settled, nor persisted a token.
- Phase 1 remains unapproved and untouched. No adapter, daemon, authorization framework, external data, global install/service, commit, push, paid call, or user library was used.

### Completed task and persisted checkbox

- [x] P0.4 — Add recovery, integrity, ownership, restart, and multi-process evidence.
  - The exact P0.4 implementation-owned row in `tasks.md` was changed to `- [x]` immediately after passing focused and full checks, and was re-read visibly checked.
  - Fresh Phase 0 exit evidence SHA-256: `f5b75ecd0078df899c74afc26a5ecb514fb78cb4165b618d5322df8f48f3b472`.

### Files changed

- `internal/domain/identity.go`
- `internal/revisionfs/publication.go`, `internal/revisionfs/publication_test.go`
- `internal/store/sqlite/identity.go`, `internal/store/sqlite/recovery.go`, `internal/store/sqlite/recovery_test.go`, `internal/store/sqlite/recovery_process_test.go`
- `docs/evidence/phase0-recovery.md`, `docs/evidence/phase0-concurrency.md`, `docs/evidence/phase0-exit.md`
- `openspec/changes/backend-foundation-and-continuity/tasks.md`, `openspec/changes/backend-foundation-and-continuity/apply-progress.md`

### Strict TDD evidence

| Cycle | RED | GREEN | TRIANGULATE / REFACTOR |
| --- | --- | --- | --- |
| Durable operation replay | `go test ./internal/store/sqlite -run '^TestOperationReplayReturnsTheDurableTerminalResult$'` failed an executable assertion: `replayed outcome = "unknown", want committed`, with a temporary compilable test-only stub. | Replaced that stub with the SQLite ledger/fingerprint/result publication implementation; focused test passed. | Replay after later current advancement, mismatched fingerprint, denied replay, and post-commit response loss are focused assertions; `gofmt` and focused/full suites passed. |
| Filesystem retry/integrity | Existing publication tests were the safety net. | Generation-specific staging, no-clobber verification, targeted checksum checks, orphan reporting, and symlink boundaries passed. | Contrasts cover post-link/pre-directory-sync retry, altered history, missing current, and root/intermediate symlinks. |
| Ownership/process contention | Existing focused store tests were the safety net. | A forced child-process termination followed by reopen commits only as a new generation. | Separate test-binary process claims the same operation; a second claim succeeds as generation two and configured exhaustion returns bounded `retryable`, then every child is killed and waited. |

### Verification and limitations

| Command | Result |
| --- | --- |
| `go test ./internal/store/sqlite -run '^TestOperationReplayReturnsTheDurableTerminalResult$'` | Expected behavioral RED, then PASS GREEN. |
| `go test ./internal/store/sqlite -run '^(TestOperation|TestRecovery|TestTargeted)' -count=1` | PASS. |
| `go test ./internal/revisionfs -run '^(TestPrepareAttemptRetriesAfterLinkWithoutClobbering|TestPrepareRejectsSymlinkedRootAndIntermediateBoundaries)$' -count=1` | PASS. |
| `go test ./internal/store/sqlite -run '^TestRecoverySurvivesTerminatedOwnerAndReopen$' -count=1` | PASS. |
| `go test ./internal/store/sqlite -run '^TestRecoveryBoundsCrossProcessRetryExhaustion$' -count=1` | PASS. |
| `go test ./internal/revisionfs ./internal/store/sqlite -count=1` | PASS. |
| `go test ./... -count=1` | PASS. |

The filesystem sync and named-fault tests are not power-loss, crash-consistency, backup, network-filesystem, or platform-support guarantees. The operation fingerprint is an opaque Phase 0 equality value, and the authorization callback is not a permission framework.

### Workload, cleanup, and exit

- Incremental P0.4 source/tests/evidence/task-checkbox changes are 864 added and 34 deleted lines before this progress section, for 898 changed lines. This 60-line P0.4 progress record brings the current incremental total to 924 added plus 34 deleted lines (958 changed), under the user-authorized 2,000-line budget. The first reviewed 22-file/1,407-line baseline was not reopened.
- The first review's nonblocking future advisories were addressed locally: root/intermediate symlinks are rejected; post-link/pre-directory-sync retry verifies instead of clobbering; unknown commit replay reads the ledger rather than assuming idempotency.
- Test fixtures use `t.TempDir()`. Process tests explicitly `Kill` and `Wait` children; `/tmp` has no `memgraphai-p0-4-*` directory, no helper process remains, and no persistent service was started. No design deviation was made.
- `docs/evidence/phase0-exit.md` now records a viable tested local Phase 0 foundation. Phase 1 remains separately unapproved; it is not started by this result.

### Remaining implementation tasks

- [ ] Resolve the permission, exact envelope/versioning, MCP library/build, pagination-token, and session-interruption contracts in `docs/evidence/phase1-contracts.md` only, documenting the future responsibilities of `internal/app/contract.go` and both adapters without writing source code; do not add ceremonial tests of prose completeness, and hand real failing contract-behavior cases to P1.1 after concrete decisions while preserving noninteractive errors and semantic parity. <!-- sdd-owner: implementation -->
- [ ] Add tests and concrete wiring in `go.mod`, `cmd/memgraph/main.go`, `internal/app/contract.go`, `internal/store/sqlite/`, `internal/telemetry/`, and `internal/testkit/`; run RED → GREEN → TRIANGULATE → REFACTOR for scope, outcomes, migration versioning, limits, required metrics, response bytes, and isolated metric failure. <!-- sdd-owner: implementation -->
- [ ] Add shared-service, CLI, and MCP tests plus concrete project/path operations in `internal/app/projects.go`, `internal/adapter/cli/`, `internal/adapter/mcp/`, and `internal/testkit/`; run RED → GREEN → TRIANGULATE → REFACTOR for identity, aliases, ambiguity/unavailability, nested/symlink paths, and noninteractive errors. <!-- sdd-owner: implementation -->
- [ ] Add continuity tests and concrete workstream/session persistence in `internal/app/continuity.go`, `internal/store/sqlite/`, `internal/adapter/cli/`, `internal/adapter/mcp/`, and `internal/testkit/`; run RED → GREEN → TRIANGULATE → REFACTOR for mismatches, missing sessions, explicit close, disconnect, interleaved logical MCP sessions, resume, and fork. <!-- sdd-owner: implementation -->
- [ ] Add document tests and concrete services in `internal/app/documents.go`, `internal/revisionfs/`, `internal/store/sqlite/`, `internal/adapter/cli/`, `internal/adapter/mcp/`, and `internal/testkit/`; run RED → GREEN → TRIANGULATE → REFACTOR for scope-only reads, history, stale writes, precommit failure, response loss, altered files, idempotent replay, and orphan reporting, without adding FTS retrieval. <!-- sdd-owner: implementation -->
- [ ] Add checkpoint tests and concrete persistence/adapters in `internal/app/checkpoints.go`, `internal/store/sqlite/`, `internal/adapter/cli/`, `internal/adapter/mcp/`, and `internal/testkit/`; run RED → GREEN → TRIANGULATE → REFACTOR for validated session binding, limits, exact revision references, freshness, explicit resume, disconnect, and fork distinction. <!-- sdd-owner: implementation -->
- [ ] Add cross-cutting tests and close only observed gaps in `internal/testkit/`, `internal/adapter/cli/`, `internal/adapter/mcp/`, `internal/store/sqlite/`, and `internal/revisionfs/`; run RED → GREEN → TRIANGULATE → REFACTOR, then record actual commands, results, and limitations in `docs/evidence/phase1-verification.md` without adding deferred features. <!-- sdd-owner: implementation -->

---

## P0.4 parent-gate correction

### Status, authority, and persisted task state

- Consumed the authoritative native status: `apply: ready`, repository-local workspace `/Users/juanmotta/Desktop/personal/memgraphai`, that workspace as the only allowed edit root, and no action-context warnings. P0.4 is the sole user-authorized scope; Phase 1 remains unapproved and untouched.
- The parent supplied proceed-attempt `sha256:5f4c601df066b5582dd9c9b3a19dbc7b8450e38931aac6b68a1ab3224b57fa9d` and owns settlement binding remediation `sha256:f5b75ecd0078df899c74afc26a5ecb514fb78cb4165b618d5322df8f48f3b472`. This worker did not acquire, settle, or persist either token.
- The P0.4 implementation row was already visibly `- [x]`; it remains visibly checked after this correction. No other task checkbox was changed. The exact unchecked Phase 1 implementation rows remain immediately above unchanged.

### Corrected behavior

- The conflict branch now commits the terminal operation row before returning. Reopen and replay preserve `conflict` after a later pointer advance.
- SQLite stores a canonical SHA-256 digest of immutable request identity: external fingerprint, caller project, document, revision, explicit-null-or-value expected current, and Markdown digest. Retry limits and rendering controls are excluded. Semantic reuse with an unchanged external fingerprint returns `idempotency_mismatch` before preparation, including pending operations.
- A new operation validates that the caller project owns the document in the claim transaction before registering or preparing a filesystem path; an unrelated project receives `binding_mismatch`.
- Actual helper-process kills now cover prepared-final-file/pre-DB-commit and DB-committed/pre-response boundaries. The former reopens as conservative `unknown` with the prior current revision; the latter reopens as persisted committed replay, including after later pointer advancement. These are separate from named filesystem fault tests.

### TDD Cycle Evidence

| Task | Test file | Layer | Safety net | RED | GREEN | TRIANGULATE | REFACTOR |
| --- | --- | --- | --- | --- | --- | --- | --- |
| Durable conflict, canonical request identity, and caller-project binding | `internal/store/sqlite/recovery_test.go` | SQLite integration | `go test ./... -count=1` passed before edits | Focused command failed with durable `unknown`, five same-fingerprint semantic reuses accepted, and wrong-project write accepted | Same focused command passed after commit, canonical digest, and ownership validation | Covers conflict after reopen/later advance; project, document, revision, expected-current, and Markdown changes; plus a distinct wrong-project request | `gofmt`; focused SQLite suite passed with no speculative abstraction |
| Real kill recovery boundaries | `internal/store/sqlite/recovery_process_test.go` | Separate-process SQLite integration | Existing process suite passed before edits | Focused command failed: prepared retry committed as generation 1 and committed boundary recovered `unknown` | Same focused command passed after real helper boundary orchestration | Contrasts precommit prior-current/unknown with postcommit persisted replay after later advance | `gofmt`; full SQLite and repository suites passed |

### Verification and cleanup

| Command | Result |
| --- | --- |
| `go test ./... -count=1` before correction | PASS safety net |
| `go test ./internal/store/sqlite -run '^(TestOperationPersistsConflictForRecoveryAndReplayAfterReopen|TestOperationRejectsSemanticMismatchBeforePreparation|TestOperationRejectsCallerProjectThatDoesNotOwnDocumentBeforePreparation)$' -count=1` | Expected behavioral RED: conflict reopened as `unknown`; all five semantic reuses and wrong-project write were accepted |
| same focused command after GREEN | PASS |
| `go test ./internal/store/sqlite -run '^(TestRecoveryProcessPreparedFinalFilePreservesPriorCurrentUntilReopen|TestRecoveryProcessCommittedBeforeResponseReplaysAfterLaterAdvance)$' -count=1` | Expected behavioral RED: resumed prepared operation was generation 1; committed boundary reopened as `unknown` |
| same process command after GREEN | PASS |
| `go test ./internal/store/sqlite -run '^(TestOperation|TestRecovery|TestTargeted)' -count=1` | PASS triangulation/refactor |
| `go test ./internal/revisionfs -count=1` | PASS safety net for untouched filesystem implementation |
| `go test ./internal/store/sqlite -count=1` | PASS |
| `go test ./... -count=1` | PASS final suite |
| `git diff --check` | PASS |

All fixtures use `t.TempDir()`. Each new helper was explicitly killed and waited; process inspection found no `RecoveryProcessHelper` process and `/tmp` contained no `memgraphai-p0-4-*` directory. No daemon, global tool, dependency change, commit, external/paid call, or user-library access occurred.

### Evidence, workload, and limitations

- Fresh exit evidence SHA-256: `81fe4010283df41fa8b71049295aa2901a14830ec02fff01d99209ecd4c9b6b4` for `docs/evidence/phase0-exit.md`.
- This bounded correction changes only the authorized P0.4 store/tests/evidence/progress surfaces and remains under the 2,000-line incremental budget when added to the prior 958-line P0.4 record. No PR, chain, commit, or size exception was created.
- The canonical digest is an internal P0.4 equality invariant, not a public operation envelope or Phase 1 permission configuration. Local `Sync`, named faults, and process kills do not prove power-loss, network filesystem, platform-support, daemon, adapter, or production authorization behavior.

---

## P1.1 shared foundation and metrics

### Status, authority, and completed task

- Consumed the authoritative native status for `backend-foundation-and-continuity`: `apply: ready`, repo-local workspace `/Users/juanmotta/Desktop/personal/memgraphai`, that workspace as the allowed edit root, and no action-context warnings.
- The user authorized only the P1.1 shared-foundation/metrics review block under 1,600 changed lines. The parent owns the supplied compact proceed token and all settlement; this worker did not acquire, settle, or persist a token/counter.
- The P1.1 implementation-owned task is complete. Its persisted row in `tasks.md` was changed immediately to `- [x]`, and its historical `cmd/memgraph/main.go` spelling was corrected to the settled canonical `cmd/memgraphai/main.go` without changing the existing probe command.

### Files changed

- `internal/app/contract.go`, `internal/app/contract_test.go`
- `internal/store/sqlite/migrations.go`, `internal/store/sqlite/migrations_test.go`, `internal/store/sqlite/identity.go`
- `internal/telemetry/operation.go`
- `docs/evidence/phase1-foundation.md`
- `openspec/changes/backend-foundation-and-continuity/tasks.md`, `openspec/changes/backend-foundation-and-continuity/apply-progress.md`

### Strict TDD and verification

| Cycle | RED | GREEN | TRIANGULATE / REFACTOR |
| --- | --- | --- | --- |
| Contract/metrics | `TestRedMetricFailureDoesNotChangeBusinessOutcome` failed behaviorally with `internal`, want `ok`. | The experimental envelope service kept `ok` despite recorder failure and recorded its exact serialized JSON byte length. | Unknown fields/version, incompatible scopes, limit bounds/defaults, and an actual app-service-to-SQLite metric record pass. |
| Migration ledger | `TestOpenRecordsFoundationMigrationVersion` failed behaviorally because `schema_migrations` did not exist. | Transactional v1 application and future-version rejection pass. | Reopen preserves Phase 0 data, has one version row, retains the persisted bounded metric, and keeps an unavailable count NULL. |
| Refactor | Existing Phase 0 suites were the safety net. | Schema creation moved into the ordered migration unit without changing established tables. | `gofmt`, focused tests, full tests, and diff whitespace validation passed. |

| Command | Result |
| --- | --- |
| `go test ./... -count=1` before edits | PASS safety net |
| `go test ./internal/app -run '^TestRedMetricFailureDoesNotChangeBusinessOutcome$' -count=1` | Expected behavioral RED |
| `go test ./internal/store/sqlite -run '^TestOpenRecordsFoundationMigrationVersion$' -count=1` | Expected behavioral RED |
| `go test ./internal/app ./internal/store/sqlite -run '^(TestExecuteJSON|TestOpen)' -count=1` | PASS focused GREEN/TRIANGULATE |
| `go test ./internal/store/sqlite -run '^TestOpenAppliesMigrationOnceAndPersistsBoundedMetric$' -count=1` | PASS actual app-to-SQLite metric path |
| `go test ./... -count=1` | PASS final |
| `git diff --check` | PASS |

The SHA-256 for `docs/evidence/phase1-foundation.md` is recorded in the final apply report. Fixtures use `t.TempDir()`; no user library, persistent process, dependency installation, commit, push, paid call, or external integration occurred.

### Boundary, workload, and remaining work

- This cohesive review block is below the user-authorized 1,600-line limit and remains below the 2,000-line total with P1.0's 159-line contract record. No PR, commit, chain settlement, or size exception was created.
- No CLI behavior beyond the existing probe, CLI/MCP adapter, MCP SDK, project/path service, workstream/session behavior, documents, checkpoints, FTS retrieval, embeddings, daemon, SDK integration, or token/view-revision issuance was added. Adapter parity and final MCP serialization remain P1.2+ work; application-envelope byte accounting is the only measured response serialization here.
- Exact remaining unchecked implementation rows:
  - [ ] Add shared-service, CLI, and MCP tests plus concrete project/path operations in `internal/app/projects.go`, `internal/adapter/cli/`, `internal/adapter/mcp/`, and `internal/testkit/`; run RED → GREEN → TRIANGULATE → REFACTOR for identity, aliases, ambiguity/unavailability, nested/symlink paths, and noninteractive errors. <!-- sdd-owner: implementation -->
  - [ ] Add continuity tests and concrete workstream/session persistence in `internal/app/continuity.go`, `internal/store/sqlite/`, `internal/adapter/cli/`, `internal/adapter/mcp/`, and `internal/testkit/`; run RED → GREEN → TRIANGULATE → REFACTOR for mismatches, missing sessions, explicit close, disconnect, interleaved logical MCP sessions, resume, and fork. <!-- sdd-owner: implementation -->
  - [ ] Add document tests and concrete services in `internal/app/documents.go`, `internal/revisionfs/`, `internal/store/sqlite/`, `internal/adapter/cli/`, `internal/adapter/mcp/`, and `internal/testkit/`; run RED → GREEN → TRIANGULATE → REFACTOR for scope-only reads, history, stale writes, precommit failure, response loss, altered files, idempotent replay, and orphan reporting, without adding FTS retrieval. <!-- sdd-owner: implementation -->
  - [ ] Add checkpoint tests and concrete persistence/adapters in `internal/app/checkpoints.go`, `internal/store/sqlite/`, `internal/adapter/cli/`, `internal/adapter/mcp/`, and `internal/testkit/`; run RED → GREEN → TRIANGULATE → REFACTOR for validated session binding, limits, exact revision references, freshness, explicit resume, disconnect, and fork distinction. <!-- sdd-owner: implementation -->
  - [ ] Add cross-cutting tests and close only observed gaps in `internal/testkit/`, `internal/adapter/cli/`, `internal/adapter/mcp/`, `internal/store/sqlite/`, and `internal/revisionfs/`; run RED → GREEN → TRIANGULATE → REFACTOR, then record actual commands, results, and limitations in `docs/evidence/phase1-verification.md` without adding deferred features. <!-- sdd-owner: implementation -->

---

## P1.1 gatekeeper correction: nil handler

- Consumed the parent status: `apply: ready`, repo-local authority at `/Users/juanmotta/Desktop/personal/memgraphai`, same allowed root, and no action-context warning. This is the existing separate P1.1 corrective review block; no P1.2 scope, commit, push, or token action was taken. The parent alone owns settlement of `sha256:1b5646acde9335f8f9224733bc1e0067a05ca325f8e6e944598b76f220ec8e3a`.
- New genuine production RED: `go test ./internal/app -run '^TestExecuteJSONNilHandlerReturnsInternalAndAttemptsMetrics$' -count=1` panicked at `internal/app/contract.go:101`. GREEN returns `internal` for a nil handler and still makes one metrics attempt; no blanket recovery was added. Triangulation passes absent input versus `{}`, missing/blank operation IDs, unsupported-version server-envelope echo, and unknown page fields.
- Commands: safety `go test ./internal/app -count=1` PASS; GREEN focused test PASS; triangulation `go test ./internal/app -run '^TestExecuteJSONTriangulatesEnvelopeEdges$' -count=1` PASS; refactor `gofmt -w internal/app/contract.go internal/app/contract_test.go && go test ./internal/app -count=1` PASS; final `go test ./... -count=1` and `git diff --check` PASS.
- Changed: `internal/app/contract.go`, `internal/app/contract_test.go`, `docs/evidence/phase1-foundation.md`, `tasks.md`, and this progress record. Corrected evidence SHA-256: `41bafb442bbc672775228f8f11b8d3b1a44a31d5ab11d1c5952cf6e9aee126c8`.
- `docs/evidence/phase1-foundation.md` now records that the original metric-isolation RED was TESTONLY and does not satisfy strict production TDD. The original P1.1 row is restored to visibly unchecked: this genuine panic RED does not retroactively cure that historical gap, so human exception/acceptance is required before P1.1 can be complete. No design deviation occurred.
- Test-only execution created no durable fixtures, build output, user-library access, global tool, dependency, persistent process, commit, push, paid call, or external integration. The P1.2–P1.6 exact unchecked rows remain unchanged immediately above; the additional exact P1.1 row is:
  - [ ] Add tests and concrete wiring in `go.mod`, `cmd/memgraphai/main.go`, `internal/app/contract.go`, `internal/store/sqlite/`, `internal/telemetry/`, and `internal/testkit/`; run RED → GREEN → TRIANGULATE → REFACTOR for scope, outcomes, migration versioning, limits, required metrics, response bytes, and isolated metric failure. <!-- sdd-owner: implementation -->

---

## P1.1 documentation reconciliation

- The user explicitly accepted the original TESTONLY metric-isolation RED deficiency as a narrow historical exception. It remains invalid as production-behavior RED and does not waive future strict TDD.
- P1.1 implementation is therefore accepted and its implementation-owned marker is checked. This supersedes only the earlier pending-exception task-state interpretation; no historical failed note is erased or relabeled.
- The nil-handler regression independently produced genuine production RED then GREEN. A prior verifier also observed the full suite passing, the nil-handler case passing 20 consecutive runs, and edge triangulation passing 10 consecutive runs.
- Those results are historical evidence, not fresh verification from this documentation-only reconciliation. Fresh verification and native review remain pending with the parent; neither full Phase 1 completion nor review closure is claimed.
- Reconciled evidence SHA-256: `6c34aea9b52e71cb0c614d5d58cd84ce1fdd2ba91d800e9344d72283e5585160` for `docs/evidence/phase1-foundation.md`.
- No source, test, dependency, review-lifecycle, commit, or settlement operation was performed during this reconciliation.

---

## P1.2 project/CLI/MCP partial implementation

### Status and boundary

- Consumed the authoritative `apply: ready` status for `backend-foundation-and-continuity`, repository-local `/Users/juanmotta/Desktop/personal/memgraphai` authority, the same allowed root, and no action-context warnings.
- The user authorized only P1.2 on baseline `267aa72a743dca232425f900a6c15330900282bc`, with the parent retaining every native SDD token/counter action. No commit, push, review action, global install, service, paid integration, user-library test access, or child agent was used.
- `docs/evidence/phase1-projects.md` documents the experimental operations and CLI surface before behavior work. Its SHA-256 is `4b83594abbd01a59c4aba20369d0421f2f6c1c1c19ffec630117375065ce4ed1`.

### Implemented and verified subset

- Added a concrete shared `ProjectService`, SQLite project list-view migration, create/list/resolve and association add/list/remove paths, CLI command normalization, and official `github.com/modelcontextprotocol/go-sdk/mcp` v1.7.0 stdio tool server.
- The first real RED was `go test ./internal/app -run '^TestProjectServiceCreatePersistsImmutableIdentity$' -count=1`, which failed `Handle(project.create) outcome = "internal", want "ok"`; GREEN passed after the shared service persisted the project through SQLite.
- The association/alias RED was `go test ./internal/app -run '^TestProjectServicePreservesExplicitAssociationAndResolutionOutcomes$' -count=1`, failing `first project list outcome = "internal", want "ok"`; GREEN passed with shared project operations. It contrasts duplicate same association, alias resolution, multiple project association ambiguity, and exact missing removal.
- The CLI RED was `go test ./internal/adapter/cli -run '^TestHumanCreateBuildsTheSharedJSONEnvelope$' -count=1`, failing with exit code 1 for the documented `--id` command. GREEN maps that human flag to the shared `project_id` input.
- The built-process MCP RED was `go test ./internal/adapter/mcp -run '^TestStdioServerListsAndCallsProjectTool$' -count=1`, returning SDK tool error `validating /properties/input ... want one of null, array`. GREEN uses a typed map input and passes a real `NewClient`/`CommandTransport` connection that lists six tools and calls `project.create` against a temporary library.

### Verification and limitations

| Command | Result |
| --- | --- |
| focused app, CLI, and MCP RED commands above | Expected behavioral failures, then GREEN PASS |
| `go test ./... -count=1` | PASS |
| `git diff --check` | PASS |

- The MCP process is client-owned, uses `Server.Run` with `StdioTransport`, exits after client close, and the test uses `t.TempDir()` both for its binary and library. No process is deliberately retained after the client session closes.
- CLI serialization is measured by the existing application recorder. The SDK does not expose a final outbound MCP wire-byte hook in this implementation, so the MCP path records the shared structured payload bytes and does not claim total JSON-RPC wire-byte measurement.

### Task state, gaps, and workload

- The P1.2 checkbox remains visibly unchecked. Missing required work includes tested opaque continuation token issuance/validation and stale view rejection, complete CLI JSON/stdin and human parity coverage, nested/unavailable path coverage, association-list parity coverage, and the requested broader CLI/MCP metrics assertions. Those gaps mean the whole P1.2 contract is not complete.
- Current source/test/docs/dependency additions and modifications are below the 2,000-line authorized block, but the task remains incomplete rather than being marked complete on partial evidence. No design deviation or deferred feature was added.
- Remaining exact unchecked P1.2 row: `- [ ] Add shared-service, CLI, and MCP tests plus concrete project/path operations in \`internal/app/projects.go\`, \`internal/adapter/cli/\`, \`internal/adapter/mcp/\`, and \`internal/testkit/\`; run RED → GREEN → TRIANGULATE → REFACTOR for identity, aliases, ambiguity/unavailability, nested/symlink paths, and noninteractive errors. <!-- sdd-owner: implementation -->`

---

## P1.2 bounded gatekeeper follow-up — completed

### Status, authority, and workload

- Consumed authoritative `apply: ready` status for the repository-local workspace `/Users/juanmotta/Desktop/personal/memgraphai`, the sole allowed edit root, with no `actionContext` warnings.
- The parent authorized bounded correction work unit `p12-project-cli-mcp` on `267aa72` and retains native acquire/settle/counter ownership. No commit, push, review action, global install, user library, daemon, HTTP, FTS retrieval, provider, paid call, or live integration occurred.
- Pre-write forecast measured 886 changed product/evidence lines, leaving 1,114 of the 2,000-line gate. Final product/evidence count is 1,362 lines: 237 tracked added/deleted plus 1,125 untracked P1.2 product/evidence lines, leaving 638. This is one bounded follow-up, not a size exception or chained PR.

### Completed task and persisted checkbox

- [x] P1.2 — project service, SQLite list-view persistence, CLI/MCP parity, bound continuation behavior, and emitted-byte metrics are complete.
  - The exact P1.2 implementation-owned row was changed from `- [ ]` to `- [x]` after final verification.
  - `docs/evidence/phase1-projects.md` SHA-256: `2e90dcc9584f6424b5bbecf5354e2088afec0fe8ab2f009db5f96bc898cf5e4e`.

### Observed completion and limits

- Project-list tokens are immutable-ID keyset continuations bound to version, operation, complete scope, empty filter, ordering, limit, cursor, and SQLite view generation. Malformed/rebound tokens are `invalid`; a changed project/path view is `conflict`.
- Built CLI process tests cover human output, JSON stdin, all six operations including association list, alias, nested noninheritance, multi-project ambiguity, moved/unavailable paths, and missing-scope rejection. The SDK `CommandTransport` MCP process covers the same six operations and structured accepted-business outcomes. Malformed SDK arguments can fail protocol schema validation before shared business validation.
- CLI records the actual writer count including JSON/human newline framing. MCP uses SDK `IOTransport` plus a newline-frame writer wrapper to record the final emitted JSON-RPC frame after successful write; recorder failure remains isolated. The test explicitly `Close`s and `Wait`s for the SDK client before opening SQLite, fixing an observed `SQLITE_BUSY` cleanup race.
- `project.association.list` is deterministic but not paginable in the settled P1.0 contract, so no new association token was invented. MCP byte attribution is not claimed for a batch containing multiple operation responses.

### TDD Cycle Evidence

| Cycle | RED | GREEN / triangulation |
| --- | --- | --- |
| Pagination | `go test ./internal/app -run '^TestProjectListIssuesBoundTokensAndRejectsStaleOrReboundTokens$' -count=1` failed: `first page next_token is empty` | Focused test passes continuation, malformed/rebound invalid outcomes, and stale-view conflict. |
| CLI pagination | `go test ./cmd/memgraphai -run '^TestProjectCLIProcessUsesHumanAndJSONForAllOperations$' -count=1` failed with JSON `invalid` list input | Built process test passes human/JSON parity and path outcomes. |
| MCP metric | `go test ./internal/adapter/mcp -run '^TestStdioServerListsAndCallsProjectTool$' -count=1` failed: `MCP response bytes = 192, payload bytes = 192` | Real SDK process passes final-frame accounting. |
| CLI metric | `go test ./internal/adapter/cli -run '^TestCLIRecordsExactNewlineDelimitedOutputBytes$' -count=1` failed: `CLI response bytes = 194, emitted bytes = 195` | Focused test passes actual writer-count accounting. |

### Final commands and files

| Command | Result |
| --- | --- |
| `go test ./internal/app -run '^(TestProjectListIssuesBoundTokensAndRejectsStaleOrReboundTokens|TestProjectServicePreservesExplicitAssociationAndResolutionOutcomes)$' -count=1` | PASS |
| `go test ./internal/adapter/cli -run '^(TestCLIRecordsExactNewlineDelimitedOutputBytes|TestHumanCreateBuildsTheSharedJSONEnvelope)$' -count=1` | PASS |
| `go test ./cmd/memgraphai -run '^TestProjectCLIProcessUsesHumanAndJSONForAllOperations$' -count=1` | PASS |
| `go test ./internal/adapter/mcp -run '^TestStdioServerListsAndCallsProjectTool$' -count=1` | PASS |
| `go test ./... -count=1` (repeated after focused checks) | PASS |
| `git diff --check` | PASS |

- Follow-up files: `internal/app/contract.go`, `internal/app/projects.go`, `internal/app/projects_test.go`, `internal/adapter/cli/cli.go`, `internal/adapter/cli/cli_test.go`, `internal/adapter/mcp/mcp.go`, `internal/adapter/mcp/mcp_test.go`, `cmd/memgraphai/main_test.go`, and `docs/evidence/phase1-projects.md`, plus task/progress artifacts.
- The existing partial `internal/store/sqlite/identity.go` change only maps duplicate project writes to stable `conflict` and delegates `AssociatePath` to exact-association behavior; it is necessary project-view integration, not unrelated identity drift. Existing migration tests prove reopen/version behavior, legacy-operation compatibility, and rollback of a failed compatibility migration; the final suite passes them unchanged.

### Remaining implementation tasks

- [ ] Add continuity tests and concrete workstream/session persistence in `internal/app/continuity.go`, `internal/store/sqlite/`, `internal/adapter/cli/`, `internal/adapter/mcp/`, and `internal/testkit/`; run RED → GREEN → TRIANGULATE → REFACTOR for mismatches, missing sessions, explicit close, disconnect, interleaved logical MCP sessions, resume, and fork. <!-- sdd-owner: implementation -->
- [ ] Add document tests and concrete services in `internal/app/documents.go`, `internal/revisionfs/`, `internal/store/sqlite/`, `internal/adapter/cli/`, `internal/adapter/mcp/`, and `internal/testkit/`; run RED → GREEN → TRIANGULATE → REFACTOR for scope-only reads, history, stale writes, precommit failure, response loss, altered files, idempotent replay, and orphan reporting, without adding FTS retrieval. <!-- sdd-owner: implementation -->
- [ ] Add checkpoint tests and concrete persistence/adapters in `internal/app/checkpoints.go`, `internal/store/sqlite/`, `internal/adapter/cli/`, `internal/adapter/mcp/`, and `internal/testkit/`; run RED → GREEN → TRIANGULATE → REFACTOR for validated session binding, limits, exact revision references, freshness, explicit resume, disconnect, and fork distinction. <!-- sdd-owner: implementation -->
- [ ] Add cross-cutting tests and close only observed gaps in `internal/testkit/`, `internal/adapter/cli/`, `internal/adapter/mcp/`, `internal/store/sqlite/`, and `internal/revisionfs/`; run RED → GREEN → TRIANGULATE → REFACTOR, then record actual commands, results, and limitations in `docs/evidence/phase1-verification.md` without adding deferred features. <!-- sdd-owner: implementation -->

---

## P1.2 independent verification reconciliation

- Independent verification passed the focused P1.2 command, full suite, repeated real CLI/MCP checks (`-count=5`), repeated association pagination checks (`-count=10`), and `git diff --check 267aa72`.
- Association listing uses `ORDER BY supplied_path LIMIT`; 52 rows produce a 50-row first page plus continuation, with cross-project rebinding `invalid` and stale views `conflict`.
- Independently captured newline-delimited JSON-RPC bytes exactly equal the persisted MCP metric. No batch-attribution guarantee is claimed.
- Child reaping uses a deterministic 2-second context and 20-millisecond terminate duration, asserts `ProcessState`, and left no current processes. Remaining direct `Wait` calls have a 20-second `exec.CommandContext` and package timeout, not proof against every possible hang.
- P1.2 implementation and its task checkbox are complete. P1.3–P1.6 remain future work; native review remains pending.
- This reconciliation changed documentation only: no source, tests, dependency, user library, authority action, commit, or process.

## Key Learnings

- Preserve failed historical evidence, then qualify it with the later independent result rather than rewriting the record.
- Keep implementation completion distinct from native review closure and from broader guarantees the tests did not establish.

---

## P1.3a shared continuity services and persistence

### Status, authority, and boundary

- Consumed authoritative `gentle-ai.sdd-status@2`: `apply: ready`, repository-local workspace `/Users/juanmotta/Desktop/personal/memgraphai`, the same sole allowed edit root, and no `actionContext` warnings.
- The user authorized only `workunitp13a-continuity-services`, with a 1,500-line P1.3a forecast inside the 2,000-line review block. The parent retains native acquire/settle ownership; this worker did not acquire, settle, or mirror any token.
- P1.3a completes the shared service and SQLite slice only. The whole P1.3 implementation row deliberately remains visibly unchecked: P1.3b still owns CLI/MCP wiring, real protocol parity, transport disconnect handling, and MCP interleaving tests.

### Completed bounded objective and persisted task state

- [x] P1.3a — shared `workstream.create/list/fork` and `session.open/status/close/disconnect/resume` behavior, v3 SQLite persistence, lifecycle CAS, active-binding validation, and valid-v2 migration coverage.
- The persisted P1.3 implementation-owned checkbox was intentionally **not** changed and remains `- [ ]`; it represents the larger adapter-inclusive task rather than this completed first cut. No other task checkbox changed.
- Continuity evidence SHA-256: `de9c5e92ed9da7bbc7700b971dead0ca3302d12352c6ce977d0293aa8080fd63` for `docs/evidence/phase1-continuity.md`.

### Files changed

- `internal/app/continuity.go`, `internal/app/continuity_test.go`
- `internal/store/sqlite/continuity.go`, `internal/store/sqlite/continuity_test.go`
- `internal/store/sqlite/migrations.go`, `internal/store/sqlite/migrations_test.go`
- `docs/evidence/phase1-continuity.md`
- `openspec/changes/backend-foundation-and-continuity/apply-progress.md`

### Strict TDD evidence and verification

| Cycle | RED | GREEN / triangulation / refactor |
| --- | --- | --- |
| Workstream create | `TestContinuityServiceCreatesWorkstreamWithCallerIDsAndProvenance` failed behaviorally with `internal`, want `ok`. | Concrete v3 store and shared service pass with supplied IDs, provenance, server UTC time, bounded list metadata, and source-preserving fork. |
| List/fork pagination | `TestContinuityServiceListsBoundedMetadataAndForksWithoutMutatingSource` failed behaviorally with `invalid` instead of a continuation. | Keyset list ordering, default/bounded page semantics, malformed/rebound/stale token cases, empty valid-project lists, and missing-project results pass. |
| Session lifecycle | `TestContinuityServiceKeepsDisconnectCloseAndResumeDistinct` failed behaviorally because open returned `invalid`. | Explicit attributable disconnect, structural closed/disconnected lookup, CAS close/no-reopen, active-binding rejection, explicit-source resume, missing/mismatched triples, and shared-service interleaving pass. |
| Migration | Existing `TestOpenRecordsFoundationMigrationVersion` failed after the v3 migration with `migration version = 3, want 2`; a later direct cross-project fork-source update test failed because SQLite accepted it. | The updated assertion and valid-v2 migration fixture preserve existing rows as open/unknown with NULL historical time; additive triggers now reject direct cross-project fork inserts/updates and resume sources on a second connection. |

A first pre-RED public stub accidentally reused the P0 `CreateWorkstream` method name and produced a duplicate-method compilation error. It was renamed before the executed behavioral RED and is documented as a setup correction, not behavioral RED.

| Command | Result |
| --- | --- |
| `go test ./internal/app ./internal/store/sqlite -count=1 -timeout=120s` before edits | PASS safety net |
| focused RED commands recorded in `docs/evidence/phase1-continuity.md` | Expected behavioral failures, then PASS GREEN |
| `go test ./internal/app ./internal/store/sqlite -count=1 -timeout=120s` | PASS triangulation/refactor |
| `go test ./... -count=1 -timeout=120s` | PASS final suite |
| `git diff --check` | PASS |

### Design conformance, workload, and cleanup

- No design deviation: IDs are caller supplied, provenance is client reported rather than authenticated, timestamps are UTC server values, legacy data does not gain invented origin/time, and only explicit close is terminal. The store validates relationships, checks affected rows, and uses transactions/CAS for lifecycle transitions.
- P1.3a product/evidence delta is 1,124 changed lines: 993 physical lines in new continuity source/test/evidence files plus 131 added/deleted migration-test lines. `apply-progress.md` is excluded as a planning artifact. This remains below the 1,500-line forecast and 2,000-line review block; no PR, chain, commit, or size exception was created.
- Fixtures use `t.TempDir()`, tests are short lived, and no user library, daemon, dependency, global install, external/paid call, commit, push, CLI/MCP source edit, P1.4 document behavior, or P1.5 checkpoint behavior occurred. Shared-service interleaving is not protocol parity or connection-disconnect evidence.

### Remaining implementation tasks

- [ ] Add continuity tests and concrete workstream/session persistence in `internal/app/continuity.go`, `internal/store/sqlite/`, `internal/adapter/cli/`, `internal/adapter/mcp/`, and `internal/testkit/`; run RED → GREEN → TRIANGULATE → REFACTOR for mismatches, missing sessions, explicit close, disconnect, interleaved logical MCP sessions, resume, and fork. <!-- sdd-owner: implementation -->
- [ ] Add document tests and concrete services in `internal/app/documents.go`, `internal/revisionfs/`, `internal/store/sqlite/`, `internal/adapter/cli/`, `internal/adapter/mcp/`, and `internal/testkit/`; run RED → GREEN → TRIANGULATE → REFACTOR for scope-only reads, history, stale writes, precommit failure, response loss, altered files, idempotent replay, and orphan reporting, without adding FTS retrieval. <!-- sdd-owner: implementation -->
- [ ] Add checkpoint tests and concrete persistence/adapters in `internal/app/checkpoints.go`, `internal/store/sqlite/`, `internal/adapter/cli/`, `internal/adapter/mcp/`, and `internal/testkit/`; run RED → GREEN → TRIANGULATE → REFACTOR for validated session binding, limits, exact revision references, freshness, explicit resume, disconnect, and fork distinction. <!-- sdd-owner: implementation -->
- [ ] Add cross-cutting tests and close only observed gaps in `internal/testkit/`, `internal/adapter/cli/`, `internal/adapter/mcp/`, `internal/store/sqlite/`, and `internal/revisionfs/`; run RED → GREEN → TRIANGULATE → REFACTOR, then record actual commands, results, and limitations in `docs/evidence/phase1-verification.md` without adding deferred features. <!-- sdd-owner: implementation -->

---

## P1.5a V4 migration evidence correction

### Status, scope, and task state

- Consumed the authoritative native status: `apply: ready`, repo-local workspace `/Users/juanmotta/Desktop/personal/memgraphai`, that workspace as the sole allowed edit root, and no action-context warnings. The parent authorized only this coverage/evidence correction under a 300 A+D cap; no production, adapter, dependency, authority, commit, service, or child action occurred.
- The P1.5 and P1.6 implementation-owned rows remain visibly unchecked. No checkbox changed because this cut supplies no missing adapter work and does not complete either aggregate task.

### Correction and verification

- Historical evidence SHA-256 `1d15f449a397041eefccb05245639fb91cdf9eb82252032d290505ae49cedac0` is preserved as failed verification input. Its claimed V4→V5 test did not exist: pre-edit `go test ./internal/store/sqlite -list '^TestOpenMigratesValidV4ToV5WithoutInventingCheckpoints$' -count=1 -timeout=120s` printed no test name. This correction is therefore coverage-only, not a manufactured RED for an already-working migration.
- Added `TestOpenMigratesValidV4ToV5WithoutInventingCheckpoints` using actual V1–V4 migration schemas and a nonempty V4 project/workstream/session/document/revision/operation fixture. It asserts V1–V6 ledger preservation, all V5/V6 checkpoint constraints, exact legacy data preservation, and zero invented checkpoints/references. The existing V5→V6 guard test remains unchanged.

| Command | Result |
| --- | --- |
| `go test ./internal/store/sqlite -list '^TestOpenMigratesValidV4ToV5WithoutInventingCheckpoints$' -count=1 -timeout=120s` | Printed the exact test name after addition. |
| `go test ./internal/store/sqlite -run '^TestOpenMigratesValidV4ToV5WithoutInventingCheckpoints$' -count=20 -v -timeout=120s` | PASS with 20 exact verbose RUN/PASS pairs. |
| `go test ./internal/app ./internal/store/sqlite -run '^TestCheckpoint' -count=5 -timeout=120s` | PASS. |
| `go test ./... -count=1 -timeout=120s` | PASS. |
| `git diff --check` | PASS. |

### Evidence, workload, and remaining work

- Changed only `internal/store/sqlite/migrations_test.go`, `docs/evidence/phase1-checkpoints-service.md`, and this progress record. Evidence SHA-256: `bd3faad0cf9fee306964ad05243af1ac24ccff42cfa44aca917b69458e03e135`.
- The correction is 113 non-planning A+D (100 test lines and 13 evidence lines); this 30-line progress addendum brings it to 143 A+D versus the starting files, below the 300-line cap. Full P1.5a source/test/evidence scope excluding OpenSpec planning is 1,142 A+D: 894 new checkpoint/evidence physical lines plus 73 migration and 175 migration-test A+D.
- This adds no power-loss, platform, backup, restoration, adapter parity, checkpoint list/update/delete, or verification-completion claim. Remaining persisted implementation rows are exactly:
  - [ ] Add checkpoint tests and concrete persistence/adapters in `internal/app/checkpoints.go`, `internal/store/sqlite/`, `internal/adapter/cli/`, `internal/adapter/mcp/`, and `internal/testkit/`; run RED → GREEN → TRIANGULATE → REFACTOR for validated session binding, limits, exact revision references, freshness, explicit resume, disconnect, and fork distinction. <!-- sdd-owner: implementation -->
  - [ ] Add cross-cutting tests and close only observed gaps in `internal/testkit/`, `internal/adapter/cli/`, `internal/adapter/mcp/`, `internal/store/sqlite/`, and `internal/revisionfs/`; run RED → GREEN → TRIANGULATE → REFACTOR, then record actual commands, results, and limitations in `docs/evidence/phase1-verification.md` without adding deferred features. <!-- sdd-owner: implementation -->

---

## P1.3b continuity adapters

### Status, task state, and completed behavior

- Consumed the supplied authoritative `apply: ready` repo-local status for `/Users/juanmotta/Desktop/personal/memgraphai`, with that root allowed and no action-context warnings. The user authorized only `p13b-continuity-adapters`; no settlement, actor attempt, review action, commit, push, external integration, user-library access, dependency change, or child agent was used.
- The persisted P1.3 implementation-owned row is now visibly checked after the P1.3a persistence and P1.3b adapter evidence passed. The remaining implementation rows are P1.4, P1.5, and P1.6.
- CLI human/JSON and MCP now dispatch all eight existing continuity operations through the concrete `ContinuityService`. Scope is request-specific; the MCP connection has no current project, workstream, or session state.
- Real isolated CLI/MCP subprocess tests compare semantic envelopes after removing only server-generated timestamps. They cover valid operations, missing/mismatched sessions, source-preserving fork/resume, two-project/workstream MCP interleaving, explicit attributable disconnect, terminal close, and raw EOF followed by fresh-process reads.
- A repeated suite exposed a metric-write race that returned `internal` for a later session operation. The MCP writer now completes its pending final-frame metric attempt before dispatching the next request; recorder failures remain ignored.

### TDD cycle evidence

| Cycle | RED | GREEN and triangulation |
| --- | --- | --- |
| CLI dispatch | `go test ./cmd/memgraphai -run '^TestContinuityCLIProcessUsesHumanAndJSON$' -count=1 -timeout=120s` failed behaviorally: human workstream create returned `memgraphai: invalid command`. | Same command passed after explicit continuity command/scope normalization; process test covers human/JSON, all operations, close/resume, and wrong scope. |
| MCP discovery | `go test ./internal/adapter/mcp -run '^TestStdioServerListsAndCallsContinuityTools$' -count=1 -timeout=120s` failed behaviorally with six rather than fourteen tools. | Same command passed after continuity registration; parity, missing/mismatched scope, interleaving, explicit disconnect, and raw EOF passed. |
| Metric isolation | The first repeated full-suite command failed on run 4 with `session.close` returning `internal`; focused `-count=10` also observed `session.disconnect` failures. | `go test ./internal/adapter/mcp -run '^TestStdioServerListsAndCallsContinuityTools$' -count=10 -timeout=120s` passed after the pending-metric barrier, followed by five full-suite passes. |

### Verification, evidence, workload, and cleanup

| Command | Result |
| --- | --- |
| `go test ./internal/adapter/cli ./internal/adapter/mcp ./cmd/memgraphai -count=1 -timeout=120s` before edits | PASS safety net |
| CLI and MCP RED commands above | Expected behavioral failures, then PASS GREEN |
| `go test ./internal/adapter/mcp -run '^(TestContinuityAdaptersHaveEquivalentOutcomes|TestContinuityMCPEOFLeavesSessionsUnchangedAndDisconnectIsScoped)$' -count=1 -timeout=120s` | PASS |
| focused MCP lifecycle test with `-count=10` | PASS after metric correction |
| `go test ./... -count=1 -timeout=120s` repeated five times after correction | PASS all five runs |
| `git diff --check 73e2c7d` | PASS |

- Evidence SHA-256: `4585635983c16c87e9e56783ae6ad8e2f4f99e88de7b9d19ca9463f8990c5990` for `docs/evidence/phase1-continuity-adapters.md`; it records exact commands, limits, and cleanup facts.
- The P1.3b product/test delta measured from `73e2c7d` is below the 1,400-line parent cap. No size exception or chain was selected.
- SDK subprocesses use deadlines, `Close`, `Wait`, and `t.TempDir()`; final process inspection found no `memgraphai` process. No EOF attribution, reconnect/reuse, daemon, FTS, document, checkpoint, or deferred behavior was added.

### Remaining implementation tasks

- [ ] Add document tests and concrete services in `internal/app/documents.go`, `internal/revisionfs/`, `internal/store/sqlite/`, `internal/adapter/cli/`, `internal/adapter/mcp/`, and `internal/testkit/`; run RED → GREEN → TRIANGULATE → REFACTOR for scope-only reads, history, stale writes, precommit failure, response loss, altered files, idempotent replay, and orphan reporting, without adding FTS retrieval. <!-- sdd-owner: implementation -->
- [ ] Add checkpoint tests and concrete persistence/adapters in `internal/app/checkpoints.go`, `internal/store/sqlite/`, `internal/adapter/cli/`, `internal/adapter/mcp/`, and `internal/testkit/`; run RED → GREEN → TRIANGULATE → REFACTOR for validated session binding, limits, exact revision references, freshness, explicit resume, disconnect, and fork distinction. <!-- sdd-owner: implementation -->
- [ ] Add cross-cutting tests and close only observed gaps in `internal/testkit/`, `internal/adapter/cli/`, `internal/adapter/mcp/`, `internal/store/sqlite/`, and `internal/revisionfs/`; run RED → GREEN → TRIANGULATE → REFACTOR, then record actual commands, results, and limitations in `docs/evidence/phase1-verification.md` without adding deferred features. <!-- sdd-owner: implementation -->

---

## P1.4a document shared services, store, and verified reads

### Status, authority, and task state

- Consumed authoritative `apply: ready` repository-local status for `/Users/juanmotta/Desktop/personal/memgraphai`, with that root allowed and no action-context warning.
- The user authorized only P1.4a shared services/store/verified reads before adapters, with a 1,500-line maximum. No acquire, settlement, review action, commit, push, adapter/cmd/dependency change, user-library access, global install, persistent process, or child agent was used.
- The aggregate P1.4 task remains visibly unchecked by explicit instruction because CLI/MCP adapter exposure is a later P1.4 cut. No persisted task checkbox changed.

### Completed bounded objective

- Added `document.create`, `document.update`, `document.list`, `document.read`, and `document.history` shared-service behavior with strict input, project-general/workstream scope isolation, base64 exact Markdown bytes, direct-service page limits/token binding, and 1 MiB input/2 MiB document-result bounds.
- Added additive SQLite v4 document scope/provenance/view schema, immutable document/revision triggers, operation-digest provenance/workstream binding, durable replay through the existing fenced ledger, invisible precommit rows, targeted orphan reporting, and descriptor-bound exact verified reads.

### Files changed

- `internal/app/documents.go`, `internal/app/documents_test.go`
- `internal/store/sqlite/documents.go`, `internal/store/sqlite/documents_test.go`, `internal/store/sqlite/migrations.go`, `internal/store/sqlite/migrations_test.go`, `internal/store/sqlite/recovery.go`
- `internal/revisionfs/read.go`, `internal/revisionfs/publication.go`, `internal/revisionfs/publication_test.go`
- `docs/evidence/phase1-documents-service.md`
- `openspec/changes/backend-foundation-and-continuity/apply-progress.md`

### TDD and verification evidence

| Cycle | RED | GREEN / triangulation |
| --- | --- | --- |
| Service create | Focused create test failed behaviorally with `internal`, want `ok`. | Passed after strict create/update behavior; scope isolation, history, stale update, direct page limits, rebound and stale token cases pass. |
| V4 migration | Focused valid-v3 reopen failed with missing `d.workstream_id`. | Passed after additive v4 scope/provenance/view migration; legacy fields stay unknown/NULL. |
| Verified bytes | Focused `ReadVerified` test failed with integrity discrepancy instead of exact bytes. | Passed after descriptor-bound read; existing symlink/nonregular tests and added historical/current integrity cases pass. |

| Command | Result |
| --- | --- |
| `go test ./internal/app ./internal/store/sqlite ./internal/revisionfs -count=1 -timeout=120s` | PASS. |
| `go test ./... -count=1 -timeout=120s` | PASS before repeats and PASS five additional independent runs. |
| `go test ./internal/app ./internal/store/sqlite ./internal/revisionfs -count=5 -timeout=120s` | PASS. |
| `git diff --check a8f098a` | PASS. |

### Design conformance, workload, and limits

- No FTS retrieval, context assembly, session attribution, checkpoint behavior, frontmatter authority, purge/import behavior, global scan tool, CLI/MCP/cmd code, or dependencies were added. Orphan reporting reuses the existing report-only storage evidence.
- The service result encodes Markdown as unpadded base64, so JSON escaping cannot inflate exact document bytes. The document result is checked at 2 MiB; existing globally unbounded envelope IDs remain a separate `internal/app/contract.go` concern outside this cut and are not claimed as a global response limit.
- The product/evidence delta is under the 1,500-line authorized forecast excluding this cumulative progress record. No chain or size exception was selected.

### Remaining implementation tasks

- [ ] Add document tests and concrete services in `internal/app/documents.go`, `internal/revisionfs/`, `internal/store/sqlite/`, `internal/adapter/cli/`, `internal/adapter/mcp/`, and `internal/testkit/`; run RED → GREEN → TRIANGULATE → REFACTOR for scope-only reads, history, stale writes, precommit failure, response loss, altered files, idempotent replay, and orphan reporting, without adding FTS retrieval. <!-- sdd-owner: implementation -->
- [ ] Add checkpoint tests and concrete persistence/adapters in `internal/app/checkpoints.go`, `internal/store/sqlite/`, `internal/adapter/cli/`, `internal/adapter/mcp/`, and `internal/testkit/`; run RED → GREEN → TRIANGULATE → REFACTOR for validated session binding, limits, exact revision references, freshness, explicit resume, disconnect, and fork distinction. <!-- sdd-owner: implementation -->
- [ ] Add cross-cutting tests and close only observed gaps in `internal/testkit/`, `internal/adapter/cli/`, `internal/adapter/mcp/`, `internal/store/sqlite/`, and `internal/revisionfs/`; run RED → GREEN → TRIANGULATE → REFACTOR, then record actual commands, results, and limitations in `docs/evidence/phase1-verification.md` without adding deferred features. <!-- sdd-owner: implementation -->

---

## P1.4a gatekeeper correction — blocked before code changes

### Status and action context consumed

- Consumed the parent-native status: `apply: ready`, repository-local workspace `/Users/juanmotta/Desktop/personal/memgraphai`, that workspace as the allowed edit root, and no `actionContext` warnings.
- Strict TDD is active and `go test ./... -count=1 -timeout=120s` passed as the existing safety net before any correction edit.
- The scope is limited to the three stated P1.4a regressions. No task checkbox, adapter, checkpoint, dependency, commit, review action, acquire/settle action, or source/test file changed.

### Delivery gate and measured budget

- The persisted task forecast still says `Decision needed before apply: Yes`, `Chained PRs recommended: Yes`, and high budget risk. The configured delivery strategy is `ask-on-risk`.
- The current P1.4a product/evidence candidate is exactly 1,175 added-plus-deleted lines from `a8f098a`: 1,001 untracked P1.4a lines plus 137 additions and 37 deletions in its tracked files. A 1,500-line whole-cut cap leaves 325 changed lines.
- The supplied `max1500wholecut` bound is not an explicit `size:exception` acceptance and does not select `auto-chain` or another chained delivery mode. Per the delivery gate, implementation stopped before RED rather than infer that missing human decision or compromise the required regression tests/evidence.

### Required decision and remaining task

- Required decision: explicitly approve `size:exception` for this 1,500-line single-cut P1.4a correction, or select `auto-chain`/a specific chained delivery mode and its assigned boundary.
- The P1.4 implementation-owned checkbox remains visibly unchecked because neither the original aggregate task nor this correction completed.
- No fresh correction evidence SHA exists: no RED/GREEN/TRIANGULATE cycle was started, and the parent-owned supplied evidence binding was not acquired, settled, replaced, or claimed.

---

## P1.4a gatekeeper correction — completed bounded fixes

### Status, boundary, and task state

- The parent explicitly resumed the same P1.4a attempt after the concrete forecast established that it fit the 1,500-line runtime bound. Native status remained `apply: ready`, repository-local authority remained `/Users/juanmotta/Desktop/personal/memgraphai`, and `actionContext.warnings` remained empty.
- Only the three authorized regressions changed: legacy v3 digest replay, pre-decode document input bounding, and generation-zero continuation freshness. No adapter, checkpoint, dependency, commit, review/native action, acquire, or settlement action occurred.
- P1.4 remains visibly unchecked in `tasks.md`: this shared-service correction does not close its later CLI/MCP adapter scope. No persisted task checkbox changed.

### Strict TDD and verification

| Cycle | RED | GREEN / triangulation |
| --- | --- | --- |
| V3 replay and view freshness | A genuine v3 schema fixture first returned list replay `ok`, want `conflict`; after that fix, committed replay returned `idempotency_mismatch`, want committed. | Exact baseline digest replays committed/conflict rows and rejects changed nonempty provenance; migrated generation-zero list and history tokens conflict after a valid view mutation. |
| Decode allocation bound | The initial missing-decoder RED was retained; the genuine prepatch behavior then failed `oversized decode allocations = 1, want zero before DecodeString`. | Exact 1 MiB raw input succeeds, one byte over and invalid raw input fail, and the isolated oversized decode path reports zero allocations. |

| Command | Result |
| --- | --- |
| `go test ./internal/app ./internal/store/sqlite -count=1 -timeout=120s` | PASS safety net before corrections. |
| Focused RED commands recorded in `docs/evidence/phase1-documents-service.md` | Expected failures, then PASS GREEN. |
| Focused app and SQLite correction commands with `-count=5` | PASS. |
| `go test ./... -count=1 -timeout=120s` | PASS final suite. |
| `git diff --check a8f098a` | PASS. |

### Evidence, workload, and cleanup

- Distinct correction evidence SHA-256: `ab4f7c9a01adab1af21b10898fe5603c2501da2feabb66bf867d6023e7d238e3` for `docs/evidence/phase1-documents-service.md`.
- The actual P1.4a product/evidence candidate is 1,310 A+D from `a8f098a` (1,058 untracked additions plus 215 tracked additions and 37 tracked deletions), leaving 190 lines under the 1,500-line bound. The correction added 135 A+D from the prior 1,175 baseline; no test, comment, or evidence was reduced to meet the bound.
- Fixtures use `t.TempDir()`; only short-lived Go test processes ran. No process helper, temporary build artifact, user library, or persistent process required cleanup.

## P1.4b document CLI and MCP adapters — historical pre-follow-up record

### Status, authorization, and historical task state

- Consumed the supplied authoritative `apply: ready` repo-local status for `backend-foundation-and-continuity`; `/Users/juanmotta/Desktop/personal/memgraphai` was the sole allowed edit root and `actionContext.warnings` was empty.
- The parent authorized only P1.4b document adapters against committed P1.4a baseline `9edf232`; this worker did not acquire or settle a token, start a child, act on native authority, commit, or push.
- The P1.4 implementation-owned task is complete. Its persisted `tasks.md` row was changed from `- [ ]` to `- [x]` immediately after all P1.4a/P1.4b requirements passed, then re-read visibly checked. P1.5 and P1.6 remain unchecked.

### Delivered adapter boundary

- CLI human and JSON operations now dispatch `document.create`, `document.update`, `document.list`, `document.read`, and `document.history` to the existing `DocumentService` with `revisionfs.New(root, nil)` and the selected SQLite store. The narrow document builder emits nested provenance and a true JSON null for create's expected revision.
- MCP exposes those five tools, bringing discovery to 19 tools, and `cmd/memgraphai` builds the same document service for MCP stdio. Both adapters retain request-declared project-general/workstream scope and final-byte metrics.
- `docs/evidence/phase1-documents-adapters.md` SHA-256 is `2111a6b9d55c13c6254440f88bda1b2f73bd623439138f6026bf3e95be944a5e`.

### TDD Cycle Evidence

| Cycle | Test layer | RED | GREEN | TRIANGULATE / REFACTOR |
| --- | --- | --- | --- | --- |
| CLI document commands | Built executable process | Human document create returned `memgraphai: invalid command`. | Same focused command passed after the narrow builder and service wiring. | All five human and JSON operations prove exact base64, nested provenance, explicit null create, and exact update expectation. |
| MCP document tools | SDK `CommandTransport` process | Tool discovery returned 14, want 19. | Same focused command passed after five registrations and service wiring. | All five tools, scope isolation, reconnect replay, and an independently captured document JSON-RPC frame passed. |

### Historical verification, workload, and cleanup

| Command | Result |
| --- | --- |
| `go test ./internal/adapter/cli ./internal/adapter/mcp ./cmd/memgraphai -count=1 -timeout=120s` | PASS safety net before edits. |
| `go test ./cmd/memgraphai ./internal/adapter/mcp -run '^(TestDocumentCLIProcessUsesAllHumanAndJSONOperations|TestStdioServerExposesDocumentToolsAndExactBytes)$' -count=1 -timeout=120s` | Expected behavioral RED, then PASS GREEN. |
| `go test ./... -count=1 -timeout=120s` | PASS. |
| `go test ./cmd/memgraphai ./internal/adapter/mcp -run '^(TestDocumentCLIProcessUsesAllHumanAndJSONOperations|TestDocumentCLIReplayRemainsReachableAfterDiscardedResponse|TestStdioServerExposesDocumentToolsAndExactBytes|TestMCPMetricEqualsIndependentlyCapturedJSONRPCFrame)$' -count=5 -timeout=120s` | PASS. |
| `go test ./... -count=5 -timeout=120s` | PASS. |
| `git diff --check` | PASS. |

- Exact P1.4b tracked source/test delta versus `9edf232` is 255 additions and 20 deletions (275 A+D). The 43-line evidence file, two-line checkbox replacement, and 40-line new progress section bring the exact scoped source/test/evidence/persistence total to 360 A+D; unrelated planning artifacts are excluded. This is one P1.4b work-unit boundary, well below 1,000 A+D; no PR, chain, or size exception was selected.
- The CLI replay test commits in one separate process while stdout is discarded, then receives the durable result through an identical second process. The MCP replay occurs after explicit close/wait and reconnect. These are durable-reachability checks, not reproduction of P0's post-commit process kill; the frozen P0/P1.4a lower-layer evidence remains the limitation.
- Fixtures use `t.TempDir()`, contexts have deadlines, command transports use a 250 ms terminate duration, and test connections are closed and waited. No sleep synchronization, user library, daemon, fault flag, dependency change, or persistent process was used.

### Remaining implementation tasks

- [ ] Add checkpoint tests and concrete persistence/adapters in `internal/app/checkpoints.go`, `internal/store/sqlite/`, `internal/adapter/cli/`, `internal/adapter/mcp/`, and `internal/testkit/`; run RED → GREEN → TRIANGULATE → REFACTOR for validated session binding, limits, exact revision references, freshness, explicit resume, disconnect, and fork distinction. <!-- sdd-owner: implementation -->
- [ ] Add cross-cutting tests and close only observed gaps in `internal/testkit/`, `internal/adapter/cli/`, `internal/adapter/mcp/`, `internal/store/sqlite/`, and `internal/revisionfs/`; run RED → GREEN → TRIANGULATE → REFACTOR, then record actual commands, results, and limitations in `docs/evidence/phase1-verification.md` without adding deferred features. <!-- sdd-owner: implementation -->

---

## P1.4b interrupted test-only follow-up

### Status, scope, and persisted task state

- Consumed the parent-supplied fresh active SDD proceed context for `p14b-document-adapters`, baseline `9edf232`, and the 1,000 A+D bound. This worker did not acquire or settle authority, start native review, create a child, commit, or change dependencies.
- The correction is test/evidence-only. P1.4a service behavior and all P1.4b production adapters remain untouched.
- P1.4 is visibly checked only after the current focused and full suites passed. P1.5 and P1.6 remain visibly unchecked.

### Strict TDD and parity result

- RED retained from the interrupted acceptance run: `TestDocumentAdaptersHaveEquivalentSemanticEnvelopes` ended with `wait MCP: signal: killed` because the request context was canceled before the registered Close/Wait cleanup. This is a process-cleanup acceptance failure, not a newly claimed production-behavior RED.
- GREEN: cancellation is registered before the Close/Wait cleanup and is backed by an independent 30-second deadline. LIFO cleanup now closes and waits before cancellation; Close, Wait, and the three-second cleanup deadline remain asserted.
- TRIANGULATE: the parity matrix now proves positive historical `document.read` exact-byte equality before tampering, wrong-scope read parity with the settled `scope_denied` outcome, and zero/negative/over-maximum `document.history` limits. These were already-working cases and are not relabeled as RED.
- A first correction run exposed two test-harness issues: the wrong-scope expectation incorrectly requested `binding_mismatch`, and a context derived from `t.Context()` was canceled before cleanup. Both were corrected without production changes.

### Current verification and cleanup

| Command | Result |
| --- | --- |
| `go test ./cmd/memgraphai ./internal/adapter/mcp -run '^(TestDocumentCLIProcessUsesAllHumanAndJSONOperations|TestDocumentCLIReplayRemainsReachableAfterDiscardedResponse|TestStdioServerExposesDocumentToolsAndExactBytes|TestMCPMetricEqualsIndependentlyCapturedJSONRPCFrame|TestDocumentAdaptersHaveEquivalentSemanticEnvelopes)$' -count=5 -timeout=120s` | PASS: `cmd/memgraphai` 13.030s; `internal/adapter/mcp` 17.246s. |
| `go test ./... -count=1 -timeout=120s` | PASS for all packages; `internal/telemetry` has no test files. |
| `pgrep -x memgraphai \| wc -l` | `0`; no built adapter process remained. |

Earlier PASS commands in the historical P1.4b record above are not the current final verification.

### Evidence and workload

- Current evidence SHA-256: `6c4f20f494fa1c79fcf51707205d533ce1f7db21d2c9fd211dda648cc3173e19` for `docs/evidence/phase1-documents-adapters.md`.
- Measured source/test delta versus `9edf232`: 566 additions plus 56 deletions = 622 A+D. The 56-line current evidence file brings the scoped source/test/new-document total to 678 A+D. Old cumulative OpenSpec planning/progress content and unrelated docs/PROJECT artifacts are excluded, as requested. The result remains below the 1,000 A+D bound.
- Fixtures use `t.TempDir()` and bounded contexts. Cleanup asserts process termination rather than ignoring killed-child errors. No daemon, user library, production source, dependency, commit, or native review action was introduced.

## Key Learnings

- A command context derived from `t.Context()` can be canceled before test cleanup; a separately bounded operation context plus LIFO cleanup registration is required when Close/Wait must run first.
- Semantic parity includes equivalent explicit failures: a selected workstream reading a project-general document correctly returns `scope_denied` through both adapters.

---

## P1.5a checkpoint shared service and persistence

### Status, authority, and task state

- Consumed the authoritative native status: `apply: ready`, repo-local workspace `/Users/juanmotta/Desktop/personal/memgraphai`, that workspace as the allowed edit root, and no `actionContext` warnings.
- The parent authorized only `p15a-checkpoint-services`, with no commit, adapter, native authority, dependency, global-install, network, process-service, or user-library action. The P1.4b commit `c31cfd4` and prior advisory record were not reopened.
- The aggregate P1.5 checkbox remains visibly unchecked by explicit instruction: this cut supplies only its shared-service/persistence portion, while CLI/MCP adapter work remains outside the authorized edit surfaces. P1.6 remains unchecked.

### Delivered boundary

- `CheckpointService` accepts only `checkpoint.save` and `checkpoint.read` with a declared active session triple, explicit checkpoint/backing-document/backing-revision IDs, bounded Markdown, exact document/revision references, and required origin provenance.
- SQLite migration V5 adds immutable checkpoint identity, exact same-project/project-general-or-same-workstream reference constraints, and a typed save/read path. Its commit atomically advances the checkpoint backing document pointer, inserts checkpoint metadata/references, and records operation success.
- A same-workstream explicitly resumed session can read its selected source checkpoint after source disconnect; a fork workstream cannot. Freshness is the referenced document's current pointer comparison, not activity recency.

### Strict TDD and verification

| Cycle | Evidence |
| --- | --- |
| RED → GREEN | Compileable initial service scaffold failed `internal`, want `ok`; V5 migration failed version `4`/table `0`, want `5`/`1`; each focused command passed after its minimal implementation. |
| TRIANGULATE | Exact active read, 64 KiB bound, V4→V5, disconnect/resume/fork, per-reference stale freshness, immutable replay, precommit restart, response-loss reachability, and tamper cases pass. |
| Follow-up RED → GREEN | Duplicate exact references initially returned `ok`, want `invalid`; focused app test passed after the duplicate guard. |

| Command | Result |
| --- | --- |
| `go test ./internal/app ./internal/store/sqlite -run '^TestCheckpoint' -count=5 -timeout=120s` | PASS. |
| `go test ./... -count=1 -timeout=120s` | PASS. |
| `git diff --check` | PASS. |

The evidence note records the complete chronological commands and does not relabel later added integration coverage as retrospective RED.

### Files, evidence, workload, and limits

- Changed: `internal/app/checkpoints.go`, `internal/app/checkpoints_test.go`, `internal/store/sqlite/checkpoints.go`, `internal/store/sqlite/checkpoints_test.go`, `internal/store/sqlite/migrations.go`, `internal/store/sqlite/migrations_test.go`, `docs/evidence/phase1-checkpoints-service.md`, and this progress record.
- Evidence SHA-256: `de43ced6333a6a9ca77f7f472f8dd1e014840616b7e07d9e1a94fbb6823e44f7`.
- Before this progress update, scoped source/test/evidence additions are 761 untracked physical lines plus tracked migration/test `115 additions / 7 deletions` (122 A+D), for 883 A+D. This remains below the hard 1,600 runtime boundary; the final progress addition must be included in the parent’s delivery accounting.
- Named filesystem interruption tests are not power-loss, backup, platform-support, model-state restoration, filesystem restoration, verification-completion, or adapter-parity guarantees. No list/update/delete/task-reference behavior was added.

### Remaining implementation tasks

- [ ] Add checkpoint tests and concrete persistence/adapters in `internal/app/checkpoints.go`, `internal/store/sqlite/`, `internal/adapter/cli/`, `internal/adapter/mcp/`, and `internal/testkit/`; run RED → GREEN → TRIANGULATE → REFACTOR for validated session binding, limits, exact revision references, freshness, explicit resume, disconnect, and fork distinction. <!-- sdd-owner: implementation -->
- [ ] Add cross-cutting tests and close only observed gaps in `internal/testkit/`, `internal/adapter/cli/`, `internal/adapter/mcp/`, `internal/store/sqlite/`, and `internal/revisionfs/`; run RED → GREEN → TRIANGULATE → REFACTOR, then record actual commands, results, and limitations in `docs/evidence/phase1-verification.md` without adding deferred features. <!-- sdd-owner: implementation -->

---

## P1.5a checkpoint persistence hardening correction

### Status, authority, and task state

- Consumed the authoritative native status: `apply: ready`, repo-local workspace `/Users/juanmotta/Desktop/personal/memgraphai`, that workspace as the sole allowed edit root, and no `actionContext` warnings.
- The parent supplied fresh-acquire `PROCEED` for the bounded `p15a-checkpoint-services` correction. This implementation used only the authorized SQLite checkpoint, migration, checkpoint evidence, and apply-progress surfaces; no adapter, list feature, dependency, network, authority, commit, or child action occurred.
- The P1.5 aggregate implementation task remains visibly unchecked by explicit instruction. This is a persistence hardening cut only, and P1.6 remains unchecked; no persisted task checkbox changed.

### Corrected behavior

- Migration V6 adds SQLite triggers that reject direct checkpoint deletion and direct checkpoint-reference update or deletion, preserving the checkpoint material aggregate against direct SQL mutation.
- `SaveCheckpoint` now checks `RowsAffected` on its `current_revision_id IS NULL` compare-and-swap. A zero-row race returns `conflict` and rolls back its checkpoint metadata, backing revision, and terminal operation success.
- The controlled test uses the existing filesystem fault point to advance the backing pointer after immutable-file publication. It adds no production test hook or framework.

### TDD Cycle Evidence

| Task | Test file | Layer | Safety net | RED | GREEN | TRIANGULATE | REFACTOR |
| --- | --- | --- | --- | --- | --- | --- | --- |
| P1.5a SQLite aggregate immutability and pointer CAS | `internal/store/sqlite/checkpoints_test.go`, `internal/store/sqlite/migrations_test.go` | SQLite integration | `go test ./internal/store/sqlite -count=1 -timeout=120s` PASS | Focused direct-SQL/CAS command failed behaviorally: direct checkpoint delete and reference update returned nil, and CAS loss returned nil instead of `conflict` | Same focused command PASS after V6 guards and one-row CAS enforcement | Separate direct checkpoint delete, reference update/delete, zero-row rollback, fresh V6, and V5→V6 migration cases PASS | `gofmt`; no production abstraction added; focused, five-run checkpoint, and full suites PASS |

### Commands and results

| Command | Result |
| --- | --- |
| `go test ./internal/store/sqlite -count=1 -timeout=120s` | PASS safety net before correction. |
| `go test ./internal/store/sqlite -run '^(TestCheckpointSchemaRejectsDirectAggregateMutation|TestCheckpointStoreRollsBackWhenBackingPointerCASDoesNotAdvance)$' -count=1 -timeout=120s` | Expected behavioral RED: direct delete/reference update succeeded and CAS loss returned nil; then PASS GREEN. |
| `go test ./internal/store/sqlite -run '^(TestCheckpoint|TestOpenMigratesValidV5ToV6AddingCheckpointGuards|TestOpenRecordsCheckpointMigrationVersion|TestOpenRecordsFoundationMigrationVersion|TestOpenMigratesValidV2ContinuityRowsWithoutInventingProvenanceOrTime|TestOpenAppliesMigrationOnceAndPersistsBoundedMetric)$' -count=1 -timeout=120s` | PASS triangulation/refactor. |
| `go test ./internal/store/sqlite -run '^TestCheckpoint' -count=5 -timeout=120s` | PASS. |
| `go test ./... -count=1 -timeout=120s` | PASS. |
| `git diff --check` | PASS. |

### Files, workload, and limits

- Changed: `internal/store/sqlite/checkpoints.go`, `internal/store/sqlite/checkpoints_test.go`, `internal/store/sqlite/migrations.go`, `internal/store/sqlite/migrations_test.go`, `docs/evidence/phase1-checkpoints-service.md`, and this progress record.
- The prior independent failed-verification evidence SHA remains preserved as `de43ced6333a6a9ca77f7f472f8dd1e014840616b7e07d9e1a94fbb6823e44f7`. Fresh checkpoint evidence SHA-256 is `1d15f449a397041eefccb05245639fb91cdf9eb82252032d290505ae49cedac0`.
- Actual P1.5a source/test/evidence accounting versus `c31cfd4` is 881 untracked physical lines plus 72 additions/1 deletion in `migrations.go` and 69 additions/6 deletions in `migrations_test.go`: **1,029 A+D**. Historical OpenSpec planning/progress content is excluded; the 1,600-line cap retains 571 lines of margin. No chain, PR, commit, or `size:exception` was selected.
- This correction does not prove power-loss, filesystem/platform durability, backup, model/filesystem restoration, adapter parity, checkpoint list/update/delete, or completed verification.

### Remaining implementation tasks

- [ ] Add checkpoint tests and concrete persistence/adapters in `internal/app/checkpoints.go`, `internal/store/sqlite/`, `internal/adapter/cli/`, `internal/adapter/mcp/`, and `internal/testkit/`; run RED → GREEN → TRIANGULATE → REFACTOR for validated session binding, limits, exact revision references, freshness, explicit resume, disconnect, and fork distinction. <!-- sdd-owner: implementation -->
- [ ] Add cross-cutting tests and close only observed gaps in `internal/testkit/`, `internal/adapter/cli/`, `internal/adapter/mcp/`, and `internal/store/sqlite/`, and `internal/revisionfs/`; run RED → GREEN → TRIANGULATE → REFACTOR, then record actual commands, results, and limitations in `docs/evidence/phase1-verification.md` without adding deferred features. <!-- sdd-owner: implementation -->


---

## P1.5a R3 reference-append correction

### Status, scope, and task state

- Consumed the authoritative `apply: ready` status for the repository-local workspace and its sole allowed root, with no `actionContext` warnings. The user authorized only the bounded `p15a-reference-append-correction` database fix; no adapter, checkpoint service, evidence document, dependency, commit, native authority, or child action changed.
- The P1.5 and P1.6 implementation-owned rows remain visibly unchecked because this is a narrow persistence correction, not completion of either aggregate task. No task checkbox changed.

### Corrected behavior and strict TDD

- Migration V7 installs `checkpoint_references_insert_immutable`. It derives the publication seal from the backing checkpoint operation's durable terminal state, so the existing save transaction may atomically insert its initial references while the operation is owned, but a later valid-scope direct append is rejected after commit.
- RED: `go test ./internal/store/sqlite -run '^TestCheckpointSchemaRejectsDirectReferenceAppendAfterPublication$' -count=1 -timeout=120s` failed with both direct valid append assertions accepting `nil` errors.
- GREEN and triangulation: the same test passed after V7; it covers a checkpoint with initial references and a terminally published empty-reference checkpoint. V4→V7 and V5→V7 migration assertions retain complete ledgers and four checkpoint immutability guards.

| Command | Result |
| --- | --- |
| focused correction/migration tests, `-count=20` | PASS |
| `go test ./internal/app ./internal/store/sqlite -run '^TestCheckpoint' -count=5 -timeout=120s` | PASS |
| `go test ./... -count=1 -timeout=120s` | PASS |
| `git diff --check` | PASS |

- Runtime test-output SHA-256: `f6c63a3851cdf96e3775d7475be57f9469f4679179648b71a737bbcfb93f928c`.
- Changed only `internal/store/sqlite/migrations.go`, `internal/store/sqlite/migrations_test.go`, and `internal/store/sqlite/checkpoints_test.go`; `internal/store/sqlite/checkpoints.go` is unchanged. The exact frozen-reference comparison uses `git diff` for tracked files and `git show` plus `git diff --no-index` for untracked files: 75 additions and 18 deletions, 93 A+D, below the 180-line boundary.

### Remaining implementation tasks

- [ ] Add checkpoint tests and concrete persistence/adapters in `internal/app/checkpoints.go`, `internal/store/sqlite/`, `internal/adapter/cli/`, `internal/adapter/mcp/`, and `internal/testkit/`; run RED → GREEN → TRIANGULATE → REFACTOR for validated session binding, limits, exact revision references, freshness, explicit resume, disconnect, and fork distinction. <!-- sdd-owner: implementation -->
- [ ] Add cross-cutting tests and close only observed gaps in `internal/testkit/`, `internal/adapter/cli/`, `internal/adapter/mcp/`, `internal/store/sqlite/`, and `internal/revisionfs/`; run RED → GREEN → TRIANGULATE → REFACTOR, then record actual commands, results, and limitations in `docs/evidence/phase1-verification.md` without adding deferred features. <!-- sdd-owner: implementation -->

---

## P1.5b checkpoint adapters — blocked

- Consumed authoritative `apply: ready` repo-local status and allowed root; `actionContext.warnings` was empty. Scope was only checkpoint save/read adapters; no P1.6, dependency, network, child, commit, or review action occurred.
- RED selection: `go test ./cmd/memgraphai ./internal/adapter/mcp -list '^(TestCheckpointCLIProcessUsesHumanAndJSON|TestStdioServerExposesCheckpointSaveAndReadTools)$' -count=1 -timeout=120s`; the run then failed behaviorally with CLI `invalid command` and MCP 19 tools, want 21. GREEN passed that focused pair.
- TRIANGULATE command `go test ./cmd/memgraphai ./internal/adapter/mcp -run '^(TestCheckpointCLIProcessUsesHumanAndJSON|TestStdioServerExposesCheckpointSaveAndReadTools|TestCheckpointAdaptersHaveEquivalentProcessOutcomes|TestMCPMetricEqualsIndependentlyCapturedJSONRPCFrame)$' -count=1 -timeout=120s` failed only in `TestCheckpointAdaptersHaveEquivalentProcessOutcomes`: an affected revision advance omitted explicit `fresh:false`.
- Changed adapter/executable/tests plus `docs/evidence/phase1-checkpoints-adapters.md`; `git diff --check` passed. No full/repeated acceptance PASS is claimed.
- The adapter cannot repair the service/store `omitempty` freshness omission within allowed surfaces; P1.5 remains visibly unchecked and P1.6 is unchanged. Source/test delta is 313 additions + 13 deletions; 20 evidence lines plus this record are below the 1,000 A+D cap.
- Remaining: `- [ ] Add checkpoint tests and concrete persistence/adapters in \`internal/app/checkpoints.go\`, \`internal/store/sqlite/\`, \`internal/adapter/cli/\`, \`internal/adapter/mcp/\`, and \`internal/testkit/\`; run RED → GREEN → TRIANGULATE → REFACTOR for validated session binding, limits, exact revision references, freshness, explicit resume, disconnect, and fork distinction. <!-- sdd-owner: implementation -->`
- Remaining: `- [ ] Add cross-cutting tests and close only observed gaps in \`internal/testkit/\`, \`internal/adapter/cli/\`, \`internal/adapter/mcp/\`, \`internal/store/sqlite/\`, and \`internal/revisionfs/\`; run RED → GREEN → TRIANGULATE → REFACTOR, then record actual commands, results, and limitations in \`docs/evidence/phase1-verification.md\` without adding deferred features. <!-- sdd-owner: implementation -->`
- Post-failure process check `pgrep -x memgraphai | wc -l` returned `0`; failed test cleanup reaped all short-lived CLI/MCP children.

---

## P1.5b checkpoint response DTO correction
- Consumed authoritative `apply: ready` status for the repo-local allowed root; `actionContext.warnings` was empty; scope was the authorized P1.5b correction only.
- Completed P1.5; its persisted implementation row is now `- [x]`. P1.6 remains unchecked.
- Changed `internal/store/sqlite/checkpoints.go`, checkpoint/MCP regression tests, and `docs/evidence/phase1-checkpoints-adapters.md`; no adapter production source, dependency, commit, child, or P1.6 work changed.
| Cycle | RED | GREEN | TRIANGULATE | REFACTOR |
| --- | --- | --- | --- | --- |
| explicit freshness response | stale read omitted `fresh` and focused test failed | response-only DTO passed | CLI/MCP true/false key presence, affected-only staleness, old-ID replay identity, binding/disconnect/resume/fork/tamper passed | request DTO/digest untouched; discovery coverage expects 21 tools |
- Verification PASS: focused SQLite, real CLI, real MCP parity/discovery/frame metrics; parity and frame metrics each `-count=5`; `go test ./... -count=1 -timeout=120s`; `git diff --check`.
- Evidence SHA-256: `9f48bdbb4455b83342f5ac61b7ab4ad7385702aa74ddcf102f43acf32e2a845b`; it preserves the distinct failed SHA `67b6fbe7211c570ff4c7a84c86844bd763353d04f7a0788360fe0d1e9f2097f1`.
- No design deviation: only `ReadCheckpoint` uses the DTO; no model/filesystem restoration, verification claim, list/update/delete, or task-reference feature was added.
- Work-unit boundary: P1.5b only; `dc8de02` scoped source/tests plus 38-line evidence are 342 A+D excluding OpenSpec planning, below the 1,000-line cap; no PR/chain/exception was created.
- Remaining: `- [ ] Add cross-cutting tests and close only observed gaps in `internal/testkit/`, `internal/adapter/cli/`, `internal/adapter/mcp/`, `internal/store/sqlite/`, and `internal/revisionfs/`; run RED → GREEN → TRIANGULATE → REFACTOR, then record actual commands, results, and limitations in `docs/evidence/phase1-verification.md` without adding deferred features. <!-- sdd-owner: implementation -->`

---

## Current-state refresh

- Authoritative task status is **10/11 complete**: Phase 0 and P1.0–P1.5 are complete, and P1.6 is the sole pending task. No final Phase 1 verification is claimed.
- Native phase status is `apply: ready`; `verify` and `archive` remain blocked until P1.6 is completed with actual cross-cutting evidence.
- Fresh validation passed for the focused SQLite checkpoint tests, the real CLI process test, and the focused MCP parity and exact-frame metric tests.
- Both five-run repetitions passed: real CLI/MCP checkpoint parity and independently captured MCP frame metrics.
- `go test ./... -count=1 -timeout=120s` passed for the full Go suite.
- `git diff --check` passed.

---

## P1.6 cross-cutting hardening and verification evidence

### Status, authority, and completed task

- Consumed authoritative `gentle-ai.sdd-status@2`: `apply: ready`, repository-local workspace `/Users/juanmotta/Desktop/personal/memgraphai`, that workspace as the sole allowed edit root, and no action-context warnings.
- The user explicitly authorized only the remaining P1.6 work unit with a 2,000 changed-line budget and deferred chaining. The workload forecast was resolved as this bounded, estimated 220–380-line implementation slice; no PR, chain, `size:exception`, commit, push, or review actor was used.
- Continued native attempt `sha256:e90236059f13ca6a09996fa5aaa94a48516f8a53a830bda183360731a9a97697` with request `p16-apply-1789423145-23972` and received `proceed`. Per instruction, this worker did not settle the parent-owned attempt.
- [x] P1.6 — cross-cutting hardening tests/fixes and `docs/evidence/phase1-verification.md` are complete. The exact implementation-owned row in `tasks.md` was changed to `- [x]` after acceptance and re-read visibly checked. No other task checkbox changed; no implementation task remains unchecked.

### Observed gaps closed

- `internal/app/contract.go` now caps the shared serialized response at 2 MiB. Oversized or malformed successful handler output becomes a small correlated `internal` envelope without partial result/count data, and telemetry records the actual fallback bytes.
- Project and continuity SQLite writes now reuse the established busy classifier instead of leaking raw lock failures upward. Their application outcome mappers preserve stable `busy` and `retryable` outcomes.
- Existing adapter, pagination, fault-injection, recovery-process, checksum, stale-write, idempotency, checkpoint, and metric tests were combined into the P1.6 acceptance matrix. No duplicate feature behavior or deferred capability was added.

### TDD Cycle Evidence

| Task | Test file | Layer | Safety net | RED | GREEN | TRIANGULATE | REFACTOR |
| --- | --- | --- | --- | --- | --- | --- | --- |
| Bounded shared responses | `internal/app/contract_test.go` | Shared contract integration | `go test ./... -count=1 -timeout=120s` PASS before edits | Oversized successful result stayed `ok` with 2,097,167 result bytes instead of a bounded fallback | Focused test PASS after a 2 MiB serialized-envelope guard | Ordinary nonempty success remains `ok`; malformed handler JSON becomes bounded valid `internal` JSON with the same valid operation ID | Named shared limit and one fallback path; `gofmt` and focused package tests PASS |
| Stable project/continuity contention | `internal/app/contract_test.go` with real SQLite connections | Application/SQLite integration | Full suite and focused app/store suites PASS | Project and continuity locked writes both returned `internal`, want `busy` | Both focused subtests PASS after store classification and app outcome mapping | Two service paths under independent locks passed; the cross-cutting app matrix passed five times and existing helper-process exhaustion remained green | Reused `mapRecoveryError`; no second driver-string classifier; focused/full suites PASS |

The first oversized RED printed the entire payload; its test-only diagnostic was narrowed and the same behavioral RED was rerun before production code changed.

### Files changed

- `internal/app/contract.go`, `internal/app/contract_test.go`
- `internal/app/projects.go`, `internal/app/continuity.go`
- `internal/store/sqlite/projects.go`, `internal/store/sqlite/continuity.go`
- `docs/evidence/phase1-verification.md`
- `openspec/changes/backend-foundation-and-continuity/tasks.md`
- `openspec/changes/backend-foundation-and-continuity/apply-progress.md`

### Verification commands and results

| Command | Result |
| --- | --- |
| Initial `go test ./... -count=1 -timeout=120s` | PASS safety net for every Go package; `internal/telemetry` has no standalone tests. |
| New bounded-response focused RED/GREEN commands | Expected behavioral RED, then PASS GREEN. |
| New contention focused RED/GREEN command | Expected behavioral RED in both project and continuity cases, then PASS GREEN. |
| Focused app hardening/pagination/metric matrix with `-count=5` | PASS. |
| Real CLI/MCP continuity, EOF/scope, document, checkpoint, and exact-frame metric matrix | PASS. |
| Focused SQLite operation/recovery/targeted/document/checkpoint matrix | PASS. |
| Focused revision filesystem prepare/read/verify matrix | PASS. |
| `go test ./internal/app ./internal/store/sqlite -count=1 -timeout=120s` | PASS. |
| Final `go test ./... -count=1 -timeout=120s` | PASS. |
| `git diff --check` | PASS. |
| `pgrep -x memgraphai \| wc -l` | `0`; no adapter process remained. |

Evidence SHA-256: `5fbb15e275c34f5c3e31c57de5285027a032f970d8c1281e0e6ba37bad26d66f` for `docs/evidence/phase1-verification.md`.

### Design conformance, workload, risks, and remaining work

- The implementation preserves shared concrete services, stable outcomes, explicit request scope, best-effort telemetry, SQLite/Markdown authority, and the existing no-daemon/no-HTTP boundary.
- Tracked source/test/task changes are 143 additions plus 10 deletions; the new verification evidence is 61 lines. Including this cumulative progress addendum remains far below the user-authorized 2,000 changed-line limit and within the P1.6 work-unit estimate. No delivery exception is needed.
- Deterministic contention uses separate SQLite connections and the configured 100 ms busy timeout; helper-process tests provide separate-process fencing/exhaustion evidence. This is not a throughput target or proof of every scheduler interleaving. Real CLI/MCP parity runs are present, but no synthetic long-held lock was held across both adapters simultaneously.
- Local sync, injected faults, and process termination do not prove physical power-loss, backup, network-filesystem, release-platform, or unsupported macOS behavior.
- No implementation-owned `- [ ]` line remains. The next phase is independent `sdd-verify`; task completion does not itself claim that independent verification result.
