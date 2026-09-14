# P1.4b document CLI and MCP adapter evidence

P1.4b exposes the settled P1.4a `DocumentService` through the built CLI and MCP stdio server. Current verification passes after correcting the parity test's process-cleanup order and completing adapter-equivalence coverage for history-token scope binding and mutation staleness.

## Delivered surface

| Interface | Evidence |
| --- | --- |
| CLI human | A built executable ran `document create`, `update`, `list`, `read`, and `history`; each emitted the established exact `ok\n` human outcome. |
| CLI JSON | The built executable ran all five operations with explicit project scope, nested provenance, unpadded base64, create `expected_revision_id: null`, and update's exact expected revision. |
| MCP stdio | A real `gomcp.CommandTransport` server listed 19 tools and ran all five document tools. |
| Metrics | An independently captured newline-delimited `document.create` JSON-RPC frame exactly matched persisted `operation_metrics.response_bytes`. |

Both executable paths construct `DocumentService{Store: store, Files: revisionfs.New(root, nil)}` against the selected SQLite library. Neither adapter derives document scope from a current project, recent activity, connection state, or session state.

## Current parity matrix

`TestDocumentAdaptersHaveEquivalentSemanticEnvelopes` compares CLI and MCP semantic envelopes for:

| Area | Cases |
| --- | --- |
| Setup and writes | project/workstream creation, project-general and workstream document creation, update, identical create replay, checksum, byte count, and provenance |
| Scope | project-general list excludes workstream documents; workstream list excludes project-general documents; wrong-scope current read and foreign historical revision read return equivalent explicit errors |
| Exact reads | current revision two and historical revision one both preserve `AP9NYXJrZG93bgo` before any fixture tampering |
| Conflicts and replay | stale update preserves current; changed immutable replay returns idempotency mismatch |
| Pagination | list and history continuation, own-token use, same-operation scope rebinding, post-revision-mutation staleness, and zero/negative/over-maximum limits for both operations |
| Integrity | current and historical files are tampered only after positive exact-byte reads; both transports then report integrity discrepancy |

These additions cover already-working service behavior. They are triangulation, not claims of a new production RED.

## Strict TDD and interrupted-follow-up evidence

| Cycle | RED / failure | GREEN / triangulation |
| --- | --- | --- |
| Historical CLI adapter work | Built-process human document create returned `memgraphai: invalid command`. | The original focused adapter command passed after narrow CLI wiring. |
| Historical MCP adapter work | MCP discovery returned 14 tools, want 19. | The original focused adapter command passed after five document tool registrations. |
| Interrupted test-only follow-up | The independent full suite and targeted parity run failed at `TestDocumentAdaptersHaveEquivalentSemanticEnvelopes` cleanup with `wait MCP: signal: killed`. The test deferred context cancellation before `t.Cleanup`, killing the child before Close/Wait. This is retained as the initial failed acceptance result, not relabeled as a production-behavior RED. | Context cancellation is now registered before the Close/Wait cleanup and uses an independent bounded context, so LIFO cleanup closes and waits before cancellation. Close and Wait errors remain asserted; the three-second cleanup deadline remains bounded. |
| Added parity cases | No new RED claimed: historical exact-byte read, wrong-scope read, history invalid-limit behavior, and history-token binding already worked. | The current focused matrix passed five times. Each transport's history token is now rebound through `document.history` with the original document input under a changed scope and returns `invalid`; after a document revision mutation, reusing each original token under its own scope returns `conflict`. Existing list-token assertions remain covered by the same matrix. |

A first post-edit focused run still failed: the wrong-scope test expected `binding_mismatch` while both adapters correctly and equivalently returned `scope_denied`; cleanup also remained vulnerable because `t.Context()` is canceled before test cleanups. The test expectation was corrected to the settled outcome and the operation context was moved to an independent 30-second deadline. No production source changed.

## Commands and results

| Command | Result |
| --- | --- |
| `go test ./cmd/memgraphai ./internal/adapter/mcp -run '^(TestDocumentCLIProcessUsesAllHumanAndJSONOperations|TestDocumentCLIReplayRemainsReachableAfterDiscardedResponse|TestStdioServerExposesDocumentToolsAndExactBytes|TestMCPMetricEqualsIndependentlyCapturedJSONRPCFrame|TestDocumentAdaptersHaveEquivalentSemanticEnvelopes)$' -count=5 -timeout=120s` | PASS: `cmd/memgraphai` in 12.652s and `internal/adapter/mcp` in 16.918s. |
| `go test ./... -count=1 -timeout=120s` | PASS for every package; `internal/telemetry` has no test files. |

The earlier PASS runs in the interrupted evidence and progress record are historical only; the two Go commands above are the current final verification.

## Boundary and cleanup

The CLI replay test commits in one process while discarding stdout, then retrieves the durable result through an identical second process. MCP replay closes and waits before reconnecting. These prove durable reachability after response loss, not P0's post-commit process-kill boundary.

All fixtures use `t.TempDir()`. Adapter processes have bounded contexts; MCP cleanup asserts Close and Wait errors and a three-second deadline. No dependency, production source, user library, daemon, advisory fix, or persistent process was added.

## Key Learnings

- A history token must be tested through `document.history`; sending it to `document.list` proves operation mismatch, not history scope binding.
- Scope rebinding keeps the original valid `document_id` so token validation, rather than a store lookup mismatch, determines the `invalid` outcome.
- Capturing transport-specific history tokens before a document revision mutation proves both adapters return `conflict` when those own-scope tokens become stale.
