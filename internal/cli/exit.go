package cli

// ExitError lets a command request a specific process exit code on the
// way out. The cmd/fglpkg entry point unwraps this and calls
// os.Exit(Code); other commands keep returning plain errors and inherit
// the default exit code (1).
//
// Used by `fglpkg audit` to distinguish "vulnerabilities found" (1)
// from "audit itself failed" (2), matching `npm audit` semantics.
type ExitError struct {
	Code int
	Err  error
}

func (e *ExitError) Error() string {
	if e.Err == nil {
		return ""
	}
	return e.Err.Error()
}

func (e *ExitError) Unwrap() error { return e.Err }
