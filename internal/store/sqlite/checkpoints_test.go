package sqlite

import (
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"memgraphai/internal/app"
	"memgraphai/internal/domain"
	"memgraphai/internal/revisionfs"
	"memgraphai/internal/testkit"
)

func TestCheckpointStoreSavesAndReadsAnExactActiveSessionHandoff(t *testing.T) {
	store, files, _ := checkpointFixture(t)
	publishCheckpointSource(t, store, files, "source-document-a", "source-revision-a", "# source\n")
	references := []byte(`[{"document_id":"source-document-a","revision_id":"source-revision-a"}]`)
	createdAt := time.Date(2026, 3, 5, 6, 7, 8, 0, time.UTC)
	if err := store.SaveCheckpoint(t.Context(), files, "checkpoint-operation-a", "checkpoint-a", "checkpoint-document-a", "checkpoint-revision-a", "project-a", "workstream-a", "session-a", []byte("# handoff\n"), references, "cli", "terminal", "reported", createdAt); err != nil {
		t.Fatalf("SaveCheckpoint() error = %v", err)
	}
	raw, err := store.ReadCheckpoint(t.Context(), files, "project-a", "workstream-a", "session-a", "checkpoint-a")
	if err != nil {
		t.Fatalf("ReadCheckpoint() error = %v", err)
	}
	var got struct {
		CheckpointID string `json:"checkpoint_id"`
		ProseBase64  string `json:"prose_base64"`
		References   []struct {
			DocumentID string `json:"document_id"`
			RevisionID string `json:"revision_id"`
			Fresh      bool   `json:"fresh"`
		} `json:"references"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("decode checkpoint result: %v", err)
	}
	if got.CheckpointID != "checkpoint-a" || got.ProseBase64 != "IyBoYW5kb2ZmCg" || len(got.References) != 1 || got.References[0].DocumentID != "source-document-a" || got.References[0].RevisionID != "source-revision-a" || !got.References[0].Fresh {
		t.Fatalf("checkpoint read = %#v, want exact fresh handoff", got)
	}
}

func TestCheckpointStoreComputesFreshnessAndPreservesDisconnectedSourceAcrossResume(t *testing.T) {
	store, files, _ := checkpointFixture(t)
	publishCheckpointSource(t, store, files, "source-document-a", "source-revision-a", "# source\n")
	refs := []byte(`[{"document_id":"source-document-a","revision_id":"source-revision-a"}]`)
	if err := store.SaveCheckpoint(t.Context(), files, "checkpoint-operation-a", "checkpoint-a", "checkpoint-document-a", "checkpoint-revision-a", "project-a", "workstream-a", "session-a", []byte("# handoff\n"), refs, "cli", "", "", time.Now()); err != nil {
		t.Fatalf("SaveCheckpoint() error = %v", err)
	}
	if err := store.DisconnectSession(t.Context(), "project-a", "workstream-a", "session-a", "adapter", time.Now()); err != nil {
		t.Fatalf("DisconnectSession() error = %v", err)
	}
	if err := store.ResumeSession(t.Context(), "project-a", "workstream-a", "session-b", "session-a", "cli", "", "", time.Now()); err != nil {
		t.Fatalf("ResumeSession() error = %v", err)
	}
	if raw, err := store.ReadCheckpoint(t.Context(), files, "project-a", "workstream-a", "session-b", "checkpoint-a"); err != nil || !checkpointReferenceFresh(t, raw) {
		t.Fatalf("ReadCheckpoint(resumed same workstream) = %s, %v; want fresh source handoff", raw, err)
	}
	if outcome, _, err := store.WriteDocument(t.Context(), files, "source-update", "source-document-a", "project-a", "", "source-revision-b", stringPointer("source-revision-a"), []byte("# updated\n"), "cli", "", "", time.Now()); err != nil || outcome != app.OK {
		t.Fatalf("WriteDocument(source update) = %q, %v", outcome, err)
	}
	if raw, err := store.ReadCheckpoint(t.Context(), files, "project-a", "workstream-a", "session-b", "checkpoint-a"); err != nil || checkpointReferenceFresh(t, raw) || !checkpointReferenceHasFreshKey(t, raw) {
		t.Fatalf("ReadCheckpoint(stale exact source) = %s, %v; want explicit fresh:false only after its pointer advances", raw, err)
	}
	if err := store.ForkWorkstream(t.Context(), "project-a", "workstream-a", "workstream-fork", "cli", "", "", time.Now()); err != nil {
		t.Fatalf("ForkWorkstream() error = %v", err)
	}
	if err := store.OpenSession(t.Context(), "project-a", "workstream-fork", "session-fork", "cli", "", "", time.Now()); err != nil {
		t.Fatalf("OpenSession(fork) error = %v", err)
	}
	if _, err := store.ReadCheckpoint(t.Context(), files, "project-a", "workstream-fork", "session-fork", "checkpoint-a"); !checkpointHasCode(err, "scope_denied") {
		t.Fatalf("ReadCheckpoint(fork) error = %v, want scope_denied", err)
	}
}

func TestCheckpointDigestPreservesFrozenPreChangeRequestFixture(t *testing.T) {
	references := []checkpointReference{{DocumentID: "source-document-a", RevisionID: "source-revision-a"}}
	got := checkpointDigest("checkpoint-operation-a", "checkpoint-a", "checkpoint-document-a", "checkpoint-revision-a", "project-a", "workstream-a", "session-a", []byte("# handoff\n"), references, "cli", "terminal", "reported")
	// Frozen from the committed request fixture at dc8de02 before response-only freshness serialization changed.
	const want = "91955649763d7a8ad045338eb42bbe2eb951d3119c809fe22eecdbb4b72d5a29"
	if got != want {
		t.Fatalf("checkpointDigest() = %q, want frozen pre-change fixture %q", got, want)
	}
}

func TestCheckpointStoreRejectsImmutableReplayAndTamperedBackingProse(t *testing.T) {
	store, files, root := checkpointFixture(t)
	publishCheckpointSource(t, store, files, "source-document-a", "source-revision-a", "# source\n")
	publishCheckpointSource(t, store, files, "source-document-b", "source-revision-b", "# source b\n")
	refs := []byte(`[{"document_id":"source-document-a","revision_id":"source-revision-a"}]`)
	if err := store.SaveCheckpoint(t.Context(), files, "checkpoint-operation-a", "checkpoint-a", "checkpoint-document-a", "checkpoint-revision-a", "project-a", "workstream-a", "session-a", []byte("# handoff\n"), refs, "cli", "", "", time.Now()); err != nil {
		t.Fatalf("SaveCheckpoint() error = %v", err)
	}
	changed := []byte(`[{"document_id":"source-document-b","revision_id":"source-revision-b"}]`)
	if err := store.SaveCheckpoint(t.Context(), files, "checkpoint-operation-a", "checkpoint-a", "checkpoint-document-a", "checkpoint-revision-a", "project-a", "workstream-a", "session-a", []byte("# handoff\n"), changed, "cli", "", "", time.Now()); !domain.IsOutcome(err, domain.IdempotencyMismatch) {
		t.Fatalf("SaveCheckpoint(changed refs) error = %v, want %q", err, domain.IdempotencyMismatch)
	}
	if err := os.WriteFile(revisionPathFor(root, "project-a", "checkpoint-document-a", "checkpoint-revision-a"), []byte("altered"), 0o600); err != nil {
		t.Fatalf("tamper checkpoint prose: %v", err)
	}
	if _, err := store.ReadCheckpoint(t.Context(), files, "project-a", "workstream-a", "session-a", "checkpoint-a"); !domain.IsOutcome(err, domain.IntegrityDiscrepancy) {
		t.Fatalf("ReadCheckpoint(tampered) error = %v, want %q", err, domain.IntegrityDiscrepancy)
	}
}

func TestCheckpointStoreKeepsPrecommitRegistrationInvisibleAndReplaysLostResponse(t *testing.T) {
	store, files, root := checkpointFixture(t)
	publishCheckpointSource(t, store, files, "source-document-a", "source-revision-a", "# source\n")
	refs := []byte(`[{"document_id":"source-document-a","revision_id":"source-revision-a"}]`)
	interrupted := revisionfs.New(root, func(point revisionfs.Point) error {
		if point == revisionfs.AfterPublish {
			return errors.New("stop before checkpoint commit")
		}
		return nil
	})
	if err := store.SaveCheckpoint(t.Context(), interrupted, "checkpoint-operation-a", "checkpoint-a", "checkpoint-document-a", "checkpoint-revision-a", "project-a", "workstream-a", "session-a", []byte("# handoff\n"), refs, "cli", "", "", time.Now()); err == nil {
		t.Fatal("SaveCheckpoint(precommit fault) error = nil, want interruption")
	}
	if _, err := store.ReadCheckpoint(t.Context(), files, "project-a", "workstream-a", "session-a", "checkpoint-a"); !checkpointHasCode(err, "not_found") {
		t.Fatalf("ReadCheckpoint(precommit) error = %v, want invisible not_found", err)
	}
	database := checkpointDatabasePath(t, store)
	if err := store.Close(); err != nil {
		t.Fatalf("Close(precommit store) error = %v", err)
	}
	var err error
	store, err = Open(t.Context(), database)
	if err != nil {
		t.Fatalf("Open(restarted store) error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.SaveCheckpoint(t.Context(), files, "checkpoint-operation-a", "checkpoint-a", "checkpoint-document-a", "checkpoint-revision-a", "project-a", "workstream-a", "session-a", []byte("# handoff\n"), refs, "cli", "", "", time.Now()); err != nil {
		t.Fatalf("SaveCheckpoint(retry) error = %v", err)
	}
	store.afterOperationCommit = func() error { return errors.New("lost response") }
	if err := store.SaveCheckpoint(t.Context(), files, "checkpoint-operation-b", "checkpoint-b", "checkpoint-document-b", "checkpoint-revision-b", "project-a", "workstream-a", "session-a", []byte("# handoff b\n"), refs, "cli", "", "", time.Now()); !domain.IsOutcome(err, domain.Unknown) {
		t.Fatalf("SaveCheckpoint(lost response) error = %v, want %q", err, domain.Unknown)
	}
	store.afterOperationCommit = nil
	if raw, err := store.ReadCheckpoint(t.Context(), files, "project-a", "workstream-a", "session-a", "checkpoint-b"); err != nil || len(raw) == 0 {
		t.Fatalf("ReadCheckpoint(reachable lost response) = %s, %v", raw, err)
	}
}

func TestCheckpointSchemaRejectsDirectAggregateMutation(t *testing.T) {
	t.Run("checkpoint deletion", func(t *testing.T) {
		store, _, _ := checkpointFixture(t)
		seedDirectCheckpointWithoutReferences(t, store, "checkpoint-direct", "checkpoint-document-direct", "checkpoint-revision-direct")
		if _, err := store.db.ExecContext(t.Context(), "DELETE FROM checkpoints WHERE checkpoint_id = 'checkpoint-direct'"); err == nil {
			t.Fatal("direct checkpoint DELETE error = nil, want immutable aggregate rejection")
		}
	})

	t.Run("reference update and deletion", func(t *testing.T) {
		store, files, _ := checkpointFixture(t)
		publishCheckpointSource(t, store, files, "source-document-a", "source-revision-a", "# source a\n")
		publishCheckpointSource(t, store, files, "source-document-b", "source-revision-b", "# source b\n")
		publishCheckpointSource(t, store, files, "source-document-c", "source-revision-c", "# source c\n")
		references := []byte(`[{"document_id":"source-document-a","revision_id":"source-revision-a"},{"document_id":"source-document-b","revision_id":"source-revision-b"}]`)
		if err := store.SaveCheckpoint(t.Context(), files, "checkpoint-operation-a", "checkpoint-a", "checkpoint-document-a", "checkpoint-revision-a", "project-a", "workstream-a", "session-a", []byte("# handoff\n"), references, "cli", "", "", time.Now()); err != nil {
			t.Fatalf("SaveCheckpoint() error = %v", err)
		}
		if _, err := store.db.ExecContext(t.Context(), `UPDATE checkpoint_references
			SET document_id = 'source-document-c', revision_id = 'source-revision-c'
			WHERE checkpoint_id = 'checkpoint-a' AND document_id = 'source-document-a'`); err == nil {
			t.Fatal("direct checkpoint reference UPDATE error = nil, want immutable aggregate rejection")
		}
		if _, err := store.db.ExecContext(t.Context(), `DELETE FROM checkpoint_references
			WHERE checkpoint_id = 'checkpoint-a' AND document_id = 'source-document-b'`); err == nil {
			t.Fatal("direct checkpoint reference DELETE error = nil, want immutable aggregate rejection")
		}
	})
}

func TestCheckpointSchemaRejectsDirectReferenceAppendAfterPublication(t *testing.T) {
	t.Run("checkpoint with initial references", func(t *testing.T) {
		store, files, _ := checkpointFixture(t)
		publishCheckpointSource(t, store, files, "source-document-a", "source-revision-a", "# source a\n")
		publishCheckpointSource(t, store, files, "source-document-b", "source-revision-b", "# source b\n")
		references := []byte(`[{"document_id":"source-document-a","revision_id":"source-revision-a"}]`)
		if err := store.SaveCheckpoint(t.Context(), files, "checkpoint-operation-a", "checkpoint-a", "checkpoint-document-a", "checkpoint-revision-a", "project-a", "workstream-a", "session-a", []byte("# handoff\n"), references, "cli", "", "", time.Now()); err != nil {
			t.Fatalf("SaveCheckpoint() error = %v", err)
		}
		if _, err := store.db.ExecContext(t.Context(), `INSERT INTO checkpoint_references(checkpoint_id, document_id, revision_id)
			VALUES ('checkpoint-a', 'source-document-b', 'source-revision-b')`); err == nil {
			t.Fatal("direct valid checkpoint reference append error = nil, want published checkpoint rejection")
		}
	})

	t.Run("checkpoint published without initial references", func(t *testing.T) {
		store, files, _ := checkpointFixture(t)
		publishCheckpointSource(t, store, files, "source-document-a", "source-revision-a", "# source a\n")
		seedDirectCheckpointWithoutReferences(t, store, "checkpoint-empty", "checkpoint-document-empty", "checkpoint-revision-empty")
		if _, err := store.db.ExecContext(t.Context(), `INSERT INTO operations(
			operation_id, fingerprint, request_fingerprint, project_id, document_id, revision_id,
			expected_current_revision_id, state
		) VALUES ('checkpoint-operation-empty', 'digest', 'checkpoint-save', 'project-a',
			'checkpoint-document-empty', 'checkpoint-revision-empty', NULL, 'committed')`); err != nil {
			t.Fatalf("seed terminal checkpoint operation: %v", err)
		}
		if _, err := store.db.ExecContext(t.Context(), `INSERT INTO checkpoint_references(checkpoint_id, document_id, revision_id)
			VALUES ('checkpoint-empty', 'source-document-a', 'source-revision-a')`); err == nil {
			t.Fatal("direct valid append to empty published checkpoint error = nil, want rejection")
		}
	})

	t.Run("publication remains sealed after restart and replay", func(t *testing.T) {
		store, files, _ := checkpointFixture(t)
		publishCheckpointSource(t, store, files, "source-document-a", "source-revision-a", "# source a\n")
		publishCheckpointSource(t, store, files, "source-document-b", "source-revision-b", "# source b\n")
		createdAt := time.Date(2026, 3, 5, 6, 7, 8, 0, time.UTC)
		references := []byte(`[{"document_id":"source-document-a","revision_id":"source-revision-a"}]`)
		if err := store.SaveCheckpoint(t.Context(), files, "checkpoint-operation-a", "checkpoint-a", "checkpoint-document-a", "checkpoint-revision-a", "project-a", "workstream-a", "session-a", []byte("# handoff\n"), references, "cli", "", "", createdAt); err != nil {
			t.Fatalf("SaveCheckpoint() error = %v", err)
		}
		database := checkpointDatabasePath(t, store)
		if err := store.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
		var err error
		store, err = Open(t.Context(), database)
		if err != nil {
			t.Fatalf("Open(restarted) error = %v", err)
		}
		t.Cleanup(func() { _ = store.Close() })
		if err := store.SaveCheckpoint(t.Context(), files, "checkpoint-operation-a", "checkpoint-a", "checkpoint-document-a", "checkpoint-revision-a", "project-a", "workstream-a", "session-a", []byte("# handoff\n"), references, "cli", "", "", createdAt); err != nil {
			t.Fatalf("SaveCheckpoint(replay) error = %v", err)
		}
		if _, err := store.db.ExecContext(t.Context(), `INSERT INTO checkpoint_references(checkpoint_id, document_id, revision_id)
			VALUES ('checkpoint-a', 'source-document-b', 'source-revision-b')`); err == nil {
			t.Fatal("direct valid append after restart and replay error = nil, want rejection")
		}
	})
}

func TestCheckpointStoreRollsBackWhenBackingPointerCASDoesNotAdvance(t *testing.T) {
	store, files, root := checkpointFixture(t)
	publishCheckpointSource(t, store, files, "source-document-a", "source-revision-a", "# source\n")
	references := []byte(`[{"document_id":"source-document-a","revision_id":"source-revision-a"}]`)
	controlled := revisionfs.New(root, func(point revisionfs.Point) error {
		if point != revisionfs.AfterPublish {
			return nil
		}
		if _, err := store.db.ExecContext(t.Context(), `UPDATE documents SET current_revision_id = 'source-revision-a'
			WHERE document_id = 'checkpoint-document-a'`); err != nil {
			t.Fatalf("advance competing checkpoint pointer: %v", err)
		}
		return nil
	})
	if err := store.SaveCheckpoint(t.Context(), controlled, "checkpoint-operation-a", "checkpoint-a", "checkpoint-document-a", "checkpoint-revision-a", "project-a", "workstream-a", "session-a", []byte("# handoff\n"), references, "cli", "", "", time.Now()); !checkpointHasCode(err, "conflict") {
		t.Fatalf("SaveCheckpoint(pointer CAS lost) error = %v, want conflict", err)
	}
	var checkpoints, revisions, committed int
	if err := store.db.QueryRowContext(t.Context(), `SELECT
		(SELECT count(*) FROM checkpoints WHERE checkpoint_id = 'checkpoint-a'),
		(SELECT count(*) FROM revisions WHERE revision_id = 'checkpoint-revision-a'),
		(SELECT count(*) FROM operations WHERE operation_id = 'checkpoint-operation-a' AND state = 'committed')`).Scan(&checkpoints, &revisions, &committed); err != nil {
		t.Fatalf("read pointer CAS rollback state: %v", err)
	}
	if checkpoints != 0 || revisions != 0 || committed != 0 {
		t.Fatalf("pointer CAS rollback state = checkpoints %d / revisions %d / committed %d, want 0/0/0", checkpoints, revisions, committed)
	}
}

func seedDirectCheckpointWithoutReferences(t *testing.T, store *Store, checkpointID, documentID, revisionID string) {
	t.Helper()
	createdAt := "2026-03-05T00:00:00Z"
	if _, err := store.db.ExecContext(t.Context(), `INSERT INTO documents(document_id, project_id, workstream_id, current_revision_id)
		VALUES (?, 'project-a', 'workstream-a', ?)`, documentID, revisionID); err != nil {
		t.Fatalf("seed checkpoint document: %v", err)
	}
	if _, err := store.db.ExecContext(t.Context(), `INSERT INTO revisions(
		revision_id, document_id, predecessor_revision_id, file_path, checksum, byte_count, origin, created_at
	) VALUES (?, ?, NULL, '/checkpoint-direct.md', 'checksum-direct', 1, 'test', ?)`, revisionID, documentID, createdAt); err != nil {
		t.Fatalf("seed checkpoint revision: %v", err)
	}
	if _, err := store.db.ExecContext(t.Context(), `INSERT INTO checkpoints(
		checkpoint_id, checkpoint_document_id, checkpoint_revision_id, project_id, workstream_id,
		session_id, origin, created_at
	) VALUES (?, ?, ?, 'project-a', 'workstream-a', 'session-a', 'test', ?)`, checkpointID, documentID, revisionID, createdAt); err != nil {
		t.Fatalf("seed checkpoint: %v", err)
	}
}

func checkpointReferenceFresh(t *testing.T, raw []byte) bool {
	t.Helper()
	var value struct {
		References []struct {
			Fresh bool `json:"fresh"`
		} `json:"references"`
	}
	if err := json.Unmarshal(raw, &value); err != nil || len(value.References) != 1 {
		t.Fatalf("checkpoint freshness response = %s, %v", raw, err)
	}
	return value.References[0].Fresh
}

func checkpointReferenceHasFreshKey(t *testing.T, raw []byte) bool {
	t.Helper()
	var value struct {
		References []map[string]json.RawMessage `json:"references"`
	}
	if err := json.Unmarshal(raw, &value); err != nil || len(value.References) != 1 {
		t.Fatalf("checkpoint freshness-key response = %s, %v", raw, err)
	}
	_, found := value.References[0]["fresh"]
	return found
}

func checkpointDatabasePath(t *testing.T, store *Store) string {
	t.Helper()
	var sequence int
	var name, path string
	if err := store.db.QueryRowContext(t.Context(), "SELECT seq, name, file FROM pragma_database_list WHERE name = 'main'").Scan(&sequence, &name, &path); err != nil || sequence != 0 || name != "main" || path == "" {
		t.Fatalf("database path = %d/%q/%q, %v", sequence, name, path, err)
	}
	return path
}

func checkpointHasCode(err error, code string) bool {
	var checkpoint CheckpointError
	return errors.As(err, &checkpoint) && checkpoint.Code == code
}

func checkpointFixture(t *testing.T) (*Store, *revisionfs.Filesystem, string) {
	t.Helper()
	store, err := Open(t.Context(), testkit.TempSQLitePath(t, "checkpoint-store"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.CreateProject(t.Context(), "project-a", "A"); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	createdAt := time.Date(2026, 3, 5, 0, 0, 0, 0, time.UTC)
	if err := store.CreateContinuityWorkstream(t.Context(), "project-a", "workstream-a", "cli", "", "", createdAt); err != nil {
		t.Fatalf("CreateContinuityWorkstream() error = %v", err)
	}
	if err := store.OpenSession(t.Context(), "project-a", "workstream-a", "session-a", "cli", "", "", createdAt); err != nil {
		t.Fatalf("OpenSession() error = %v", err)
	}
	root := t.TempDir()
	return store, revisionfs.New(root, nil), root
}

func publishCheckpointSource(t *testing.T, store *Store, files *revisionfs.Filesystem, documentID, revisionID, content string) {
	t.Helper()
	createdAt := time.Date(2026, 3, 5, 1, 2, 3, 0, time.UTC)
	if err := store.RegisterDocumentCreate(t.Context(), "source-operation-"+revisionID, documentID, "project-a", "", revisionID, nil, []byte(content), "cli", "", "", createdAt); err != nil {
		t.Fatalf("RegisterDocumentCreate() error = %v", err)
	}
	if outcome, _, err := store.WriteDocument(t.Context(), files, "source-operation-"+revisionID, documentID, "project-a", "", revisionID, nil, []byte(content), "cli", "", "", createdAt); err != nil || outcome != app.OK {
		t.Fatalf("WriteDocument() = %q, %v", outcome, err)
	}
}
