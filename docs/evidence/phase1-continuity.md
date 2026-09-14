# P1.3a continuity shared-service contract

P1.3a implements only shared application and SQLite persistence behavior. CLI and MCP wiring, protocol parity, connection disconnect handling, and MCP interleaving tests are deliberately deferred to P1.3b.

## Exact operations

All operations use `memgraphai.experimental/v1alpha1`, caller-provided stable IDs, and the existing explicit scope. The service never generates IDs, selects by name, or uses recency. `origin`, `client`, and `model` are precisely named client-reported provenance inputs; they are not authentication grants or restored model context. The service supplies UTC timestamps.

| Operation | Required scope | Input | Result |
| --- | --- | --- | --- |
| `workstream.create` | `project` | `workstream_id`, `origin`, optional `client`, optional `model` | Stored metadata for the supplied ID. |
| `workstream.list` | `project` | `{}` and bounded page | ID-keyset ordered metadata, default limit 50 and maximum 200. |
| `workstream.fork` | `workstream` naming the source | `workstream_id`, `origin`, optional `client`, optional `model` | A distinct same-project workstream with `forked_from_workstream_id`; the source is unchanged. |
| `session.open` | `workstream` | `session_id`, `origin`, optional `client`, optional `model` | A new `open` session after parent validation. |
| `session.status` | exact `session` triple | `{}` | Structural lookup of an `open`, `disconnected`, or `closed` stored session. |
| `session.close` | exact `session` triple | `{}` | CAS transition from `open` or `disconnected` to terminal `closed`; a repeated close is `conflict`. |
| `session.disconnect` | exact `session` triple | `observed_by` | CAS transition from `open` to nonterminal `disconnected` only when an adapter supplies an attributable observation. Raw EOF is not an operation input and causes no persistence. |
| `session.resume` | `workstream` | `session_id`, `source_session_id`, `origin`, optional `client`, optional `model` | A new `open` session bound to the selected workstream only when its explicit source session is in that same project/workstream. |

`session.status` uses structural validation so closed and disconnected sessions remain inspectable. `ValidateActiveBinding` is separate and rejects those states for later session-attributed mutations. There is no reopen operation and no implicit latest-session fallback.

## Persistence and migration

Schema v3 is additive from valid v2. It adds workstream/source/provenance metadata, session lifecycle/provenance/resume-source metadata, and a workstream-list view generation. Existing Phase 0 rows retain their IDs and relationships. Legacy workstreams preserve their bindings with provenance `unknown` and NULL historical timestamps; they have no lifecycle status. Legacy sessions additionally read as `open`. No legacy origin or time is invented.

Workstream and session IDs plus their project/workstream bindings remain immutable. Existing composite foreign keys continue to enforce project ownership on every opened connection. SQLite triggers additionally reject cross-project fork and resume sources where an additive foreign key is impractical. Every lifecycle transition validates its exact structural triple, checks affected rows, and uses a transaction/CAS predicate to report `conflict` rather than lose an update.

Workstream lists are bounded SQL keyset reads ordered by immutable `workstream_id`. The existing alpha token pattern binds version, operation, complete scope, filters, order, limit, cursor, and a SQLite view generation. An empty list for an existing project is `ok`; a missing project is `not_found`.

## P1.3a boundary and forecast

Forecast before behavioral edits: approximately 1,180 product/test/evidence lines across the allowed shared-service, SQLite, testkit, and evidence surfaces. This is below the user-authorized 1,500-line P1.3a ceiling and the 2,000-line review block. The P1.3 parent checkbox remains unchecked because adapter work and real protocol tests are P1.3b. Documents, checkpoints, FTS retrieval, context restoration, and adapters are not part of this cut.

## Strict TDD evidence

| Cycle | RED command and observed result | GREEN and triangulation |
| --- | --- | --- |
| Caller IDs and provenance | `go test ./internal/app -run '^TestContinuityServiceCreatesWorkstreamWithCallerIDsAndProvenance$' -count=1 -timeout=120s` failed behaviorally: `outcome = "internal", want "ok"`. | The exact test passed after the v3 migration and concrete service/store create path; list/fork coverage proves caller IDs remain stable metadata. |
| Bounded workstream metadata and fork | `go test ./internal/app -run '^TestContinuityServiceListsBoundedMetadataAndForksWithoutMutatingSource$' -count=1 -timeout=120s` failed behaviorally: `outcome = "invalid", want one metadata result and continuation`. | The same test passed after SQL keyset listing, bound continuation issuance, and source-preserving fork. A later test covers malformed/rebound/stale tokens, empty existing-project lists, missing projects, and shared-service scope interleaving. |
| Session lifecycle | `go test ./internal/app -run '^TestContinuityServiceKeepsDisconnectCloseAndResumeDistinct$' -count=1 -timeout=120s` failed behaviorally: `open outcome = "invalid", want ok`. | Open, structural status, attributable disconnect, CAS close, no reopen, explicit-source resume, missing/mismatched triples, and active-binding rejection pass through the concrete store. |
| Migration version assertion maintenance (not behavioral RED) | After v3 was implemented, `TestOpenRecordsFoundationMigrationVersion` reported `migration version = 3, want 2`; updating this expected version is test maintenance, not evidence of a new production-behavior RED. | The updated assertion and valid-v2 preservation fixture pass: legacy sessions are `open`, provenance is `unknown`, and historical timestamps remain NULL. Second-connection tests cover ownership rejection and lifecycle CAS. |
| Fork update ownership | `go test ./internal/store/sqlite -run '^TestContinuityStoreUsesCASLifecycleAndDatabaseOwnershipConstraints$' -count=1 -timeout=120s` failed behaviorally: a direct cross-project fork-source update returned nil. | The additive v3 trigger rejects cross-project direct inserts and updates while permitting no arbitrary cross-project source relationship. |

Before the first behavioral RED, an attempted production stub reused the existing `Store.CreateWorkstream` method name and the first focused command failed to compile with a duplicate-method error. The stub was renamed to `CreateContinuityWorkstream` before rerunning; that compilation failure is not claimed as behavioral RED.

## Commands and limits

| Command | Result |
| --- | --- |
| `go test ./internal/app ./internal/store/sqlite -count=1 -timeout=120s` before P1.3a behavior | PASS safety net |
| behavioral RED commands above, excluding version-assertion maintenance | Expected behavioral failures, followed by PASS GREEN |
| `go test ./internal/app ./internal/store/sqlite -count=1 -timeout=120s` | PASS triangulation/refactor |
| `go test ./... -count=1 -timeout=120s` | PASS final suite |
| `git diff --check` | PASS |

Independent verification also passed `go test ./... -count=1 -timeout=120s` and `go test ./internal/app ./internal/store/sqlite -run '^(TestContinuity.*|TestOpenMigratesValidV2ContinuityRowsWithoutInventingProvenanceOrTime)$' -count=10 -timeout=90s`. This observed the current passing state, not a rerun of historical RED evidence.

Fixtures use `t.TempDir()` SQLite databases, and only short-lived tests ran. No CLI or MCP file changed; the interleaving test exercises declared shared-service scopes only and does not claim protocol parity, connection-disconnect handling, or MCP interleaving coverage. No user library, daemon, external call, dependency, global install, commit, or push occurred.
