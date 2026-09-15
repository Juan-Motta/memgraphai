package app

import (
	"encoding/json"
	"memgraphai/internal/revisionfs"
	"memgraphai/internal/store/sqlite"
	"memgraphai/internal/testkit"
	"strings"
	"testing"
)

func TestDocumentWriteRejectsUnsafePathIDsBeforeStore(t *testing.T) {
	for _, field := range []string{"operation", "project", "document", "revision"} {
		for _, bad := range []string{"with space", "with/slash", "with:colon", "á", ".."} {
			t.Run(field+"/"+bad, func(t *testing.T) {
				store := &documentStoreStub{}
				input := map[string]any{"document_id": "doc", "revision_id": "rev", "expected_revision_id": nil, "content_base64": "YQ", "provenance": map[string]string{"origin": "human author"}}
				request := Request{Operation: "document.create", OperationID: "op", Scope: Scope{Kind: "project", ProjectID: "project"}}
				switch field {
				case "operation":
					request.OperationID = bad
				case "project":
					request.Scope.ProjectID = bad
				case "document":
					input["document_id"] = bad
				case "revision":
					input["revision_id"] = bad
				}
				request.Input, _ = json.Marshal(input)
				result := (DocumentService{Store: store, Files: revisionfs.New(t.TempDir(), nil)}).Handle(t.Context(), request)
				if result.Outcome != Invalid || store.registered.OperationID != "" || store.written.OperationID != "" {
					t.Fatalf("outcome=%s registered=%q written=%q; want invalid without store calls", result.Outcome, store.registered.OperationID, store.written.OperationID)
				}
			})
		}
	}
}

func TestProjectCreateRejectsUnsafeIDButPreservesNames(t *testing.T) {
	store, err := sqlite.Open(t.Context(), testkit.TempSQLitePath(t, "safe-project"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	service := ProjectService{Store: store}
	for _, id := range []string{"bad project", "bad/project", ".."} {
		raw, _ := json.Marshal(map[string]string{"project_id": id, "name": "Human Project: α"})
		if result := service.Handle(t.Context(), Request{Operation: "project.create", Scope: Scope{Kind: "library"}, Input: raw}); result.Outcome != Invalid {
			t.Errorf("%q = %s, want invalid", id, result.Outcome)
		}
		if _, err := store.ProjectName(t.Context(), id); err == nil {
			t.Errorf("invalid project %q persisted", id)
		}
	}
	if result := service.Handle(t.Context(), Request{Operation: "project.create", Scope: Scope{Kind: "library"}, Input: json.RawMessage(`{"project_id":"valid_project-1","name":"Human Project: α"}`)}); result.Outcome != OK {
		t.Fatal(result.Outcome)
	}
}

func TestCheckpointSaveRejectsUnsafePathIDs(t *testing.T) {
	for _, field := range []string{"operation", "project", "checkpoint_document_id", "checkpoint_revision_id"} {
		t.Run(field, func(t *testing.T) {
			input := map[string]any{"checkpoint_id": "checkpoint", "checkpoint_document_id": "doc", "checkpoint_revision_id": "rev", "prose_base64": "YQ", "references": []map[string]string{{"document_id": "source", "revision_id": "source-rev"}}, "provenance": map[string]string{"origin": "human author"}}
			request := Request{Operation: "checkpoint.save", OperationID: "op", Scope: Scope{Kind: "session", ProjectID: "project", WorkstreamID: "stream", SessionID: "session"}}
			switch field {
			case "operation":
				request.OperationID = "bad/op"
			case "project":
				request.Scope.ProjectID = "bad project"
			default:
				input[field] = "bad/id"
			}
			request.Input, _ = json.Marshal(input)
			if result := (CheckpointService{Store: checkpointStoreStub{}, Files: revisionfs.New(t.TempDir(), nil)}).Handle(t.Context(), request); result.Outcome != Invalid {
				t.Fatalf("outcome=%s, want invalid", result.Outcome)
			}
		})
	}
}

func TestDocumentPathValidationPreservesNonPathText(t *testing.T) {
	store := &documentStoreStub{}
	request := Request{Operation: "document.create", OperationID: "valid_op-1", Scope: Scope{Kind: "workstream", ProjectID: "project", WorkstreamID: "work stream:α"}, Input: json.RawMessage(`{"document_id":"doc","revision_id":"rev","expected_revision_id":null,"content_base64":"YQ","provenance":{"origin":"human author: α","client":"client / 1","model":"provider/model"}}`)}
	if result := (DocumentService{Store: store, Files: revisionfs.New(t.TempDir(), nil)}).Handle(t.Context(), request); result.Outcome != OK {
		t.Fatal(result.Outcome)
	}
	if store.written.Provenance.Origin != "human author: α" || store.written.WorkstreamID != "work stream:α" {
		t.Fatalf("non-path fields changed: %+v", store.written)
	}
}

func TestPublicationPreservesLongProjectAndOperationIDs(t *testing.T) {
	store, err := sqlite.Open(t.Context(), testkit.TempSQLitePath(t, "long-identities"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	projectID, operationID := strings.Repeat("p", 255), strings.Repeat("o", 255)
	raw, _ := json.Marshal(map[string]string{"project_id": projectID, "name": "Long valid identity"})
	if result := (ProjectService{Store: store}).Handle(t.Context(), Request{Operation: "project.create", Scope: Scope{Kind: "library"}, Input: raw}); result.Outcome != OK {
		t.Fatalf("project create=%s, want previously accepted path-safe ID", result.Outcome)
	}
	service := DocumentService{Store: store, Files: revisionfs.New(t.TempDir(), nil)}
	result := service.Handle(t.Context(), Request{Operation: "document.create", OperationID: operationID, Scope: Scope{Kind: "project", ProjectID: projectID}, Input: json.RawMessage(`{"document_id":"doc","revision_id":"rev","expected_revision_id":null,"content_base64":"YQ","provenance":{"origin":"cli"}}`)})
	if result.Outcome != OK {
		t.Fatalf("document create=%s, want filesystem-supported project and operation IDs", result.Outcome)
	}
	for _, field := range []string{"document_id", "revision_id"} {
		input := map[string]any{"document_id": "other-doc", "revision_id": "other-rev", "expected_revision_id": nil, "content_base64": "YQ", "provenance": map[string]string{"origin": "cli"}}
		input[field] = strings.Repeat("x", 129)
		raw, _ := json.Marshal(input)
		result := service.Handle(t.Context(), Request{Operation: "document.create", OperationID: "other-op", Scope: Scope{Kind: "project", ProjectID: projectID}, Input: raw})
		if result.Outcome != Invalid {
			t.Fatalf("oversize %s=%s, want existing document/revision bound", field, result.Outcome)
		}
	}
}

func TestProjectCreateRejectsFilesystemOversizeBeforePersistence(t *testing.T) {
	store, err := sqlite.Open(t.Context(), testkit.TempSQLitePath(t, "oversize-project"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	projectID := strings.Repeat("p", 256)
	raw, _ := json.Marshal(map[string]string{"project_id": projectID, "name": "Oversize"})
	if result := (ProjectService{Store: store}).Handle(t.Context(), Request{Operation: "project.create", Scope: Scope{Kind: "library"}, Input: raw}); result.Outcome != Invalid {
		t.Fatalf("create=%s, want invalid before physical path failure", result.Outcome)
	}
	if _, err := store.ProjectName(t.Context(), projectID); err == nil {
		t.Fatal("filesystem-oversize project persisted")
	}
}
