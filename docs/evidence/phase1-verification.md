# Phase 1 verification evidence

P1.6 hardening completed on the repository-local Phase 1 slice. The verification found and closed two cross-cutting gaps: successful handler output could exceed the proposed 2 MiB serialized-response bound, and project/continuity writes translated bounded SQLite contention to `internal` instead of the stable `busy` outcome. No deferred feature was added.

## Tested environment

- `go version go1.27.0 darwin/arm64`
- `Darwin 25.6.0 arm64`
- SQLite integration remains `modernc.org/sqlite v1.58.0`.
- Every database, revision library, and built test executable used a Go temporary directory.

These are observations for this local run, not minimum-OS, signing, release, power-loss, or other-platform claims.

## Strict TDD chronology

| Cycle | RED | GREEN | TRIANGULATE | REFACTOR |
| --- | --- | --- | --- | --- |
| Bounded shared response | `go test ./internal/app -run '^TestExecuteJSONBoundsOversizedSuccessfulResponses$' -count=1 -timeout=120s` failed with `ok` and 2,097,167 result bytes instead of a bounded `internal` response. | The shared executor now replaces an oversized or unmarshalable successful result with a small correlated `internal` envelope, clears partial result/count data, and records the actual fallback bytes. The focused test passed. | Ordinary nonempty success remains `ok`; malformed handler JSON becomes valid bounded JSON with the same valid operation ID. Both focused cases passed. | A named shared limit and one fallback path cover size and marshal failures without adapter-specific duplication. `gofmt` and the focused package suite passed. |
| Stable contention outcome | `go test ./internal/app -run '^TestProjectAndContinuityContentionReturnStableBusy$' -count=1 -timeout=120s` failed in both subtests with `internal`, want `busy`. | Project and continuity SQLite write errors now reuse the established recovery error classifier, and both application mappers preserve `busy`/`retryable`. The focused test passed. | Separate project and workstream writes, each blocked by an independently held SQLite write transaction, returned `busy` within one second; the combined focused matrix passed five times. | The fix reuses `mapRecoveryError` rather than introducing another driver-string classifier. `gofmt`, focused store/app tests, and the full suite passed. |

The initial oversized RED emitted an impractically large failure value; the test-only diagnostic was narrowed and the behavioral RED was rerun before production code changed.

## Cross-cutting acceptance matrix

| Concern | Executed evidence | Result |
| --- | --- | --- |
| Bounded responses and isolated metric failure | New response-bound tests plus `TestExecuteJSONDefaultsListLimitAndIsolatesRecorderFailure` | PASS ×5; fallback metrics match serialized bytes and do not retain a result count. |
| Pagination edges and stale/rebound views | Project, association, workstream, and document page tests | PASS ×5; bounded limits and bound/stale tokens retain stable outcomes. |
| CLI/MCP semantic parity and scope isolation | Real-process continuity, EOF/session-scope, document, and checkpoint parity tests | PASS; requests remained explicitly scoped across both adapters. |
| Final wire accounting | Independently captured MCP JSON-RPC frame tests | PASS for project, document, and checkpoint operations. |
| Crash/restart, idempotency, unknown outcomes, and contention | SQLite operation/recovery/targeted/document/checkpoint tests, including helper-process termination and retry exhaustion | PASS; committed replay, conservative unknown, stale conflict, and bounded retry evidence remained green. |
| Integrity and immutable publication | Targeted SQLite checks plus revision filesystem prepare/read/verify boundary tests | PASS; altered, missing, symlinked, and nonregular revision paths remain ineligible. |
| Full repository | `go test ./... -count=1 -timeout=120s` | PASS for every Go package; `internal/telemetry` has no standalone test files. |

## Commands run

```text
go test ./... -count=1 -timeout=120s

go test ./internal/app -run '^(TestExecuteJSONBoundsOversizedSuccessfulResponses|TestExecuteJSONResponseBoundPreservesOrdinarySuccessAndContainsMalformedHandlerOutput|TestProjectAndContinuityContentionReturnStableBusy|TestExecuteJSONDefaultsListLimitAndIsolatesRecorderFailure|TestProjectListIssuesBoundTokensAndRejectsStaleOrReboundTokens|TestAssociationListIsBoundedAndRejectsReboundOrStaleTokens|TestContinuityServiceBindsWorkstreamPagesAndInterleavedSessionScopes|TestDocumentServiceBindsBoundedListTokensBeforeAllocation)$' -count=5 -timeout=120s

go test ./internal/adapter/mcp -run '^(TestContinuityAdaptersHaveEquivalentOutcomes|TestContinuityMCPEOFLeavesSessionsUnchangedAndDisconnectIsScoped|TestDocumentAdaptersHaveEquivalentSemanticEnvelopes|TestCheckpointAdaptersHaveEquivalentProcessOutcomes|TestMCPMetricEqualsIndependentlyCapturedJSONRPCFrame)$' -count=1 -timeout=180s

go test ./internal/store/sqlite -run '^(TestOperation|TestRecovery|TestTargeted|TestDocumentStore|TestCheckpointStore)' -count=1 -timeout=120s

go test ./internal/revisionfs -run '^(TestPrepare|TestReadVerified|TestVerify|TestEnsureDir)' -count=1 -timeout=120s

go test ./internal/app ./internal/store/sqlite -count=1 -timeout=120s
go test ./... -count=1 -timeout=120s
git diff --check
pgrep -x memgraphai | wc -l
```

The final full suite passed, `git diff --check` produced no output, and the process count was `0`.

## Limits and non-goals

- The deterministic contention RED uses separate SQLite connections with the configured 100 ms busy timeout; helper-process recovery tests provide the separate-process fencing and exhaustion evidence. It is not a throughput target or proof for every scheduler interleaving.
- Real CLI and MCP process parity tests run in the same matrix, but this run did not hold a synthetic long-lived database lock across both adapters simultaneously.
- Filesystem sync, injected faults, and process termination do not prove physical power-loss durability, backup coherence, network-filesystem behavior, or release-platform support.
- No FTS retrieval, automatic context assembly, daemon, HTTP service, embeddings, provider integration, purge behavior, checkpoint list/update/delete, or restoration claim was introduced.

## Remediation evidence for failed verification revision

This additive evidence is bound to the native ledger's failed verification revision `sha256:788c7b583146a9e2bff483ab3f4ea6103ca9558fd92468afd8519e30d7b66fef`; the historical report retained label is `sha256:ab7fe8a16d04d4fa1c70a05374de01550f46aa16afba7e0835ee07620727aae5`. It does not replace that failed report, relabel the historical TESTONLY metric-isolation RED, or claim a new RED for already-correct production behavior.

The existing `TestExecuteJSONIgnoresProductionRecorderFailure` regression exercises the production `Service.ExecuteJSON` recorder path with a real `internal/store/sqlite.Store`. Its first request persists an operation metric. Repeating the same valid operation ID makes the production recorder reject the duplicate primary key while the second handler execution and business response remain `ok`; the serialized response, result, correlation ID, and result count remain stable, and exactly one metric row is durable.

| Remediation step | Command | Result |
| --- | --- | --- |
| Production-path regression | `go test ./internal/app -run '^TestExecuteJSONIgnoresProductionRecorderFailure$' -count=1 -timeout=120s` | PASS: `ok memgraphai/internal/app` in 1.059s. |
| Package triangulation | `go test ./internal/app -count=1 -timeout=120s` | PASS: `ok memgraphai/internal/app` in 0.909s. |

No product or test code changed in this remediation. The historical TESTONLY limitation and its narrow human exception remain recorded in `docs/evidence/phase1-foundation.md`, `tasks.md`, `apply-progress.md`, and the failed verification report. Fresh independent verification remains required before archive.
