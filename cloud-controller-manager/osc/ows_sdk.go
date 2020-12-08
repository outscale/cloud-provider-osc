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

// ********************* CCM oscSdkFCU Def & functions *********************

// oscSdkFCU is an implementation of the OSC interface, backed by osc-sdk-go
type oscSdkFCU struct {
	config *osc.Configuration
	auth   context.Context
	api    *osc.APIClient
}

// Implementation of OSC.Instances
func (s *oscSdkFCU) ReadVms(ctx context.Context, request *osc.ReadVmsOpts) ([]osc.Vm, error) {
	// Instances are not paged
	results := []osc.Vm{}
	requestTime := time.Now()
	for {
		response, httpRes, err := s.api.VmApi.ReadVms(s.auth, request)
		if err != nil {
			recordAWSMetric("describe_instance", 0, err)
			return nil, fmt.Errorf("error listing OSC instances: %q", err)
		}
	}
	timeTaken := time.Since(requestTime).Seconds()
	recordAWSMetric("describe_instance", timeTaken, nil)
	return results.Vms, nil
}

// Implements OSC.DescribeSecurityGroups
func (s *oscSdkFCU) ReadSecurityGroups(ctx context.Context, request *osc.ReadSecurityGroups) ([]osc.SecurityGroup, error) {
	// Security groups are not paged
	results := []osc.SecurityGroup{}
	requestTime := time.Now()
	for {
		response, httpRes, err := s.api.SecurityGroupApi.ReadSecurityGroups(s.auth, request)
		if err != nil {
			recordAWSMetric("describe_security_groups", 0, err)
			return nil, fmt.Errorf("error listing OSC security groups: %q", err)
		}
	}
	timeTaken := time.Since(requestTime).Seconds()
	recordAWSMetric("describe_security_groups", timeTaken, nil)
	return results.SecurityGroups, nil
}

func (s *oscSdkFCU) DescribeSubnets(ctx context.Context, request *osc.ReadSubnetsOpts) ([]osc.Subnet, error) {
	// Subnets are not paged
	response, httpRes, err := s.api.SubnetApi.DescribeSubnets(s.auth, request)
	if err != nil {
		return nil, fmt.Errorf("error listing OSC subnets: %q", err)
	}
	return response.Subnets, nil
}

func (s *oscSdkFCU) CreateSecurityGroup(ctx context.Context, request *osc.CreateSecurityGroupOpts) (osc.CreateSecurityGroupResponse, error) {
	return s.api.SecurityGroupApi.CreateSecurityGroup(s.auth, request)
}

func (s *oscSdkFCU) DeleteSecurityGroup(ctx context.Context, request *osc.DeleteSecurityGroupOpts) (osc.DeleteSecurityGroupResponse, error) {
	return s.api.SecurityGroupApi.DeleteSecurityGroup(s.auth, request)
}

func (s *oscSdkFCU) CreateSecurityGroupRule(ctx context.Context, request *osc.CreateSecurityGroupRuleOpts) (osc.CreateSecurityGroupRuleResponse, error) {
	return s.api.SecurityGroupApi.CreateSecurityGroupRule(s.auth, request)
}

func (s *oscSdkFCU) DeleteSecurityGroupRuleRequest(ctx context.Context, request *osc.DeleteSecurityGroupRuleOpts) (osc.DeleteSecurityGroupRuleResponse, error) {
	return s.api.SecurityGroupApi.DeleteSecurityGroupRuleRequest(s.auth, request)
}

func (s *oscSdkFCU) CreateTags(ctx context.Context, request *osc.CreateTagsInput) (osc.CreateTagsOutput, error) {
	debugPrintCallerFunctionName()
	requestTime := time.Now()
	resp, err := s.api.TagApi.CreateTags(s.auth, request)
	timeTaken := time.Since(requestTime).Seconds()
	recordAWSMetric("create_tags", timeTaken, err)
	return resp, err
}

func (s *oscSdkFCU) DescribeRouteTables(ctx context.Context, request *osc.ReadRouteTablesOpts) ([]osc.RouteTables, error) {
	results := []*osc.RouteTable{}
	requestTime := time.Now()
	for {
		response, httpRes, err := s.api.RouteTableApi.ReadRouteTables(s.auth, request)
		if err != nil {
			recordAWSMetric("describe_route_tables", 0, err)
			return nil, fmt.Errorf("error listing OSC route tables: %q", err)
		}

		results = append(results, response.RouteTables...)
	}
	timeTaken := time.Since(requestTime).Seconds()
	recordAWSMetric("describe_route_tables", timeTaken, nil)
	return results.RouteTables, nil
}

func (s *oscSdkFCU) CreateRoute(ctx context.Context, request *osc.CreateRouteOpts) (osc.CreateRouteResponse, error) {
	return s.api.RouteApi.CreateRoute(s.auth, request)
}

func (s *oscSdkFCU) DeleteRoute(ctx context.Context, request *osc.DeleteRouteOpts) (osc.DeleteRouteResponse, error) {
	return s.api.RouteApi.DeleteRoute(s.auth, request)
}

func (s *oscSdkFCU) UpdateVm(ctx context.Context, request *osc.UpdateVmOpts) (osc.UpdateVmResponse, error) {
	return s.api.VmApi.UpdateVm(s.auth, request)
}

func (s *oscSdkFCU) DescribeVpcs(ctx context.Context, request *osc.ReadNetsOpts) (osc.ReadNetsResponse, error) {
	return s.api.ReadNets(s.auth, request)
}
