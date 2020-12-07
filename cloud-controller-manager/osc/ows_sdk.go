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

package osc

import (
	"fmt"
	"time"

	"github.com/outscale/osc-sdk-go"
)

// ********************* CCM oscSdkOSC Def & functions *********************

// oscSdkOSC is an implementation of the OSC interface, backed by osc-sdk-go
type oscSdkOSC struct {
	config *osc.Configuration
	auth   context.Context
	api    *osc.APIClient
}

// Implementation of OSC.Instances
func (s *oscSdkOSC) ReadVms(ctx context.Context, request *osc.DescribeInstancesInput) ([]*osc.Instance, error) {
	// Instances are paged
	results := []*osc.Instance{}
	var nextToken *string
	requestTime := time.Now()
	for {
		response, err := s.api.VmApi.ReadVms(s.auth.request)
		if err != nil {
			recordAWSMetric("describe_instance", 0, err)
			return nil, fmt.Errorf("error listing OSC instances: %q", err)
		}

		for _, reservation := range response.Reservations {
			results = append(results, reservation.Instances...)
		}

		nextToken = response.NextToken
		if aws.StringValue(nextToken) == "" {
			break
		}
		request.NextToken = nextToken
	}
	timeTaken := time.Since(requestTime).Seconds()
	recordAWSMetric("describe_instance", timeTaken, nil)
	return results, nil
}

// Implements OSC.DescribeSecurityGroups
func (s *oscSdkOSC) DescribeSecurityGroups(ctx context.Context, request *osc.DescribeSecurityGroupsInput) ([]*osc.SecurityGroup, error) {
	// Security groups are paged
	results := []*osc.SecurityGroup{}
	var nextToken *string
	requestTime := time.Now()
	for {
		response, err := s.oscIf.DescribeSecurityGroups(request)
		if err != nil {
			recordAWSMetric("describe_security_groups", 0, err)
			return nil, fmt.Errorf("error listing AWS security groups: %q", err)
		}

		results = append(results, response.SecurityGroups...)

		nextToken = response.NextToken
		if aws.StringValue(nextToken) == "" {
			break
		}
		request.NextToken = nextToken
	}
	timeTaken := time.Since(requestTime).Seconds()
	recordAWSMetric("describe_security_groups", timeTaken, nil)
	return results, nil
}

func (s *oscSdkOSC) DescribeSubnets(ctx context.Context, request *osc.DescribeSubnetsInput) ([]*osc.Subnet, error) {
	// Subnets are not paged
	response, err := s.oscIf.DescribeSubnets(request)
	if err != nil {
		return nil, fmt.Errorf("error listing AWS subnets: %q", err)
	}
	return response.Subnets, nil
}

func (s *oscSdkOSC) CreateSecurityGroup(ctx context.Context, request *osc.CreateSecurityGroupInput) (*osc.CreateSecurityGroupOutput, error) {
	return s.oscIf.CreateSecurityGroup(request)
}

func (s *oscSdkOSC) DeleteSecurityGroup(ctx context.Context, request *osc.DeleteSecurityGroupInput) (*osc.DeleteSecurityGroupOutput, error) {
	return s.oscIf.DeleteSecurityGroup(request)
}

func (s *oscSdkOSC) AuthorizeSecurityGroupIngress(ctx context.Context, request *osc.AuthorizeSecurityGroupIngressInput) (*osc.AuthorizeSecurityGroupIngressOutput, error) {
	return s.oscIf.AuthorizeSecurityGroupIngress(request)
}

func (s *oscSdkOSC) RevokeSecurityGroupIngress(ctx context.Context, request *osc.RevokeSecurityGroupIngressInput) (*osc.RevokeSecurityGroupIngressOutput, error) {
	return s.oscIf.RevokeSecurityGroupIngress(request)
}

func (s *oscSdkOSC) CreateTags(ctx context.Context, request *osc.CreateTagsInput) (*osc.CreateTagsOutput, error) {
	debugPrintCallerFunctionName()
	requestTime := time.Now()
	resp, err := s.osc.CreateTags(request)
	timeTaken := time.Since(requestTime).Seconds()
	recordAWSMetric("create_tags", timeTaken, err)
	return resp, err
}

func (s *oscSdkOSC) DescribeRouteTables(ctx context.Context, request *osc.DescribeRouteTablesInput) ([]*osc.RouteTable, error) {
	results := []*osc.RouteTable{}
	var nextToken *string
	requestTime := time.Now()
	for {
		response, err := s.oscIf.DescribeRouteTables(request)
		if err != nil {
			recordAWSMetric("describe_route_tables", 0, err)
			return nil, fmt.Errorf("error listing AWS route tables: %q", err)
		}

		results = append(results, response.RouteTables...)

		nextToken = response.NextToken
		if aws.StringValue(nextToken) == "" {
			break
		}
		request.NextToken = nextToken
	}
	timeTaken := time.Since(requestTime).Seconds()
	recordAWSMetric("describe_route_tables", timeTaken, nil)
	return results, nil
}

func (s *oscSdkOSC) CreateRoute(ctx context.Context, request *osc.CreateRouteInput) (*osc.CreateRouteOutput, error) {
	return s.oscIf.CreateRoute(request)
}

func (s *oscSdkOSC) DeleteRoute(ctx context.Context, request *osc.DeleteRouteInput) (*osc.DeleteRouteOutput, error) {
	return s.oscIf.DeleteRoute(request)
}

func (s *oscSdkOSC) ModifyInstanceAttribute(ctx context.Context, request *osc.ModifyInstanceAttributeInput) (*osc.ModifyInstanceAttributeOutput, error) {
	return s.oscIf.ModifyInstanceAttribute(request)
}

func (s *oscSdkOSC) DescribeVpcs(ctx context.Context, request *osc.DescribeVpcsInput) (*osc.DescribeVpcsOutput, error) {
	return s.oscIf.DescribeVpcs(request)
}
