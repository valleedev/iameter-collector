package daemon

// ServiceStatus describes whether IA METER's background daemon is
// registered with the OS's service manager and currently running.
type ServiceStatus struct {
	Installed bool
	Running   bool
	Detail    string
}

// ServiceManager registers/unregisters `iameter daemon` as a per-user
// background service (section 20): systemd --user on Linux, a
// LaunchAgent on macOS, a logon Scheduled Task on Windows. None of these
// require administrator/root privileges.
type ServiceManager interface {
	// Install registers binaryPath (the absolute path to the iameter
	// executable) to run `iameter daemon` automatically. Idempotent.
	Install(binaryPath string) error

	// Uninstall removes the registration. Idempotent — safe to call when
	// nothing is installed.
	Uninstall() error

	// Status reports whether the service is registered and running.
	Status() (ServiceStatus, error)
}

// NewServiceManager returns the platform-appropriate manager.
func NewServiceManager() ServiceManager {
	return newPlatformServiceManager()
}
