# Phase 1 project operations

## Contract surface

P1.2 exposes the experimental `memgraphai.experimental/v1alpha1` project operations:

- `project.create`, `project.list`, and `project.resolve` require library scope.
- `project.association.add`, `.list`, and `.remove` require explicit target-project scope.
- CLI human flags and JSON stdin construct the shared envelope; MCP typed tool inputs do the same.

No adapter derives missing scope from a path, working directory, connection, name, or prior request. A missing association scope is `scope_denied`; no path match selects a project; several project matches are `ambiguous`; unavailable and unmatched paths are `not_found` in this alpha envelope.

## Observed pagination

`project.list` is keyset ordered by immutable `project_id`, with contract limits 1–200 (default 50). A continuation is unpadded base64url canonical JSON binding the experimental version, operation, complete scope digest, empty-filter digest, `project_id:asc`, limit, cursor, and SQLite view generation. The continuation returns `conflict` after an observed project/association view change; malformed or differently limited tokens return `invalid`.

`project.association.list` is bounded and deterministic: SQLite orders by `supplied_path` and applies `LIMIT`. A 52-row behavior test returns the first 50 rows plus a continuation, then rejects cross-project rebinding as `invalid` and a changed association view as `conflict`.

## Actual adapter evidence

The built CLI process performs a human create and JSON-stdin calls for all six operations. It verifies project-list pagination issuance, a symlink alias, nested-path noninheritance, same-path multi-project ambiguity, moved-path unavailability, explicit association removal, and missing-scope rejection.

The SDK `CommandTransport` process lists exactly six tools and performs the same six operations. It asserts structured shared outcomes for the same alias, nested, ambiguous, unavailable, and missing-scope cases; malformed SDK arguments may be rejected by protocol schema validation before business validation, while accepted business inputs use the shared stable outcome.

## Final emitted-byte metrics

The CLI records the actual writer byte count: JSON output includes its trailing newline and human output includes its rendered line framing. MCP now runs the official SDK with `IOTransport` and a writer wrapper. The wrapper buffers complete newline-delimited JSON-RPC frames, finds only pending operation IDs in the emitted response, and records that exact frame length after a successful write. It does not call the recorder for initialization frames or unrelated responses; recorder errors remain ignored.

This replaces the initial partial implementation's payload-only MCP metric. The metric is protocol-frame bytes, not merely the embedded shared payload. It does not promise byte attribution for an SDK batch containing more than one operation response.

## Strict TDD evidence

| Cycle | RED command and observed failure | GREEN / triangulation |
| --- | --- | --- |
| Keyset continuation | `go test ./internal/app -run '^TestProjectListIssuesBoundTokensAndRejectsStaleOrReboundTokens$' -count=1` failed: `first page next_token is empty` | Focused app test passes for continuation, malformed token, limit rebinding, and stale view conflict. |
| CLI pagination flags | `go test ./cmd/memgraphai -run '^TestProjectCLIProcessUsesHumanAndJSONForAllOperations$' -count=1` failed with CLI `invalid` list input | Focused built-process test passes for human/JSON CLI parity and path outcomes. |
| MCP frame metric | `go test ./internal/adapter/mcp -run '^TestStdioServerListsAndCallsProjectTool$' -count=1` failed: `MCP response bytes = 192, payload bytes = 192` | The same real SDK stdio test passes after `IOTransport` writer framing records a larger final JSON-RPC response. |
| CLI frame metric | `go test ./internal/adapter/cli -run '^TestCLIRecordsExactNewlineDelimitedOutputBytes$' -count=1` failed: `CLI response bytes = 194, emitted bytes = 195` | Focused test passes after recording actual writer bytes. |

## Commands observed after GREEN

```text
go test ./internal/app -run '^(TestProjectListIssuesBoundTokensAndRejectsStaleOrReboundTokens|TestProjectServicePreservesExplicitAssociationAndResolutionOutcomes)$' -count=1
go test ./internal/adapter/cli -run '^(TestCLIRecordsExactNewlineDelimitedOutputBytes|TestHumanCreateBuildsTheSharedJSONEnvelope)$' -count=1
go test ./cmd/memgraphai -run '^TestProjectCLIProcessUsesHumanAndJSONForAllOperations$' -count=1
go test ./internal/adapter/mcp -run '^TestStdioServerListsAndCallsProjectTool$' -count=1
go test ./... -count=1
git diff --check
```

All listed commands passed in this follow-up. CLI and MCP tests use `t.TempDir()` libraries; the MCP cleanup explicitly calls both `Close` and `Wait` on the client session. No user library, daemon, HTTP service, FTS retrieval, embedding, provider, live integration, or dependency beyond the already-approved official SDK was used.

## Independent P1.2 verification — PASS

The earlier independent gate recorded P1.2 as incomplete because association listing had no proven bound/continuation, MCP metric equality was not independently captured, and cleanup was not deadline-bounded. That failure history remains valid for the earlier candidate; the evidence below supersedes its current-state conclusion.

Independent verification passed the full suite, repeated real CLI/MCP process checks, and repeated association pagination checks. The MCP metric test independently captured the actual newline-delimited JSON-RPC frame and proved byte equality with the persisted metric. Child-reaping coverage uses a deterministic 2-second context and 20-millisecond terminate duration, asserts `ProcessState`, and left no current processes.

```text
go test ./internal/app ./internal/adapter/mcp ./cmd/memgraphai -run '^(TestAssociationListIsBoundedAndRejectsReboundOrStaleTokens|TestMCPMetricEqualsIndependentlyCapturedJSONRPCFrame|TestCommandTransportReapsUnresponsiveChild|TestProjectCLIProcessUsesHumanAndJSONForAllOperations)$' -count=1 -timeout=60s
go test ./... -count=1 -timeout=120s
go test ./cmd/memgraphai ./internal/adapter/mcp -run '^(TestProjectCLIProcessUsesHumanAndJSONForAllOperations|TestStdioServerListsAndCallsProjectTool|TestMCPMetricEqualsIndependentlyCapturedJSONRPCFrame|TestCommandTransportReapsUnresponsiveChild)$' -count=5 -timeout=60s
go test ./internal/app -run '^TestAssociationListIsBoundedAndRejectsReboundOrStaleTokens$' -count=10 -timeout=60s
git diff --check 267aa72
```

All commands passed. Remaining direct `Wait` calls are backed by a 20-second `exec.CommandContext` and package timeout; this does not prove every possible hang is impossible. No MCP batch-attribution guarantee is claimed. P1.2 implementation is complete, while native review remains pending. No user library, dependency addition, commit, or authority action was involved.
