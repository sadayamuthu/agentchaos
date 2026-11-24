package main

import "runtime/debug"

// version can be set at build time with -ldflags "-X main.version=...".
// When it is empty, the module version stamped by `go install module@vX.Y.Z`
// is used, and a local build reports "dev".
var version = ""

// currentVersion reports the version of this binary.
func currentVersion() string {
	info, ok := debug.ReadBuildInfo()
	return resolveVersion(version, info, ok)
}

func resolveVersion(ldflags string, info *debug.BuildInfo, ok bool) string {
	if ldflags != "" {
		return ldflags
	}
	if ok && info != nil {
		if v := info.Main.Version; v != "" && v != "(devel)" {
			return v
		}
	}
	return "dev"
}
