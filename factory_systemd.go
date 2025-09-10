//go:build linux

package svcmgr

// ConfigSystemd returns the default configuration for systemd
//
//nolint:revive // Clear naming for multiple config types
func ConfigSystemd() *ServiceConfig {
	return &ServiceConfig{
		Type:         ServiceTypeSystemd,
		ServiceDir:   "/etc/systemd/system", // Standard systemd user unit location
		ChpstPath:    "",                    // Not applicable for systemd
		LoggerPath:   "journalctl",          // systemd uses journald
		RunsvdirPath: "systemctl",           // systemctl manages services
		SupportedOps: allOperations(),       // Systemd can support all ops with workarounds
	}
}

// NewClientSystemdWithConfig creates a new systemd client with the specified configuration
func NewClientSystemdWithConfig(serviceName string, config *ServiceConfig) *ClientSystemd {
	client := NewClientSystemd(serviceName)

	// Apply config settings if provided
	if config != nil && config.Type == ServiceTypeSystemd {
		if config.RunsvdirPath != "" {
			client.SystemctlPath = config.RunsvdirPath
		}
	}

	return client
}

// ServiceBuilderSystemd creates a service builder configured for systemd
func ServiceBuilderSystemd(name, dir string) *BuilderSystemd {
	sb := NewServiceBuilder(name, dir)
	return NewBuilderSystemd(sb)
}
