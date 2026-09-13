# Phase 0 exit — viable local foundation

## Decision

**The Phase 0 local foundation is viable for the tested Go, SQLite, and filesystem experiment.** P0.1 evidenced the selected local SQLite/FTS5 executable path, P0.2 evidenced identity and binding constraints, P0.3 evidenced prepare-plus-pointer publication, and P0.4 now adds restart, integrity, operation replay, fencing, and separate-process contention evidence.

This is a Phase 0 exit result only. It does not authorize Phase 1, adapters, a daemon, an authorization framework, FTS retrieval, external data access, commits, or release behavior.

## Closure record

**Phase 0 implementation, verification, and native review are complete for the tested local foundation.** This section records the result after review; it is a documentation-only addendum, not part of the frozen reviewed candidate.

- P0.1–P0.3: review `review-fa3b6ffe8b91a0fb` approved and acknowledged; the authorized local baseline is commit `39d8bee6747450dcfe7514cd86fa8595babe3213`.
- P0.4: review `review-1fec503d20e0b932` approved by all four reviewers and acknowledged successfully. Its authority is consumed (`burned`); no review continuation is pending.
- Reviewed candidate: `730ff55b0bf90e6f5edeee0a2ac365caa6056a47`, covering 10 files and 1,501 added-plus-deleted lines against the baseline, within the 2,000-line budget.
- The earlier escalated review `review-a43d9367ffa83553` remains preserved in the history; this closure does not rewrite its failed outcome.
- Eight advisory findings were explicitly non-blocking. They are separate potential follow-up work, not correction requests or grounds to reopen this candidate.

### Final verification evidence

The independent verifier observed these commands pass on the corrected implementation:

```sh
go test ./... -count=1
go test ./internal/revisionfs -count=20
go test ./internal/store/sqlite -run '^TestRecovery(Process|BoundsCrossProcess)' -count=5
```

The regression fixes cover checked missing-root creation, validation of concurrent directory-creation winners, and rejection of symlink/non-regular revision leaves before content reads. Matching regular-file retries remain accepted; checksum mismatches remain rejected. File-descriptor identity checks reduce replacement-race exposure but are not a complete TOCTOU or OS-sandbox guarantee.

Verification left repository status unchanged and no recovery helper process running. P0.4 remains uncommitted; no push was performed. This closure neither authorizes a commit nor starts Phase 1 or archives the broader OpenSpec change.

## Evidence status

| Area | Record | Result |
| --- | --- | --- |
| Build and SQLite/FTS5 | `docs/evidence/phase0-build.md` | Viable for the tested local Go path. |
| Identity and binding | `docs/evidence/phase0-identity.md` | Immutable IDs, explicit resolution, binding checks, and association-versus-grant distinctions observed. |
| Publication | `docs/evidence/phase0-publication.md` | Prepared immutable Markdown remains invisible until the SQLite pointer transaction commits. |
| Recovery and integrity | `docs/evidence/phase0-recovery.md` | Operation fingerprint/result replay, authorization recheck, fencing, targeted checksum discrepancies, and orphan reporting observed. |
| Contention and restart | `docs/evidence/phase0-concurrency.md` | Separate-process termination/reopen and bounded same-operation retry exhaustion observed. |

## P0.4 exit assertions

- A reused operation ID with a different fingerprint returns `idempotency_mismatch` before filesystem preparation.
- Every execution and replay invokes the supplied authorization check; a denied replay exposes no saved result.
- The SQLite transaction records the revision, current pointer, and terminal committed operation result together. An unknown response is resolved only by that stored result, never inferred rollback.
- An authorized committed replay returns its original revision after a legitimate later revision advances current.
- Generation-specific staging and generation-checked commit fence stale owners. Existing immutable destinations are verified rather than overwritten, and active prior-attempt staging is not removed by takeover.
- Targeted current/history verification reports missing or altered Markdown as `integrity_discrepancy`; unmatched immutable files are reported as orphans without import or deletion.
- A separate process can be terminated after claiming an operation; a reopened store safely continues under a new generation. A separate-process same-operation retry reaches configured exhaustion with stable `retryable` in a bounded test.

## Limitations and next gate

The filesystem `Sync`, link, and named-fault tests are not proof of power-loss durability, filesystem crash consistency, backup coherence, network filesystem behavior, or a supported-platform promise. The current operation fingerprint is an opaque Phase 0 equality value, not a public envelope or permission configuration. The local authorization callback is deliberately not an authorization framework.

Phase 1 remains **unapproved** until a separate human authorization. If later approved, its contract decision work must preserve these operation, integrity, and explicit-scope constraints without treating the Phase 0 experiment as a production service.

## Process cleanup facts

All P0.4 SQLite databases, libraries, staging paths, and readiness files are allocated through Go test `t.TempDir()` fixtures. The subprocess tests call `Process.Kill` and `Wait`; no helper remains after a passing test. No user library was opened, no daemon or global service was started, no global installation occurred, and no commit or external call was made.

## P0.4 parent-gate correction

The Phase 0 result remains viable for the tested local experiment, with the following corrections now evidenced:

- Conflict terminal results commit with their operation transaction and survive reopen, replay, and a later pointer advancement.
- Operation identity is an internally derived canonical digest of every immutable write field, including Markdown, and rejects semantic changes before preparation even when a caller reuses the external fingerprint.
- The caller project must own the document before a new operation can write a file; this local relation check is not a Phase 1 permission framework.
- Actual subprocess termination at a prepared-final-file/pre-commit boundary leaves the prior current revision and produces only conservative `unknown` recovery until retry.
- Actual subprocess termination after the SQLite commit but before response returns the persisted result after reopen and remains replayable after a later pointer advance.

Fresh commands for this correction are recorded in apply progress. They are local Go/SQLite evidence only and do not expand Phase 1 approval, adapter behavior, permission configuration, envelope versioning, or filesystem power-loss claims.
