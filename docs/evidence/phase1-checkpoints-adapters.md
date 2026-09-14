# Phase 1 checkpoint adapter acceptance

P1.5b acceptance **passed**. A response-only SQLite DTO now always serializes each read reference's `fresh` field without changing checkpoint request decoding, canonical request bytes, or persisted operation fingerprints.

## Case matrix

| Test | Transport | Cases proved | Result |
| --- | --- | --- | --- |
| `TestCheckpointStoreComputesFreshnessAndPreservesDisconnectedSourceAcrossResume` | SQLite + revision files | exact reference; unrelated activity stays `fresh:true`; only an advanced reference becomes present `fresh:false`; disconnect/resume/fork scope | PASS |
| `TestCheckpointDigestPreservesFrozenPreChangeRequestFixture` | SQLite | request digest remains `919556…d5a29`, frozen from `dc8de02` fixture | PASS |
| `TestCheckpointCLIProcessUsesHumanAndJSON` | built CLI | human/JSON save/read, exact bytes, provenance, binding, newline metric | PASS |
| `TestCheckpointAdaptersHaveEquivalentProcessOutcomes` | isolated built CLI and SDK subprocesses | explicit `fresh:true` and `fresh:false` key presence, reconnect replay after advance with unchanged identity/live staleness, changed-input mismatch, scope mismatch, disconnect/resume/fork, 64 KiB, tamper | PASS ×5 |
| `TestMCPMetricEqualsIndependentlyCapturedJSONRPCFrame` | SDK IO frame | persisted response bytes equal independently captured checkpoint JSON-RPC frames | PASS ×5 |
| discovery coverage | SDK subprocess | project, continuity, document, and checkpoint discovery remains 21 tools | PASS |

## Strict TDD chronology

- RED: before the DTO correction, stale reads produced `references` without `fresh`; the focused store test failed with the exact missing-key JSON response. The frozen digest test first exposed its independently captured pre-change value.
- GREEN: `ReadCheckpoint` switched only its response collection to `checkpointResponseReference`, where `fresh` has no `omitempty`; the focused store tests passed.
- TRIANGULATE: real CLI/MCP process parity asserts key presence for both true and false, advances only the referenced document, then replays the original committed operation ID and receives the original identity with live `fresh:false`.
- REFACTOR: request DTO tags and digest inputs remain untouched. The full suite exposed three stale 19-tool discovery expectations; coverage-only assertions now retain the prior cases while expecting the established 21-tool surface.

## Commands and limits

| Command | Result |
| --- | --- |
| `go test ./internal/store/sqlite -run '^TestCheckpoint' -count=1 -timeout=120s` | PASS |
| `go test ./cmd/memgraphai -run '^TestCheckpointCLIProcessUsesHumanAndJSON$' -count=1 -timeout=120s` | PASS |
| `go test ./internal/adapter/mcp -run '^(TestStdioServerExposesCheckpointSaveAndReadTools|TestCheckpointAdaptersHaveEquivalentProcessOutcomes|TestMCPMetricEqualsIndependentlyCapturedJSONRPCFrame)$' -count=1 -timeout=120s` | PASS |
| checkpoint parity and frame-metric commands, each `-count=5 -timeout=120s` | PASS |
| `go test ./... -count=1 -timeout=120s` | PASS |
| `git diff --check` | PASS |

All tests use temporary libraries. MCP cleanup closes and waits for each session before its test context is canceled. P1.5b adds CLI/MCP production wiring; the freshness correction changes only the shared response DTO. No daemon, model/filesystem restoration, completed-verification claim, list/update/delete, task-reference operation, or dependency was added.

Replay compatibility is supported by the independently frozen pre-change digest, unchanged request canonicalization, and current durable replay tests. An end-to-end database created by the old executable and replayed by the new executable was not exercised.

## Preserved failed history

The earlier P1.5b attempt failed at `67b6fbe7211c570ff4c7a84c86844bd763353d04f7a0788360fe0d1e9f2097f1` because stale `fresh:false` was omitted. That result remains historical evidence and is distinct from this accepted correction.
