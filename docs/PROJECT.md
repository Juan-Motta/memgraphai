# MemGraph AI — Consolidated Product Definition

> **Working name:** MemGraph AI is a working name, not a final brand.
>
> **Document revision:** 0.4.1 (draft; scoped-read and purge clarification). This is a document revision, not a software release.
>
> **Document status:** Consolidated product definition for review. This document records product intent, confirmed requirements, proposed defaults, and unresolved design questions. It does **not** claim that the product is implemented or that a detailed technical design is approved.

## 1. Executive summary

MemGraph AI is a local, agent-first system for durable project knowledge and continuity. It gives coding agents a bounded way to retrieve current state and historical knowledge across interrupted or changing sessions without treating a raw transcript as the product.

The initial product is one Go executable for macOS. It exposes the same concrete application services through CLI and MCP adapters. CLI processes exit after completing work; MCP lifetime is owned by its client connection. A mandatory resident daemon, HTTP service, browser UI, and desktop implementation are out of scope for the initial product.

The product stores a central per-user knowledge library outside repositories. Markdown is the primary document format, while SQLite is authoritative for operational state such as identities, links, sessions, tasks, and revision visibility. Retrieval begins with exact lookup, filters, lists, and SQLite FTS5 section search. Embeddings are optional and must never be required for core operation.

## 2. How to read this document

Requirements use three confidence labels:

| Label | Meaning |
| --- | --- |
| **Confirmed** | User requirement or settled product direction for planning purposes. |
| **Proposed default** | Recommended starting behavior that still needs validation through design or testing. |
| **Open decision** | A material choice that must be resolved before detailed implementation. |

Where this document names operations, files, dependencies, or layouts, those names are product-definition examples unless explicitly labeled confirmed. No listed API, command, performance target, or feature should be read as already implemented.

## 3. Product purpose

### 3.1 Problem

Coding work spans agents, clients, branches, worktrees, research threads, and interrupted sessions. Chat history alone is an unreliable continuity mechanism because:

- context windows are bounded;
- a prior retrieval may no longer remain in model context;
- a tool-call timeline is not a durable checkpoint;
- session recency does not identify the objective being resumed;
- hidden model state and unsaved working files cannot be reconstructed safely;
- unrelated workstreams can coexist in one repository or worktree.

### 3.2 Product outcome

**Confirmed:** Any authorized coding agent, including Claude Code and Codex through MCP or CLI, can request focused project knowledge on demand. The system supports:

- project planning and general documentation;
- specifications, designs, and SDD artifacts as documents, without becoming an autonomous SDD orchestrator;
- structured tasks and verification evidence;
- research, decisions, learnings, and external links;
- checkpoints, handoffs, restarts, resumes, and forks;
- current and historical document revisions with provenance;
- bounded retrieval suitable for limited agent context.

### 3.3 What the product is not

**Confirmed:** MemGraph AI is not a raw transcript sink, model-training system, source-control replacement, security sandbox, or self-certifying implementation/test runner. It does not guarantee retained model context or restore hidden model state and unsaved files.

## 4. Product principles

1. **Agent-first and human-readable:** agents are primary operational clients; source knowledge remains inspectable Markdown.
2. **Stable identity and explicit scope:** identities survive location changes; ambiguity never guesses or merges.
3. **Shared behavior:** every adapter uses the same application services and protections.
4. **Bounded, provenance-rich retrieval:** expand deliberately from cited excerpts to sections and full documents.
5. **Local-first and optional intelligence:** no mandatory daemon, embedding provider, or local inference runtime.
6. **Durability before sophistication:** recovery, backup, concurrency, and isolation precede vector search.
7. **Measured, not invented:** observations establish performance and resource goals; unsupported SLOs do not.

## 5. Identity and scope model

### 5.1 Project identity

**Confirmed:**

- `project_id` is immutable and is the canonical project identity.
- A display name is mutable and may have previous-name aliases.
- A project may have zero, one, or multiple repository/path associations.
- A research project needs no repository.
- Repository path, worktree, branch, and commit reference are context, not identity.
- Internal links and storage use stable IDs rather than names or mutable paths.
- A move, rename, or branch change does not create a new project.
- Current working directory or path may suggest an unambiguous known project.
- Path associations normalize lexical segments and resolve symlinks where the filesystem permits; they do not unconditionally fold case.
- Unavailable or moved paths are reported explicitly. Aliases, symlinks, nested associations, and same-named projects must not silently merge.
- Ambiguity produces an explicit error; it never causes guessing or an unapproved deepest-prefix choice.

Association is not authorization. Invocation permissions, session scope, and local access policy remain distinct from project-path associations. In the initial single-user local threat boundary, explicit scope selection prevents accidental cross-context retrieval and configured operation permissions still apply. An agent with operating-system access to the library can bypass service API controls, so this boundary is not a secure sandbox. Cross-scope access requires both explicit selection and an allowed policy; optional telemetry failure cannot grant or deny it.

### 5.2 Workstreams and sessions

**Confirmed:**

- A persistent `workstream_id` identifies an objective and survives agent or client changes.
- A `session_id` identifies one run within a workstream.
- Session provenance records origin/client and records model identity only when the client reports it.
- General project documents and workstream-specific documents are separate scopes.
- Provenance and scope are separate concepts: a document can originate in one session while belonging to project-general or workstream scope.
- Session context requires a validated project/workstream/session binding and combines general project knowledge with that bound workstream.
- Sessionless operations never infer a current workstream. Project-only content browsing selects project-general documents only; it is an explicit scoped read, not session context assembly, and does not require a workstream.
- Listing a project's workstreams is a separate metadata-list operation subject to permissions; discovering a workstream does not retrieve its documents or authorize reading them. Reading content across workstreams requires the explicit cross-scope selection and policy below.
- Cross-workstream and whole-library reads require explicitly selected scope and an allowed policy; absence of a project never widens scope automatically.
- There is no “latest active session” fallback.
- Resume and fork are distinct actions and must remain distinguishable.
- Every operation is normalized to its required explicit scope before application execution; a session is included only where the operation requires one.
- Scoped search, list, exact-source, and section reads may be sessionless for any caller when project/workstream scope is explicit and caller/provenance is declared, subject to configured permission policy. Read analytics is best-effort as described in Section 11; actor disclosure is not authorization, while business writes persist required provenance.
- Context assembly, resume, checkpoints, and session-attributed writes require a validated project/workstream/session binding.
- Bootstrap project discovery or creation, session opening, and maintenance may be sessionless as described by their operation contracts.
- Actor self-description never grants access or determines whether an operation is eligible to be sessionless.
- One MCP connection may carry multiple logical sessions; mutable connection-global “current session” state must not authorize or route operations between them.
- Defaults are allowed only for an already established, unambiguous binding. “Latest,” environment values, and path suggestions never confer authorization.
- Supplied project, workstream, and session IDs are checked for mutual consistency and permission before execution.

A session becomes ended only through explicit close. A transport disconnect is an observation, not business completion or trustworthy global session termination; an observably associated interrupted run may be marked disconnected or of unknown state. Reconnect and resume never implicitly reuse another session or workstream context. The exact session-status and interruption-detection policy remains open.

**Proposed default:** Record the source of each provenance field as client-reported, service-observed, user-supplied, or service-internal as appropriate. Client/model self-report is not authenticated identity, and process ancestry does not make it so. Internal IDs and revision provenance derive from actual service records.

## 6. Continuity scenario

The following example is normative for scope behavior, though its names and identifiers are illustrative.

- Project **North Store** has stable project identity independent of its repository path.
- Workstream **WS-AUTH** covers authentication.
- Claude session **SES-101** is interrupted after saving a checkpoint: integration verification is pending.
- Workstream **WS-PAY** contains independent payment research and has more recent activity.
- Codex session **SES-103** explicitly resumes **WS-AUTH**.

On resume, SES-103 receives general North Store knowledge plus WS-AUTH sources. It does not receive WS-PAY-exclusive context merely because WS-PAY is newer or uses the same branch/worktree.

The checkpoint is not a snapshot of the model or filesystem. Its references must be checked against current documents and tasks. It can say that integration verification was pending, but it cannot prove that verification occurred, restore unsaved edits, or imply that previously retrieved text is still in model context.

## 7. Runtime and architecture boundaries

### 7.1 Confirmed direction

| Area | Direction |
| --- | --- |
| Backend | Go. |
| Distribution | One global executable; no Node runtime requirement. |
| Initial platform | macOS first, with a portable core for later operating systems. |
| Application core | Shared concrete application services independent of CLI and MCP. |
| CLI lifetime | Command process exits after completing work. |
| MCP lifetime | Owned by the client connection. |
| Future adapter | A local adapter may connect a desktop application to the same core. |
| Persistence | SQLite plus Markdown documents in a central per-user library. |
| Web stack | No daemon, HTTP, or IPC framework is required up front. `net/http` is only a possible future interface. |

The architecture should favor direct concrete services until actual variation requires an abstraction. It should not introduce early generic repositories, provider layers, plugin systems, microservices, or graph databases.

### 7.2 Candidate dependencies, not commitments

`modernc.org/sqlite` and Goldmark are candidates, not selections. SQLite FTS5 must be verified in the chosen build and packaging path. Vector engines, embedding runtimes, provider integrations, and local model packaging are deferred.

## 8. Storage and authority

### 8.1 Library layout

**Proposed default:** Use a configurable central library rooted at `~/.memgraph/` while MemGraph AI remains the working name:

```text
~/.memgraph/
├── config.json
├── database.sqlite
├── library/
│   └── projects/
│       └── <stable-project-id>/
│           └── documents/
├── models/        # optional
└── cache/         # optional and rebuildable
```

The final path may change with final branding. The path is configurable and independent of the binary location.

### 8.2 Source-of-truth boundaries

**Confirmed:**

| Data | Authority |
| --- | --- |
| Document prose | Markdown source content. |
| Project/workstream/session identity | SQLite operational state. |
| Task status and assignment | SQLite operational state. |
| Links and associations | SQLite operational state. |
| Current visible revision pointer | SQLite operational state. |
| Search index and derived summaries | Rebuildable derived data with explicit provenance. |

There must be no competing editable authority for the same fact. Exact frontmatter ownership and schema remain unresolved; metadata must not silently diverge between Markdown and SQLite.

### 8.3 Revisions, publication, and recovery

**Confirmed invariants:**

- Filesystem and SQLite changes cannot be assumed to form one atomic transaction.
- Writes use explicit revisions, short database transactions, and idempotent operations.
- Concurrent updates must not overwrite a newer revision silently.
- Crash recovery and integrity checking are mandatory.
- Historical revisions and `supersedes` relationships retain provenance, except content explicitly purged under the lifecycle rules in Section 9.2.
- Retirement and purge changes must propagate so affected documents are not served from stale indexes or caches.
- Out-of-band Markdown changes are detected through targeted revision/checksum verification rather than silently accepted as current. The product reports an integrity discrepancy or imports the content explicitly as a new revision; it need not watch external editors or rescan the entire library on every call.
- Concurrent CLI and multiple MCP processes are a baseline workload. Transactions stay short and contention uses bounded waits/retries; exhaustion returns a stable busy, retryable, or conflict outcome rather than a raw database error or unbounded wait.

Phase 1 publication path IDs (project, document, revision, and write operation IDs) use nonempty ASCII letters, digits, hyphens, or underscores. Document and revision inputs retain their existing 128-character bound; project and operation IDs are limited only by the 255-byte filesystem component boundary, not by that document/revision bound. They are validated before durable reservations; names and provenance remain free text. Each explicit write invocation makes at most one preparation attempt. A later identical invocation may claim a new fenced generation without a lifetime retry cap; immutable request binding and terminal replay still apply. Known global revision-ID collisions fail before preparation, and the publication transaction handles races as conflicts without replacing the existing revision. Files prepared by a losing concurrent writer remain report-only orphans, not visible revisions.

In Phase 1, required Markdown content is durably prepared first, then the new current pointer commits in a short SQLite transaction that rejects a stale expected revision. Phase 2 adds required FTS rows to that same publication commit; Phase 1 does not require FTS. If preparation or required indexing fails before the applicable commit, the new revision does not become current, the prior current revision remains when one exists, and any staged file is not a published document. Success is acknowledged only after commit; newly started scoped reads can then retrieve the valid current revision subject to query, filters, permissions, and limits.

If process or transport loss makes the commit outcome unknown, the operation reports an unknown outcome when possible or resolves it through operation ID/idempotency recovery; it must not claim rollback or blindly repeat a possibly committed write. Reconstructible FTS does not permit deferring the required publication commit. Optional embeddings and summaries remain independent asynchronous work.

**Design candidate:** Immutable Markdown revisions with durable preparation before the SQLite publication transaction. This is not a combined filesystem/database transaction or a settled file protocol. File publication, flush guarantees, interruption recovery, backup behavior, WAL, timeouts, and retry strategy remain open and must be validated against crash windows, concurrency, recovery, and human inspection needs.

### 8.4 Backup and restore

**Confirmed:** Copying a live SQLite file and document directory independently is not a coherent backup strategy. The product must:

- distinguish authoritative data from rebuildable indexes and caches;
- provide a tested coherent backup and restore procedure;
- verify restored identity, revision visibility, document checksums, and task/link integrity;
- exercise interrupted-write and recovery behavior from the beginning, even if release qualification comes later.

## 9. Capability model

Names below describe minimal user intents, not committed SDK signatures or commands.

| Capability | Intent |
| --- | --- |
| Project | Resolve, create, and list stable projects and associations. |
| Workstream | Resolve, create, list, resume, or fork persistent objectives. |
| Session | Open and close scoped runs with provenance governed by Section 5.2. |
| Documents | Create, update, list, read a section, or read a full revision. |
| Knowledge | Search exact sources, metadata, and indexed sections. |
| Tasks | List, claim, transition, and attach dependencies or evidence. |
| Links | Connect documents, tasks, decisions, artifacts, sources, and revisions. |
| Checkpoints | Save a bounded handoff with references to current state. |
| Context | Retrieve bounded general plus bound-workstream context. |
| Maintenance | Run integrity checks, index rebuilds, backups/restores, explicit content/project purge, and telemetry export/purge. |

### 9.1 Adapter behavior

**Confirmed:**

- CLI supports human-readable and JSON output.
- MCP and CLI use the same operation envelope, application logic, validation, authorization boundaries, and logging.
- Adapters normalize contextual operations to the binding rules in Section 5.2 before invoking shared services.
- Parity means identical semantics and protections for operations exposed through both adapters, not identical feature availability.
- Transport-neutral application services leave room for a future app without making transport behavior authoritative.
- Responses are bounded and paginated where needed.
- Noninteractive use never receives an unexpected prompt.
- Errors are stable and distinguish ambiguity, missing identity, scope denial, conflict, stale revision, and invalid transition.
- Operations are intents, not a requirement that each operation become a future UI screen.

**Proposed default:** Sensitive or destructive maintenance, including permanent content/project purge, telemetry purge, and restore over an existing library, is CLI-first and is not exposed through MCP by default. This availability choice does not weaken CLI/MCP parity for operations exposed through both adapters and is not a permanent CLI-only commitment.

Exact names, envelope fields, error taxonomy, pagination tokens, and compatibility policy require detailed design and contract tests.

### 9.2 Documents, tasks, and knowledge lifecycle

**Confirmed:**

- Documents preserve original Markdown and stable source references/checksums.
- Current and historical status are distinguishable.
- Updates record revision and provenance.
- Learnings are promoted from workstream scope to project-general scope only by an explicit action.
- Tasks have structured state, dependencies, assignments/claims, and verification evidence.
- A task status alone cannot self-certify verification.
- An accepted specification does not imply implementation.
- Task evidence may register test results supplied by work; MemGraph AI does not execute tests on user code as part of task status.
- Retirement is soft deletion: content is hidden from default current retrieval, including search, lists, and context assembly, while authorized explicit history remains available.
- Purge is an explicit irreversible maintenance action that removes targeted content/revisions and relevant indexes/caches from the active library, including corresponding content-bearing telemetry and references under Section 11.2. It does not promise forensic secure erase or deletion from external backups, providers, or client copies; backup retention and exclusions are disclosed separately.
- Purge never silently rewrites historical backups, and future in-app editing does not imply support for direct file editing.

**Proposed default:** Retirement, rather than purge, is the default removal action. Permanent content purge remains governed by the maintenance availability and permission policy in Section 9.1.

## 10. Retrieval model

### 10.1 Baseline retrieval

**Confirmed:** The first complete retrieval system uses:

1. exact identity and source lookup;
2. filtered list operations;
3. SQLite FTS5 retrieval over document sections;
4. bounded context assembly.

The original Markdown remains preserved. Once baseline FTS ships in Phase 2, required FTS rows and the current revision pointer commit together under the publication contract in Section 8.3. A later revision becomes current only when it validly supersedes the expected current revision. Success means the transaction committed; there is no published-but-index-pending success state. Newly started scoped reads after acknowledgement can retrieve the valid current revision subject to permissions, query, filters, and limits; eligibility does not guarantee top rank, and an old revision must not be presented as current.

Indexes remain reconstructible and should incrementally process changed sections rather than reprocessing everything without cause. Rebuildability does not defer required FTS work or prove that an ordinary publication succeeded. Parsing and other preparation occur outside the short publication transaction; embeddings, summaries, and external network requests are not part of it. The exact filesystem/database recovery protocol remains open and does not imply combined filesystem/SQLite ACID.

### 10.2 Progressive context

**Proposed default:** A context request begins with:

- current constraints and goal;
- structured current tasks and blockers once delivered in Phase 3; before then, checkpoint notes may describe task-like state but structured task state is reported as unavailable, never empty, completed, or inferred;
- checkpoint references and their freshness;
- small source excerpts with stable references.

The caller can then request a complete section and, explicitly, a full document. Every response applies query budgets, result limits, pagination, and freshness indicators. Derived summaries must be labeled separately from source truth.

### 10.3 Optional semantic retrieval

**Confirmed boundaries:**

- Keyword search remains sufficient for complete core operation.
- Hybrid keyword/vector fusion is optional and selected only after evaluation.
- No fixed rank weights or uncalibrated “truth” score are promised.
- Embeddings support disabled, local, and external modes.
- External embedding use requires explicit opt-in.
- There is no silent fallback from local processing to an external provider.
- Provider/model/dimension namespace changes require compatible reindexing.
- A stale indexing job must not publish obsolete output as current.

**Open design area:** External embeddings are recommended before local embeddings to reduce initial packaging complexity, but that sequence is not a hard user constraint.

### 10.4 Work while processes are active

**Confirmed direction:** Embeddings, derived summaries, and rebuild maintenance may run asynchronously only while an invoked process or MCP connection is active. Baseline FTS publication remains governed by Section 10.1. Pending asynchronous work resumes on next use; no perpetual worker is required.

A normal rebuild of a healthy FTS index must preserve publication visibility and concurrent lifecycle changes throughout rebuild and activation. It must not hide newly committed current rows, restore superseded or retired content to default current retrieval, or reintroduce purged content from a stale snapshot; normal scope, explicit-history, and permission checks still apply. This does not promise uninterrupted availability when the existing index is corrupt.

Detailed worker behavior remains to be designed, including bounded retries, cross-process leases, cancellation, idempotent publication, stale-job rejection, and the Phase 2 rebuild/activation method (for example, revision-aware reconciliation or an atomic swap). Local models should load lazily and use bounded cross-process resources. The implementation must be evaluated to avoid unlimited model copies without requiring a global inference daemon.

## 11. Analytics, privacy, and evaluation

### 11.1 Operation telemetry

MCP response sizes count the complete emitted JSON-RPC frame, including its newline. Metric correlation uses the connection's JSON-RPC request ID, carried only in process, rather than the caller-supplied operation ID or response contents. Missing or canceled response metrics never hold a pending-response gate over subsequent calls.

**Confirmed:** Phase 1 records operation ID, timestamp, interface, declared project/workstream/session scope, status, total backend duration, serialized response bytes, and result counts where available. Counter-metric failure remains independent of operation success.

Detailed analytics adds requested and executed search mode, safe categorical filters, model/index revision, stage durations, document/section counts, character and UTF-8 byte counts, approximate tokens, and approximation method or tokenizer/model version where applicable. Character measurement needs an exact design definition. Approximate tokens are not actual agent context usage or billing.

### 11.2 Privacy and retention

**Proposed collection defaults:** Normal telemetry stores no raw queries, free-text filter values, full payloads, or content-bearing error/log strings; safe categorical metadata is allowed. Raw filters and detailed query capture require explicit opt-in for controlled evaluation. Local retention is bounded, with pagination, export, and purge. Initial query-capture opt-in, thresholds, access-policy UX, precise retention defaults, redaction, export format, and telemetry placement remain open.

**Confirmed purge privacy:** Within the declared purge scope of the active library, content or project purge removes corresponding captured queries, payloads, content-bearing telemetry, and references that carry target content. Content-bearing data includes target text, excerpts, captured queries, and revealing names, titles, paths, or URLs—not just document bodies. The guarantee covers active indexes, caches, and in-progress derived artifacts so targeted content does not survive only in analytics or staging. This does not promise forensic secure erase or deletion from backups, providers, or client copies, whose limits are disclosed separately.

Opaque project/document/revision IDs in accounting, job reports, or telemetry are distinct from references carrying target content. They may remain only under the disclosed noncontent-residue policy in Section 17, without retained target text or a path to retrieve purged content; opaque does not mean anonymous or automatically retained. Reports of skipped purged revisions must not repeat their content. Disclosed noncontent aggregates and accounting, including immutable tariff records without source content, may also remain. Responses must identify what is retained rather than describe it as purged; exact residue fields and retention periods remain open.

Metrics failure never converts a successful operation into failure, and no Prometheus or Redis dependency is required. Precise purge-scope UX and backup retention remain open.

### 11.3 Quality evaluation

Retrieval quality is evaluated against curated queries, expected sources, and relevance labels. Response volume is not a quality metric. Evaluations should cover freshness, scope isolation, historical retrieval, deletion propagation, and graceful behavior without embeddings.

## 12. External cost accounting

**Confirmed:** Cost accounting separates ingestion/update, query retrieval, reindexing, future external model responses in an agent playground, and local resource use. Local work has no per-call provider bill but still consumes resources.

External records distinguish provider-reported, estimated, and unknown usage. Each cost record preserves provider, model, billing unit, quantity, currency, tariff, tariff effective date, cache behavior, attempts, and known timeout ambiguity. A timeout must not be automatically recorded as zero cost.

Historical tariff snapshots are immutable. Tariffs are user-configured or sourced with recorded provenance and effective date; no bundled rate is treated as authoritative or latest. Without a rate, usage is still reported while monetary cost remains unknown—not zero—and an ordinary requested operation is not blocked solely for lack of an estimate. Separate spend gates may still apply. Rates may use tokens or another provider billing unit. Cost estimates are not invoices.

**Proposed defaults:** Start with user-configured tariffs. For a costly external reindex, a preview describes a bounded plan: a fixed manifest of document/section revision IDs, explicit scope, provider/model, recorded tariff, and disclosed quantity/cost allowance. The manifest contains references to existing source revisions, not a new sensitive backup copy. Confirmation authorizes only that plan. Normal later publications neither invalidate nor expand it; later revisions require separate incremental jobs under their applicable consent and spend rules.

Before each external submission, the operation rechecks authorization and data eligibility. Deleted, purged, retired, unavailable, or no-longer-eligible revisions are skipped and reported rather than uploaded from the manifest, and stale completed results cannot become current. For a current-knowledge plan, a captured revision superseded before dispatch is explicitly reported as superseded and skipped; its replacement is not silently substituted or charged to that plan, and needs a separate incremental job with its own applicable authorization and spend. If superseded after dispatch, incurred provider charges may remain and are reported, but the result never publishes as current. Completion reports processed, skipped, and remaining coverage so completed planned processing is not mistaken for complete coverage of the latest project. This proposed current-knowledge behavior does not add default historical semantic indexing. Scope expansion, provider/model or tariff change, or exceeding the planned allowance requires a refreshed preview and confirmation. Requests already dispatched externally cannot be undone. Automated mode never prompts and instead returns a structured approval-required outcome; a missing estimate never auto-approves. Which routine external calls require confirmation, cost thresholds, unavailable-estimate handling, and optional preview expiry mechanics remain open until Phase 5; this does not introduce a token framework or always-on snapshot service.

## 13. Future human experience

This is future intent, not current implementation scope. The desktop product should be a simple reader and explorer—not an administration console—with likely **Projects**, **Explore**, and **Ask** areas. Project views may summarize work, tasks, documents, and history; sessions remain contextual rather than primary navigation.

Boards and cards render only explicit structured data. Source Markdown remains available. In-app editing is intended but secondary, and external editors are not mandatory. A retrieval playground may expose advanced read/search options; grounded agent Q&A is read-only by default, cites sources, and may show tool traces but never hidden chain-of-thought.

The app uses a later adapter to the shared core. No desktop framework, transport, browser server, or IPC design is selected here.

## 14. Non-goals for the current product

Current non-goals are cloud or multi-device sync; mandatory external editor integration; code indexing; graph databases; microservices; plugins; autonomous SDD orchestration; mandatory embedding providers; mandatory background daemons or global inference services; browser/desktop implementation; early generic abstractions; and executing user-code tests as part of task status. Engram may inform research, but compatibility aliases and migration from Engram are not current goals. The product can serve the same broad memory use case through its own contracts.

## 15. Delivery roadmap

Roadmap numbers indicate sequence, not completed milestones or committed release dates.

| Phase | Outcome | Key proof |
| --- | --- | --- |
| 0 | Minimal invariants and SQLite packaging spike | FTS5/build viability, identity constraints, revision/crash model, portable filesystem boundary. No vector or local-inference frontloading. |
| 1 | Early complete CLI/MCP vertical slice | Project/workstream/session identity, documents, checkpoint, Phase 1 analytics, concurrency and durability baseline through both adapters. |
| 2 | FTS section retrieval and bounded context | Stable sections, cross-process post-publication eligibility, progressive reads, freshness, limits, rebuildability, scope isolation. |
| 3 | Tasks, artifacts, links, and decisions | Structured transitions, dependencies, provenance, evidence registration, explicit learning promotion. |
| 4 | Detailed analytics and evaluation | Privacy modes, retention, stage metrics, expected-source relevance evaluation. |
| 5 | Optional embeddings and external cost accounting | Explicit provider opt-in, namespaced indexes, stale-job safety; external then local is a recommendation, not a fixed requirement. |
| 6 | Release and backup/restore qualification | Supported macOS packaging, coherent backup/restore, recovery evidence, upgrade and portability qualification. |

Backup principles and recovery invariants apply from the beginning even though release qualification is Phase 6. Desktop UI, retrieval playgrounds, and agent Q&A remain deferred.

## 16. Engineering and validation strategy

When implementation begins, use strict **RED → GREEN → TRIANGULATE → REFACTOR** discipline for behavior changes.

Required validation categories include:

- deterministic contract tests with fixtures and protocol clients for CLI/MCP envelopes, plus separate opt-in live Claude and Codex smoke tests early; live integrations are not unit fixtures or required CI dependencies and incur no external cost without consent;
- CLI/MCP parity for envelopes, errors, scope, and protections on operations exposed through both;
- process crashes between filesystem and SQLite publication steps;
- concurrent CLI/MCP processes, stale revisions, leases, bounded contention/retries, recovery, and idempotency;
- project and workstream scope isolation, including same-worktree streams and interleaved logical sessions on one MCP connection;
- index rebuild, incompatible model namespace, stale job publication, and deletion propagation;
- coherent backup and restore;
- no-embedding operation and external-provider opt-in;
- measured CPU, memory, startup time, latency, disk use, and context size.

No numeric performance target is approved by this document. Measurements should establish baselines and inform later SLO decisions. macOS architectures, minimum OS version, filesystem behavior, packaging, and signing support must be confirmed.

## 17. Risks and open decisions

| Topic | Current position | Decision or evidence needed |
| --- | --- | --- |
| Metadata authority and publication | Markdown owns prose; SQLite owns operational state; Phase 1 commits the current pointer after durable Markdown preparation, and Phase 2 adds required FTS rows to the same publication success. | Exact frontmatter schema, durable file preparation/publication, crash reconciliation, operation-ID recovery, targeted checksum validation for served current and historical revisions, and conflict mechanics. |
| Scope, sessions, and local access | Operation-specific scope, permitted sessionless reads, validated sessions where required, no connection-global routing, and explicit-plus-allowed cross-scope behavior are settled. | Exact permission interfaces/UX, session interruption/status policy, default-binding representation, and operation-specific envelopes. |
| Supported macOS targets | macOS first with portable core. | CPU architectures, minimum versions, filesystem assumptions, packaging, signing, and FTS5 build evidence. |
| Section identity | FTS retrieval operates on stable source sections. | Identity rules across heading rename, movement, split/merge, duplicate headings, and revision history. |
| Provider and model packaging | Embeddings are optional; external requires opt-in; bounded current-knowledge reindex confirmation and explicit superseded-revision accounting are proposed. | Provider interfaces, local runtime, model distribution/licensing, resource locking, reindex compatibility, and—before Phase 5—classification of routine versus costly calls, thresholds, and optional preview expiry. |
| Storage recovery | Combined filesystem/SQLite atomicity is impossible; unknown commit outcomes require idempotent resolution. | Exact interruption policy, recovery states, integrity checks, orphan handling, fault-injection evidence, and backup protocol. |
| Lifecycle and noncontent residue | Purge removes target text and content-bearing references; opaque IDs are not automatically anonymous or eligible for retention. | Which noncontent identifiers, tombstones, and accounting/job references may remain, their retention periods and disclosure, without target content or a path to retrieve it. |
| Analytics privacy | Phase 1 fields and metric-failure independence are required; collection defaults are proposed, while scoped active-library removal of content-bearing telemetry on content/project purge is required by Section 11.2. | Confirm collection defaults, precise retention, physical telemetry placement and backup implications, redaction, export, purge-scope UX, and character/token definitions. |
| Operation contracts | Intent set, binding behavior, provenance trust distinction, stable errors, and exposed-operation parity are required; provenance-source categories remain proposed in Section 5.2. | Exact schemas, pagination, compatibility/versioning, confirmation-plan representation, and interface-specific capability exposure. |
| FTS rebuild activation | Healthy-index rebuilds preserve concurrent publication and lifecycle visibility without resurrecting stale content. | Phase 2 reconciliation/activation method, bounded work, corrupt-index behavior, and concurrency evidence. |
| Hybrid retrieval | Optional and evaluation-driven. | Benchmark set, fusion method, calibration, quality thresholds, and fallback behavior. |

## 18. Acceptance checklist for this definition

- [ ] Working name, document revision 0.4.1, and review-only status are explicit; the revision is not a software release and no implementation or approved design is implied.
- [ ] Purpose is local agent continuity, not transcript storage, training, source control, or security sandboxing.
- [ ] Project, workstream, session, path, and provenance stay distinct; recency cannot leak unrelated scope.
- [ ] Go, one executable, shared CLI/MCP services, and no Node requirement are clear.
- [ ] No resident daemon, HTTP server, browser UI, or desktop implementation is required now.
- [ ] Markdown/SQLite authority, revision, concurrency, recovery, deletion, backup, and restore invariants are clear.
- [ ] Baseline retrieval works without embeddings; external processing is opt-in with no silent fallback.
- [ ] Source, summaries, telemetry estimates, and costs are distinguished; tasks cannot self-certify verification.
- [ ] Future UI boundaries and unresolved decisions are visible without inventing APIs or targets.

### 18.1 Behavioral acceptance cases

- [ ] Two logical sessions interleave operations over one MCP connection without project/workstream/session scope mixing; reconnect/resume cannot inherit another context, and only explicit close marks a session ended.
- [ ] Human and agent callers use the same permission policy for sessionless scoped search, list, exact-source, and section reads; actor self-description changes neither access nor session eligibility.
- [ ] A sessionless read with missing required scope is rejected without inferring a workstream or widening scope; session-required context, resume, checkpoint, and attributed-write operations reject a missing or inconsistent session.
- [ ] A project-only content read returns project-general documents, not WS-AUTH or WS-PAY documents. An authorized workstream listing exposes permitted metadata only; reading their content requires explicit scope selection and permission.
- [ ] Path aliases and symlinks resolve consistently where available without unconditional case folding, while nested associations, same-named projects, unavailable paths, and moved repositories produce explicit outcomes without silent merge or unapproved prefix selection.
- [ ] Phase 1 commits the current pointer after durable Markdown preparation; after Phase 2, required FTS rows join that same publication success. A precommit preparation/index failure leaves the prior current revision, and an unknown commit outcome is resolved idempotently rather than blindly repeated.
- [ ] Concurrent CLI and MCP writers use bounded contention handling; retry exhaustion, stale writes, crashes, and recovery produce stable outcomes rather than raw lock errors, overwrite, or indefinite waits.
- [ ] Targeted direct alteration of served current or historical Markdown is detected by revision/checksum verification and requires discrepancy handling or explicit revision import.
- [ ] Retirement hides content from default current search, lists, and context while authorized explicit history remains; explicit purge removes scoped active-library content and associated content telemetry, with backup/external-copy and aggregate-accounting limits disclosed.
- [ ] After content/project purge, active-library telemetry, job reports, and derived staging artifacts do not expose the target's text, excerpts, captured queries, or revealing names/titles. Any policy-retained opaque IDs and noncontent accounting are disclosed and cannot retrieve purged content.
- [ ] Core retrieval works with embeddings disabled; unknown tariffs preserve usage with unknown cost and sourced tariffs retain provenance/effective date without rewriting history.
- [ ] Starting a healthy-index FTS rebuild at R1, then publishing R2 and retiring or purging content before activation, leaves applicable current R2 searchable and does not resurrect removed rows when the rebuild finishes.
- [ ] If the proposed fixed-manifest current-knowledge reindex flow is adopted, normal later writes do not invalidate the whole plan; a captured revision superseded before dispatch is skipped and reported, its replacement requires a separately authorized incremental job, and completion distinguishes processed, skipped, and remaining latest-project coverage. A revision superseded after dispatch may retain incurred charges, but its result never publishes as current.
- [ ] If proposed maintenance defaults are adopted, contract tests preserve shared CLI/MCP semantics while confirming CLI-first destructive availability and retirement as the default removal action.

## 19. Decision history and superseded alternatives

| Alternative | Status | Reason |
| --- | --- | --- |
| TypeScript/Node backend | Superseded | Go and a single executable with no Node runtime requirement are confirmed. |
| Mandatory resident daemon | Superseded | CLI commands exit after work and MCP is client-owned; pending work resumes on later use. |
| Browser UI now | Superseded for current scope | No browser server/UI is planned; a future desktop reader is intended but deferred. |
| Administration-heavy human UI | Superseded as product direction | The future UI should prioritize reading, exploration, grounded sources, and simple project/work views. |
| Engram compatibility, aliases, or migration | Not current | Engram is only a research reference; no compatibility or migration commitment exists. |
| Vector-first retrieval | Superseded for initial phases | Exact, filtered, and FTS5 section retrieval must establish a complete baseline first. |
| Path or latest session as identity | Superseded | Stable project/workstream IDs and explicit resume scope prevent accidental conflation. |

### Revision 0.3 decisions

- Operation-specific sessionless reads replace an unverifiable human-versus-agent gate; explicit scope, permission, and recorded caller/provenance still apply.
- Atomic database publication of the current pointer plus required FTS rows removes an intermediate success state, while durable file preparation and recovery remain open.
- A fixed reindex manifest is the **proposed default** to avoid starvation under continuous writes and to preserve purge and permission checks without expanding authorized work.
- Confidence labels now separate settled invariants from proposed provenance categories and proposed default maintenance availability.
- Maintenance explicitly covers content/project purge and telemetry purge; CLI-first availability and retirement-by-default remain **proposed defaults**.
- Phase-relevant details—including interruption recovery, historical checksum checks, purge residue, telemetry placement, and Phase 5 pricing/expiry rules—are postponed, not implemented.

### Revision 0.4 decisions

- Session context now always requires a validated project/workstream/session binding, while explicit project-only and whole-library reads remain distinct scoped operations.
- Healthy FTS rebuilds preserve concurrent current and lifecycle visibility; the bounded Phase 2 reconciliation/activation method remains open.
- Proposed current-knowledge reindex plans skip and report revisions superseded before dispatch, never substitute replacements silently, and distinguish plan completion from latest-project coverage.
- Content/project purge must remove scoped active-library content-bearing analytics, while collection defaults, backups, external copies, noncontent aggregates, and secure-erasure limits remain clearly separated.

### Revision 0.4.1 clarifications

- Project-only browsing means project-general content; workstream discovery and cross-workstream content retrieval are separate, permission-checked operations.
- Purge removes content-bearing references as well as prose, including active derived staging artifacts. Opaque identifiers may survive only under an explicit, disclosed noncontent-residue policy; their exact retention is not decided here.

The open decisions in Section 17 must be resolved at their relevant roadmap phase while preserving confirmed invariants; they do not all block Phase 0. This definition does not select SDD/OpenSpec, a desktop framework, an HTTP/IPC transport, or a vector engine.
