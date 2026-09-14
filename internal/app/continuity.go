package app

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"time"

	"memgraphai/internal/domain"
)

// ContinuityStore is the narrow persistence surface for P1.3 continuity operations.
type ContinuityStore interface {
	CreateContinuityWorkstream(context.Context, string, string, string, string, string, time.Time) error
	VisitWorkstreams(context.Context, string, string, int, func(string, string, string, string, string, time.Time)) (int64, error)
	ForkWorkstream(context.Context, string, string, string, string, string, string, time.Time) error
	OpenSession(context.Context, string, string, string, string, string, string, time.Time) error
	SessionStatus(context.Context, string, string, string) (string, string, string, string, string, time.Time, error)
	CloseSession(context.Context, string, string, string, time.Time) error
	DisconnectSession(context.Context, string, string, string, string, time.Time) error
	ResumeSession(context.Context, string, string, string, string, string, string, string, time.Time) error
	ValidateActiveBinding(context.Context, string, string, string) error
}

// ContinuityService owns explicit workstream and session behavior without adapter state.
type ContinuityService struct {
	Store ContinuityStore
	Now   func() time.Time
}

// Handle accepts only continuity operations with an explicit declared scope.
func (s ContinuityService) Handle(ctx context.Context, request Request) Result {
	if s.Store == nil {
		return Result{Outcome: Internal}
	}
	switch request.Operation {
	case "workstream.create":
		return s.createWorkstream(ctx, request)
	case "workstream.list":
		return s.listWorkstreams(ctx, request)
	case "workstream.fork":
		return s.forkWorkstream(ctx, request)
	case "session.open":
		return s.openSession(ctx, request)
	case "session.status":
		return s.sessionStatus(ctx, request)
	case "session.close":
		return s.closeSession(ctx, request)
	case "session.disconnect":
		return s.disconnectSession(ctx, request)
	case "session.resume":
		return s.resumeSession(ctx, request)
	default:
		return Result{Outcome: Invalid}
	}
}

type continuityProvenanceInput struct {
	Origin string `json:"origin"`
	Client string `json:"client"`
	Model  string `json:"model"`
}

type createWorkstreamInput struct {
	WorkstreamID string `json:"workstream_id"`
	continuityProvenanceInput
}

func (s ContinuityService) createWorkstream(ctx context.Context, request Request) Result {
	if request.Scope.Kind != "project" {
		return Result{Outcome: ScopeDenied}
	}
	var input createWorkstreamInput
	if err := decodeStrict(request.Input, &input); err != nil || !validID(input.WorkstreamID) || !validID(input.Origin) {
		return Result{Outcome: Invalid}
	}
	if err := s.Store.CreateContinuityWorkstream(ctx, request.Scope.ProjectID, input.WorkstreamID, input.Origin, input.Client, input.Model, s.now()()); err != nil {
		return Result{Outcome: continuityOutcome(err)}
	}
	return continuityResult(map[string]string{"workstream_id": input.WorkstreamID, "project_id": request.Scope.ProjectID, "origin": input.Origin}, 1)
}

func (s ContinuityService) listWorkstreams(ctx context.Context, request Request) Result {
	if request.Scope.Kind != "project" {
		return Result{Outcome: ScopeDenied}
	}
	limit := 50
	page := Page{Limit: &limit}
	if request.Page != nil {
		page = *request.Page
		if page.Limit != nil {
			limit = *page.Limit
		}
		page.Limit = &limit
	}
	if limit < 1 || limit > 200 {
		return Result{Outcome: Invalid}
	}
	request.Page = &page
	if err := decodeStrict(request.Input, &struct{}{}); err != nil {
		return Result{Outcome: Invalid}
	}
	cursor := ""
	if request.Page.Token != "" {
		token, ok := decodeContinuityPageToken(request.Page.Token, request)
		if !ok {
			return Result{Outcome: Invalid}
		}
		cursor = token.Cursor
	}
	workstreams := make([]map[string]string, 0, *request.Page.Limit+1)
	generation, err := s.Store.VisitWorkstreams(ctx, request.Scope.ProjectID, cursor, *request.Page.Limit+1, func(id, source, origin, client, model string, createdAt time.Time) {
		metadata := map[string]string{"workstream_id": id, "origin": origin}
		if source != "" {
			metadata["forked_from_workstream_id"] = source
		}
		if client != "" {
			metadata["client"] = client
		}
		if model != "" {
			metadata["model"] = model
		}
		if !createdAt.IsZero() {
			metadata["created_at"] = createdAt.UTC().Format(time.RFC3339Nano)
		}
		workstreams = append(workstreams, metadata)
	})
	if err != nil {
		return Result{Outcome: continuityOutcome(err)}
	}
	if request.Page.Token != "" {
		token, _ := decodeContinuityPageToken(request.Page.Token, request)
		if token.ViewRevision != generation {
			return Result{Outcome: Conflict}
		}
	}
	if len(workstreams) <= *request.Page.Limit {
		return continuityResult(map[string]any{"workstreams": workstreams}, len(workstreams))
	}
	workstreams = workstreams[:*request.Page.Limit]
	result := continuityResult(map[string]any{"workstreams": workstreams}, len(workstreams))
	result.Page = &ResponsePage{NextToken: encodeContinuityPageToken(request, workstreams[len(workstreams)-1]["workstream_id"], generation)}
	return result
}

func (s ContinuityService) forkWorkstream(ctx context.Context, request Request) Result {
	if request.Scope.Kind != "workstream" {
		return Result{Outcome: ScopeDenied}
	}
	var input createWorkstreamInput
	if err := decodeStrict(request.Input, &input); err != nil || !validID(input.WorkstreamID) || !validID(input.Origin) {
		return Result{Outcome: Invalid}
	}
	if err := s.Store.ForkWorkstream(ctx, request.Scope.ProjectID, request.Scope.WorkstreamID, input.WorkstreamID, input.Origin, input.Client, input.Model, s.now()()); err != nil {
		return Result{Outcome: continuityOutcome(err)}
	}
	return continuityResult(map[string]string{"workstream_id": input.WorkstreamID, "project_id": request.Scope.ProjectID, "forked_from_workstream_id": request.Scope.WorkstreamID}, 1)
}

type sessionInput struct {
	SessionID       string `json:"session_id"`
	SourceSessionID string `json:"source_session_id"`
	continuityProvenanceInput
}

func (s ContinuityService) openSession(ctx context.Context, request Request) Result {
	if request.Scope.Kind != "workstream" {
		return Result{Outcome: ScopeDenied}
	}
	var input sessionInput
	if err := decodeStrict(request.Input, &input); err != nil || !validID(input.SessionID) || !validID(input.Origin) {
		return Result{Outcome: Invalid}
	}
	if err := s.Store.OpenSession(ctx, request.Scope.ProjectID, request.Scope.WorkstreamID, input.SessionID, input.Origin, input.Client, input.Model, s.now()()); err != nil {
		return Result{Outcome: continuityOutcome(err)}
	}
	return continuityResult(map[string]string{"session_id": input.SessionID, "project_id": request.Scope.ProjectID, "workstream_id": request.Scope.WorkstreamID, "status": "open"}, 1)
}

func (s ContinuityService) sessionStatus(ctx context.Context, request Request) Result {
	if request.Scope.Kind != "session" {
		return Result{Outcome: ScopeDenied}
	}
	if err := decodeStrict(request.Input, &struct{}{}); err != nil {
		return Result{Outcome: Invalid}
	}
	status, source, origin, client, model, openedAt, err := s.Store.SessionStatus(ctx, request.Scope.ProjectID, request.Scope.WorkstreamID, request.Scope.SessionID)
	if err != nil {
		return Result{Outcome: continuityOutcome(err)}
	}
	value := map[string]string{"session_id": request.Scope.SessionID, "project_id": request.Scope.ProjectID,
		"workstream_id": request.Scope.WorkstreamID, "status": status, "origin": origin}
	if source != "" {
		value["resumed_from_session_id"] = source
	}
	if client != "" {
		value["client"] = client
	}
	if model != "" {
		value["model"] = model
	}
	if !openedAt.IsZero() {
		value["opened_at"] = openedAt.UTC().Format(time.RFC3339Nano)
	}
	return continuityResult(value, 1)
}

func (s ContinuityService) closeSession(ctx context.Context, request Request) Result {
	if request.Scope.Kind != "session" {
		return Result{Outcome: ScopeDenied}
	}
	if err := decodeStrict(request.Input, &struct{}{}); err != nil {
		return Result{Outcome: Invalid}
	}
	if err := s.Store.CloseSession(ctx, request.Scope.ProjectID, request.Scope.WorkstreamID, request.Scope.SessionID, s.now()()); err != nil {
		return Result{Outcome: continuityOutcome(err)}
	}
	return continuityResult(map[string]string{"session_id": request.Scope.SessionID, "status": "closed"}, 1)
}

func (s ContinuityService) disconnectSession(ctx context.Context, request Request) Result {
	if request.Scope.Kind != "session" {
		return Result{Outcome: ScopeDenied}
	}
	var input struct {
		ObservedBy string `json:"observed_by"`
	}
	if err := decodeStrict(request.Input, &input); err != nil || !validID(input.ObservedBy) {
		return Result{Outcome: Invalid}
	}
	if err := s.Store.DisconnectSession(ctx, request.Scope.ProjectID, request.Scope.WorkstreamID, request.Scope.SessionID, input.ObservedBy, s.now()()); err != nil {
		return Result{Outcome: continuityOutcome(err)}
	}
	return continuityResult(map[string]string{"session_id": request.Scope.SessionID, "status": "disconnected"}, 1)
}

func (s ContinuityService) resumeSession(ctx context.Context, request Request) Result {
	if request.Scope.Kind != "workstream" {
		return Result{Outcome: ScopeDenied}
	}
	var input sessionInput
	if err := decodeStrict(request.Input, &input); err != nil || !validID(input.SessionID) || !validID(input.SourceSessionID) || !validID(input.Origin) {
		return Result{Outcome: Invalid}
	}
	if err := s.Store.ResumeSession(ctx, request.Scope.ProjectID, request.Scope.WorkstreamID, input.SessionID, input.SourceSessionID, input.Origin, input.Client, input.Model, s.now()()); err != nil {
		return Result{Outcome: continuityOutcome(err)}
	}
	return continuityResult(map[string]string{"session_id": input.SessionID, "project_id": request.Scope.ProjectID,
		"workstream_id": request.Scope.WorkstreamID, "resumed_from_session_id": input.SourceSessionID, "status": "open"}, 1)
}

type continuityPageToken struct {
	V            string `json:"v"`
	Operation    string `json:"operation"`
	ScopeDigest  string `json:"scope_digest"`
	FilterDigest string `json:"filter_digest"`
	Order        string `json:"order"`
	Limit        int    `json:"limit"`
	Cursor       string `json:"cursor"`
	ViewRevision int64  `json:"view_revision"`
}

func encodeContinuityPageToken(request Request, cursor string, viewRevision int64) string {
	encoded, _ := json.Marshal(continuityPageToken{V: ContractVersion, Operation: request.Operation,
		ScopeDigest: pageDigest(request.Scope), FilterDigest: pageDigest(struct{}{}), Order: "workstream_id:asc",
		Limit: *request.Page.Limit, Cursor: cursor, ViewRevision: viewRevision})
	return base64.RawURLEncoding.EncodeToString(encoded)
}

func decodeContinuityPageToken(encoded string, request Request) (continuityPageToken, bool) {
	decoded, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return continuityPageToken{}, false
	}
	var token continuityPageToken
	if err := decodeStrict(decoded, &token); err != nil || token.V != ContractVersion || token.Operation != request.Operation ||
		token.ScopeDigest != pageDigest(request.Scope) || token.FilterDigest != pageDigest(struct{}{}) || token.Order != "workstream_id:asc" ||
		token.Limit != *request.Page.Limit || !validID(token.Cursor) || token.ViewRevision < 0 {
		return continuityPageToken{}, false
	}
	return token, true
}

func (s ContinuityService) now() func() time.Time {
	if s.Now != nil {
		return s.Now
	}
	return time.Now
}

type codedContinuityError interface{ OutcomeCode() string }

func continuityOutcome(err error) string {
	if coded, ok := err.(codedContinuityError); ok {
		switch coded.OutcomeCode() {
		case NotFound, Conflict:
			return coded.OutcomeCode()
		}
	}
	if domain.IsOutcome(err, domain.BindingMismatch) {
		return BindingMismatch
	}
	return Internal
}

func continuityResult(value any, count int) Result {
	encoded, err := json.Marshal(value)
	if err != nil {
		return Result{Outcome: Internal}
	}
	return Result{Outcome: OK, Value: encoded, ResultCount: &count}
}
