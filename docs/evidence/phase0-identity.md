# Phase 0 identity and binding evidence

## Scope

P0.2 exercised only isolated SQLite databases and temporary filesystem paths. It establishes a narrow, concrete identity experiment; it adds no adapter, document publication, recovery, lease, or Phase 1 behavior.

## Observed behavior

- `projects.project_id` is a SQLite primary key protected by a `BEFORE UPDATE` trigger. `RenameProjectID` reaches that trigger and returns the stable `immutable` outcome; separate projects may retain the same case-preserved display name.
- A path association retains the supplied absolute spelling and resolved target. Existing symlinks resolve to their target, so an alias can resolve to one project. Two distinct projects associated with the same resolved target return `many`, not a selection.
- Resolution is exact by resolved path. An existing nested child does not select its associated parent by deepest-prefix inference. A moved or otherwise unavailable supplied association returns `unavailable`; the moved destination has no association and returns `none`.
- Workstreams are project-owned, and sessions require the same stored project/workstream pair. Cross-project session creation and supplied binding mismatches return `binding_mismatch`.
- Association is discovery data only. `domain.AuthorizeProject` requires an explicit project grant and returns `scope_denied` when a project is merely associated.

The authorization function is deliberately a minimal explicit input for this Phase 0 experiment, not a permission framework. The local single-user process is not a sandbox or security boundary; actor descriptions and path association do not grant permission.

## Strict TDD evidence

| Cycle | RED command and actual assertion | GREEN | TRIANGULATE | REFACTOR |
| --- | --- | --- | --- | --- |
| Association is not authorization | `go test ./internal/domain -run '^TestAuthorizeProjectDoesNotTreatAssociationAsPermission$'` failed: `AuthorizeProject() error = <nil>, want "scope_denied"` | The same command passed after the smallest explicit-grant check. | The same test's distinct explicit-grant assertion passed after the denial assertion, proving a granted project remains allowed. | Typed `domain.Error` and `IsOutcome` keep stable results independent of store messages. |
| SQLite identity, associations, and bindings | `go test ./internal/store/sqlite -run '^(TestStorePreservesProjectIDsAndCase|TestResolvePathReportsOneManyNoneAndUnavailableWithoutGuessing|TestStoreRejectsCrossProjectAndMismatchedBindings)$'` failed with three executable assertions: immutable rename returned `<nil>`; alias resolution returned `none`; cross-project session creation returned `<nil>`. | The same command passed after the SQLite schema, immutable-ID trigger, exact association resolution, and relationship checks were implemented. | An added real nested directory still returned `none` (no prefix guess); the focused suite also covered one alias, many aliases, moved-source `unavailable`, moved-destination `none`, same-named projects, invalid cross-project session, mismatched session binding, and valid binding. | The workstream ownership check was made private and the focused suite remained green. |

## Commands run

| Command | Result |
| --- | --- |
| `go test ./...` before P0.2 edits | PASS safety net |
| `go test ./internal/domain -run '^TestAuthorizeProjectDoesNotTreatAssociationAsPermission$'` | expected behavioral RED, then PASS GREEN |
| `go test ./internal/store/sqlite -run '^TestStorePreservesProjectIDsAndCase$'` | expected behavioral RED: immutable rename returned `<nil>` |
| `go test ./internal/store/sqlite -run '^(TestStorePreservesProjectIDsAndCase|TestResolvePathReportsOneManyNoneAndUnavailableWithoutGuessing|TestStoreRejectsCrossProjectAndMismatchedBindings)$'` | expected behavioral RED with identity, resolution, and binding assertions, then PASS GREEN |
| `go test ./internal/domain ./internal/store/sqlite -run '^(TestAuthorizeProjectDoesNotTreatAssociationAsPermission|TestStorePreservesProjectIDsAndCase|TestResolvePathReportsOneManyNoneAndUnavailableWithoutGuessing|TestStoreRejectsCrossProjectAndMismatchedBindings)$'` | PASS triangulation and refactor checks |
| `go test ./...` | PASS final suite |

`openspec/config.yaml` now records the runner as verified from P0.1 Go evidence. Temporary databases and paths use `t.TempDir()` and are cleaned by Go's test framework. No external library path, user data, persistent process, global installation, commit, or dependency download was used for P0.2.
