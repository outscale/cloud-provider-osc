/*
Copyright 2019 The Kubernetes Authors.

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
	"os"

	"github.com/aws/aws-sdk-go/aws/endpoints"
	"github.com/aws/aws-sdk-go/service/elb"
)

// ********************* CCM ServiceResolver functions *********************

// MetadataResolver resolver for osc metadata service
func MetadataResolver() endpoints.ResolverFunc {
	return func(service, region string, optFns ...func(*endpoints.Options)) (endpoints.ResolvedEndpoint, error) {
		return endpoints.ResolvedEndpoint{
			URL:           "http://169.254.169.254/latest",
			SigningRegion: "custom-signing-region",
		}, nil
	}
}

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
