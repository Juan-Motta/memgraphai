# Establish the backend foundation and continuity slice

## Intent

Plan roadmap Phases 0 and 1 for MemGraph AI: first obtain reproducible evidence for the storage and packaging foundations, then deliver an early complete continuity slice through CLI and MCP. This proposal authorizes planning only. Implementation requires explicit user approval after design and tasks are reviewed.

`docs/PROJECT.md` revision 0.4.1 remains the product authority. Confirmed requirements stay binding; proposed defaults remain proposals; open technical decisions are resolved only with phase-appropriate evidence.

## Scope

### Phase 0 — foundation evidence

- Establish the smallest testable Go boundary, adding seams only for filesystem durability and fault injection, time, and identifier generation.
- Evaluate Go SQLite options for a macOS one-executable path, including an executable FTS5 feature probe; do not preselect a candidate.
- Exercise constraints for stable project identity, path associations, workstream/session bindings, ambiguity, aliases, symlinks, and nested paths.
- Model durable Markdown preparation followed by a short SQLite current-revision transaction with expected-revision rejection.
- Classify crash windows and unknown commit outcomes through operation IDs, idempotent recovery, orphan handling, and targeted checksum checks.
- Measure bounded concurrency behavior and record evidence without inventing performance targets or unsupported platform claims.

### Phase 0 exit gate

Phase 1 may begin only after Phase 0 records an evidence-backed SQLite/build choice, minimal portable filesystem boundary, identity constraints, and coherent publication/recovery model. If FTS5 or one-binary viability fails, record the blocker and evaluated alternative before seeking approval to continue.

### Phase 1 — continuity vertical slice

- Provide shared concrete application services and operation contracts used by CLI and MCP adapters.
- Support project identity and path associations; workstream creation/listing; session open/explicit close; and validated project/workstream/session bindings.
- Support revisioned project-general and explicitly selected workstream documents with durable publication, history, stale-write conflicts, and integrity discrepancy reporting.
- Support bounded checkpoints, explicit resume into a new run, and observably distinct fork behavior without recency fallback.
- Provide human-readable and JSON CLI output plus semantically equivalent MCP envelopes for operations exposed through both adapters.
- Record confirmed Phase 1 telemetry: operation ID, timestamp, interface, declared scope, status, total backend duration, serialized response bytes, and result counts where available. Counter failure must not change operation success.
- Establish durability and concurrency behavior for simultaneous CLI and MCP processes, including bounded retries and stable busy, retryable, conflict, and unknown-outcome results.

## Affected areas

| Area | Planned effect |
| --- | --- |
| Runtime | One Go executable for macOS-first use; CLI exits after work and MCP lifetime follows its connection. |
| Application core | Concrete transport-neutral services own validation, scope, authorization boundaries, and business behavior. |
| Persistence | SQLite owns operational state; Markdown owns document prose; revisions and checksums preserve provenance. |
| Interfaces | CLI and MCP normalize inputs before shared services and preserve semantic parity. |
| Validation | Deterministic contract, protocol-client, fault-injection, and concurrent-process evidence. |

## Observable success criteria

- The intended one-binary path demonstrates the selected SQLite integration and FTS5 availability, or documents a specific blocker and alternative.
- Deterministic evidence rejects ambiguous identity resolution, inconsistent bindings, silent merges, and stale revision replacement.
- Every modeled interruption is classifiable and recoverable idempotently; an unknown commit outcome is never reported as rollback.
- Targeted checks detect direct alteration of served current and historical Markdown fixtures.
- CLI and MCP expose equivalent shared semantics, stable outcomes, explicit scope enforcement, and protections.
- Interleaved logical sessions on one MCP connection cannot mix scope; only explicit close ends a session.
- Project-only reads return project-general content, while workstream content requires explicit permitted selection.
- A document is acknowledged only after its current pointer commits; precommit failure preserves the previous current revision.
- Concurrent access terminates in bounded success or stable conflict/retryable outcomes, never silent overwrite, raw lock leakage, or indefinite wait.
- Checkpoints, resume, and fork preserve their distinct continuity semantics without claiming model/filesystem restoration or verification.
- Deterministic tests provide evidence; no live Claude/Codex integration is required for CI or run without separate consent.

## Non-goals

- Phase 2 FTS search, section identity, index publication, bounded context retrieval, or rebuild behavior beyond Phase 0 viability evidence.
- Tasks, links, detailed analytics, embeddings, external providers, cost controls, purge-policy completion, or backup/release qualification.
- Daemons, HTTP/IPC infrastructure, Node, browser/desktop UI, graph databases, plugins, generic repositories, or autonomous SDD orchestration.
- Selecting unsupported macOS targets, signing policy, numeric SLOs, exact public API names, or telemetry retention defaults in this proposal.

## Risks and mitigations

| Risk | Mitigation |
| --- | --- |
| SQLite/filesystem publication cannot be atomic. | Gate Phase 1 on a fault-injected, idempotent recovery model and never infer rollback from an unknown commit result. |
| SQLite or FTS5 packaging conflicts with one-executable delivery. | Require an executable Phase 0 probe and document blockers before selecting the integration. |
| CLI/MCP behavior or scope drifts. | Keep defaults and normalization in shared contracts/services; verify both adapters with common fixtures. |
| Concurrency leaks raw storage behavior or loses revisions. | Use short transactions, expected revisions, measured bounded waits/retries, and stable application outcomes. |
| Later-roadmap concerns expand the slice. | Enforce the non-goals and retain unresolved later decisions for their roadmap phases. |
| Phase 1 may exceed the 2,000-line review budget, and the combined Phase 0/Phase 1 scope still carries high budget risk. | During task planning, pause under `ask-on-risk` for a delivery decision; do not infer chaining or a size exception. |

## Rollback and recovery

Because this proposal is planning-only, rollback is deletion or revision of its planning artifacts. During any separately approved implementation, each reviewable slice must preserve the prior valid current revision, use reversible schema evolution where feasible, and include recovery evidence before it becomes a dependency. A failed Phase 0 hypothesis stops Phase 1 rather than weakening confirmed invariants.

## Approval gate

No apply work, dependency installation, runtime testing, commit, paid call, or live integration is authorized by this proposal. Complete specifications, design, and review-sized tasks first; obtain explicit user approval before apply. If task sizing exposes review-budget risk, ask the user for the delivery strategy before implementation slicing.
