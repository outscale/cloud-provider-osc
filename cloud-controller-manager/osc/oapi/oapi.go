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

	osc "github.com/outscale/osc-sdk-go/v3/pkg/osc"
)

const (
	PublicIPPoolTag = "OscK8sIPPool"
)

func (c *OscClient) APIClient() *osc.Client {
	return c.api
}

func (c *OscClient) ReadVms(ctx context.Context, req osc.ReadVmsRequest) ([]osc.Vm, error) {
	resp, err := c.api.ReadVms(ctx, req)
	if err != nil {
		return nil, err
	}
	return *resp.Vms, nil
}

func (c *OscClient) ListPublicIpsFromPool(ctx context.Context, pool string) ([]osc.PublicIp, error) {
	req := osc.ReadPublicIpsRequest{
		Filters: &osc.FiltersPublicIp{
			TagKeys:   &[]string{PublicIPPoolTag},
			TagValues: &[]string{pool},
		},
	}
	resp, err := c.api.ReadPublicIps(ctx, req)
	if err != nil {
		return nil, err
	}
	return *resp.PublicIps, nil
}

func (c *OscClient) ReadLoadBalancers(ctx context.Context, req osc.ReadLoadBalancersRequest) ([]osc.LoadBalancer, error) {
	resp, err := c.api.ReadLoadBalancers(ctx, req)
	if err != nil {
		return nil, err
	}
	return *resp.LoadBalancers, nil
}

func (c *OscClient) ReadLoadBalancerTags(ctx context.Context, req osc.ReadLoadBalancerTagsRequest) ([]osc.LoadBalancerTag, error) {
	resp, err := c.api.ReadLoadBalancerTags(ctx, req)
	if err != nil {
		return nil, err
	}
	return *resp.Tags, nil
}

func (c *OscClient) CreateLoadBalancer(ctx context.Context, req osc.CreateLoadBalancerRequest) (*osc.LoadBalancer, error) {
	resp, err := c.api.CreateLoadBalancer(ctx, req)
	if err != nil {
		return nil, err
	}
	return resp.LoadBalancer, nil
}

func (c *OscClient) CreateLoadBalancerListeners(ctx context.Context, req osc.CreateLoadBalancerListenersRequest) (*osc.LoadBalancer, error) {
	resp, err := c.api.CreateLoadBalancerListeners(ctx, req)
	if err != nil {
		return nil, err
	}
	return resp.LoadBalancer, nil
}

func (c *OscClient) DeleteLoadBalancer(ctx context.Context, req osc.DeleteLoadBalancerRequest) error {
	_, err := c.api.DeleteLoadBalancer(ctx, req)
	return err
}

func (c *OscClient) DeleteLoadBalancerListeners(ctx context.Context, req osc.DeleteLoadBalancerListenersRequest) (*osc.LoadBalancer, error) {
	resp, err := c.api.DeleteLoadBalancerListeners(ctx, req)
	if err != nil {
		return nil, err
	}
	return resp.LoadBalancer, nil
}

func (c *OscClient) RegisterVmsInLoadBalancer(ctx context.Context, req osc.RegisterVmsInLoadBalancerRequest) error {
	_, err := c.api.RegisterVmsInLoadBalancer(ctx, req)
	return err
}

func (c *OscClient) DeregisterVmsInLoadBalancer(ctx context.Context, req osc.DeregisterVmsInLoadBalancerRequest) error {
	_, err := c.api.DeregisterVmsInLoadBalancer(ctx, req)
	return err
}

func (c *OscClient) UpdateLoadBalancer(ctx context.Context, req osc.UpdateLoadBalancerRequest) error {
	_, err := c.api.UpdateLoadBalancer(ctx, req)
	return err
}

func (c *OscClient) ReadSecurityGroups(ctx context.Context, req osc.ReadSecurityGroupsRequest) ([]osc.SecurityGroup, error) {
	resp, err := c.api.ReadSecurityGroups(ctx, req)
	if err != nil {
		return nil, err
	}
	return *resp.SecurityGroups, nil
}

func (c *OscClient) ReadSubnets(ctx context.Context, req osc.ReadSubnetsRequest) ([]osc.Subnet, error) {
	// Subnets are not paged
	resp, err := c.api.ReadSubnets(ctx, req)
	if err != nil {
		return nil, err
	}
	return *resp.Subnets, nil
}

func (c *OscClient) CreateSecurityGroup(ctx context.Context, req osc.CreateSecurityGroupRequest) (*osc.SecurityGroup, error) {
	resp, err := c.api.CreateSecurityGroup(ctx, req)
	if err != nil {
		return nil, err
	}
	return resp.SecurityGroup, nil
}

func (c *OscClient) DeleteSecurityGroup(ctx context.Context, req osc.DeleteSecurityGroupRequest) error {
	_, err := c.api.DeleteSecurityGroup(ctx, req)
	return err
}

func (c *OscClient) CreateSecurityGroupRule(ctx context.Context, req osc.CreateSecurityGroupRuleRequest) (*osc.SecurityGroup, error) {
	resp, err := c.api.CreateSecurityGroupRule(ctx, req)
	if err != nil {
		return nil, err
	}
	return resp.SecurityGroup, nil
}

func (c *OscClient) DeleteSecurityGroupRule(ctx context.Context, req osc.DeleteSecurityGroupRuleRequest) (*osc.SecurityGroup, error) {
	resp, err := c.api.DeleteSecurityGroupRule(ctx, req)
	if err != nil {
		return nil, err
	}
	return resp.SecurityGroup, nil
}

func (c *OscClient) CreateTags(ctx context.Context, req osc.CreateTagsRequest) error {
	_, err := c.api.CreateTags(ctx, req)
	return err
}

func (c *OscClient) ReadRouteTables(ctx context.Context, req osc.ReadRouteTablesRequest) ([]osc.RouteTable, error) {
	resp, err := c.api.ReadRouteTables(ctx, req)
	if err != nil {
		return nil, err
	}
	return *resp.RouteTables, nil
}

func (c *OscClient) CreateRoute(ctx context.Context, req osc.CreateRouteRequest) (*osc.RouteTable, error) {
	resp, err := c.api.CreateRoute(ctx, req)
	if err != nil {
		return nil, err
	}
	return resp.RouteTable, nil
}

func (c *OscClient) DeleteRoute(ctx context.Context, req osc.DeleteRouteRequest) (*osc.RouteTable, error) {
	resp, err := c.api.DeleteRoute(ctx, req)
	if err != nil {
		return nil, err
	}
	return resp.RouteTable, nil
}
