// Package buildinfo holds build-time metadata injected via -ldflags -X at
// compile time (see the Makefile and .goreleaser.yaml). Defaults make a plain
// `go build`/`go run` identify itself as a dev build.
package buildinfo

var (
	// Version is the release version or snapshot identifier.
	Version = "dev"
	// Commit is the short git commit the binary was built from.
	Commit = "none"
	// Date is the build timestamp (RFC3339, UTC).
	Date = "unknown"
)

// String renders the full build identity for `version` output.
func String() string {
	return Version + " (commit " + Commit + ", built " + Date + ")"
}
