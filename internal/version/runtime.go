package version

import "runtime"

// RuntimeInfo returns "os/arch", e.g. "linux/amd64".
func RuntimeInfo() string {
	return runtime.GOOS + "/" + runtime.GOARCH
}
