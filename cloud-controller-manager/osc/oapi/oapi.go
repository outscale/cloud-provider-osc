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
	"context"

	osc "github.com/outscale/osc-sdk-go/v2"
)

const (
	PublicIPPoolTag = "OscK8sIPPool"
)

func (c *OscClient) APIClient() *osc.APIClient {
	return c.api
}

func (c *OscClient) ReadVms(ctx context.Context, req osc.ReadVmsRequest) ([]osc.Vm, error) {
	resp, httpRes, err := c.api.VmApi.ReadVms(c.WithAuth(ctx)).ReadVmsRequest(req).Execute()
	err = logAndExtractError(ctx, "ReadVms", req, httpRes, err)
	if err != nil {
		return nil, err
	}
	return resp.GetVms(), nil
}

func (c *OscClient) ListPublicIpsFromPool(ctx context.Context, pool string) ([]osc.PublicIp, error) {
	req := osc.ReadPublicIpsRequest{
		Filters: &osc.FiltersPublicIp{
			TagKeys:   &[]string{PublicIPPoolTag},
			TagValues: &[]string{pool},
		},
	}
	resp, httpRes, err := c.api.PublicIpApi.ReadPublicIps(c.WithAuth(ctx)).ReadPublicIpsRequest(req).Execute()
	err = logAndExtractError(ctx, "ReadPublicIps", req, httpRes, err)
	if err != nil {
		return nil, err
	}
	return resp.GetPublicIps(), nil
}

func (c *OscClient) ReadLoadBalancers(ctx context.Context, req osc.ReadLoadBalancersRequest) ([]osc.LoadBalancer, error) {
	resp, httpRes, err := c.api.LoadBalancerApi.ReadLoadBalancers(c.WithAuth(ctx)).ReadLoadBalancersRequest(req).Execute()
	err = logAndExtractError(ctx, "ReadLoadBalancers", req, httpRes, err)
	if err != nil {
		return nil, err
	}
	return resp.GetLoadBalancers(), nil
}

func (c *OscClient) ReadLoadBalancerTags(ctx context.Context, req osc.ReadLoadBalancerTagsRequest) ([]osc.LoadBalancerTag, error) {
	resp, httpRes, err := c.api.LoadBalancerApi.ReadLoadBalancerTags(c.WithAuth(ctx)).ReadLoadBalancerTagsRequest(req).Execute()
	err = logAndExtractError(ctx, "ReadLoadBalancerTags", req, httpRes, err)
	if err != nil {
		return nil, err
	}
	return resp.GetTags(), nil
}

func (c *OscClient) CreateLoadBalancer(ctx context.Context, req osc.CreateLoadBalancerRequest) (*osc.LoadBalancer, error) {
	resp, httpRes, err := c.api.LoadBalancerApi.CreateLoadBalancer(c.WithAuth(ctx)).CreateLoadBalancerRequest(req).Execute()
	err = logAndExtractError(ctx, "CreateLoadBalancer", req, httpRes, err)
	if err != nil {
		return nil, err
	}
	return resp.LoadBalancer, nil
}

func (c *OscClient) CreateLoadBalancerListeners(ctx context.Context, req osc.CreateLoadBalancerListenersRequest) (*osc.LoadBalancer, error) {
	resp, httpRes, err := c.api.ListenerApi.CreateLoadBalancerListeners(c.WithAuth(ctx)).CreateLoadBalancerListenersRequest(req).Execute()
	err = logAndExtractError(ctx, "CreateLoadBalancerListeners", req, httpRes, err)
	if err != nil {
		return nil, err
	}
	return resp.LoadBalancer, nil
}

func (c *OscClient) DeleteLoadBalancer(ctx context.Context, req osc.DeleteLoadBalancerRequest) error {
	_, httpRes, err := c.api.LoadBalancerApi.DeleteLoadBalancer(c.WithAuth(ctx)).DeleteLoadBalancerRequest(req).Execute()
	err = logAndExtractError(ctx, "DeleteLoadBalancer", req, httpRes, err)
	return err
}

func (c *OscClient) DeleteLoadBalancerListeners(ctx context.Context, req osc.DeleteLoadBalancerListenersRequest) (*osc.LoadBalancer, error) {
	resp, httpRes, err := c.api.ListenerApi.DeleteLoadBalancerListeners(c.WithAuth(ctx)).DeleteLoadBalancerListenersRequest(req).Execute()
	err = logAndExtractError(ctx, "DeleteLoadBalancerListeners", req, httpRes, err)
	if err != nil {
		return nil, err
	}
	return resp.LoadBalancer, nil
}

func (c *OscClient) RegisterVmsInLoadBalancer(ctx context.Context, req osc.RegisterVmsInLoadBalancerRequest) error {
	_, httpRes, err := c.api.LoadBalancerApi.RegisterVmsInLoadBalancer(c.WithAuth(ctx)).RegisterVmsInLoadBalancerRequest(req).Execute()
	err = logAndExtractError(ctx, "RegisterVmsInLoadBalancer", req, httpRes, err)
	return err
}

func (c *OscClient) DeregisterVmsInLoadBalancer(ctx context.Context, req osc.DeregisterVmsInLoadBalancerRequest) error {
	_, httpRes, err := c.api.LoadBalancerApi.DeregisterVmsInLoadBalancer(c.WithAuth(ctx)).DeregisterVmsInLoadBalancerRequest(req).Execute()
	err = logAndExtractError(ctx, "DeregisterVmsInLoadBalancer", req, httpRes, err)
	return err
}

func (c *OscClient) UpdateLoadBalancer(ctx context.Context, req osc.UpdateLoadBalancerRequest) error {
	_, httpRes, err := c.api.LoadBalancerApi.UpdateLoadBalancer(c.WithAuth(ctx)).UpdateLoadBalancerRequest(req).Execute()
	err = logAndExtractError(ctx, "UpdateLoadBalancer", req, httpRes, err)
	return err
}

func (c *OscClient) ReadSecurityGroups(ctx context.Context, req osc.ReadSecurityGroupsRequest) ([]osc.SecurityGroup, error) {
	resp, httpRes, err := c.api.SecurityGroupApi.ReadSecurityGroups(c.WithAuth(ctx)).ReadSecurityGroupsRequest(req).Execute()
	err = logAndExtractError(ctx, "ReadSecurityGroups", req, httpRes, err)
	if err != nil {
		return nil, err
	}
	return resp.GetSecurityGroups(), nil
}

func (c *OscClient) ReadSubnets(ctx context.Context, req osc.ReadSubnetsRequest) ([]osc.Subnet, error) {
	// Subnets are not paged
	resp, httpRes, err := c.api.SubnetApi.ReadSubnets(c.WithAuth(ctx)).ReadSubnetsRequest(req).Execute()
	err = logAndExtractError(ctx, "ReadSubnets", req, httpRes, err)
	if err != nil {
		return nil, err
	}
	return resp.GetSubnets(), nil
}

func (c *OscClient) CreateSecurityGroup(ctx context.Context, req osc.CreateSecurityGroupRequest) (*osc.SecurityGroup, error) {
	resp, httpRes, err := c.api.SecurityGroupApi.CreateSecurityGroup(c.WithAuth(ctx)).CreateSecurityGroupRequest(req).Execute()
	err = logAndExtractError(ctx, "CreateSecurityGroup", req, httpRes, err)
	if err != nil {
		return nil, err
	}
	return resp.SecurityGroup, err
}

func (c *OscClient) DeleteSecurityGroup(ctx context.Context, req osc.DeleteSecurityGroupRequest) error {
	_, httpRes, err := c.api.SecurityGroupApi.DeleteSecurityGroup(c.WithAuth(ctx)).DeleteSecurityGroupRequest(req).Execute()
	return logAndExtractError(ctx, "DeleteSecurityGroup", req, httpRes, err)
}

func (c *OscClient) CreateSecurityGroupRule(ctx context.Context, req osc.CreateSecurityGroupRuleRequest) (*osc.SecurityGroup, error) {
	resp, httpRes, err := c.api.SecurityGroupRuleApi.CreateSecurityGroupRule(c.WithAuth(ctx)).CreateSecurityGroupRuleRequest(req).Execute()
	err = logAndExtractError(ctx, "CreateSecurityGroupRule", req, httpRes, err)
	if err != nil {
		return nil, err
	}
	return resp.SecurityGroup, err
}

func (c *OscClient) DeleteSecurityGroupRule(ctx context.Context, req osc.DeleteSecurityGroupRuleRequest) (*osc.SecurityGroup, error) {
	resp, httpRes, err := c.api.SecurityGroupRuleApi.DeleteSecurityGroupRule(c.WithAuth(ctx)).DeleteSecurityGroupRuleRequest(req).Execute()
	err = logAndExtractError(ctx, "DeleteSecurityGroupRule", req, httpRes, err)
	if err != nil {
		return nil, err
	}
	return resp.SecurityGroup, nil
}

func (c *OscClient) CreateTags(ctx context.Context, req osc.CreateTagsRequest) error {
	_, httpRes, err := c.api.TagApi.CreateTags(c.WithAuth(ctx)).CreateTagsRequest(req).Execute()
	return logAndExtractError(ctx, "CreateTags", req, httpRes, err)
}

func (c *OscClient) ReadRouteTables(ctx context.Context, req osc.ReadRouteTablesRequest) ([]osc.RouteTable, error) {
	resp, httpRes, err := c.api.RouteTableApi.ReadRouteTables(c.WithAuth(ctx)).ReadRouteTablesRequest(req).Execute()
	err = logAndExtractError(ctx, "ReadRouteTables", req, httpRes, err)
	if err != nil {
		return nil, err
	}
	return resp.GetRouteTables(), nil
}

func (c *OscClient) CreateRoute(ctx context.Context, req osc.CreateRouteRequest) (*osc.RouteTable, error) {
	resp, httpRes, err := c.api.RouteApi.CreateRoute(c.WithAuth(ctx)).CreateRouteRequest(req).Execute()
	err = logAndExtractError(ctx, "CreateRoute", req, httpRes, err)
	if err != nil {
		return nil, err
	}
	return resp.RouteTable, nil
}

func (c *OscClient) DeleteRoute(ctx context.Context, req osc.DeleteRouteRequest) (*osc.RouteTable, error) {
	resp, httpRes, err := c.api.RouteApi.DeleteRoute(c.WithAuth(ctx)).DeleteRouteRequest(req).Execute()
	err = logAndExtractError(ctx, "DeleteRoute", req, httpRes, err)
	if err != nil {
		return nil, err
	}
	return resp.RouteTable, nil
}

func (c *OscClient) UpdateVM(ctx context.Context, req osc.UpdateVmRequest) (*osc.Vm, error) {
	resp, httpRes, err := c.api.VmApi.UpdateVm(c.WithAuth(ctx)).UpdateVmRequest(req).Execute()
	err = logAndExtractError(ctx, "UpdateVM", req, httpRes, err)
	if err != nil {
		return nil, err
	}
	return resp.Vm, nil
}
