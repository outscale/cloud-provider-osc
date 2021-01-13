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

// ********************* CCM oscSdk Def & functions *********************

// oscSdk is an implementation of the FCU interface, backed by osc-sdk-go
type oscSdk struct {
	config *osc.Configuration
	auth   context.Context
	api    *osc.APIClient
	//fcu    *osc.
}


// Implementation of OSC.Vm
func (s *oscSdk) ReadVms(request *osc.ReadVmsOpts) ([]osc.Vm, *_nethttp.Response, error) {
	// Instances are not paged
	response := osc.ReadVmsResponse{}
	requestTime := time.Now()
	var httpRes *_nethttp.Response
	var err error
	response, httpRes, err = s.api.VmApi.ReadVms(s.auth, request)
	if err != nil {
		recordOSCMetric("read_vms", 0, err)
		return nil, httpRes, fmt.Errorf("error listing OSC instances: %q", err)
	}

	timeTaken := time.Since(requestTime).Seconds()
	recordOSCMetric("read_vms", timeTaken, nil)
	return response.Vms, httpRes, nil
}

// Implements OSC.ReadSecurityGroups
func (s *oscSdk) ReadSecurityGroups(request *osc.ReadSecurityGroupsOpts) ([]osc.SecurityGroup, *_nethttp.Response, error) {
	// Security groups are not paged
	results := osc.ReadSecurityGroupsResponse{}
	requestTime := time.Now()
	var httpRes *_nethttp.Response
	var err error
	results, httpRes, err = s.api.SecurityGroupApi.ReadSecurityGroups(s.auth, request)
	if err != nil {
		recordOSCMetric("read_security_groups", 0, err)
		return nil, httpRes, fmt.Errorf("error listing OSC security groups: %q", err)
	}

	timeTaken := time.Since(requestTime).Seconds()
	recordOSCMetric("read_security_groups", timeTaken, nil)
	return results.SecurityGroups, httpRes, nil
}

func (s *oscSdk) ReadSubnets(request *osc.ReadSubnetsOpts) ([]osc.Subnet, *_nethttp.Response, error) {
	// Subnets are not paged
	response, httpRes, err := s.api.SubnetApi.ReadSubnets(s.auth, request)
	if err != nil {
		return nil, httpRes, fmt.Errorf("error listing OSC subnets: %q", err)
	}
	return response.Subnets, httpRes, nil
}

func (s *oscSdk) CreateSecurityGroup(request *osc.CreateSecurityGroupOpts) (osc.CreateSecurityGroupResponse, *_nethttp.Response, error) {
	return s.api.SecurityGroupApi.CreateSecurityGroup(s.auth, request)
}

func (s *oscSdk) DeleteSecurityGroup(request *osc.DeleteSecurityGroupOpts) (osc.DeleteSecurityGroupResponse, *_nethttp.Response, error) {
	return s.api.SecurityGroupApi.DeleteSecurityGroup(s.auth, request)
}

func (s *oscSdk) CreateSecurityGroupRule(request *osc.CreateSecurityGroupRuleOpts) (osc.CreateSecurityGroupRuleResponse, *_nethttp.Response, error) {
	return s.api.SecurityGroupRuleApi.CreateSecurityGroupRule(s.auth, request)
}

func (s *oscSdk) DeleteSecurityGroupRule(request *osc.DeleteSecurityGroupRuleOpts) (osc.DeleteSecurityGroupRuleResponse, *_nethttp.Response, error) {
	return s.api.SecurityGroupRuleApi.DeleteSecurityGroupRule(s.auth, request)
}

func (s *oscSdk) CreateTags(request *osc.CreateTagsOpts) (osc.CreateTagsResponse, *_nethttp.Response, error) {
	debugPrintCallerFunctionName()
	requestTime := time.Now()
	resp, httpRes, err := s.api.TagApi.CreateTags(s.auth, request)
	timeTaken := time.Since(requestTime).Seconds()
	recordOSCMetric("create_tags", timeTaken, err)
	return resp, httpRes, err
}

func (s *oscSdk) ReadRouteTables(request *osc.ReadRouteTablesOpts) ([]osc.RouteTable, *_nethttp.Response, error) {
	results := osc.ReadRouteTablesResponse{}
	requestTime := time.Now()
	var httpRes *_nethttp.Response
	var err error
		results, httpRes, err = s.api.RouteTableApi.ReadRouteTables(s.auth, request)
		if err != nil {
			recordOSCMetric("read_route_tables", 0, err)
			return nil, httpRes, fmt.Errorf("error listing OSC route tables: %q", err)
		}

	timeTaken := time.Since(requestTime).Seconds()
	recordOSCMetric("read_route_tables", timeTaken, nil)
	return results.RouteTables, httpRes, nil
}

func (s *oscSdk) CreateRoute(request *osc.CreateRouteOpts) (osc.CreateRouteResponse, *_nethttp.Response, error) {
	return s.api.RouteApi.CreateRoute(s.auth, request)
}

func (s *oscSdk) DeleteRoute(request *osc.DeleteRouteOpts) (osc.DeleteRouteResponse, *_nethttp.Response, error) {
	return s.api.RouteApi.DeleteRoute(s.auth, request)
}

func (s *oscSdk) UpdateVm(request *osc.UpdateVmOpts) (osc.UpdateVmResponse, *_nethttp.Response, error) {
	return s.api.VmApi.UpdateVm(s.auth, request)
}

func (s *oscSdk) ReadNets(request *osc.ReadNetsOpts) (osc.ReadNetsResponse, *_nethttp.Response, error) {
	return s.api.NetApi.ReadNets(s.auth, request)
}




func (s *oscSdk) CreateLoadBalancer(request *osc.CreateLoadBalancerOpts) (osc.CreateLoadBalancerResponse, *_nethttp.Response, error) {
    return s.api.LoadBalancerApi.CreateLoadBalancer(s.auth, request)
}
func (s *oscSdk) DeleteLoadBalancer(request *osc.DeleteLoadBalancerOpts) (osc.DeleteLoadBalancerResponse, *_nethttp.Response, error) {
    return s.api.LoadBalancerApi.DeleteLoadBalancer(s.auth, request)
}
func (s *oscSdk) ReadLoadBalancers(request *osc.ReadLoadBalancersOpts) (osc.ReadLoadBalancersResponse, *_nethttp.Response, error) {
    return s.api.LoadBalancerApi.ReadLoadBalancers(s.auth, request)
}
func (s *oscSdk) UpdateLoadBalancer(request *osc.UpdateLoadBalancerOpts) (osc.UpdateLoadBalancerResponse, *_nethttp.Response, error) {
    return s.api.LoadBalancerApi.UpdateLoadBalancer(s.auth, request)
}
func (s *oscSdk) ReadLoadBalancerTags(request *osc.ReadLoadBalancerTagsOpts) (osc.ReadLoadBalancerTagsResponse, *_nethttp.Response, error) {
    return s.api.LoadBalancerApi.ReadLoadBalancerTags(s.auth, request)
}
func (s *oscSdk) CreateLoadBalancerTags(request *osc.CreateLoadBalancerTagsOpts) (osc.CreateLoadBalancerTagsResponse, *_nethttp.Response, error) {
    return s.api.LoadBalancerApi.CreateLoadBalancerTags(s.auth, request)
}
func (s *oscSdk) RegisterVmsInLoadBalancer(request *osc.RegisterVmsInLoadBalancerOpts) (osc.RegisterVmsInLoadBalancerResponse, *_nethttp.Response, error) {
    return s.api.LoadBalancerApi.RegisterVmsInLoadBalancer(s.auth, request)
}
func (s *oscSdk) DeregisterVmsInLoadBalancer(request *osc.DeregisterVmsInLoadBalancerOpts) (osc.DeregisterVmsInLoadBalancerResponse, *_nethttp.Response, error) {
    return s.api.LoadBalancerApi.DeregisterVmsInLoadBalancer(s.auth, request)
}
func (s *oscSdk) CreateLoadBalancerPolicy(request *osc.CreateLoadBalancerPolicyOpts) (osc.CreateLoadBalancerPolicyResponse, *_nethttp.Response, error) {
    return s.api.LoadBalancerPolicyApi.CreateLoadBalancerPolicy(s.auth, request)
}
func (s *oscSdk) CreateLoadBalancerListeners(request *osc.CreateLoadBalancerListenersOpts) (osc.CreateLoadBalancerListenersResponse, *_nethttp.Response, error) {
    return s.api.ListenerApi.CreateLoadBalancerListeners(s.auth, request)
}
func (s *oscSdk) DeleteLoadBalancerListeners(request *osc.DeleteLoadBalancerListenersOpts) (osc.DeleteLoadBalancerListenersResponse, *_nethttp.Response, error) {
    return s.api.ListenerApi.DeleteLoadBalancerListeners(s.auth, request)
}
func (s *oscSdk) ReadVmsHealth(request *osc.ReadVmsHealthOpts) (osc.ReadVmsHealthResponse, *_nethttp.Response, error) {
    return s.api.LoadBalancerApi.ReadVmsHealth(s.auth, request)
}