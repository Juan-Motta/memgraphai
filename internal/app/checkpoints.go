package app

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"time"

	"memgraphai/internal/domain"
	"memgraphai/internal/revisionfs"
)

const (
	checkpointProseLimit     = 64 << 10
	checkpointReferenceLimit = 50
	checkpointResponseLimit  = 2 << 20
)

// CheckpointProvenance records the required origin and optional client-reported fields.
type CheckpointProvenance struct {
	Origin string `json:"origin"`
	Client string `json:"client,omitempty"`
	Model  string `json:"model,omitempty"`
}

// CheckpointReference identifies one exact already-published document revision.
type CheckpointReference struct {
	DocumentID string `json:"document_id"`
	RevisionID string `json:"revision_id"`
	Fresh      bool   `json:"fresh"`
}

// CheckpointRecord is the bounded handoff selected by its immutable checkpoint ID.
type CheckpointRecord struct {
	CheckpointID         string
	CheckpointDocumentID string
	CheckpointRevisionID string
	ProjectID            string
	WorkstreamID         string
	SessionID            string
	Prose                []byte
	References           []CheckpointReference
	Provenance           CheckpointProvenance
	CreatedAt            time.Time
}

// CheckpointStore owns durable checkpoint identity, exact-reference validation, and reads.
type CheckpointStore interface {
	SaveCheckpoint(context.Context, *revisionfs.Filesystem, string, string, string, string, string, string, string, []byte, []byte, string, string, string, time.Time) error
	ReadCheckpoint(context.Context, *revisionfs.Filesystem, string, string, string, string) (json.RawMessage, error)
}

// CheckpointService exposes only selected save/read handoffs; list, update, delete, and task references are excluded.
type CheckpointService struct {
	Store CheckpointStore
	Files *revisionfs.Filesystem
	Now   func() time.Time
}

func (s CheckpointService) Handle(ctx context.Context, request Request) Result {
	if s.Store == nil || s.Files == nil {
		return Result{Outcome: Internal}
	}
	switch request.Operation {
	case "checkpoint.save":
		return s.save(ctx, request)
	case "checkpoint.read":
		return s.read(ctx, request)
	default:
		return Result{Outcome: Invalid}
	}
}

type checkpointSaveInput struct {
	CheckpointID         string                `json:"checkpoint_id"`
	CheckpointDocumentID string                `json:"checkpoint_document_id"`
	CheckpointRevisionID string                `json:"checkpoint_revision_id"`
	ProseBase64          string                `json:"prose_base64"`
	References           []CheckpointReference `json:"references"`
	Provenance           CheckpointProvenance  `json:"provenance"`
}

func (s CheckpointService) save(ctx context.Context, request Request) Result {
	if request.Scope.Kind != "session" || !validCheckpointID(request.Scope.ProjectID) || !validCheckpointID(request.Scope.WorkstreamID) || !validCheckpointID(request.Scope.SessionID) {
		return Result{Outcome: ScopeDenied}
	}
	var input checkpointSaveInput
	if err := decodeStrict(request.Input, &input); err != nil || !validCheckpointID(input.CheckpointID) || !validCheckpointID(input.CheckpointDocumentID) || !validCheckpointID(input.CheckpointRevisionID) || !validCheckpointProvenance(input.Provenance) || !validCheckpointReferences(input.References) {
		return Result{Outcome: Invalid}
	}
	prose, err := decodeCheckpointProse(input.ProseBase64)
	if err != nil {
		return Result{Outcome: Invalid}
	}
	references, _ := json.Marshal(input.References)
	createdAt := s.now()()
	if err := s.Store.SaveCheckpoint(ctx, s.Files, request.OperationID, input.CheckpointID, input.CheckpointDocumentID, input.CheckpointRevisionID,
		request.Scope.ProjectID, request.Scope.WorkstreamID, request.Scope.SessionID, prose, references,
		input.Provenance.Origin, input.Provenance.Client, input.Provenance.Model, createdAt); err != nil {
		return Result{Outcome: checkpointOutcome(err)}
	}
	raw, err := s.Store.ReadCheckpoint(ctx, s.Files, request.Scope.ProjectID, request.Scope.WorkstreamID, request.Scope.SessionID, input.CheckpointID)
	if err != nil {
		return Result{Outcome: checkpointOutcome(err)}
	}
	return checkpointRawResult(raw)
}

func (s CheckpointService) read(ctx context.Context, request Request) Result {
	if request.Scope.Kind != "session" || !validCheckpointID(request.Scope.ProjectID) || !validCheckpointID(request.Scope.WorkstreamID) || !validCheckpointID(request.Scope.SessionID) {
		return Result{Outcome: ScopeDenied}
	}
	var input struct {
		CheckpointID string `json:"checkpoint_id"`
	}
	if err := decodeStrict(request.Input, &input); err != nil || !validCheckpointID(input.CheckpointID) {
		return Result{Outcome: Invalid}
	}
	raw, err := s.Store.ReadCheckpoint(ctx, s.Files, request.Scope.ProjectID, request.Scope.WorkstreamID, request.Scope.SessionID, input.CheckpointID)
	if err != nil {
		return Result{Outcome: checkpointOutcome(err)}
	}
	return checkpointRawResult(raw)
}

func decodeCheckpointProse(encoded string) ([]byte, error) {
	if len(encoded) > base64.RawStdEncoding.EncodedLen(checkpointProseLimit) {
		return nil, base64.CorruptInputError(0)
	}
	prose, err := base64.RawStdEncoding.DecodeString(encoded)
	if err != nil || len(prose) > checkpointProseLimit {
		return nil, base64.CorruptInputError(0)
	}
	return prose, nil
}

func validCheckpointID(value string) bool { return validID(value) && len(value) <= 128 }
func validCheckpointProvenance(value CheckpointProvenance) bool {
	return validCheckpointID(value.Origin) && len(value.Client) <= 128 && len(value.Model) <= 128
}
func validCheckpointReferences(references []CheckpointReference) bool {
	if len(references) == 0 || len(references) > checkpointReferenceLimit {
		return false
	}
	seen := make(map[string]bool, len(references))
	for _, reference := range references {
		if !validCheckpointID(reference.DocumentID) || !validCheckpointID(reference.RevisionID) || reference.Fresh {
			return false
		}
		key := reference.DocumentID + "\x00" + reference.RevisionID
		if seen[key] {
			return false
		}
		seen[key] = true
	}
	return true
}

func checkpointRawResult(raw json.RawMessage) Result {
	if !json.Valid(raw) || len(raw) > checkpointResponseLimit {
		return Result{Outcome: Invalid}
	}
	return Result{Outcome: OK, Value: raw, ResultCount: intPointer(1)}
}

func (s CheckpointService) now() func() time.Time {
	if s.Now != nil {
		return s.Now
	}
	return time.Now
}

type codedCheckpointError interface{ OutcomeCode() string }

func checkpointOutcome(err error) string {
	if coded, ok := err.(codedCheckpointError); ok {
		return coded.OutcomeCode()
	}
	for _, outcome := range []domain.Outcome{domain.BindingMismatch, domain.ScopeDenied, domain.Conflict, domain.IdempotencyMismatch, domain.IntegrityDiscrepancy, domain.Busy, domain.Retryable, domain.Unknown} {
		if domain.IsOutcome(err, outcome) {
			return string(outcome)
		}
	}
	return Internal
}
