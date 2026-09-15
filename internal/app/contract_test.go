package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"memgraphai/internal/store/sqlite"
	"memgraphai/internal/telemetry"
	"memgraphai/internal/testkit"
)

type recorderCapture struct {
	operation telemetry.Operation
	err       error
	calls     int
}

func (r *recorderCapture) Record(_ context.Context, operation telemetry.Operation) error {
	r.calls++
	r.operation = operation
	return r.err
}

func TestExecuteJSONRejectsUnknownFieldsAndUnsupportedVersionsBeforeBusinessBehavior(t *testing.T) {
	for _, test := range []struct {
		name string
		raw  string
		want string
	}{
		{"unknown envelope field", `{"contract_version":"memgraphai.experimental/v1alpha1","operation_id":"op-1","operation":"project.create","scope":{"kind":"library"},"input":{},"unexpected":true}`, Invalid},
		{"unknown nested scope field", `{"contract_version":"memgraphai.experimental/v1alpha1","operation_id":"op-2","operation":"project.create","scope":{"kind":"library","unexpected":true},"input":{}}`, Invalid},
		{"unsupported version", `{"contract_version":"memgraphai.experimental/v9","operation_id":"op-3","operation":"project.create","scope":{"kind":"library"},"input":{}}`, UnsupportedContractVersion},
	} {
		t.Run(test.name, func(t *testing.T) {
			called := false
			_, response := Service{}.ExecuteJSON(t.Context(), []byte(test.raw), "cli", func(context.Context, Request) Result {
				called = true
				return Result{Outcome: OK}
			})
			if called {
				t.Fatal("handler ran for rejected request")
			}
			if response.Outcome.Code != test.want {
				t.Fatalf("outcome = %q, want %q", response.Outcome.Code, test.want)
			}
		})
	}
}

func TestExecuteJSONRejectsMismatchedScopeAndInvalidPageLimits(t *testing.T) {
	for _, raw := range []string{
		`{"contract_version":"memgraphai.experimental/v1alpha1","operation_id":"op-scope","operation":"project.create","scope":{"kind":"project","project_id":"project-a","session_id":"session-a"},"input":{}}`,
		`{"contract_version":"memgraphai.experimental/v1alpha1","operation_id":"op-zero","operation":"project.list","scope":{"kind":"library"},"input":{},"page":{"limit":0}}`,
		`{"contract_version":"memgraphai.experimental/v1alpha1","operation_id":"op-large","operation":"project.list","scope":{"kind":"library"},"input":{},"page":{"limit":201}}`,
	} {
		_, response := Service{}.ExecuteJSON(t.Context(), []byte(raw), "mcp", func(context.Context, Request) Result {
			t.Fatal("handler ran for invalid request")
			return Result{}
		})
		if response.Outcome.Code != Invalid {
			t.Fatalf("outcome = %q, want %q", response.Outcome.Code, Invalid)
		}
	}
}

func TestExecuteJSONNilHandlerReturnsInternalAndAttemptsMetrics(t *testing.T) {
	recorder := &recorderCapture{}
	serialized, response := (Service{Recorder: recorder}).ExecuteJSON(t.Context(), []byte(`{"contract_version":"memgraphai.experimental/v1alpha1","operation_id":"op-nil-handler","operation":"project.create","scope":{"kind":"library"},"input":{}}`), "cli", nil)
	if response.Outcome.Code != Internal {
		t.Fatalf("outcome = %q, want %q", response.Outcome.Code, Internal)
	}
	if recorder.calls != 1 || recorder.operation.Outcome != Internal || recorder.operation.ResponseBytes != len(serialized) {
		t.Fatalf("metric = %#v, calls = %d; want one internal response metric", recorder.operation, recorder.calls)
	}
}

func TestExecuteJSONTriangulatesEnvelopeEdges(t *testing.T) {
	for _, test := range []struct {
		name, raw, want string
		called          bool
		echoID          string
	}{
		{"absent input", `{"contract_version":"memgraphai.experimental/v1alpha1","operation_id":"op-no-input","operation":"project.create","scope":{"kind":"library"}}`, Invalid, false, "op-no-input"},
		{"empty object input", `{"contract_version":"memgraphai.experimental/v1alpha1","operation_id":"op-empty-input","operation":"project.create","scope":{"kind":"library"},"input":{}}`, OK, true, "op-empty-input"},
		{"missing operation ID", `{"contract_version":"memgraphai.experimental/v1alpha1","operation":"project.create","scope":{"kind":"library"},"input":{}}`, Invalid, false, ""},
		{"blank operation ID", `{"contract_version":"memgraphai.experimental/v1alpha1","operation_id":" ","operation":"project.create","scope":{"kind":"library"},"input":{}}`, Invalid, false, ""},
		{"unsupported version uses server envelope", `{"contract_version":"memgraphai.experimental/v9","operation_id":"op-version","operation":"project.create","scope":{"kind":"library"},"input":{}}`, UnsupportedContractVersion, false, "op-version"},
		{"unknown page field", `{"contract_version":"memgraphai.experimental/v1alpha1","operation_id":"op-page","operation":"project.list","scope":{"kind":"library"},"input":{},"page":{"limit":1,"unexpected":true}}`, Invalid, false, "op-page"},
	} {
		t.Run(test.name, func(t *testing.T) {
			called := false
			_, response := Service{}.ExecuteJSON(t.Context(), []byte(test.raw), "mcp", func(context.Context, Request) Result {
				called = true
				return Result{Outcome: OK}
			})
			if response.Outcome.Code != test.want || called != test.called {
				t.Fatalf("outcome/called = %q/%t, want %q/%t", response.Outcome.Code, called, test.want, test.called)
			}
			if response.ContractVersion != ContractVersion || (response.OperationID == nil) != (test.echoID == "") {
				t.Fatalf("envelope = %#v, want server version and echoed ID=%q", response, test.echoID)
			}
			if test.echoID != "" && *response.OperationID != test.echoID {
				t.Fatalf("operation ID = %q, want echo %q", *response.OperationID, test.echoID)
			}
		})
	}
}

func TestExecuteJSONBoundsOversizedSuccessfulResponses(t *testing.T) {
	recorder := &recorderCapture{}
	oversized, err := json.Marshal(map[string]string{"payload": strings.Repeat("x", (2<<20)+1)})
	if err != nil {
		t.Fatalf("marshal oversized fixture: %v", err)
	}
	called := false
	serialized, response := (Service{Recorder: recorder}).ExecuteJSON(t.Context(), []byte(`{"contract_version":"memgraphai.experimental/v1alpha1","operation_id":"op-oversized","operation":"project.list","scope":{"kind":"library"},"input":{}}`), "mcp", func(context.Context, Request) Result {
		called = true
		count := 1
		return Result{Outcome: OK, Value: oversized, ResultCount: &count}
	})
	if !called {
		t.Fatal("handler did not run for valid bounded request")
	}
	if response.Outcome.Code != Internal || response.Result != nil || response.MetricResultCount != nil {
		t.Fatalf("oversized response outcome/result-bytes/count = %q/%d/%v, want internal/0/nil", response.Outcome.Code, len(response.Result), response.MetricResultCount)
	}
	if response.OperationID == nil || *response.OperationID != "op-oversized" {
		t.Fatalf("oversized response operation ID = %#v, want correlated op-oversized", response.OperationID)
	}
	if len(serialized) > 2<<20 {
		t.Fatalf("serialized response bytes = %d, want at most %d", len(serialized), 2<<20)
	}
	if recorder.calls != 1 || recorder.operation.Outcome != Internal || recorder.operation.ResponseBytes != len(serialized) || recorder.operation.ResultCount != nil {
		t.Fatalf("oversized metric = %#v, calls = %d; want one exact bounded internal metric", recorder.operation, recorder.calls)
	}
}

func TestExecuteJSONResponseBoundPreservesOrdinarySuccessAndContainsMalformedHandlerOutput(t *testing.T) {
	request := []byte(`{"contract_version":"memgraphai.experimental/v1alpha1","operation_id":"op-triangulate","operation":"project.list","scope":{"kind":"library"},"input":{}}`)
	t.Run("ordinary success", func(t *testing.T) {
		count := 1
		serialized, response := (Service{}).ExecuteJSON(t.Context(), request, "cli", func(context.Context, Request) Result {
			return Result{Outcome: OK, Value: json.RawMessage(`{"value":"bounded"}`), ResultCount: &count}
		})
		if response.Outcome.Code != OK || string(response.Result) != `{"value":"bounded"}` || response.MetricResultCount == nil || *response.MetricResultCount != 1 {
			t.Fatalf("ordinary response outcome/result/count = %q/%s/%v, want ok/bounded/1", response.Outcome.Code, response.Result, response.MetricResultCount)
		}
		if len(serialized) > 2<<20 {
			t.Fatalf("ordinary serialized response bytes = %d, want at most %d", len(serialized), 2<<20)
		}
	})
	t.Run("malformed handler output", func(t *testing.T) {
		serialized, response := (Service{}).ExecuteJSON(t.Context(), request, "mcp", func(context.Context, Request) Result {
			return Result{Outcome: OK, Value: json.RawMessage(`{"broken"`)}
		})
		if response.Outcome.Code != Internal || response.Result != nil || response.OperationID == nil || *response.OperationID != "op-triangulate" {
			t.Fatalf("malformed response outcome/result-bytes/operation-id = %q/%d/%v, want internal/0/op-triangulate", response.Outcome.Code, len(response.Result), response.OperationID)
		}
		if len(serialized) > 2<<20 || !json.Valid(serialized) {
			t.Fatalf("malformed fallback bytes/valid = %d/%t, want bounded valid JSON", len(serialized), json.Valid(serialized))
		}
	})
}

func TestProjectAndContinuityContentionReturnStableBusy(t *testing.T) {
	database := testkit.TempSQLitePath(t, "cross-service-contention")
	store, err := sqlite.Open(t.Context(), database)
	if err != nil {
		t.Fatalf("open service store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.CreateProject(t.Context(), "project-a", "Alpha"); err != nil {
		t.Fatalf("create contention fixture project: %v", err)
	}

	lock := func(t *testing.T) (*sql.DB, *sql.Tx) {
		t.Helper()
		db, err := sql.Open("sqlite", database+"?_pragma=busy_timeout(100)&_pragma=foreign_keys(1)")
		if err != nil {
			t.Fatalf("open lock connection: %v", err)
		}
		tx, err := db.BeginTx(t.Context(), nil)
		if err != nil {
			_ = db.Close()
			t.Fatalf("begin lock transaction: %v", err)
		}
		if _, err := tx.ExecContext(t.Context(), "UPDATE project_views SET generation = generation WHERE singleton = 1"); err != nil {
			_ = tx.Rollback()
			_ = db.Close()
			t.Fatalf("acquire write lock: %v", err)
		}
		return db, tx
	}

	for _, test := range []struct {
		name string
		run  func() Result
	}{
		{"project write", func() Result {
			return (ProjectService{Store: store}).Handle(t.Context(), Request{Operation: "project.create", Scope: Scope{Kind: "library"}, Input: json.RawMessage(`{"project_id":"project-b","name":"Beta"}`)})
		}},
		{"continuity write", func() Result {
			return (ContinuityService{Store: store}).Handle(t.Context(), Request{Operation: "workstream.create", Scope: Scope{Kind: "project", ProjectID: "project-a"}, Input: json.RawMessage(`{"workstream_id":"stream-a","origin":"hardening"}`)})
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			db, tx := lock(t)
			started := time.Now()
			result := test.run()
			elapsed := time.Since(started)
			if err := tx.Rollback(); err != nil {
				t.Fatalf("release write lock: %v", err)
			}
			if err := db.Close(); err != nil {
				t.Fatalf("close lock connection: %v", err)
			}
			if result.Outcome != Busy {
				t.Fatalf("contention outcome = %q, want %q", result.Outcome, Busy)
			}
			if elapsed > time.Second {
				t.Fatalf("contention elapsed = %s, want bounded under 1s", elapsed)
			}
		})
	}
}

func TestExecuteJSONDefaultsListLimitAndIsolatesRecorderFailure(t *testing.T) {
	recorder := &recorderCapture{err: errors.New("recorder unavailable")}
	clock := func() time.Time { return time.Date(2026, 3, 1, 1, 2, 3, 0, time.UTC) }
	serialized, response := (Service{Recorder: recorder, Now: clock}).ExecuteJSON(t.Context(), []byte(`{"contract_version":"memgraphai.experimental/v1alpha1","operation_id":"op-metric","operation":"project.list","scope":{"kind":"project","project_id":"project-a"},"input":{}}`), "cli", func(_ context.Context, request Request) Result {
		if request.Page == nil || request.Page.Limit == nil || *request.Page.Limit != 50 {
			t.Fatalf("normalized page = %#v, want default limit 50", request.Page)
		}
		count := 2
		return Result{Outcome: OK, Value: json.RawMessage(`{"projects":["a","b"]}`), ResultCount: &count}
	})
	if response.Outcome.Code != OK {
		t.Fatalf("outcome = %q, want %q after recorder failure", response.Outcome.Code, OK)
	}
	if recorder.calls != 1 || recorder.operation.ResponseBytes != len(serialized) || recorder.operation.BackendDuration < 0 {
		t.Fatalf("metric = %#v, calls = %d; want one exact-byte nonnegative-duration attempt", recorder.operation, recorder.calls)
	}
	if recorder.operation.ResultCount == nil || *recorder.operation.ResultCount != 2 || recorder.operation.ProjectID != "project-a" {
		t.Fatalf("metric scope/count = %#v, want project-a and 2", recorder.operation)
	}
}

func TestExecuteJSONIgnoresProductionRecorderFailure(t *testing.T) {
	database := testkit.TempSQLitePath(t, "duplicate-metric-isolation")
	store, err := sqlite.Open(t.Context(), database)
	if err != nil {
		t.Fatalf("open production recorder: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	request := []byte(`{"contract_version":"memgraphai.experimental/v1alpha1","operation_id":"op-duplicate-metric","operation":"project.list","scope":{"kind":"library"},"input":{}}`)
	handlerCalls := 0
	handler := func(context.Context, Request) Result {
		handlerCalls++
		count := 1
		return Result{Outcome: OK, Value: json.RawMessage(`{"projects":["alpha"]}`), ResultCount: &count}
	}
	service := Service{Recorder: store}

	firstBytes, first := service.ExecuteJSON(t.Context(), request, "cli", handler)
	secondBytes, second := service.ExecuteJSON(t.Context(), request, "cli", handler)
	if first.Outcome.Code != OK || second.Outcome.Code != OK {
		t.Fatalf("first/second outcome = %q/%q, want ok/ok", first.Outcome.Code, second.Outcome.Code)
	}
	if handlerCalls != 2 {
		t.Fatalf("handler calls = %d, want 2", handlerCalls)
	}
	if string(secondBytes) != string(firstBytes) || string(second.Result) != string(first.Result) {
		t.Fatalf("second response changed after duplicate metric: bytes=%s result=%s", secondBytes, second.Result)
	}
	if second.OperationID == nil || *second.OperationID != "op-duplicate-metric" || second.MetricResultCount == nil || *second.MetricResultCount != 1 {
		t.Fatalf("second response correlation/count = %#v/%v, want op-duplicate-metric/1", second.OperationID, second.MetricResultCount)
	}

	db, err := sql.Open("sqlite", database)
	if err != nil {
		t.Fatalf("open metric verification connection: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	var persisted int
	if err := db.QueryRowContext(t.Context(), "SELECT count(*) FROM operation_metrics WHERE operation_id = ?", "op-duplicate-metric").Scan(&persisted); err != nil {
		t.Fatalf("read persisted production metric: %v", err)
	}
	if persisted != 1 {
		t.Fatalf("persisted production metrics = %d, want 1 after duplicate-ID rejection", persisted)
	}
}
