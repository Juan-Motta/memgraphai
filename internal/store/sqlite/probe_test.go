package sqlite

import (
	"testing"

	"memgraphai/internal/testkit"
)

func TestProbeReportsFTS5AndCommittedTransaction(t *testing.T) {
	result, err := Probe(t.Context(), testkit.TempSQLitePath(t, "probe"), "project")
	if err != nil {
		t.Fatalf("Probe() error = %v", err)
	}
	if got, want := result.Driver, "modernc-sqlite"; got != want {
		t.Fatalf("Probe() driver = %q, want %q", got, want)
	}
	if !result.FTS5CompileOption {
		t.Fatal("Probe() reported FTS5 compile option disabled")
	}
	if got, want := result.FTS5Matches, 1; got != want {
		t.Fatalf("Probe() FTS5 matches = %d, want %d", got, want)
	}
	if got, want := result.TransactionValue, "committed"; got != want {
		t.Fatalf("Probe() transaction value = %q, want %q", got, want)
	}
	if got, want := result.RolledBackRows, 0; got != want {
		t.Fatalf("Probe() rows visible after rollback = %d, want %d", got, want)
	}
}

func TestProbeRollbackLeavesNoRowVisible(t *testing.T) {
	result, err := Probe(t.Context(), testkit.TempSQLitePath(t, "rollback"), "project")
	if err != nil {
		t.Fatalf("Probe() error = %v", err)
	}
	if got, want := result.RolledBackRows, 0; got != want {
		t.Fatalf("Probe() rows visible after rollback = %d, want %d", got, want)
	}
}

func TestProbeUsesTheRequestedFTS5Query(t *testing.T) {
	result, err := Probe(t.Context(), testkit.TempSQLitePath(t, "no-match"), "unavailable")
	if err != nil {
		t.Fatalf("Probe() error = %v", err)
	}
	if got, want := result.FTS5Matches, 0; got != want {
		t.Fatalf("Probe() FTS5 matches = %d, want %d for an absent term", got, want)
	}
}
