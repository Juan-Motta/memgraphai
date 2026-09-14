package app

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"

	"memgraphai/internal/revisionfs"
	"memgraphai/internal/store/sqlite"
	"memgraphai/internal/testkit"
)

type documentStoreStub struct {
	registered DocumentWrite
	written    DocumentWrite
	documents  []revisionfs.DocumentRevision
	generation int64
}

func (s *documentStoreStub) RegisterDocumentCreate(_ context.Context, operationID, documentID, projectID, workstreamID, revisionID string, expected *string, content []byte, origin, client, model string, createdAt time.Time) error {
	s.registered = DocumentWrite{OperationID: operationID, DocumentID: documentID, ProjectID: projectID, WorkstreamID: workstreamID, RevisionID: revisionID, ExpectedRevision: expected, Content: content, Provenance: DocumentProvenance{Origin: origin, Client: client, Model: model}, CreatedAt: createdAt}
	return nil
}
func (s *documentStoreStub) WriteDocument(_ context.Context, _ *revisionfs.Filesystem, operationID, documentID, projectID, workstreamID, revisionID string, expected *string, content []byte, origin, client, model string, createdAt time.Time) (string, string, error) {
	s.written = DocumentWrite{OperationID: operationID, DocumentID: documentID, ProjectID: projectID, WorkstreamID: workstreamID, RevisionID: revisionID, ExpectedRevision: expected, Content: content, Provenance: DocumentProvenance{Origin: origin, Client: client, Model: model}, CreatedAt: createdAt}
	return OK, revisionID, nil
}
func (s *documentStoreStub) VisitDocuments(_ context.Context, _, _, _ string, _ int, visit func(revisionfs.DocumentRevision)) (int64, error) {
	for _, document := range s.documents {
		visit(document)
	}
	return s.generation, nil
}
func (*documentStoreStub) VisitDocumentHistory(context.Context, string, string, string, int64, int, func(revisionfs.DocumentRevision)) (int64, error) {
	return 0, nil
}
func (*documentStoreStub) ReadDocument(context.Context, *revisionfs.Filesystem, string, string, string, *string) (revisionfs.DocumentRevision, []byte, error) {
	return revisionfs.DocumentRevision{}, nil, nil
}

func TestDocumentServiceScopesHistoryAndRejectsStaleUpdates(t *testing.T) {
	store, err := sqlite.Open(t.Context(), testkit.TempSQLitePath(t, "document-service"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()
	if err := store.CreateProject(t.Context(), "project-a", "A"); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	if err := store.CreateContinuityWorkstream(t.Context(), "project-a", "stream-a", "cli", "", "", time.Now()); err != nil {
		t.Fatalf("CreateContinuityWorkstream() error = %v", err)
	}
	service := DocumentService{Store: store, Files: revisionfs.New(t.TempDir(), nil), Now: func() time.Time { return time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC) }}
	create := func(operation, scope, document, revision, expected, text string) Result {
		return service.Handle(t.Context(), Request{ContractVersion: ContractVersion, OperationID: operation, Operation: "document.create", Scope: Scope{Kind: scope, ProjectID: "project-a", WorkstreamID: map[string]string{"workstream": "stream-a"}[scope]}, Input: json.RawMessage(`{"document_id":"` + document + `","revision_id":"` + revision + `","expected_revision_id":` + expected + `,"content_base64":"` + base64.RawStdEncoding.EncodeToString([]byte(text)) + `","provenance":{"origin":"cli"}}`)})
	}
	if result := create("op-project", "project", "document-general", "revision-1", "null", "# general\n"); result.Outcome != OK {
		t.Fatalf("project create outcome = %q", result.Outcome)
	}
	if result := create("op-workstream", "workstream", "document-workstream", "revision-w", "null", "# workstream\n"); result.Outcome != OK {
		t.Fatalf("workstream create outcome = %q", result.Outcome)
	}
	readProject := service.Handle(t.Context(), Request{Operation: "document.read", Scope: Scope{Kind: "project", ProjectID: "project-a"}, Input: json.RawMessage(`{"document_id":"document-general"}`)})
	if readProject.Outcome != OK {
		t.Fatalf("project general read outcome = %q", readProject.Outcome)
	}
	wrongScope := service.Handle(t.Context(), Request{Operation: "document.read", Scope: Scope{Kind: "project", ProjectID: "project-a"}, Input: json.RawMessage(`{"document_id":"document-workstream"}`)})
	if wrongScope.Outcome != ScopeDenied {
		t.Fatalf("project workstream read outcome = %q, want %q", wrongScope.Outcome, ScopeDenied)
	}
	update := service.Handle(t.Context(), Request{Operation: "document.update", OperationID: "op-update", Scope: Scope{Kind: "project", ProjectID: "project-a"}, Input: json.RawMessage(`{"document_id":"document-general","revision_id":"revision-2","expected_revision_id":"revision-1","content_base64":"` + base64.RawStdEncoding.EncodeToString([]byte("# newer\n")) + `","provenance":{"origin":"cli"}}`)})
	if update.Outcome != OK {
		t.Fatalf("update outcome = %q", update.Outcome)
	}
	stale := service.Handle(t.Context(), Request{Operation: "document.update", OperationID: "op-stale", Scope: Scope{Kind: "project", ProjectID: "project-a"}, Input: json.RawMessage(`{"document_id":"document-general","revision_id":"revision-stale","expected_revision_id":"revision-1","content_base64":"` + base64.RawStdEncoding.EncodeToString([]byte("# stale\n")) + `","provenance":{"origin":"cli"}}`)})
	if stale.Outcome != Conflict {
		t.Fatalf("stale update outcome = %q, want %q", stale.Outcome, Conflict)
	}
	history := service.Handle(t.Context(), Request{Operation: "document.history", Scope: Scope{Kind: "project", ProjectID: "project-a"}, Input: json.RawMessage(`{"document_id":"document-general"}`)})
	if history.Outcome != OK || !bytes.Contains(history.Value, []byte(`"revision-2"`)) || !bytes.Contains(history.Value, []byte(`"revision-1"`)) {
		t.Fatalf("history = %s / %q, want exact revision metadata", history.Value, history.Outcome)
	}
}

func TestDocumentServiceBindsBoundedListTokensBeforeAllocation(t *testing.T) {
	store := &documentStoreStub{generation: 1, documents: []revisionfs.DocumentRevision{{DocumentID: "document-a", RevisionID: "revision-a", ProjectID: "project-a"}, {DocumentID: "document-b", RevisionID: "revision-b", ProjectID: "project-a"}}}
	service := DocumentService{Store: store, Files: revisionfs.New(t.TempDir(), nil)}
	limit := 1
	first := service.Handle(t.Context(), Request{Operation: "document.list", Scope: Scope{Kind: "project", ProjectID: "project-a"}, Input: json.RawMessage(`{}`), Page: &Page{Limit: &limit}})
	if first.Outcome != OK || first.Page == nil || first.Page.NextToken == "" {
		t.Fatalf("first page = %#v, want bounded continuation", first)
	}
	otherLimit := 2
	if rebound := service.Handle(t.Context(), Request{Operation: "document.list", Scope: Scope{Kind: "project", ProjectID: "project-a"}, Input: json.RawMessage(`{}`), Page: &Page{Limit: &otherLimit, Token: first.Page.NextToken}}); rebound.Outcome != Invalid {
		t.Fatalf("rebound token outcome = %q, want %q", rebound.Outcome, Invalid)
	}
	store.generation = 2
	if stale := service.Handle(t.Context(), Request{Operation: "document.list", Scope: Scope{Kind: "project", ProjectID: "project-a"}, Input: json.RawMessage(`{}`), Page: &Page{Limit: &limit, Token: first.Page.NextToken}}); stale.Outcome != Conflict {
		t.Fatalf("stale token outcome = %q, want %q", stale.Outcome, Conflict)
	}
	for _, page := range []*Page{nil, {}, {Limit: intPointer(0)}, {Limit: intPointer(-1)}, {Limit: intPointer(201)}} {
		result := service.Handle(t.Context(), Request{Operation: "document.list", Scope: Scope{Kind: "project", ProjectID: "project-a"}, Input: json.RawMessage(`{}`), Page: page})
		if page != nil && page.Limit != nil && (*page.Limit < 1 || *page.Limit > 200) && result.Outcome != Invalid {
			t.Fatalf("limit %d outcome = %q, want invalid", *page.Limit, result.Outcome)
		}
	}
}

func TestDecodeDocumentContentBoundsBeforeAllocation(t *testing.T) {
	maximum := bytes.Repeat([]byte("x"), documentInputLimit)
	encodedMaximum := base64.RawStdEncoding.EncodeToString(maximum)
	content, err := decodeDocumentContent(encodedMaximum)
	if err != nil || len(content) != documentInputLimit {
		t.Fatalf("decode exact maximum = %d, %v; want %d bytes", len(content), err, documentInputLimit)
	}
	over := base64.RawStdEncoding.EncodeToString(append(maximum, 'x'))
	if _, err := decodeDocumentContent(over); err == nil {
		t.Fatal("decode one byte over maximum error = nil, want invalid")
	}
	if allocations := testing.AllocsPerRun(5, func() {
		if _, err := decodeDocumentContent(over); err == nil {
			t.Error("oversized decode error = nil, want invalid")
		}
	}); allocations != 0 {
		t.Fatalf("oversized decode allocations = %v, want zero before DecodeString", allocations)
	}
	if _, err := decodeDocumentContent(encodedMaximum[:len(encodedMaximum)-1] + "!"); err == nil {
		t.Fatal("decode invalid raw base64 error = nil, want invalid")
	}
}

func TestDocumentServiceCreatesProjectGeneralRevisionWithExplicitNullExpectation(t *testing.T) {
	store := &documentStoreStub{}
	service := DocumentService{Store: store, Files: revisionfs.New(t.TempDir(), nil), Now: func() time.Time {
		return time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	}}
	content := base64.RawStdEncoding.EncodeToString([]byte("# exact\n"))
	request := Request{ContractVersion: ContractVersion, OperationID: "document-create-1", Operation: "document.create",
		Scope: Scope{Kind: "project", ProjectID: "project-a"}, Input: []byte(`{"document_id":"document-a","revision_id":"revision-a","expected_revision_id":null,"content_base64":"` + content + `","provenance":{"origin":"cli","client":"terminal","model":"reported"}}`)}

	result := service.Handle(t.Context(), request)
	if result.Outcome != OK {
		t.Fatalf("Handle(document.create) outcome = %q, want %q", result.Outcome, OK)
	}
	if store.registered.ExpectedRevision != nil || store.written.DocumentID != "document-a" || store.written.RevisionID != "revision-a" || string(store.written.Content) != "# exact\n" || store.written.Provenance.Model != "reported" {
		t.Fatalf("durable write = %#v / %#v, want explicit-null initial revision and exact provenance/content", store.registered, store.written)
	}
}
