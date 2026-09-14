package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

// Project is the metadata returned by deterministic project listings.
type Project struct {
	ID   string
	Name string
}

// ProjectList is one SQLite-consistent keyset page and its view generation.
type ProjectList struct {
	Projects       []Project
	ViewGeneration int64
}

// ProjectError lets the application map expected persistence outcomes without
// exposing driver messages through an adapter.
type ProjectError struct{ Code string }

func (e ProjectError) Error() string       { return e.Code }
func (e ProjectError) OutcomeCode() string { return e.Code }

const (
	projectConflict = "conflict"
	projectNotFound = "not_found"
)

// ListProjects returns a bounded immutable-ID keyset page from one view.
func (s *Store) ListProjects(ctx context.Context, afterID string, limit int) (ProjectList, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return ProjectList{}, err
	}
	defer tx.Rollback()
	var generation int64
	if err := tx.QueryRowContext(ctx, "SELECT generation FROM project_views WHERE singleton = 1").Scan(&generation); err != nil {
		return ProjectList{}, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT project_id, display_name FROM projects
		WHERE project_id > ? ORDER BY project_id LIMIT ?`, afterID, limit)
	if err != nil {
		return ProjectList{}, err
	}
	defer rows.Close()
	projects := make([]Project, 0, limit)
	for rows.Next() {
		var project Project
		if err := rows.Scan(&project.ID, &project.Name); err != nil {
			return ProjectList{}, err
		}
		projects = append(projects, project)
	}
	if err := rows.Err(); err != nil {
		return ProjectList{}, err
	}
	if err := tx.Commit(); err != nil {
		return ProjectList{}, err
	}
	return ProjectList{Projects: projects, ViewGeneration: generation}, nil
}

// VisitProjects lets the transport-neutral service consume a bounded page without
// importing this persistence package or treating it as a generic repository.
func (s *Store) VisitProjects(ctx context.Context, afterID string, limit int, visit func(id, name string)) (int64, error) {
	page, err := s.ListProjects(ctx, afterID, limit)
	if err != nil {
		return 0, err
	}
	for _, project := range page.Projects {
		visit(project.ID, project.Name)
	}
	return page.ViewGeneration, nil
}

// ProjectExists determines whether an explicit project identity is present.
func (s *Store) ProjectExists(ctx context.Context, projectID string) (bool, error) {
	var found int
	err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM projects WHERE project_id = ?", projectID).Scan(&found)
	return found == 1, err
}

// AddProjectPath creates a path association when the exact association is new.
// It requires an initially existing path and preserves associations across projects.
func (s *Store) AddProjectPath(ctx context.Context, projectID, path string) error {
	found, err := s.ProjectExists(ctx, projectID)
	if err != nil {
		return err
	}
	if !found {
		return ProjectError{Code: projectNotFound}
	}
	resolved, err := resolveExistingPath(path)
	if err != nil {
		return err
	}
	supplied, err := absolutePath(path)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO project_paths(project_id, supplied_path, resolved_path)
		VALUES (?, ?, ?) ON CONFLICT(project_id, supplied_path) DO NOTHING`, projectID, supplied, resolved)
	return err
}

// VisitProjectPaths reads one bounded supplied-path keyset page and its view generation.
func (s *Store) VisitProjectPaths(ctx context.Context, projectID, afterPath string, limit int, visit func(string)) (int64, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var generation int64
	if err := tx.QueryRowContext(ctx, "SELECT generation FROM project_views WHERE singleton = 1").Scan(&generation); err != nil {
		return 0, err
	}
	var found int
	if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM projects WHERE project_id = ?", projectID).Scan(&found); err != nil {
		return 0, err
	}
	if found != 1 {
		return 0, ProjectError{Code: projectNotFound}
	}
	rows, err := tx.QueryContext(ctx, `SELECT supplied_path FROM project_paths
		WHERE project_id = ? AND supplied_path > ? ORDER BY supplied_path LIMIT ?`, projectID, afterPath, limit)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			return 0, err
		}
		visit(path)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return generation, nil
}

// RemoveProjectPath removes only the exact supplied-path association.
func (s *Store) RemoveProjectPath(ctx context.Context, projectID, path string) error {
	found, err := s.ProjectExists(ctx, projectID)
	if err != nil {
		return err
	}
	if !found {
		return ProjectError{Code: projectNotFound}
	}
	supplied, err := absolutePath(path)
	if err != nil {
		return err
	}
	result, err := s.db.ExecContext(ctx, "DELETE FROM project_paths WHERE project_id = ? AND supplied_path = ?", projectID, supplied)
	if err != nil {
		return err
	}
	removed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if removed != 1 {
		return ProjectError{Code: projectNotFound}
	}
	return nil
}

func projectError(err error) error {
	if err == nil {
		return nil
	}
	if strings.Contains(err.Error(), "constraint") || strings.Contains(err.Error(), "UNIQUE") {
		return ProjectError{Code: projectConflict}
	}
	return err
}

func isProjectNotFound(err error) bool {
	var projectError ProjectError
	return errors.As(err, &projectError) && projectError.Code == projectNotFound
}
