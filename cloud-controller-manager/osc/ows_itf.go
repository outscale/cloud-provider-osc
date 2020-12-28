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
    _nethttp "net/http"

	"github.com/outscale/osc-sdk-go/osc"
)

// ********************* CCM API interfaces *********************

// FCU is an abstraction over AWS', to allow mocking/other implementations
// Note that the DescribeX functions return a list, so callers don't need to deal with paging
type FCU interface {
	// Query OSC for instances matching the filter
	ReadVms(context.Context, *osc.ReadVmsOpts) ([]osc.Vm, *_nethttp.Response, error)
	ReadSecurityGroups(context.Context, *osc.ReadSecurityGroupsOpts) ([]osc.SecurityGroup, *_nethttp.Response, error)

    CreateSecurityGroup(context.Context, *osc.CreateSecurityGroupOpts) (osc.CreateSecurityGroupResponse, *_nethttp.Response, error)
    DeleteSecurityGroup(context.Context, *osc.DeleteSecurityGroupOpts) (osc.DeleteSecurityGroupResponse, *_nethttp.Response, error)

    CreateSecurityGroupRule(context.Context, *osc.CreateSecurityGroupRuleOpts) (osc.CreateSecurityGroupRuleResponse, *_nethttp.Response, error)
    DeleteSecurityGroupRule(context.Context, *osc.DeleteSecurityGroupRuleOpts) (osc.DeleteSecurityGroupRuleResponse, *_nethttp.Response, error)

    ReadSubnets(context.Context, *osc.ReadSubnetsOpts) ([]osc.Subnet, *_nethttp.Response, error)

    CreateTags(context.Context, *osc.CreateTagsOpts) (osc.CreateTagsResponse, *_nethttp.Response, error)

    ReadRouteTables(context.Context, *osc.ReadRouteTablesOpts) ([]osc.RouteTable, *_nethttp.Response, error)
    CreateRoute(context.Context, *osc.CreateRouteTableOpts) (osc.CreateRouteTableResponse, *_nethttp.Response, error)
    DeleteRoute(context.Context, *osc.DeleteRouteTableOpts) (osc.DeleteRouteTableResponse, *_nethttp.Response, error)

    UpdateVm(context.Context, *osc.UpdateVmOpts) (osc.UpdateVmResponse, *_nethttp.Response, error)

    ReadNets(context.Context, *osc.ReadNetsOpts) (osc.ReadNetsResponse, *_nethttp.Response, error)
}

// LBU is a simple pass-through of OSC' LBU client interface, which allows for testing
type LBU interface {
    CreateLoadBalancer(*osc.CreateLoadBalancerOpts) (osc.CreateLoadBalancerResponse, *_nethttp.Response, error)
    DeleteLoadBalancer(*osc.DeleteLoadBalancerOpts) (osc.DeleteLoadBalancerResponse, *_nethttp.Response, error)
    ReadLoadBalancers(*osc.ReadLoadBalancersOpts) (osc.ReadLoadBalancersResponse, *_nethttp.Response, error)
    UpdateLoadBalancer(*osc.UpdateLoadBalancerOpts) (osc.UpdateLoadBalancerResponse, *_nethttp.Response, error)


    ReadLoadBalancerTags(*osc.ReadLoadBalancerTagsOpts) (osc.ReadLoadBalancerTagsResponse, *_nethttp.Response, error)
    CreateLoadBalancerTags(*osc.CreateLoadBalancerTagsOpts) (osc.CreateLoadBalancerTagsResponse, *_nethttp.Response, error)

    RegisterVmsInLoadBalancer(*osc.RegisterVmsInLoadBalancerOpts) (osc.RegisterVmsInLoadBalancerResponse, *_nethttp.Response, error)
    DeregisterVmsInLoadBalancer(*osc.DeregisterVmsInLoadBalancerOpts) (osc.DeregisterVmsInLoadBalancerResponse, *_nethttp.Response, error)

    CreateLoadBalancerPolicy(*osc.CreateLoadBalancerPolicyOpts) (osc.CreateLoadBalancerPolicyResponse, *_nethttp.Response, error)

    CreateLoadBalancerListeners(*osc.CreateLoadBalancerListenersOpts) (osc.CreateLoadBalancerListenersResponse, *_nethttp.Response, error)
    DeleteLoadBalancerListeners(*osc.DeleteLoadBalancerListenersOpts) (osc.DeleteLoadBalancerListenersResponse, *_nethttp.Response, error)

    ReadVmsHealth(*osc.ReadVmsHealthOpts) (osc.ReadVmsHealthResponse, *_nethttp.Response, error)

}

// EC2Metadata is an abstraction over the AWS metadata service.
type EC2Metadata interface {
	Available() bool
	GetInstanceIdentityDocument() (ec2metadata.EC2InstanceIdentityDocument, error)
	// Query the EC2 metadata service (used to discover instance-id etc)
	GetMetadata(path string) (string, error)
}
