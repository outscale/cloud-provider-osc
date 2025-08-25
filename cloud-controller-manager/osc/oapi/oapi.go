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

func (c *OscClient) APIClient() *osc.APIClient {
	return c.api
}

func (c *OscClient) ReadVms(ctx context.Context, req osc.ReadVmsRequest) ([]osc.Vm, error) {
	// Instances are paged
	resp, httpRes, err := c.api.VmApi.ReadVms(c.WithAuth(ctx)).ReadVmsRequest(req).Execute()
	err = logAndExtractError(ctx, "ReadVms", req, httpRes, err)
	if err != nil {
		return nil, err
	}
	return resp.GetVms(), nil
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
