// Package version holds build metadata stamped in at link time.
package version

import "fmt"

// Version and Commit are set with -ldflags "-X" by the Makefile and the
// release pipeline. They default to "dev" and "unknown" for plain go builds.
var (
	Version = "dev"
	Commit  = "unknown"
)

// String returns the one-line version banner a component prints.
func String(component string) string {
	return fmt.Sprintf("%s %s (commit %s)", component, Version, Commit)
}
