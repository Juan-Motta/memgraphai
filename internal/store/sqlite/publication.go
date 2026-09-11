package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"memgraphai/internal/domain"
)

// Publication describes revision metadata prepared outside SQLite.
type Publication struct {
	DocumentID      string
	RevisionID      string
	ExpectedCurrent *string
	FilePath        string
	Checksum        string
	ByteCount       int64
}

// CreateDocument registers a document whose current revision is initially empty.
func (s *Store) CreateDocument(ctx context.Context, documentID, projectID string) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO documents(document_id, project_id, current_revision_id)
		VALUES (?, ?, NULL)`, documentID, projectID)
	return err
}

// PublishRevision atomically records a prepared revision and advances its document pointer.
func (s *Store) PublishRevision(ctx context.Context, publication Publication) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin publication transaction: %w", err)
	}
	defer tx.Rollback()

	var current sql.NullString
	if err := tx.QueryRowContext(ctx, `SELECT current_revision_id FROM documents WHERE document_id = ?`, publication.DocumentID).Scan(&current); err != nil {
		return fmt.Errorf("read expected current revision: %w", err)
	}
	if !matchesExpectedCurrent(current, publication.ExpectedCurrent) {
		return domain.Error{Outcome: domain.Conflict}
	}

	var predecessor any
	if current.Valid {
		predecessor = current.String
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO revisions(
		revision_id, document_id, predecessor_revision_id, file_path, checksum, byte_count
	) VALUES (?, ?, ?, ?, ?, ?)`,
		publication.RevisionID, publication.DocumentID, predecessor,
		publication.FilePath, publication.Checksum, publication.ByteCount,
	); err != nil {
		return fmt.Errorf("record prepared revision: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE documents SET current_revision_id = ? WHERE document_id = ?`,
		publication.RevisionID, publication.DocumentID,
	); err != nil {
		return fmt.Errorf("advance current revision: %w", err)
	}
	if s.beforeCommit != nil {
		if err := s.beforeCommit(); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit publication pointer: %w", err)
	}
	if s.afterCommit != nil && s.afterCommit() != nil {
		return domain.Error{Outcome: domain.Unknown}
	}
	return nil
}

// CurrentRevision returns a document's visible revision when one has committed.
func (s *Store) CurrentRevision(ctx context.Context, documentID string) (string, bool, error) {
	var revision sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT current_revision_id FROM documents WHERE document_id = ?`, documentID).Scan(&revision)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return revision.String, revision.Valid, nil
}

func matchesExpectedCurrent(current sql.NullString, expected *string) bool {
	if expected == nil {
		return !current.Valid
	}
	return current.Valid && current.String == *expected
}
