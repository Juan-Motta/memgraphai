package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"memgraphai/internal/domain"
)

// ContinuityError keeps expected persistence outcomes out of driver messages.
type ContinuityError struct{ Code string }

func (e ContinuityError) Error() string       { return e.Code }
func (e ContinuityError) OutcomeCode() string { return e.Code }

const (
	continuityNotFound = "not_found"
	continuityConflict = "conflict"
)

// CreateContinuityWorkstream persists caller-provided metadata under one project.
func (s *Store) CreateContinuityWorkstream(ctx context.Context, projectID, workstreamID, origin, client, model string, createdAt time.Time) error {
	if err := s.requireProject(ctx, projectID); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO workstreams(
		workstream_id, project_id, created_at, origin, client_provenance, model_provenance
	) VALUES (?, ?, ?, ?, ?, ?)`, workstreamID, projectID, utcText(createdAt), origin,
		nullableValue(client), nullableValue(model))
	return continuityError(err)
}

// VisitWorkstreams reads one immutable-ID keyset page and its current view generation.
func (s *Store) VisitWorkstreams(ctx context.Context, projectID, afterID string, limit int, visit func(string, string, string, string, string, time.Time)) (int64, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	if err := requireProjectInTx(ctx, tx, projectID); err != nil {
		return 0, err
	}
	var generation int64
	if err := tx.QueryRowContext(ctx, "SELECT generation FROM continuity_views WHERE singleton = 1").Scan(&generation); err != nil {
		return 0, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT workstream_id, COALESCE(forked_from_workstream_id, ''),
		origin, COALESCE(client_provenance, ''), COALESCE(model_provenance, ''), created_at
		FROM workstreams WHERE project_id = ? AND workstream_id > ?
		ORDER BY workstream_id LIMIT ?`, projectID, afterID, limit)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	for rows.Next() {
		var id, source, origin, client, model string
		var created sql.NullString
		if err := rows.Scan(&id, &source, &origin, &client, &model, &created); err != nil {
			return 0, err
		}
		visit(id, source, origin, client, model, parseUTCTime(created))
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return generation, nil
}

// ForkWorkstream creates a same-project metadata fork without changing its source.
func (s *Store) ForkWorkstream(ctx context.Context, projectID, sourceID, workstreamID, origin, client, model string, createdAt time.Time) error {
	if err := s.requireWorkstream(ctx, projectID, sourceID); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO workstreams(
		workstream_id, project_id, forked_from_workstream_id, created_at, origin, client_provenance, model_provenance
	) VALUES (?, ?, ?, ?, ?, ?, ?)`, workstreamID, projectID, sourceID, utcText(createdAt), origin,
		nullableValue(client), nullableValue(model))
	return continuityError(err)
}

// OpenSession creates a new explicit run after validating its parent workstream.
func (s *Store) OpenSession(ctx context.Context, projectID, workstreamID, sessionID, origin, client, model string, openedAt time.Time) error {
	if err := s.requireWorkstream(ctx, projectID, workstreamID); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO sessions(
		session_id, project_id, workstream_id, status, opened_at, origin, client_provenance, model_provenance
	) VALUES (?, ?, ?, 'open', ?, ?, ?, ?)`, sessionID, projectID, workstreamID, utcText(openedAt), origin,
		nullableValue(client), nullableValue(model))
	return continuityError(err)
}

// SessionStatus structurally validates a triple and reports even terminal sessions.
func (s *Store) SessionStatus(ctx context.Context, projectID, workstreamID, sessionID string) (string, string, string, string, string, time.Time, error) {
	if err := s.requireSession(ctx, projectID, workstreamID, sessionID); err != nil {
		return "", "", "", "", "", time.Time{}, err
	}
	var status, source, origin, client, model string
	var opened sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT status, COALESCE(resumed_from_session_id, ''), origin,
		COALESCE(client_provenance, ''), COALESCE(model_provenance, ''), opened_at
		FROM sessions WHERE session_id = ? AND project_id = ? AND workstream_id = ?`,
		sessionID, projectID, workstreamID).Scan(&status, &source, &origin, &client, &model, &opened)
	if err != nil {
		return "", "", "", "", "", time.Time{}, continuityError(err)
	}
	return status, source, origin, client, model, parseUTCTime(opened), nil
}

// CloseSession compares lifecycle state so a close cannot silently overwrite another transition.
func (s *Store) CloseSession(ctx context.Context, projectID, workstreamID, sessionID string, closedAt time.Time) error {
	return s.transitionSession(ctx, projectID, workstreamID, sessionID, "closed", "", closedAt, "open", "disconnected")
}

// DisconnectSession persists only an adapter-attributable observation, never raw EOF.
func (s *Store) DisconnectSession(ctx context.Context, projectID, workstreamID, sessionID, observedBy string, observedAt time.Time) error {
	if strings.TrimSpace(observedBy) == "" {
		return ContinuityError{Code: continuityConflict}
	}
	return s.transitionSession(ctx, projectID, workstreamID, sessionID, "disconnected", observedBy, observedAt, "open")
}

// ResumeSession requires an explicit source belonging to the already selected workstream.
func (s *Store) ResumeSession(ctx context.Context, projectID, workstreamID, sessionID, sourceSessionID, origin, client, model string, openedAt time.Time) error {
	if err := s.requireSession(ctx, projectID, workstreamID, sourceSessionID); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO sessions(
		session_id, project_id, workstream_id, status, opened_at, origin, client_provenance,
		model_provenance, resumed_from_session_id
	) VALUES (?, ?, ?, 'open', ?, ?, ?, ?, ?)`, sessionID, projectID, workstreamID, utcText(openedAt),
		origin, nullableValue(client), nullableValue(model), sourceSessionID)
	return continuityError(err)
}

// ValidateActiveBinding rejects non-open sessions without weakening structural lookup.
func (s *Store) ValidateActiveBinding(ctx context.Context, projectID, workstreamID, sessionID string) error {
	if err := s.requireSession(ctx, projectID, workstreamID, sessionID); err != nil {
		return err
	}
	var status string
	if err := s.db.QueryRowContext(ctx, `SELECT status FROM sessions
		WHERE session_id = ? AND project_id = ? AND workstream_id = ?`, sessionID, projectID, workstreamID).Scan(&status); err != nil {
		return continuityError(err)
	}
	if status != "open" {
		return ContinuityError{Code: continuityConflict}
	}
	return nil
}

func (s *Store) transitionSession(ctx context.Context, projectID, workstreamID, sessionID, next, observedBy string, changedAt time.Time, states ...string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := requireSessionInTx(ctx, tx, projectID, workstreamID, sessionID); err != nil {
		return err
	}
	placeholders := strings.TrimRight(strings.Repeat("?,", len(states)), ",")
	arguments := []any{next, utcText(changedAt), nullableValue(observedBy), sessionID, projectID, workstreamID}
	for _, state := range states {
		arguments = append(arguments, state)
	}
	result, err := tx.ExecContext(ctx, `UPDATE sessions SET status = ?, lifecycle_at = ?, disconnect_observed_by = ?
		WHERE session_id = ? AND project_id = ? AND workstream_id = ? AND status IN (`+placeholders+`)`, arguments...)
	if err != nil {
		return continuityError(err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return ContinuityError{Code: continuityConflict}
	}
	return tx.Commit()
}

func (s *Store) requireProject(ctx context.Context, projectID string) error {
	found, err := s.ProjectExists(ctx, projectID)
	if err != nil {
		return err
	}
	if !found {
		return ContinuityError{Code: continuityNotFound}
	}
	return nil
}

func requireProjectInTx(ctx context.Context, tx *sql.Tx, projectID string) error {
	var found int
	if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM projects WHERE project_id = ?", projectID).Scan(&found); err != nil {
		return err
	}
	if found != 1 {
		return ContinuityError{Code: continuityNotFound}
	}
	return nil
}

func (s *Store) requireWorkstream(ctx context.Context, projectID, workstreamID string) error {
	var actualProject string
	err := s.db.QueryRowContext(ctx, "SELECT project_id FROM workstreams WHERE workstream_id = ?", workstreamID).Scan(&actualProject)
	if errors.Is(err, sql.ErrNoRows) {
		return ContinuityError{Code: continuityNotFound}
	}
	if err != nil {
		return err
	}
	if actualProject != projectID {
		return domain.Error{Outcome: domain.BindingMismatch}
	}
	return nil
}

func (s *Store) requireSession(ctx context.Context, projectID, workstreamID, sessionID string) error {
	return requireSessionQuery(ctx, s.db, projectID, workstreamID, sessionID)
}

func requireSessionInTx(ctx context.Context, tx *sql.Tx, projectID, workstreamID, sessionID string) error {
	return requireSessionQuery(ctx, tx, projectID, workstreamID, sessionID)
}

type sessionQueryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func requireSessionQuery(ctx context.Context, queryer sessionQueryer, projectID, workstreamID, sessionID string) error {
	var actualProject, actualWorkstream string
	err := queryer.QueryRowContext(ctx, "SELECT project_id, workstream_id FROM sessions WHERE session_id = ?", sessionID).Scan(&actualProject, &actualWorkstream)
	if errors.Is(err, sql.ErrNoRows) {
		return ContinuityError{Code: continuityNotFound}
	}
	if err != nil {
		return err
	}
	if actualProject != projectID || actualWorkstream != workstreamID {
		return domain.Error{Outcome: domain.BindingMismatch}
	}
	return nil
}

func utcText(value time.Time) string { return value.UTC().Format(time.RFC3339Nano) }

func parseUTCTime(value sql.NullString) time.Time {
	if !value.Valid || value.String == "" {
		return time.Time{}
	}
	parsed, err := time.Parse(time.RFC3339Nano, value.String)
	if err != nil {
		return time.Time{}
	}
	return parsed.UTC()
}

func continuityError(err error) error {
	if err == nil {
		return nil
	}
	if strings.Contains(strings.ToLower(err.Error()), "constraint") || strings.Contains(strings.ToLower(err.Error()), "unique") {
		return ContinuityError{Code: continuityConflict}
	}
	return mapRecoveryError(err)
}
