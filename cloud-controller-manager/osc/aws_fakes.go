// +build !providerless

/*
Copyright 2017 The Kubernetes Authors.

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
	"strings"

	"github.com/outscale/osc-sdk-go/osc"

	"github.com/aws/aws-sdk-go/aws/ec2metadata"

	"k8s.io/klog"

	context "context"
	_nethttp "net/http"
)

// FakeOSCServices is an fake AWS session used for testing
type FakeOSCServices struct {
	region                      string
	instances                   []osc.Vm
	selfInstance                osc.Vm
	networkInterfacesMacs       []string
	networkInterfacesPrivateIPs [][]string
	networkInterfacesVpcIDs     []string

	fcu      FakeFCU
	lbu      LBU
	metadata *FakeMetadata
}

// NewFakeOSCServices creates a new FakeOSCServices
func NewFakeOSCServices(clusterID string) *FakeOSCServices {
	s := &FakeOSCServices{}
	s.region = "us-east-1"
	s.fcu = &FakeFCUImpl{osc: s}
	s.lbu = &FakeLBU{osc: s}
	s.metadata = &FakeMetadata{osc: s}

	s.networkInterfacesMacs = []string{"aa:bb:cc:dd:ee:00", "aa:bb:cc:dd:ee:01"}
	s.networkInterfacesVpcIDs = []string{"vpc-mac0", "vpc-mac1"}

	selfInstance := osc.Vm{}
	selfInstance.VmId = "i-self"
	selfInstance.Placement = osc.Placement{
		SubregionName: "us-east-1a",
	}
	selfInstance.PrivateDnsName = "ip-172-20-0-100.ec2.internal"
	selfInstance.PrivateIp = "192.168.0.1"
	selfInstance.PublicIp = "1.2.3.4"
	s.selfInstance = selfInstance
	s.instances = []osc.Vm{selfInstance}

	var tag osc.ResourceTag
	tag.Key = TagNameKubernetesClusterLegacy
	tag.Value = clusterID
	selfInstance.Tags = []osc.ResourceTag{tag}

	return s
}

// WithAz sets the ec2 placement availability zone
func (s *FakeOSCServices) WithAz(az string) *FakeOSCServices {
	if (s.selfInstance.Placement == osc.Placement{}) {
		s.selfInstance.Placement = osc.Placement{}
	}
	s.selfInstance.Placement.SubregionName = az
	return s
}

// Compute returns a fake EC2 client
func (s *FakeOSCServices) Compute(region string) (FCU, error) {
	return s.fcu, nil
}

// LoadBalancing returns a fake LBU client
func (s *FakeOSCServices) LoadBalancing(region string) (LBU, error) {
	return s.lbu, nil
}

// Metadata returns a fake EC2Metadata client
func (s *FakeOSCServices) Metadata() (EC2Metadata, error) {
	return s.metadata, nil
}

// FakeFCU is a fake EC2 client used for testing
type FakeFCU interface {
	FCU
	CreateSubnet(osc.Subnet) (osc.CreateSubnetResponse, error)
	RemoveSubnets()
	CreateRouteTable(osc.RouteTable) (osc.CreateRouteTableResponse, error)
	RemoveRouteTables()
}

// FakeFCUImpl is an implementation of the FakeFCU interface used for testing
type FakeFCUImpl struct {
	osc                      *FakeOSCServices
	Subnets                  []osc.Subnet
	ReadSubnetsOpts          *osc.ReadSubnetsOpts
	RouteTables              []osc.RouteTable
	ReadRouteTablesOpts      *osc.ReadRouteTablesOpts
}

// ReadVms returns fake instance descriptions
func (fcui *FakeFCUImpl) ReadVms(request *osc.ReadVmsOpts) ([]osc.Vm, *_nethttp.Response, error) {
	matches := []osc.Vm{}

    requestVm := request.ReadVmsRequest.Value().(osc.ReadVmsRequest).Filters
	for _, instance := range fcui.osc.instances {
		if requestVm.VmIds != nil {
			if instance.VmId == "" {
				klog.Warning("Instance with no instance id: ", instance)
				continue
			}

			found := false
			for _, instanceID := range requestVm.VmIds {
				if instanceID == instance.VmId {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
// 		if request.Filters != nil {
// 			allMatch := true
// 			for _, filter := range request.Filters {
// 				if !instanceMatchesFilter(instance, filter) {
// 					allMatch = false
// 					break
// 				}
// 			}
// 			if !allMatch {
// 				continue
// 			}
// 		}
		matches = append(matches, instance)
	}

	return matches, nil, nil
}

// DescribeSecurityGroups is not implemented but is required for interface
// conformance
func (fcui *FakeFCUImpl) ReadSecurityGroups(request *osc.ReadSecurityGroupsOpts) ([]osc.SecurityGroup, *_nethttp.Response, error) {
	panic("Not implemented")
}

// CreateSecurityGroup is not implemented but is required for interface
// conformance
func (fcui *FakeFCUImpl) CreateSecurityGroup(request *osc.CreateSecurityGroupOpts) (osc.CreateSecurityGroupResponse, *_nethttp.Response, error) {
	panic("Not implemented")
}

// DeleteSecurityGroup is not implemented but is required for interface
// conformance
func (fcui *FakeFCUImpl) DeleteSecurityGroup(request *osc.DeleteSecurityGroupOpts) (osc.DeleteSecurityGroupResponse, *_nethttp.Response, error) {
	panic("Not implemented")
}

// AuthorizeSecurityGroupIngress is not implemented but is required for
// interface conformance
func (fcui *FakeFCUImpl) CreateSecurityGroupRule(request *osc.CreateSecurityGroupRuleOpts) (osc.CreateSecurityGroupRuleResponse, *_nethttp.Response, error) {
	panic("Not implemented")
}

// RevokeSecurityGroupIngress is not implemented but is required for interface
// conformance
func (fcui *FakeFCUImpl) DeleteSecurityGroupRule(request *osc.DeleteSecurityGroupRuleOpts) (osc.DeleteSecurityGroupRuleResponse, *_nethttp.Response, error) {
	panic("Not implemented")
}

// CreateSubnet creates fake subnets
func (fcui *FakeFCUImpl) CreateSubnet(request osc.Subnet) (osc.CreateSubnetResponse, error) {
	fcui.Subnets = append(fcui.Subnets, request)
	response := osc.CreateSubnetResponse{
		Subnet: request,
	}
	return response, nil
}

// DescribeSubnets returns fake subnet descriptions
func (fcui *FakeFCUImpl) ReadSubnets(request *osc.ReadSubnetsOpts) ([]osc.Subnet, *_nethttp.Response, error) {
	fcui.ReadSubnetsOpts = request
	return fcui.Subnets, nil, nil
}

// RemoveSubnets clears subnets on client
func (fcui *FakeFCUImpl) RemoveSubnets() {
	fcui.Subnets = fcui.Subnets[:0]
}

// CreateTags is not implemented but is required for interface conformance
func (fcui *FakeFCUImpl) CreateTags(request *osc.CreateTagsOpts) (osc.CreateTagsResponse, *_nethttp.Response, error) {
	panic("Not implemented")
}

// DescribeRouteTables returns fake route table descriptions
func (fcui *FakeFCUImpl) ReadRouteTables(request *osc.ReadRouteTablesOpts) ([]osc.RouteTable, *_nethttp.Response, error) {
	fcui.ReadRouteTablesOpts = request
	return fcui.RouteTables, nil, nil
}

// CreateRouteTable creates fake route tables
func (fcui *FakeFCUImpl) CreateRouteTable(request osc.RouteTable) (osc.CreateRouteTableResponse, error) {
	fcui.RouteTables = append(fcui.RouteTables, request)
	response := osc.CreateRouteTableResponse{
		RouteTable: request,
	}
	return response, nil
}

// RemoveRouteTables clears route tables on client
func (fcui *FakeFCUImpl) RemoveRouteTables() {
	fcui.RouteTables = fcui.RouteTables[:0]
}

// CreateRoute is not implemented but is required for interface conformance
func (fcui *FakeFCUImpl) CreateRoute(request *osc.CreateRouteOpts) (osc.CreateRouteResponse, *_nethttp.Response, error) {
	panic("Not implemented")
}

// DeleteRoute is not implemented but is required for interface conformance
func (fcui *FakeFCUImpl) DeleteRoute(request *osc.DeleteRouteOpts) (osc.DeleteRouteResponse, *_nethttp.Response, error) {
	panic("Not implemented")
}

// ModifyInstanceAttribute is not implemented but is required for interface
// conformance
func (fcui *FakeFCUImpl) UpdateVm(request *osc.UpdateVmOpts) (osc.UpdateVmResponse, *_nethttp.Response, error) {
	panic("Not implemented")
}

// DescribeVpcs returns fake VPC descriptions
func (fcui *FakeFCUImpl) ReadNets(request *osc.ReadNetsOpts) (osc.ReadNetsResponse, *_nethttp.Response, error) {
	return osc.ReadNetsResponse{Nets: []osc.Net{{IpRange: "172.20.0.0/16"}}}, nil, nil
}

// FakeMetadata is a fake EC2 metadata service client used for testing
type FakeMetadata struct {
	osc *FakeOSCServices
}

// GetInstanceIdentityDocument mocks base method
func (m *FakeMetadata) GetInstanceIdentityDocument() (ec2metadata.EC2InstanceIdentityDocument, error) {
	return ec2metadata.EC2InstanceIdentityDocument{}, nil
}

// Available mocks base method
func (m *FakeMetadata) Available() bool {

	return true
}

// GetMetadata returns fake EC2 metadata for testing
func (m *FakeMetadata) GetMetadata(key string) (string, error) {
	networkInterfacesPrefix := "network/interfaces/macs/"
	i := m.osc.selfInstance
	if key == "placement/availability-zone" {
		az := ""
		if (i.Placement != osc.Placement{}) {
			az = i.Placement.SubregionName
		}
		return az, nil
	} else if key == "instance-id" {
		return i.VmId, nil
	} else if key == "local-hostname" {
		return i.PrivateDnsName, nil
	} else if key == "public-hostname" {
		return i.PublicDnsName, nil
	} else if key == "local-ipv4" {
		return i.PrivateIp, nil
	} else if key == "public-ipv4" {
		return i.PublicIp, nil
	} else if strings.HasPrefix(key, networkInterfacesPrefix) {
		if key == networkInterfacesPrefix {
			return strings.Join(m.osc.networkInterfacesMacs, "/\n") + "/\n", nil
		}

		keySplit := strings.Split(key, "/")
		macParam := keySplit[3]
		if len(keySplit) == 5 && keySplit[4] == "vpc-id" {
			for i, macElem := range m.osc.networkInterfacesMacs {
				if macParam == macElem {
					return m.osc.networkInterfacesVpcIDs[i], nil
				}
			}
		}
		if len(keySplit) == 5 && keySplit[4] == "local-ipv4s" {
			for i, macElem := range m.osc.networkInterfacesMacs {
				if macParam == macElem {
					return strings.Join(m.osc.networkInterfacesPrivateIPs[i], "/\n"), nil
				}
			}
		}

		return "", nil
	}

	return "", nil
}

// FakeLBU is a fake LBU client used for testing
type FakeLBU struct {
	osc *FakeOSCServices
}

// CreateLoadBalancer is not implemented but is required for interface
// conformance
func (lbu *FakeLBU) CreateLoadBalancer(input *osc.CreateLoadBalancerOpts) (osc.CreateLoadBalancerResponse, *_nethttp.Response, error) {
	panic("Not implemented")
}

// DeleteLoadBalancer is not implemented but is required for interface
// conformance
func (lbu *FakeLBU) DeleteLoadBalancer(input *osc.DeleteLoadBalancerOpts) (osc.DeleteLoadBalancerResponse, *_nethttp.Response, error) {
	panic("Not implemented")
}

// DescribeLoadBalancers is not implemented but is required for interface
// conformance
func (lbu *FakeLBU) ReadLoadBalancers(input *osc.ReadLoadBalancersOpts) (osc.ReadLoadBalancersResponse, *_nethttp.Response, error) {
	panic("Not implemented")
}

// AddTags is not implemented but is required for interface conformance
func (lbu *FakeLBU) CreateLoadBalancerTags(input *osc.CreateLoadBalancerTagsOpts) (osc.CreateLoadBalancerTagsResponse, *_nethttp.Response, error) {
	panic("Not implemented")
}

// RegisterInstancesWithLoadBalancer is not implemented but is required for
// interface conformance
func (lbu *FakeLBU) RegisterVmsInLoadBalancer(input *osc.RegisterVmsInLoadBalancerOpts) (osc.RegisterVmsInLoadBalancerResponse, *_nethttp.Response, error) {
	panic("Not implemented")
}

// DeregisterInstancesFromLoadBalancer is not implemented but is required for
// interface conformance
func (lbu *FakeLBU) DeregisterVmsInLoadBalancer(input *osc.DeregisterVmsInLoadBalancerOpts) (osc.DeregisterVmsInLoadBalancerResponse, *_nethttp.Response, error) {
	panic("Not implemented")
}

// DetachLoadBalancerFromSubnets is not implemented but is required for
// interface conformance
func (lbu *FakeLBU) DetachLoadBalancerFromSubnets(input *osc.DeregisterVmsInLoadBalancerOpts) (osc.DeregisterVmsInLoadBalancerResponse, *_nethttp.Response, error) {
	panic("Not implemented")
}

// AttachLoadBalancerToSubnets is not implemented but is required for interface
// conformance
func (lbu *FakeLBU) AttachLoadBalancerToSubnets(input *osc.RegisterVmsInLoadBalancerOpts) (osc.RegisterVmsInLoadBalancerOpts, *_nethttp.Response, error) {
	panic("Not implemented")
}

// CreateLoadBalancerListeners is not implemented but is required for interface
// conformance
func (lbu *FakeLBU) CreateLoadBalancerListeners(input *osc.CreateLoadBalancerListenersOpts) (osc.CreateLoadBalancerListenersResponse, *_nethttp.Response, error) {
	panic("Not implemented")
}

// DeleteLoadBalancerListeners is not implemented but is required for interface
// conformance
func (lbu *FakeLBU) DeleteLoadBalancerListeners(input *osc.DeleteLoadBalancerListenersOpts) (osc.DeleteLoadBalancerListenersResponse, *_nethttp.Response, error) {
	panic("Not implemented")
}

// ApplySecurityGroupsToLoadBalancer is not implemented but is required for
// interface conformance
func (lbu *FakeLBU) ApplySecurityGroupsToLoadBalancer(input *osc.CreateLoadBalancerListenersOpts) (osc.CreateLoadBalancerListenersResponse, *_nethttp.Response, error) {
	panic("Not implemented")
}

// ConfigureHealthCheck is not implemented but is required for interface
// conformance
func (lbu *FakeLBU) ReadVmsHealth(input *osc.ReadVmsHealthOpts) (osc.ReadVmsHealthResponse, *_nethttp.Response, error) {
	panic("Not implemented")
}

// CreateLoadBalancerPolicy is not implemented but is required for interface
// conformance
func (lbu *FakeLBU) CreateLoadBalancerPolicy(input *osc.CreateLoadBalancerPolicyOpts) (osc.CreateLoadBalancerPolicyResponse, *_nethttp.Response, error) {
	panic("Not implemented")
}

// SetLoadBalancerPoliciesForBackendServer is not implemented but is required
// for interface conformance
func (lbu *FakeLBU) SetLoadBalancerPoliciesForBackendServer(input *osc.CreateLoadBalancerPolicyOpts) (osc.CreateLoadBalancerPolicyResponse, *_nethttp.Response, error) {
	panic("Not implemented")
}

// SetLoadBalancerPoliciesOfListener is not implemented but is required for
// interface conformance
func (lbu *FakeLBU) SetLoadBalancerPoliciesOfListener(input *osc.CreateLoadBalancerPolicyOpts) (osc.CreateLoadBalancerPolicyResponse, *_nethttp.Response, error) {
	panic("Not implemented")
}

// DescribeLoadBalancerPolicies is not implemented but is required for
// interface conformance
func (lbu *FakeLBU) DescribeLoadBalancerPolicies(input *osc.ReadLoadBalancersOpts) (osc.ReadLoadBalancersResponse, *_nethttp.Response, error) {
	panic("Not implemented")
}

// DescribeLoadBalancerAttributes is not implemented but is required for
// interface conformance
func (lbu *FakeLBU) DescribeLoadBalancerAttributes(input *osc.ReadLoadBalancersOpts) (osc.ReadLoadBalancersResponse, *_nethttp.Response, error) {
	panic("Not implemented")
}

// UpdateLoadBalancer is not implemented but is required for
// interface conformance
func (lbu *FakeLBU) UpdateLoadBalancer(input *osc.UpdateLoadBalancerOpts) (osc.UpdateLoadBalancerResponse, *_nethttp.Response, error) {
	panic("Not implemented")
}

// ReadLoadBalancerTags is not implemented but is required for
// interface conformance
func (lbu *FakeLBU) ReadLoadBalancerTags(*osc.ReadLoadBalancerTagsOpts) (osc.ReadLoadBalancerTagsResponse, *_nethttp.Response, error){
    panic("Not implemented")
}

// expectDescribeLoadBalancers is not implemented but is required for interface
// conformance
func (lbu *FakeLBU) expectDescribeLoadBalancers(loadBalancerName string) {
	panic("Not implemented")
}

// func instanceMatchesFilter(instance osc.Vm, filter *osc.FiltersVm) bool {
//     filter.VmIds[0]
//     instance.PrivateDnsName
//
//
//
// 	name := *filter.Name
// 	if name == "private-dns-name" {
// 		if instance.PrivateDnsName == "" {
// 			return false
// 		}
// 		return contains(filter.Values, *instance.PrivateDnsName)
// 	}
//
// 	if name == "instance-state-name" {
// 		return contains(filter.Values, *instance.State.Name)
// 	}
//
// 	if name == "tag-key" {
// 		for _, instanceTag := range instance.Tags {
// 			if contains(filter.Values, instanceTag.Key) {
// 				return true
// 			}
// 		}
// 		return false
// 	}
//
// 	if strings.HasPrefix(name, "tag:") {
// 		tagName := name[4:]
// 		for _, instanceTag := range instance.Tags {
// 			if instanceTag.Key == tagName && contains(filter.Values, instanceTag.Value) {
// 				return true
// 			}
// 		}
// 		return false
// 	}
//
// 	panic("Unknown filter name: " + name)
// }

func contains(haystack []*string, needle string) bool {
	for _, s := range haystack {
		// (deliberately panic if s == nil)
		if needle == *s {
			return true
		}
	}
	return false
}
