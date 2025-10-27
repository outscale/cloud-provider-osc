package utils

import "strings"

var (
	version = "dev"
)

// GetVersion retrieves the version of the plugins
func GetVersion() string {
	if !strings.HasPrefix(version, "v") {
		return "dev"
	}
	return version
}
