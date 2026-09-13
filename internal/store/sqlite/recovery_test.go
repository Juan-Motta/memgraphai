package sqlite

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"memgraphai/internal/domain"
	"memgraphai/internal/revisionfs"
	"memgraphai/internal/testkit"
)

func TestOperationReplayReturnsTheDurableTerminalResult(t *testing.T) {
	fixture := newRecoveryFixture(t)
	first := operationRequest("operation-1", "fingerprint-1", "revision-1", nil, "# first\n")
	if result, err := fixture.store.ExecuteOperation(t.Context(), fixture.files, first, allow); err != nil || result.Outcome != domain.Outcome(operationCommitted) {
		t.Fatalf("ExecuteOperation(first) = %#v, %v; want committed", result, err)
	}
	second := operationRequest("operation-2", "fingerprint-2", "revision-2", stringPointer("revision-1"), "# second\n")
	if _, err := fixture.store.ExecuteOperation(t.Context(), fixture.files, second, allow); err != nil {
		t.Fatalf("ExecuteOperation(second) error = %v", err)
	}
	result, err := fixture.store.ExecuteOperation(t.Context(), fixture.files, first, allow)
	if err != nil || result.Outcome != domain.Outcome(operationCommitted) || result.RevisionID != "revision-1" {
		t.Fatalf("ExecuteOperation(replay after later advance) = %#v, %v; want committed revision-1", result, err)
	}
	if current, _, err := fixture.store.CurrentRevision(t.Context(), "document-1"); err != nil || current != "revision-2" {
		t.Fatalf("CurrentRevision() = %q, %v; want later revision-2", current, err)
	}
}

func TestOperationPersistsConflictForRecoveryAndReplayAfterReopen(t *testing.T) {
	fixture := newRecoveryFixture(t)
	first := operationRequest("operation-1", "fingerprint-1", "revision-1", nil, "# first\n")
	if _, err := fixture.store.ExecuteOperation(t.Context(), fixture.files, first, allow); err != nil {
		t.Fatalf("ExecuteOperation(first) error = %v", err)
	}
	conflict := operationRequest("operation-2", "fingerprint-2", "revision-2", nil, "# stale\n")
	if result, err := fixture.store.ExecuteOperation(t.Context(), fixture.files, conflict, allow); err != nil || result.Outcome != domain.Conflict {
		t.Fatalf("ExecuteOperation(conflict) = %#v, %v; want durable conflict", result, err)
	}
	if err := fixture.store.Close(); err != nil {
		t.Fatalf("Close(conflict store) error = %v", err)
	}
	store, err := Open(t.Context(), fixture.database)
	if err != nil {
		t.Fatalf("Open(recovery store) error = %v", err)
	}
	defer store.Close()
	result, err := store.RecoverOperation(t.Context(), conflict.ID, conflict.Fingerprint, allow)
	if err != nil || result.Outcome != domain.Conflict {
		t.Fatalf("RecoverOperation(conflict) = %#v, %v; want durable conflict", result, err)
	}
	later := operationRequest("operation-3", "fingerprint-3", "revision-3", stringPointer("revision-1"), "# later\n")
	if _, err := store.ExecuteOperation(t.Context(), fixture.files, later, allow); err != nil {
		t.Fatalf("ExecuteOperation(later) error = %v", err)
	}
	result, err = store.ExecuteOperation(t.Context(), fixture.files, conflict, allow)
	if err != nil || result.Outcome != domain.Conflict {
		t.Fatalf("ExecuteOperation(conflict replay) = %#v, %v; want durable conflict", result, err)
	}
	if current, _, err := store.CurrentRevision(t.Context(), "document-1"); err != nil || current != "revision-3" {
		t.Fatalf("CurrentRevision() = %q, %v; want later revision-3", current, err)
	}
}

func TestOperationRejectsSemanticMismatchBeforePreparation(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(OperationRequest) OperationRequest
	}{
		{"project", func(request OperationRequest) OperationRequest { request.ProjectID = "project-2"; return request }},
		{"document", func(request OperationRequest) OperationRequest { request.DocumentID = "document-2"; return request }},
		{"revision", func(request OperationRequest) OperationRequest { request.RevisionID = "revision-2"; return request }},
		{"expected-current", func(request OperationRequest) OperationRequest {
			request.ExpectedCurrent = stringPointer("revision-0")
			return request
		}},
		{"markdown", func(request OperationRequest) OperationRequest {
			request.Markdown = []byte("# changed\n")
			return request
		}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			fixture := newRecoveryFixture(t)
			if err := fixture.store.CreateProject(t.Context(), "project-2", "Other"); err != nil {
				t.Fatalf("CreateProject(project-2) error = %v", err)
			}
			if err := fixture.store.CreateDocument(t.Context(), "document-2", "project-1"); err != nil {
				t.Fatalf("CreateDocument(document-2) error = %v", err)
			}
			request := operationRequest("operation-1", "fingerprint-1", "revision-1", nil, "# first\n")
			if _, err := fixture.store.ClaimOperation(t.Context(), request); err != nil {
				t.Fatalf("ClaimOperation() error = %v", err)
			}
			changed := test.mutate(request)
			if _, err := fixture.store.ExecuteOperation(t.Context(), fixture.files, changed, allow); !domain.IsOutcome(err, domain.IdempotencyMismatch) {
				t.Fatalf("ExecuteOperation(changed %s) error = %v, want %q", test.name, err, domain.IdempotencyMismatch)
			}
			path := revisionPathFor(fixture.library, changed.ProjectID, changed.DocumentID, changed.RevisionID)
			if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("Stat(changed request path) error = %v, want no preparation", err)
			}
		})
	}
}

func TestOperationRejectsCallerProjectThatDoesNotOwnDocumentBeforePreparation(t *testing.T) {
	fixture := newRecoveryFixture(t)
	if err := fixture.store.CreateProject(t.Context(), "project-2", "Other"); err != nil {
		t.Fatalf("CreateProject(project-2) error = %v", err)
	}
	request := operationRequest("operation-1", "fingerprint-1", "revision-1", nil, "# first\n")
	request.ProjectID = "project-2"
	if _, err := fixture.store.ExecuteOperation(t.Context(), fixture.files, request, allow); !domain.IsOutcome(err, domain.BindingMismatch) {
		t.Fatalf("ExecuteOperation(wrong project) error = %v, want %q", err, domain.BindingMismatch)
	}
	if _, err := os.Stat(revisionPathFor(fixture.library, request.ProjectID, request.DocumentID, request.RevisionID)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Stat(wrong-project path) error = %v, want no preparation", err)
	}
}

func TestOperationRejectsMismatchedReplayAndRechecksAuthorization(t *testing.T) {
	fixture := newRecoveryFixture(t)
	request := operationRequest("operation-1", "fingerprint-1", "revision-1", nil, "# first\n")
	if _, err := fixture.store.ExecuteOperation(t.Context(), fixture.files, request, allow); err != nil {
		t.Fatalf("ExecuteOperation() error = %v", err)
	}
	mismatch := request
	mismatch.Fingerprint = "changed-payload"
	if _, err := fixture.store.ExecuteOperation(t.Context(), fixture.files, mismatch, allow); !domain.IsOutcome(err, domain.IdempotencyMismatch) {
		t.Fatalf("ExecuteOperation(mismatch) error = %v, want %q", err, domain.IdempotencyMismatch)
	}
	denied := func(_ context.Context) error { return domain.Error{Outcome: domain.ScopeDenied} }
	if _, err := fixture.store.RecoverOperation(t.Context(), request.ID, request.Fingerprint, denied); !domain.IsOutcome(err, domain.ScopeDenied) {
		t.Fatalf("RecoverOperation(denied replay) error = %v, want %q", err, domain.ScopeDenied)
	}
}

func TestRecoveryDoesNotInferUnknownAndResolvesLostResponseFromLedger(t *testing.T) {
	fixture := newRecoveryFixture(t)
	request := operationRequest("operation-1", "fingerprint-1", "revision-1", nil, "# first\n")
	fixture.store.afterOperationCommit = func() error { return errors.New("lost response") }
	if _, err := fixture.store.ExecuteOperation(t.Context(), fixture.files, request, allow); !domain.IsOutcome(err, domain.Unknown) {
		t.Fatalf("ExecuteOperation(lost response) error = %v, want %q", err, domain.Unknown)
	}
	fixture.store.afterOperationCommit = nil
	result, err := fixture.store.RecoverOperation(t.Context(), request.ID, request.Fingerprint, allow)
	if err != nil || result.Outcome != domain.Outcome(operationCommitted) || result.RevisionID != request.RevisionID {
		t.Fatalf("RecoverOperation(lost response) = %#v, %v; want committed stored result", result, err)
	}
	unknown, err := fixture.store.RecoverOperation(t.Context(), "missing-operation", "missing-fingerprint", allow)
	if err != nil || unknown.Outcome != domain.Unknown {
		t.Fatalf("RecoverOperation(missing) = %#v, %v; want unknown without rollback inference", unknown, err)
	}
}

func TestRecoveryFencesTakeoverAndPreservesActiveAttemptStaging(t *testing.T) {
	fixture := newRecoveryFixture(t)
	request := operationRequest("operation-1", "fingerprint-1", "revision-1", nil, "# first\n")
	first, err := fixture.store.ClaimOperation(t.Context(), request)
	if err != nil || first.Generation != 1 {
		t.Fatalf("ClaimOperation(first) = %#v, %v; want generation 1", first, err)
	}
	interrupted := revisionfs.New(fixture.library, func(point revisionfs.Point) error {
		if point == revisionfs.BeforePublish {
			return errors.New("stop before publish")
		}
		return nil
	})
	if _, err := interrupted.PrepareAttempt(request.ID, first.Generation, request.ProjectID, request.DocumentID, request.RevisionID, request.Markdown); err == nil {
		t.Fatal("PrepareAttempt() error = nil, want active staging interruption")
	}
	activeStaging := filepath.Join(fixture.library, ".staging", request.ID, "1", request.RevisionID+".tmp")
	if _, err := os.Stat(activeStaging); err != nil {
		t.Fatalf("Stat(active attempt staging) error = %v, want retained staging", err)
	}
	second, err := fixture.store.ClaimOperation(t.Context(), request)
	if err != nil || second.Generation != 2 {
		t.Fatalf("ClaimOperation(takeover) = %#v, %v; want generation 2", second, err)
	}
	if _, err := os.Stat(activeStaging); err != nil {
		t.Fatalf("Stat(old active staging after takeover) error = %v, want no stale cleanup", err)
	}
	prepared, err := fixture.files.PrepareAttempt(request.ID, first.Generation, request.ProjectID, request.DocumentID, request.RevisionID, request.Markdown)
	if err != nil {
		t.Fatalf("PrepareAttempt(stale owner) error = %v", err)
	}
	if _, err := fixture.store.commitOperation(t.Context(), request, prepared, first.Generation); !domain.IsOutcome(err, domain.Retryable) {
		t.Fatalf("commitOperation(stale owner) error = %v, want %q", err, domain.Retryable)
	}
	if result, err := fixture.store.ExecuteOperation(t.Context(), fixture.files, request, allow); err != nil || result.Outcome != domain.Outcome(operationCommitted) || result.Generation != 3 {
		t.Fatalf("ExecuteOperation(takeover) = %#v, %v; want fenced committed generation 3", result, err)
	}
}

func TestTargetedIntegrityAndOrphanReportingNeverAutoImports(t *testing.T) {
	fixture := newRecoveryFixture(t)
	first := operationRequest("operation-1", "fingerprint-1", "revision-1", nil, "# first\n")
	if _, err := fixture.store.ExecuteOperation(t.Context(), fixture.files, first, allow); err != nil {
		t.Fatalf("ExecuteOperation(first) error = %v", err)
	}
	second := operationRequest("operation-2", "fingerprint-2", "revision-2", stringPointer("revision-1"), "# second\n")
	if _, err := fixture.store.ExecuteOperation(t.Context(), fixture.files, second, allow); err != nil {
		t.Fatalf("ExecuteOperation(second) error = %v", err)
	}
	if err := os.WriteFile(revisionPath(fixture.library, "revision-1"), []byte("altered"), 0o600); err != nil {
		t.Fatalf("WriteFile(altered history) error = %v", err)
	}
	if err := fixture.store.VerifyRevision(t.Context(), fixture.files, "revision-1"); !domain.IsOutcome(err, domain.IntegrityDiscrepancy) {
		t.Fatalf("VerifyRevision(altered history) error = %v, want %q", err, domain.IntegrityDiscrepancy)
	}
	if err := os.Remove(revisionPath(fixture.library, "revision-2")); err != nil {
		t.Fatalf("Remove(current) error = %v", err)
	}
	if err := fixture.store.VerifyRevision(t.Context(), fixture.files, "revision-2"); !domain.IsOutcome(err, domain.IntegrityDiscrepancy) {
		t.Fatalf("VerifyRevision(missing current) error = %v, want %q", err, domain.IntegrityDiscrepancy)
	}
	orphan, err := fixture.files.Prepare("orphan-operation", "project-1", "document-1", "orphan-revision", []byte("# orphan\n"))
	if err != nil {
		t.Fatalf("Prepare(orphan) error = %v", err)
	}
	orphans, err := fixture.store.Orphans(t.Context(), fixture.files)
	if err != nil || len(orphans) != 1 || orphans[0] != orphan.Path {
		t.Fatalf("Orphans() = %v, %v; want unimported %q", orphans, err, orphan.Path)
	}
}

type recoveryFixture struct {
	store    *Store
	files    *revisionfs.Filesystem
	library  string
	database string
}

func newRecoveryFixture(t *testing.T) recoveryFixture {
	t.Helper()
	database := testkit.TempSQLitePath(t, "recovery")
	store, err := Open(t.Context(), database)
	if err != nil {
		t.Fatalf("Open(recovery store) error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.CreateProject(t.Context(), "project-1", "Project"); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	if err := store.CreateDocument(t.Context(), "document-1", "project-1"); err != nil {
		t.Fatalf("CreateDocument() error = %v", err)
	}
	library := filepath.Join(t.TempDir(), "library")
	return recoveryFixture{store: store, files: revisionfs.New(library, nil), library: library, database: database}
}

func operationRequest(id, fingerprint, revision string, expected *string, markdown string) OperationRequest {
	return OperationRequest{ID: id, Fingerprint: fingerprint, ProjectID: "project-1", DocumentID: "document-1", RevisionID: revision, ExpectedCurrent: expected, Markdown: []byte(markdown)}
}

func allow(context.Context) error { return nil }

func revisionPath(library, revisionID string) string {
	return revisionPathFor(library, "project-1", "document-1", revisionID)
}

func revisionPathFor(library, projectID, documentID, revisionID string) string {
	return filepath.Join(library, "projects", projectID, "documents", documentID, "revisions", revisionID+".md")
}
