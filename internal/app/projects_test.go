package app

import (
	"encoding/json"
	"fmt"
	"testing"

	"memgraphai/internal/store/sqlite"
	"memgraphai/internal/testkit"
)

func TestProjectServiceCreatePersistsImmutableIdentity(t *testing.T) {
	store, err := sqlite.Open(t.Context(), testkit.TempSQLitePath(t, "project-service-create"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	request := projectRequest(t, "op-create", "project.create", Scope{Kind: "library"}, `{"project_id":"project-a","name":"Alpha"}`)
	result := (ProjectService{Store: store}).Handle(t.Context(), request)
	if result.Outcome != OK {
		t.Fatalf("Handle(project.create) outcome = %q, want %q", result.Outcome, OK)
	}
	if name, err := store.ProjectName(t.Context(), "project-a"); err != nil || name != "Alpha" {
		t.Fatalf("ProjectName() = %q, %v; want persisted Alpha", name, err)
	}
}

func TestProjectServicePreservesExplicitAssociationAndResolutionOutcomes(t *testing.T) {
	store, err := sqlite.Open(t.Context(), testkit.TempSQLitePath(t, "project-service-associations"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	service := ProjectService{Store: store}
	root := testkit.TempProjectDir(t, "root")
	alias := root + "-alias"
	testkit.SymlinkOrSkip(t, root, alias)

	for _, id := range []string{"project-a", "project-b", "project-c"} {
		result := service.Handle(t.Context(), projectRequest(t, "create-"+id, "project.create", Scope{Kind: "library"}, `{"project_id":"`+id+`","name":"same name"}`))
		if result.Outcome != OK {
			t.Fatalf("create %q outcome = %q, want %q", id, result.Outcome, OK)
		}
	}
	limit := 2
	first := service.Handle(t.Context(), Request{ContractVersion: ContractVersion, OperationID: "list-one", Operation: "project.list", Scope: Scope{Kind: "library"}, Input: json.RawMessage(`{}`), Page: &Page{Limit: &limit}})
	if first.Outcome != OK {
		t.Fatalf("first project list outcome = %q, want %q", first.Outcome, OK)
	}

	added := service.Handle(t.Context(), projectRequest(t, "add-a", "project.association.add", Scope{Kind: "project", ProjectID: "project-a"}, `{"path":`+quoteJSON(root)+`}`))
	if added.Outcome != OK {
		t.Fatalf("association add outcome = %q, want %q", added.Outcome, OK)
	}
	if replay := service.Handle(t.Context(), projectRequest(t, "add-a-again", "project.association.add", Scope{Kind: "project", ProjectID: "project-a"}, `{"path":`+quoteJSON(root)+`}`)); replay.Outcome != OK {
		t.Fatalf("idempotent association outcome = %q, want %q", replay.Outcome, OK)
	}
	if resolved := service.Handle(t.Context(), projectRequest(t, "resolve-alias", "project.resolve", Scope{Kind: "library"}, `{"path":`+quoteJSON(alias)+`}`)); resolved.Outcome != OK {
		t.Fatalf("alias resolution outcome = %q, want %q", resolved.Outcome, OK)
	}
	if added := service.Handle(t.Context(), projectRequest(t, "add-b", "project.association.add", Scope{Kind: "project", ProjectID: "project-b"}, `{"path":`+quoteJSON(alias)+`}`)); added.Outcome != OK {
		t.Fatalf("second association outcome = %q, want %q", added.Outcome, OK)
	}
	if ambiguous := service.Handle(t.Context(), projectRequest(t, "resolve-many", "project.resolve", Scope{Kind: "library"}, `{"path":`+quoteJSON(root)+`}`)); ambiguous.Outcome != Ambiguous {
		t.Fatalf("ambiguous resolution outcome = %q, want %q", ambiguous.Outcome, Ambiguous)
	}
	if removed := service.Handle(t.Context(), projectRequest(t, "remove-a", "project.association.remove", Scope{Kind: "project", ProjectID: "project-a"}, `{"path":`+quoteJSON(root)+`}`)); removed.Outcome != OK {
		t.Fatalf("remove outcome = %q, want %q", removed.Outcome, OK)
	}
	if missing := service.Handle(t.Context(), projectRequest(t, "remove-missing", "project.association.remove", Scope{Kind: "project", ProjectID: "project-a"}, `{"path":`+quoteJSON(root)+`}`)); missing.Outcome != NotFound {
		t.Fatalf("missing remove outcome = %q, want %q", missing.Outcome, NotFound)
	}
}

func quoteJSON(value string) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}

func TestProjectListIssuesBoundTokensAndRejectsStaleOrReboundTokens(t *testing.T) {
	store, err := sqlite.Open(t.Context(), testkit.TempSQLitePath(t, "project-pagination"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	service := ProjectService{Store: store}
	for _, id := range []string{"project-a", "project-b", "project-c"} {
		if result := service.Handle(t.Context(), projectRequest(t, "create-"+id, "project.create", Scope{Kind: "library"}, `{"project_id":"`+id+`","name":"`+id+`"}`)); result.Outcome != OK {
			t.Fatalf("create %q outcome = %q, want ok", id, result.Outcome)
		}
	}
	limit := 1
	raw, response := Service{}.ExecuteJSON(t.Context(), projectEnvelope("list-first", &Page{Limit: &limit, Token: ""}), "test", service.Handle)
	if response.Outcome.Code != OK {
		t.Fatalf("first page outcome = %q, want ok", response.Outcome.Code)
	}
	var envelope struct {
		Page struct {
			NextToken string `json:"next_token"`
		} `json:"page"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("decode first page: %v", err)
	}
	if envelope.Page.NextToken == "" {
		t.Fatal("first page next_token is empty, want an opaque continuation token")
	}

	_, continued := Service{}.ExecuteJSON(t.Context(), projectEnvelope("list-next", &Page{Limit: &limit, Token: envelope.Page.NextToken}), "test", service.Handle)
	if continued.Outcome.Code != OK {
		t.Fatalf("continued page outcome = %q, want ok", continued.Outcome.Code)
	}
	_, malformed := Service{}.ExecuteJSON(t.Context(), projectEnvelope("list-malformed", &Page{Limit: &limit, Token: "%%%"}), "test", service.Handle)
	if malformed.Outcome.Code != Invalid {
		t.Fatalf("malformed token outcome = %q, want invalid", malformed.Outcome.Code)
	}
	wrongLimit := 2
	_, rebound := Service{}.ExecuteJSON(t.Context(), projectEnvelope("list-rebound", &Page{Limit: &wrongLimit, Token: envelope.Page.NextToken}), "test", service.Handle)
	if rebound.Outcome.Code != Invalid {
		t.Fatalf("rebound token outcome = %q, want invalid", rebound.Outcome.Code)
	}
	if result := service.Handle(t.Context(), projectRequest(t, "create-later", "project.create", Scope{Kind: "library"}, `{"project_id":"project-d","name":"project-d"}`)); result.Outcome != OK {
		t.Fatalf("create later outcome = %q, want ok", result.Outcome)
	}
	_, stale := Service{}.ExecuteJSON(t.Context(), projectEnvelope("list-stale", &Page{Limit: &limit, Token: envelope.Page.NextToken}), "test", service.Handle)
	if stale.Outcome.Code != Conflict {
		t.Fatalf("stale token outcome = %q, want conflict", stale.Outcome.Code)
	}
}

func TestAssociationListIsBoundedAndRejectsReboundOrStaleTokens(t *testing.T) {
	store, err := sqlite.Open(t.Context(), testkit.TempSQLitePath(t, "association-pagination"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	service := ProjectService{Store: store}
	for _, id := range []string{"project-a", "project-b"} {
		if result := service.Handle(t.Context(), projectRequest(t, "create-"+id, "project.create", Scope{Kind: "library"}, `{"project_id":"`+id+`","name":"`+id+`"}`)); result.Outcome != OK {
			t.Fatalf("create %q outcome = %q", id, result.Outcome)
		}
	}
	for i := 0; i < 52; i++ {
		path := testkit.TempProjectDir(t, fmt.Sprintf("path-%03d", i))
		if err := store.AssociatePath(t.Context(), "project-a", path); err != nil {
			t.Fatalf("AssociatePath(%d) error = %v", i, err)
		}
	}
	firstRaw, first := Service{}.ExecuteJSON(t.Context(), associationEnvelope("first", "project-a", nil), "test", service.Handle)
	if first.Outcome.Code != OK || first.MetricResultCount == nil || *first.MetricResultCount != 50 || first.Page == nil || first.Page.NextToken == "" {
		t.Fatalf("first association page = %#v, want 50 results and continuation", first)
	}
	var firstValue struct {
		Result struct {
			Paths []string `json:"paths"`
		} `json:"result"`
	}
	if err := json.Unmarshal(firstRaw, &firstValue); err != nil || len(firstValue.Result.Paths) != 50 {
		t.Fatalf("first page paths = %d, %v; want 50", len(firstValue.Result.Paths), err)
	}
	limit := 50
	if _, next := (Service{}).ExecuteJSON(t.Context(), associationEnvelope("next", "project-a", &Page{Limit: &limit, Token: first.Page.NextToken}), "test", service.Handle); next.Outcome.Code != OK {
		t.Fatalf("continued page outcome = %q, want ok", next.Outcome.Code)
	}
	if _, rebound := (Service{}).ExecuteJSON(t.Context(), associationEnvelope("rebound", "project-b", &Page{Limit: &limit, Token: first.Page.NextToken}), "test", service.Handle); rebound.Outcome.Code != Invalid {
		t.Fatalf("cross-project token outcome = %q, want invalid", rebound.Outcome.Code)
	}
	if err := store.AssociatePath(t.Context(), "project-b", testkit.TempProjectDir(t, "later")); err != nil {
		t.Fatalf("later association error = %v", err)
	}
	if _, stale := (Service{}).ExecuteJSON(t.Context(), associationEnvelope("stale", "project-a", &Page{Limit: &limit, Token: first.Page.NextToken}), "test", service.Handle); stale.Outcome.Code != Conflict {
		t.Fatalf("stale token outcome = %q, want conflict", stale.Outcome.Code)
	}
}

func projectEnvelope(operationID string, page *Page) []byte {
	raw, _ := json.Marshal(Request{ContractVersion: ContractVersion, OperationID: operationID, Operation: "project.list", Scope: Scope{Kind: "library"}, Input: json.RawMessage(`{}`), Page: page})
	return raw
}

func associationEnvelope(operationID, projectID string, page *Page) []byte {
	raw, _ := json.Marshal(Request{ContractVersion: ContractVersion, OperationID: operationID, Operation: "project.association.list", Scope: Scope{Kind: "project", ProjectID: projectID}, Input: json.RawMessage(`{}`), Page: page})
	return raw
}

func projectRequest(t *testing.T, operationID, operation string, scope Scope, input string) Request {
	t.Helper()
	return Request{
		ContractVersion: ContractVersion,
		OperationID:     operationID,
		Operation:       operation,
		Scope:           scope,
		Input:           json.RawMessage(input),
	}
}
