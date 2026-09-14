package app

import (
	"encoding/json"
	"testing"
	"time"

	"memgraphai/internal/store/sqlite"
	"memgraphai/internal/testkit"
)

func TestContinuityServiceCreatesWorkstreamWithCallerIDsAndProvenance(t *testing.T) {
	store, err := sqlite.Open(t.Context(), testkit.TempSQLitePath(t, "continuity-create"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.CreateProject(t.Context(), "project-a", "Alpha"); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	service := ContinuityService{Store: store, Now: func() time.Time {
		return time.Date(2026, 3, 2, 3, 4, 5, 0, time.UTC)
	}}
	result := service.Handle(t.Context(), continuityRequest(t, "workstream.create", Scope{Kind: "project", ProjectID: "project-a"}, `{"workstream_id":"stream-a","origin":"claude-code","client":"claude","model":"client-reported-model"}`))
	if result.Outcome != OK {
		t.Fatalf("Handle(workstream.create) outcome = %q, want %q", result.Outcome, OK)
	}

	var value struct {
		WorkstreamID string `json:"workstream_id"`
		ProjectID    string `json:"project_id"`
		Origin       string `json:"origin"`
	}
	if err := json.Unmarshal(result.Value, &value); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if value.WorkstreamID != "stream-a" || value.ProjectID != "project-a" || value.Origin != "claude-code" {
		t.Fatalf("workstream result = %#v, want caller IDs and origin", value)
	}
}

func TestContinuityServiceListsBoundedMetadataAndForksWithoutMutatingSource(t *testing.T) {
	store, err := sqlite.Open(t.Context(), testkit.TempSQLitePath(t, "continuity-list-fork"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	for _, projectID := range []string{"project-a", "project-empty"} {
		if err := store.CreateProject(t.Context(), projectID, projectID); err != nil {
			t.Fatalf("CreateProject(%q) error = %v", projectID, err)
		}
	}
	service := ContinuityService{Store: store, Now: func() time.Time { return time.Date(2026, 3, 2, 3, 4, 5, 0, time.UTC) }}
	for _, id := range []string{"stream-a", "stream-c"} {
		if result := service.Handle(t.Context(), continuityRequest(t, "workstream.create", Scope{Kind: "project", ProjectID: "project-a"}, `{"workstream_id":"`+id+`","origin":"cli"}`)); result.Outcome != OK {
			t.Fatalf("create %q outcome = %q, want ok", id, result.Outcome)
		}
	}
	limit := 1
	first := service.Handle(t.Context(), Request{ContractVersion: ContractVersion, OperationID: "op-list-first", Operation: "workstream.list", Scope: Scope{Kind: "project", ProjectID: "project-a"}, Input: json.RawMessage(`{}`), Page: &Page{Limit: &limit}})
	if first.Outcome != OK || first.Page == nil || first.Page.NextToken == "" || first.ResultCount == nil || *first.ResultCount != 1 {
		t.Fatalf("first workstream page = %#v, want one metadata result and continuation", first)
	}
	fork := service.Handle(t.Context(), continuityRequest(t, "workstream.fork", Scope{Kind: "workstream", ProjectID: "project-a", WorkstreamID: "stream-a"}, `{"workstream_id":"stream-b","origin":"cli","client":"terminal"}`))
	if fork.Outcome != OK {
		t.Fatalf("Handle(workstream.fork) outcome = %q, want ok", fork.Outcome)
	}
	allLimit := 10
	all := service.Handle(t.Context(), Request{ContractVersion: ContractVersion, OperationID: "op-list-all", Operation: "workstream.list", Scope: Scope{Kind: "project", ProjectID: "project-a"}, Input: json.RawMessage(`{}`), Page: &Page{Limit: &allLimit}})
	var listed struct {
		Workstreams []struct {
			WorkstreamID string `json:"workstream_id"`
			ForkedFrom   string `json:"forked_from_workstream_id"`
		} `json:"workstreams"`
	}
	if all.Outcome != OK || json.Unmarshal(all.Value, &listed) != nil || len(listed.Workstreams) != 3 || listed.Workstreams[0].WorkstreamID != "stream-a" || listed.Workstreams[1].ForkedFrom != "stream-a" {
		t.Fatalf("listed workstreams = %#v, want source unchanged plus a distinct fork", listed)
	}
	empty := service.Handle(t.Context(), Request{ContractVersion: ContractVersion, OperationID: "op-list-empty", Operation: "workstream.list", Scope: Scope{Kind: "project", ProjectID: "project-empty"}, Input: json.RawMessage(`{}`), Page: &Page{Limit: &allLimit}})
	if empty.Outcome != OK || empty.ResultCount == nil || *empty.ResultCount != 0 {
		t.Fatalf("empty existing project list = %#v, want ok empty result", empty)
	}
	missing := service.Handle(t.Context(), Request{ContractVersion: ContractVersion, OperationID: "op-list-missing", Operation: "workstream.list", Scope: Scope{Kind: "project", ProjectID: "project-missing"}, Input: json.RawMessage(`{}`), Page: &Page{Limit: &allLimit}})
	if missing.Outcome != NotFound {
		t.Fatalf("missing project list outcome = %q, want not_found", missing.Outcome)
	}
}

func TestContinuityServiceKeepsDisconnectCloseAndResumeDistinct(t *testing.T) {
	store, err := sqlite.Open(t.Context(), testkit.TempSQLitePath(t, "continuity-sessions"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.CreateProject(t.Context(), "project-a", "Alpha"); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	clock := func() time.Time { return time.Date(2026, 3, 2, 3, 4, 5, 0, time.UTC) }
	service := ContinuityService{Store: store, Now: clock}
	if result := service.Handle(t.Context(), continuityRequest(t, "workstream.create", Scope{Kind: "project", ProjectID: "project-a"}, `{"workstream_id":"stream-a","origin":"cli"}`)); result.Outcome != OK {
		t.Fatalf("create workstream outcome = %q, want ok", result.Outcome)
	}
	scope := Scope{Kind: "workstream", ProjectID: "project-a", WorkstreamID: "stream-a"}
	opened := service.Handle(t.Context(), continuityRequest(t, "session.open", scope, `{"session_id":"session-a","origin":"cli","model":"client-model"}`))
	if opened.Outcome != OK {
		t.Fatalf("open outcome = %q, want ok", opened.Outcome)
	}
	sessionScope := Scope{Kind: "session", ProjectID: "project-a", WorkstreamID: "stream-a", SessionID: "session-a"}
	if status := service.Handle(t.Context(), continuityRequest(t, "session.status", sessionScope, `{}`)); status.Outcome != OK {
		t.Fatalf("initial status outcome = %q, want structural lookup", status.Outcome)
	}
	if rawEOF := service.Handle(t.Context(), continuityRequest(t, "session.disconnect", sessionScope, `{}`)); rawEOF.Outcome != Invalid {
		t.Fatalf("unattributable disconnect outcome = %q, want invalid", rawEOF.Outcome)
	}
	if disconnected := service.Handle(t.Context(), continuityRequest(t, "session.disconnect", sessionScope, `{"observed_by":"mcp-request-42"}`)); disconnected.Outcome != OK {
		t.Fatalf("attributable disconnect outcome = %q, want ok", disconnected.Outcome)
	}
	if err := store.ValidateActiveBinding(t.Context(), "project-a", "stream-a", "session-a"); err == nil {
		t.Fatal("ValidateActiveBinding(disconnected) error = nil, want rejection")
	}
	if closed := service.Handle(t.Context(), continuityRequest(t, "session.close", sessionScope, `{}`)); closed.Outcome != OK {
		t.Fatalf("close disconnected session outcome = %q, want ok", closed.Outcome)
	}
	if repeated := service.Handle(t.Context(), continuityRequest(t, "session.close", sessionScope, `{}`)); repeated.Outcome != Conflict {
		t.Fatalf("repeated close outcome = %q, want conflict", repeated.Outcome)
	}
	if reopened := service.Handle(t.Context(), continuityRequest(t, "session.open", scope, `{"session_id":"session-a","origin":"cli"}`)); reopened.Outcome != Conflict {
		t.Fatalf("open with closed session ID outcome = %q, want conflict instead of reopen", reopened.Outcome)
	}
	resumed := service.Handle(t.Context(), continuityRequest(t, "session.resume", scope, `{"session_id":"session-b","source_session_id":"session-a","origin":"codex","client":"codex-cli"}`))
	if resumed.Outcome != OK {
		t.Fatalf("resume outcome = %q, want a distinct new open session", resumed.Outcome)
	}
	resumedScope := Scope{Kind: "session", ProjectID: "project-a", WorkstreamID: "stream-a", SessionID: "session-b"}
	var value struct {
		Status               string `json:"status"`
		ResumedFromSessionID string `json:"resumed_from_session_id"`
	}
	status := service.Handle(t.Context(), continuityRequest(t, "session.status", resumedScope, `{}`))
	if status.Outcome != OK || json.Unmarshal(status.Value, &value) != nil || value.Status != "open" || value.ResumedFromSessionID != "session-a" {
		t.Fatalf("resumed status = %#v / %#v, want distinct open session with explicit source", status, value)
	}
	source := service.Handle(t.Context(), continuityRequest(t, "session.status", sessionScope, `{}`))
	var sourceValue struct {
		Status string `json:"status"`
	}
	if source.Outcome != OK || json.Unmarshal(source.Value, &sourceValue) != nil || sourceValue.Status != "closed" {
		t.Fatalf("source after resume = %#v / %#v, want unchanged closed source", source, sourceValue)
	}
}

func TestContinuityServiceRejectsMissingAndCrossWorkstreamBindings(t *testing.T) {
	store, err := sqlite.Open(t.Context(), testkit.TempSQLitePath(t, "continuity-mismatch"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.CreateProject(t.Context(), "project-a", "Alpha"); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	service := ContinuityService{Store: store}
	for _, id := range []string{"stream-a", "stream-b"} {
		if result := service.Handle(t.Context(), continuityRequest(t, "workstream.create", Scope{Kind: "project", ProjectID: "project-a"}, `{"workstream_id":"`+id+`","origin":"cli"}`)); result.Outcome != OK {
			t.Fatalf("create %q outcome = %q", id, result.Outcome)
		}
	}
	streamB := Scope{Kind: "workstream", ProjectID: "project-a", WorkstreamID: "stream-b"}
	if opened := service.Handle(t.Context(), continuityRequest(t, "session.open", streamB, `{"session_id":"session-b","origin":"cli"}`)); opened.Outcome != OK {
		t.Fatalf("open stream-b session outcome = %q", opened.Outcome)
	}
	missingScope := Scope{Kind: "session", ProjectID: "project-a", WorkstreamID: "stream-a", SessionID: "missing"}
	if missing := service.Handle(t.Context(), continuityRequest(t, "session.status", missingScope, `{}`)); missing.Outcome != NotFound {
		t.Fatalf("missing session status outcome = %q, want not_found", missing.Outcome)
	}
	mismatchedScope := Scope{Kind: "session", ProjectID: "project-a", WorkstreamID: "stream-a", SessionID: "session-b"}
	if mismatched := service.Handle(t.Context(), continuityRequest(t, "session.status", mismatchedScope, `{}`)); mismatched.Outcome != BindingMismatch {
		t.Fatalf("mismatched status outcome = %q, want binding_mismatch", mismatched.Outcome)
	}
	streamA := Scope{Kind: "workstream", ProjectID: "project-a", WorkstreamID: "stream-a"}
	if resume := service.Handle(t.Context(), continuityRequest(t, "session.resume", streamA, `{"session_id":"session-c","source_session_id":"session-b","origin":"cli"}`)); resume.Outcome != BindingMismatch {
		t.Fatalf("cross-workstream resume outcome = %q, want binding_mismatch", resume.Outcome)
	}
}

func TestContinuityServiceRejectsMissingRequiredScopeAndDuplicateStableIDs(t *testing.T) {
	store, err := sqlite.Open(t.Context(), testkit.TempSQLitePath(t, "continuity-scope-duplicates"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.CreateProject(t.Context(), "project-a", "Alpha"); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	service := ContinuityService{Store: store}
	if missing := service.Handle(t.Context(), continuityRequest(t, "workstream.create", Scope{Kind: "library"}, `{"workstream_id":"stream-a","origin":"cli"}`)); missing.Outcome != ScopeDenied {
		t.Fatalf("library-scoped workstream create outcome = %q, want scope_denied", missing.Outcome)
	}
	projectScope := Scope{Kind: "project", ProjectID: "project-a"}
	created := continuityRequest(t, "workstream.create", projectScope, `{"workstream_id":"stream-a","origin":"cli"}`)
	if result := service.Handle(t.Context(), created); result.Outcome != OK {
		t.Fatalf("first workstream create outcome = %q", result.Outcome)
	}
	if duplicate := service.Handle(t.Context(), created); duplicate.Outcome != Conflict {
		t.Fatalf("duplicate workstream ID outcome = %q, want conflict", duplicate.Outcome)
	}
}

func TestContinuityServiceBindsWorkstreamPagesAndInterleavedSessionScopes(t *testing.T) {
	store, err := sqlite.Open(t.Context(), testkit.TempSQLitePath(t, "continuity-pages-interleaving"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.CreateProject(t.Context(), "project-a", "Alpha"); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	service := ContinuityService{Store: store}
	for _, streamID := range []string{"stream-a", "stream-b", "stream-c"} {
		if result := service.Handle(t.Context(), continuityRequest(t, "workstream.create", Scope{Kind: "project", ProjectID: "project-a"}, `{"workstream_id":"`+streamID+`","origin":"cli"}`)); result.Outcome != OK {
			t.Fatalf("create %q outcome = %q", streamID, result.Outcome)
		}
	}
	limit := 1
	first := service.Handle(t.Context(), Request{ContractVersion: ContractVersion, OperationID: "op-page-first", Operation: "workstream.list", Scope: Scope{Kind: "project", ProjectID: "project-a"}, Input: json.RawMessage(`{}`), Page: &Page{Limit: &limit}})
	if first.Outcome != OK || first.Page == nil || first.Page.NextToken == "" {
		t.Fatalf("first bounded page = %#v, want continuation", first)
	}
	if malformed := service.Handle(t.Context(), Request{ContractVersion: ContractVersion, OperationID: "op-page-malformed", Operation: "workstream.list", Scope: Scope{Kind: "project", ProjectID: "project-a"}, Input: json.RawMessage(`{}`), Page: &Page{Limit: &limit, Token: "%%%"}}); malformed.Outcome != Invalid {
		t.Fatalf("malformed continuation outcome = %q, want invalid", malformed.Outcome)
	}
	otherLimit := 2
	if rebound := service.Handle(t.Context(), Request{ContractVersion: ContractVersion, OperationID: "op-page-rebound", Operation: "workstream.list", Scope: Scope{Kind: "project", ProjectID: "project-a"}, Input: json.RawMessage(`{}`), Page: &Page{Limit: &otherLimit, Token: first.Page.NextToken}}); rebound.Outcome != Invalid {
		t.Fatalf("rebound continuation outcome = %q, want invalid", rebound.Outcome)
	}
	if created := service.Handle(t.Context(), continuityRequest(t, "workstream.create", Scope{Kind: "project", ProjectID: "project-a"}, `{"workstream_id":"stream-d","origin":"cli"}`)); created.Outcome != OK {
		t.Fatalf("create stale view row outcome = %q", created.Outcome)
	}
	if stale := service.Handle(t.Context(), Request{ContractVersion: ContractVersion, OperationID: "op-page-stale", Operation: "workstream.list", Scope: Scope{Kind: "project", ProjectID: "project-a"}, Input: json.RawMessage(`{}`), Page: &Page{Limit: &limit, Token: first.Page.NextToken}}); stale.Outcome != Conflict {
		t.Fatalf("stale continuation outcome = %q, want conflict", stale.Outcome)
	}
	for _, session := range []struct{ streamID, sessionID string }{{"stream-a", "session-a"}, {"stream-b", "session-b"}} {
		if result := service.Handle(t.Context(), continuityRequest(t, "session.open", Scope{Kind: "workstream", ProjectID: "project-a", WorkstreamID: session.streamID}, `{"session_id":"`+session.sessionID+`","origin":"shared-test"}`)); result.Outcome != OK {
			t.Fatalf("open %q outcome = %q", session.sessionID, result.Outcome)
		}
	}
	for _, session := range []struct{ streamID, sessionID string }{{"stream-a", "session-a"}, {"stream-b", "session-b"}, {"stream-a", "session-a"}} {
		result := service.Handle(t.Context(), continuityRequest(t, "session.status", Scope{Kind: "session", ProjectID: "project-a", WorkstreamID: session.streamID, SessionID: session.sessionID}, `{}`))
		var value struct {
			SessionID string `json:"session_id"`
		}
		if result.Outcome != OK || json.Unmarshal(result.Value, &value) != nil || value.SessionID != session.sessionID {
			t.Fatalf("interleaved status for %q = %#v / %#v, want its declared session", session.sessionID, result, value)
		}
	}
}

func TestContinuityServiceNormalizesAndBoundsWorkstreamPagination(t *testing.T) {
	store, err := sqlite.Open(t.Context(), testkit.TempSQLitePath(t, "continuity-page-bounds"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.CreateProject(t.Context(), "project-a", "Alpha"); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	service := ContinuityService{Store: store}
	for i := 0; i < 51; i++ {
		input := `{"workstream_id":"stream-` + string(rune('a'+i/26)) + string(rune('a'+i%26)) + `","origin":"cli"}`
		if result := service.Handle(t.Context(), continuityRequest(t, "workstream.create", Scope{Kind: "project", ProjectID: "project-a"}, input)); result.Outcome != OK {
			t.Fatalf("create workstream %d outcome = %q, want ok", i, result.Outcome)
		}
	}

	for _, test := range []struct {
		name string
		page *Page
	}{
		{name: "nil page"},
		{name: "empty page", page: &Page{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := Request{ContractVersion: ContractVersion, OperationID: "op-default", Operation: "workstream.list", Scope: Scope{Kind: "project", ProjectID: "project-a"}, Input: json.RawMessage(`{}`), Page: test.page}
			result, panicked := callContinuityService(service, t, request)
			if panicked {
				t.Fatal("Handle(workstream.list) panicked")
			}
			if result.Outcome != OK || result.ResultCount == nil || *result.ResultCount != 50 || result.Page == nil || result.Page.NextToken == "" {
				t.Fatalf("default page = %#v, want 50 rows and continuation", result)
			}
			if request.Page != test.page || request.Page != nil && request.Page.Limit != nil {
				t.Fatalf("request page mutated to %#v", request.Page)
			}
		})
	}

	for _, test := range []struct {
		name  string
		limit int
		want  string
	}{
		{name: "zero", limit: 0, want: Invalid},
		{name: "negative", limit: -1, want: Invalid},
		{name: "above maximum", limit: 201, want: Invalid},
		{name: "maximum int", limit: int(^uint(0) >> 1), want: Invalid},
		{name: "minimum", limit: 1, want: OK},
		{name: "maximum", limit: 200, want: OK},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := Request{ContractVersion: ContractVersion, OperationID: "op-bounds", Operation: "workstream.list", Scope: Scope{Kind: "project", ProjectID: "project-a"}, Input: json.RawMessage(`{}`), Page: &Page{Limit: &test.limit}}
			result, panicked := callContinuityService(service, t, request)
			if panicked {
				t.Fatal("Handle(workstream.list) panicked")
			}
			if result.Outcome != test.want {
				t.Fatalf("limit %d outcome = %q, want %q", test.limit, result.Outcome, test.want)
			}
		})
	}
}

func callContinuityService(service ContinuityService, t *testing.T, request Request) (result Result, panicked bool) {
	t.Helper()
	defer func() {
		if recover() != nil {
			panicked = true
		}
	}()
	result = service.Handle(t.Context(), request)
	return result, false
}

func continuityRequest(t *testing.T, operation string, scope Scope, input string) Request {
	t.Helper()
	return Request{ContractVersion: ContractVersion, OperationID: "op-" + operation, Operation: operation, Scope: scope, Input: json.RawMessage(input)}
}
