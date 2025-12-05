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
	"fmt"

	"github.com/outscale/cloud-provider-osc/ccm/utils"
	osc "github.com/outscale/osc-sdk-go/v3/pkg/osc"
	"github.com/outscale/osc-sdk-go/v3/pkg/profile"
	options "github.com/outscale/osc-sdk-go/v3/pkg/utils"
)

// OscClient is an OAPI client.
type OscClient struct {
	api *osc.Client
}

// NewOscClient builds an OAPI client.
func NewOscClient(region string, prof *profile.Profile) (*OscClient, error) {
	prof.Region = region
	ua := "osc-cloud-controller-manager/" + utils.GetVersion()
	lg := OAPILogger{}
	client, err := osc.NewClient(prof, options.WithUseragent(ua), options.WithLogging(lg))
	if err != nil {
		return nil, fmt.Errorf("unable to initialize OAPI client: %w", err)
	}

	return &OscClient{
		api: client,
	}, nil
}
