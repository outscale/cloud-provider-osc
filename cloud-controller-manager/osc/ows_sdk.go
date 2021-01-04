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

	"github.com/outscale/osc-sdk-go/osc"
	context "context"
	_nethttp "net/http"
)

// ********************* CCM oscSdkFCU Def & functions *********************

// oscSdkFCU is an implementation of the OSC interface, backed by osc-sdk-go
type oscSdkFCU struct {
	config *osc.Configuration
	auth   context.Context
	api    *osc.APIClient
	//fcu    *osc.
}

// Implementation of OSC.Vm
func (s *oscSdkFCU) ReadVms(request *osc.ReadVmsOpts) ([]osc.Vm, *_nethttp.Response, error) {
	// Instances are not paged
	response := osc.ReadVmsResponse{}
	requestTime := time.Now()
	var httpRes *_nethttp.Response
	for {
		response, httpRes, err := s.api.VmApi.ReadVms(s.auth, request)
		if err != nil {
			recordOSCMetric("describe_instance", 0, err)
			return nil, httpRes, fmt.Errorf("error listing OSC instances: %q", err)
		}
	}
	timeTaken := time.Since(requestTime).Seconds()
	recordOSCMetric("describe_instance", timeTaken, nil)
	return response.Vms, httpRes, nil
}

// Implements OSC.ReadSecurityGroups
func (s *oscSdkFCU) ReadSecurityGroups(request *osc.ReadSecurityGroupsOpts) ([]osc.SecurityGroup, *_nethttp.Response, error) {
	// Security groups are not paged
	results := osc.ReadSecurityGroupsResponse{}
	requestTime := time.Now()
	var httpRes *_nethttp.Response
	for {
		results, httpRes, err := s.api.SecurityGroupApi.ReadSecurityGroups(s.auth, request)
		if err != nil {
			recordOSCMetric("describe_security_groups", 0, err)
			return nil, httpRes, fmt.Errorf("error listing OSC security groups: %q", err)
		}
	}
	timeTaken := time.Since(requestTime).Seconds()
	recordOSCMetric("describe_security_groups", timeTaken, nil)
	return results.SecurityGroups, httpRes, nil
}

func (s *oscSdkFCU) ReadSubnets(request *osc.ReadSubnetsOpts) ([]osc.Subnet, *_nethttp.Response, error) {
	// Subnets are not paged
	response, httpRes, err := s.api.SubnetApi.ReadSubnets(s.auth, request)
	if err != nil {
		return nil, httpRes, fmt.Errorf("error listing OSC subnets: %q", err)
	}
	return response.Subnets, httpRes, nil
}

func (s *oscSdkFCU) CreateSecurityGroup(request *osc.CreateSecurityGroupOpts) (osc.CreateSecurityGroupResponse, *_nethttp.Response, error) {
	return s.api.SecurityGroupApi.CreateSecurityGroup(s.auth, request)
}

func (s *oscSdkFCU) DeleteSecurityGroup(request *osc.DeleteSecurityGroupOpts) (osc.DeleteSecurityGroupResponse, *_nethttp.Response, error) {
	return s.api.SecurityGroupApi.DeleteSecurityGroup(s.auth, request)
}

func (s *oscSdkFCU) CreateSecurityGroupRule(request *osc.CreateSecurityGroupRuleOpts) (osc.CreateSecurityGroupRuleResponse, *_nethttp.Response, error) {
	return s.api.SecurityGroupRuleApi.CreateSecurityGroupRule(s.auth, request)
}

func (s *oscSdkFCU) DeleteSecurityGroupRule(request *osc.DeleteSecurityGroupRuleOpts) (osc.DeleteSecurityGroupRuleResponse, *_nethttp.Response, error) {
	return s.api.SecurityGroupRuleApi.DeleteSecurityGroupRule(s.auth, request)
}

func (s *oscSdkFCU) CreateTags(request *osc.CreateTagsOpts) (osc.CreateTagsResponse, *_nethttp.Response, error) {
	debugPrintCallerFunctionName()
	requestTime := time.Now()
	resp, httpRes, err := s.api.TagApi.CreateTags(s.auth, request)
	timeTaken := time.Since(requestTime).Seconds()
	recordOSCMetric("create_tags", timeTaken, err)
	return resp, httpRes, err
}

func (s *oscSdkFCU) ReadRouteTables(request *osc.ReadRouteTablesOpts) ([]osc.RouteTable, *_nethttp.Response, error) {
	results := osc.ReadRouteTablesResponse{}
	requestTime := time.Now()
	var httpRes *_nethttp.Response
	for {
		results, httpRes, err := s.api.RouteTableApi.ReadRouteTables(s.auth, request)
		if err != nil {
			recordOSCMetric("describe_route_tables", 0, err)
			return nil, httpRes, fmt.Errorf("error listing OSC route tables: %q", err)
		}
        // A verifier
		 //results = append(results, results.RouteTables...)
	}
	timeTaken := time.Since(requestTime).Seconds()
	recordOSCMetric("describe_route_tables", timeTaken, nil)
	return results.RouteTables, httpRes, nil
}

func (s *oscSdkFCU) CreateRoute(request *osc.CreateRouteOpts) (osc.CreateRouteResponse, *_nethttp.Response, error) {
	return s.api.RouteApi.CreateRoute(s.auth, request)
}

func (s *oscSdkFCU) DeleteRoute(request *osc.DeleteRouteOpts) (osc.DeleteRouteResponse, *_nethttp.Response, error) {
	return s.api.RouteApi.DeleteRoute(s.auth, request)
}

func (s *oscSdkFCU) UpdateVm(request *osc.UpdateVmOpts) (osc.UpdateVmResponse, *_nethttp.Response, error) {
	return s.api.VmApi.UpdateVm(s.auth, request)
}

func (s *oscSdkFCU) ReadNets(request *osc.ReadNetsOpts) (osc.ReadNetsResponse, *_nethttp.Response, error) {
	return s.api.NetApi.ReadNets(s.auth, request)
}
