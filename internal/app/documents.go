package app

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strconv"
	"time"

	"memgraphai/internal/domain"
	"memgraphai/internal/revisionfs"
)

const (
	documentInputLimit    = 1 << 20
	documentResponseLimit = 2 << 20
)

// DocumentProvenance is immutable client-reported revision provenance.
type DocumentProvenance struct {
	Origin string `json:"origin"`
	Client string `json:"client,omitempty"`
	Model  string `json:"model,omitempty"`
}

// DocumentRevision is metadata and exact verified Markdown for one revision.
type DocumentRevision = revisionfs.DocumentRevision

// DocumentWrite is the immutable create or update request passed to persistence.
type DocumentWrite struct {
	OperationID      string
	DocumentID       string
	RevisionID       string
	ProjectID        string
	WorkstreamID     string
	ExpectedRevision *string
	Content          []byte
	Provenance       DocumentProvenance
	CreatedAt        time.Time
}

// DocumentWriteResult is the durable terminal write result.
type DocumentWriteResult struct {
	Outcome    string
	RevisionID string
}

// DocumentStore is the concrete document persistence boundary for P1.4a.
type DocumentStore interface {
	RegisterDocumentCreate(context.Context, string, string, string, string, string, *string, []byte, string, string, string, time.Time) error
	WriteDocument(context.Context, *revisionfs.Filesystem, string, string, string, string, string, *string, []byte, string, string, string, time.Time) (string, string, error)
	VisitDocuments(context.Context, string, string, string, int, func(revisionfs.DocumentRevision)) (int64, error)
	VisitDocumentHistory(context.Context, string, string, string, int64, int, func(revisionfs.DocumentRevision)) (int64, error)
	ReadDocument(context.Context, *revisionfs.Filesystem, string, string, string, *string) (revisionfs.DocumentRevision, []byte, error)
}

// DocumentService owns transport-neutral document behavior. Its implementation follows
// after behavioral RED tests establish the P1.4a service and store invariants.
type DocumentService struct {
	Store DocumentStore
	Files *revisionfs.Filesystem
	Now   func() time.Time
}

// Handle accepts only document operations within their declared project or workstream.
func (s DocumentService) Handle(ctx context.Context, request Request) Result {
	if s.Store == nil || s.Files == nil {
		return Result{Outcome: Internal}
	}
	switch request.Operation {
	case "document.create":
		return s.write(ctx, request, true)
	case "document.update":
		return s.write(ctx, request, false)
	case "document.list":
		return s.list(ctx, request)
	case "document.read":
		return s.read(ctx, request)
	case "document.history":
		return s.history(ctx, request)
	default:
		return Result{Outcome: Invalid}
	}
}

type documentWriteInput struct {
	DocumentID       string             `json:"document_id"`
	RevisionID       string             `json:"revision_id"`
	ExpectedRevision *string            `json:"expected_revision_id"`
	ContentBase64    string             `json:"content_base64"`
	Provenance       DocumentProvenance `json:"provenance"`
}

func (s DocumentService) write(ctx context.Context, request Request, create bool) Result {
	projectID, workstreamID, code := documentScope(request.Scope)
	if code != "" {
		return Result{Outcome: code}
	}
	var input documentWriteInput
	if err := decodeStrict(request.Input, &input); err != nil || !validDocumentID(input.DocumentID) || !validDocumentID(input.RevisionID) || !validDocumentProvenance(input.Provenance) {
		return Result{Outcome: Invalid}
	}
	var rawFields map[string]json.RawMessage
	if json.Unmarshal(request.Input, &rawFields) != nil {
		return Result{Outcome: Invalid}
	}
	expectedRaw, expectedPresent := rawFields["expected_revision_id"]
	if create && (!expectedPresent || string(expectedRaw) != "null") || !create && (!expectedPresent || input.ExpectedRevision == nil || !validDocumentID(*input.ExpectedRevision)) || input.RevisionID == dereference(input.ExpectedRevision) {
		return Result{Outcome: Invalid}
	}
	content, err := decodeDocumentContent(input.ContentBase64)
	if err != nil {
		return Result{Outcome: Invalid}
	}
	write := DocumentWrite{OperationID: request.OperationID, DocumentID: input.DocumentID, RevisionID: input.RevisionID,
		ProjectID: projectID, WorkstreamID: workstreamID, ExpectedRevision: input.ExpectedRevision, Content: content,
		Provenance: input.Provenance, CreatedAt: s.now()()}
	if create {
		if err := s.Store.RegisterDocumentCreate(ctx, write.OperationID, write.DocumentID, write.ProjectID, write.WorkstreamID, write.RevisionID, write.ExpectedRevision, write.Content, write.Provenance.Origin, write.Provenance.Client, write.Provenance.Model, write.CreatedAt); err != nil {
			return Result{Outcome: documentOutcome(err)}
		}
	}
	writtenOutcome, writtenRevision, err := s.Store.WriteDocument(ctx, s.Files, write.OperationID, write.DocumentID, write.ProjectID, write.WorkstreamID, write.RevisionID, write.ExpectedRevision, write.Content, write.Provenance.Origin, write.Provenance.Client, write.Provenance.Model, write.CreatedAt)
	if err != nil {
		return Result{Outcome: documentOutcome(err)}
	}
	if writtenOutcome != OK || writtenRevision != input.RevisionID {
		return Result{Outcome: writtenOutcome}
	}
	revision, _, err := s.Store.ReadDocument(ctx, s.Files, projectID, workstreamID, input.DocumentID, &writtenRevision)
	if err != nil {
		return Result{Outcome: documentOutcome(err)}
	}
	return documentMetadataResult(revision, false)
}

func (s DocumentService) list(ctx context.Context, request Request) Result {
	projectID, workstreamID, code := documentScope(request.Scope)
	if code != "" {
		return Result{Outcome: code}
	}
	if err := decodeStrict(request.Input, &struct{}{}); err != nil {
		return Result{Outcome: Invalid}
	}
	limit, token, code := documentPage(request, "document_id:asc", "")
	if code != "" {
		return Result{Outcome: code}
	}
	request.Page = &Page{Limit: &limit}
	items := make([]DocumentRevision, 0, limit+1)
	view, err := s.Store.VisitDocuments(ctx, projectID, workstreamID, token.Cursor, limit+1, func(item DocumentRevision) { items = append(items, item) })
	if err != nil {
		return Result{Outcome: documentOutcome(err)}
	}
	if token.hasView && token.ViewRevision != view {
		return Result{Outcome: Conflict}
	}
	more := len(items) > limit
	if more {
		items = items[:limit]
	}
	value := make([]map[string]any, 0, len(items))
	for _, item := range items {
		value = append(value, documentMetadata(item))
	}
	result := documentResult(map[string]any{"documents": value}, len(items))
	if more {
		result.Page = &ResponsePage{NextToken: encodeDocumentPage(request, "document_id:asc", "", items[len(items)-1].DocumentID, 0, view)}
	}
	return result
}

func (s DocumentService) read(ctx context.Context, request Request) Result {
	projectID, workstreamID, code := documentScope(request.Scope)
	if code != "" {
		return Result{Outcome: code}
	}
	var input struct {
		DocumentID string  `json:"document_id"`
		RevisionID *string `json:"revision_id"`
	}
	if err := decodeStrict(request.Input, &input); err != nil || !validDocumentID(input.DocumentID) || input.RevisionID != nil && !validDocumentID(*input.RevisionID) {
		return Result{Outcome: Invalid}
	}
	revision, content, err := s.Store.ReadDocument(ctx, s.Files, projectID, workstreamID, input.DocumentID, input.RevisionID)
	if err != nil {
		return Result{Outcome: documentOutcome(err)}
	}
	value := documentMetadata(revision)
	value["content_base64"] = base64.RawStdEncoding.EncodeToString(content)
	return documentResult(value, 1)
}

func (s DocumentService) history(ctx context.Context, request Request) Result {
	projectID, workstreamID, code := documentScope(request.Scope)
	if code != "" {
		return Result{Outcome: code}
	}
	var input struct {
		DocumentID string `json:"document_id"`
	}
	if err := decodeStrict(request.Input, &input); err != nil || !validDocumentID(input.DocumentID) {
		return Result{Outcome: Invalid}
	}
	limit, token, code := documentPage(request, "revision_rowid:desc", input.DocumentID)
	if code != "" {
		return Result{Outcome: code}
	}
	request.Page = &Page{Limit: &limit}
	afterRowID := int64(0)
	if token.Cursor != "" {
		afterRowID, _ = strconv.ParseInt(token.Cursor, 10, 64)
	}
	items := make([]DocumentRevision, 0, limit+1)
	view, err := s.Store.VisitDocumentHistory(ctx, projectID, workstreamID, input.DocumentID, afterRowID, limit+1, func(item DocumentRevision) { items = append(items, item) })
	if err != nil {
		return Result{Outcome: documentOutcome(err)}
	}
	if token.hasView && token.ViewRevision != view {
		return Result{Outcome: Conflict}
	}
	more := len(items) > limit
	if more {
		items = items[:limit]
	}
	value := make([]map[string]any, 0, len(items))
	for _, item := range items {
		value = append(value, documentMetadata(item))
	}
	result := documentResult(map[string]any{"revisions": value}, len(items))
	if more {
		result.Page = &ResponsePage{NextToken: encodeDocumentPage(request, "revision_rowid:desc", input.DocumentID, "", items[len(items)-1].Cursor, view)}
	}
	return result
}

type documentPageToken struct {
	V            string `json:"v"`
	Operation    string `json:"operation"`
	ScopeDigest  string `json:"scope_digest"`
	FilterDigest string `json:"filter_digest"`
	Order        string `json:"order"`
	Limit        int    `json:"limit"`
	Cursor       string `json:"cursor"`
	ViewRevision int64  `json:"view_revision"`
	hasView      bool
}

func documentPage(request Request, order, filter string) (int, documentPageToken, string) {
	limit := 50
	if request.Page != nil {
		if request.Page.Limit != nil {
			if *request.Page.Limit < 1 || *request.Page.Limit > 200 {
				return 0, documentPageToken{}, Invalid
			}
			limit = *request.Page.Limit
		} else if request.Page.Token != "" {
			return 0, documentPageToken{}, Invalid
		}
		if request.Page.Token != "" {
			decoded, err := base64.RawURLEncoding.DecodeString(request.Page.Token)
			var token documentPageToken
			if err != nil || decodeStrict(decoded, &token) != nil || token.V != ContractVersion || token.Operation != request.Operation || token.ScopeDigest != pageDigest(request.Scope) || token.FilterDigest != pageDigest(map[string]string{"document_id": filter}) || token.Order != order || token.Limit != limit || token.Cursor == "" || token.ViewRevision < 0 {
				return 0, documentPageToken{}, Invalid
			}
			if order == "revision_rowid:desc" {
				if row, err := strconv.ParseInt(token.Cursor, 10, 64); err != nil || row < 1 {
					return 0, documentPageToken{}, Invalid
				} else {
					token.Cursor = strconv.FormatInt(row, 10)
				}
			}
			token.hasView = true
			return limit, token, ""
		}
	}
	return limit, documentPageToken{}, ""
}

func decodeDocumentContent(encoded string) ([]byte, error) {
	if len(encoded) > base64.RawStdEncoding.EncodedLen(documentInputLimit) {
		return nil, base64.CorruptInputError(0)
	}
	content, err := base64.RawStdEncoding.DecodeString(encoded)
	if err != nil || len(content) > documentInputLimit {
		return nil, base64.CorruptInputError(0)
	}
	return content, nil
}

func encodeDocumentPage(request Request, order, filter, cursor string, rowID, view int64) string {
	if rowID != 0 {
		cursor = strconv.FormatInt(rowID, 10)
	}
	encoded, _ := json.Marshal(documentPageToken{V: ContractVersion, Operation: request.Operation, ScopeDigest: pageDigest(request.Scope), FilterDigest: pageDigest(map[string]string{"document_id": filter}), Order: order, Limit: *request.Page.Limit, Cursor: cursor, ViewRevision: view})
	return base64.RawURLEncoding.EncodeToString(encoded)
}

func documentScope(scope Scope) (string, string, string) {
	switch scope.Kind {
	case "project":
		if !validDocumentID(scope.ProjectID) {
			return "", "", Invalid
		}
		return scope.ProjectID, "", ""
	case "workstream":
		if !validDocumentID(scope.ProjectID) || !validDocumentID(scope.WorkstreamID) {
			return "", "", Invalid
		}
		return scope.ProjectID, scope.WorkstreamID, ""
	default:
		return "", "", ScopeDenied
	}
}

func documentMetadataResult(revision DocumentRevision, content bool) Result {
	return documentResult(documentMetadata(revision), 1)
}
func documentMetadata(revision DocumentRevision) map[string]any {
	value := map[string]any{"document_id": revision.DocumentID, "revision_id": revision.RevisionID, "project_id": revision.ProjectID, "checksum": revision.Checksum, "byte_count": revision.ByteCount, "provenance": DocumentProvenance{Origin: revision.Provenance.Origin, Client: revision.Provenance.Client, Model: revision.Provenance.Model}, "created_at": revision.CreatedAt.UTC().Format(time.RFC3339Nano)}
	if revision.WorkstreamID != "" {
		value["workstream_id"] = revision.WorkstreamID
	}
	return value
}
func documentResult(value any, count int) Result {
	encoded, err := json.Marshal(value)
	if err != nil || len(encoded) > documentResponseLimit {
		return Result{Outcome: Invalid}
	}
	return Result{Outcome: OK, Value: encoded, ResultCount: intPointer(count)}
}
func validDocumentID(value string) bool { return validID(value) && len(value) <= 128 }
func validDocumentProvenance(value DocumentProvenance) bool {
	return validDocumentID(value.Origin) && len(value.Client) <= 128 && len(value.Model) <= 128
}
func dereference(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
func (s DocumentService) now() func() time.Time {
	if s.Now != nil {
		return s.Now
	}
	return time.Now
}

type codedDocumentError interface{ OutcomeCode() string }

func documentOutcome(err error) string {
	if coded, ok := err.(codedDocumentError); ok {
		return coded.OutcomeCode()
	}
	for _, outcome := range []domain.Outcome{domain.BindingMismatch, domain.ScopeDenied, domain.Conflict, domain.IdempotencyMismatch, domain.IntegrityDiscrepancy, domain.Busy, domain.Retryable, domain.Unknown} {
		if domain.IsOutcome(err, outcome) {
			return string(outcome)
		}
	}
	return Internal
}
