// Package app owns transport-neutral Phase 1 request validation and outcomes.
package app

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"time"

	"memgraphai/internal/telemetry"
)

const ContractVersion = "memgraphai.experimental/v1alpha1"

const (
	OK                         = "ok"
	Invalid                    = "invalid"
	NotFound                   = "not_found"
	Ambiguous                  = "ambiguous"
	ScopeDenied                = "scope_denied"
	BindingMismatch            = "binding_mismatch"
	Conflict                   = "conflict"
	IntegrityDiscrepancy       = "integrity_discrepancy"
	Busy                       = "busy"
	Retryable                  = "retryable"
	Unknown                    = "unknown"
	IdempotencyMismatch        = "idempotency_mismatch"
	UnsupportedContractVersion = "unsupported_contract_version"
	Internal                   = "internal"
)

// Request is the experimental shared envelope before an operation-specific service runs.
type Request struct {
	ContractVersion string          `json:"contract_version"`
	OperationID     string          `json:"operation_id"`
	Operation       string          `json:"operation"`
	Scope           Scope           `json:"scope"`
	Input           json.RawMessage `json:"input"`
	Page            *Page           `json:"page,omitempty"`
}

// Scope names the complete caller-declared context; it never derives a default.
type Scope struct {
	Kind         string `json:"kind"`
	ProjectID    string `json:"project_id,omitempty"`
	WorkstreamID string `json:"workstream_id,omitempty"`
	SessionID    string `json:"session_id,omitempty"`
}

// Page is a bounded list/history request primitive. Token binding is implemented with
// the concrete list and history services that issue view revisions in later work.
type Page struct {
	Limit *int   `json:"limit,omitempty"`
	Token string `json:"token,omitempty"`
}

// Outcome is a stable, non-content response summary.
type Outcome struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Response is the exact experimental JSON result envelope emitted by this foundation.
type Response struct {
	ContractVersion string          `json:"contract_version"`
	OperationID     *string         `json:"operation_id"`
	Outcome         Outcome         `json:"outcome"`
	Result          json.RawMessage `json:"result,omitempty"`
}

// Result is the bounded transport-neutral value returned by a future concrete service.
type Result struct {
	Outcome     string
	Value       json.RawMessage
	ResultCount *int
}

// Handler is the minimal seam for concrete Phase 1 services added in later work units.
type Handler func(context.Context, Request) Result

// Service validates the shared envelope and records the actual serialized JSON response.
type Service struct {
	Recorder telemetry.Recorder
	Now      func() time.Time
}

// ExecuteJSON normalizes a request, invokes only valid business behavior, and returns
// the serialized alpha envelope whose exact byte count it records best-effort.
func (s Service) ExecuteJSON(ctx context.Context, raw []byte, iface string, handler Handler) ([]byte, Response) {
	started := s.now()()
	request, code := ParseRequest(raw)
	response := Response{ContractVersion: ContractVersion, Outcome: outcome(code)}
	if validID(request.OperationID) {
		operationID := request.OperationID
		response.OperationID = &operationID
	}
	var resultCount *int
	if code == "" {
		result := Result{Outcome: Internal}
		if handler != nil {
			result = handler(ctx, request)
			if result.Outcome == "" {
				result.Outcome = Internal
			}
		}
		response.Outcome = outcome(result.Outcome)
		if result.Outcome == OK {
			response.Result = result.Value
			resultCount = result.ResultCount
		}
	}
	serialized, err := json.Marshal(response)
	if err != nil {
		response = Response{ContractVersion: ContractVersion, Outcome: outcome(Internal)}
		serialized, _ = json.Marshal(response)
	}
	if s.Recorder != nil {
		_ = s.Recorder.Record(ctx, telemetry.Operation{
			OperationID:     request.OperationID,
			RecordedAt:      s.now()(),
			Interface:       iface,
			ScopeKind:       request.Scope.Kind,
			ProjectID:       request.Scope.ProjectID,
			WorkstreamID:    request.Scope.WorkstreamID,
			SessionID:       request.Scope.SessionID,
			Outcome:         response.Outcome.Code,
			BackendDuration: s.now()().Sub(started),
			ResponseBytes:   len(serialized),
			ResultCount:     resultCount,
		})
	}
	return serialized, response
}

// ParseRequest rejects unknown envelope fields and normalizes explicit scope/page bounds.
func ParseRequest(raw []byte) (Request, string) {
	var request Request
	if err := decodeStrict(raw, &request); err != nil {
		return request, Invalid
	}
	if request.ContractVersion == "" || !validID(request.OperationID) || !validID(request.Operation) || !validScope(request.Scope) || !validInput(request.Input) {
		return request, Invalid
	}
	if request.ContractVersion != ContractVersion {
		return request, UnsupportedContractVersion
	}
	if request.Page != nil {
		if !pageable(request.Operation) || request.Page.Limit == nil || *request.Page.Limit < 1 || *request.Page.Limit > 200 {
			return request, Invalid
		}
	} else if pageable(request.Operation) {
		limit := 50
		request.Page = &Page{Limit: &limit}
	}
	return request, ""
}

func (s *Scope) UnmarshalJSON(data []byte) error { return decodeStrict(data, (*scopeWire)(s)) }

type scopeWire Scope

func (p *Page) UnmarshalJSON(data []byte) error { return decodeStrict(data, (*pageWire)(p)) }

type pageWire Page

func decodeStrict(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return err
	}
	return nil
}

func validScope(scope Scope) bool {
	switch scope.Kind {
	case "library":
		return scope.ProjectID == "" && scope.WorkstreamID == "" && scope.SessionID == ""
	case "project":
		return validID(scope.ProjectID) && scope.WorkstreamID == "" && scope.SessionID == ""
	case "workstream":
		return validID(scope.ProjectID) && validID(scope.WorkstreamID) && scope.SessionID == ""
	case "session":
		return validID(scope.ProjectID) && validID(scope.WorkstreamID) && validID(scope.SessionID)
	default:
		return false
	}
}

func validInput(input json.RawMessage) bool {
	return len(input) > 0 && json.Valid(input) && bytes.HasPrefix(bytes.TrimSpace(input), []byte("{"))
}

func validID(value string) bool { return strings.TrimSpace(value) != "" }

func pageable(operation string) bool {
	switch operation {
	case "project.list", "workstream.list", "document.list", "document.history", "checkpoint.list":
		return true
	default:
		return false
	}
}

func (s Service) now() func() time.Time {
	if s.Now != nil {
		return s.Now
	}
	return time.Now
}

func outcome(code string) Outcome {
	messages := map[string]string{
		OK:                         "operation completed",
		Invalid:                    "invalid request",
		NotFound:                   "requested resource was not found",
		Ambiguous:                  "request is ambiguous",
		ScopeDenied:                "scope denied",
		BindingMismatch:            "binding mismatch",
		Conflict:                   "operation conflict",
		IntegrityDiscrepancy:       "integrity discrepancy",
		Busy:                       "operation is busy",
		Retryable:                  "operation may be retried",
		Unknown:                    "operation outcome is unknown",
		IdempotencyMismatch:        "operation identity mismatch",
		UnsupportedContractVersion: "unsupported contract version",
		Internal:                   "internal operation failure",
	}
	message, ok := messages[code]
	if !ok {
		return Outcome{Code: Invalid, Message: messages[Invalid]}
	}
	return Outcome{Code: code, Message: message}
}
