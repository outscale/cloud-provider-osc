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
	s.fcu = &FakeOSCImpl{osc: s}
	s.lbu = &FaklBUU{osc: s}
	s.metadata = &FakeMetadata{aws: s}

	s.networkInterfacesMacs = []string{"aa:bb:cc:dd:ee:00", "aa:bb:cc:dd:ee:01"}
	s.networkInterfacesVpcIDs = []string{"vpc-mac0", "vpc-mac1"}

	selfInstance := &osc.Vm{}
	selfInstance.InstanceId = "i-self"
	selfInstance.Placement = &osc.Placement{
		SubregionName: "us-east-1a",
	}
	selfInstance.PrivateDnsName = "ip-172-20-0-100.ec2.internal"
	selfInstance.PrivateIpAddress = "192.168.0.1"
	selfInstance.PublicIpAddress = "1.2.3.4"
	s.selfInstance = selfInstance
	s.instances = []osc.Vm{selfInstance}

	var tag osc.Tag
	tag.Key = TagNameKubernetesClusterLegacy
	tag.Value = clusterID
	selfInstance.Tags = []osc.Tag{&tag}

	return s
}

// WithAz sets the ec2 placement availability zone
func (s *FakeOSCServices) WithAz(az string) *FakeOSCServices {
	if s.selfInstance.Placement == nil {
		s.selfInstance.Placement = &osc.Placement{}
	}
	s.selfInstance.Placement.AvailabilityZone = az
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

// FakeEC2 is a fake EC2 client used for testing
type FakeFCU interface {
	FCU
	CreateSubnet(osc.Subnet) (osc.SubregionName, error)
	RemoveSubnets()
	CreateRouteTable(osc.RouteTable) (osc.CreateRouteTableResponse, error)
	RemoveRouteTables()
}

// FakeFBUImpl is an implementation of the FakeEC2 interface used for testing
type FakeFCUImpl struct {
	osc                      *FakeOSCServices
	Subnets                  []osc.Subnet
	ReadSubnetsOpts          *osc.ReadSubnetsOpts
	RouteTables              []osc.RouteTable
	ReadRouteTablesOpts      *osc.ReadRouteTablesOpts
}

// DescribeInstances returns fake instance descriptions
func (fcui *FakeFCUImpl) DescribeInstances(request *osc.ReadVmsOpts) ([]osc.Vm, error) {
	matches := []osc.Vm{}
	for _, instance := range fcui.osc.instances {
		if request.InstanceIds != nil {
			if instance.InstanceId == nil {
				klog.Warning("Instance with no instance id: ", instance)
				continue
			}

			found := false
			for _, instanceID := range request.InstanceIds {
				if *instanceID == *instance.InstanceId {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		if request.Filters != nil {
			allMatch := true
			for _, filter := range request.Filters {
				if !instanceMatchesFilter(instance, filter) {
					allMatch = false
					break
				}
			}
			if !allMatch {
				continue
			}
		}
		matches = append(matches, instance)
	}

	return matches, nil
}

// DescribeSecurityGroups is not implemented but is required for interface
// conformance
func (fcui *FakeFCUImpl) DescribeSecurityGroups(request *osc.ReadSecurityGroupsOpts) ([]osc.SecurityGroup, error) {
	panic("Not implemented")
}

// CreateSecurityGroup is not implemented but is required for interface
// conformance
func (fcui *FakeFCUImpl) CreateSecurityGroup(*osc.CreateSecurityGroupOpts) (osc.CreateSecurityGroupResponse, error) {
	panic("Not implemented")
}

// DeleteSecurityGroup is not implemented but is required for interface
// conformance
func (fcui *FakeFCUImpl) DeleteSecurityGroup(*osc.DeleteSecurityGroupOpts) (osc.DeleteSecurityGroupResponse, error) {
	panic("Not implemented")
}

// AuthorizeSecurityGroupIngress is not implemented but is required for
// interface conformance
func (fcui *FakeFCUImpl) AuthorizeSecurityGroupIngress(*osc.CreateSecurityGroupRuleOpts) (osc.SecurityGroupRuleResponse, error) {
	panic("Not implemented")
}

// RevokeSecurityGroupIngress is not implemented but is required for interface
// conformance
func (fcui *FakeFCUImpl) RevokeSecurityGroupIngress(osc.DeleteSecurityGroupRuleOpts) (osc.DeleteSecurityGroupRuleResponse, error) {
	panic("Not implemented")
}

// CreateSubnet creates fake subnets
func (fcui *FakeFCUImpl) CreateSubnet(request *osc.Subnet) (osc.CreateSubnetResponse, error) {
	fcui.Subnets = append(fcui.Subnets, request)
	response := &osc.CreateSubnetResponse{
		Subnet: request,
	}
	return response, nil
}

// DescribeSubnets returns fake subnet descriptions
func (fcui *FakeFCUImpl) DescribeSubnets(request *osc.ReadSubnetsOpts) ([]osc.Subnet, error) {
	fcui.ReadSubnetsOpts = request
	return fcui.Subnets, nil
}

// RemoveSubnets clears subnets on client
func (fcui *FakeFCUImpl) RemoveSubnets() {
	fcui.Subnets = fcui.Subnets[:0]
}

// CreateTags is not implemented but is required for interface conformance
func (fcui *FakeFCUImpl) CreateTags(*osc.CreateTagsopts) (osc.CreateTagsResponse, error) {
	panic("Not implemented")
}

// DescribeRouteTables returns fake route table descriptions
func (fcui *FakeFCUImpl) DescribeRouteTables(request *osc.ReadRouteTablesOpts) ([]osc.RouteTable, error) {
	fcui.ReadRouteTablesOpts = request
	return fcui.RouteTables, nil
}

// CreateRouteTable creates fake route tables
func (fcui *FakeFCUImpl) CreateRouteTable(request *osc.RouteTable) (osc.CreateRouteTableResponse, error) {
	fcui.RouteTables = append(fcui.RouteTables, request)
	response := &osc.CreateRouteTableResponse{
		RouteTable: request,
	}
	return response, nil
}

// RemoveRouteTables clears route tables on client
func (fcui *FakeFCUImpl) RemoveRouteTables() {
	fcui.RouteTables = fcui.RouteTables[:0]
}

// CreateRoute is not implemented but is required for interface conformance
func (fcui *FakeFCUImpl) CreateRoute(request *osc.CreateRouteOpts) (osc.CreateRouteResponse, error) {
	panic("Not implemented")
}

// DeleteRoute is not implemented but is required for interface conformance
func (fcui *FakeFCUImpl) DeleteRoute(request *osc.DeleteRouteOpts) (osc.DeleteRouteResponse, error) {
	panic("Not implemented")
}

// ModifyInstanceAttribute is not implemented but is required for interface
// conformance
func (fcui *FakeFCUImpl) ModifyInstanceAttribute(request *osc.ModifyInstanceAttributeInput) (osc.ModifyInstanceAttributeOutput, error) {
	panic("Not implemented")
}

// DescribeVpcs returns fake VPC descriptions
func (fcui *FakeFCUImpl) DescribeVpcs(request *osc.ReadNetsOpts) (osc.ReadNetsResponse, error) {
	return &osc.ReadNetsResponse{NetIds: []osc.Vpc{{CidrBlock: "172.20.0.0/16"}}}, nil
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
		if i.Placement != nil {
			az = i.Placement.AvailabilityZone
		}
		return az, nil
	} else if key == "instance-id" {
		return i.InstanceId, nil
	} else if key == "local-hostname" {
		return aws.StringValue(i.PrivateDnsName, nil
	} else if key == "public-hostname" {
		return aws.StringValue(i.PublicDnsName, nil
	} else if key == "local-ipv4" {
		return i.PrivateIpAddress, nil
	} else if key == "public-ipv4" {
		return i.PublicIpAddress, nil
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
func (lbu *FakeLBU) CreateLoadBalancer(*lbu.CreateLoadBalancerInput) (*lbu.CreateLoadBalancerOutput, error) {
	panic("Not implemented")
}

// DeleteLoadBalancer is not implemented but is required for interface
// conformance
func (lbu *FakeLBU) DeleteLoadBalancer(input *lbu.DeleteLoadBalancerInput) (*lbu.DeleteLoadBalancerOutput, error) {
	panic("Not implemented")
}

// DescribeLoadBalancers is not implemented but is required for interface
// conformance
func (lbu *FakeLBU) DescribeLoadBalancers(input *lbu.DescribeLoadBalancersInput) (*lbu.DescribeLoadBalancersOutput, error) {
	panic("Not implemented")
}

// AddTags is not implemented but is required for interface conformance
func (lbu *FakeLBU) AddTags(input *lbu.AddTagsInput) (*lbu.AddTagsOutput, error) {
	panic("Not implemented")
}

// RegisterInstancesWithLoadBalancer is not implemented but is required for
// interface conformance
func (lbu *FakeLBU) RegisterInstancesWithLoadBalancer(*lbu.RegisterInstancesWithLoadBalancerInput) (*lbu.RegisterInstancesWithLoadBalancerOutput, error) {
	panic("Not implemented")
}

// DeregisterInstancesFromLoadBalancer is not implemented but is required for
// interface conformance
func (lbu *FakeLBU) DeregisterInstancesFromLoadBalancer(*lbu.DeregisterInstancesFromLoadBalancerInput) (*lbu.DeregisterInstancesFromLoadBalancerOutput, error) {
	panic("Not implemented")
}

// DetachLoadBalancerFromSubnets is not implemented but is required for
// interface conformance
func (lbu *FakeLBU) DetachLoadBalancerFromSubnets(*lbu.DetachLoadBalancerFromSubnetsInput) (*lbu.DetachLoadBalancerFromSubnetsOutput, error) {
	panic("Not implemented")
}

// AttachLoadBalancerToSubnets is not implemented but is required for interface
// conformance
func (lbu *FakeLBU) AttachLoadBalancerToSubnets(*lbu.AttachLoadBalancerToSubnetsInput) (*lbu.AttachLoadBalancerToSubnetsOutput, error) {
	panic("Not implemented")
}

// CreateLoadBalancerListeners is not implemented but is required for interface
// conformance
func (lbu *FakeLBU) CreateLoadBalancerListeners(*lbu.CreateLoadBalancerListenersInput) (*lbu.CreateLoadBalancerListenersOutput, error) {
	panic("Not implemented")
}

// DeleteLoadBalancerListeners is not implemented but is required for interface
// conformance
func (lbu *FakeLBU) DeleteLoadBalancerListeners(*lbu.DeleteLoadBalancerListenersInput) (*lbu.DeleteLoadBalancerListenersOutput, error) {
	panic("Not implemented")
}

// ApplySecurityGroupsToLoadBalancer is not implemented but is required for
// interface conformance
func (lbu *FakeLBU) ApplySecurityGroupsToLoadBalancer(*lbu.ApplySecurityGroupsToLoadBalancerInput) (*lbu.ApplySecurityGroupsToLoadBalancerOutput, error) {
	panic("Not implemented")
}

// ConfigureHealthCheck is not implemented but is required for interface
// conformance
func (lbu *FakeLBU) ConfigureHealthCheck(*lbu.ConfigureHealthCheckInput) (*lbu.ConfigureHealthCheckOutput, error) {
	panic("Not implemented")
}

// CreateLoadBalancerPolicy is not implemented but is required for interface
// conformance
func (lbu *FakeLBU) CreateLoadBalancerPolicy(*lbu.CreateLoadBalancerPolicyInput) (*lbu.CreateLoadBalancerPolicyOutput, error) {
	panic("Not implemented")
}

// SetLoadBalancerPoliciesForBackendServer is not implemented but is required
// for interface conformance
func (lbu *FakeLBU) SetLoadBalancerPoliciesForBackendServer(*lbu.SetLoadBalancerPoliciesForBackendServerInput) (*lbu.SetLoadBalancerPoliciesForBackendServerOutput, error) {
	panic("Not implemented")
}

// SetLoadBalancerPoliciesOfListener is not implemented but is required for
// interface conformance
func (lbu *FakeLBU) SetLoadBalancerPoliciesOfListener(input *lbu.SetLoadBalancerPoliciesOfListenerInput) (*lbu.SetLoadBalancerPoliciesOfListenerOutput, error) {
	panic("Not implemented")
}

// DescribeLoadBalancerPolicies is not implemented but is required for
// interface conformance
func (lbu *FakeLBU) DescribeLoadBalancerPolicies(input *lbu.DescribeLoadBalancerPoliciesInput) (*lbu.DescribeLoadBalancerPoliciesOutput, error) {
	panic("Not implemented")
}

// DescribeLoadBalancerAttributes is not implemented but is required for
// interface conformance
func (lbu *FakeLBU) DescribeLoadBalancerAttributes(*lbu.DescribeLoadBalancerAttributesInput) (*lbu.DescribeLoadBalancerAttributesOutput, error) {
	panic("Not implemented")
}

// ModifyLoadBalancerAttributes is not implemented but is required for
// interface conformance
func (lbu *FakeLBU) ModifyLoadBalancerAttributes(*lbu.ModifyLoadBalancerAttributesInput) (*lbu.ModifyLoadBalancerAttributesOutput, error) {
	panic("Not implemented")
}

// expectDescribeLoadBalancers is not implemented but is required for interface
// conformance
func (lbu *FakeLBU) expectDescribeLoadBalancers(loadBalancerName string) {
	panic("Not implemented")
}

func instanceMatchesFilter(instance osc.Vm, filter *ec2.Filter) bool {
	name := *filter.Name
	if name == "private-dns-name" {
		if instance.PrivateDnsName == nil {
			return false
		}
		return contains(filter.Values, *instance.PrivateDnsName)
	}

	if name == "instance-state-name" {
		return contains(filter.Values, *instance.State.Name)
	}

	if name == "tag-key" {
		for _, instanceTag := range instance.Tags {
			if contains(filter.Values, aws.StringValue(instanceTag.Key)) {
				return true
			}
		}
		return false
	}

	if strings.HasPrefix(name, "tag:") {
		tagName := name[4:]
		for _, instanceTag := range instance.Tags {
			if aws.StringValue(instanceTag.Key) == tagName && contains(filter.Values, aws.StringValue(instanceTag.Value)) {
				return true
			}
		}
		return false
	}

	panic("Unknown filter name: " + name)
}

func contains(haystack []*string, needle string) bool {
	for _, s := range haystack {
		// (deliberately panic if s == nil)
		if needle == *s {
			return true
		}
	}
	return false
}
