package sqlite

import (
	"context"
	"database/sql"
	"fmt"
)

const foundationSchemaVersion = 2

const projectSchemaVersion = 2

var projectSchema = []string{
	`CREATE TABLE IF NOT EXISTS project_views (
		singleton INTEGER PRIMARY KEY CHECK (singleton = 1),
		generation INTEGER NOT NULL
	)`,
	`INSERT OR IGNORE INTO project_views(singleton, generation) VALUES (1, 0)`,
	`CREATE TRIGGER IF NOT EXISTS projects_view_after_insert AFTER INSERT ON projects
		BEGIN UPDATE project_views SET generation = generation + 1 WHERE singleton = 1; END`,
	`CREATE TRIGGER IF NOT EXISTS paths_view_after_insert AFTER INSERT ON project_paths
		BEGIN UPDATE project_views SET generation = generation + 1 WHERE singleton = 1; END`,
	`CREATE TRIGGER IF NOT EXISTS paths_view_after_delete AFTER DELETE ON project_paths
		BEGIN UPDATE project_views SET generation = generation + 1 WHERE singleton = 1; END`,
}

var foundationSchema = []string{
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
	`CREATE TABLE IF NOT EXISTS operation_metrics (
		operation_id TEXT PRIMARY KEY NOT NULL,
		recorded_at TEXT NOT NULL,
		interface TEXT NOT NULL,
		scope_kind TEXT NOT NULL,
		project_id TEXT,
		workstream_id TEXT,
		session_id TEXT,
		outcome TEXT NOT NULL,
		backend_duration_ns INTEGER NOT NULL,
		response_bytes INTEGER NOT NULL,
		result_count INTEGER
	)`,
}

func applyMigrations(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin foundation migration: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY NOT NULL
	)`); err != nil {
		return fmt.Errorf("create migration ledger: %w", err)
	}
	var current int
	if err := tx.QueryRowContext(ctx, "SELECT COALESCE(MAX(version), 0) FROM schema_migrations").Scan(&current); err != nil {
		return fmt.Errorf("read migration ledger: %w", err)
	}
	if current > foundationSchemaVersion {
		return fmt.Errorf("unsupported future schema version %d", current)
	}
	if current < 1 {
		for _, statement := range foundationSchema {
			if _, err := tx.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf("apply foundation migration: %w", err)
			}
		}
		if err := upgradeLegacyOperationColumns(ctx, tx); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO schema_migrations(version) VALUES (1)"); err != nil {
			return fmt.Errorf("record foundation migration: %w", err)
		}
	}
	if current < projectSchemaVersion {
		for _, statement := range projectSchema {
			if _, err := tx.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf("apply project migration: %w", err)
			}
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO schema_migrations(version) VALUES (?)", projectSchemaVersion); err != nil {
			return fmt.Errorf("record project migration: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit foundation migration: %w", err)
	}
	return nil
}

func upgradeLegacyOperationColumns(ctx context.Context, tx *sql.Tx) error {
	rows, err := tx.QueryContext(ctx, "PRAGMA table_info(operations)")
	if err != nil {
		return fmt.Errorf("inspect operation schema: %w", err)
	}
	columns := make(map[string]bool)
	for rows.Next() {
		var index, notNull, primaryKey int
		var name, columnType string
		var defaultValue any
		if err := rows.Scan(&index, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			rows.Close()
			return fmt.Errorf("inspect operation column: %w", err)
		}
		columns[name] = true
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("inspect operation schema rows: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close operation schema rows: %w", err)
	}
	for _, column := range []struct {
		name      string
		statement string
	}{
		{"request_fingerprint", "ALTER TABLE operations ADD COLUMN request_fingerprint TEXT"},
		{"project_id", "ALTER TABLE operations ADD COLUMN project_id TEXT"},
	} {
		if columns[column.name] {
			continue
		}
		if _, err := tx.ExecContext(ctx, column.statement); err != nil {
			return fmt.Errorf("add operation %s: %w", column.name, err)
		}
	}
	return nil
}
