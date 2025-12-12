/*
SPDX-FileCopyrightText: 2025 Outscale SAS <opensource@outscale.com>

SPDX-License-Identifier: BSD-3-Clause
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
	// awsConfig.WithLogLevel(aws.LogDebugWithSigning | aws.LogDebugWithHTTPBody | aws.LogDebugWithRequestRetries | aws.LogDebugWithRequestErrors)
	sess, err := session.NewSession(awsConfig)
	if err != nil {
		return Metadata{}, fmt.Errorf("unable to fetch metadata: %w", err)
	}
	addHandlers(&sess.Handlers)
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
