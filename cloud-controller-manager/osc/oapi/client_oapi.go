/*
Copyright 2014 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
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
