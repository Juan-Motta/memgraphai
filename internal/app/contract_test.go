package app

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"memgraphai/internal/telemetry"
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
