package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"memgraphai/internal/domain"
	"memgraphai/internal/revisionfs"
)

// CheckpointError maps expected checkpoint persistence outcomes without driver details.
type CheckpointError struct{ Code string }

func (e CheckpointError) Error() string       { return e.Code }
func (e CheckpointError) OutcomeCode() string { return e.Code }

type checkpointReference struct {
	DocumentID string `json:"document_id"`
	RevisionID string `json:"revision_id"`
	Fresh      bool   `json:"fresh,omitempty"`
}

// checkpointResponseReference is intentionally separate from checkpointReference:
// persisted request bytes retain their canonical, compatibility-sensitive encoding.
type checkpointResponseReference struct {
	DocumentID string `json:"document_id"`
	RevisionID string `json:"revision_id"`
	Fresh      bool   `json:"fresh"`
}

type checkpointClaim struct {
	generation int64
	outcome    domain.Outcome
}

// SaveCheckpoint registers the complete immutable request before publication, then commits
// its backing revision, pointer, checkpoint metadata, references, and terminal result together.
func (s *Store) SaveCheckpoint(ctx context.Context, files *revisionfs.Filesystem, operationID, checkpointID, checkpointDocumentID, checkpointRevisionID, projectID, workstreamID, sessionID string, prose, referencesJSON []byte, origin, client, model string, createdAt time.Time) error {
	references, err := decodeCheckpointReferences(referencesJSON)
	if err != nil {
		return CheckpointError{Code: "invalid"}
	}
	digest := checkpointDigest(operationID, checkpointID, checkpointDocumentID, checkpointRevisionID, projectID, workstreamID, sessionID, prose, references, origin, client, model)
	claim, err := s.claimCheckpoint(ctx, operationID, checkpointID, checkpointDocumentID, checkpointRevisionID, projectID, workstreamID, sessionID, digest, references)
	if err != nil || claim.outcome != "" {
		return err
	}
	prepared, err := files.PrepareAttempt(operationID, claim.generation, projectID, checkpointDocumentID, checkpointRevisionID, prose)
	if err != nil {
		return mapRecoveryError(err)
	}
	return s.commitCheckpoint(ctx, operationID, checkpointID, checkpointDocumentID, checkpointRevisionID, projectID, workstreamID, sessionID, digest, references, origin, client, model, createdAt, prepared, claim.generation)
}

func (s *Store) claimCheckpoint(ctx context.Context, operationID, checkpointID, documentID, revisionID, projectID, workstreamID, sessionID, digest string, references []checkpointReference) (checkpointClaim, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return checkpointClaim{}, mapRecoveryError(err)
	}
	defer tx.Rollback()
	if err := validateActiveSessionInTx(ctx, tx, projectID, workstreamID, sessionID); err != nil {
		return checkpointClaim{}, err
	}
	if err := validateCheckpointReferencesInTx(ctx, tx, projectID, workstreamID, references); err != nil {
		return checkpointClaim{}, err
	}
	var storedDigest, state string
	var generation int64
	err = tx.QueryRowContext(ctx, `SELECT fingerprint, state, owner_generation FROM operations WHERE operation_id = ?`, operationID).Scan(&storedDigest, &state, &generation)
	if errors.Is(err, sql.ErrNoRows) {
		var count int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM checkpoints WHERE checkpoint_id = ?`, checkpointID).Scan(&count); err != nil {
			return checkpointClaim{}, mapRecoveryError(err)
		}
		if count != 0 {
			return checkpointClaim{}, CheckpointError{Code: "conflict"}
		}
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM documents WHERE document_id = ?`, documentID).Scan(&count); err != nil {
			return checkpointClaim{}, mapRecoveryError(err)
		}
		if count != 0 {
			return checkpointClaim{}, CheckpointError{Code: "conflict"}
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO documents(document_id, project_id, workstream_id, current_revision_id) VALUES (?, ?, ?, NULL)`, documentID, projectID, workstreamID); err != nil {
			return checkpointClaim{}, documentStoreError(err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO operations(operation_id, fingerprint, request_fingerprint, project_id, document_id, revision_id, expected_current_revision_id, state) VALUES (?, ?, 'checkpoint-save', ?, ?, ?, NULL, ?)`, operationID, digest, projectID, documentID, revisionID, operationRegistered); err != nil {
			return checkpointClaim{}, mapRecoveryError(err)
		}
		state, generation = operationRegistered, 0
	} else if err != nil {
		return checkpointClaim{}, mapRecoveryError(err)
	} else if storedDigest != digest {
		return checkpointClaim{}, domain.Error{Outcome: domain.IdempotencyMismatch}
	}
	if state == operationCommitted || state == operationConflict {
		if err := tx.Commit(); err != nil {
			return checkpointClaim{}, mapRecoveryError(err)
		}
		return checkpointClaim{outcome: domain.Outcome(state)}, nil
	}
	if generation >= 3 {
		if err := tx.Commit(); err != nil {
			return checkpointClaim{}, mapRecoveryError(err)
		}
		return checkpointClaim{outcome: domain.Retryable}, CheckpointError{Code: "retryable"}
	}
	generation++
	if _, err := tx.ExecContext(ctx, `UPDATE operations SET state = ?, owner_generation = ? WHERE operation_id = ? AND owner_generation = ? AND state IN (?, ?)`, operationOwned, generation, operationID, generation-1, operationRegistered, operationOwned); err != nil {
		return checkpointClaim{}, mapRecoveryError(err)
	}
	if err := tx.Commit(); err != nil {
		return checkpointClaim{}, mapRecoveryError(err)
	}
	return checkpointClaim{generation: generation}, nil
}

func (s *Store) commitCheckpoint(ctx context.Context, operationID, checkpointID, documentID, revisionID, projectID, workstreamID, sessionID, digest string, references []checkpointReference, origin, client, model string, createdAt time.Time, prepared revisionfs.Prepared, generation int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return mapRecoveryError(err)
	}
	defer tx.Rollback()
	if err := validateActiveSessionInTx(ctx, tx, projectID, workstreamID, sessionID); err != nil {
		return err
	}
	var state, storedDigest string
	var owner int64
	if err := tx.QueryRowContext(ctx, `SELECT state, fingerprint, owner_generation FROM operations WHERE operation_id = ?`, operationID).Scan(&state, &storedDigest, &owner); err != nil {
		return mapRecoveryError(err)
	}
	if storedDigest != digest {
		return domain.Error{Outcome: domain.IdempotencyMismatch}
	}
	if state == operationCommitted || state == operationConflict {
		return nil
	}
	if state != operationOwned || owner != generation {
		return domain.Error{Outcome: domain.Retryable}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO revisions(revision_id, document_id, predecessor_revision_id, file_path, checksum, byte_count, origin, client_provenance, model_provenance, created_at) VALUES (?, ?, NULL, ?, ?, ?, ?, ?, ?, ?)`, revisionID, documentID, prepared.Path, prepared.Checksum, prepared.Bytes, defaultOrigin(origin), nullableValue(client), nullableValue(model), nullableTime(createdAt)); err != nil {
		return documentStoreError(err)
	}
	updated, err := tx.ExecContext(ctx, `UPDATE documents SET current_revision_id = ? WHERE document_id = ? AND current_revision_id IS NULL`, revisionID, documentID)
	if err != nil {
		return mapRecoveryError(err)
	}
	rows, err := updated.RowsAffected()
	if err != nil {
		return mapRecoveryError(err)
	}
	if rows != 1 {
		return CheckpointError{Code: "conflict"}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO checkpoints(checkpoint_id, checkpoint_document_id, checkpoint_revision_id, project_id, workstream_id, session_id, origin, client_provenance, model_provenance, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, checkpointID, documentID, revisionID, projectID, workstreamID, sessionID, origin, nullableValue(client), nullableValue(model), utcText(createdAt)); err != nil {
		return documentStoreError(err)
	}
	for _, reference := range references {
		if _, err := tx.ExecContext(ctx, `INSERT INTO checkpoint_references(checkpoint_id, document_id, revision_id) VALUES (?, ?, ?)`, checkpointID, reference.DocumentID, reference.RevisionID); err != nil {
			return documentStoreError(err)
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE operations SET state = ?, result_outcome = ?, result_revision_id = ? WHERE operation_id = ? AND owner_generation = ? AND state = ?`, operationCommitted, operationCommitted, revisionID, operationID, generation, operationOwned); err != nil {
		return mapRecoveryError(err)
	}
	if err := tx.Commit(); err != nil {
		return mapRecoveryError(err)
	}
	if s.afterOperationCommit != nil && s.afterOperationCommit() != nil {
		return domain.Error{Outcome: domain.Unknown}
	}
	return nil
}

// ReadCheckpoint validates the requesting active session, permits only the same workstream,
// and computes reference freshness from each source document's current pointer.
func (s *Store) ReadCheckpoint(ctx context.Context, files *revisionfs.Filesystem, projectID, workstreamID, sessionID, checkpointID string) (json.RawMessage, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, mapRecoveryError(err)
	}
	defer tx.Rollback()
	if err := validateActiveSessionInTx(ctx, tx, projectID, workstreamID, sessionID); err != nil {
		return nil, err
	}
	var sourceSession, documentID, revisionID, origin, client, model, createdAt, path, checksum string
	var bytes int64
	err = tx.QueryRowContext(ctx, `SELECT c.session_id, c.checkpoint_document_id, c.checkpoint_revision_id, c.origin, COALESCE(c.client_provenance, ''), COALESCE(c.model_provenance, ''), c.created_at, r.file_path, r.checksum, r.byte_count FROM checkpoints c JOIN revisions r ON r.revision_id = c.checkpoint_revision_id WHERE c.checkpoint_id = ? AND c.project_id = ? AND c.workstream_id = ?`, checkpointID, projectID, workstreamID).Scan(&sourceSession, &documentID, &revisionID, &origin, &client, &model, &createdAt, &path, &checksum, &bytes)
	if errors.Is(err, sql.ErrNoRows) {
		var exists int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM checkpoints WHERE checkpoint_id = ?`, checkpointID).Scan(&exists); err != nil {
			return nil, mapRecoveryError(err)
		}
		if exists != 0 {
			return nil, CheckpointError{Code: "scope_denied"}
		}
		return nil, CheckpointError{Code: "not_found"}
	}
	if err != nil {
		return nil, mapRecoveryError(err)
	}
	rows, err := tx.QueryContext(ctx, `SELECT cr.document_id, cr.revision_id, CASE WHEN d.current_revision_id = cr.revision_id THEN 1 ELSE 0 END FROM checkpoint_references cr JOIN documents d ON d.document_id = cr.document_id WHERE cr.checkpoint_id = ? ORDER BY cr.document_id, cr.revision_id`, checkpointID)
	if err != nil {
		return nil, mapRecoveryError(err)
	}
	defer rows.Close()
	references := make([]checkpointResponseReference, 0)
	for rows.Next() {
		var reference checkpointResponseReference
		var fresh int
		if err := rows.Scan(&reference.DocumentID, &reference.RevisionID, &fresh); err != nil {
			return nil, mapRecoveryError(err)
		}
		reference.Fresh = fresh == 1
		references = append(references, reference)
	}
	if err := rows.Err(); err != nil {
		return nil, mapRecoveryError(err)
	}
	if err := tx.Commit(); err != nil {
		return nil, mapRecoveryError(err)
	}
	prose, err := revisionfs.ReadVerified(path, checksum, bytes)
	if err != nil {
		return nil, domain.Error{Outcome: domain.IntegrityDiscrepancy}
	}
	return json.Marshal(map[string]any{"checkpoint_id": checkpointID, "checkpoint_document_id": documentID, "checkpoint_revision_id": revisionID, "project_id": projectID, "workstream_id": workstreamID, "session_id": sourceSession, "prose_base64": base64.RawStdEncoding.EncodeToString(prose), "references": references, "provenance": map[string]string{"origin": origin, "client": client, "model": model}, "created_at": createdAt})
}

func validateActiveSessionInTx(ctx context.Context, tx *sql.Tx, projectID, workstreamID, sessionID string) error {
	if err := requireSessionInTx(ctx, tx, projectID, workstreamID, sessionID); err != nil {
		return err
	}
	var status string
	if err := tx.QueryRowContext(ctx, `SELECT status FROM sessions WHERE session_id = ?`, sessionID).Scan(&status); err != nil {
		return continuityError(err)
	}
	if status != "open" {
		return ContinuityError{Code: continuityConflict}
	}
	return nil
}

func validateCheckpointReferencesInTx(ctx context.Context, tx *sql.Tx, projectID, workstreamID string, references []checkpointReference) error {
	if len(references) == 0 || len(references) > 50 {
		return CheckpointError{Code: "invalid"}
	}
	seen := map[string]bool{}
	for _, reference := range references {
		if reference.DocumentID == "" || reference.RevisionID == "" || reference.Fresh {
			return CheckpointError{Code: "invalid"}
		}
		key := reference.DocumentID + "\x00" + reference.RevisionID
		if seen[key] {
			return CheckpointError{Code: "invalid"}
		}
		seen[key] = true
		var count int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM revisions r JOIN documents d ON d.document_id = r.document_id WHERE d.document_id = ? AND r.revision_id = ? AND d.project_id = ? AND (d.workstream_id IS NULL OR d.workstream_id = ?)`, reference.DocumentID, reference.RevisionID, projectID, workstreamID).Scan(&count); err != nil {
			return mapRecoveryError(err)
		}
		if count != 1 {
			return CheckpointError{Code: "scope_denied"}
		}
	}
	return nil
}

func decodeCheckpointReferences(raw []byte) ([]checkpointReference, error) {
	var references []checkpointReference
	if err := json.Unmarshal(raw, &references); err != nil {
		return nil, err
	}
	return references, nil
}

func checkpointDigest(operationID, checkpointID, documentID, revisionID, projectID, workstreamID, sessionID string, prose []byte, references []checkpointReference, origin, client, model string) string {
	referencesJSON, _ := json.Marshal(references)
	proseDigest := sha256.Sum256(prose)
	fields := []string{operationID, checkpointID, documentID, revisionID, projectID, workstreamID, sessionID, fmt.Sprintf("%x", proseDigest), string(referencesJSON), origin, client, model}
	canonical := ""
	for _, field := range fields {
		canonical += fmt.Sprintf("%d:%s;", len(field), field)
	}
	digest := sha256.Sum256([]byte(canonical))
	return fmt.Sprintf("%x", digest)
}
