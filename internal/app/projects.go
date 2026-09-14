package app

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"

	"memgraphai/internal/domain"
)

// ProjectStore is the narrow concrete persistence surface needed by P1.2.
type ProjectStore interface {
	CreateProject(context.Context, string, string) error
	VisitProjects(context.Context, string, int, func(string, string)) (int64, error)
	AssociatePath(context.Context, string, string) error
	VisitProjectPaths(context.Context, string, string, int, func(string)) (int64, error)
	RemoveProjectPath(context.Context, string, string) error
	ResolvePath(context.Context, string) (domain.Resolution, error)
}

// ProjectService owns validation and transport-neutral project behavior.
type ProjectService struct{ Store ProjectStore }

// Handle executes only project operations with the explicitly required scope.
func (s ProjectService) Handle(ctx context.Context, request Request) Result {
	if s.Store == nil {
		return Result{Outcome: Internal}
	}
	switch request.Operation {
	case "project.create":
		return s.create(ctx, request)
	case "project.list":
		return s.list(ctx, request)
	case "project.resolve":
		return s.resolve(ctx, request)
	case "project.association.add":
		return s.addAssociation(ctx, request)
	case "project.association.list":
		return s.listAssociations(ctx, request)
	case "project.association.remove":
		return s.removeAssociation(ctx, request)
	default:
		return Result{Outcome: Invalid}
	}
}

func (s ProjectService) create(ctx context.Context, request Request) Result {
	if request.Scope.Kind != "library" {
		return Result{Outcome: ScopeDenied}
	}
	var input struct {
		ProjectID string `json:"project_id"`
		Name      string `json:"name"`
	}
	if err := decodeProjectInput(request.Input, &input); err != nil || !validID(input.ProjectID) || !validID(input.Name) {
		return Result{Outcome: Invalid}
	}
	if err := s.Store.CreateProject(ctx, input.ProjectID, input.Name); err != nil {
		return Result{Outcome: projectErrorOutcome(err)}
	}
	return projectResult(map[string]string{"project_id": input.ProjectID, "name": input.Name}, 1)
}

func (s ProjectService) list(ctx context.Context, request Request) Result {
	if request.Scope.Kind != "library" || request.Page == nil || request.Page.Limit == nil {
		return Result{Outcome: ScopeDenied}
	}
	if err := decodeProjectInput(request.Input, &struct{}{}); err != nil {
		return Result{Outcome: Invalid}
	}
	cursor := ""
	if request.Page.Token != "" {
		decoded, ok := decodeProjectPageToken(request.Page.Token, request)
		if !ok {
			return Result{Outcome: Invalid}
		}
		cursor = decoded.Cursor
	}
	projects := make([]map[string]string, 0, *request.Page.Limit+1)
	generation, err := s.Store.VisitProjects(ctx, cursor, *request.Page.Limit+1, func(id, name string) {
		projects = append(projects, map[string]string{"project_id": id, "name": name})
	})
	if err != nil {
		return Result{Outcome: projectErrorOutcome(err)}
	}
	if request.Page.Token != "" {
		decoded, _ := decodeProjectPageToken(request.Page.Token, request)
		if decoded.ViewRevision != generation {
			return Result{Outcome: Conflict}
		}
	}
	result := projectResult(map[string]any{"projects": projects}, len(projects))
	if len(projects) <= *request.Page.Limit {
		return result
	}
	projects = projects[:*request.Page.Limit]
	result.Value, _ = json.Marshal(map[string]any{"projects": projects})
	result.ResultCount = intPointer(len(projects))
	result.Page = &ResponsePage{NextToken: encodeProjectPageToken(request, projects[len(projects)-1]["project_id"], generation)}
	return result
}

func (s ProjectService) resolve(ctx context.Context, request Request) Result {
	if request.Scope.Kind != "library" {
		return Result{Outcome: ScopeDenied}
	}
	var input pathInput
	if err := decodeProjectInput(request.Input, &input); err != nil || !validID(input.Path) {
		return Result{Outcome: Invalid}
	}
	resolution, err := s.Store.ResolvePath(ctx, input.Path)
	if err != nil {
		return Result{Outcome: Internal}
	}
	switch resolution.Outcome {
	case domain.One:
		return projectResult(map[string]string{"project_id": string(resolution.Projects[0])}, 1)
	case domain.Many:
		return Result{Outcome: Ambiguous}
	case domain.Unavailable, domain.None:
		return Result{Outcome: NotFound}
	default:
		return Result{Outcome: Internal}
	}
}

func (s ProjectService) addAssociation(ctx context.Context, request Request) Result {
	if request.Scope.Kind != "project" || !validID(request.Scope.ProjectID) {
		return Result{Outcome: ScopeDenied}
	}
	var input pathInput
	if err := decodeProjectInput(request.Input, &input); err != nil || !validID(input.Path) {
		return Result{Outcome: Invalid}
	}
	if err := s.Store.AssociatePath(ctx, request.Scope.ProjectID, input.Path); err != nil {
		return Result{Outcome: projectErrorOutcome(err)}
	}
	return projectResult(map[string]string{"project_id": request.Scope.ProjectID, "path": input.Path}, 1)
}

func (s ProjectService) listAssociations(ctx context.Context, request Request) Result {
	if request.Scope.Kind != "project" || !validID(request.Scope.ProjectID) || request.Page == nil || request.Page.Limit == nil {
		return Result{Outcome: ScopeDenied}
	}
	if err := decodeProjectInput(request.Input, &struct{}{}); err != nil {
		return Result{Outcome: Invalid}
	}
	cursor := ""
	if request.Page.Token != "" {
		decoded, ok := decodeProjectPageToken(request.Page.Token, request)
		if !ok {
			return Result{Outcome: Invalid}
		}
		cursor = decoded.Cursor
	}
	paths := make([]string, 0, *request.Page.Limit+1)
	generation, err := s.Store.VisitProjectPaths(ctx, request.Scope.ProjectID, cursor, *request.Page.Limit+1, func(path string) {
		paths = append(paths, path)
	})
	if err != nil {
		return Result{Outcome: projectErrorOutcome(err)}
	}
	if request.Page.Token != "" {
		decoded, _ := decodeProjectPageToken(request.Page.Token, request)
		if decoded.ViewRevision != generation {
			return Result{Outcome: Conflict}
		}
	}
	result := projectResult(map[string]any{"paths": paths}, len(paths))
	if len(paths) <= *request.Page.Limit {
		return result
	}
	paths = paths[:*request.Page.Limit]
	result.Value, _ = json.Marshal(map[string]any{"paths": paths})
	result.ResultCount = intPointer(len(paths))
	result.Page = &ResponsePage{NextToken: encodeProjectPageToken(request, paths[len(paths)-1], generation)}
	return result
}

func (s ProjectService) removeAssociation(ctx context.Context, request Request) Result {
	if request.Scope.Kind != "project" || !validID(request.Scope.ProjectID) {
		return Result{Outcome: ScopeDenied}
	}
	var input pathInput
	if err := decodeProjectInput(request.Input, &input); err != nil || !validID(input.Path) {
		return Result{Outcome: Invalid}
	}
	if err := s.Store.RemoveProjectPath(ctx, request.Scope.ProjectID, input.Path); err != nil {
		return Result{Outcome: projectErrorOutcome(err)}
	}
	return projectResult(map[string]string{"project_id": request.Scope.ProjectID, "path": input.Path}, 1)
}

type pathInput struct {
	Path string `json:"path"`
}

type codedProjectError interface{ OutcomeCode() string }

func projectErrorOutcome(err error) string {
	if coded, ok := err.(codedProjectError); ok {
		switch coded.OutcomeCode() {
		case Conflict, NotFound:
			return coded.OutcomeCode()
		}
	}
	return Internal
}

func projectResult(value any, count int) Result {
	encoded, err := json.Marshal(value)
	if err != nil {
		return Result{Outcome: Internal}
	}
	return Result{Outcome: OK, Value: encoded, ResultCount: &count}
}

type projectPageToken struct {
	V            string `json:"v"`
	Operation    string `json:"operation"`
	ScopeDigest  string `json:"scope_digest"`
	FilterDigest string `json:"filter_digest"`
	Order        string `json:"order"`
	Limit        int    `json:"limit"`
	Cursor       string `json:"cursor"`
	ViewRevision int64  `json:"view_revision"`
}

func encodeProjectPageToken(request Request, cursor string, viewRevision int64) string {
	encoded, _ := json.Marshal(projectPageToken{
		V: ContractVersion, Operation: request.Operation, ScopeDigest: pageDigest(request.Scope),
		FilterDigest: pageDigest(struct{}{}), Order: projectPageOrder(request.Operation), Limit: *request.Page.Limit,
		Cursor: cursor, ViewRevision: viewRevision,
	})
	return base64.RawURLEncoding.EncodeToString(encoded)
}

func decodeProjectPageToken(encoded string, request Request) (projectPageToken, bool) {
	bytes, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return projectPageToken{}, false
	}
	var token projectPageToken
	if err := decodeStrict(bytes, &token); err != nil || token.V != ContractVersion ||
		token.Operation != request.Operation || token.ScopeDigest != pageDigest(request.Scope) ||
		token.FilterDigest != pageDigest(struct{}{}) || token.Order != projectPageOrder(request.Operation) ||
		token.Limit != *request.Page.Limit || !validID(token.Cursor) || token.ViewRevision < 0 {
		return projectPageToken{}, false
	}
	return token, true
}

func projectPageOrder(operation string) string {
	if operation == "project.association.list" {
		return "supplied_path:asc"
	}
	return "project_id:asc"
}

func pageDigest(value any) string {
	encoded, _ := json.Marshal(value)
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}

func intPointer(value int) *int { return &value }

func decodeProjectInput(input json.RawMessage, target any) error { return decodeStrict(input, target) }
