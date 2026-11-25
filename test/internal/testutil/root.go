// Package testutil holds helpers shared by the test suites under test/.
package testutil

import (
	"os"
	"path/filepath"
	"runtime"
)

// RepoRoot returns the repository root, found from this file's location so
// tests work whatever directory `go test` runs them from.
func RepoRoot() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		panic("testutil: cannot locate caller")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
}

// Path joins elements onto the repository root.
func Path(elem ...string) string {
	return filepath.Join(append([]string{RepoRoot()}, elem...)...)
}

// MustRead reads a file under the repository root or panics.
func MustRead(elem ...string) []byte {
	b, err := os.ReadFile(Path(elem...))
	if err != nil {
		panic(err)
	}
	return b
}
