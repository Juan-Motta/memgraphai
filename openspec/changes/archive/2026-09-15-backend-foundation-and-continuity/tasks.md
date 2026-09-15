# Backend foundation and continuity tasks

Phase 0 is complete, and Phase 1 tasks P1.1–P1.5 are implemented. P1.6 remains pending. Scope excludes FTS retrieval, automatic context assembly, purge implementation, embeddings, daemons, HTTP, and provider integrations.

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | Phase 0: 650–1,150; conditional Phase 1: 1,220–2,120; combined: 1,870–3,270 |
| 2,000-line combined-scope budget risk | High |
| Chained PRs recommended | Yes, as an optional delivery approach |
| Suggested split | Phase 0 evidence slices → Phase 1 contracts/store → identity/adapters → continuity → documents → checkpoints/hardening |
| Delivery strategy | ask-on-risk |
| Chain strategy | pending |

Decision needed before apply: Yes
Chained PRs recommended: Yes, as an optional delivery approach
Chain strategy: pending
2,000-line combined-scope budget risk: High

Phase 0's 650–1,150-line estimate fits within the 2,000-line budget. Conditional Phase 1's 1,220–2,120-line estimate may exceed it, and the combined 1,870–3,270-line estimate retains high budget risk. The suggested smaller cohesive slices are optional recommendations, not a requirement that each work unit stay below 2,000 lines; no chain strategy or size exception is selected. The parent must obtain the `ask-on-risk` delivery decision before implementation if review-budget risk remains.

## Phase 0 — foundation evidence

**Dependency:** none beyond the accepted proposal, specifications, and design. Go, dependencies, SQLite/FTS5, build behavior, and the test runner are unverified; no toolchain is claimed to work. Tests and code for each work unit stay together. The root module and dependency-lock paths are `go.mod` and `go.sum`.

### P0.1 — Prove the executable and SQLite/FTS5 path

**Estimated diff:** 120–220 lines. **Start:** `go.mod`, `go.sum`, a selected driver, and a verified runner are absent or unverified. **Finish:** root module/lock paths `go.mod` and `go.sum` exist, a working runner is verified, and `docs/evidence/phase0-build.md` records candidate comparison, tested build conditions, executable shape, transaction behavior, the FTS5 compile-option check, and a real FTS5 create/query probe—or a prerequisite blocker with an evaluated alternative. **Verify:** record only tested OS, architecture, toolchain, and build conditions. **Rollback:** remove the module/probe and evidence as one unit.

- [x] Establish the root module and dependency-lock paths (`go.mod`, `go.sum`) and verify the proposed runner before any behavioral RED; then, only if that setup succeeds, build the smallest candidate probes and accompanying failing/contrasting tests in `cmd/memgraphai/`, `internal/store/sqlite/`, and `internal/testkit/`, run RED → GREEN → TRIANGULATE → REFACTOR, and record results in `docs/evidence/phase0-build.md` without preselecting a driver or claiming unsupported macOS targets. <!-- sdd-owner: implementation -->

**TDD evidence:** Missing Go, imports, build tools, or runner is a prerequisite setup failure recorded separately, never behavioral RED. After setup works, RED captures true probe/behavior failures; GREEN proves only the smallest executable/SQLite path; TRIANGULATE compares viable candidates/build conditions and pairs compile-option evidence with a real FTS5 probe; REFACTOR isolates the probe and removes speculative abstractions.

**Acceptance:** Phase 1 remains blocked unless one executable plus SQLite/FTS5 is evidenced, or a specific blocker and evaluated alternative are recorded.

### P0.2 — Establish identity and binding constraints

**Estimated diff:** 120–200 lines. **Depends on:** P0.1. **Start:** identity rules exist only in design. **Finish:** `internal/domain/`, `internal/store/sqlite/`, and `internal/testkit/` demonstrate immutable project IDs, path associations, workstream/session relationships, explicit scope, and binding rejection. **Verify:** results distinguish none/one/many and reject silent merges, guesses, and inconsistent relationships. **Rollback:** revert the domain/schema experiment together.

- [x] Add identity constraint tests and the smallest concrete domain/SQLite behavior in `internal/domain/`, `internal/store/sqlite/`, and `internal/testkit/`; run RED → GREEN → TRIANGULATE → REFACTOR for aliases, symlinks, nested/unavailable/moved paths, same-named projects, cross-project workstreams, mismatched sessions, and association-versus-authorization, recording results in `docs/evidence/phase0-identity.md`. <!-- sdd-owner: implementation -->

**TDD evidence:** RED fails on the ambiguity and binding cases; GREEN preserves stable IDs and explicit ambiguity/unavailable outcomes; TRIANGULATE proves no environment, current directory, deepest-prefix, or recency fallback selects scope; REFACTOR consolidates typed outcomes without a generic repository or unapproved public API.

**Acceptance:** Valid bindings remain distinct; ambiguous, unavailable, unauthorized, and inconsistent combinations are observable and never silently selected.

### P0.3 — Prove durable revision preparation and publication

**Estimated diff:** 180–300 lines. **Depends on:** P0.1–P0.2. **Start:** no filesystem or publication behavior is verified. **Finish:** `internal/revisionfs/`, `internal/store/sqlite/`, and `docs/evidence/phase0-publication.md` demonstrate durable Markdown preparation followed by a short expected-revision SQLite commit. **Verify:** precommit failure preserves the prior current revision and success follows pointer commit. **Rollback:** delete experiment fixtures/staging and revert this unit.

- [x] Add fault-injection tests and the narrow filesystem/publication implementation in `internal/testkit/`, `internal/revisionfs/`, and `internal/store/sqlite/`; run RED → GREEN → TRIANGULATE → REFACTOR across prepare/sync/no-clobber, expected-revision rejection, pointer commit, response loss, first publication, replacement, and preparation failure, recording evidence in `docs/evidence/phase0-publication.md`. <!-- sdd-owner: implementation -->

**TDD evidence:** RED names every publication interruption boundary; GREEN uses ID-based immutable revision paths and keeps SQLite operational metadata authoritative; TRIANGULATE covers stale writers, existing destinations, and missing prior revisions; REFACTOR preserves short transactions and removes duplicate setup.

**Acceptance:** Prepared content is not current before commit; stale writes return stable conflict; precommit failure leaves the prior current revision unchanged.

### P0.4 — Prove recovery, integrity, idempotency, and contention

**Estimated diff:** 230–430 lines. **Depends on:** P0.3. **Start:** crash states and lock behavior are unclassified. **Finish:** `internal/store/sqlite/`, `internal/revisionfs/`, `internal/testkit/`, and `docs/evidence/phase0-recovery.md`, `docs/evidence/phase0-concurrency.md`, and `docs/evidence/phase0-exit.md` classify operation states, unknown outcomes, orphans, checksum discrepancies, fenced recovery, bounded retries, and cross-process contention. **Verify:** authorized retries are idempotent where proven; exhaustion is stable and bounded. **Rollback:** remove recovery/concurrency fixtures, ledger changes, and the exit record together.

- [x] Add restart, checksum, orphan, takeover, and multi-process tests with the minimum recovery/ownership/retry implementation in `internal/testkit/`, `internal/store/sqlite/`, and `internal/revisionfs/`; run RED → GREEN → TRIANGULATE → REFACTOR for unknown commits, lost responses, altered/missing files, active staging, stale writers, simultaneous retries, idempotency mismatch, authorization recheck, committed replay after later revision advancement, and retry exhaustion, then record recovery, concurrency, and Phase 0 exit evidence. <!-- sdd-owner: implementation -->

**TDD evidence:** RED covers each crash-table state, raw-lock failure, different-fingerprint reuse, unauthorized replay, and replay after a later current revision; GREEN adds operation fingerprint/idempotency checks, fenced ownership, targeted verification, orphan reporting, and stable busy/retryable/conflict/unknown mapping; TRIANGULATE varies process interleavings, takeover timing, authorization, fingerprints, and later revision advancement; REFACTOR keeps timing policy explicit and documents driver/filesystem limits.

**Acceptance:** Recovery never infers rollback from an unknown commit, overwrites immutable files, cleans active staging, leaks raw lock errors, silently overwrites newer revisions, or waits indefinitely. A reused operation ID with a different fingerprint returns `idempotency_mismatch` without execution; replay rechecks authorization; an authorized replay of a committed operation returns its stored result without republishing even after a later revision advances. `docs/evidence/phase0-exit.md` is the owned P0.4 output and states whether Phase 1 is viable. If the accumulated implementation candidate exceeds 2,000 changed lines, stop for the configured delivery decision rather than compressing evidence or tests.

### Phase 0 exit record

P0.4 owns `docs/evidence/phase0-exit.md`, linking the four evidence areas and stating whether the SQLite/build path and recovery model are viable. An incomplete or failed gate blocks Phase 1 and does not weaken confirmed invariants.

## Phase 1 — conditional continuity slice

After P0.4 records a viable Phase 0 exit, P1.0 may run as contract decision/design work. Only after P1.0 resolves the contracts and the user separately approves implementation may behavioral Phase 1 tasks P1.1–P1.6 begin. The approval gate is prose, not an implementation checkbox.

### P1.0 — Resolve contracts required before Phase 1 behavior

**Estimated diff:** 80–160 lines. **Depends on:** `docs/evidence/phase0-exit.md`; no implementation approval is needed to prepare the contract decision/design record. **Start:** permission configuration, operation envelope/versioning, MCP library, pagination tokens, and session interruption statuses remain open. **Finish:** `docs/evidence/phase1-contracts.md` records only the choices required by Phase 1 and their compatibility boundaries. **Verify:** P1.1 can express real behavior tests against the record. **Rollback:** revert the contract record and keep P1.1–P1.6 blocked.

- [x] Resolve the permission, exact envelope/versioning, MCP library/build, pagination-token, and session-interruption contracts in `docs/evidence/phase1-contracts.md` only, documenting the future responsibilities of `internal/app/contract.go` and both adapters without writing source code; do not add ceremonial tests of prose completeness, and hand real failing contract-behavior cases to P1.1 after concrete decisions while preserving noninteractive errors and semantic parity. <!-- sdd-owner: implementation -->

**TDD evidence handoff:** P1.0 is contract decision/design, not a prose-test work unit. RED begins in P1.1 against the concrete decisions; GREEN implements the minimum behavior; TRIANGULATE contrasts missing, ambiguous, denied, stale, and paginated inputs across adapters; REFACTOR removes duplicate mapping and leaves later-roadmap decisions out of scope.

**Acceptance:** Permission, envelope, MCP-library, pagination, and interruption-status contracts are explicit enough to implement and test; adapters cannot invent alternate semantics.

### P1.1 — Add the concrete application/store foundation and telemetry

**Estimated diff:** 180–300 lines. **Depends on:** P0 exit, P1.0, and the evidenced SQLite path. **Start:** Phase 0 has established the root `go.mod`/`go.sum` module and probe; the shared service graph is absent. **Finish:** `cmd/memgraphai/main.go`, `internal/app/contract.go`, `internal/store/sqlite/`, `internal/telemetry/`, and `internal/testkit/` provide ordered schema opening, operation context, stable outcomes, limits, authorization boundaries, and best-effort metrics. **Verify:** metric failure cannot alter business success and raw content is absent. **Rollback:** revert this foundation, migration, tests, and evidence together.

- [x] Add tests and concrete wiring in `go.mod`, `cmd/memgraphai/main.go`, `internal/app/contract.go`, `internal/store/sqlite/`, `internal/telemetry/`, and `internal/testkit/`; run RED → GREEN → TRIANGULATE → REFACTOR for scope, outcomes, migration versioning, limits, required metrics, response bytes, and isolated metric failure. <!-- sdd-owner: implementation -->

**Accepted implementation exception:** The user explicitly accepted the historical TESTONLY metric-isolation RED deficiency for P1.1 only. The deficiency remains documented and is not valid production RED or a waiver of future strict TDD. The independent nil-handler RED/GREEN correction and prior repeated verification do not rewrite that history; fresh verification and native review remain parent-owned and pending.

**TDD evidence:** RED exposes missing scope/metric invariants; GREEN keeps SQLite imports in `internal/store/sqlite/` and adapters out of business logic; TRIANGULATE covers success, invalid, denied, conflict, retryable, and unknown results through CLI/MCP-shaped requests; REFACTOR keeps migration application minimal and the executable usable alone.

**Acceptance:** Shared requests carry explicit scope; telemetry is best effort; the executable requires no daemon or Node runtime.

### P1.2 — Deliver projects and adapter parity

**Estimated diff:** 160–280 lines. **Depends on:** P1.1. **Start:** foundation contracts exist but no project operation is exposed. **Finish:** `internal/app/projects.go`, `internal/adapter/cli/`, `internal/adapter/mcp/`, and common fixtures support project create/list, associations, and explicit resolution. **Verify:** human/JSON CLI and MCP express equivalent semantics without prompting or guessing. **Rollback:** revert project behavior and adapter tests together.

- [x] Add shared-service, CLI, and MCP tests plus concrete project/path operations in `internal/app/projects.go`, `internal/adapter/cli/`, `internal/adapter/mcp/`, and `internal/testkit/`; run RED → GREEN → TRIANGULATE → REFACTOR for identity, aliases, ambiguity/unavailability, nested/symlink paths, and noninteractive errors. <!-- sdd-owner: implementation -->

**TDD evidence:** RED fails on ambiguous and denied requests; GREEN reuses P0 identity constraints; TRIANGULATE compares identical valid/missing/ambiguous/denied requests across adapters and proves listing does not grant content access; REFACTOR keeps parsing/rendering out of services.

**Acceptance:** Project IDs survive mutable names/paths and both adapters return equivalent explicit outcomes.

**Independent P1.2 verification:** PASS for bounded association continuation/rebinding/staleness, exact independently captured MCP frame metrics, deterministic child reaping, repeated real CLI/MCP parity, and the full suite. P1.2 implementation is complete; native review remains pending. P1.3–P1.5 are implemented, and P1.6 remains unchecked.

### P1.3 — Deliver workstreams and explicit session continuity

**Estimated diff:** 180–320 lines. **Depends on:** P1.2 and P1.0 session-status contract. **Start:** projects can be selected but continuity is absent. **Finish:** `internal/app/continuity.go`, SQLite entities, `internal/adapter/cli/`, and `internal/adapter/mcp/` support workstream create/list/fork and session open/validate/explicit close/resume. **Verify:** declared bindings are enforced; disconnect does not close or reuse a session. **Rollback:** revert this unit without reverting project identity.

- [x] Add continuity tests and concrete workstream/session persistence in `internal/app/continuity.go`, `internal/store/sqlite/`, `internal/adapter/cli/`, `internal/adapter/mcp/`, and `internal/testkit/`; run RED → GREEN → TRIANGULATE → REFACTOR for mismatches, missing sessions, explicit close, disconnect, interleaved logical MCP sessions, resume, and fork. <!-- sdd-owner: implementation -->

**TDD evidence:** RED fails on missing/inconsistent binding; GREEN uses declared scope and transport-local connection state; TRIANGULATE interleaves two MCP sessions and contrasts resume, fork, same-worktree streams, and newer unrelated workstreams; REFACTOR centralizes binding validation and preserves the source on fork.

**Acceptance:** Only explicit close ends a session; resume creates a new run for an explicit workstream; fork is distinct and source-preserving.

### P1.4 — Deliver revisioned project/workstream documents

**Estimated diff:** 260–440 lines. **Depends on:** P1.3, P0.3–P0.4, and P1.0. **Start:** publication evidence exists but document operations are absent. **Finish:** `internal/app/documents.go`, `internal/revisionfs/`, `internal/store/sqlite/`, and both adapters support exact current/history reads and project-general or explicitly selected workstream publication. **Verify:** provenance, history, stale conflicts, durable publication, and checksum discrepancies are equivalent across adapters. **Rollback:** revert service, schema additions, adapters, and tests together; preserve the prior current revision.

- [x] Add document tests and concrete services in `internal/app/documents.go`, `internal/revisionfs/`, `internal/store/sqlite/`, `internal/adapter/cli/`, `internal/adapter/mcp/`, and `internal/testkit/`; run RED → GREEN → TRIANGULATE → REFACTOR for scope-only reads, history, stale writes, precommit failure, response loss, altered files, idempotent replay, and orphan reporting, without adding FTS retrieval. <!-- sdd-owner: implementation -->

**TDD evidence:** RED names publication and scope invariants; GREEN applies the proven immutable-file/current-pointer protocol; TRIANGULATE exercises project-general versus workstream documents, concurrent writers, and integrity discrepancies through both adapters; REFACTOR removes duplicate scope/publication mapping. If the accumulated implementation candidate exceeds 2,000 lines, stop for the configured `ask-on-risk` delivery decision rather than requiring this work unit to be split or weakening tests.

**Acceptance:** Acknowledgement follows pointer commit; stale writes cannot replace newer revisions; exact reads never broaden scope or invoke FTS/context assembly.

### P1.5 — Deliver bounded checkpoints and continuity handoffs

**Estimated diff:** 140–240 lines. **Depends on:** P1.3–P1.4. **Start:** sessions and revisions work independently. **Finish:** `internal/app/checkpoints.go`, `internal/testkit/`, and both adapters save/read bounded checkpoint handoffs referencing exact revisions and distinguish resume from fork. **Verify:** no model/filesystem restoration or completed-verification claim is emitted. **Rollback:** revert checkpoint behavior and fixtures without altering document history.

- [x] Add checkpoint tests and concrete persistence/adapters in `internal/app/checkpoints.go`, `internal/store/sqlite/`, `internal/adapter/cli/`, `internal/adapter/mcp/`, and `internal/testkit/`; run RED → GREEN → TRIANGULATE → REFACTOR for validated session binding, limits, exact revision references, freshness, explicit resume, disconnect, and fork distinction. <!-- sdd-owner: implementation -->

**TDD evidence:** RED fails on invalid bindings, oversized content, and implicit selection; GREEN stores bounded prose/references under the established authority boundary; TRIANGULATE reads after newer revisions, unrelated activity, and disconnect; REFACTOR reuses continuity validation and keeps freshness explicit.

**Acceptance:** Checkpoints are bounded scoped handoffs; resume selects an explicit workstream/new run; fork remains separate.

### P1.6 — Harden the slice and record verification evidence

**Estimated diff:** 220–380 lines. **Depends on:** P1.1–P1.5. **Start:** each feature unit passes focused checks. **Finish:** `internal/testkit/`, both adapters, `internal/store/sqlite/`, `internal/revisionfs/`, and `docs/evidence/phase1-verification.md` cover parity, crash restart, integrity, idempotency, pagination, and concurrent processes. **Verify:** use `go test ./...` only if the apply phase establishes a configured runner; record toolchain limitations instead of claiming success. **Rollback:** remove hardening-only fixtures/evidence without reverting independently usable units.

- [x] Add cross-cutting tests and close only observed gaps in `internal/testkit/`, `internal/adapter/cli/`, `internal/adapter/mcp/`, `internal/store/sqlite/`, and `internal/revisionfs/`; run RED → GREEN → TRIANGULATE → REFACTOR, then record actual commands, results, and limitations in `docs/evidence/phase1-verification.md` without adding deferred features. <!-- sdd-owner: implementation -->

**TDD evidence:** RED covers bounded responses, pagination edges, scope mixing, raw lock leakage, metric failure, crash windows, stale writes, and unknown outcomes; GREEN fixes observed gaps; TRIANGULATE combines deterministic fixtures, protocol clients, fault injection, and multi-process contention; REFACTOR removes duplication and unsupported claims.

**Acceptance:** CLI and MCP provide equivalent explicit-scope semantics, durable revisions, bounded continuity, isolated telemetry failure, and stable contention outcomes.

## Verification and planning guard

The Go runner is verified, and current focused and full-suite evidence supports completed implementation tasks through P1.6. `docs/evidence/phase1-verification.md` records the cross-cutting apply evidence; independent SDD verification remains a separate phase and is not claimed by task completion.
