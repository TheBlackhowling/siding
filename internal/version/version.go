package version

import "runtime"

// Set via -ldflags at release time. GoVersion falls back to the toolchain
// that built the binary; GoReleaser has no {{.GoVersion}} template key.
var (
	Version   = "dev"
	Commit    = "none"
	Date      = "unknown"
	GoVersion = ""
)

func Info() map[string]string {
	goVer := GoVersion
	if goVer == "" {
		goVer = runtime.Version()
	}
	return map[string]string{
		"version":    Version,
		"commit":     Commit,
		"date":       Date,
		"go_version": goVer,
	}
}
