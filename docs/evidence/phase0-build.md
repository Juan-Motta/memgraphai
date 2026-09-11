# Phase 0.1 build and SQLite evidence

**Status:** viable on the tested machine only. This record is evidence for P0.1, not a macOS support, signing, packaging, or release claim.

## Scope and runner setup

The root module is named `memgraphai`. Its executable is `cmd/memgraphai`; the historical `cmd/memgraph` spelling was not created.

| Item | Observed result |
| --- | --- |
| Go toolchain | `go version go1.27.0 darwin/arm64` |
| Host | macOS 26.6.2 (build 25G83), `arm64` |
| C environment observed during comparison | `CGO_ENABLED=1`, `CC=clang`, Apple clang 21.0.0; `/Library/Developer/CommandLineTools` |
| Root runner before behavioral RED | `go test ./...` exited 0 with `memgraphai/cmd/memgraphai [no test files]` after the root module and non-behavioral package marker existed. |
| Final configured runner | `go test ./...` exited 0 for `cmd/memgraphai`, `internal/store/sqlite`, and `internal/testkit`. |
| Dependency lock | `go.mod` and `go.sum` exist; the selected direct module is `modernc.org/sqlite v1.58.0`. Downloads were normal project-local Go module resolution only; no global tool was installed. |

## Candidate comparison

Both candidates used a disposable module and `:memory:` database. Each executable performed all of the following: `SELECT sqlite_compileoption_used('ENABLE_FTS5')`, `CREATE VIRTUAL TABLE ... USING fts5`, insert one row, `MATCH` query for `project`, and a transaction that created/read back `committed`.

| Candidate | Build condition | Compile option | Real FTS5 query | Transaction | Executable and observed links |
| --- | --- | --- | --- | --- | --- |
| `github.com/mattn/go-sqlite3 v1.14.52` | `go build -tags sqlite_fts5 -o probe .` | `1` | `fts5_matches=1` | `committed` | Mach-O 64-bit `arm64`; `/usr/lib/libSystem.B.dylib`, `/usr/lib/libresolv.9.dylib` |
| `modernc.org/sqlite v1.58.0` | `go build -o probe .` | `1` | `fts5_matches=1` | `committed` | Mach-O 64-bit `arm64`; `/usr/lib/libSystem.B.dylib`, `/usr/lib/libresolv.9.dylib` |

The first candidate command initially stopped because its disposable module lacked a `go.sum` entry; `go mod tidy` created the temporary lock and the rerun succeeded. This was dependency setup, not a behavioral RED.

`modernc.org/sqlite` was selected from these observations because the root runner and executable evidence work without the `sqlite_fts5` build tag or a C compiler requirement. The selected root path also passed with `CGO_ENABLED=0`. This is a local selection for P0.1, not a portability promise.

## Small reusable probe

`internal/store/sqlite.Probe` deliberately contains no product schema or migration. It opens an isolated DSN, corroborates FTS5 with the compile-option query, proves it by creating/querying a real FTS5 virtual table, and verifies a committed transaction. `cmd/memgraphai` exposes only `memgraphai probe`; it uses an in-memory database and reports the probe outcome. Invalid command input returns `usage: memgraphai probe` with exit status 2.

Tests use `t.TempDir()` database paths through `internal/testkit.TempSQLitePath`; no user library or real database path was opened.

## Strict TDD cycle evidence

| Work item | RED command and result | GREEN command and result | TRIANGULATE command and result | REFACTOR |
| --- | --- | --- | --- | --- |
| Isolated test database path | `go test ./internal/testkit -run '^TestTempSQLitePathBuildsAPathInsideTheTestDirectory$'` failed: `undefined: TempSQLitePath`. | After the smallest helper, the focused test passed. The first green assertion incorrectly compared against a second `t.TempDir()` allocation; the assertion was corrected to verify the returned existing temp directory, then passed. | Added `TestTempSQLitePathSeparatesNamedArtifacts`; `go test ./internal/testkit -run '^TestTempSQLitePath'` passed. | Helper remained one function; `gofmt` and focused package suite passed. |
| SQLite/FTS5 and transaction | `go test ./internal/store/sqlite -run '^TestProbeReportsFTS5AndCommittedTransaction$'` failed: `undefined: Probe`. | After the smallest `Probe` implementation, that focused test passed. | Added absent-term FTS query case; `go test ./internal/store/sqlite -run '^TestProbe'` passed with one match for `project` and zero for `unavailable`. | No abstraction added; `gofmt` and package suite passed. |
| Executable probe shape | `go test ./cmd/memgraphai -run '^TestProbeCommandReportsSQLiteEvidence$'` failed because `main` was undeclared. | After the `probe` command, the focused test passed. | Added invalid-argument coverage; it initially exposed `go run` appending `exit status 2` to captured output, so the test isolated the application output line without changing correct production behavior; the focused two-test run passed. | No source refactor was needed; `gofmt` and package suite passed. |

## Follow-up transaction rollback correction

### Original evidence limitation

The original P0.1 RED entries above remain historical evidence and are not relabeled: `undefined: Probe` and `main undeclared` were compilation failures. They established missing API/executable setup, not executable behavioral RED evidence for transaction rollback. This follow-up does not claim to make the original RED behavioral or retroactively change its TDD sequence.

### Newly executed behavioral cycle

A minimal compilable `ProbeResult.RolledBackRows` stub first returned `-1` solely to permit an executable assertion. The new test then exercised the real SQLite probe and failed on the actual returned value, rather than on an undefined symbol:

| Step | Command and observed result |
| --- | --- |
| Safety net | `go test ./internal/store/sqlite ./cmd/memgraphai` passed before the follow-up changes. |
| RED | `go test ./internal/store/sqlite -run '^TestProbeRollbackLeavesNoRowVisible$'` failed with `Probe() rows visible after rollback = -1, want 0`. |
| GREEN | `gofmt -w internal/store/sqlite/probe.go internal/store/sqlite/probe_test.go && go test ./internal/store/sqlite -run '^TestProbeRollbackLeavesNoRowVisible$'` passed after `Probe` began, inserted `rolled back`, rolled back, then counted that exact value. |
| TRIANGULATE | `go test ./internal/store/sqlite -run '^TestProbe(ReportsFTS5AndCommittedTransaction|RollbackLeavesNoRowVisible|UsesTheRequestedFTS5Query)$'` passed. It asserts the committed value is exactly `committed`, while the distinct rolled-back value has exactly zero visible rows, plus the existing present/absent FTS5 query cases. |
| REFACTOR | No production refactor was needed after the minimal transaction/check sequence; `gofmt` and the focused suite remained green. |

The exact new rollback assertion is `result.RolledBackRows == 0`; its explicit setup inserts the distinct value `rolled back` inside a transaction that calls `Rollback`. The contrasting committed assertion remains `result.TransactionValue == "committed"`. These assertions prove that the committed value remains visible and the rolled-back value does not; they do not claim crash recovery, publication, schema, identity, or later-phase behavior.

## Final commands and results

```text
go test ./internal/testkit ./internal/store/sqlite ./cmd/memgraphai
# PASS: all three packages

go test ./...
# PASS: cmd/memgraphai, internal/store/sqlite, internal/testkit

go build -o /tmp/memgraphai-p0-1-build/memgraphai ./cmd/memgraphai
/tmp/memgraphai-p0-1-build/memgraphai probe
# memgraphai: SQLite FTS5 probe succeeded (matches=1, transaction=committed)

CGO_ENABLED=0 go test ./...
# PASS: cmd/memgraphai, internal/store/sqlite, internal/testkit
CGO_ENABLED=0 go build -o /tmp/memgraphai-p0-1-cgo-disabled/memgraphai ./cmd/memgraphai
/tmp/memgraphai-p0-1-cgo-disabled/memgraphai probe
# memgraphai: SQLite FTS5 probe succeeded (matches=1, transaction=committed)
```

`file` reported both selected-driver binaries as Mach-O 64-bit `arm64`; `otool -L` listed only `/usr/lib/libSystem.B.dylib` and `/usr/lib/libresolv.9.dylib` in this environment. Temporary candidate modules and both `/tmp/memgraphai-p0-1-*` build directories were removed after observation; cleanup verified that none remained.

### Follow-up full-suite and process verification

```text
go test ./...
# PASS: cmd/memgraphai, internal/store/sqlite, internal/testkit

go build -o /tmp/memgraphai-p0-1-rollback/memgraphai ./cmd/memgraphai
/tmp/memgraphai-p0-1-rollback/memgraphai probe
# memgraphai: SQLite FTS5 probe succeeded (matches=1, transaction=committed)

rm -rf /tmp/memgraphai-p0-1-rollback
# cleanup verified: /tmp/memgraphai-p0-1-rollback absent
```

The follow-up used only the project-local Go toolchain, a temporary executable path, and test-created temporary databases; it did not install global tools, access a user library, create a commit, or start a persistent process.

## Limitations and next boundary

Only the macOS host, architecture, toolchain, links, and commands shown above were tested. No other macOS architecture, operating-system version, signing path, release package, filesystem durability claim, schema, FTS retrieval feature, user library, service adapter, or Phase 1 behavior is evidenced here. P0.2 remains blocked on this completed P0.1 work unit and must establish identity constraints separately.
