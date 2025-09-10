//go:build !linux

package svcmgr

// ConfigSystemd returns an error on non-Linux systems
//
//nolint:revive // Clear naming for multiple config types
func ConfigSystemd() *ServiceConfig {
	// Return a config that indicates systemd is not available
	return &ServiceConfig{
		Type:         ServiceTypeSystemd,
		ServiceDir:   "",
		ChpstPath:    "",
		LoggerPath:   "",
		RunsvdirPath: "",
		SupportedOps: make(map[Operation]struct{}), // No operations supported
	}
}
