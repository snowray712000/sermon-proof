// Package version holds the release version; tools/release and tools/npmpack read it.
package version

// Version is three-part semver (npm only accepts formal releases). Bump it, commit,
// then run tools/release and tools/npmpack.
const Version = "0.1.0"
