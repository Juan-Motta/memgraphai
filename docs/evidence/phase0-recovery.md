# Phase 0 recovery evidence

## Observed recovery model

P0.4 adds a local SQLite operation ledger. An operation row binds its operation ID to an opaque request fingerprint, document/revision identity, expected current revision, fenced owner generation, and terminal result. `ClaimOperation` inserts or reads that row and atomically increments the generation for a nonterminal owner. A stale generation cannot record the revision metadata, pointer, or terminal result.

`ExecuteOperation` calls the supplied authorization function before every registration, retry, and saved-result replay. The operation ID is not a grant. A different fingerprint returns `idempotency_mismatch` without filesystem work. A committed replay returns its persisted revision result even after another committed operation advances the document pointer. The SQLite publication transaction inserts the revision metadata, advances the pointer, and writes the committed operation result together; an injected lost response is `unknown` to that caller, while a later authorized recovery reads the durable result rather than inferring rollback.

Attempt-specific staging is `.staging/<operation-id>/<generation>/`. A retry after an interruption after the immutable hard link but before final-directory sync verifies the deterministic destination checksum and byte length, then completes the required directory sync without replacement. The older attempt is not cleaned merely because a newer generation claims ownership. Root and existing intermediate library directories reject symlink traversal.

## Integrity and orphan classification

`VerifyRevision` checks only the revision requested through SQLite metadata. Altered historical content and a missing current file both return `integrity_discrepancy`; there is no fallback, rescan, or auto-import. `Orphans` compares immutable revision paths with stored revision metadata and reports unmatched files only. The P0.4 fixture leaves the reported orphan untouched.

## Strict TDD evidence

| Cycle | Executable assertion and result | GREEN / triangulation / refactor |
| --- | --- | --- |
| Durable replay | RED: `go test ./internal/store/sqlite -run '^TestOperationReplayReturnsTheDurableTerminalResult$'` failed with `replayed outcome = "unknown", want committed` using a compilable test-only stub. | GREEN replaced the stub with the SQLite operation ledger; the focused test passes and proves replayed `revision-1` remains the stored result after `revision-2` advances current. |
| Retry identity and response loss | Focused tests cover a changed fingerprint and a denied replay. | A response-loss hook returns `unknown` after commit; recovery returns the stored committed result, while a missing row stays `unknown` rather than becoming rollback. |
| Fencing and staging | A first owner stops before publication, then a second claim increments the generation. | The stale owner receives `retryable` on commit, active generation-one staging remains after takeover, and the later owner commits safely. |
| Integrity and orphan reporting | Current and historical paths are deliberately removed or changed after commit. | Targeted verification reports `integrity_discrepancy`; a prepared unmatched revision is reported as an orphan and is neither imported nor deleted. |
| Filesystem boundary contrasts | Existing preparation coverage remains a safety net. | Retry after `AfterPublish` preserves exact content; a changed attempt payload reports integrity; root and intermediate symlink boundaries are rejected. |

## Commands and limits

| Command | Result |
| --- | --- |
| `go test ./internal/store/sqlite -run '^TestOperationReplayReturnsTheDurableTerminalResult$'` | Expected behavioral RED, then PASS GREEN. |
| `go test ./internal/store/sqlite -run '^(TestOperation|TestRecovery|TestTargeted)'` | PASS: replay, mismatch, authorization, unknown outcome, takeover, checksum, and orphan cases. |
| `go test ./internal/revisionfs -run '^(TestPrepareAttemptRetriesAfterLinkWithoutClobbering|TestPrepareRejectsSymlinkedRootAndIntermediateBoundaries)$'` | PASS: post-link retry and symlink boundary contrasts. |

The named filesystem faults and process termination checks exercise this implementation's retry and ordering paths. File and directory `Sync` calls are not power-loss, crash-consistency, backup, or platform-support guarantees.

## P0.4 parent-gate correction

The ledger now stores a canonical SHA-256 digest of the external fingerprint, caller project ID, document ID, revision ID, explicit-null-or-value expected current revision, and Markdown SHA-256. `MaxAttempts` and all rendering/retry controls are excluded. The original external fingerprint is stored separately only for `RecoverOperation` callers; it cannot substitute for the canonical equality check. Reusing an operation ID with the same external fingerprint but a changed project, document, revision, expected current value, or Markdown is rejected as `idempotency_mismatch` before filesystem preparation, including while the prior operation is pending.

A fresh request validates that the caller project owns the document in the same claim transaction before it can register or prepare a file. A mismatched caller project returns `binding_mismatch` and leaves no revision path. For an existing operation, canonical identity is compared first so a changed project remains an idempotency mismatch rather than becoming an alternate write path.

The stale-current branch now commits its `conflict` terminal operation row before returning. Fresh reopen coverage confirms that both `RecoverOperation` and a later replay return that durable conflict after a separate operation advances current.
