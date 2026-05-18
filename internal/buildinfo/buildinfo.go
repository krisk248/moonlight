// Package buildinfo holds constants stamped at build time via -ldflags.
// Source builds leave the defaults — lifecycle enforcement treats those as
// "unrestricted dev mode."
package buildinfo

// These are overridable via:
//   go build -ldflags="-X github.com/krisk248/moonlight/internal/buildinfo.BuildDate=2026-05-18 \
//                       -X github.com/krisk248/moonlight/internal/buildinfo.BuildID=ml-2026-05-18-abc \
//                       -X github.com/krisk248/moonlight/internal/buildinfo.RevocationURL=https://..."
var (
	BuildDate     = "SOURCE_BUILD"
	BuildID       = "source-build"
	Version       = "0.1.0"
	RevocationURL = ""
)

// IsSourceBuild reports whether this binary was compiled without the build
// pipeline (i.e. a `go run` or `go build` from a developer's checkout).
func IsSourceBuild() bool { return BuildDate == "SOURCE_BUILD" }
