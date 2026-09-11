package testkit

// Fault returns its configured error only at the named test boundary.
type Fault struct {
	Point string
	Err   error
}

// FailAt creates a deterministic one-boundary fault for filesystem tests.
func FailAt(point string, err error) Fault {
	return Fault{Point: point, Err: err}
}

// At returns the configured interruption for point, if any.
func (f Fault) At(point string) error {
	if f.Point == point {
		return f.Err
	}
	return nil
}
