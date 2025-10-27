/*
SPDX-FileCopyrightText: 2025 Outscale SAS <opensource@outscale.com>

SPDX-License-Identifier: BSD-3-Clause
*/
package oapi

import (
	"context"

	"github.com/aws/aws-sdk-go/aws/request" //nolint:staticcheck
	"github.com/aws/aws-sdk-go/service/elb" //nolint:staticcheck
)

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
