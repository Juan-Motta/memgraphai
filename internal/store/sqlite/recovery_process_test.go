package sqlite

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"memgraphai/internal/domain"
	"memgraphai/internal/revisionfs"
	"memgraphai/internal/testkit"
)

func TestRecoveryProcessHelper(t *testing.T) {
	mode := os.Getenv("MEMGRAPH_RECOVERY_HELPER")
	if mode == "" {
		return
	}
	store, err := Open(context.Background(), os.Getenv("MEMGRAPH_RECOVERY_DB"))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	defer store.Close()
	ready := func() {
		if err := os.WriteFile(os.Getenv("MEMGRAPH_RECOVERY_READY"), []byte(mode), 0o600); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(4)
		}
	}
	switch mode {
	case "claim":
		_, err = store.ClaimOperation(context.Background(), operationRequest("operation-1", "fingerprint-1", "revision-1", nil, "# first\n"))
	case "prepared":
		request := operationRequest("operation-2", "fingerprint-2", "revision-2", stringPointer("revision-1"), "# prepared\n")
		claim, claimErr := store.ClaimOperation(context.Background(), request)
		if claimErr == nil {
			_, err = newProcessFilesystem(os.Getenv("MEMGRAPH_RECOVERY_LIBRARY")).PrepareAttempt(request.ID, claim.Generation, request.ProjectID, request.DocumentID, request.RevisionID, request.Markdown)
		} else {
			err = claimErr
		}
	case "committed":
		request := operationRequest("operation-2", "fingerprint-2", "revision-2", stringPointer("revision-1"), "# committed\n")
		store.afterOperationCommit = func() error {
			ready()
			select {}
		}
		_, err = store.ExecuteOperation(context.Background(), newProcessFilesystem(os.Getenv("MEMGRAPH_RECOVERY_LIBRARY")), request, allow)
	default:
		err = fmt.Errorf("unknown recovery helper mode %q", mode)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(3)
	}
	ready()
	select {}
}

func TestRecoverySurvivesTerminatedOwnerAndReopen(t *testing.T) {
	database, library := processFixture(t)
	ready := filepath.Join(t.TempDir(), "claim-ready")
	child := startRecoveryHelper(t, "claim", database, library, ready)
	waitForFile(t, ready)
	if err := child.Process.Kill(); err != nil {
		t.Fatalf("Kill(claim owner) error = %v", err)
	}
	if err := child.Wait(); err == nil {
		t.Fatal("Wait(terminated claim owner) error = nil, want process termination")
	}
	store, err := Open(t.Context(), database)
	if err != nil {
		t.Fatalf("Open(restart) error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	files := newProcessFilesystem(library)
	request := operationRequest("operation-1", "fingerprint-1", "revision-1", nil, "# first\n")
	result, err := store.ExecuteOperation(t.Context(), files, request, allow)
	if err != nil || result.Outcome != domain.Outcome(operationCommitted) || result.Generation != 2 {
		t.Fatalf("ExecuteOperation(after terminated owner) = %#v, %v; want fenced generation-2 commit", result, err)
	}
}

func TestRecoveryProcessPreparedFinalFilePreservesPriorCurrentUntilReopen(t *testing.T) {
	database, library := processFixture(t)
	commitProcessRevision(t, database, library, operationRequest("operation-1", "fingerprint-1", "revision-1", nil, "# first\n"))
	ready := filepath.Join(t.TempDir(), "prepared-ready")
	child := startRecoveryHelper(t, "prepared", database, library, ready)
	waitForFile(t, ready)
	if err := child.Process.Kill(); err != nil {
		t.Fatalf("Kill(prepared helper) error = %v", err)
	}
	if err := child.Wait(); err == nil {
		t.Fatal("Wait(prepared helper) error = nil, want process termination")
	}
	store, err := Open(t.Context(), database)
	if err != nil {
		t.Fatalf("Open(reopen) error = %v", err)
	}
	defer store.Close()
	prepared := operationRequest("operation-2", "fingerprint-2", "revision-2", stringPointer("revision-1"), "# prepared\n")
	if result, err := store.RecoverOperation(t.Context(), prepared.ID, prepared.Fingerprint, allow); err != nil || result.Outcome != domain.Unknown {
		t.Fatalf("RecoverOperation(prepared) = %#v, %v; want conservative unknown", result, err)
	}
	if current, _, err := store.CurrentRevision(t.Context(), "document-1"); err != nil || current != "revision-1" {
		t.Fatalf("CurrentRevision(prepared reopen) = %q, %v; want prior revision-1", current, err)
	}
	if result, err := store.ExecuteOperation(t.Context(), newProcessFilesystem(library), prepared, allow); err != nil || result.Outcome != domain.Outcome(operationCommitted) || result.Generation != 2 {
		t.Fatalf("ExecuteOperation(prepared reopen) = %#v, %v; want generation-2 commit", result, err)
	}
}

func TestRecoveryProcessCommittedBeforeResponseReplaysAfterLaterAdvance(t *testing.T) {
	database, library := processFixture(t)
	commitProcessRevision(t, database, library, operationRequest("operation-1", "fingerprint-1", "revision-1", nil, "# first\n"))
	ready := filepath.Join(t.TempDir(), "committed-ready")
	child := startRecoveryHelper(t, "committed", database, library, ready)
	waitForFile(t, ready)
	if err := child.Process.Kill(); err != nil {
		t.Fatalf("Kill(committed helper) error = %v", err)
	}
	if err := child.Wait(); err == nil {
		t.Fatal("Wait(committed helper) error = nil, want process termination")
	}
	store, err := Open(t.Context(), database)
	if err != nil {
		t.Fatalf("Open(reopen) error = %v", err)
	}
	defer store.Close()
	committed := operationRequest("operation-2", "fingerprint-2", "revision-2", stringPointer("revision-1"), "# committed\n")
	if result, err := store.RecoverOperation(t.Context(), committed.ID, committed.Fingerprint, allow); err != nil || result.Outcome != domain.Outcome(operationCommitted) || result.RevisionID != committed.RevisionID {
		t.Fatalf("RecoverOperation(committed) = %#v, %v; want persisted committed revision", result, err)
	}
	later := operationRequest("operation-3", "fingerprint-3", "revision-3", stringPointer("revision-2"), "# later\n")
	if _, err := store.ExecuteOperation(t.Context(), newProcessFilesystem(library), later, allow); err != nil {
		t.Fatalf("ExecuteOperation(later) error = %v", err)
	}
	if result, err := store.ExecuteOperation(t.Context(), newProcessFilesystem(library), committed, allow); err != nil || result.Outcome != domain.Outcome(operationCommitted) || result.RevisionID != committed.RevisionID {
		t.Fatalf("ExecuteOperation(committed replay) = %#v, %v; want persisted committed revision", result, err)
	}
	if current, _, err := store.CurrentRevision(t.Context(), "document-1"); err != nil || current != "revision-3" {
		t.Fatalf("CurrentRevision(committed replay) = %q, %v; want later revision-3", current, err)
	}
}

func TestRecoveryBoundsEachCrossProcessRetryWithoutExhaustingRecovery(t *testing.T) {
	database, library := processFixture(t)
	store, err := Open(t.Context(), database)
	if err != nil {
		t.Fatalf("Open(parent) error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ready := filepath.Join(t.TempDir(), "retry-ready")
	child := startRecoveryHelper(t, "claim", database, library, ready)
	waitForFile(t, ready)
	request := operationRequest("operation-1", "fingerprint-1", "revision-1", nil, "# first\n")
	if claimed, err := store.ClaimOperation(t.Context(), request); err != nil || claimed.Generation != 2 {
		t.Fatalf("ClaimOperation(simultaneous retry) = %#v, %v; want generation 2", claimed, err)
	}
	started := time.Now()
	exhausted, err := store.ClaimOperation(t.Context(), request)
	elapsed := time.Since(started)
	if err != nil || exhausted.Outcome != "" || exhausted.Generation != 3 {
		t.Fatalf("ClaimOperation(exhausted) = %#v, %v; want one bounded new claim at generation 3", exhausted, err)
	}
	if elapsed > time.Second {
		t.Fatalf("ClaimOperation() waited %s, want bounded under 1s", elapsed)
	}
	if err := child.Process.Kill(); err != nil {
		t.Fatalf("Kill(simultaneous retry owner) error = %v", err)
	}
	if err := child.Wait(); err == nil {
		t.Fatal("Wait(terminated retry owner) error = nil, want process termination")
	}
}

func processFixture(t *testing.T) (string, string) {
	t.Helper()
	database := testkit.TempSQLitePath(t, "recovery-process")
	store, err := Open(t.Context(), database)
	if err != nil {
		t.Fatalf("Open(fixture) error = %v", err)
	}
	if err := store.CreateProject(t.Context(), "project-1", "Project"); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	if err := store.CreateDocument(t.Context(), "document-1", "project-1"); err != nil {
		t.Fatalf("CreateDocument() error = %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close(fixture) error = %v", err)
	}
	return database, filepath.Join(t.TempDir(), "library")
}

func newProcessFilesystem(library string) *revisionfs.Filesystem { return revisionfs.New(library, nil) }

func commitProcessRevision(t *testing.T, database, library string, request OperationRequest) {
	t.Helper()
	store, err := Open(t.Context(), database)
	if err != nil {
		t.Fatalf("Open(commit fixture) error = %v", err)
	}
	defer store.Close()
	if result, err := store.ExecuteOperation(t.Context(), newProcessFilesystem(library), request, allow); err != nil || result.Outcome != domain.Outcome(operationCommitted) {
		t.Fatalf("ExecuteOperation(commit fixture) = %#v, %v; want committed", result, err)
	}
}

func startRecoveryHelper(t *testing.T, mode, database, library, ready string) *exec.Cmd {
	t.Helper()
	command := exec.Command(os.Args[0], "-test.run=^TestRecoveryProcessHelper$")
	command.Env = append(os.Environ(), "MEMGRAPH_RECOVERY_HELPER="+mode, "MEMGRAPH_RECOVERY_DB="+database, "MEMGRAPH_RECOVERY_LIBRARY="+library, "MEMGRAPH_RECOVERY_READY="+ready)
	if err := command.Start(); err != nil {
		t.Fatalf("Start(%s helper) error = %v", mode, err)
	}
	return command
}

func waitForFile(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("helper did not create readiness file %q", path)
}
