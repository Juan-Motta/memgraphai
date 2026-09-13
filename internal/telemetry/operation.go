// Package telemetry records bounded operational facts without business content.
package telemetry

import (
	"context"
	"time"
)

// Operation is the required Phase 1 accounting record. ResultCount is nil when an
// operation has no truthful count to report.
type Operation struct {
	OperationID     string
	RecordedAt      time.Time
	Interface       string
	ScopeKind       string
	ProjectID       string
	WorkstreamID    string
	SessionID       string
	Outcome         string
	BackendDuration time.Duration
	ResponseBytes   int
	ResultCount     *int
}

// Recorder persists best-effort operation accounting outside business outcomes.
type Recorder interface {
	Record(context.Context, Operation) error
}
