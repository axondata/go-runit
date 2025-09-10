package svcmgr

// ConfigDaemontools returns the default configuration for daemontools
func ConfigDaemontools() *ServiceConfig {
	config := &ServiceConfig{
		Type:         ServiceTypeDaemontools,
		ServiceDir:   "/service",
		ChpstPath:    "setuidgid", // or envuidgid
		LoggerPath:   "multilog",
		RunsvdirPath: "svscan",
		SupportedOps: allOperations(),
	}

	// Daemontools doesn't support these operations
	delete(config.SupportedOps, OpOnce) // No 'o' command
	delete(config.SupportedOps, OpQuit) // No 'q' command

	return config
}

// ServiceBuilderDaemontools creates a service builder configured for daemontools
func ServiceBuilderDaemontools(name, dir string) *ServiceBuilder {
	return NewServiceBuilderWithConfig(name, dir, ConfigDaemontools())
}
