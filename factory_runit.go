package runit

// ConfigRunit returns the default configuration for runit
//
//nolint:revive // Clear naming for multiple config types
func ConfigRunit() *ServiceConfig {
	return &ServiceConfig{
		Type:         ServiceTypeRunit,
		ServiceDir:   "/etc/service",
		ChpstPath:    "chpst",
		LoggerPath:   "svlogd",
		RunsvdirPath: "runsvdir",
		SupportedOps: allOperations(),
	}
}

// ServiceBuilderRunit creates a service builder configured for runit
func ServiceBuilderRunit(name, dir string) *ServiceBuilder {
	return NewServiceBuilderWithConfig(name, dir, ConfigRunit())
}
