/*
SPDX-FileCopyrightText: 2025 Outscale SAS <opensource@outscale.com>

SPDX-License-Identifier: BSD-3-Clause
*/
package oapi

import (
	"fmt"
	"reflect"
	"unsafe"

	osc "github.com/outscale/osc-sdk-go/v2"
)

// LoadConfig loads a config (either from env or from .osc/config.json).
// TODO: switch to SDKv3 profiles once it is released
// TODO: custom LBU endpoints are not handled as ConfigEnv does not handle non OAPI custom endpoints.
func LoadConfig() (*osc.ConfigEnv, error) {
	configEnv := osc.NewConfigEnv()
	if configEnv.AccessKey != nil || configEnv.SecretKey != nil {
		return configEnv, nil
	}
	fcfg, err := osc.LoadDefaultConfigFile()
	if err != nil {
		return nil, err
	}
	profile := "default"
	if configEnv.ProfileName != nil && *configEnv.ProfileName != "" {
		profile = *configEnv.ProfileName
	}
	return configEnvFromConfigFile(fcfg, profile)
}

func configEnvFromConfigFile(cfg *osc.ConfigFile, profile string) (*osc.ConfigEnv, error) {
	v := reflect.Indirect(reflect.ValueOf(cfg)).Field(0)
	// this is the only way to access a non exported field. SDKv3 will allow us to access all profiles.
	fcfg := reflect.NewAt(v.Type(), unsafe.Pointer(v.UnsafeAddr())).Elem().Interface().(map[string]osc.Profile) //nolint:gosec
	pcfg, found := fcfg[profile]
	if !found {
		return nil, fmt.Errorf("profile %q not found", profile)
	}
	ecfg := &osc.ConfigEnv{}
	if pcfg.AccessKey != "" {
		ecfg.AccessKey = &pcfg.AccessKey
	}
	if pcfg.SecretKey != "" {
		ecfg.SecretKey = &pcfg.SecretKey
	}
	if pcfg.Region != "" {
		ecfg.Region = &pcfg.Region
	}
	if pcfg.X509ClientCert != "" {
		ecfg.X509ClientCert = &pcfg.X509ClientCert
	}
	if pcfg.X509ClientCertB64 != "" {
		ecfg.X509ClientCertB64 = &pcfg.X509ClientCertB64
	}
	if pcfg.X509ClientKey != "" {
		ecfg.X509ClientKey = &pcfg.X509ClientKey
	}
	if pcfg.X509ClientKeyB64 != "" {
		ecfg.X509ClientKeyB64 = &pcfg.X509ClientKeyB64
	}
	if pcfg.Endpoints.API != "" {
		ecfg.OutscaleApiEndpoint = &pcfg.Endpoints.API
	}
	return ecfg, nil
}
