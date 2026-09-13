package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"memgraphai/internal/domain"
	"memgraphai/internal/telemetry"
)

// Store owns narrow Phase 0 identity and publication experiments.
type Store struct {
	db                   *sql.DB
	beforeCommit         func() error
	afterCommit          func() error
	afterOperationCommit func() error
}

// Open creates the Phase 0 identity schema in an isolated SQLite database.
func Open(ctx context.Context, dsn string) (*Store, error) {
	db, err := sql.Open(probeDriver, foreignKeyDSN(dsn))
	if err != nil {
		return nil, fmt.Errorf("open identity store: %w", err)
	}
	store := &Store{db: db}
	if err := store.initialize(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) initialize(ctx context.Context) error {
	return applyMigrations(ctx, s.db)
}

// Close releases the underlying test database.
func (s *Store) Close() error { return s.db.Close() }

// Record persists a bounded Phase 1 metric without consulting it for business behavior.
func (s *Store) Record(ctx context.Context, operation telemetry.Operation) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO operation_metrics(
		operation_id, recorded_at, interface, scope_kind, project_id, workstream_id,
		session_id, outcome, backend_duration_ns, response_bytes, result_count
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		operation.OperationID, operation.RecordedAt.UTC().Format(time.RFC3339Nano), operation.Interface,
		operation.ScopeKind, nullableValue(operation.ProjectID), nullableValue(operation.WorkstreamID),
		nullableValue(operation.SessionID), operation.Outcome, operation.BackendDuration.Nanoseconds(),
		operation.ResponseBytes, nullableCount(operation.ResultCount))
	return err
}

func nullableCount(value *int) any {
	if value == nil {
		return nil
	}
	return *value
}

func (s *Store) CreateProject(ctx context.Context, id, name string) error {
	_, err := s.db.ExecContext(ctx, "INSERT INTO projects(project_id, display_name) VALUES (?, ?)", id, name)
	return err
}

// RenameProjectID reaches the SQLite immutability trigger instead of changing identity.
func (s *Store) RenameProjectID(ctx context.Context, from, to string) error {
	_, err := s.db.ExecContext(ctx, "UPDATE projects SET project_id = ? WHERE project_id = ?", to, from)
	if err != nil && strings.Contains(err.Error(), "project_id_immutable") {
		return domain.Error{Outcome: domain.Immutable}
	}
	return err
}

func (s *Store) ProjectName(ctx context.Context, id string) (string, error) {
	var name string
	err := s.db.QueryRowContext(ctx, "SELECT display_name FROM projects WHERE project_id = ?", id).Scan(&name)
	return name, err
}

// AssociatePath records supplied spelling separately from its resolved identity.
func (s *Store) AssociatePath(ctx context.Context, projectID, path string) error {
	resolved, err := resolveExistingPath(path)
	if err != nil {
		return err
	}
	supplied, err := absolutePath(path)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx,
		"INSERT INTO project_paths(project_id, supplied_path, resolved_path) VALUES (?, ?, ?)",
		projectID, supplied, resolved,
	)
	return err
}

// ResolvePath returns explicit association outcomes and never uses a path prefix.
func (s *Store) ResolvePath(ctx context.Context, path string) (domain.Resolution, error) {
	supplied, err := absolutePath(path)
	if err != nil {
		return domain.Resolution{}, err
	}
	resolved, err := resolveExistingPath(path)
	if err != nil {
		var matches int
		err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM project_paths WHERE supplied_path = ?", supplied).Scan(&matches)
		if err != nil {
			return domain.Resolution{}, err
		}
		if matches > 0 {
			return domain.Resolution{Outcome: domain.Unavailable}, nil
		}
		return domain.Resolution{Outcome: domain.None}, nil
	}

	rows, err := s.db.QueryContext(ctx,
		"SELECT project_id FROM project_paths WHERE resolved_path = ? ORDER BY project_id", resolved)
	if err != nil {
		return domain.Resolution{}, err
	}
	defer rows.Close()

	projects := make([]domain.ProjectID, 0)
	for rows.Next() {
		var id domain.ProjectID
		if err := rows.Scan(&id); err != nil {
			return domain.Resolution{}, err
		}
		if len(projects) == 0 || projects[len(projects)-1] != id {
			projects = append(projects, id)
		}
	}
	if err := rows.Err(); err != nil {
		return domain.Resolution{}, err
	}
	switch len(projects) {
	case 0:
		return domain.Resolution{Outcome: domain.None}, nil
	case 1:
		return domain.Resolution{Outcome: domain.One, Projects: projects}, nil
	default:
		return domain.Resolution{Outcome: domain.Many, Projects: projects}, nil
	}
}

func (s *Store) CreateWorkstream(ctx context.Context, id, projectID string) error {
	_, err := s.db.ExecContext(ctx, "INSERT INTO workstreams(workstream_id, project_id) VALUES (?, ?)", id, projectID)
	return err
}

func (s *Store) CreateSession(ctx context.Context, id, projectID, workstreamID string) error {
	if err := s.validateWorkstreamProject(ctx, projectID, workstreamID); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx,
		"INSERT INTO sessions(session_id, project_id, workstream_id) VALUES (?, ?, ?)", id, projectID, workstreamID)
	return err
}

// ValidateBinding requires every supplied identity to describe one stored relationship.
func (s *Store) ValidateBinding(ctx context.Context, projectID, workstreamID, sessionID string) error {
	var matches int
	err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM sessions
		WHERE session_id = ? AND project_id = ? AND workstream_id = ?`, sessionID, projectID, workstreamID).Scan(&matches)
	if err != nil {
		return err
	}
	if matches != 1 {
		return domain.Error{Outcome: domain.BindingMismatch}
	}
	return nil
}

func foreignKeyDSN(dsn string) string {
	separator := "?"
	if strings.Contains(dsn, "?") {
		separator = "&"
	}
	return dsn + separator + "_pragma=foreign_keys(1)&_pragma=busy_timeout(100)"
}

func (s *Store) validateWorkstreamProject(ctx context.Context, projectID, workstreamID string) error {
	var matches int
	err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM workstreams
		WHERE workstream_id = ? AND project_id = ?`, workstreamID, projectID).Scan(&matches)
	if err != nil {
		return err
	}
	if matches != 1 {
		return domain.Error{Outcome: domain.BindingMismatch}
	}
	return nil
}

func absolutePath(path string) (string, error) {
	return filepath.Abs(filepath.Clean(path))
}

func resolveExistingPath(path string) (string, error) {
	if _, err := os.Stat(path); err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	return absolutePath(resolved)
}
