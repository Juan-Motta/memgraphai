package app

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"

	"memgraphai/internal/revisionfs"
)

type checkpointStoreStub struct{}

func (checkpointStoreStub) SaveCheckpoint(context.Context, *revisionfs.Filesystem, string, string, string, string, string, string, string, []byte, []byte, string, string, string, time.Time) error {
	return nil
}
func (checkpointStoreStub) ReadCheckpoint(context.Context, *revisionfs.Filesystem, string, string, string, string) (json.RawMessage, error) {
	return json.RawMessage(`{"checkpoint_id":"checkpoint-a","checkpoint_document_id":"checkpoint-document-a","checkpoint_revision_id":"checkpoint-revision-a","project_id":"project-a","workstream_id":"workstream-a","session_id":"session-a","prose_base64":"IyBoYW5kb2ZmCg","references":[{"document_id":"source-document-a","revision_id":"source-revision-a","fresh":true}],"provenance":{"origin":"cli"},"created_at":"2026-03-05T00:00:00Z"}`), nil
}

func TestCheckpointProseBoundRejectsOversizeBeforeDecodeAllocation(t *testing.T) {
	maximum := make([]byte, checkpointProseLimit)
	encoded := base64.RawStdEncoding.EncodeToString(maximum)
	if prose, err := decodeCheckpointProse(encoded); err != nil || len(prose) != checkpointProseLimit {
		t.Fatalf("decode exact checkpoint maximum = %d, %v; want %d bytes", len(prose), err, checkpointProseLimit)
	}
	oversize := base64.RawStdEncoding.EncodeToString(append(maximum, 'x'))
	if allocations := testing.AllocsPerRun(5, func() {
		if _, err := decodeCheckpointProse(oversize); err == nil {
			t.Error("oversized checkpoint prose error = nil, want invalid")
		}
	}); allocations != 0 {
		t.Fatalf("oversized checkpoint decode allocations = %v, want zero", allocations)
	}
}

func TestCheckpointSaveRejectsDuplicateExactReferences(t *testing.T) {
	request := Request{Operation: "checkpoint.save", Scope: Scope{Kind: "session", ProjectID: "project-a", WorkstreamID: "workstream-a", SessionID: "session-a"}, Input: json.RawMessage(`{"checkpoint_id":"checkpoint-a","checkpoint_document_id":"checkpoint-document-a","checkpoint_revision_id":"checkpoint-revision-a","prose_base64":"IyBoYW5kb2ZmCg","references":[{"document_id":"source-document-a","revision_id":"source-revision-a"},{"document_id":"source-document-a","revision_id":"source-revision-a"}],"provenance":{"origin":"cli"}}`)}
	result := (CheckpointService{Store: checkpointStoreStub{}, Files: revisionfs.New(t.TempDir(), nil)}).Handle(t.Context(), request)
	if result.Outcome != Invalid {
		t.Fatalf("Handle(duplicate references) outcome = %q, want %q", result.Outcome, Invalid)
	}
}

func TestCheckpointSaveRequiresAnActiveExplicitSessionBinding(t *testing.T) {
	request := Request{
		Operation:   "checkpoint.save",
		OperationID: "checkpoint-save",
		Scope:       Scope{Kind: "session", ProjectID: "project-a", WorkstreamID: "workstream-a", SessionID: "session-a"},
		Input:       json.RawMessage(`{"checkpoint_id":"checkpoint-a","checkpoint_document_id":"checkpoint-document-a","checkpoint_revision_id":"checkpoint-revision-a","prose_base64":"IyBoYW5kb2ZmCg","references":[{"document_id":"source-document-a","revision_id":"source-revision-a"}],"provenance":{"origin":"cli"}}`),
	}
	result := (CheckpointService{Store: checkpointStoreStub{}, Files: revisionfs.New(t.TempDir(), nil), Now: func() time.Time { return time.Date(2026, 3, 5, 0, 0, 0, 0, time.UTC) }}).Handle(t.Context(), request)
	if result.Outcome != OK {
		t.Fatalf("Handle(checkpoint.save) outcome = %q, want %q for an active explicit binding", result.Outcome, OK)
	}
}
