package sqlite

import (
	"memgraphai/internal/domain"
	"memgraphai/internal/revisionfs"
	"os"
	"testing"
	"time"
)

func TestRevisionCollisionDoesNotReserveDocumentOrPrepareFile(t *testing.T) {
	store, files, root := documentFixture(t)
	publishDocument(t, store, files, "op-original", "original", "shared-rev", nil, "original", "", time.Now())
	write := documentCreateWrite("op-collision", "collision", "shared-rev", nil, "different", "", time.Now())
	err := registerDocumentWrite(t, store, write)
	if !domain.IsOutcome(err, domain.Conflict) {
		t.Errorf("register collision=%v, want conflict", err)
	}
	var count int
	if err := store.db.QueryRowContext(t.Context(), `SELECT count(*) FROM documents WHERE document_id='collision'`).Scan(&count); err != nil || count != 0 {
		t.Errorf("collision reservation count=%d/%v", count, err)
	}
	if _, err := os.Stat(revisionPathFor(root, "project-a", "collision", "shared-rev")); !os.IsNotExist(err) {
		t.Errorf("collision file exists or stat failed: %v", err)
	}
	if _, content, err := store.ReadDocument(t.Context(), files, "project-a", "", "original", nil); err != nil || string(content) != "original" {
		t.Fatalf("original=%q/%v", content, err)
	}
}

// Both operations have claimed and prepared before either commits: the commit
// constraint must decide the winner, rather than relying on the initial check.
func TestRevisionCollisionBetweenPreparedOwnersReturnsTerminalConflict(t *testing.T) {
	fixture := newRecoveryFixture(t)
	if err := fixture.store.CreateDocument(t.Context(), "document-2", "project-1"); err != nil {
		t.Fatal(err)
	}
	first := operationRequest("op-first", "first", "shared-rev", nil, "original")
	second := operationRequest("op-second", "second", "shared-rev", nil, "different")
	second.DocumentID = "document-2"
	claimFirst, err := fixture.store.ClaimOperation(t.Context(), first)
	if err != nil {
		t.Fatal(err)
	}
	claimSecond, err := fixture.store.ClaimOperation(t.Context(), second)
	if err != nil {
		t.Fatal(err)
	}
	preparedFirst, err := fixture.files.PrepareAttempt(first.ID, claimFirst.Generation, first.ProjectID, first.DocumentID, first.RevisionID, first.Markdown)
	if err != nil {
		t.Fatal(err)
	}
	preparedSecond, err := fixture.files.PrepareAttempt(second.ID, claimSecond.Generation, second.ProjectID, second.DocumentID, second.RevisionID, second.Markdown)
	if err != nil {
		t.Fatal(err)
	}
	if result, err := fixture.store.commitOperation(t.Context(), first, preparedFirst, claimFirst.Generation); err != nil || result.Outcome != domain.Outcome(operationCommitted) {
		t.Fatalf("first=%+v/%v", result, err)
	}
	result, err := fixture.store.commitOperation(t.Context(), second, preparedSecond, claimSecond.Generation)
	if err != nil || result.Outcome != domain.Conflict {
		t.Fatalf("second=%+v/%v, want conflict", result, err)
	}
	replay, err := fixture.store.ExecuteOperation(t.Context(), fixture.files, second, allow)
	if err != nil || replay.Outcome != domain.Conflict || replay.Generation != claimSecond.Generation {
		t.Fatalf("replay=%+v/%v, want stable conflict without new claim", replay, err)
	}
	if _, content, err := fixture.store.ReadDocument(t.Context(), fixture.files, "project-1", "", first.DocumentID, nil); err != nil || string(content) != "original" {
		t.Fatalf("original=%q/%v", content, err)
	}
}

func TestCheckpointRevisionCollisionDoesNotReserveBackingDocument(t *testing.T) {
	store, files, _ := checkpointFixture(t)
	publishCheckpointSource(t, store, files, "source", "shared-rev", "original")
	refs := []byte(`[{"document_id":"source","revision_id":"shared-rev"}]`)
	err := store.SaveCheckpoint(t.Context(), files, "op-collision", "checkpoint", "checkpoint-doc", "shared-rev", "project-a", "workstream-a", "session-a", []byte("handoff"), refs, "cli", "", "", time.Now())
	if !domain.IsOutcome(err, domain.Conflict) {
		t.Fatalf("collision=%v, want conflict", err)
	}
	var count int
	if err := store.db.QueryRowContext(t.Context(), `SELECT count(*) FROM documents WHERE document_id='checkpoint-doc'`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("reservation count=%d/%v", count, err)
	}
}

func TestCheckpointPreparedRevisionCollisionPersistsConflict(t *testing.T) {
	store, files, _ := checkpointFixture(t)
	publishCheckpointSource(t, store, files, "source", "source-rev", "source")
	refs := []checkpointReference{{DocumentID: "source", RevisionID: "source-rev"}}
	digest := checkpointDigest("checkpoint-op", "checkpoint", "checkpoint-doc", "shared-rev", "project-a", "workstream-a", "session-a", []byte("handoff"), refs, "cli", "", "")
	claim, err := store.claimCheckpoint(t.Context(), "checkpoint-op", "checkpoint", "checkpoint-doc", "shared-rev", "project-a", "workstream-a", "session-a", digest, refs)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := files.PrepareAttempt("checkpoint-op", claim.generation, "project-a", "checkpoint-doc", "shared-rev", []byte("handoff"))
	if err != nil {
		t.Fatal(err)
	}
	publishCheckpointSource(t, store, files, "winner", "shared-rev", "winner")
	err = store.commitCheckpoint(t.Context(), "checkpoint-op", "checkpoint", "checkpoint-doc", "shared-rev", "project-a", "workstream-a", "session-a", digest, refs, "cli", "", "", time.Now(), prepared, claim.generation)
	if !checkpointHasCode(err, "conflict") {
		t.Fatalf("commit collision=%v, want conflict", err)
	}
	err = store.SaveCheckpoint(t.Context(), files, "checkpoint-op", "checkpoint", "checkpoint-doc", "shared-rev", "project-a", "workstream-a", "session-a", []byte("handoff"), []byte(`[{"document_id":"source","revision_id":"source-rev"}]`), "cli", "", "", time.Now())
	if !checkpointHasCode(err, "conflict") {
		t.Fatalf("replay collision=%v, want durable conflict", err)
	}
	if _, content, err := store.ReadDocument(t.Context(), files, "project-a", "", "winner", nil); err != nil || string(content) != "winner" {
		t.Fatalf("winner=%q/%v", content, err)
	}
}

func TestConcurrentRevisionCollisionKeepsOneWinner(t *testing.T) {
	fixture := newRecoveryFixture(t)
	other, err := Open(t.Context(), fixture.database)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	if err := fixture.store.CreateDocument(t.Context(), "document-2", "project-1"); err != nil {
		t.Fatal(err)
	}
	requests := []OperationRequest{operationRequest("first", "first", "shared", nil, "first"), operationRequest("second", "second", "shared", nil, "second")}
	requests[1].DocumentID = "document-2"
	stores := []*Store{fixture.store, other}
	prepared := make([]revisionfs.Prepared, 2)
	generations := make([]int64, 2)
	for i, request := range requests {
		claim, err := stores[i].ClaimOperation(t.Context(), request)
		if err != nil {
			t.Fatal(err)
		}
		generations[i] = claim.Generation
		prepared[i], err = fixture.files.PrepareAttempt(request.ID, claim.Generation, request.ProjectID, request.DocumentID, request.RevisionID, request.Markdown)
		if err != nil {
			t.Fatal(err)
		}
	}
	type completion struct {
		index  int
		result OperationResult
		err    error
	}
	done := make(chan completion, 2)
	start := make(chan struct{})
	for i := range requests {
		go func() {
			<-start
			result, err := stores[i].commitOperation(t.Context(), requests[i], prepared[i], generations[i])
			done <- completion{i, result, err}
		}()
	}
	close(start)
	winner := -1
	loser := -1
	for range requests {
		finished := <-done
		if finished.err == nil && finished.result.Outcome == domain.Outcome(operationCommitted) {
			if winner != -1 {
				t.Fatal("two revision owners committed")
			}
			winner = finished.index
		} else if domain.IsOutcome(finished.err, domain.Busy) || finished.err == nil && finished.result.Outcome == domain.Conflict {
			loser = finished.index
		} else {
			t.Fatalf("unexpected concurrent result=%+v/%v", finished.result, finished.err)
		}
	}
	if winner < 0 || loser < 0 {
		t.Fatalf("winner=%d loser=%d", winner, loser)
	}
	replay, err := stores[loser].ExecuteOperation(t.Context(), fixture.files, requests[loser], allow)
	if !domain.IsOutcome(err, domain.Conflict) && !(err == nil && replay.Outcome == domain.Conflict) {
		t.Fatalf("loser retry=%+v/%v, want conflict", replay, err)
	}
	_, content, err := fixture.store.ReadDocument(t.Context(), fixture.files, "project-1", "", requests[winner].DocumentID, nil)
	if err != nil || string(content) != string(requests[winner].Markdown) {
		t.Fatalf("winner changed=%q/%v", content, err)
	}
}
