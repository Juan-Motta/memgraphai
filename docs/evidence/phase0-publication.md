# Phase 0 publication evidence

## Scope and observed result

P0.3 exercised a narrow local Markdown preparation boundary and a short SQLite metadata transaction. `revisionfs.Prepare` creates the ID-only library/staging/revision directories, syncs each newly created directory's parent, writes and syncs the staged Markdown file, hard-links it into the final revision path without replacement, and syncs the final revision directory. SQLite then inserts immutable revision metadata and advances the document current pointer in one transaction.

The implementation accepts only nonempty ASCII letter, digit, underscore, and hyphen path components. Final paths are therefore rooted beneath the test library and do not use display names or supplied paths.

## TDD evidence

| Cycle | RED command and observed failure | GREEN | TRIANGULATE and refactor |
| --- | --- | --- | --- |
| Immutable Markdown preparation | `go test ./internal/revisionfs -run '^TestPreparePublishesImmutableMarkdown$'` failed with `Prepare() error = revision preparation is not implemented`. | The same focused test passed after staged write/file sync, durable directory creation, no-clobber link publication, and SHA-256/byte metadata were implemented. | `go test ./internal/revisionfs -run '^TestPrepare'` passed for a missing library root, exact content/checksum, duplicate final path preserving the original content, unsafe revision ID rejection, and an injected pre-publication interruption leaving no final path. `safeComponent`, `ensureDir`, and `syncDir` keep the portability boundary narrow. |
| Per-connection SQLite foreign keys | `go test ./internal/store/sqlite -run '^TestStoreRejectsForeignKeyViolationsOnEveryUsableConnection$'` failed with `direct SQL foreign-key violation error = nil, want constraint rejection`. | The focused test passed after `Open` applied modernc's `_pragma=foreign_keys(1)` DSN setting. | The test holds one pooled connection while acquiring a second and executes a direct invalid `project_paths` insert on that second connection; it is rejected by SQLite rather than by a Go precheck. |
| SQLite pointer publication | `go test ./internal/store/sqlite -run '^TestPublishRevisionMakesFirstPreparedRevisionCurrent$'` failed with `CreateDocument() error = document publication is not implemented`. | The focused first-publication test passed after the document/revision metadata transaction was added. | `go test ./internal/store/sqlite -run '^TestPublishRevision'` passed for first publication, valid replacement, stale expected-current conflict, a pre-commit interruption retaining the previous pointer while prepared Markdown remains invisible, and an injected post-commit response loss returning `unknown` while the committed pointer remains visible. |

## Verification

| Command | Result |
| --- | --- |
| `go test ./internal/domain ./internal/store/sqlite ./internal/testkit` | PASS safety net before P0.3 edits |
| `go test ./internal/revisionfs -run '^TestPreparePublishesImmutableMarkdown$'` | expected RED, then PASS GREEN |
| `go test ./internal/store/sqlite -run '^TestStoreRejectsForeignKeyViolationsOnEveryUsableConnection$'` | expected RED, then PASS GREEN |
| `go test ./internal/store/sqlite -run '^TestPublishRevisionMakesFirstPreparedRevisionCurrent$'` | expected RED, then PASS GREEN |
| `go test ./internal/revisionfs -run '^TestPrepare'` | PASS triangulation/refactor |
| `go test ./internal/store/sqlite -run '^(TestPublishRevision|TestStoreRejectsForeignKeyViolationsOnEveryUsableConnection$)'` | PASS triangulation/refactor |
| `go test ./...` | PASS final suite |

## Boundary and limitations

A prepared immutable Markdown file is not visible as a document revision until SQLite commits its pointer. A stale expected revision returns the stable `conflict` outcome and does not advance the pointer. A simulated response loss after a successful commit returns stable `unknown`; this P0.3 experiment does not implement operation-ledger replay, ownership fencing, restart recovery, checksum verification on reads, orphan classification, or concurrent retry behavior. Those are P0.4 responsibilities.

The fault injections return errors at named in-process boundaries. They demonstrate this implementation's error paths and pointer preservation; they do **not** prove power-loss durability, filesystem crash consistency, or a supported filesystem/platform guarantee.

Tests used `t.TempDir()` libraries and SQLite databases only. Go test processes were short-lived, no user library or persistent process was opened, and test fixtures are cleaned by the Go test framework. No dependency installation, global installation, commit, paid call, or Phase 1 source was performed.
