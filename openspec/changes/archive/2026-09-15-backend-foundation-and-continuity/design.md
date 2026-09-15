# Design: prove the foundation, then deliver continuity

## Decision summary

This change has two gates. Phase 0 is an evidence-producing spike; Phase 1 may start only if that evidence supports a SQLite driver/build path and a recoverable Markdown publication protocol. No recommendation below is approved for implementation, and no dependency, build, runtime, or platform claim has been verified.

| Topic | Design recommendation |
| --- | --- |
| Shape | **Proposed pending approval:** one Go module at this standalone repository root, one executable, concrete application services, and thin CLI/MCP adapters. |
| Persistence | **Proposed pending Phase 0 evidence:** one SQLite store package selected after the driver spike, plus a narrow durable-files boundary for immutable Markdown revisions. |
| Publication | **Proposed pending Phase 0 evidence:** register write intent, durably prepare an immutable revision file, then atomically commit revision metadata, the current pointer, and operation outcome in SQLite. |
| Scope | **Proposed pending approval:** every service method receives an explicit operation context and operation-specific scope; adapters hold no business scope or mutable “current session.” |
| Delivery | Phase 0 produces evidence only. Phase 1 is conditional and remains blocked on explicit implementation approval. |

Phase 1 has no FTS retrieval, section identity, automatic context assembly, tasks, embeddings, purge system, daemon, HTTP service, or speculative provider framework.

## Proposed Go boundaries

```text
go.mod
cmd/memgraph/main.go                 # process wiring; CLI or MCP stdio mode
internal/app/
├── contract.go                      # operation context, scope, outcomes, page bounds
├── projects.go                      # ProjectService
├── continuity.go                    # WorkstreamService and SessionService
├── documents.go                     # DocumentService and publication orchestration
└── checkpoints.go                   # CheckpointService, resume, and fork
internal/domain/                     # IDs, entities, binding rules, typed errors
internal/store/sqlite/
├── store.go                         # concrete SQL persistence and transactions
└── migrations/0001_foundation.sql   # embedded, forward-only initial schema
internal/revisionfs/                 # durable prepare, verify, reconcile; OS seam
internal/adapter/cli/                # parsing and human/JSON rendering only
internal/adapter/mcp/                # connection-owned stdio protocol only
internal/telemetry/                  # best-effort operation accounting
internal/testkit/                    # fault points, fixtures, clock/ID controls
```

The SQLite import is confined to `internal/store/sqlite`; its exact driver follows the Phase 0 comparison. `revisionfs` is the only portability interface proposed now because file sync, rename, path inspection, and injected crashes need control. Time and ID generation are the only other injected seams. Application services and the SQLite store remain concrete; there is no generic repository, transport abstraction, plugin system, or service locator.

`main.go` opens the same store and service graph for both modes. A CLI command performs one operation and exits. MCP lives only for its client connection, and each MCP request carries its own declared scope.

## Minimal data model and authority

| Entity | Minimal relationships and constraints |
| --- | --- |
| `project` | Immutable `project_id`; mutable display name; zero-to-many aliases and path associations. Names and paths are not identity. |
| `project_path` | Belongs to one project; stores supplied and normalized/resolved forms plus availability evidence. Resolution returns explicit none/one/many outcomes. |
| `workstream` | Belongs to one project; optional `forked_from_workstream_id` must reference the same project. |
| `session` | Belongs to one workstream and its project; explicit lifecycle state and provenance; only explicit close sets ended. |
| `document` | Belongs to a project and exactly one content scope: project-general or one workstream in that project. Holds the SQLite current-revision pointer. |
| `revision` | Belongs to one document; immutable ID, predecessor, file path, checksum, byte count, and provenance; uniqueness prevents duplicate sequence/current publication. |
| `checkpoint` | Belongs to one validated session/workstream; references a checkpoint-kind document revision plus bounded structured references. |
| `operation` | Unique operation ID, request fingerprint, state, target revision, terminal outcome, and fenced preparation/recovery ownership generation for write recovery. |
| `operation_metric` | Best-effort accounting keyed by operation ID; it is not consulted for authorization or business success. |

SQLite exclusively owns identities, relationships, lifecycle states, aliases/paths, provenance fields, revision visibility, checksums, and operation recovery. Markdown exclusively owns document/checkpoint prose.

**Proposed pending evidence:** Phase 1 revision files contain Markdown prose only and no authoritative YAML frontmatter. Operational metadata is not duplicated into editable frontmatter. A future generated header or sidecar would be a non-authoritative projection and could not be used to repair SQLite. Recovery instead uses the operation ledger, deterministic immutable path, and checksum. This keeps prose ownership separate from SQLite recovery ownership.

**Proposed revision layout:**

```text
<library>/projects/<project-id>/documents/<document-id>/
└── revisions/<revision-id>.md
<library>/.staging/<operation-id>/<ownership-generation>/<revision-id>.tmp
```

IDs, not display names or paths, determine placement. Published revision files are immutable. Out-of-band edits are discrepancies; an explicit import creates a new revision rather than changing database facts. Schema migrations are embedded numbered SQL files; Phase 1 begins with `0001_foundation.sql` and does not build a migration framework beyond ordered application and recorded schema version.

## Scope and service contracts

Every call receives `OperationContext { operation_id, interface, actor_provenance, declared_scope }`. A declared scope is one of library, project-general, or an explicit project/workstream; a validated binding additionally contains project/workstream/session IDs. Actor description is provenance, not authorization.

The shared application layer performs normalization, permission checks, relationship validation, limits, and outcome mapping before business execution:

- `ProjectService`: create/list projects and associations; resolve explicitly without deepest-prefix or recency guessing.
- `WorkstreamService`: create/list metadata and fork within a project. Listing does not grant content access.
- `SessionService`: open, validate, explicitly close, and resume into a new session. Disconnect is not close.
- `DocumentService`: create/update/list/read exact current or historical revisions in explicit scope; verify checksums before serving targeted files.
- `CheckpointService`: save a bounded checkpoint under a validated session and read an explicitly selected handoff.

Project-only document reads select project-general documents only. Workstream reads name that workstream and pass policy. Checkpoint save, resume, and session-attributed writes require a validated binding. No service uses environment, current directory, latest activity, or MCP connection state as authorization. Path input may only produce a resolution candidate, including ambiguity/unavailable outcomes.

Stable outcome categories proposed for internal contracts are `invalid`, `not_found`, `ambiguous`, `scope_denied`, `binding_mismatch`, `conflict`, `integrity_discrepancy`, `busy`, `retryable`, `unknown`, and `internal`. Public field names and compatibility versioning remain a pre-implementation contract decision; adapters must not invent different semantics.

## Publication and retry identity

**Proposed pending Phase 0 fault and concurrency evidence:** a write request fingerprint is the canonical hash of operation kind, declared scope, target IDs, expected current revision (including explicit null), content checksum, and behavior-affecting options. It excludes interface rendering and retry timing.

1. In a short transaction, insert the operation ID and fingerprint as `registered`, or inspect an existing row. Before replaying a saved result or reconciling pending work, revalidate the caller's current authorization for the recorded declared scope and binding; possession of an operation ID never exposes a result.
2. The same operation ID and fingerprint returns its stored terminal result or resumes reconciliation after authorization. The same operation ID with a different fingerprint returns stable `idempotency_mismatch` and executes nothing.
3. Before preparation or reconciliation, claim one fenced ownership generation in SQLite. Simultaneous retries that do not own that generation may observe or wait within the bounded contention policy, but cannot prepare, publish, mutate operation state, or clean staging.
4. Use attempt-specific staging owned by that generation. Write and sync the file, publish to the deterministic immutable revision path with a no-clobber primitive, and sync the containing directory through `revisionfs`. An existing destination is verified against the recorded revision/checksum; it is never overwritten.
5. In one short SQLite transaction, require the same ownership generation, verify the expected current pointer, insert revision metadata, advance the document pointer, and mark the operation committed with its durable result.
6. Acknowledge success only after that transaction commits. On response loss, an authorized retry resolves the durable operation outcome and immutable revision without requiring that revision to remain current.

A recovery takeover must atomically fence the prior generation before acting. It may clean only staging proven to belong to a fenced, inactive generation; an old preparer cannot commit after takeover or overwrite the immutable destination. The exact claim/expiry and no-clobber filesystem primitives remain Phase 0 evidence-gated; this is a minimum safety invariant, not a lease framework.

Whether the tested filesystem and Go APIs provide the required rename/sync/no-clobber behavior is Phase 0 evidence, not a support promise. The final protocol may change if the experiments disprove it.

### Publication and recovery crash states

| Last durable state / crash window | Visible current | Recovery action and result |
| --- | --- | --- |
| No operation row | Prior revision | Retry may register a fresh intent after authorization. |
| `registered`; no durable prepared file | Prior revision | One fenced owner may prepare; other retries cannot mutate or clean its staging. |
| Partial/unsynced staging file | Prior revision | Its owner may resume; a recovery owner may remove it only after atomically fencing and proving the old generation inactive. It is never served. |
| Immutable file durably prepared; pointer not committed | Prior revision | The fenced owner verifies checksum, then retries the publication transaction if expected current still matches; otherwise it records conflict and leaves/reports an orphan. No retry overwrites the file. |
| Commit attempted; outcome unknown | Unknown until queried | Read the durable operation outcome and linked revision. A committed operation is valid when its revision row, operation/fingerprint link, immutable file, and checksum agree; the current pointer may equal it or have legitimately advanced through a consistent successor chain. Retry publication only when non-commit is proven; return `unknown` or integrity discrepancy for missing, divergent, or regressed state. |
| Commit complete; response lost | This revision or a legitimate later revision | After authorization recheck, the same operation/fingerprint returns stored success without republishing, even when a later write is now current. |
| Committed revision file missing or checksum differs | Current or history is ineligible to serve | Return integrity discrepancy; never fall back silently or infer file content from frontmatter. |
| Prepared file has no matching operation | Prior or unrelated later revision | Report/quarantine as orphan; never auto-import, publish, overwrite, or let an unproven cleaner remove active staging. |

Recovery is targeted at pending operations and requested current/historical revisions; Phase 1 does not continuously watch or rescan the library. Orphan deletion policy and coherent backup qualification remain later work, but an orphan cannot become visible merely because it exists.

## Adapter parity and telemetry flow

1. CLI or MCP parses transport input into the same application request and explicit scope.
2. The application service validates scope/policy and returns one transport-neutral result/outcome.
3. The adapter serializes that result: CLI human or JSON; MCP protocol envelope.
4. The adapter reports actual serialized response bytes to the telemetry recorder; application execution reports backend duration, status, and available result counts.
5. Telemetry write failure is logged safely and never changes, retries, authorizes, or denies the business result.

Common contract fixtures run through the application service, CLI JSON, and MCP protocol client and compare semantic outcomes, not presentation. Human CLI rendering has focused golden tests. Noninteractive paths return structured errors and never prompt. Logs and Phase 1 metrics avoid raw prose, query text, paths, titles, or content-bearing error strings by design; retention and purge behavior are not settled here.

Required metric fields are operation ID, timestamp, interface, declared project/workstream/session scope, status, total backend duration, serialized response bytes, and result counts when available. Missing inapplicable counts remain absent.

## Phase 0 experiments and exit record

No experiment has run yet. After implementation approval, Phase 0 would:

1. Build equivalent minimal probes for viable Go SQLite candidates and record compiler/linking conditions, executable shape, transaction behavior, and `SELECT sqlite_compileoption_used('ENABLE_FTS5')` plus a real FTS5 create/query probe.
2. Run the intended macOS build locally and record only the tested OS, architecture, Go toolchain, binary/link characteristics, and failures. These observations do not promise supported targets, signing, or release packaging.
3. Apply the proposed schema constraints against alias, symlink, nested path, unavailable path, same-name, cross-project workstream, and mismatched session fixtures.
4. Inject a stop at each publication step in the crash table, restart from disk, and verify idempotent classification, prior-current preservation, later-current advancement after a committed operation, orphan reporting, authorization recheck on replay, and current/historical checksum discrepancies.
5. Run separate processes contending on the same and different documents, including simultaneous retries of one operation. Force ownership takeover around prepare/publish/cleanup to prove fencing, attempt-specific staging, no-clobber immutable publication, and bounded loser behavior; measure lock duration, retry count, completion, conflict, and raw resource use.
6. Produce an evidence note selecting the driver/protocol or naming a blocker and tested alternative. Phase 1 remains blocked if one-executable/FTS5 viability or recovery coherence fails.

**Proposed configuration values, not measured SLOs:** a 1-second total SQLite contention budget with at most three application attempts; list page default 50 and maximum 200; checkpoint prose maximum 64 KiB; document input maximum 1 MiB; serialized response maximum 2 MiB. Phase 0/early Phase 1 measurements may justify changing these before approval. They are safety bounds, not latency, throughput, support, or capacity promises.

## Conditional Phase 1 sequence

After the Phase 0 exit record and a separate user approval:

1. Establish schema/opening, operation context, stable outcomes, scope validation, permission policy, limits, and best-effort telemetry.
2. Deliver project and path-association behavior through the shared service, CLI JSON/human output, and MCP.
3. Add workstream creation/listing/fork plus session open/validate/explicit close/resume; prove interleaved MCP requests have no connection-global routing state.
4. Add revisioned project-general documents, then explicit workstream documents, using the proven publication and recovery protocol.
5. Add bounded checkpoint save/read referencing exact revisions; keep resume and fork observably distinct.
6. Harden with cross-process contention, crash restart, integrity, idempotency, pagination, and cross-adapter parity cases.

Each step must leave the executable usable without later steps. Review sizing occurs in tasks; Phase 0's 650–1,150-line estimate fits within the 2,000 changed-line budget, Phase 1's 1,220–2,120-line estimate may exceed it, and the combined 1,870–3,270-line scope retains high budget risk. `ask-on-risk` must pause for a delivery decision rather than assume chaining or a size exception.

## Testing and evidence strategy

For every behavior, implementation follows:

- **RED:** add one failing domain/contract/fault test that states the invariant and stable outcome.
- **GREEN:** implement the smallest concrete service/store/adapter behavior to pass it.
- **TRIANGULATE:** add a contrasting scope, adapter, process, stale-write, or crash case that prevents a fixture-only solution.
- **REFACTOR:** remove duplication while preserving concrete boundaries and rerun the focused and full available suites.

Test layers are domain constraint tables; SQLite integration/migration tests; revisionfs fault-injection restart tests; concurrent multi-process tests; shared operation contracts; CLI JSON/human renderer tests; and an MCP stdio protocol client. Fixtures cover same-worktree workstreams, ambiguous paths, interleaved logical sessions, stale writers, unknown commits, altered historical files, and metric failure.

`go test ./...` is only a proposed future command. Go, dependencies, FTS5, the test runner, and builds are currently unverified; no tests, installs, runtime probes, source implementation, commits, paid calls, or live Claude/Codex integrations were performed for this design.

## Risks and decisions remaining before implementation

- SQLite driver, build mode, FTS5 availability, and durable filesystem semantics require Phase 0 evidence.
- Exact permission configuration, operation envelope fields/versioning, MCP library choice, pagination token encoding, and session interruption statuses need contracts before their Phase 1 behavior is written.
- Proposed resource bounds need measurement; telemetry placement and retention remain open without weakening metric-failure isolation.
- The crash protocol may expose orphan accumulation; Phase 1 must report and safely reconcile it, not invent full purge or backup machinery.
- Phase 1 likely exceeds the review budget; task planning must trigger the configured delivery gate.
- Phase 2 FTS publication/search, automatic context assembly, and all later lifecycle/intelligence infrastructure remain explicitly excluded.
