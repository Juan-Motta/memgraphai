package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

const probeDriver = "sqlite"

// ProbeResult captures the minimal SQLite capabilities needed for Phase 0 evidence.
type ProbeResult struct {
	Driver            string
	FTS5CompileOption bool
	FTS5Matches       int
	TransactionValue  string
	RolledBackRows    int
}

// Probe opens an isolated SQLite database and exercises FTS5 plus a committed transaction.
func Probe(ctx context.Context, dsn, query string) (ProbeResult, error) {
	if dsn == "" {
		dsn = ":memory:"
	}

	db, err := sql.Open(probeDriver, dsn)
	if err != nil {
		return ProbeResult{}, fmt.Errorf("open SQLite probe database: %w", err)
	}
	defer db.Close()

	var compileOption int
	if err := db.QueryRowContext(ctx, "SELECT sqlite_compileoption_used('ENABLE_FTS5')").Scan(&compileOption); err != nil {
		return ProbeResult{}, fmt.Errorf("read FTS5 compile option: %w", err)
	}
	if _, err := db.ExecContext(ctx, "CREATE VIRTUAL TABLE probe_documents USING fts5(body)"); err != nil {
		return ProbeResult{}, fmt.Errorf("create FTS5 probe table: %w", err)
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO probe_documents(body) VALUES ('durable project knowledge')"); err != nil {
		return ProbeResult{}, fmt.Errorf("insert FTS5 probe document: %w", err)
	}

	var matches int
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM probe_documents WHERE probe_documents MATCH ?", query).Scan(&matches); err != nil {
		return ProbeResult{}, fmt.Errorf("query FTS5 probe table: %w", err)
	}
	if _, err := db.ExecContext(ctx, "CREATE TABLE probe_transactions (value TEXT)"); err != nil {
		return ProbeResult{}, fmt.Errorf("create transaction probe table: %w", err)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return ProbeResult{}, fmt.Errorf("begin transaction probe: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO probe_transactions(value) VALUES ('committed')"); err != nil {
		tx.Rollback()
		return ProbeResult{}, fmt.Errorf("insert transaction probe value: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return ProbeResult{}, fmt.Errorf("commit transaction probe: %w", err)
	}

	var transactionValue string
	if err := db.QueryRowContext(ctx, "SELECT value FROM probe_transactions").Scan(&transactionValue); err != nil {
		return ProbeResult{}, fmt.Errorf("read committed transaction probe value: %w", err)
	}

	rollbackTx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return ProbeResult{}, fmt.Errorf("begin rollback transaction probe: %w", err)
	}
	if _, err := rollbackTx.ExecContext(ctx, "INSERT INTO probe_transactions(value) VALUES ('rolled back')"); err != nil {
		rollbackTx.Rollback()
		return ProbeResult{}, fmt.Errorf("insert rollback transaction probe value: %w", err)
	}
	if err := rollbackTx.Rollback(); err != nil {
		return ProbeResult{}, fmt.Errorf("rollback transaction probe: %w", err)
	}

	var rolledBackRows int
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM probe_transactions WHERE value = 'rolled back'").Scan(&rolledBackRows); err != nil {
		return ProbeResult{}, fmt.Errorf("read rollback transaction probe value: %w", err)
	}

	return ProbeResult{
		Driver:            "modernc-sqlite",
		FTS5CompileOption: compileOption == 1,
		FTS5Matches:       matches,
		TransactionValue:  transactionValue,
		RolledBackRows:    rolledBackRows,
	}, nil
}
