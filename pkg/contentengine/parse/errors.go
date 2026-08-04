package parse

import "fmt"

// MalformedDirective is returned when a :::secret or :::card block isn't
// properly closed, or a stray closing ::: fence appears with nothing
// open. Errors are values, per docs/web-refactor-spec.md §7.1 requirement
// 4 — this package never panics or calls os.Exit/log.Fatal; presenting
// the error (CLI spec golden rule 6: fail loud, name the fix) is the
// caller's job.
type MalformedDirective struct {
	// Line is the 1-indexed source line of the opening fence (for an
	// unclosed block) or of the stray fence itself.
	Line   int
	Reason string
}

func (e MalformedDirective) Error() string {
	return fmt.Sprintf("malformed directive block at line %d: %s", e.Line, e.Reason)
}
