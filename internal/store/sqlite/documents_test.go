package sqlite

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"memgraphai/internal/app"
	"memgraphai/internal/domain"
	"memgraphai/internal/revisionfs"
	"memgraphai/internal/testkit"
)

func TestDocumentStorePublishesExactProjectGeneralRevision(t *testing.T) {
	store, files, _ := documentFixture(t)
	createdAt := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	publishDocument(t, store, files, "operation-a", "document-a", "revision-a", nil, "# exact\n", "terminal", createdAt)
	revision, content, err := store.ReadDocument(t.Context(), files, "project-a", "", "document-a", nil)
	if err != nil || string(content) != "# exact\n" || revision.Provenance.Model != "reported" || !revision.CreatedAt.Equal(createdAt) {
		t.Fatalf("ReadDocument(current) = %#v, %q, %v; want exact committed provenance", revision, content, err)
	}
}

func TestDocumentStoreReplaysLostResponseAndRejectsChangedProvenance(t *testing.T) {
	store, files, _ := documentFixture(t)
	created := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	write := documentCreateWrite("operation-a", "document-a", "revision-a", nil, "# exact\n", "terminal", created)
	if err := registerDocumentWrite(t, store, write); err != nil {
		t.Fatalf("RegisterDocumentCreate() error = %v", err)
	}
	store.afterOperationCommit = func() error { return errors.New("lost response") }
	if _, _, err := store.WriteDocument(t.Context(), files, "operation-a", "document-a", "project-a", "", "revision-a", nil, []byte("# exact\n"), "cli", "terminal", "reported", created); !domain.IsOutcome(err, domain.Unknown) {
		t.Fatalf("WriteDocument(lost response) error = %v, want unknown", err)
	}
	store.afterOperationCommit = nil
	if err := registerDocumentWrite(t, store, write); err != nil {
		t.Fatalf("RegisterDocumentCreate(replay) error = %v", err)
	}
	outcome, revisionID, err := store.WriteDocument(t.Context(), files, "operation-a", "document-a", "project-a", "", "revision-a", nil, []byte("# exact\n"), "cli", "terminal", "reported", created)
	if err != nil || outcome != app.OK || revisionID != "revision-a" {
		t.Fatalf("WriteDocument(replay) = %q/%q, %v; want durable result", outcome, revisionID, err)
	}
	if _, _, err := store.WriteDocument(t.Context(), files, "operation-a", "document-a", "project-a", "", "revision-a", nil, []byte("# exact\n"), "cli", "changed-client", "reported", created); !domain.IsOutcome(err, domain.IdempotencyMismatch) {
		t.Fatalf("WriteDocument(changed provenance) error = %v, want mismatch", err)
	}
}

func TestDocumentCreateRegistrationSurvivesRestartWithoutReservationHijack(t *testing.T) {
	ctx := t.Context()
	path := testkit.TempSQLitePath(t, "document-registration")
	created := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	write := documentCreateWrite("operation-a", "document-a", "revision-a", nil, "# exact\n", "terminal", created)
	request := OperationRequest{ID: write.OperationID, Fingerprint: "document-write", ProjectID: write.ProjectID, WorkstreamID: write.WorkstreamID, DocumentID: write.DocumentID, RevisionID: write.RevisionID, ExpectedCurrent: write.ExpectedRevision, Markdown: write.Content, Origin: write.Provenance.Origin, Client: write.Provenance.Client, Model: write.Provenance.Model, CreatedAt: write.CreatedAt}
	store, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if err := store.CreateProject(ctx, "project-a", "A"); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	if err := registerDocumentWrite(t, store, write); err != nil {
		t.Fatalf("RegisterDocumentCreate() error = %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	store, err = Open(ctx, path)
	if err != nil {
		t.Fatalf("Open(restart) error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	for name, change := range map[string]func(*app.DocumentWrite){
		"revision":   func(changed *app.DocumentWrite) { changed.RevisionID = "revision-changed" },
		"content":    func(changed *app.DocumentWrite) { changed.Content = []byte("# changed\n") },
		"provenance": func(changed *app.DocumentWrite) { changed.Provenance.Client = "changed-client" },
	} {
		changed := write
		change(&changed)
		if err := registerDocumentWrite(t, store, changed); !domain.IsOutcome(err, domain.IdempotencyMismatch) {
			t.Errorf("RegisterDocumentCreate(changed %s) error = %v, want mismatch", name, err)
		}
	}
	if err := registerDocumentWrite(t, store, write); err != nil {
		t.Fatalf("RegisterDocumentCreate(retry) error = %v", err)
	}

	files := revisionfs.New(t.TempDir(), nil)
	result, err := store.ExecuteOperation(ctx, files, request, documentAllow)
	if err != nil || result.Outcome != domain.Outcome(operationCommitted) {
		t.Fatalf("ExecuteOperation(retry) = %#v, %v; want committed", result, err)
	}
	var fingerprint, state, documentID string
	if err := store.db.QueryRowContext(ctx, `SELECT fingerprint, state, document_id FROM operations WHERE operation_id = ?`, request.ID).Scan(&fingerprint, &state, &documentID); err != nil {
		t.Fatalf("read operation ledger: %v", err)
	}
	if fingerprint != operationDigest(request) || state != operationCommitted || documentID != request.DocumentID {
		t.Fatalf("operation ledger = %q/%q/%q; want canonical committed reservation", fingerprint, state, documentID)
	}
	var documentErr DocumentError
	hijack := documentCreateWrite("operation-other", "document-a", "revision-other", nil, "# hijack\n", "terminal", created)
	if err := registerDocumentWrite(t, store, hijack); !errors.As(err, &documentErr) || documentErr.Code != "conflict" {
		t.Fatalf("RegisterDocumentCreate(hijack) error = %v, want document conflict", err)
	}
	changed := request
	changed.Markdown = []byte("# changed\n")
	if _, err := store.ExecuteOperation(ctx, files, changed, documentAllow); !domain.IsOutcome(err, domain.IdempotencyMismatch) {
		t.Fatalf("ExecuteOperation(changed content) error = %v, want mismatch", err)
	}
	changed = request
	changed.Client = "changed-client"
	if _, err := store.ExecuteOperation(ctx, files, changed, documentAllow); !domain.IsOutcome(err, domain.IdempotencyMismatch) {
		t.Fatalf("ExecuteOperation(changed provenance) error = %v, want mismatch", err)
	}
	writeB := documentCreateWrite("operation-b", "document-b", "revision-b", nil, "# b\n", "terminal", created)
	if err := registerDocumentWrite(t, store, writeB); err != nil {
		t.Fatalf("RegisterDocumentCreate(document-b) error = %v", err)
	}
	writeC := documentCreateWrite("operation-b", "document-c", "revision-c", nil, "# c\n", "terminal", created)
	if err := registerDocumentWrite(t, store, writeC); err == nil {
		t.Fatal("RegisterDocumentCreate(operation reuse) error = nil, want conflict")
	}
	var rolledBack int
	if err := store.db.QueryRowContext(ctx, `SELECT count(*) FROM documents WHERE document_id = 'document-c'`).Scan(&rolledBack); err != nil || rolledBack != 0 {
		t.Fatalf("failed registration left %d document rows, error %v", rolledBack, err)
	}
}

func TestDocumentStoreKeepsPrecommitFailuresInvisibleAndReportsOrphans(t *testing.T) {
	store, _, root := documentFixture(t)
	created := time.Now()
	write := documentCreateWrite("operation-a", "document-a", "revision-a", nil, "# orphan\n", "", created)
	if err := registerDocumentWrite(t, store, write); err != nil {
		t.Fatalf("RegisterDocumentCreate() error = %v", err)
	}
	interrupted := revisionfs.New(root, func(point revisionfs.Point) error {
		if point == revisionfs.AfterPublish {
			return errors.New("stop after publish")
		}
		return nil
	})
	if _, _, err := store.WriteDocument(t.Context(), interrupted, "operation-a", "document-a", "project-a", "", "revision-a", nil, []byte("# orphan\n"), "cli", "", "reported", created); err == nil {
		t.Fatal("WriteDocument(precommit fault) error = nil, want failure")
	}
	count := 0
	if _, err := store.VisitDocuments(t.Context(), "project-a", "", "", 10, func(revisionfs.DocumentRevision) { count++ }); err != nil || count != 0 {
		t.Fatalf("VisitDocuments(precommit) = %d, %v; want invisible empty row", count, err)
	}
	orphans, err := store.Orphans(t.Context(), interrupted)
	if err != nil || len(orphans) != 1 {
		t.Fatalf("Orphans() = %v, %v; want one report-only immutable file", orphans, err)
	}
}

func TestDocumentStoreRejectsForeignHistoryAndCurrentIntegrityDiscrepancy(t *testing.T) {
	store, files, root := documentFixture(t)
	publishDocument(t, store, files, "operation-a", "document-a", "revision-a", nil, "# first\n", "", time.Now())
	if outcome, _, err := store.WriteDocument(t.Context(), files, "operation-update", "document-a", "project-a", "", "revision-current", stringPointer("revision-a"), []byte("# current\n"), "cli", "", "reported", time.Now()); err != nil || outcome != app.OK {
		t.Fatalf("WriteDocument(current update) = %q, %v", outcome, err)
	}
	publishDocument(t, store, files, "operation-b", "document-b", "revision-b", nil, "# other\n", "", time.Now())
	foreign := "revision-b"
	if _, _, err := store.ReadDocument(t.Context(), files, "project-a", "", "document-a", &foreign); !domain.IsOutcome(err, domain.BindingMismatch) {
		t.Fatalf("ReadDocument(foreign history) error = %v, want binding mismatch", err)
	}
	if err := os.WriteFile(revisionPathFor(root, "project-a", "document-a", "revision-a"), []byte("altered"), 0o600); err != nil {
		t.Fatalf("WriteFile(altered history) error = %v", err)
	}
	historical := "revision-a"
	if _, _, err := store.ReadDocument(t.Context(), files, "project-a", "", "document-a", &historical); !domain.IsOutcome(err, domain.IntegrityDiscrepancy) {
		t.Fatalf("ReadDocument(altered history) error = %v, want integrity discrepancy", err)
	}
	currentPath := revisionPathFor(root, "project-a", "document-a", "revision-current")
	if err := os.Remove(currentPath); err != nil {
		t.Fatalf("Remove(current) error = %v", err)
	}
	if err := os.Mkdir(currentPath, 0o755); err != nil {
		t.Fatalf("Mkdir(nonregular current) error = %v", err)
	}
	if _, _, err := store.ReadDocument(t.Context(), files, "project-a", "", "document-a", nil); !domain.IsOutcome(err, domain.IntegrityDiscrepancy) {
		t.Fatalf("ReadDocument(nonregular current) error = %v, want integrity discrepancy", err)
	}
}

func publishDocument(t *testing.T, store *Store, files *revisionfs.Filesystem, operationID, documentID, revisionID string, expected *string, content, client string, createdAt time.Time) {
	t.Helper()
	write := documentCreateWrite(operationID, documentID, revisionID, expected, content, client, createdAt)
	if err := registerDocumentWrite(t, store, write); err != nil {
		t.Fatalf("RegisterDocumentCreate(%s) error = %v", documentID, err)
	}
	outcome, _, err := store.WriteDocument(t.Context(), files, operationID, documentID, "project-a", "", revisionID, expected, []byte(content), "cli", client, "reported", createdAt)
	if err != nil || outcome != app.OK {
		t.Fatalf("WriteDocument(%s) = %q, %v", revisionID, outcome, err)
	}
}

func documentFixture(t *testing.T) (*Store, *revisionfs.Filesystem, string) {
	t.Helper()
	store, err := Open(t.Context(), testkit.TempSQLitePath(t, "document-fixture"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.CreateProject(t.Context(), "project-a", "A"); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	root := t.TempDir()
	return store, revisionfs.New(root, nil), root
}

func documentCreateWrite(operationID, documentID, revisionID string, expected *string, content, client string, createdAt time.Time) app.DocumentWrite {
	return app.DocumentWrite{
		OperationID: operationID, DocumentID: documentID, RevisionID: revisionID, ProjectID: "project-a", ExpectedRevision: expected,
		Content: []byte(content), Provenance: app.DocumentProvenance{Origin: "cli", Client: client, Model: "reported"}, CreatedAt: createdAt,
	}
}

func registerDocumentWrite(t *testing.T, store *Store, write app.DocumentWrite) error {
	t.Helper()
	return store.RegisterDocumentCreate(t.Context(), write.OperationID, write.DocumentID, write.ProjectID, write.WorkstreamID, write.RevisionID, write.ExpectedRevision, write.Content, write.Provenance.Origin, write.Provenance.Client, write.Provenance.Model, write.CreatedAt)
}

func documentAllow(context.Context) error { return nil }
