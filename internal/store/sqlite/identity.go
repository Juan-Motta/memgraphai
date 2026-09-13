package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"memgraphai/internal/domain"
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
	for _, statement := range []string{
		"PRAGMA foreign_keys = ON",
		`CREATE TABLE IF NOT EXISTS projects (
			project_id TEXT PRIMARY KEY NOT NULL,
			display_name TEXT NOT NULL
		)`,
		`CREATE TRIGGER IF NOT EXISTS projects_id_immutable
			BEFORE UPDATE OF project_id ON projects
			BEGIN SELECT RAISE(ABORT, 'project_id_immutable'); END`,
		`CREATE TABLE IF NOT EXISTS project_paths (
			project_id TEXT NOT NULL REFERENCES projects(project_id),
			supplied_path TEXT NOT NULL,
			resolved_path TEXT NOT NULL,
			PRIMARY KEY (project_id, supplied_path)
		)`,
		`CREATE TABLE IF NOT EXISTS workstreams (
			workstream_id TEXT PRIMARY KEY NOT NULL,
			project_id TEXT NOT NULL REFERENCES projects(project_id),
			UNIQUE (workstream_id, project_id)
		)`,
		`CREATE TABLE IF NOT EXISTS sessions (
			session_id TEXT PRIMARY KEY NOT NULL,
			project_id TEXT NOT NULL,
			workstream_id TEXT NOT NULL,
			FOREIGN KEY (workstream_id, project_id)
				REFERENCES workstreams(workstream_id, project_id)
		)`,
		`CREATE TABLE IF NOT EXISTS documents (
			document_id TEXT PRIMARY KEY NOT NULL,
			project_id TEXT NOT NULL REFERENCES projects(project_id),
			current_revision_id TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS revisions (
			revision_id TEXT PRIMARY KEY NOT NULL,
			document_id TEXT NOT NULL REFERENCES documents(document_id),
			predecessor_revision_id TEXT,
			file_path TEXT NOT NULL,
			checksum TEXT NOT NULL,
			byte_count INTEGER NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS operations (
			operation_id TEXT PRIMARY KEY NOT NULL,
			fingerprint TEXT NOT NULL,
			request_fingerprint TEXT NOT NULL,
			project_id TEXT NOT NULL REFERENCES projects(project_id),
			document_id TEXT NOT NULL REFERENCES documents(document_id),
			revision_id TEXT NOT NULL,
			expected_current_revision_id TEXT,
			owner_generation INTEGER NOT NULL DEFAULT 0,
			state TEXT NOT NULL,
			result_outcome TEXT,
			result_revision_id TEXT
		)`,
	} {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("initialize identity schema: %w", err)
		}
	}
	if err := s.ensureOperationColumns(ctx); err != nil {
		return err
	}
	return nil
}

func (s *Store) ensureOperationColumns(ctx context.Context) error {
	rows, err := s.db.QueryContext(ctx, "PRAGMA table_info(operations)")
	if err != nil {
		return fmt.Errorf("inspect operation schema: %w", err)
	}
	defer rows.Close()
	columns := map[string]bool{}
	for rows.Next() {
		var index int
		var name, typ string
		var notNull, primaryKey int
		var defaultValue any
		if err := rows.Scan(&index, &name, &typ, &notNull, &defaultValue, &primaryKey); err != nil {
			return fmt.Errorf("inspect operation column: %w", err)
		}
		columns[name] = true
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("inspect operation schema rows: %w", err)
	}
	for name, statement := range map[string]string{
		"request_fingerprint": "ALTER TABLE operations ADD COLUMN request_fingerprint TEXT",
		"project_id":          "ALTER TABLE operations ADD COLUMN project_id TEXT",
	} {
		if !columns[name] {
			if _, err := s.db.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf("add operation %s: %w", name, err)
			}
		}
	}
	return nil
}

// Close releases the underlying test database.
func (s *Store) Close() error { return s.db.Close() }

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
