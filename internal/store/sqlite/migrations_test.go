package sqlite

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"

	"memgraphai/internal/app"
	"memgraphai/internal/domain"
	"memgraphai/internal/revisionfs"
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
	err = store.db.QueryRowContext(t.Context(), "SELECT MAX(version) FROM schema_migrations").Scan(&version)
	if err != nil {
		t.Fatalf("read schema migration version: %v", err)
	}
	if version != 4 {
		t.Fatalf("migration version = %d, want 4", version)
	}
}

func TestOpenMigratesValidV2ContinuityRowsWithoutInventingProvenanceOrTime(t *testing.T) {
	path := testkit.TempSQLitePath(t, "valid-v2-continuity")
	db, err := sql.Open(probeDriver, foreignKeyDSN(path))
	if err != nil {
		t.Fatalf("open v2 fixture: %v", err)
	}
	for _, statement := range foundationSchema {
		if _, err := db.ExecContext(t.Context(), statement); err != nil {
			t.Fatalf("apply valid v1 statement: %v", err)
		}
	}
	for _, statement := range projectSchema {
		if _, err := db.ExecContext(t.Context(), statement); err != nil {
			t.Fatalf("apply valid v2 statement: %v", err)
		}
	}
	if _, err := db.ExecContext(t.Context(), `CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY NOT NULL);
		INSERT INTO schema_migrations(version) VALUES (1), (2);
		INSERT INTO projects(project_id, display_name) VALUES ('project-a', 'Alpha');
		INSERT INTO workstreams(workstream_id, project_id) VALUES ('stream-legacy', 'project-a');
		INSERT INTO sessions(session_id, project_id, workstream_id) VALUES ('session-legacy', 'project-a', 'stream-legacy');`); err != nil {
		t.Fatalf("seed valid v2 fixture: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close v2 fixture: %v", err)
	}

	store, err := Open(t.Context(), path)
	if err != nil {
		t.Fatalf("Open(v2) error = %v", err)
	}
	defer store.Close()
	var workstreamOrigin string
	var workstreamCreated sql.NullString
	if err := store.db.QueryRowContext(t.Context(), `SELECT origin, created_at FROM workstreams
		WHERE workstream_id = 'stream-legacy'`).Scan(&workstreamOrigin, &workstreamCreated); err != nil {
		t.Fatalf("read migrated workstream: %v", err)
	}
	var status, sessionOrigin string
	var openedAt sql.NullString
	if err := store.db.QueryRowContext(t.Context(), `SELECT status, origin, opened_at FROM sessions
		WHERE session_id = 'session-legacy'`).Scan(&status, &sessionOrigin, &openedAt); err != nil {
		t.Fatalf("read migrated session: %v", err)
	}
	if workstreamOrigin != "unknown" || workstreamCreated.Valid || status != "open" || sessionOrigin != "unknown" || openedAt.Valid {
		t.Fatalf("migrated legacy metadata = %q/%v/%q/%q/%v, want unknown provenance, open state, and NULL times", workstreamOrigin, workstreamCreated.Valid, status, sessionOrigin, openedAt.Valid)
	}
	var version int
	if err := store.db.QueryRowContext(t.Context(), "SELECT MAX(version) FROM schema_migrations").Scan(&version); err != nil || version != 4 {
		t.Fatalf("continuity migration version = %d, %v; want 4, nil", version, err)
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
	if migrations != 4 || bytes != len(serialized) || resultCount != 3 {
		t.Fatalf("migration/metric = %d/%d/%d, want 4/%d/3", migrations, bytes, resultCount, len(serialized))
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

func TestOpenMigratesValidV3DocumentRowsWithoutInventingRevisionProvenance(t *testing.T) {
	path := testkit.TempSQLitePath(t, "valid-v3-documents")
	db, err := sql.Open(probeDriver, foreignKeyDSN(path))
	if err != nil {
		t.Fatalf("open v3 fixture: %v", err)
	}
	for _, schema := range [][]string{foundationSchema, projectSchema, continuitySchema} {
		for _, statement := range schema {
			if _, err := db.ExecContext(t.Context(), statement); err != nil {
				t.Fatalf("apply v3 statement: %v", err)
			}
		}
	}
	if _, err := db.ExecContext(t.Context(), `CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY NOT NULL);
		INSERT INTO schema_migrations(version) VALUES (1), (2), (3);
		INSERT INTO projects(project_id, display_name) VALUES ('project-a', 'A');
		INSERT INTO documents(document_id, project_id, current_revision_id) VALUES
			('document-a', 'project-a', 'revision-a2'), ('document-b', 'project-a', 'revision-b');
		INSERT INTO revisions(revision_id, document_id, predecessor_revision_id, file_path, checksum, byte_count) VALUES
			('revision-a', 'document-a', NULL, '/legacy-a.md', 'checksum-a', 6),
			('revision-a2', 'document-a', 'revision-a', '/legacy-a2.md', 'checksum-a2', 7),
			('revision-b', 'document-b', NULL, '/legacy-b.md', 'checksum-b', 6);
		INSERT INTO operations(operation_id, fingerprint, request_fingerprint, project_id, document_id,
			revision_id, expected_current_revision_id, owner_generation, state, result_outcome, result_revision_id) VALUES
			('op-committed', '81d530558c8fd18cf52e18ca189c5937d1db14635adb06639f5e5a88facab57c', 'document-write', 'project-a', 'document-a', 'revision-a', NULL, 7, 'committed', 'committed', 'revision-a'),
			('op-conflict', '81d530558c8fd18cf52e18ca189c5937d1db14635adb06639f5e5a88facab57c', 'document-write', 'project-a', 'document-a', 'revision-a', NULL, 8, 'conflict', 'conflict', NULL);`); err != nil {
		t.Fatalf("seed valid v3 document fixture: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close v3 fixture: %v", err)
	}

	store, err := Open(t.Context(), path)
	if err != nil {
		t.Fatalf("Open(v3) error = %v", err)
	}
	defer store.Close()
	var workstream, origin, client, model, created sql.NullString
	if err := store.db.QueryRowContext(t.Context(), `SELECT d.workstream_id, r.origin, r.client_provenance,
		r.model_provenance, r.created_at FROM documents d JOIN revisions r ON r.document_id = d.document_id
		WHERE d.document_id = 'document-a'`).Scan(&workstream, &origin, &client, &model, &created); err != nil {
		t.Fatalf("read migrated v3 document: %v", err)
	}
	if workstream.Valid || origin.String != "unknown" || client.Valid || model.Valid || created.Valid {
		t.Fatalf("migrated document metadata = %v/%q/%v/%v/%v, want NULL scope/client/model/time and unknown origin", workstream.Valid, origin.String, client.Valid, model.Valid, created.Valid)
	}

	service := app.DocumentService{Store: store, Files: revisionfs.New(t.TempDir(), nil)}
	limit := 1
	list := service.Handle(t.Context(), app.Request{Operation: "document.list", Scope: app.Scope{Kind: "project", ProjectID: "project-a"}, Input: []byte(`{}`), Page: &app.Page{Limit: &limit}})
	history := service.Handle(t.Context(), app.Request{Operation: "document.history", Scope: app.Scope{Kind: "project", ProjectID: "project-a"}, Input: []byte(`{"document_id":"document-a"}`), Page: &app.Page{Limit: &limit}})
	if list.Outcome != app.OK || list.Page == nil || history.Outcome != app.OK || history.Page == nil {
		t.Fatalf("migrated list/history = %#v / %#v, want continuations", list, history)
	}
	var token struct {
		ViewRevision int64 `json:"view_revision"`
	}
	encoded, err := base64.RawURLEncoding.DecodeString(list.Page.NextToken)
	if err != nil || json.Unmarshal(encoded, &token) != nil || token.ViewRevision != 0 {
		t.Fatalf("migrated list token = %q / %#v / %v, want generation 0", list.Page.NextToken, token, err)
	}
	if err := store.CreateDocument(t.Context(), "document-c", "project-a"); err != nil {
		t.Fatalf("CreateDocument(mutate migrated view) error = %v", err)
	}
	if replay := service.Handle(t.Context(), app.Request{Operation: "document.list", Scope: app.Scope{Kind: "project", ProjectID: "project-a"}, Input: []byte(`{}`), Page: &app.Page{Limit: &limit, Token: list.Page.NextToken}}); replay.Outcome != app.Conflict {
		t.Fatalf("migrated list replay outcome = %q, want %q", replay.Outcome, app.Conflict)
	}
	if replay := service.Handle(t.Context(), app.Request{Operation: "document.history", Scope: app.Scope{Kind: "project", ProjectID: "project-a"}, Input: []byte(`{"document_id":"document-a"}`), Page: &app.Page{Limit: &limit, Token: history.Page.NextToken}}); replay.Outcome != app.Conflict {
		t.Fatalf("migrated history replay outcome = %q, want %q", replay.Outcome, app.Conflict)
	}

	request := OperationRequest{ID: "op-committed", Fingerprint: "document-write", ProjectID: "project-a", DocumentID: "document-a", RevisionID: "revision-a", Markdown: []byte("# baseline\n")}
	allow := func(context.Context) error { return nil }
	for id, want := range map[string]string{"op-committed": "committed", "op-conflict": "conflict"} {
		request.ID = id
		result, err := store.ExecuteOperation(t.Context(), nil, request, allow)
		if err != nil || string(result.Outcome) != want {
			t.Fatalf("ExecuteOperation(%s legacy replay) = %#v, %v; want %s", id, result, err, want)
		}
	}
	request.ID, request.Origin = "op-committed", "cli"
	if _, err := store.ExecuteOperation(t.Context(), nil, request, allow); !domain.IsOutcome(err, domain.IdempotencyMismatch) {
		t.Fatalf("ExecuteOperation(changed legacy provenance) error = %v, want %q", err, domain.IdempotencyMismatch)
	}
}
