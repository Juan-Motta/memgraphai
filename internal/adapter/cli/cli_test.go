package cli

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestCLIRecordsExactNewlineDelimitedOutputBytes(t *testing.T) {
	library := t.TempDir()
	var stdout, stderr bytes.Buffer
	if code := Run(t.Context(), []string{"--library", library, "--json", "--operation-id", "cli-wire-bytes", "project", "create", "--id", "project-a", "--name", "Alpha"}, nil, &stdout, &stderr); code != 0 {
		t.Fatalf("Run() code = %d, stderr = %q", code, stderr.String())
	}
	db, err := sql.Open("sqlite", filepath.Join(library, "database.sqlite"))
	if err != nil {
		t.Fatalf("open metrics database: %v", err)
	}
	defer db.Close()
	var recorded int
	if err := db.QueryRowContext(t.Context(), "SELECT response_bytes FROM operation_metrics WHERE operation_id = 'cli-wire-bytes'").Scan(&recorded); err != nil {
		t.Fatalf("read CLI metric: %v", err)
	}
	if recorded != stdout.Len() {
		t.Fatalf("CLI response bytes = %d, emitted bytes = %d; want newline-delimited final output", recorded, stdout.Len())
	}
}

func TestHumanCreateBuildsTheSharedJSONEnvelope(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run(t.Context(), []string{"--library", t.TempDir(), "--json", "--operation-id", "cli-create", "project", "create", "--id", "project-a", "--name", "Alpha"}, bytes.NewReader(nil), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run() code = %d, stderr = %q; want success", code, stderr.String())
	}
	var response struct {
		Outcome struct {
			Code string `json:"code"`
		} `json:"outcome"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Outcome.Code != "ok" {
		t.Fatalf("human create outcome = %q, want ok", response.Outcome.Code)
	}
}
