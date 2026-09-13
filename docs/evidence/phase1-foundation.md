# Phase 1.1 foundation and telemetry evidence

## Scope and status

This evidence covers only P1.1's shared experimental contract, additive SQLite
migration ledger, and best-effort bounded metrics. The consumed native status was
`apply: ready` for `backend-foundation-and-continuity` in repo-local mode, with the
repository as the allowed edit root and no action-context warnings. The parent
selected this separate shared-foundation/metrics review block and retained all
proceed-token settlement.

The request/result contract is exactly `memgraphai.experimental/v1alpha1`; it is not
a stable v1 promise. Application validation requires an explicit complete scope and
never infers one from a path, environment, connection, current directory, recency, or
local-owner permission. This validation prevents accidental cross-context operations;
it is neither per-agent authorization nor an OS sandbox.

## Observed behavior

- Strict decoding rejects unknown envelope and scope fields before the handler runs.
  Missing scope IDs, incompatible IDs for a scope kind, unsupported versions, and
  page limits outside 1 through 200 return stable non-content outcomes.
- Paginable operation primitives normalize an omitted first-page limit to 50. Token
  binding and view-revision issuance are deliberately deferred to the concrete list
  and history services; no listing operation is claimed here.
- The app service serializes its experimental JSON envelope before accounting and
  records the exact byte length of that serialization, total backend duration, declared
  scope, interface, outcome, and an optional available result count. The SQLite sink
  persists this bounded record; neither type nor schema accepts raw request or result
  content.
- A recorder error is ignored after the business response is computed, so an `ok`
  response remains `ok` when metric persistence fails.
- Opening a database creates or reuses an additive version-one migration ledger and
  preserves existing Phase 0 tables/data. A future ledger version is rejected without
  changing its recorded value. The existing DSN foreign-key pragma remains active for
  each usable SQLite connection.

## Strict TDD evidence

| Cycle | RED | GREEN | TRIANGULATE / REFACTOR |
| --- | --- | --- | --- |
| Contract metric isolation (historical limitation) | The recorded `TestRedMetricFailureDoesNotChangeBusinessOutcome` failure used temporary TESTONLY behavior, not the existing production service. It does **not** meet strict production-TDD RED criteria. | The current recorder-failure GREEN test remains retained. | Unknown envelope/scope fields, unsupported versions, incompatible scopes, and invalid limits remain covered, but this does not retroactively supply the missing original feature RED. |
| Migration versioning | `go test ./internal/store/sqlite -run '^TestOpenRecordsFoundationMigrationVersion$' -count=1` failed behaviorally because `schema_migrations` did not exist. | The ordered, transactional v1 migration and future-version rejection tests passed. | Reopen preserves a Phase 0 project, has one ledger row, and receives a metric from the actual app-service-to-SQLite path; an unavailable count remains SQL `NULL`. |
| Refactor | Focused green tests were retained as safety nets. | Schema statements moved from `Store.initialize` into the ordered migration unit without changing Phase 0 schema semantics. | `gofmt`, focused suites, full suite, and `git diff --check` passed. |

## Commands observed

| Command | Result |
| --- | --- |
| `go test ./... -count=1` before P1.1 edits | PASS safety net |
| `go test ./internal/app -run '^TestRedMetricFailureDoesNotChangeBusinessOutcome$' -count=1` | Historical TESTONLY RED only; it is not strict production-behavior evidence. |
| `go test ./internal/store/sqlite -run '^TestOpenRecordsFoundationMigrationVersion$' -count=1` | Expected behavioral RED: `schema_migrations` missing |
| `go test ./internal/app -run '^(TestExecuteJSONRejectsUnknownFieldsAndUnsupportedVersionsBeforeBusinessBehavior|TestExecuteJSONRejectsMismatchedScopeAndInvalidPageLimits|TestExecuteJSONDefaultsListLimitAndIsolatesRecorderFailure)$' -count=1` | PASS GREEN |
| `go test ./internal/store/sqlite -run '^(TestOpenRecordsFoundationMigrationVersion|TestOpenRejectsFutureSchemaWithoutChangingIt)$' -count=1` | PASS GREEN |
| `go test ./internal/store/sqlite -run '^TestOpenAppliesMigrationOnceAndPersistsBoundedMetric$' -count=1` | PASS TRIANGULATE |
| `go test ./... -count=1` | PASS final |
| `git diff --check` | PASS |

## Corrective P1.1 nil-handler evidence

A new regression used the pre-existing production `Service.ExecuteJSON` directly with a valid request and a nil handler. It was a genuine RED: `go test ./internal/app -run '^TestExecuteJSONNilHandlerReturnsInternalAndAttemptsMetrics$' -count=1` failed with `panic: runtime error: invalid memory address or nil pointer dereference` at `internal/app/contract.go:101`. GREEN adds only a nil-handler branch: it returns `internal`, does not use blanket panic recovery, and still attempts one metric with the final response byte count.

Triangulation confirms absent `input` is invalid while `{}` reaches the handler; missing or blank operation IDs are invalid and unechoed; unsupported versions retain the server contract envelope and a valid ID; and unknown page fields are invalid before the handler runs. The existing recorder-failure GREEN test remains intact. This correction fixes the observed panic but does **not** retroactively supply the missing original metric-isolation production RED.

## Narrow human exception and current status

The user explicitly accepted the original TESTONLY metric-isolation RED deficiency as a narrow historical exception. This accepts P1.1 implementation with that deficiency documented; it neither converts the result into valid production-behavior RED nor waives strict TDD for future work.

A prior verifier independently observed the full suite passing, the nil-handler regression passing 20 consecutive runs, and envelope-edge triangulation passing 10 consecutive runs. Those are historical results, not fresh verification by this documentation reconciliation. Fresh verification and native review remain pending with the parent; full Phase 1 and review closure are not claimed.

| Command | Result |
| --- | --- |
| `go test ./internal/app -count=1` before correction | PASS safety net |
| `go test ./internal/app -run '^TestExecuteJSONNilHandlerReturnsInternalAndAttemptsMetrics$' -count=1` | Genuine RED: nil-handler panic at `contract.go:101` |
| same focused command after GREEN | PASS |
| `go test ./internal/app -run '^TestExecuteJSONTriangulatesEnvelopeEdges$' -count=1` | PASS triangulation |
| `gofmt -w internal/app/contract.go internal/app/contract_test.go && go test ./internal/app -count=1` | PASS refactor/safety net |

Tests create no durable fixture or build artifact; only short-lived Go test processes ran. No dependency, global tool, user library, persistent process, commit, push, paid call, or external integration was used.

## Limitations and deferred boundaries

No CLI operation beyond the existing Phase 0 probe was added, and no CLI/MCP adapter,
MCP SDK, daemon, SDK integration, project/path service, workstream/session service,
document service, checkpoint service, FTS retrieval, embeddings, or pagination listing
service was introduced. Therefore this evidence does not claim CLI/MCP protocol parity,
final MCP response-byte measurement, token/view-revision binding, production owner-local
filesystem access, or an authorization framework. The official Go MCP SDK remains a
future compatibility-unverified proposal. Tests use only `t.TempDir()` SQLite fixtures;
no user library, persistent process, dependency installation, commit, push, paid call,
or external integration was used.

## Key Learnings

1. The original metric-isolation RED remains TESTONLY evidence and is not production-behavior RED.
2. The nil-handler regression supplied an independent genuine RED/GREEN cycle without repairing that history.
3. P1.1 implementation is accepted only under the explicit narrow exception; future strict-TDD duties remain unchanged.
4. Prior passing verification is historical; fresh verification and native review remain parent-owned and pending.
