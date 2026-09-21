package migrate

import "fmt"

// StepError reports an upgrade that stopped partway. The file named by Path
// is left at ReachedSchema, the last format it was successfully written at.
type StepError struct {
	Path          string
	Step          string
	ReachedSchema int
	Cause         error
}

func (e *StepError) Error() string {
	return fmt.Sprintf("upgrading %s stopped at step %q: %v (the file is at format %d)", e.Path, e.Step, e.Cause, e.ReachedSchema)
}

func (e *StepError) Unwrap() error { return e.Cause }

// unreadableError reports a settings file that could not be read or parsed
// as a YAML mapping: a broken file rather than an out-of-date one.
type unreadableError struct {
	Path string
	Err  error
}

func (e *unreadableError) Error() string {
	return fmt.Sprintf("reading %s: %v", e.Path, e.Err)
}

func (e *unreadableError) Unwrap() error { return e.Err }
