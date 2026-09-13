# Phase 0 concurrency evidence

## Observed bounded ownership behavior

Ownership is a SQLite generation, not a process-local lock. One retry may take over a nonterminal operation by committing the next generation. The old owner can finish its own filesystem attempt, but its SQLite commit requires the exact current generation and therefore returns stable `retryable`. Immutable filesystem publication uses no-clobber hard links and verifies an existing destination before reuse.

The local attempt bound defaults to three generations and can be set lower by the Phase 0 caller. Once the bound is reached, `ClaimOperation` returns `retryable` with the durable generation and does not wait, write staging, or leak a raw SQLite error. The bound is a recovery safety limit, not a latency SLO.

## Separate-process tests

`TestRecoverySurvivesTerminatedOwnerAndReopen` starts a separate Go test-binary helper against a file-backed SQLite fixture. The helper claims generation one, writes a readiness file, and is forcibly terminated. The parent waits for process termination, reopens SQLite from disk, and commits the same operation as generation two.

`TestRecoveryBoundsCrossProcessRetryExhaustion` starts a separate helper that claims the same operation and remains alive. The parent takes generation two, then immediately receives stable `retryable` at its explicit two-attempt bound. Readiness files synchronize the tests; the only polling is bounded observation of process readiness, not timing used to manufacture an outcome. The assertion requires the exhaustion call to finish in under one second.

| Command | Result |
| --- | --- |
| `go test ./internal/store/sqlite -run '^TestRecoverySurvivesTerminatedOwnerAndReopen$' -count=1` | PASS: actual child termination, reopen, and fenced generation-two completion. |
| `go test ./internal/store/sqlite -run '^TestRecoveryBoundsCrossProcessRetryExhaustion$' -count=1` | PASS: actual separate-process same-operation retry and stable bounded exhaustion. |
| `go test ./internal/revisionfs ./internal/store/sqlite -count=1` | PASS: recovery, filesystem, and process checks together. |

The tests prove local process and SQLite behavior on the tested machine only. They do not prove filesystem power-loss behavior, a production lease policy, networked filesystems, daemon operation, or adapter contention semantics.

## P0.4 parent-gate correction

Two additional helpers use actual subprocess termination rather than named filesystem faults. At the prepared-final-file/pre-DB-commit boundary, the helper claims generation one, completes `PrepareAttempt`, signals readiness, and is killed before any SQLite publication transaction. On reopen, recovery is conservatively `unknown`, the prior current revision remains visible, and a new generation can safely commit the prepared immutable destination.

At the DB-committed/pre-response boundary, the helper enters the existing `afterOperationCommit` hook only after the transaction commits, signals readiness, and is killed before its caller receives a response. Reopen reads the durable committed result. A later revision then advances the pointer, and replay of the terminated operation still returns its stored earlier revision without republishing or changing current. Every helper is explicitly killed and waited; named fault coverage remains separate ordering evidence.
