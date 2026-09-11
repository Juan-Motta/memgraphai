package main

import (
	"os/exec"
	"strings"
	"testing"
)

func TestProbeCommandReportsSQLiteEvidence(t *testing.T) {
	command := exec.Command("go", "run", ".", "probe")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("go run . probe error = %v\noutput:\n%s", err, output)
	}

	if got, want := strings.TrimSpace(string(output)), "memgraphai: SQLite FTS5 probe succeeded (matches=1, transaction=committed)"; got != want {
		t.Fatalf("go run . probe output = %q, want %q", got, want)
	}
}

func TestCommandRejectsAnUnknownArgument(t *testing.T) {
	command := exec.Command("go", "run", ".", "unknown")
	output, err := command.CombinedOutput()
	if err == nil {
		t.Fatalf("go run . unknown succeeded with output %q", output)
	}

	if got, want := strings.SplitN(strings.TrimSpace(string(output)), "\n", 2)[0], "usage: memgraphai probe"; got != want {
		t.Fatalf("go run . unknown application output = %q, want %q", got, want)
	}
}
