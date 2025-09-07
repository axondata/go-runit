package runit

// Version is the current version of the go-runit library
const Version = "1.0.0"

// VersionInfo contains detailed version information
type VersionInfo struct {
	// Version is the semantic version
	Version string
	// Protocol is the runit protocol version supported
	Protocol string
	// Compatible indicates compatibility with daemontools
	Compatible bool
}

// GetVersion returns the current version information
func GetVersion() VersionInfo {
	return VersionInfo{
		Version:    Version,
		Protocol:   "runit/daemontools",
		Compatible: true,
	}
}
