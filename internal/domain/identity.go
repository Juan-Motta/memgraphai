package domain

// ProjectID is immutable once persisted by the SQLite store.
type ProjectID string

// Outcome is a stable identity or scope result.
type Outcome string

const (
	None                 Outcome = "none"
	One                  Outcome = "one"
	Many                 Outcome = "many"
	Unavailable          Outcome = "unavailable"
	Immutable            Outcome = "immutable"
	BindingMismatch      Outcome = "binding_mismatch"
	ScopeDenied          Outcome = "scope_denied"
	Conflict             Outcome = "conflict"
	Unknown              Outcome = "unknown"
	IdempotencyMismatch  Outcome = "idempotency_mismatch"
	IntegrityDiscrepancy Outcome = "integrity_discrepancy"
	Busy                 Outcome = "busy"
	Retryable            Outcome = "retryable"
)

// Error carries a stable outcome without exposing store details.
type Error struct{ Outcome Outcome }

func (e Error) Error() string { return string(e.Outcome) }

// IsOutcome reports whether err carries an expected stable outcome.
func IsOutcome(err error, outcome Outcome) bool {
	e, ok := err.(Error)
	return ok && e.Outcome == outcome
}

// Resolution reports an explicit path-association result.
type Resolution struct {
	Outcome  Outcome
	Projects []ProjectID
}

// AuthorizeProject requires an explicit grant; association is not authorization.
func AuthorizeProject(project ProjectID, grants map[ProjectID]bool) error {
	if !grants[project] {
		return Error{Outcome: ScopeDenied}
	}
	return nil
}
