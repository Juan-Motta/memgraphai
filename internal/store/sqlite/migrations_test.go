package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"memgraphai/internal/app"
	"memgraphai/internal/telemetry"
	"memgraphai/internal/testkit"
)

func TestOpenRecordsFoundationMigrationVersion(t *testing.T) {
	store, err := Open(t.Context(), testkit.TempSQLitePath(t, "migration-version"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	var version int
	err = store.db.QueryRowContext(t.Context(), "SELECT version FROM schema_migrations").Scan(&version)
	if err != nil {
		t.Fatalf("read schema migration version: %v", err)
	}
	if version != 1 {
		t.Fatalf("migration version = %d, want 1", version)
	}
}

func TestOpenAppliesMigrationOnceAndPersistsBoundedMetric(t *testing.T) {
	path := testkit.TempSQLitePath(t, "migration-repeat")
	store, err := Open(t.Context(), path)
	if err != nil {
		t.Fatalf("first Open() error = %v", err)
	}
	if err := store.CreateProject(t.Context(), "project-a", "A"); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	count := 3
	serialized, response := (app.Service{
		Recorder: store,
		Now:      func() time.Time { return time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC) },
	}).ExecuteJSON(t.Context(), []byte(`{"contract_version":"memgraphai.experimental/v1alpha1","operation_id":"op-metric","operation":"project.create","scope":{"kind":"project","project_id":"project-a"},"input":{}}`), "cli", func(context.Context, app.Request) app.Result {
		return app.Result{Outcome: app.OK, Value: json.RawMessage(`{"project_id":"project-a"}`), ResultCount: &count}
	})
	if response.Outcome.Code != app.OK {
		t.Fatalf("app response outcome = %q, want %q", response.Outcome.Code, app.OK)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("first Close() error = %v", err)
	}

	store, err = Open(t.Context(), path)
	if err != nil {
		t.Fatalf("second Open() error = %v", err)
	}
	defer store.Close()
	if got, err := store.ProjectName(t.Context(), "project-a"); err != nil || got != "A" {
		t.Fatalf("ProjectName() = %q, %v; want preserved project", got, err)
	}
	var migrations, bytes, resultCount int
	if err := store.db.QueryRowContext(t.Context(), `SELECT
		(SELECT count(*) FROM schema_migrations), response_bytes, result_count
		FROM operation_metrics WHERE operation_id = 'op-metric'`).Scan(&migrations, &bytes, &resultCount); err != nil {
		t.Fatalf("read persisted migration and metric: %v", err)
	}
	if migrations != 1 || bytes != len(serialized) || resultCount != 3 {
		t.Fatalf("migration/metric = %d/%d/%d, want 1/%d/3", migrations, bytes, resultCount, len(serialized))
	}
	if err := store.Record(t.Context(), telemetry.Operation{
		OperationID: "op-no-count", RecordedAt: time.Now(), Interface: "mcp", ScopeKind: "library",
		Outcome: "ok", ResponseBytes: 20,
	}); err != nil {
		t.Fatalf("Record(no count) error = %v", err)
	}
	var absentCount sql.NullInt64
	if err := store.db.QueryRowContext(t.Context(), "SELECT result_count FROM operation_metrics WHERE operation_id = 'op-no-count'").Scan(&absentCount); err != nil {
		t.Fatalf("read absent result count: %v", err)
	}
	if absentCount.Valid {
		t.Fatalf("unavailable result count = %d, want NULL", absentCount.Int64)
	}
}

func TestOpenUpgradesLegacyOperationsSchema(t *testing.T) {
	path := testkit.TempSQLitePath(t, "legacy-operations")
	db, err := sql.Open(probeDriver, foreignKeyDSN(path))
	if err != nil {
		t.Fatalf("open legacy fixture: %v", err)
	}
	if _, err := db.ExecContext(t.Context(), `CREATE TABLE operations (
		operation_id TEXT PRIMARY KEY NOT NULL,
		fingerprint TEXT NOT NULL,
		document_id TEXT NOT NULL,
		revision_id TEXT NOT NULL,
		expected_current_revision_id TEXT,
		owner_generation INTEGER NOT NULL DEFAULT 0,
		state TEXT NOT NULL,
		result_outcome TEXT,
		result_revision_id TEXT
	)`); err != nil {
		t.Fatalf("create legacy operations table: %v", err)
	}
	if _, err := db.ExecContext(t.Context(), `INSERT INTO operations (
		operation_id, fingerprint, document_id, revision_id, expected_current_revision_id,
		owner_generation, state, result_outcome, result_revision_id
	) VALUES ('op-old', 'fingerprint-old', 'document-old', 'revision-old',
		'expected-old', 7, 'committed', 'ok', 'result-old')`); err != nil {
		t.Fatalf("insert legacy operation: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close legacy fixture: %v", err)
	}

	for attempt := 1; attempt <= 2; attempt++ {
		store, err := Open(t.Context(), path)
		if err != nil {
			t.Fatalf("Open() attempt %d error = %v", attempt, err)
		}
		var operationID, fingerprint, documentID, revisionID, expected, state, outcome, result string
		var generation int
		var requestFingerprint, projectID sql.NullString
		err = store.db.QueryRowContext(t.Context(), `SELECT operation_id, fingerprint,
			request_fingerprint, project_id, document_id, revision_id,
			expected_current_revision_id, owner_generation, state, result_outcome,
			result_revision_id FROM operations WHERE operation_id = 'op-old'`).Scan(
			&operationID, &fingerprint, &requestFingerprint, &projectID, &documentID, &revisionID,
			&expected, &generation, &state, &outcome, &result,
		)
		if err != nil {
			store.Close()
			t.Fatalf("read upgraded operation on attempt %d: %v", attempt, err)
		}
		if operationID != "op-old" || fingerprint != "fingerprint-old" || documentID != "document-old" ||
			revisionID != "revision-old" || expected != "expected-old" || generation != 7 ||
			state != "committed" || outcome != "ok" || result != "result-old" {
			store.Close()
			t.Fatalf("legacy operation changed on attempt %d", attempt)
		}
		if requestFingerprint.Valid || projectID.Valid {
			store.Close()
			t.Fatalf("compatibility columns on attempt %d = %v/%v, want NULL defaults", attempt, requestFingerprint, projectID)
		}
		if err := store.Close(); err != nil {
			t.Fatalf("Close() attempt %d error = %v", attempt, err)
		}
	}
}

func TestFailedMigrationRollsBackCompatibilityColumns(t *testing.T) {
	path := testkit.TempSQLitePath(t, "migration-rollback")
	db, err := sql.Open(probeDriver, foreignKeyDSN(path))
	if err != nil {
		t.Fatalf("open rollback fixture: %v", err)
	}
	if _, err := db.ExecContext(t.Context(), "CREATE TABLE operations (operation_id TEXT PRIMARY KEY NOT NULL)"); err != nil {
		t.Fatalf("create legacy operations table: %v", err)
	}
	if _, err := db.ExecContext(t.Context(), `CREATE TABLE schema_migrations (
		version INTEGER PRIMARY KEY NOT NULL CHECK (version != 1)
	)`); err != nil {
		t.Fatalf("create rejecting migration ledger: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close rollback fixture: %v", err)
	}

	if _, err := Open(t.Context(), path); err == nil {
		t.Fatal("Open() error = nil, want rejected migration record")
	}
	verify, err := sql.Open(probeDriver, foreignKeyDSN(path))
	if err != nil {
		t.Fatalf("reopen rollback fixture: %v", err)
	}
	defer verify.Close()
	var compatibilityColumns, versions int
	if err := verify.QueryRowContext(t.Context(), `SELECT count(*) FROM pragma_table_info('operations')
		WHERE name IN ('request_fingerprint', 'project_id')`).Scan(&compatibilityColumns); err != nil {
		t.Fatalf("inspect rolled back operation columns: %v", err)
	}
	if err := verify.QueryRowContext(t.Context(), "SELECT count(*) FROM schema_migrations").Scan(&versions); err != nil {
		t.Fatalf("inspect rolled back migration ledger: %v", err)
	}
	if compatibilityColumns != 0 || versions != 0 {
		t.Fatalf("rolled back columns/versions = %d/%d, want 0/0", compatibilityColumns, versions)
	}
}

func TestOpenRejectsFutureSchemaWithoutChangingIt(t *testing.T) {
	path := testkit.TempSQLitePath(t, "future-migration")
	db, err := sql.Open(probeDriver, foreignKeyDSN(path))
	if err != nil {
		t.Fatalf("open future fixture: %v", err)
	}
	if _, err := db.ExecContext(t.Context(), "CREATE TABLE schema_migrations (version INTEGER NOT NULL)"); err != nil {
		t.Fatalf("create migration table: %v", err)
	}
	if _, err := db.ExecContext(t.Context(), "INSERT INTO schema_migrations(version) VALUES (99)"); err != nil {
		t.Fatalf("insert future migration: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close future fixture: %v", err)
	}

	if _, err := Open(t.Context(), path); err == nil {
		t.Fatal("Open(future schema) error = nil, want rejection")
	}

	verify, err := sql.Open(probeDriver, foreignKeyDSN(path))
	if err != nil {
		t.Fatalf("reopen future fixture: %v", err)
	}
	defer verify.Close()
	var version int
	if err := verify.QueryRowContext(t.Context(), "SELECT version FROM schema_migrations").Scan(&version); err != nil {
		t.Fatalf("read preserved future version: %v", err)
	}
	if version != 99 {
		t.Fatalf("future schema version changed to %d, want 99", version)
	}
}
