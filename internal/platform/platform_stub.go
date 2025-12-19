//go:build !darwin
// +build !darwin

package platform

// GetSystemSOCKSProxy is a stub for non-macOS platforms.
// On non-darwin systems system-wide SOCKS proxy detection is not implemented,
// so return empty host, port 0, enabled=false and no error.
func GetSystemSOCKSProxy() (host string, port int, enabled bool, err error) {
	return "", 0, false, nil
}

// SetupDockReopenHandler is a no-op on non-macOS platforms.
// On macOS a native handler is installed to show the window when Dock icon is clicked.
func SetupDockReopenHandler(showWindowCallback func()) {
	// no-op on non-darwin platforms
}

// CleanupDockReopenHandler is a no-op on non-macOS platforms.
func CleanupDockReopenHandler() {
	// no-op on non-darwin platforms
}
