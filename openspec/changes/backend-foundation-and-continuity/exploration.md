# Explore backend foundation and continuity

## Outcome

Roadmap phases 0 and 1 can proceed to proposal without reopening product review. The settled product definition is sufficient to plan a technical spike followed by a bounded CLI/MCP continuity slice. No unresolved user or product decision blocks the proposal.

This exploration is planning evidence only. It does not approve implementation, select dependencies, or claim runtime validation.

## Scope guardrails

| In scope | Out of scope |
| --- | --- |
| Phase 0 evidence for SQLite packaging, identity constraints, revision recovery, and a portable filesystem boundary | Phase 2 FTS retrieval behavior beyond proving FTS5/build viability |
| Phase 1 project, workstream, session, document, checkpoint, and continuity behavior | Tasks, links, semantic retrieval, detailed analytics, backup qualification, desktop/browser work |
| One Go executable with shared concrete application services and CLI/MCP adapters | Daemons, HTTP/IPC infrastructure, generic repositories, plugins, graph databases |
| Human-readable and JSON CLI output plus MCP contract parity | Live or paid client/provider testing without separate consent |
| Confirmed Phase 1 operational telemetry | Promotion of proposed telemetry defaults into settled requirements |

Implementation remains gated on explicit user approval.

## Lean implementation sequence

### Phase 0 — technical spike

The spike should answer technical questions with reproducible evidence, not expand product behavior.

1. **Establish the smallest testable Go boundary.** Define portable seams only where operating-system behavior must be controlled: filesystem durability/fault injection, time, and identifier generation. Keep application services concrete.
2. **Evaluate SQLite packaging.** Compare viable Go SQLite candidates against one-executable macOS packaging, FTS5 availability, transaction behavior, and supported architecture/toolchain evidence. A candidate named in the product document is not preselected.
3. **Exercise identity constraints.** Prototype schema constraints for immutable project IDs, path associations, workstream/session bindings, explicit ambiguity, and non-merging aliases or nested paths.
4. **Exercise revision publication and recovery.** Model durable Markdown preparation followed by a short SQLite current-pointer transaction with expected-revision rejection, operation IDs, unknown-outcome recovery, orphan handling, and targeted checksum discrepancy detection.
5. **Exercise baseline contention and crash windows.** Use deterministic fault injection and concurrent-process cases to characterize bounded waits/retries and stable conflict or retryable outcomes. Record measurements rather than inventing targets.
6. **Record evidence-backed recommendations.** Carry forward only the selected SQLite/build approach, minimal filesystem boundary, schema constraints, and publication/recovery state model needed by Phase 1.

#### Phase 0 acceptance

- FTS5 and the selected SQLite integration are demonstrated in the intended one-binary build path, or a specific blocker and alternative are documented.
- Identity and binding constraints reject ambiguous, inconsistent, and silently merging cases in deterministic tests.
- Every modeled filesystem/database interruption leaves a classifiable state with an idempotent recovery path; unknown commit outcomes are never reported as rollback.
- Stale expected revisions cannot replace newer current revisions.
- Targeted checksum checks detect direct alteration of served current and historical Markdown fixtures.
- Contention experiments end in bounded success, conflict, or retryable/busy outcomes rather than raw lock errors or indefinite waits.
- macOS/toolchain/filesystem observations are recorded as evidence; unsupported architecture, minimum-version, signing, or packaging claims remain undecided.

### Phase boundary

Phase 1 starts only after Phase 0 records an evidence-backed SQLite/build choice and a coherent publication/recovery model. Phase 0 proves FTS5 viability but does not deliver FTS search, section identity, index publication, or rebuild behavior; those remain Phase 2.

### Phase 1 — CLI/MCP continuity slice

Build one behavior at a time through the shared application service and both adapters, keeping contract tests transport-neutral and adapter tests focused on normalization and serialization.

1. **Shared operation contract and store lifecycle.** Establish operation IDs, stable outcome categories, explicit scope inputs, noninteractive behavior, bounded response conventions, schema migration/opening, and confirmed Phase 1 telemetry fields. Telemetry counter failure remains independent of operation success.
2. **Project identity through both adapters.** Create, list, and explicitly resolve stable projects and path associations. Prove ambiguity, moved/unavailable path, alias, symlink, and nested-association outcomes without guessing.
3. **Workstream and session continuity.** Create/list workstreams; open and explicitly close sessions with provenance; validate project/workstream/session consistency. A disconnect does not close a session, and one MCP connection carries no mutable authorization-routing context shared across logical sessions.
4. **Revisioned documents.** Create, update, list, and read project-general or explicitly selected workstream documents. Publish prepared Markdown by committing the current pointer with expected-revision checks; preserve history and expose integrity discrepancies.
5. **Checkpoint, resume, and fork.** Save bounded checkpoints only under a validated session binding. Resume an explicitly selected workstream into a new run without recency fallback; fork remains observably distinct and does not mutate the source workstream.
6. **Cross-adapter durability hardening.** Run the same fixtures against CLI JSON and MCP envelopes, plus human-readable CLI checks. Exercise concurrent CLI/MCP writers, retries, crashes, idempotent recovery, bounded pagination, and response-size accounting.

#### Phase 1 acceptance

- CLI and MCP call the same concrete application behavior and return semantically equivalent envelopes, stable errors, scope enforcement, and protections for each operation exposed through both.
- Project, workstream, and session identities remain distinct; missing or inconsistent required bindings are rejected without recency, environment, path, or connection-global authorization fallback.
- Two logical sessions can interleave over one MCP connection without scope mixing, and only explicit close marks a session ended.
- Project-only reads return project-general documents; workstream content requires explicit permitted selection, while workstream listing reveals metadata only.
- Document acknowledgement occurs only after the current-pointer transaction commits; precommit failure preserves the prior current revision, stale writes conflict, and unknown outcomes resolve through operation identity.
- Concurrent CLI and MCP access produces bounded stable outcomes and never silently overwrites a newer revision.
- Checkpoints cite current-state references without claiming filesystem/model snapshots or verification; explicit resume and fork preserve their distinct semantics.
- Confirmed Phase 1 telemetry records operation ID, timestamp, interface, declared scope, status, total backend duration, serialized response bytes, and available result counts without storing raw content by necessity.
- Deterministic contract and protocol-client tests provide acceptance evidence. Live Claude/Codex smoke tests remain separately consented and are not CI requirements.
- Core operation has no daemon, HTTP, Node, embeddings, FTS retrieval, task system, or new infrastructure dependency.

## Decision classification

### User/product decisions required before proposal

None. The proposal can preserve all confirmed requirements, keep proposed defaults labeled as proposals, and make technical selections conditional on Phase 0 evidence.

### Technical hypotheses for Phase 0 evidence

| Hypothesis | Required evidence |
| --- | --- |
| A Go SQLite integration can provide FTS5 in the intended macOS one-binary path. | Build/package matrix and executable feature probe. |
| Short SQLite transactions can enforce identity bindings and stale-revision rejection under baseline concurrency. | Constraint, transaction, and concurrent-process tests. |
| Durable file preparation plus a database current pointer can recover every modeled crash window without false success or false rollback. | Fault-injection state matrix and idempotent recovery tests. |
| A small portable filesystem boundary is enough for durability and checksum testing without abstracting all storage. | Concrete spike implementation and portability notes. |
| Bounded lock handling can map exhaustion to stable application outcomes. | Measured contention tests with explicit retry/wait bounds proposed from evidence. |

### Later decisions that do not block this proposal

- Supported macOS architectures, minimum version, signing, and release packaging should follow Phase 0 evidence and are not invented here.
- Exact session interruption statuses, permission UX, envelope field names/versioning, pagination tokens, and telemetry retention/placement need later specification or design before their relevant implementation work.
- Frontmatter ownership and exact file publication protocol need Phase 0 evidence and design; Markdown prose and SQLite operational authority are already settled.
- Section identity, FTS publication/rebuild, tasks, detailed analytics, embeddings, cost controls, purge residue policy, and backup qualification belong to later roadmap phases.

## Risks and planning implications

- Phase 0's 650–1,150-line estimate fits within the 2,000 changed-line review budget. Phase 1's 1,220–2,120-line estimate may exceed it, and the combined 1,870–3,270-line scope still carries high budget risk. The proposal may define the outcome, but task planning must pause for the configured `ask-on-risk` delivery decision before implementation slicing; no chain strategy or size exception is inferred.
- SQLite/filesystem durability claims are the highest technical risk and must remain evidence-gated.
- Adapter parity can drift if CLI or MCP owns business defaults; normalization must end before invoking shared services.
- Broad permission, telemetry, purge, or FTS design would leak later phases into this change and should be excluded.

## Recommended next step

Create the proposal for `backend-foundation-and-continuity` with only Phases 0 and 1, explicit implementation approval gating, Phase 0 evidence as the entry condition for Phase 1, and the review-budget delivery decision deferred until implementation task sizing exposes the review slices.
