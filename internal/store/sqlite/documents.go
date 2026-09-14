package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"memgraphai/internal/domain"
	"memgraphai/internal/revisionfs"
)

// DocumentError keeps expected document outcomes out of driver error text.
type DocumentError struct{ Code string }

func (e DocumentError) Error() string       { return e.Code }
func (e DocumentError) OutcomeCode() string { return e.Code }

// RegisterDocumentCreate atomically binds an initially invisible row to its complete request.
func (s *Store) RegisterDocumentCreate(ctx context.Context, operationID, documentID, projectID, workstreamID, revisionID string, expectedRevision *string, content []byte, origin, client, model string, createdAt time.Time) error {
	request := OperationRequest{ID: operationID, Fingerprint: "document-write", ProjectID: projectID, WorkstreamID: workstreamID,
		DocumentID: documentID, RevisionID: revisionID, ExpectedCurrent: expectedRevision, Markdown: content,
		Origin: origin, Client: client, Model: model, CreatedAt: createdAt}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := validateDocumentScopeInTx(ctx, tx, projectID, workstreamID, "", false); err != nil {
		return err
	}
	var actualProject, actualWorkstream string
	err = tx.QueryRowContext(ctx, `SELECT project_id, COALESCE(workstream_id, '') FROM documents WHERE document_id = ?`, documentID).Scan(&actualProject, &actualWorkstream)
	if errors.Is(err, sql.ErrNoRows) {
		if _, err := tx.ExecContext(ctx, `INSERT INTO documents(document_id, project_id, workstream_id, current_revision_id) VALUES (?, ?, ?, NULL)`, documentID, projectID, nullableValue(workstreamID)); err != nil {
			return documentStoreError(err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO operations(
			operation_id, fingerprint, request_fingerprint, project_id, document_id, revision_id, expected_current_revision_id, state
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, request.ID, operationDigest(request), request.Fingerprint,
			request.ProjectID, request.DocumentID, request.RevisionID, nullableString(request.ExpectedCurrent), operationRegistered); err != nil {
			return documentStoreError(err)
		}
		return tx.Commit()
	}
	if err != nil {
		return err
	}
	if actualProject != projectID || actualWorkstream != workstreamID {
		return DocumentError{Code: "scope_denied"}
	}
	var operationProject, operationDocument, digest string
	if err := tx.QueryRowContext(ctx, `SELECT project_id, document_id, fingerprint FROM operations WHERE operation_id = ?`, operationID).Scan(&operationProject, &operationDocument, &digest); errors.Is(err, sql.ErrNoRows) {
		return DocumentError{Code: "conflict"}
	} else if err != nil {
		return err
	}
	if operationProject != projectID || operationDocument != documentID {
		return DocumentError{Code: "conflict"}
	}
	if digest != operationDigest(request) {
		return domain.Error{Outcome: domain.IdempotencyMismatch}
	}
	return tx.Commit()
}

// WriteDocument reuses the fenced operation ledger and revalidates scope on every replay.
func (s *Store) WriteDocument(ctx context.Context, files *revisionfs.Filesystem, operationID, documentID, projectID, workstreamID, revisionID string, expectedRevision *string, content []byte, origin, client, model string, createdAt time.Time) (string, string, error) {
	result, err := s.ExecuteOperation(ctx, files, OperationRequest{
		ID: operationID, Fingerprint: "document-write", ProjectID: projectID, WorkstreamID: workstreamID, DocumentID: documentID, RevisionID: revisionID,
		ExpectedCurrent: expectedRevision, Markdown: content, Origin: origin, Client: client, Model: model, CreatedAt: createdAt,
	}, func(ctx context.Context) error {
		return s.validateDocumentScope(ctx, projectID, workstreamID, documentID)
	})
	if err != nil {
		return "", "", err
	}
	if result.Outcome == domain.Outcome(operationCommitted) {
		return "ok", result.RevisionID, nil
	}
	return string(result.Outcome), result.RevisionID, nil
}

// VisitDocuments returns only published current metadata for one exact scope.
func (s *Store) VisitDocuments(ctx context.Context, projectID, workstreamID, afterID string, limit int, visit func(revisionfs.DocumentRevision)) (int64, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	if err := validateDocumentScopeInTx(ctx, tx, projectID, workstreamID, "", false); err != nil {
		return 0, err
	}
	var generation int64
	if err := tx.QueryRowContext(ctx, "SELECT generation FROM document_views WHERE singleton = 1").Scan(&generation); err != nil {
		return 0, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT d.document_id, d.project_id, COALESCE(d.workstream_id, ''), r.revision_id, r.checksum, r.byte_count, r.origin, COALESCE(r.client_provenance, ''), COALESCE(r.model_provenance, ''), r.created_at FROM documents d JOIN revisions r ON r.revision_id = d.current_revision_id WHERE d.project_id = ? AND ((? = '' AND d.workstream_id IS NULL) OR d.workstream_id = ?) AND d.document_id > ? ORDER BY d.document_id LIMIT ?`, projectID, workstreamID, workstreamID, afterID, limit)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	for rows.Next() {
		revision, err := scanDocumentRevision(rows)
		if err != nil {
			return 0, err
		}
		visit(revision)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return generation, nil
}

// VisitDocumentHistory returns newest-first metadata for one exact scoped document.
func (s *Store) VisitDocumentHistory(ctx context.Context, projectID, workstreamID, documentID string, beforeRowID int64, limit int, visit func(revisionfs.DocumentRevision)) (int64, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	if err := validateDocumentScopeInTx(ctx, tx, projectID, workstreamID, documentID, true); err != nil {
		return 0, err
	}
	var generation int64
	if err := tx.QueryRowContext(ctx, "SELECT generation FROM document_views WHERE singleton = 1").Scan(&generation); err != nil {
		return 0, err
	}
	if beforeRowID == 0 {
		beforeRowID = 1<<62 - 1
	}
	rows, err := tx.QueryContext(ctx, `SELECT r.rowid, d.document_id, d.project_id, COALESCE(d.workstream_id, ''), r.revision_id, r.checksum, r.byte_count, r.origin, COALESCE(r.client_provenance, ''), COALESCE(r.model_provenance, ''), r.created_at FROM revisions r JOIN documents d ON d.document_id = r.document_id WHERE r.document_id = ? AND r.rowid < ? ORDER BY r.rowid DESC LIMIT ?`, documentID, beforeRowID, limit)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	for rows.Next() {
		var rowID int64
		revision, err := scanDocumentRevisionWithRow(rows, &rowID)
		if err != nil {
			return 0, err
		}
		revision.Cursor = rowID
		visit(revision)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return generation, nil
}

// ReadDocument proves scope and revision ownership before reading descriptor-bound bytes.
func (s *Store) ReadDocument(ctx context.Context, files *revisionfs.Filesystem, projectID, workstreamID, documentID string, requestedRevision *string) (revisionfs.DocumentRevision, []byte, error) {
	if err := s.validateDocumentScope(ctx, projectID, workstreamID, documentID); err != nil {
		return revisionfs.DocumentRevision{}, nil, err
	}
	var revisionID string
	if requestedRevision == nil {
		var current sql.NullString
		if err := s.db.QueryRowContext(ctx, "SELECT current_revision_id FROM documents WHERE document_id = ?", documentID).Scan(&current); err != nil {
			return revisionfs.DocumentRevision{}, nil, documentStoreError(err)
		}
		if !current.Valid {
			return revisionfs.DocumentRevision{}, nil, DocumentError{Code: "not_found"}
		}
		revisionID = current.String
	} else {
		var owner string
		err := s.db.QueryRowContext(ctx, "SELECT document_id FROM revisions WHERE revision_id = ?", *requestedRevision).Scan(&owner)
		if errors.Is(err, sql.ErrNoRows) {
			return revisionfs.DocumentRevision{}, nil, DocumentError{Code: "not_found"}
		}
		if err != nil {
			return revisionfs.DocumentRevision{}, nil, err
		}
		if owner != documentID {
			return revisionfs.DocumentRevision{}, nil, domain.Error{Outcome: domain.BindingMismatch}
		}
		revisionID = *requestedRevision
	}
	row := s.db.QueryRowContext(ctx, `SELECT d.document_id, d.project_id, COALESCE(d.workstream_id, ''), r.revision_id, r.checksum, r.byte_count, r.origin, COALESCE(r.client_provenance, ''), COALESCE(r.model_provenance, ''), r.created_at, r.file_path FROM revisions r JOIN documents d ON d.document_id = r.document_id WHERE r.revision_id = ? AND r.document_id = ?`, revisionID, documentID)
	var path string
	revision, err := scanDocumentRevisionAndPath(row, &path)
	if err != nil {
		return revisionfs.DocumentRevision{}, nil, documentStoreError(err)
	}
	content, err := revisionfs.ReadVerified(path, revision.Checksum, revision.ByteCount)
	if err != nil {
		return revisionfs.DocumentRevision{}, nil, domain.Error{Outcome: domain.IntegrityDiscrepancy}
	}
	return revision, content, nil
}

func (s *Store) validateDocumentScope(ctx context.Context, projectID, workstreamID, documentID string) error {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := validateDocumentScopeInTx(ctx, tx, projectID, workstreamID, documentID, true); err != nil {
		return err
	}
	return tx.Commit()
}

func validateDocumentScopeInTx(ctx context.Context, tx *sql.Tx, projectID, workstreamID, documentID string, requireDocument bool) error {
	if err := requireProjectInTx(ctx, tx, projectID); err != nil {
		return err
	}
	if workstreamID != "" {
		var actualProject string
		err := tx.QueryRowContext(ctx, "SELECT project_id FROM workstreams WHERE workstream_id = ?", workstreamID).Scan(&actualProject)
		if errors.Is(err, sql.ErrNoRows) {
			return DocumentError{Code: "not_found"}
		}
		if err != nil {
			return err
		}
		if actualProject != projectID {
			return domain.Error{Outcome: domain.BindingMismatch}
		}
	}
	if !requireDocument {
		return nil
	}
	var actualProject, actualWorkstream string
	err := tx.QueryRowContext(ctx, "SELECT project_id, COALESCE(workstream_id, '') FROM documents WHERE document_id = ?", documentID).Scan(&actualProject, &actualWorkstream)
	if errors.Is(err, sql.ErrNoRows) {
		return DocumentError{Code: "not_found"}
	}
	if err != nil {
		return err
	}
	if actualProject != projectID || actualWorkstream != workstreamID {
		return DocumentError{Code: "scope_denied"}
	}
	return nil
}

type documentScanner interface{ Scan(...any) error }

func scanDocumentRevision(scanner documentScanner) (revisionfs.DocumentRevision, error) {
	return scanDocumentRevisionWithRow(scanner, nil)
}
func scanDocumentRevisionWithRow(scanner documentScanner, rowID *int64) (revisionfs.DocumentRevision, error) {
	var revision revisionfs.DocumentRevision
	var created sql.NullString
	values := []any{&revision.DocumentID, &revision.ProjectID, &revision.WorkstreamID, &revision.RevisionID, &revision.Checksum, &revision.ByteCount, &revision.Provenance.Origin, &revision.Provenance.Client, &revision.Provenance.Model, &created}
	if rowID != nil {
		values = append([]any{rowID}, values...)
	}
	if err := scanner.Scan(values...); err != nil {
		return revisionfs.DocumentRevision{}, err
	}
	revision.CreatedAt = parseUTCTime(created)
	return revision, nil
}
func scanDocumentRevisionAndPath(scanner documentScanner, path *string) (revisionfs.DocumentRevision, error) {
	var revision revisionfs.DocumentRevision
	var created sql.NullString
	if err := scanner.Scan(&revision.DocumentID, &revision.ProjectID, &revision.WorkstreamID, &revision.RevisionID, &revision.Checksum, &revision.ByteCount, &revision.Provenance.Origin, &revision.Provenance.Client, &revision.Provenance.Model, &created, path); err != nil {
		return revisionfs.DocumentRevision{}, err
	}
	revision.CreatedAt = parseUTCTime(created)
	return revision, nil
}
func documentStoreError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return DocumentError{Code: "not_found"}
	}
	if strings.Contains(strings.ToLower(err.Error()), "constraint") || strings.Contains(strings.ToLower(err.Error()), "unique") || strings.Contains(err.Error(), "immutable") {
		return DocumentError{Code: "conflict"}
	}
	return fmt.Errorf("document store: %w", err)
}
