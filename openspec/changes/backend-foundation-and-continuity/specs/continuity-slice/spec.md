# Continuity Slice Specification

## Purpose

Phase 1 MUST provide an early continuity slice through shared concrete application services and CLI/MCP adapters. It MUST preserve explicit identity and scope, durable revision history, and bounded continuity semantics without introducing Phase 2 FTS retrieval or automatic context assembly.

## Requirements

### Requirement: Shared CLI and MCP operation semantics

Operations exposed through both adapters MUST use the same application behavior, validation, authorization boundaries, stable outcomes, and logging semantics. CLI output MUST support human-readable and JSON forms; MCP responses MUST be semantically equivalent without requiring final public field names in this phase.

#### Scenario: Adapter parity

- GIVEN the same valid or invalid operation is sent through CLI and MCP
- WHEN both adapters normalize the request and invoke the application service
- THEN they enforce the same scope and protections
- AND their responses express equivalent success or stable error outcomes

#### Scenario: Noninteractive operation

- GIVEN an operation runs without an interactive user
- WHEN input is incomplete, ambiguous, unauthorized, conflicting, or requires retry
- THEN it returns a structured stable outcome
- AND it does not prompt unexpectedly or widen scope

### Requirement: Explicit project, workstream, and session continuity

The slice MUST support stable project identity and associations, workstream creation and listing, session open and explicit close, and validated project/workstream/session bindings. Sessionless operations MUST remain explicitly scoped where allowed; session-required operations MUST reject missing or inconsistent bindings. A disconnect MUST NOT implicitly close or reuse a session.

#### Scenario: Project-only document scope

- GIVEN a project contains project-general documents and workstream documents
- WHEN an authorized project-only read is requested
- THEN only project-general content is eligible
- AND workstream content requires explicit permitted selection

#### Scenario: Interleaved logical sessions

- GIVEN one MCP connection carries two logical sessions
- WHEN their operations are interleaved
- THEN each operation uses its declared validated scope
- AND connection-global mutable state cannot route one session through another

#### Scenario: Explicit close and resume boundary

- GIVEN a session is disconnected without an explicit close
- WHEN a later run resumes work
- THEN the disconnected session is not treated as ended solely by transport loss
- AND resume requires an explicit selected workstream and a new validated run

### Requirement: Revisioned document publication

The slice MUST support project-general and explicitly selected workstream documents with current and historical revisions, provenance, integrity discrepancy reporting, and stale expected-revision conflicts. A document MUST be acknowledged only after its current pointer commits; precommit failure MUST preserve the prior current revision.

#### Scenario: Successful publication

- GIVEN valid prepared Markdown and the expected current revision
- WHEN the publication transaction commits
- THEN the new revision becomes current and its history and provenance remain available
- AND success is acknowledged only after commit

#### Scenario: Stale publication

- GIVEN another writer has already advanced the current revision
- WHEN a writer submits an older expected revision
- THEN the operation returns a stable conflict outcome
- AND the newer current revision remains visible

### Requirement: Checkpoint, resume, and fork distinction

The slice MUST support bounded checkpoints under a validated session binding, explicit resume into a new run, and a fork operation that is observably distinct from resume. Checkpoints MUST reference current state without claiming model or filesystem restoration, retained model context, or verification.

#### Scenario: Checkpoint handoff

- GIVEN a valid project/workstream/session binding
- WHEN a bounded checkpoint is saved
- THEN it records a resumable handoff with references and freshness-relevant state
- AND it does not claim unsaved files, hidden model state, or completed verification were restored

#### Scenario: Resume

- GIVEN a checkpoint for an explicitly selected workstream
- WHEN a new session resumes it
- THEN the new run is bound to that workstream and its permitted project context
- AND another newer or unrelated workstream is not selected by recency

#### Scenario: Fork

- GIVEN an existing workstream is explicitly selected
- WHEN a fork is created
- THEN the new workstream is identifiable as distinct from its source
- AND the source workstream is not mutated merely by creating the fork

### Requirement: Required Phase 1 operational metrics

Each Phase 1 operation MUST attempt to record an operation identity, timestamp, interface, declared project/workstream/session scope, status, total backend duration, serialized response size, and available result counts. Metrics failure MUST NOT change the business outcome, and collection MUST NOT require raw content.

#### Scenario: Successful operation with metrics failure

- GIVEN an operation completes successfully
- WHEN its counter or metric recording fails
- THEN the operation remains successful
- AND the failure is isolated from the returned business result

#### Scenario: Scoped operation accounting

- GIVEN an operation declares its applicable scope and interface
- WHEN accounting is recorded
- THEN the declared scope and interface are represented with outcome and duration
- AND missing inapplicable result counts are not invented

### Requirement: Phase boundary protection

The Phase 1 slice MUST operate without a mandatory daemon, HTTP service, Node runtime, embeddings, Phase 2 FTS retrieval, or automatic bounded context assembly. It MUST retain open decisions such as exact envelopes, pagination tokens, session interruption statuses, and retention defaults for later design.

#### Scenario: Exact continuity read

- GIVEN an authorized explicit project or workstream scope
- WHEN a caller requests an exact document, revision, list, or checkpoint continuity operation
- THEN the slice returns only the requested bounded information and provenance
- AND it does not perform FTS retrieval or automatically assemble broader context
