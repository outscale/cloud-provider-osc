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
	"context"
	"errors"
	"fmt"
	"time"

	osc "github.com/outscale/osc-sdk-go/v2"
)

// ********************* CCM oscSdkCompute Def & functions *********************

// oscSdkCompute is an implementation of the some EC2 interface and OSC Interface, backed by aws-sdk-go and osc-sdk-go
type oscSdkCompute struct {
	client *osc.APIClient
	ctx    context.Context
}

// Implementation of ReadVms
func (s *oscSdkCompute) ReadVms(request *osc.ReadVmsRequest) ([]osc.Vm, error) {
	// Instances are paged
	var results []osc.Vm
	requestTime := time.Now()
	response, httpRes, err := s.client.VmApi.ReadVms(s.ctx).ReadVmsRequest(*request).Execute()
	if err != nil {
		recordAWSMetric("describe_instance", 0, err)
		if httpRes != nil {
			return nil, fmt.Errorf("error listing instances: %q (Status:%v)", err, httpRes.Status)
		}
		return nil, fmt.Errorf("error listing instances: %q", err)
	}

	if !response.HasVms() {
		return nil, errors.New("error listing instances: Vm has not been set")
	}

	results = *response.Vms
	timeTaken := time.Since(requestTime).Seconds()
	recordAWSMetric("describe_instance", timeTaken, nil)
	return results, nil
}

// Implements EC2.ReadSecurityGroups
func (s *oscSdkCompute) ReadSecurityGroups(request *osc.ReadSecurityGroupsRequest) ([]osc.SecurityGroup, error) {
	requestTime := time.Now()
	response, httpRes, err := s.client.SecurityGroupApi.ReadSecurityGroups(s.ctx).ReadSecurityGroupsRequest(*request).Execute()
	if err != nil {
		recordAWSMetric("describe_security_groups", 0, err)
		if httpRes != nil {
			return nil, fmt.Errorf("error listing security groups: %q (Status:%v)", err, httpRes.Status)
		}
		return nil, fmt.Errorf("error listing security groups: %q", err)
	}

	if !response.HasSecurityGroups() {
		return nil, errors.New("error listing security groups: SecurityGroups not set")
	}

	timeTaken := time.Since(requestTime).Seconds()
	recordAWSMetric("describe_security_groups", timeTaken, nil)
	return response.GetSecurityGroups(), nil
}

func (s *oscSdkCompute) DescribeSubnets(request *osc.ReadSubnetsRequest) ([]osc.Subnet, error) {
	// Subnets are not paged
	response, _, err := s.client.SubnetApi.ReadSubnets(s.ctx).ReadSubnetsRequest(*request).Execute()
	if err != nil {
		return nil, fmt.Errorf("error listing subnets: %q", err)
	}

	if !response.HasSubnets() {
		return nil, errors.New("error listing subnets: Got no subnets")
	}

	return response.GetSubnets(), nil
}

func (s *oscSdkCompute) CreateSecurityGroup(request *osc.CreateSecurityGroupRequest) (*osc.CreateSecurityGroupResponse, error) {
	response, _, err := s.client.SecurityGroupApi.CreateSecurityGroup(s.ctx).CreateSecurityGroupRequest(*request).Execute()
	return &response, err
}

func (s *oscSdkCompute) DeleteSecurityGroup(request *osc.DeleteSecurityGroupRequest) (*osc.DeleteSecurityGroupResponse, error) {
	response, _, err := s.client.SecurityGroupApi.DeleteSecurityGroup(s.ctx).DeleteSecurityGroupRequest(*request).Execute()
	return &response, err
}

func (s *oscSdkCompute) CreateSecurityGroupRule(request *osc.CreateSecurityGroupRuleRequest) (*osc.CreateSecurityGroupRuleResponse, error) {
	response, _, err := s.client.SecurityGroupRuleApi.CreateSecurityGroupRule(s.ctx).CreateSecurityGroupRuleRequest(*request).Execute()
	return &response, err
}

func (s *oscSdkCompute) DeleteSecurityGroupRule(request *osc.DeleteSecurityGroupRuleRequest) (*osc.DeleteSecurityGroupRuleResponse, error) {
	response, _, err := s.client.SecurityGroupRuleApi.DeleteSecurityGroupRule(s.ctx).DeleteSecurityGroupRuleRequest(*request).Execute()
	return &response, err
}

func (s *oscSdkCompute) CreateTags(request *osc.CreateTagsRequest) (*osc.CreateTagsResponse, error) {
	debugPrintCallerFunctionName()
	requestTime := time.Now()
	resp, _, err := s.client.TagApi.CreateTags(s.ctx).CreateTagsRequest(*request).Execute()
	timeTaken := time.Since(requestTime).Seconds()
	recordAWSMetric("create_tags", timeTaken, err)
	return &resp, err
}

func (s *oscSdkCompute) ReadRouteTables(request *osc.ReadRouteTablesRequest) ([]osc.RouteTable, error) {
	requestTime := time.Now()
	response, _, err := s.client.RouteTableApi.ReadRouteTables(s.ctx).ReadRouteTablesRequest(*request).Execute()
	if err != nil {
		recordAWSMetric("describe_route_tables", 0, err)
		return nil, fmt.Errorf("error listing route tables: %q", err)
	}

	if !response.HasRouteTables() {
		return nil, errors.New("error listing route tables: RouteTable not set")
	}
	timeTaken := time.Since(requestTime).Seconds()
	recordAWSMetric("describe_route_tables", timeTaken, nil)

	return response.GetRouteTables(), nil
}

func (s *oscSdkCompute) CreateRoute(request *osc.CreateRouteRequest) (*osc.CreateRouteResponse, error) {
	response, _, err := s.client.RouteApi.CreateRoute(s.ctx).CreateRouteRequest(*request).Execute()
	return &response, err
}

func (s *oscSdkCompute) DeleteRoute(request *osc.DeleteRouteRequest) (*osc.DeleteRouteResponse, error) {
	response, _, err := s.client.RouteApi.DeleteRoute(s.ctx).DeleteRouteRequest(*request).Execute()
	return &response, err
}

func (s *oscSdkCompute) UpdateVM(request *osc.UpdateVmRequest) (*osc.UpdateVmResponse, error) {
	response, _, err := s.client.VmApi.UpdateVm(s.ctx).UpdateVmRequest(*request).Execute()
	return &response, err
}
