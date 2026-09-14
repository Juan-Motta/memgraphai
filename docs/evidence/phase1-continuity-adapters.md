# P1.3b continuity adapter evidence

P1.3b exposes the already-approved P1.3a continuity services through the built CLI and MCP stdio server. It adds no application or SQLite behavior, dependency, daemon, or transport-loss lifecycle mutation.

## Verified behavior

| Surface | Evidence |
| --- | --- |
| CLI | Built-process JSON requests cover all eight continuity operations, a meaningful human `workstream create` command, explicit close/status, resume source identity, and mismatched session scope. |
| MCP | A real SDK `CommandTransport` lists all 14 project/continuity tools and calls every continuity tool through the executable. |
| Parity | Isolated CLI and MCP libraries receive identical requests and return identical semantic envelopes for create, list, fork, open, status, disconnect, close, resume, mismatched binding, and missing session. The comparison removes only server-generated `created_at` and `opened_at` timestamps. |
| Connection lifecycle | A client close/EOF makes no persistence change; fresh CLI processes read both source sessions as `open`. An explicit MCP `session.disconnect` with `observed_by` changes only the named session, while another project/workstream session remains `open`. |

## Strict TDD cycle evidence

| Cycle | RED command and observed result | GREEN | TRIANGULATE / refactor |
| --- | --- | --- | --- |
| Built CLI continuity dispatch | `go test ./cmd/memgraphai -run '^TestContinuityCLIProcessUsesHumanAndJSON$' -count=1 -timeout=120s` failed behaviorally: human `workstream create` exited 2 with `memgraphai: invalid command`. | The same command passed after CLI normalization selected `ContinuityService` and accepted workstream/session commands. | The process test exercises JSON/human forms, all operations, close/resume, and binding mismatch; `gofmt` and package checks passed. |
| SDK MCP continuity discovery | `go test ./internal/adapter/mcp -run '^TestStdioServerListsAndCallsContinuityTools$' -count=1 -timeout=120s` failed behaviorally: `ListTools() = 6 tools, want 14 project and continuity tools`. | The same command passed after exact continuity tool registration and shared-handler dispatch. | Process parity, missing/mismatched scope, two-project session interleaving, explicit attribution, and EOF tests passed with no adapter-local session routing. |
| MCP metrics isolation | A repeated full-suite run failed behaviorally at `TestStdioServerListsAndCallsContinuityTools`: `session.disconnect` or `session.close` returned `internal` while the prior response metric could still write SQLite. | `go test ./internal/adapter/mcp -run '^TestStdioServerListsAndCallsContinuityTools$' -count=10 -timeout=120s` passed after a pending-metric completion barrier before the next shared operation. | The metric retains final-frame byte accounting, recorder failures remain ignored, and five complete repository runs passed without the lifecycle race. |

The RED results above were executed before the corresponding production adapter wiring. They are not compilation failures and do not reinterpret P1.3a service evidence.

## Commands and results

| Command | Result |
| --- | --- |
| `go test ./internal/adapter/cli ./internal/adapter/mcp ./cmd/memgraphai -count=1 -timeout=120s` before P1.3b edits | PASS safety net |
| CLI RED command above | Expected behavioral failure, then PASS GREEN |
| MCP RED command above | Expected behavioral failure, then PASS GREEN |
| `go test ./internal/adapter/mcp -run '^(TestContinuityAdaptersHaveEquivalentOutcomes|TestContinuityMCPEOFLeavesSessionsUnchangedAndDisconnectIsScoped)$' -count=1 -timeout=120s` | PASS |
| `go test ./internal/adapter/mcp -run '^(TestStdioServerListsAndCallsProjectTool|TestStdioServerListsAndCallsContinuityTools|TestContinuityAdaptersHaveEquivalentOutcomes|TestContinuityMCPEOFLeavesSessionsUnchangedAndDisconnectIsScoped|TestMCPMetricEqualsIndependentlyCapturedJSONRPCFrame)$' -count=1 -timeout=120s` | PASS |
| `go test ./internal/adapter/cli ./cmd/memgraphai -count=1 -timeout=120s` | PASS |
| `go test ./internal/adapter/mcp -run '^TestStdioServerListsAndCallsContinuityTools$' -count=10 -timeout=120s` | PASS after the metric-isolation correction |
| `go test ./... -count=1 -timeout=120s` repeated five times | PASS on retry runs 1/5 through 5/5; every package passed and `internal/telemetry` correctly reported no test files. |
| `git diff --check 73e2c7d` | PASS after the final product checks. |

## Limits and cleanup

- The MCP tool server carries no connection-global current project, workstream, or session; every request supplies full explicit scope.
- The adapter treats raw connection end as unattributable and does not call `session.disconnect`; only a received explicit tool call with `observed_by` is attributable.
- Each SDK subprocess test uses a deadline, calls `Close`, then `Wait`; fixtures and build outputs are allocated with `t.TempDir()` and are cleaned by the Go test framework. Final process inspection returned no `memgraphai` process.
- This local evidence does not claim power-loss durability, automatic reconnect/reuse, hidden context restoration, daemon behavior, FTS retrieval, or a public stable API.
