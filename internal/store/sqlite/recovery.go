package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"memgraphai/internal/domain"
	"memgraphai/internal/revisionfs"
)

const (
	operationRegistered = "registered"
	operationOwned      = "owned"
	operationCommitted  = "committed"
	operationConflict   = "conflict"
)

// OperationRequest is the fixed write identity used across response-loss retries.
// Fingerprint is caller-provided provenance, while the store derives its durable
// equality value from every immutable behavior-affecting request field.
type OperationRequest struct {
	ID              string
	Fingerprint     string
	ProjectID       string
	WorkstreamID    string
	DocumentID      string
	RevisionID      string
	ExpectedCurrent *string
	Markdown        []byte
	Origin          string
	Client          string
	Model           string
	CreatedAt       time.Time
	MaxAttempts     int
}

// OperationResult is the durable terminal result, not a transport acknowledgement.
type OperationResult struct {
	Outcome    domain.Outcome
	RevisionID string
	Generation int64
}

// Authorize is called for every attempt and replay; an operation ID grants nothing.
type Authorize func(context.Context) error

// ExecuteOperation registers an operation, fences one owner, prepares a
// generation-specific immutable file, and commits both pointer and terminal
// operation result in one SQLite transaction.
func (s *Store) ExecuteOperation(ctx context.Context, files *revisionfs.Filesystem, request OperationRequest, authorize Authorize) (OperationResult, error) {
	if err := authorize(ctx); err != nil {
		return OperationResult{}, err
	}
	claim, err := s.ClaimOperation(ctx, request)
	if err != nil || claim.Outcome != "" {
		return claim, err
	}
	prepared, err := files.PrepareAttempt(request.ID, claim.Generation, request.ProjectID, request.DocumentID, request.RevisionID, request.Markdown)
	if err != nil {
		return OperationResult{}, mapRecoveryError(err)
	}
	return s.commitOperation(ctx, request, prepared, claim.Generation)
}

// ClaimOperation atomically registers or takes over a nonterminal operation.
// A later generation fences every prior owner from the SQLite publication commit.
func (s *Store) ClaimOperation(ctx context.Context, request OperationRequest) (OperationResult, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return OperationResult{}, mapRecoveryError(err)
	}
	defer tx.Rollback()
	digest := operationDigest(request)
	var storedDigest string
	err = tx.QueryRowContext(ctx, "SELECT fingerprint FROM operations WHERE operation_id = ?", request.ID).Scan(&storedDigest)
	if errors.Is(err, sql.ErrNoRows) {
		if err := validateDocumentProjectInTx(ctx, tx, request.ProjectID, request.DocumentID); err != nil {
			return OperationResult{}, err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO operations(
			operation_id, fingerprint, request_fingerprint, project_id, document_id, revision_id, expected_current_revision_id, state
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(operation_id) DO NOTHING`, request.ID, digest, request.Fingerprint, request.ProjectID,
			request.DocumentID, request.RevisionID, nullableString(request.ExpectedCurrent), operationRegistered); err != nil {
			return OperationResult{}, mapRecoveryError(err)
		}
	} else if err != nil {
		return OperationResult{}, mapRecoveryError(err)
	} else if storedDigest != digest {
		return OperationResult{}, domain.Error{Outcome: domain.IdempotencyMismatch}
	}
	var fingerprint, state string
	var generation int64
	var resultOutcome, resultRevision sql.NullString
	if err := tx.QueryRowContext(ctx, `SELECT fingerprint, state, owner_generation, result_outcome, result_revision_id
		FROM operations WHERE operation_id = ?`, request.ID).Scan(&fingerprint, &state, &generation, &resultOutcome, &resultRevision); err != nil {
		return OperationResult{}, mapRecoveryError(err)
	}
	if fingerprint != digest {
		return OperationResult{}, domain.Error{Outcome: domain.IdempotencyMismatch}
	}
	if err := validateDocumentProjectInTx(ctx, tx, request.ProjectID, request.DocumentID); err != nil {
		return OperationResult{}, err
	}
	if state == operationCommitted || state == operationConflict {
		if err := tx.Commit(); err != nil {
			return OperationResult{}, mapRecoveryError(err)
		}
		return storedResult(resultOutcome, resultRevision, generation), nil
	}
	maxAttempts := request.MaxAttempts
	if maxAttempts == 0 {
		maxAttempts = 3
	}
	if generation >= int64(maxAttempts) {
		if err := tx.Commit(); err != nil {
			return OperationResult{}, mapRecoveryError(err)
		}
		return OperationResult{Outcome: domain.Retryable, Generation: generation}, nil
	}
	generation++
	if _, err := tx.ExecContext(ctx, `UPDATE operations SET state = ?, owner_generation = ?
		WHERE operation_id = ? AND owner_generation = ? AND state IN (?, ?)`, operationOwned, generation,
		request.ID, generation-1, operationRegistered, operationOwned); err != nil {
		return OperationResult{}, mapRecoveryError(err)
	}
	if err := tx.Commit(); err != nil {
		return OperationResult{}, mapRecoveryError(err)
	}
	return OperationResult{Generation: generation}, nil
}

func (s *Store) commitOperation(ctx context.Context, request OperationRequest, prepared revisionfs.Prepared, generation int64) (OperationResult, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return OperationResult{}, mapRecoveryError(err)
	}
	defer tx.Rollback()
	var state string
	var ownedGeneration int64
	if err := tx.QueryRowContext(ctx, `SELECT state, owner_generation FROM operations WHERE operation_id = ?`, request.ID).Scan(&state, &ownedGeneration); err != nil {
		return OperationResult{}, mapRecoveryError(err)
	}
	if state == operationCommitted || state == operationConflict {
		return s.recoverInTx(ctx, tx, request.ID, ownedGeneration)
	}
	if state != operationOwned || ownedGeneration != generation {
		return OperationResult{}, domain.Error{Outcome: domain.Retryable}
	}
	var current sql.NullString
	if err := tx.QueryRowContext(ctx, `SELECT current_revision_id FROM documents WHERE document_id = ?`, request.DocumentID).Scan(&current); err != nil {
		return OperationResult{}, mapRecoveryError(err)
	}
	if !matchesExpectedCurrent(current, request.ExpectedCurrent) {
		result, err := s.recordTerminal(ctx, tx, request.ID, generation, domain.Conflict, "")
		if err != nil {
			return OperationResult{}, err
		}
		if err := tx.Commit(); err != nil {
			return OperationResult{}, mapRecoveryError(err)
		}
		return result, nil
	}
	var predecessor any
	if current.Valid {
		predecessor = current.String
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO revisions(
		revision_id, document_id, predecessor_revision_id, file_path, checksum, byte_count,
		origin, client_provenance, model_provenance, created_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, request.RevisionID, request.DocumentID, predecessor,
		prepared.Path, prepared.Checksum, prepared.Bytes, defaultOrigin(request.Origin), nullableValue(request.Client),
		nullableValue(request.Model), nullableTime(request.CreatedAt)); err != nil {
		return OperationResult{}, mapRecoveryError(err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE documents SET current_revision_id = ? WHERE document_id = ?`, request.RevisionID, request.DocumentID); err != nil {
		return OperationResult{}, mapRecoveryError(err)
	}
	result, err := s.recordTerminal(ctx, tx, request.ID, generation, domain.Outcome(operationCommitted), request.RevisionID)
	if err != nil {
		return OperationResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return OperationResult{}, mapRecoveryError(err)
	}
	if s.afterOperationCommit != nil && s.afterOperationCommit() != nil {
		return OperationResult{}, domain.Error{Outcome: domain.Unknown}
	}
	return result, nil
}

func (s *Store) recordTerminal(ctx context.Context, tx *sql.Tx, operationID string, generation int64, outcome domain.Outcome, revisionID string) (OperationResult, error) {
	if _, err := tx.ExecContext(ctx, `UPDATE operations SET state = ?, result_outcome = ?, result_revision_id = ?
		WHERE operation_id = ? AND owner_generation = ? AND state = ?`, string(outcome), string(outcome), nullableValue(revisionID), operationID, generation, operationOwned); err != nil {
		return OperationResult{}, mapRecoveryError(err)
	}
	return OperationResult{Outcome: outcome, RevisionID: revisionID, Generation: generation}, nil
}

// RecoverOperation reads only durable operation state. A nonterminal row stays
// unknown; recovery never turns a missing response into inferred rollback.
func (s *Store) RecoverOperation(ctx context.Context, operationID, fingerprint string, authorize Authorize) (OperationResult, error) {
	if err := authorize(ctx); err != nil {
		return OperationResult{}, err
	}
	var actual, outcome, revision sql.NullString
	var state string
	var generation int64
	err := s.db.QueryRowContext(ctx, `SELECT request_fingerprint, state, owner_generation, result_outcome, result_revision_id
		FROM operations WHERE operation_id = ?`, operationID).Scan(&actual, &state, &generation, &outcome, &revision)
	if errors.Is(err, sql.ErrNoRows) {
		return OperationResult{Outcome: domain.Unknown}, nil
	}
	if err != nil {
		return OperationResult{}, mapRecoveryError(err)
	}
	if actual.String != fingerprint {
		return OperationResult{}, domain.Error{Outcome: domain.IdempotencyMismatch}
	}
	if state != operationCommitted && state != operationConflict {
		return OperationResult{Outcome: domain.Unknown, Generation: generation}, nil
	}
	return storedResult(outcome, revision, generation), nil
}

// VerifyRevision performs a targeted current or historical immutable-file check.
func (s *Store) VerifyRevision(ctx context.Context, files *revisionfs.Filesystem, revisionID string) error {
	var path, checksum string
	var bytes int64
	err := s.db.QueryRowContext(ctx, `SELECT file_path, checksum, byte_count FROM revisions WHERE revision_id = ?`, revisionID).Scan(&path, &checksum, &bytes)
	if err != nil {
		return err
	}
	if err := revisionfs.Verify(path, checksum, bytes); err != nil {
		return domain.Error{Outcome: domain.IntegrityDiscrepancy}
	}
	return nil
}

// Orphans reports immutable files without SQLite revision metadata; it never imports or deletes them.
func (s *Store) Orphans(ctx context.Context, files *revisionfs.Filesystem) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT file_path FROM revisions")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	known := map[string]bool{}
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			return nil, err
		}
		known[path] = true
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return files.Orphans(known)
}

func (s *Store) recoverInTx(ctx context.Context, tx *sql.Tx, operationID string, generation int64) (OperationResult, error) {
	var outcome, revision sql.NullString
	if err := tx.QueryRowContext(ctx, `SELECT result_outcome, result_revision_id FROM operations WHERE operation_id = ?`, operationID).Scan(&outcome, &revision); err != nil {
		return OperationResult{}, mapRecoveryError(err)
	}
	return storedResult(outcome, revision, generation), nil
}

func validateDocumentProjectInTx(ctx context.Context, tx *sql.Tx, projectID, documentID string) error {
	var matches int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM documents
		WHERE document_id = ? AND project_id = ?`, documentID, projectID).Scan(&matches); err != nil {
		return mapRecoveryError(err)
	}
	if matches != 1 {
		return domain.Error{Outcome: domain.BindingMismatch}
	}
	return nil
}

func operationDigest(request OperationRequest) string {
	if request.WorkstreamID == "" && request.Origin == "" && request.Client == "" && request.Model == "" && request.CreatedAt.IsZero() {
		return operationDigestV3(request)
	}
	return operationDigestV4(request)
}

// operationDigestV3 preserves the baseline durable identity for legacy rows.
func operationDigestV3(request OperationRequest) string {
	markdown := sha256.Sum256(request.Markdown)
	fields := []string{
		request.Fingerprint,
		request.ProjectID,
		request.DocumentID,
		request.RevisionID,
		fmt.Sprintf("%x", markdown),
	}
	if request.ExpectedCurrent == nil {
		fields = append(fields, "expected-current:nil")
	} else {
		fields = append(fields, "expected-current:value", *request.ExpectedCurrent)
	}
	var canonical strings.Builder
	for _, field := range fields {
		fmt.Fprintf(&canonical, "%d:%s;", len(field), field)
	}
	digest := sha256.Sum256([]byte(canonical.String()))
	return fmt.Sprintf("%x", digest)
}

func operationDigestV4(request OperationRequest) string {
	markdown := sha256.Sum256(request.Markdown)
	fields := []string{
		request.Fingerprint,
		request.ProjectID,
		request.WorkstreamID,
		request.DocumentID,
		request.RevisionID,
		fmt.Sprintf("%x", markdown),
		defaultOrigin(request.Origin),
		request.Client,
		request.Model,
	}
	if request.ExpectedCurrent == nil {
		fields = append(fields, "expected-current:nil")
	} else {
		fields = append(fields, "expected-current:value", *request.ExpectedCurrent)
	}
	var canonical strings.Builder
	for _, field := range fields {
		fmt.Fprintf(&canonical, "%d:%s;", len(field), field)
	}
	digest := sha256.Sum256([]byte(canonical.String()))
	return fmt.Sprintf("%x", digest)
}

func storedResult(outcome, revision sql.NullString, generation int64) OperationResult {
	return OperationResult{Outcome: domain.Outcome(outcome.String), RevisionID: revision.String, Generation: generation}
}

func defaultOrigin(origin string) string {
	if origin == "" {
		return "unknown"
	}
	return origin
}

func nullableTime(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func nullableString(value *string) any {
	if value == nil {
		return nil
	}
	return *value
}

func nullableValue(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func mapRecoveryError(err error) error {
	if err == nil {
		return nil
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "database is locked") || strings.Contains(message, "sqlite_busy") || strings.Contains(message, "busy") {
		return domain.Error{Outcome: domain.Busy}
	}
	if errors.Is(err, revisionfs.ErrIntegrity) {
		return domain.Error{Outcome: domain.IntegrityDiscrepancy}
	}
	return fmt.Errorf("recovery store: %w", err)
}
