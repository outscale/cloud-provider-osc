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

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/ec2metadata"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/elb"
	osc "github.com/outscale/osc-sdk-go/v2"
)

// OAPI is the interface for OAPI calls.
type OAPI interface {
	ReadVms(ctx context.Context, req osc.ReadVmsRequest) ([]osc.Vm, error)
	UpdateVM(ctx context.Context, req osc.UpdateVmRequest) (*osc.Vm, error)

	ReadSecurityGroups(ctx context.Context, req osc.ReadSecurityGroupsRequest) ([]osc.SecurityGroup, error)
	CreateSecurityGroup(ctx context.Context, req osc.CreateSecurityGroupRequest) (*osc.SecurityGroup, error)
	DeleteSecurityGroup(ctx context.Context, req osc.DeleteSecurityGroupRequest) error

	CreateSecurityGroupRule(ctx context.Context, req osc.CreateSecurityGroupRuleRequest) (*osc.SecurityGroup, error)
	DeleteSecurityGroupRule(ctx context.Context, req osc.DeleteSecurityGroupRuleRequest) (*osc.SecurityGroup, error)

	ReadSubnets(ctx context.Context, req osc.ReadSubnetsRequest) ([]osc.Subnet, error)

	CreateTags(ctx context.Context, req osc.CreateTagsRequest) error

	ReadRouteTables(ctx context.Context, req osc.ReadRouteTablesRequest) ([]osc.RouteTable, error)
	CreateRoute(ctx context.Context, req osc.CreateRouteRequest) (*osc.RouteTable, error)
	DeleteRoute(ctx context.Context, req osc.DeleteRouteRequest) (*osc.RouteTable, error)
}

// LoadBalancer is the interface for API calls using the AWS gateway.
type LoadBalancer interface {
	CreateLoadBalancerWithContext(ctx aws.Context, req *elb.CreateLoadBalancerInput, opts ...request.Option) (*elb.CreateLoadBalancerOutput, error)
	DeleteLoadBalancerWithContext(ctx aws.Context, req *elb.DeleteLoadBalancerInput, opts ...request.Option) (*elb.DeleteLoadBalancerOutput, error)
	DescribeLoadBalancersWithContext(ctx aws.Context, req *elb.DescribeLoadBalancersInput, opts ...request.Option) (*elb.DescribeLoadBalancersOutput, error)

	AddTagsWithContext(ctx aws.Context, req *elb.AddTagsInput, opts ...request.Option) (*elb.AddTagsOutput, error)
	DescribeTagsWithContext(ctx aws.Context, req *elb.DescribeTagsInput, opts ...request.Option) (*elb.DescribeTagsOutput, error)

	RegisterInstancesWithLoadBalancerWithContext(ctx aws.Context, req *elb.RegisterInstancesWithLoadBalancerInput, opts ...request.Option) (*elb.RegisterInstancesWithLoadBalancerOutput, error)
	DeregisterInstancesFromLoadBalancerWithContext(ctx aws.Context, req *elb.DeregisterInstancesFromLoadBalancerInput, opts ...request.Option) (*elb.DeregisterInstancesFromLoadBalancerOutput, error)
	CreateLoadBalancerPolicyWithContext(ctx aws.Context, req *elb.CreateLoadBalancerPolicyInput, opts ...request.Option) (*elb.CreateLoadBalancerPolicyOutput, error)

	SetLoadBalancerPoliciesForBackendServerWithContext(ctx aws.Context, req *elb.SetLoadBalancerPoliciesForBackendServerInput, opts ...request.Option) (*elb.SetLoadBalancerPoliciesForBackendServerOutput, error)
	SetLoadBalancerPoliciesOfListenerWithContext(ctx aws.Context, req *elb.SetLoadBalancerPoliciesOfListenerInput, opts ...request.Option) (*elb.SetLoadBalancerPoliciesOfListenerOutput, error)
	DescribeLoadBalancerPoliciesWithContext(ctx aws.Context, req *elb.DescribeLoadBalancerPoliciesInput, opts ...request.Option) (*elb.DescribeLoadBalancerPoliciesOutput, error)

	DetachLoadBalancerFromSubnetsWithContext(ctx aws.Context, req *elb.DetachLoadBalancerFromSubnetsInput, opts ...request.Option) (*elb.DetachLoadBalancerFromSubnetsOutput, error)
	AttachLoadBalancerToSubnetsWithContext(ctx aws.Context, req *elb.AttachLoadBalancerToSubnetsInput, opts ...request.Option) (*elb.AttachLoadBalancerToSubnetsOutput, error)

	CreateLoadBalancerListenersWithContext(ctx aws.Context, req *elb.CreateLoadBalancerListenersInput, opts ...request.Option) (*elb.CreateLoadBalancerListenersOutput, error)
	DeleteLoadBalancerListenersWithContext(ctx aws.Context, req *elb.DeleteLoadBalancerListenersInput, opts ...request.Option) (*elb.DeleteLoadBalancerListenersOutput, error)
	SetLoadBalancerListenerSSLCertificateWithContext(ctx aws.Context, req *elb.SetLoadBalancerListenerSSLCertificateInput, opts ...request.Option) (*elb.SetLoadBalancerListenerSSLCertificateOutput, error)

	ApplySecurityGroupsToLoadBalancerWithContext(ctx aws.Context, req *elb.ApplySecurityGroupsToLoadBalancerInput, opts ...request.Option) (*elb.ApplySecurityGroupsToLoadBalancerOutput, error)

	ConfigureHealthCheckWithContext(ctx aws.Context, req *elb.ConfigureHealthCheckInput, opts ...request.Option) (*elb.ConfigureHealthCheckOutput, error)

	DescribeLoadBalancerAttributesWithContext(ctx aws.Context, req *elb.DescribeLoadBalancerAttributesInput, opts ...request.Option) (*elb.DescribeLoadBalancerAttributesOutput, error)
	ModifyLoadBalancerAttributesWithContext(ctx aws.Context, req *elb.ModifyLoadBalancerAttributesInput, opts ...request.Option) (*elb.ModifyLoadBalancerAttributesOutput, error)
}

// EC2Metadata is an abstraction over the AWS metadata service.
type EC2Metadata interface {
	Available() bool
	GetInstanceIdentityDocument() (ec2metadata.EC2InstanceIdentityDocument, error)
	// Query the EC2 metadata service (used to discover instance-id etc)
	GetMetadata(path string) (string, error)
}
