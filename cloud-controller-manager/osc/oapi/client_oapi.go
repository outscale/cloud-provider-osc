/*
SPDX-FileCopyrightText: 2025 Outscale SAS <opensource@outscale.com>

SPDX-License-Identifier: BSD-3-Clause
*/
package oapi

import (
	"context"
	"errors"
	"fmt"

	"github.com/outscale/cloud-provider-osc/cloud-controller-manager/utils"
	osc "github.com/outscale/osc-sdk-go/v2"
)

// OscClient is an OAPI client.
type OscClient struct {
	accessKey, secretKey string
	api                  *osc.APIClient
}

// NewOscClient builds an OAPI client.
func NewOscClient(region string, configEnv *osc.ConfigEnv) (*OscClient, error) {
	configEnv.Region = &region
	config, err := configEnv.Configuration()
	if err != nil {
		return nil, fmt.Errorf("load osc config: %w", err)
	}
	config.UserAgent = "osc-cloud-controller-manager/" + utils.GetVersion()
	client := osc.NewAPIClient(config)

	if configEnv.AccessKey == nil {
		return nil, errors.New("load osc config: missing access key")
	}
	if configEnv.SecretKey == nil {
		return nil, errors.New("load osc config: missing secret key")
	}
	return &OscClient{
		accessKey: *configEnv.AccessKey,
		secretKey: *configEnv.SecretKey,
		api:       client,
	}, nil
}

func (c *OscClient) WithAuth(ctx context.Context) context.Context {
	return context.WithValue(ctx, osc.ContextAWSv4, osc.AWSv4{
		AccessKey: c.accessKey,
		SecretKey: c.secretKey,
	})
}
