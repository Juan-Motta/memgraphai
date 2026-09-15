# Backend Foundation Evidence Specification

## Purpose

Phase 0 MUST produce reproducible evidence for the storage, packaging, identity, filesystem, concurrency, and recovery foundations required before Phase 1. Evidence is planning and validation output; it MUST NOT claim unsupported platform support or select an option without evidence.

## Requirements

### Requirement: Reproducible SQLite and executable evidence

Phase 0 MUST record an evidence-backed SQLite integration and one-executable build path, including an executable FTS5 feature probe. The evidence MUST identify blockers and an evaluated alternative if viability fails, without delivering Phase 2 retrieval behavior.

#### Scenario: Viable build path

- GIVEN the intended macOS-first executable path is evaluated
- WHEN the SQLite build and feature probe complete
- THEN the evidence records the tested integration, build conditions, and FTS5 result
- AND it does not claim untested architectures, minimum OS versions, signing, or release support

#### Scenario: Viability failure

- GIVEN the evaluated path cannot provide the required build or FTS5 evidence
- WHEN Phase 0 records the result
- THEN it identifies the blocker and an evaluated alternative
- AND Phase 1 is not treated as unblocked

### Requirement: Stable identity and binding constraints

The foundation MUST demonstrate constraints for immutable project identity, multiple path associations, workstream and session relationships, explicit scope, and ambiguity. It MUST reject inconsistent bindings, silent merges, and unapproved path or recency guesses.

#### Scenario: Ambiguous association

- GIVEN aliases, symlinks, nested associations, unavailable paths, or same-named projects create ambiguity
- WHEN identity resolution is exercised
- THEN the result is an explicit ambiguity or unavailable outcome
- AND no project or workstream is silently selected

#### Scenario: Consistent binding

- GIVEN project, workstream, and session identities are supplied
- WHEN their relationships are validated
- THEN inconsistent or unauthorized combinations are rejected
- AND a valid combination preserves each identity as distinct

### Requirement: Revision publication and recovery evidence

The foundation MUST model durable Markdown preparation followed by a short SQLite publication transaction with expected-revision conflict detection. It MUST classify modeled interruption windows, support idempotent operation recovery, detect orphan or checksum discrepancies, and preserve the prior current revision when publication does not commit.

#### Scenario: Preparation or required precommit failure

- GIVEN a prior current revision exists
- WHEN preparation or another required step fails before the publication commit
- THEN the prior revision remains current
- AND the staged content is not acknowledged as published

#### Scenario: Unknown commit outcome

- GIVEN process or transport loss makes commit status unknown
- WHEN recovery is requested using operation identity
- THEN the result is classified and resolved idempotently when possible
- AND the system does not claim rollback or blindly repeat a possibly committed write

#### Scenario: Direct source alteration

- GIVEN served current or historical Markdown is altered outside the publication path
- WHEN targeted checksum verification runs
- THEN an integrity discrepancy is reported or the content is explicitly imported as a new revision
- AND the altered content is not silently accepted as current

### Requirement: Bounded contention outcomes

The foundation MUST characterize concurrent CLI and MCP access with short transactions and bounded waits or retries. Exhaustion MUST produce a stable busy, retryable, conflict, or unknown outcome rather than a raw storage error, silent overwrite, or indefinite wait.

#### Scenario: Concurrent stale update

- GIVEN two writers target the same current revision
- WHEN one writer commits before the other
- THEN the later stale writer receives a stable conflict outcome
- AND it cannot replace the newer revision silently

#### Scenario: Contention exhaustion

- GIVEN concurrent processes exhaust the configured bounded contention handling
- WHEN an operation stops retrying
- THEN it returns a stable retryable or busy outcome
- AND the process does not wait indefinitely
