// Package buildinfo holds values stamped in at build time.
package buildinfo

// Version is set by the release build: -ldflags "-X github.com/heylatch/latchadmin/internal/buildinfo.Version=1.2.3".
// Development builds report 0.0.0-dev and never offer to update themselves.
var Version = "0.0.0-dev"

// Repo is the GitHub repository releases are downloaded from.
const Repo = "heylatch/latchadmin"

// IsDev reports whether this is a local build without a release version.
func IsDev() bool { return Version == "0.0.0-dev" }
