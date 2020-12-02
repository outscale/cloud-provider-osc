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
	"github.com/aws/aws-sdk-go/aws/ec2metadata"

    "context"

	"github.com/outscale/osc-sdk-go/osc"
)

// ********************* CCM API interfaces *********************

// OSC is an abstraction over AWS', to allow mocking/other implementations
// Note that the DescribeX functions return a list, so callers don't need to deal with paging
type OSC interface {
	// Query OSC for instances matching the filter
	ReadVms(*osc.ReadVmsOpts) ([]osc.Vm, error)
	ReadSecurityGroups(*osc.ReadSecurityGroupsOpts) ([]osc.SecurityGroups, error)

    CreateSecurityGroup(*osc.CreateSecurityGroupOpts) (osc.CreateSecurityGroupResponse, error)
    DeleteSecurityGroup(*osc.DeleteSecurityGroupOpts) (osc.DeleteSecurityGroupResponse, error)

    CreateSecurityGroupRule(*osc.CreateSecurityGroupRuleOpts) (osc.CreateSecurityGroupRuleResponse, error)
    DeleteSecurityGroupRule(*osc.DeleteSecurityGroupRuleOpts) (osc.DeleteSecurityGroupRuleResponse, error)

    ReadSubnets(*osc.ReadSubnetsOpts) ([]osc.Subnet, error)

    CreateTags(*osc.CreateTagsOpts) (osc.CreateTagsResponse, error)

    ReadRouteTables(*osc.ReadRouteTablesOpts) ([]osc.RouteTables, error)
    CreateRouteTable(*osc.CreateRouteTableOpts) (osc.CreateRouteTableResponse, error)
    DeleteRouteTable(*osc.DeleteRouteTableOpts) (osc.DeleteRouteTableResponse, error)

    UpdateVm(*osc.UpdateVmOpts) (osc.UpdateVmResponse, error)

    ReadNets(*osc.ReadNetsOpts) (osc.ReadNetsResponse, error)
}

// LBU is a simple pass-through of OSC' LBU client interface, which allows for testing
type LBU interface {
    CreateLoadBalancer(*osc.CreateLoadBalancerOpts) (osc.CreateLoadBalancerResponse, error)
    DeleteLoadBalancer(*osc.DeleteLoadBalancerOpts) (osc.DeleteLoadBalancerResponse, error)
    ReadLoadBalancerTags(*osc.ReadLoadBalancerTagsOpts) (osc.ReadLoadBalancerTagsResponse, error)
    CreateLoadBalancerTags(*osc.CreateLoadBalancerTagsOpts) (osc.CreateLoadBalancerTagsResponse, error)
    RegisterVmsInLoadBalancer(*osc.RegisterVmsInLoadBalancerOpts) (osc.RegisterVmsInLoadBalancerResponse, error)
    DeregisterVmsInLoadBalancer(*osc.DeregisterVmsInLoadBalancerOpts) (osc.DeregisterVmsInLoadBalancerResponse, error)
    CreateLoadBalancerPolicy(*osc.CreateLoadBalancerPolicyOpts) (osc.CreateLoadBalancerPolicyResponse, error)


	//CreateLoadBalancer(*elb.CreateLoadBalancerInput) (*elb.CreateLoadBalancerOutput, error)
	//DeleteLoadBalancer(*elb.DeleteLoadBalancerInput) (*elb.DeleteLoadBalancerOutput, error)
	//DescribeLoadBalancers(*elb.DescribeLoadBalancersInput) (*elb.DescribeLoadBalancersOutput, error)
	//AddTags(*elb.AddTagsInput) (*elb.AddTagsOutput, error)
	//RegisterInstancesWithLoadBalancer(*elb.RegisterInstancesWithLoadBalancerInput) (*elb.RegisterInstancesWithLoadBalancerOutput, error)
	//DeregisterInstancesFromLoadBalancer(*elb.DeregisterInstancesFromLoadBalancerInput) (*elb.DeregisterInstancesFromLoadBalancerOutput, error)
	//CreateLoadBalancerPolicy(*elb.CreateLoadBalancerPolicyInput) (*elb.CreateLoadBalancerPolicyOutput, error)
	SetLoadBalancerPoliciesForBackendServer(*elb.SetLoadBalancerPoliciesForBackendServerInput) (*elb.SetLoadBalancerPoliciesForBackendServerOutput, error)
	SetLoadBalancerPoliciesOfListener(input *elb.SetLoadBalancerPoliciesOfListenerInput) (*elb.SetLoadBalancerPoliciesOfListenerOutput, error)
	DescribeLoadBalancerPolicies(input *elb.DescribeLoadBalancerPoliciesInput) (*elb.DescribeLoadBalancerPoliciesOutput, error)

	DetachLoadBalancerFromSubnets(*elb.DetachLoadBalancerFromSubnetsInput) (*elb.DetachLoadBalancerFromSubnetsOutput, error)
	AttachLoadBalancerToSubnets(*elb.AttachLoadBalancerToSubnetsInput) (*elb.AttachLoadBalancerToSubnetsOutput, error)

	CreateLoadBalancerListeners(*elb.CreateLoadBalancerListenersInput) (*elb.CreateLoadBalancerListenersOutput, error)
	DeleteLoadBalancerListeners(*elb.DeleteLoadBalancerListenersInput) (*elb.DeleteLoadBalancerListenersOutput, error)

	ApplySecurityGroupsToLoadBalancer(*elb.ApplySecurityGroupsToLoadBalancerInput) (*elb.ApplySecurityGroupsToLoadBalancerOutput, error)

	ConfigureHealthCheck(*elb.ConfigureHealthCheckInput) (*elb.ConfigureHealthCheckOutput, error)

	DescribeLoadBalancerAttributes(*elb.DescribeLoadBalancerAttributesInput) (*elb.DescribeLoadBalancerAttributesOutput, error)
	ModifyLoadBalancerAttributes(*elb.ModifyLoadBalancerAttributesInput) (*elb.ModifyLoadBalancerAttributesOutput, error)
}

// EC2Metadata is an abstraction over the AWS metadata service.
type EC2Metadata interface {
	Available() bool
	GetInstanceIdentityDocument() (ec2metadata.EC2InstanceIdentityDocument, error)
	// Query the EC2 metadata service (used to discover instance-id etc)
	GetMetadata(path string) (string, error)
}
