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
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/ec2metadata"
	"github.com/aws/aws-sdk-go/aws/session"
)

// Metadata represents OSC metadata.
type Metadata struct {
	InstanceID       string
	Region           string
	AvailabilityZone string
}

// FetchMetadata queries the metadata server.
func FetchMetadata() (Metadata, error) {
	awsConfig := &aws.Config{
		EndpointResolver: MetadataResolver(),
	}
	awsConfig.WithLogLevel(aws.LogDebugWithSigning | aws.LogDebugWithHTTPBody | aws.LogDebugWithRequestRetries | aws.LogDebugWithRequestErrors)
	sess, err := session.NewSession(awsConfig)
	if err != nil {
		return Metadata{}, fmt.Errorf("unable to fetch metadata: %w", err)
	}
	// addOscUserAgent(&sess.Handlers)
	svc := ec2metadata.New(sess)

	return fetchMetadata(svc)
}

func fetchMetadata(svc EC2Metadata) (Metadata, error) {
	if !svc.Available() {
		return Metadata{}, errors.New("EC2 instance metadata is not available")
	}

	instanceID, err := svc.GetMetadata("instance-id")
	if err != nil || instanceID == "" {
		return Metadata{}, errors.New("could not get valid VM instance ID")
	}

	availabilityZone, err := svc.GetMetadata("placement/availability-zone")
	if err != nil || len(availabilityZone) < 2 {
		return Metadata{}, errors.New("could not get valid VM availability zone")
	}
	region := availabilityZone[0 : len(availabilityZone)-1]

	return Metadata{
		InstanceID:       instanceID,
		Region:           region,
		AvailabilityZone: availabilityZone,
	}, nil
}
