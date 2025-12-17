/*
SPDX-FileCopyrightText: 2025 Outscale SAS <opensource@outscale.com>

SPDX-License-Identifier: BSD-3-Clause
*/
package oapi

import (
	"context"

	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/elb"
	osc "github.com/outscale/osc-sdk-go/v2"
)

// OAPI is the interface for OAPI calls.
type OAPI interface {
	CheckCredentials(ctx context.Context) error

	ReadVms(ctx context.Context, req osc.ReadVmsRequest) ([]osc.Vm, error)

	ReadLoadBalancers(ctx context.Context, req osc.ReadLoadBalancersRequest) ([]osc.LoadBalancer, error)
	CreateLoadBalancer(ctx context.Context, req osc.CreateLoadBalancerRequest) (*osc.LoadBalancer, error)
	UpdateLoadBalancer(ctx context.Context, req osc.UpdateLoadBalancerRequest) error
	CreateLoadBalancerListeners(ctx context.Context, req osc.CreateLoadBalancerListenersRequest) (*osc.LoadBalancer, error)
	DeleteLoadBalancer(ctx context.Context, req osc.DeleteLoadBalancerRequest) error
	DeleteLoadBalancerListeners(ctx context.Context, req osc.DeleteLoadBalancerListenersRequest) (*osc.LoadBalancer, error)
	RegisterVmsInLoadBalancer(ctx context.Context, req osc.RegisterVmsInLoadBalancerRequest) error
	DeregisterVmsInLoadBalancer(ctx context.Context, req osc.DeregisterVmsInLoadBalancerRequest) error

	GetPublicIp(ctx context.Context, id string) (*osc.PublicIp, error)
	ListPublicIpsFromPool(ctx context.Context, pool string) ([]osc.PublicIp, error)

	ReadSecurityGroups(ctx context.Context, req osc.ReadSecurityGroupsRequest) ([]osc.SecurityGroup, error)
	CreateSecurityGroup(ctx context.Context, req osc.CreateSecurityGroupRequest) (*osc.SecurityGroup, error)
	DeleteSecurityGroup(ctx context.Context, req osc.DeleteSecurityGroupRequest) error

	CreateSecurityGroupRule(ctx context.Context, req osc.CreateSecurityGroupRuleRequest) (*osc.SecurityGroup, error)
	DeleteSecurityGroupRule(ctx context.Context, req osc.DeleteSecurityGroupRuleRequest) (*osc.SecurityGroup, error)

	ReadSubnets(ctx context.Context, req osc.ReadSubnetsRequest) ([]osc.Subnet, error)
	ReadRouteTables(ctx context.Context, req osc.ReadRouteTablesRequest) ([]osc.RouteTable, error)

	CreateTags(ctx context.Context, req osc.CreateTagsRequest) error
}

// LBU is the interface for API calls using the AWS LBU gateway.
type LBU interface {
	DescribeLoadBalancersWithContext(ctx context.Context, req *elb.DescribeLoadBalancersInput, opts ...request.Option) (*elb.DescribeLoadBalancersOutput, error)

	CreateLoadBalancerPolicyWithContext(ctx context.Context, req *elb.CreateLoadBalancerPolicyInput, opts ...request.Option) (*elb.CreateLoadBalancerPolicyOutput, error)
	SetLoadBalancerPoliciesForBackendServerWithContext(ctx context.Context, req *elb.SetLoadBalancerPoliciesForBackendServerInput, opts ...request.Option) (*elb.SetLoadBalancerPoliciesForBackendServerOutput, error)

	DescribeLoadBalancerAttributesWithContext(ctx context.Context, req *elb.DescribeLoadBalancerAttributesInput, opts ...request.Option) (*elb.DescribeLoadBalancerAttributesOutput, error)
	ModifyLoadBalancerAttributesWithContext(ctx context.Context, req *elb.ModifyLoadBalancerAttributesInput, opts ...request.Option) (*elb.ModifyLoadBalancerAttributesOutput, error)
}

// EC2Metadata is an abstraction over the AWS metadata service.
type EC2Metadata interface {
	Available() bool
	// Query the EC2 metadata service (used to discover instance-id etc)
	GetMetadata(path string) (string, error)
}
