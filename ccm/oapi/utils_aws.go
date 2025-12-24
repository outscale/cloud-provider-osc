/*
SPDX-FileCopyrightText: 2025 Outscale SAS <opensource@outscale.com>

SPDX-License-Identifier: BSD-3-Clause
*/
package oapi

import (
	"os"

	"github.com/aws/aws-sdk-go/aws/endpoints" //nolint:staticcheck
	"github.com/aws/aws-sdk-go/service/elb"   //nolint:staticcheck
)

// ********************* CCM ServiceResolver functions *********************

func endpoint(region string, service string) string {
	return "https://" + service + "." + region + ".outscale.com"
}

// ServiceResolver resolver for osc service
func ServiceResolver(region string) endpoints.ResolverFunc {
	return func(service, region string, optFns ...func(*endpoints.Options)) (endpoints.ResolvedEndpoint, error) {
		if service == elb.EndpointsID {
			url := os.Getenv("OSC_ENDPOINT_LBU")
			if url == "" {
				url = endpoint(region, "lbu")
			}
			return endpoints.ResolvedEndpoint{
				URL:           url,
				SigningRegion: region,
				SigningName:   service,
			}, nil
		}

		return endpoints.DefaultResolver().EndpointFor(
			service, region, optFns...)
	}
}
