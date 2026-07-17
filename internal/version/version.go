// Package version exposes the build version shared by all PPG binaries.
//
// Version is stamped at build time via
//
//	-ldflags "-X github.com/owulveryck/poc-agentic-platform/internal/version.Version=v1.0.0"
//
// (wired in the Makefile and the GoReleaser config). Unstamped builds fall
// back to the module version recorded by the Go toolchain, then to "devel".
package version

import "runtime/debug"

// Version is the stamped release version; empty on unstamped builds.
var Version string

// String returns the version to print for -version.
func String() string {
	if Version != "" {
		return Version
	}
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		return bi.Main.Version
	}
	return "devel"
}
