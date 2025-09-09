package runit

// ConfigS6 returns the default configuration for s6
func ConfigS6() *ServiceConfig {
	config := &ServiceConfig{
		Type:         ServiceTypeS6,
		ServiceDir:   "/run/service", // Common s6 location
		ChpstPath:    "s6-setuidgid",
		LoggerPath:   "s6-log",
		RunsvdirPath: "s6-svscan",
		SupportedOps: allOperations(),
	}

	// S6 doesn't support SIGSTOP/SIGCONT
	delete(config.SupportedOps, OpPause)
	delete(config.SupportedOps, OpCont)

	return config
}

// ServiceBuilderS6 creates a service builder configured for s6
func ServiceBuilderS6(name, dir string) *ServiceBuilder {
	return NewServiceBuilderWithConfig(name, dir, ConfigS6())
}
