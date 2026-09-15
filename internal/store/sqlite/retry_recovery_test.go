package sqlite

import (
	"errors"
	"memgraphai/internal/revisionfs"
	"strings"
	"testing"
	"time"
)

func TestDocumentCreateRecoversAfterRepeatedPreparationFailures(t *testing.T) {
	store, files, root := documentFixture(t)
	created := time.Now()
	write := documentCreateWrite("op-retry", "doc-retry", "rev-retry", nil, "content", "", created)
	if err := registerDocumentWrite(t, store, write); err != nil {
		t.Fatal(err)
	}
	interrupted := revisionfs.New(root, func(point revisionfs.Point) error {
		if point == revisionfs.BeforePublish {
			return errors.New("injected interruption")
		}
		return nil
	})
	for i := 0; i < 4; i++ {
		if _, _, err := store.WriteDocument(t.Context(), interrupted, write.OperationID, write.DocumentID, "project-a", "", write.RevisionID, nil, write.Content, "cli", "", "reported", created); err == nil {
			t.Errorf("interrupted attempt %d did not reach filesystem", i+1)
		}
	}
	outcome, revision, err := store.WriteDocument(t.Context(), files, write.OperationID, write.DocumentID, "project-a", "", write.RevisionID, nil, write.Content, "cli", "", "reported", created)
	if err != nil || outcome != "ok" || revision != write.RevisionID {
		t.Fatalf("explicit retry after repaired filesystem=%s/%s/%v, want ok", outcome, revision, err)
	}
	if _, content, err := store.ReadDocument(t.Context(), files, "project-a", "", write.DocumentID, nil); err != nil || string(content) != "content" {
		t.Fatalf("read recovered=%q/%v", content, err)
	}
}

func TestCheckpointSaveRecoversAfterRepeatedPreparationFailures(t *testing.T) {
	store, files, root := checkpointFixture(t)
	publishCheckpointSource(t, store, files, "source", "source-rev", "source")
	refs := []byte(`[{"document_id":"source","revision_id":"source-rev"}]`)
	interrupted := revisionfs.New(root, func(point revisionfs.Point) error {
		if point == revisionfs.BeforePublish {
			return errors.New("injected interruption")
		}
		return nil
	})
	save := func(fs *revisionfs.Filesystem) error {
		return store.SaveCheckpoint(t.Context(), fs, "op-retry", "checkpoint", "checkpoint-doc", "checkpoint-rev", "project-a", "workstream-a", "session-a", []byte("handoff"), refs, "cli", "", "", time.Now())
	}
	for i := 0; i < 4; i++ {
		if err := save(interrupted); err == nil {
			t.Fatalf("interrupted attempt %d succeeded", i+1)
		}
	}
	if err := save(files); err != nil {
		t.Fatalf("explicit retry after repaired filesystem=%v, want success", err)
	}
	if _, err := store.ReadCheckpoint(t.Context(), files, "project-a", "workstream-a", "session-a", "checkpoint"); err != nil {
		t.Fatal(err)
	}
}

func TestDocumentRegistrationRejectsUnsafePathIDsWithoutRows(t *testing.T) {
	for _, field := range []string{"operation", "document", "revision"} {
		t.Run(field, func(t *testing.T) {
			store, _, _ := documentFixture(t)
			write := documentCreateWrite("op", "doc", "rev", nil, "content", "", time.Now())
			switch field {
			case "operation":
				write.OperationID = "bad/op"
			case "document":
				write.DocumentID = "bad/doc"
			case "revision":
				write.RevisionID = "bad/rev"
			}
			err := registerDocumentWrite(t, store, write)
			coded, ok := err.(DocumentError)
			if !ok || coded.Code != "invalid" {
				t.Errorf("registration=%v, want invalid", err)
			}
			for _, table := range []string{"documents", "operations"} {
				var count int
				if err := store.db.QueryRowContext(t.Context(), "SELECT count(*) FROM "+table).Scan(&count); err != nil || count != 0 {
					t.Errorf("%s count=%d/%v, want no writes", table, count, err)
				}
			}
		})
	}
}

func TestDocumentRegistrationRejectsFilesystemOversizeOperationWithoutRows(t *testing.T) {
	store, _, _ := documentFixture(t)
	write := documentCreateWrite(strings.Repeat("o", 256), "doc", "rev", nil, "content", "", time.Now())
	err := registerDocumentWrite(t, store, write)
	if coded, ok := err.(DocumentError); !ok || coded.Code != "invalid" {
		t.Errorf("register=%v, want invalid", err)
	}
	var count int
	if err := store.db.QueryRowContext(t.Context(), `SELECT count(*) FROM documents`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("documents=%d/%v, want no reservation", count, err)
	}
}
