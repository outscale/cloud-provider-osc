package oapi

import (
	"github.com/outscale/osc-sdk-go/v3/pkg/profile"
)

// LoadConfig loads a config (either from env or from .osc/config.json).
// TODO: switch to SDKv3 profiles once it is released
// TODO: custom LBU endpoints are not handled as ConfigEnv does not handle non OAPI custom endpoints.
func LoadConfig() (*profile.Profile, error) {
	return profile.NewProfileFromStrandardConfiguration("", "")
}
