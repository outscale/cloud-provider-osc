// +build !providerless

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
	"context"
	"fmt"
	"io"
	"reflect"
	"strings"
	"testing"

    _nethttp "net/http"
    "github.com/antihax/optional"

    "github.com/outscale/osc-sdk-go/osc"

	//"github.com/aws/aws-sdk-go/aws"
	//"github.com/aws/aws-sdk-go/service/ec2"
	//"github.com/aws/aws-sdk-go/service/elb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
)

const TestClusterID = "clusterid.test"
const TestClusterName = "testCluster"

type MockedFakeFCU struct {
	*FakeFCUImpl
	mock.Mock
}

func (m *MockedFakeFCU) expectReadSecurityGroups(clusterID, securityGroupName string) {
	tags := []osc.ResourceTag{
		{
			Key:   fmt.Sprintf("%s%s", TagNameKubernetesClusterPrefix, clusterID),
			Value: ResourceLifecycleOwned,
		},
		{
			Key:   TagNameKubernetesClusterLegacy,
			Value: clusterID,
		},
	}

	m.On("ReadSecurityGroups", &osc.ReadSecurityGroupsOpts{
		ReadSecurityGroupsRequest: optional.NewInterface(
			osc.ReadSecurityGroupsRequest{
				Filters: osc.FiltersSecurityGroup{
					SecurityGroupNames: []string{securityGroupName},
					NetIds: []string{"vpc-123456"},
				},
			}),
	}).Return([]osc.SecurityGroup{{Tags: tags, SecurityGroupId: "sg-12345"}})
}

func (m *MockedFakeFCU) ReadSecurityGroups(request *osc.ReadSecurityGroupsOpts) ([]osc.SecurityGroup, *_nethttp.Response, error) {
	args := m.Called(request)
	return args.Get(0).([]osc.SecurityGroup), nil, nil
}

type MockedFakeLBU struct {
	*FakeLBU
	mock.Mock
}

func (m *MockedFakeLBU) ReadLoadBalancers(input *osc.ReadLoadBalancersOpts) (osc.ReadLoadBalancersResponse, *_nethttp.Response, error) {
	args := m.Called(input)
	return args.Get(0).(osc.ReadLoadBalancersResponse), nil, nil
}

func (m *MockedFakeLBU) expectReadLoadBalancers(loadBalancerName string) {
	m.On("ReadLoadBalancers", &osc.ReadLoadBalancersOpts{
		ReadLoadBalancersRequest: optional.NewInterface(
			osc.ReadLoadBalancersRequest{
				Filters: osc.FiltersLoadBalancer{
					LoadBalancerNames: []string{loadBalancerName},
				},
			}),
	}).Return(osc.ReadLoadBalancersResponse{LoadBalancers: []osc.LoadBalancer{{}}})
}

func (m *MockedFakeLBU) CreateTags(input *osc.CreateTagsOpts) (osc.CreateTagsResponse, *_nethttp.Response, error) {
	args := m.Called(input)
	return args.Get(0).(osc.CreateTagsResponse), nil, nil
}

// func (m *MockedFakeLBU) ConfigureHealthCheck(input *elb.ConfigureHealthCheckInput) (*elb.ConfigureHealthCheckOutput, error) {
// 	args := m.Called(input)
// 	if args.Get(0) == nil {
// 		return nil, args.Error(1)
// 	}
// 	return args.Get(0).(*elb.ConfigureHealthCheckOutput), args.Error(1)
// }
//
// func (m *MockedFakeLBU) expectConfigureHealthCheck(loadBalancerName *string, expectedHC *elb.HealthCheck, returnErr error) {
// 	expected := &elb.ConfigureHealthCheckInput{HealthCheck: expectedHC, LoadBalancerName: loadBalancerName}
// 	call := m.On("ConfigureHealthCheck", expected)
// 	if returnErr != nil {
// 		call.Return(nil, returnErr)
// 	} else {
// 		call.Return(&elb.ConfigureHealthCheckOutput{}, nil)
// 	}
// }

func TestReadOSCCloudConfig(t *testing.T) {
	tests := []struct {
		name string

		reader io.Reader
		osc    Services

		expectError bool
		zone        string
	}{
		{
			"No config reader",
			nil, nil,
			true, "",
		},
		{
			"Empty config, no metadata",
			strings.NewReader(""), nil,
			true, "",
		},
		{
			"No zone in config, no metadata",
			strings.NewReader("[global]\n"), nil,
			true, "",
		},
		{
			"Zone in config, no metadata",
			strings.NewReader("[global]\nzone = eu-west-1a"), nil,
			false, "eu-west-1a",
		},
		{
			"No zone in config, metadata does not have zone",
			strings.NewReader("[global]\n"), newMockedFakeOSCServices(TestClusterID).WithAz(""),
			true, "",
		},
		{
			"No zone in config, metadata has zone",
			strings.NewReader("[global]\n"), newMockedFakeOSCServices(TestClusterID),
			false, "eu-west-2a",
		},
		{
			"Zone in config should take precedence over metadata",
			strings.NewReader("[global]\nzone = eu-west-1a"), newMockedFakeOSCServices(TestClusterID),
			false, "eu-west-1a",
		},
	}

	for _, test := range tests {
		t.Logf("Running test case %s", test.name)
		var metadata EC2Metadata
		if test.osc != nil {
			metadata, _ = test.osc.Metadata()
		}
		cfg, err := readOSCCloudConfig(test.reader)
		if err == nil {
			err = updateConfigZone(cfg, metadata)
		}
		if test.expectError {
			if err == nil {
				t.Errorf("Should error for case %s (cfg=%v)", test.name, cfg)
			}
		} else {
			if err != nil {
				t.Errorf("Should succeed for case: %s", test.name)
			}
			if cfg.Global.Zone != test.zone {
				t.Errorf("Incorrect zone value (%s vs %s) for case: %s",
					cfg.Global.Zone, test.zone, test.name)
			}
		}
	}
}

type ServiceDescriptor struct {
	name                         string
	region                       string
	signingRegion, signingMethod string
	signingName                  string
}

func TestOverridesActiveConfig(t *testing.T) {
	tests := []struct {
		name string

		reader io.Reader
		osc    Services

		expectError        bool
		active             bool
		servicesOverridden []ServiceDescriptor
	}{
		{
			"No overrides",
			strings.NewReader(`
				[global]
				`),
			nil,
			false, false,
			[]ServiceDescriptor{},
		},
		{
			"Missing Service Name",
			strings.NewReader(`
                [global]

                [ServiceOverride "1"]
                 Region=sregion
                 URL=https://s3.foo.bar
                 SigningRegion=sregion
                 SigningMethod = sign
                `),
			nil,
			true, false,
			[]ServiceDescriptor{},
		},
		{
			"Missing Service Region",
			strings.NewReader(`
                [global]

                [ServiceOverride "1"]
                 Service=s3
                 URL=https://s3.foo.bar
                 SigningRegion=sregion
                 SigningMethod = sign
                 `),
			nil,
			true, false,
			[]ServiceDescriptor{},
		},
		{
			"Missing URL",
			strings.NewReader(`
                  [global]

                  [ServiceOverride "1"]
                   Service="s3"
                   Region=sregion
                   SigningRegion=sregion
                   SigningMethod = sign
                  `),
			nil,
			true, false,
			[]ServiceDescriptor{},
		},
		{
			"Missing Signing Region",
			strings.NewReader(`
                [global]

                [ServiceOverride "1"]
                 Service=s3
                 Region=sregion
                 URL=https://s3.foo.bar
                 SigningMethod = sign
                 `),
			nil,
			true, false,
			[]ServiceDescriptor{},
		},
		{
			"Active Overrides",
			strings.NewReader(`
                [Global]

               [ServiceOverride "1"]
                Service = "s3      "
                Region = sregion
                URL = https://s3.foo.bar
                SigningRegion = sregion
                SigningMethod = v4
                `),
			nil,
			false, true,
			[]ServiceDescriptor{{name: "s3", region: "sregion", signingRegion: "sregion", signingMethod: "v4"}},
		},
		{
			"Multiple Overridden Services",
			strings.NewReader(`
                [Global]
                 vpc = vpc-abc1234567

				[ServiceOverride "1"]
                  Service=s3
                  Region=sregion1
                  URL=https://s3.foo.bar
                  SigningRegion=sregion1
                  SigningMethod = v4

				[ServiceOverride "2"]
                  Service=ec2
                  Region=sregion2
                  URL=https://ec2.foo.bar
                  SigningRegion=sregion2
                  SigningMethod = v4`),
			nil,
			false, true,
			[]ServiceDescriptor{{name: "s3", region: "sregion1", signingRegion: "sregion1", signingMethod: "v4"},
				{name: "ec2", region: "sregion2", signingRegion: "sregion2", signingMethod: "v4"}},
		},
		{
			"Duplicate Services",
			strings.NewReader(`
                [Global]
                 vpc = vpc-abc1234567

				[ServiceOverride "1"]
                  Service=s3
                  Region=sregion1
                  URL=https://s3.foo.bar
                  SigningRegion=sregion
                  SigningMethod = sign

				[ServiceOverride "2"]
                  Service=s3
                  Region=sregion1
                  URL=https://s3.foo.bar
                  SigningRegion=sregion
                  SigningMethod = sign`),
			nil,
			true, false,
			[]ServiceDescriptor{},
		},
		{
			"Multiple Overridden Services in Multiple regions",
			strings.NewReader(`
                 [global]

				[ServiceOverride "1"]
                 Service=s3
                 Region=region1
                 URL=https://s3.foo.bar
                 SigningRegion=sregion1

				[ServiceOverride "2"]
                 Service=ec2
                 Region=region2
                 URL=https://ec2.foo.bar
                 SigningRegion=sregion
                 SigningMethod = v4
                 `),
			nil,
			false, true,
			[]ServiceDescriptor{{name: "s3", region: "region1", signingRegion: "sregion1", signingMethod: ""},
				{name: "ec2", region: "region2", signingRegion: "sregion", signingMethod: "v4"}},
		},
		{
			"Multiple regions, Same Service",
			strings.NewReader(`
                 [global]

				[ServiceOverride "1"]
                Service=s3
                Region=region1
                URL=https://s3.foo.bar
                SigningRegion=sregion1
                SigningMethod = v3

				[ServiceOverride "2"]
                 Service=s3
                 Region=region2
                 URL=https://s3.foo.bar
                 SigningRegion=sregion1
				 SigningMethod = v4
                 SigningName = "name"
                 `),
			nil,
			false, true,
			[]ServiceDescriptor{{name: "s3", region: "region1", signingRegion: "sregion1", signingMethod: "v3"},
				{name: "s3", region: "region2", signingRegion: "sregion1", signingMethod: "v4", signingName: "name"}},
		},
	}

	for _, test := range tests {
		t.Logf("Running test case %s", test.name)
		cfg, err := readOSCCloudConfig(test.reader)
		if err == nil {
			err = cfg.validateOverrides()
		}
		if test.expectError {
			if err == nil {
				t.Errorf("Should error for case %s (cfg=%v)", test.name, cfg)
			}
		} else {
			if err != nil {
				t.Errorf("Should succeed for case: %s, got %v", test.name, err)
			}

			if len(cfg.ServiceOverride) != len(test.servicesOverridden) {
				t.Errorf("Expected %d overridden services, received %d for case %s",
					len(test.servicesOverridden), len(cfg.ServiceOverride), test.name)
			} else {
				for _, sd := range test.servicesOverridden {
					var found *struct {
						Service       string
						Region        string
						URL           string
						SigningRegion string
						SigningMethod string
						SigningName   string
					}
					for _, v := range cfg.ServiceOverride {
						if v.Service == sd.name && v.Region == sd.region {
							found = v
							break
						}
					}
					if found == nil {
						t.Errorf("Missing override for service %s in case %s",
							sd.name, test.name)
					} else {
						if found.SigningRegion != sd.signingRegion {
							t.Errorf("Expected signing region '%s', received '%s' for case %s",
								sd.signingRegion, found.SigningRegion, test.name)
						}
						if found.SigningMethod != sd.signingMethod {
							t.Errorf("Expected signing method '%s', received '%s' for case %s",
								sd.signingMethod, found.SigningRegion, test.name)
						}
						targetName := fmt.Sprintf("https://%s.foo.bar", sd.name)
						if found.URL != targetName {
							t.Errorf("Expected Endpoint '%s', received '%s' for case %s",
								targetName, found.URL, test.name)
						}
						if found.SigningName != sd.signingName {
							t.Errorf("Expected signing name '%s', received '%s' for case %s",
								sd.signingName, found.SigningName, test.name)
						}

						fn := cfg.getResolver()
						ep1, e := fn(sd.name, sd.region, nil)
						if e != nil {
							t.Errorf("Expected a valid endpoint for %s in case %s",
								sd.name, test.name)
						} else {
							targetName := fmt.Sprintf("https://%s.foo.bar", sd.name)
							if ep1.URL != targetName {
								t.Errorf("Expected endpoint url: %s, received %s in case %s",
									targetName, ep1.URL, test.name)
							}
							if ep1.SigningRegion != sd.signingRegion {
								t.Errorf("Expected signing region '%s', received '%s' in case %s",
									sd.signingRegion, ep1.SigningRegion, test.name)
							}
							if ep1.SigningMethod != sd.signingMethod {
								t.Errorf("Expected signing method '%s', received '%s' in case %s",
									sd.signingMethod, ep1.SigningRegion, test.name)
							}
						}
					}
				}
			}
		}
	}
}

func TestNewOSCCloud(t *testing.T) {
	tests := []struct {
		name string

		reader      io.Reader
		oscServices Services

		expectError bool
		region      string
	}{
		{
			"No config reader",
			nil, newMockedFakeOSCServices(TestClusterID).WithAz(""),
			true, "",
		},
		{
			"Config specifies valid zone",
			strings.NewReader("[global]\nzone = eu-west-2a"), newMockedFakeOSCServices(TestClusterID),
			false, "eu-west-2",
		},
		{
			"Gets zone from metadata when not in config",
			strings.NewReader("[global]\n"),
			newMockedFakeOSCServices(TestClusterID),
			false, "eu-west-2",
		},
		{
			"No zone in config or metadata",
			strings.NewReader("[global]\n"),
			newMockedFakeOSCServices(TestClusterID).WithAz(""),
			true, "",
		},
	}

	for _, test := range tests {
		t.Logf("Running test case %s", test.name)
		cfg, err := readOSCCloudConfig(test.reader)
		t.Logf("After readOSCCloudConfig")

		var c *Cloud
		if err == nil {
		    t.Logf("if  newOSCCloud Before")
			c, err = newOSCCloud(*cfg, test.oscServices)
			t.Logf("if  newOSCCloud After")
		}
		t.Logf("After newOSCCloud")
		if test.expectError {
			if err == nil {
				t.Errorf("Should error for case %s", test.name)
			}
		} else {
			if err != nil {
				t.Errorf("Should succeed for case: %s, got %v", test.name, err)
			} else if c.region != test.region {
				t.Errorf("Incorrect region value (%s vs %s) for case: %s",
					c.region, test.region, test.name)
			}
		}
	}
}

func mockInstancesResp(selfInstance osc.Vm, instances []osc.Vm) (*Cloud, *FakeOSCServices) {
	oscServices := newMockedFakeOSCServices(TestClusterID)
	oscServices.instances = instances
	oscServices.selfInstance = selfInstance
	oscCloud, err := newOSCCloud(CloudConfig{}, oscServices)
	if err != nil {
		panic(err)
	}
	oscCloud.kubeClient = fake.NewSimpleClientset()
	return oscCloud, oscServices
}

func mockAvailabilityZone(availabilityZone string) *Cloud {
	oscServices := newMockedFakeOSCServices(TestClusterID).WithAz(availabilityZone)
	oscCloud, err := newOSCCloud(CloudConfig{}, oscServices)
	if err != nil {
		panic(err)
	}
	oscCloud.kubeClient = fake.NewSimpleClientset()
	return oscCloud
}

func testHasNodeAddress(t *testing.T, addrs []v1.NodeAddress, addressType v1.NodeAddressType, address string) {
	for _, addr := range addrs {
		if addr.Type == addressType && addr.Address == address {
			return
		}
	}
	t.Errorf("Did not find expected address: %s:%s in %v", addressType, address, addrs)
}

func TestNodeAddresses(t *testing.T) {
	// Note these instances have the same name
	// (we test that this produces an error)
	var instance0 osc.Vm
	var instance1 osc.Vm
	var instance2 osc.Vm

	// ClusterID needs to be set
	tags := []osc.ResourceTag{
		{Key: TagNameKubernetesClusterPrefix + TestClusterID, Value: ResourceLifecycleOwned},
		{Key: TagNameClusterNode, Value: "instance-same.ec2.internal"},
	}

	//0
	instance0.VmId = "i-0"
	instance0.PrivateDnsName = "instance-same.ec2.internal"
	instance0.PrivateIp = "192.168.0.1"
	instance0.PublicDnsName = "instance-same.ec2.external"
	instance0.PublicIp = "1.2.3.4"
	instance0.Nics = []osc.NicLight{
		{
			State: "in-use",
			PrivateIps: []osc.PrivateIpLightForVm{
				{
					PrivateIp: "192.168.0.1",
				},
			},
		},
	}
	instance0.VmType = "c3.large"
	instance0.Placement = osc.Placement{SubregionName: "eu-west-2a"}
	instance0.Tags = tags
// 	state0 := osc.VmStates{
// 		VmState: "running",
// 	}
	instance0.State = "running"

	//1
	instance1.VmId = "i-1"
	instance1.PrivateDnsName = "instance-same.ec2.internal"
	instance1.PrivateIp = "192.168.0.2"
	instance1.VmType = "c3.large"
	instance1.Placement = osc.Placement{SubregionName: "eu-west-2a"}
	instance1.Tags = tags
// 	state1 := osc.VmStates{
// 		VmState: "running",
// 	}
	instance1.State = "running"

	//2
	instance2.VmId = "i-2"
	instance2.PrivateDnsName = "instance-other.ec2.internal"
	instance2.PrivateIp = "192.168.0.1"
	instance2.PublicIp = "1.2.3.4"
	instance2.VmType = "c3.large"
	instance2.Placement = osc.Placement{SubregionName: "eu-west-2a"}
	instance2.Tags = tags
// 	state2 := osc.VmStates{
// 		VmState: "running",
// 	}
	instance2.State = "running"

	instances := []osc.Vm{instance0, instance1, instance2}

	osc1, _ := mockInstancesResp(instance0, []osc.Vm{instance0})
	_, err1 := osc1.NodeAddresses(context.TODO(), "instance-mismatch.ec2.internal")
	if err1 == nil {
		t.Errorf("Should error when no instance found")
	}

	osc2, _ := mockInstancesResp(instance2, instances)
	_, err2 := osc2.NodeAddresses(context.TODO(), "instance-same.ec2.internal")
	if err2 == nil {
		t.Errorf("Should error when multiple instances found")
	}

	osc3, _ := mockInstancesResp(instance0, instances[0:1])
	// change node name so it uses the instance instead of metadata
	osc3.selfOSCInstance.nodeName = "foo"
	addrs3, err3 := osc3.NodeAddresses(context.TODO(), "instance-same.ec2.internal")
	if err3 != nil {
		t.Errorf("Should not error when instance found")
	}
	if len(addrs3) != 5 {
		t.Errorf("Should return exactly 5 NodeAddresses")
	}
	testHasNodeAddress(t, addrs3, v1.NodeInternalIP, "192.168.0.1")
	testHasNodeAddress(t, addrs3, v1.NodeExternalIP, "1.2.3.4")
	testHasNodeAddress(t, addrs3, v1.NodeExternalDNS, "instance-same.ec2.external")
	testHasNodeAddress(t, addrs3, v1.NodeInternalDNS, "instance-same.ec2.internal")
	testHasNodeAddress(t, addrs3, v1.NodeHostName, "instance-same.ec2.internal")
}

func TestNodeAddressesWithMetadata(t *testing.T) {
	var instance osc.Vm

	// ClusterID needs to be set
	tags := []osc.ResourceTag{
	    {
            Key: TagNameKubernetesClusterLegacy,
            Value: TestClusterID,
	    },

	}

	instanceName := "instance.ec2.internal"
	instance.VmId = "i-0"
	instance.PrivateDnsName = instanceName
	instance.PublicIp = "2.3.4.5"
	instance.VmType = "c3.large"
	instance.Placement = osc.Placement{SubregionName: "eu-west-2a"}
	instance.Tags = tags
// 	state := osc.VmStates{
// 		VmState: "running",
// 	}
	instance.State = "running"

	instances := []osc.Vm{instance}
	oscCloud, oscServices := mockInstancesResp(instance, instances)

	oscServices.networkInterfacesMacs = []string{"0a:26:89:f3:9c:f6", "0a:77:64:c4:6a:48"}
	oscServices.networkInterfacesPrivateIPs = [][]string{{"192.168.0.1"}, {"192.168.0.2"}}
	addrs, err := oscCloud.NodeAddresses(context.TODO(), "")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	testHasNodeAddress(t, addrs, v1.NodeInternalIP, "192.168.0.1")
	testHasNodeAddress(t, addrs, v1.NodeInternalIP, "192.168.0.2")
	testHasNodeAddress(t, addrs, v1.NodeExternalIP, "2.3.4.5")
}

func TestParseMetadataLocalHostname(t *testing.T) {
	tests := []struct {
		name        string
		metadata    string
		hostname    string
		internalDNS []string
	}{
		{
			"single hostname",
			"ip-172-31-16-168.us-west-2.compute.internal",
			"ip-172-31-16-168.us-west-2.compute.internal",
			[]string{"ip-172-31-16-168.us-west-2.compute.internal"},
		},
		{
			"dhcp options set with three additional domain names",
			"ip-172-31-16-168.us-west-2.compute.internal example.com example.ca example.org",
			"ip-172-31-16-168.us-west-2.compute.internal",
			[]string{"ip-172-31-16-168.us-west-2.compute.internal", "ip-172-31-16-168.example.com", "ip-172-31-16-168.example.ca", "ip-172-31-16-168.example.org"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			hostname, internalDNS := parseMetadataLocalHostname(test.metadata)
			if hostname != test.hostname {
				t.Errorf("got hostname %v, expected %v", hostname, test.hostname)
			}
			for i, v := range internalDNS {
				if v != test.internalDNS[i] {
					t.Errorf("got an internalDNS %v, expected %v", v, test.internalDNS[i])
				}
			}
		})
	}
}

func TestGetRegion(t *testing.T) {
	osc := mockAvailabilityZone("us-west-2e")
	zones, ok := osc.Zones()
	if !ok {
		t.Fatalf("Unexpected missing zones impl")
	}
	zone, err := zones.GetZone(context.TODO())
	if err != nil {
		t.Fatalf("unexpected error %v", err)
	}
	if zone.Region != "us-west-2" {
		t.Errorf("Unexpected region: %s", zone.Region)
	}
	if zone.FailureDomain != "us-west-2e" {
		t.Errorf("Unexpected FailureDomain: %s", zone.FailureDomain)
	}
}

func TestFindVPCID(t *testing.T) {
	oscServices := newMockedFakeOSCServices(TestClusterID)
	c, err := newOSCCloud(CloudConfig{}, oscServices)
	if err != nil {
		t.Errorf("Error building osc cloud: %v", err)
	}
	vpcID, err := c.findVPCID()
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if vpcID != "vpc-mac0" {
		t.Errorf("Unexpected vpcID: %s", vpcID)
	}
}

func constructSubnets(subnetsIn map[int]map[string]string) (subnetsOut []osc.Subnet) {
	for i := range subnetsIn {
		subnetsOut = append(
			subnetsOut,
			constructSubnet(
				subnetsIn[i]["id"],
				subnetsIn[i]["az"],
			),
		)
	}
	return
}

func constructSubnet(id string, az string) osc.Subnet {
	return osc.Subnet{
		SubnetId:         id,
		SubregionName: az,
		NetId:            "vpc-123456",
		Tags: []osc.ResourceTag{
			{
				Key:   fmt.Sprintf("%s%s", TagNameKubernetesClusterPrefix, TestClusterID),
				Value: ResourceLifecycleOwned,
			},
		},
	}

}

func constructRouteTables(routeTablesIn map[string]bool) (routeTablesOut []osc.RouteTable) {
	routeTablesOut = append(routeTablesOut,
		osc.RouteTable{
			LinkRouteTables: []osc.LinkRouteTable{{Main: true}},
			Routes: []osc.Route{{
				DestinationIpRange: "0.0.0.0/0",
				GatewayId:            "igw-main",
			}},
		})

	for subnetID := range routeTablesIn {
		routeTablesOut = append(
			routeTablesOut,
			constructRouteTable(
				subnetID,
				routeTablesIn[subnetID],
			),
		)
	}
	return
}

func constructRouteTable(subnetID string, public bool) osc.RouteTable {
	var gatewayID string
	if public {
		gatewayID = "igw-" + subnetID[len(subnetID)-8:8]
	} else {
		gatewayID = "vgw-" + subnetID[len(subnetID)-8:8]
	}
	return osc.RouteTable{
		LinkRouteTables: []osc.LinkRouteTable{{SubnetId: subnetID}},
		Routes: []osc.Route{{
			DestinationIpRange: "0.0.0.0/0",
			GatewayId:            gatewayID,
		}},
	}
}

func TestSubnetIDsinVPC(t *testing.T) {
	oscServices := newMockedFakeOSCServices(TestClusterID)
	c, err := newOSCCloud(CloudConfig{}, oscServices)
	c.netID = "vpc-123456"
	if err != nil {
		t.Errorf("Error building osc cloud: %v", err)
		return
	}

	// test with 3 subnets from 3 different AZs
	subnets := make(map[int]map[string]string)
	subnets[0] = make(map[string]string)
	subnets[0]["id"] = "subnet-a0000001"
	subnets[0]["az"] = "af-south-1a"
	subnets[1] = make(map[string]string)
	subnets[1]["id"] = "subnet-b0000001"
	subnets[1]["az"] = "af-south-1b"
	subnets[2] = make(map[string]string)
	subnets[2]["id"] = "subnet-c0000001"
	subnets[2]["az"] = "af-south-1c"
	constructedSubnets := constructSubnets(subnets)
	oscServices.fcu.RemoveSubnets()
	for _, subnet := range constructedSubnets {
		oscServices.fcu.CreateSubnet(subnet)
	}

	routeTables := map[string]bool{
		"subnet-a0000001": true,
		"subnet-b0000001": true,
		"subnet-c0000001": true,
	}
	constructedRouteTables := constructRouteTables(routeTables)
	oscServices.fcu.RemoveRouteTables()
	for _, rt := range constructedRouteTables {
		oscServices.fcu.CreateRouteTable(rt)
	}
	request1111 := &osc.ReadSubnetsOpts{}
	res, _, _ := oscServices.fcu.ReadSubnets(request1111)
	t.Logf("oscServices.fcu.DescribeSubnets----: %v", res)

	request2222 := &osc.ReadRouteTablesOpts{}
	rt, _, err := oscServices.fcu.ReadRouteTables(request2222)
	t.Logf("oscServices.fcu.DescribeRouteTables----: %v", rt)

	subnetsRes, err := c.findSubnets()
	t.Logf("subnetsRes, err----: %v", subnetsRes)

	result, err := c.findLBUSubnets(false)
	if err != nil {
		t.Errorf("Error listing subnets: %v", err)
		return
	}

	if len(result) != 3 {
		t.Errorf("Expected 3 subnets but got %d", len(result))
		return
	}

	resultSet := make(map[string]bool)
	for _, v := range result {
		resultSet[v] = true
	}

	for i := range subnets {
		if !resultSet[subnets[i]["id"]] {
			t.Errorf("Expected subnet%d '%s' in result: %v", i, subnets[i]["id"], result)
			return
		}
	}

	// test implicit routing table - when subnets are not explicitly linked to a table they should use main
	constructedRouteTables = constructRouteTables(map[string]bool{})
	oscServices.fcu.RemoveRouteTables()
	for _, rt := range constructedRouteTables {
		oscServices.fcu.CreateRouteTable(rt)
	}

	result, err = c.findLBUSubnets(false)
	if err != nil {
		t.Errorf("Error listing subnets: %v", err)
		return
	}

	if len(result) != 3 {
		t.Errorf("Expected 3 subnets but got %d", len(result))
		return
	}

	resultSet = make(map[string]bool)
	for _, v := range result {
		resultSet[v] = true
	}

	for i := range subnets {
		if !resultSet[subnets[i]["id"]] {
			t.Errorf("Expected subnet%d '%s' in result: %v", i, subnets[i]["id"], result)
			return
		}
	}

	// Test with 5 subnets from 3 different AZs.
	// Add 2 duplicate AZ subnets lexicographically chosen one is the middle element in array to
	// check that we both choose the correct entry when it comes after and before another element
	// in the same AZ.
	subnets[3] = make(map[string]string)
	subnets[3]["id"] = "subnet-c0000000"
	subnets[3]["az"] = "af-south-1c"
	subnets[4] = make(map[string]string)
	subnets[4]["id"] = "subnet-c0000002"
	subnets[4]["az"] = "af-south-1c"
	constructedSubnets = constructSubnets(subnets)
	oscServices.fcu.RemoveSubnets()
	for _, subnet := range constructedSubnets {
		oscServices.fcu.CreateSubnet(subnet)
	}
	routeTables["subnet-c0000000"] = true
	routeTables["subnet-c0000002"] = true
	constructedRouteTables = constructRouteTables(routeTables)
	oscServices.fcu.RemoveRouteTables()
	for _, rt := range constructedRouteTables {
		oscServices.fcu.CreateRouteTable(rt)
	}

	result, err = c.findLBUSubnets(false)
	if err != nil {
		t.Errorf("Error listing subnets: %v", err)
		return
	}

	if len(result) != 3 {
		t.Errorf("Expected 3 subnets but got %d", len(result))
		return
	}

	expected := []string{"subnet-a0000001", "subnet-b0000001", "subnet-c0000000"}
	for _, s := range result {
		if !contains(expected, s) {
			t.Errorf("Unexpected subnet '%s' found", s)
			return
		}
	}

	delete(routeTables, "subnet-c0000002")

	// test with 6 subnets from 3 different AZs
	// with 3 private subnets
	subnets[4] = make(map[string]string)
	subnets[4]["id"] = "subnet-d0000001"
	subnets[4]["az"] = "af-south-1a"
	subnets[5] = make(map[string]string)
	subnets[5]["id"] = "subnet-d0000002"
	subnets[5]["az"] = "af-south-1b"

	constructedSubnets = constructSubnets(subnets)
	oscServices.fcu.RemoveSubnets()
	for _, subnet := range constructedSubnets {
		oscServices.fcu.CreateSubnet(subnet)
	}

	routeTables["subnet-a0000001"] = false
	routeTables["subnet-b0000001"] = false
	routeTables["subnet-c0000001"] = false
	routeTables["subnet-c0000000"] = true
	routeTables["subnet-d0000001"] = true
	routeTables["subnet-d0000002"] = true
	constructedRouteTables = constructRouteTables(routeTables)
	oscServices.fcu.RemoveRouteTables()
	for _, rt := range constructedRouteTables {
		oscServices.fcu.CreateRouteTable(rt)
	}
	result, err = c.findLBUSubnets(false)
	if err != nil {
		t.Errorf("Error listing subnets: %v", err)
		return
	}

	if len(result) != 3 {
		t.Errorf("Expected 3 subnets but got %d", len(result))
		return
	}

	expected = []string{"subnet-c0000000", "subnet-d0000001", "subnet-d0000002"}
	for _, s := range result {
		if !contains(expected, s) {
			t.Errorf("Unexpected subnet '%s' found", s)
			return
		}
	}
}

func TestSecurityGroupRuleExistsHandlesMultipleSecurityGroupIds(t *testing.T) {
	oldSecurityGroupRule := osc.SecurityGroupRule{
		SecurityGroupsMembers: []osc.SecurityGroupsMember{
			{SecurityGroupId: "firstSecurityGroupId"},
			{SecurityGroupId: "secondSecurityGroupId"},
			{SecurityGroupId: "thirdSecurityGroupId"},
		},
	}

	existingSecurityGroupRule := osc.SecurityGroupRule{
		SecurityGroupsMembers: []osc.SecurityGroupsMember{
			{SecurityGroupId: "secondSecurityGroupId"},
		},
	}

	newSecurityGroupRule := osc.SecurityGroupRule{
		SecurityGroupsMembers: []osc.SecurityGroupsMember{
			{SecurityGroupId: "fourthSecurityGroupId"},
		},
	}

	equals := securityGroupRuleExists(existingSecurityGroupRule, oldSecurityGroupRule, false)
	if !equals {
		t.Errorf("Should have been considered equal since first is in the second array of groups")
	}

	equals = securityGroupRuleExists(newSecurityGroupRule, oldSecurityGroupRule, false)
	if equals {
		t.Errorf("Should have not been considered equal since first is not in the second array of groups")
	}

	// The first pair matches, but the second does not
	newSecurityGroupRule2 := osc.SecurityGroupRule{
		SecurityGroupsMembers: []osc.SecurityGroupsMember{
			{SecurityGroupId: "firstSecurityGroupId"},
			{SecurityGroupId: "fourthSecurityGroupId"},
		},
	}
	equals = securityGroupRuleExists(newSecurityGroupRule2, oldSecurityGroupRule, false)
	if equals {
		t.Errorf("Should have not been considered equal since first is not in the second array of groups")
	}
}

func TestSecurityGroupRuleExistsHandlesRangeSubsets(t *testing.T) {
	// Two existing scenarios we'll test against
	emptySecurityGroupRule := osc.SecurityGroupRule{}

	oldSecurityGroupRule := osc.SecurityGroupRule{
		IpRanges: []string{
		    "10.0.0.0/8",
		    "192.168.1.0/24",
		},
	}

	// Two already existing ranges and a new one
	existingSecurityGroupRule := osc.SecurityGroupRule{
		IpRanges: []string{
			"10.0.0.0/8",
		},
	}
	existingSecurityGroupRule2 := osc.SecurityGroupRule{
		IpRanges: []string{
			"192.168.1.0/24",
		},
	}

	newSecurityGroupRule := osc.SecurityGroupRule{
		IpRanges: []string{
			"172.16.0.0/16",
		},
	}

	exists := securityGroupRuleExists(emptySecurityGroupRule, emptySecurityGroupRule, false)
	if !exists {
		t.Errorf("Should have been considered existing since we're comparing a range array against itself")
	}
	exists = securityGroupRuleExists(oldSecurityGroupRule, oldSecurityGroupRule, false)
	if !exists {
		t.Errorf("Should have been considered existing since we're comparing a range array against itself")
	}

	exists = securityGroupRuleExists(existingSecurityGroupRule, oldSecurityGroupRule, false)
	if !exists {
		t.Errorf("Should have been considered existing since 10.* is in oldSecurityGroupRule's array of ranges")
	}
	exists = securityGroupRuleExists(existingSecurityGroupRule2, oldSecurityGroupRule, false)
	if !exists {
		t.Errorf("Should have been considered existing since 192.* is in oldSecurityGroupRule2's array of ranges")
	}

	exists = securityGroupRuleExists(newSecurityGroupRule, emptySecurityGroupRule, false)
	if exists {
		t.Errorf("Should have not been considered existing since we compared against a missing array of ranges")
	}
	exists = securityGroupRuleExists(newSecurityGroupRule, oldSecurityGroupRule, false)
	if exists {
		t.Errorf("Should have not been considered existing since 172.* is not in oldSecurityGroupRule's array of ranges")
	}
}

func TestSecurityGroupRuleExistsHandlesMultipleGroupIdsWithUserIds(t *testing.T) {
	oldSecurityGroupRule := osc.SecurityGroupRule{
		SecurityGroupsMembers: []osc.SecurityGroupsMember{
			{SecurityGroupId: "firstSecurityGroupId", AccountId: "firstUserId"},
			{SecurityGroupId: "secondSecurityGroupId", AccountId: "secondUserId"},
			{SecurityGroupId: "thirdSecurityGroupId", AccountId: "thirdUserId"},
		},
	}

	existingSecurityGroupRule := osc.SecurityGroupRule{
		SecurityGroupsMembers: []osc.SecurityGroupsMember{
			{SecurityGroupId: "secondSecurityGroupId", AccountId: "secondUserId"},
		},
	}

	newSecurityGroupRule := osc.SecurityGroupRule{
		SecurityGroupsMembers: []osc.SecurityGroupsMember{
			{SecurityGroupId: "secondSecurityGroupId", AccountId: "anotherUserId"},
		},
	}

	equals := securityGroupRuleExists(existingSecurityGroupRule, oldSecurityGroupRule, true)
	if !equals {
		t.Errorf("Should have been considered equal since first is in the second array of groups")
	}

	equals = securityGroupRuleExists(newSecurityGroupRule, oldSecurityGroupRule, true)
	if equals {
		t.Errorf("Should have not been considered equal since first is not in the second array of groups")
	}
}

func TestFindInstanceByNodeNameExcludesTerminatedInstances(t *testing.T) {
	oscStates := []struct {
		id       int64
		state    string
		expected bool
	}{
		{0, "pending", true},
		{16, "running", true},
		{32, "shutting-down", true},
		{48, "terminated", false},
		{64, "stopping", true},
		{80, "stopped", true},
	}
	oscServices := newMockedFakeOSCServices(TestClusterID)

	nodeName := types.NodeName("my-dns.internal")

	// ClusterID needs to be set
	tags := []osc.ResourceTag{
		{Key: TagNameKubernetesClusterPrefix + TestClusterID, Value: ResourceLifecycleOwned},
		{Key: TagNameClusterNode, Value: string(nodeName)},
	}

	var testInstance osc.Vm
	testInstance.PrivateDnsName = string(nodeName)
	testInstance.Tags = tags

	oscDefaultInstances := oscServices.instances
	for _, oscState := range oscStates {
		id := "i-" + oscState.state
		testInstance.VmId = id
		// A verifier osc.VmState{VmId: testInstance.VmId, CurrentState: oscState.state}
		testInstance.State = oscState.state

		oscServices.instances = append(oscDefaultInstances, testInstance)

		c, err := newOSCCloud(CloudConfig{}, oscServices)
		if err != nil {
			t.Errorf("Error building osc cloud: %v", err)
			return
		}

		resultInstance, err := c.findInstanceByNodeName(nodeName)

		if oscState.expected {
			if err != nil || resultInstance.VmId == "" {
				t.Errorf("Expected to find instance %v", testInstance.VmId)
				return
			}
			if resultInstance.VmId != testInstance.VmId {
				t.Errorf("Wrong instance returned by findInstanceByNodeName() expected: %v, actual: %v", testInstance.VmId, resultInstance.VmId)
				return
			}
		} else {
			if err == nil && resultInstance.VmId != "" {
				t.Errorf("Did not expect to find instance %v", resultInstance.VmId)
				return
			}
		}
	}
}

func TestGetInstanceByNodeNameBatching(t *testing.T) {
	oscServices := newMockedFakeOSCServices(TestClusterID)
	c, err := newOSCCloud(CloudConfig{}, oscServices)
	assert.Nil(t, err, "Error building osc cloud: %v", err)

	tags := []osc.ResourceTag{
	    {
	        Key: TagNameKubernetesClusterPrefix + TestClusterID,
	        Value: "",
	    },
	}

	nodeNames := []string{}
	for i := 0; i < 200; i++ {
		nodeName := fmt.Sprintf("ip-171-20-42-%d.ec2.internal", i)
		nodeNames = append(nodeNames, nodeName)
		oscInstance := osc.Vm{}
		instanceID := fmt.Sprintf("i-abcedf%d", i)
		oscInstance.VmId = instanceID
		oscInstance.PrivateDnsName = nodeName
		oscInstance.State = "InService"
		oscInstance.Tags = tags
		oscServices.instances = append(oscServices.instances, oscInstance)

	}

	instances, err := c.getInstancesByNodeNames(nodeNames)
	assert.Nil(t, err, "Error getting instances by nodeNames %v: %v", nodeNames, err)
	assert.NotEmpty(t, instances)
	assert.Equal(t, 200, len(instances), "Expected 200 but got less")
}

func TestDescribeLoadBalancerOnDelete(t *testing.T) {
	oscServices := newMockedFakeOSCServices(TestClusterID)
	c, _ := newOSCCloud(CloudConfig{}, oscServices)
	c.netID = "vpc-123456"
	oscServices.lbu.(*MockedFakeLBU).expectReadLoadBalancers("aid")

	c.EnsureLoadBalancerDeleted(context.TODO(), TestClusterName, &v1.Service{ObjectMeta: metav1.ObjectMeta{Name: "myservice", UID: "id"}})
}

func TestDescribeLoadBalancerOnUpdate(t *testing.T) {
	oscServices := newMockedFakeOSCServices(TestClusterID)
	c, _ := newOSCCloud(CloudConfig{}, oscServices)
	c.netID = "vpc-123456"
	oscServices.lbu.(*MockedFakeLBU).expectReadLoadBalancers("aid")

	c.UpdateLoadBalancer(context.TODO(), TestClusterName, &v1.Service{ObjectMeta: metav1.ObjectMeta{Name: "myservice", UID: "id"}}, []*v1.Node{})
}

func TestDescribeLoadBalancerOnGet(t *testing.T) {
	oscServices := newMockedFakeOSCServices(TestClusterID)
	c, _ := newOSCCloud(CloudConfig{}, oscServices)
	oscServices.lbu.(*MockedFakeLBU).expectReadLoadBalancers("aid")

	c.GetLoadBalancer(context.TODO(), TestClusterName, &v1.Service{ObjectMeta: metav1.ObjectMeta{Name: "myservice", UID: "id"}})
}

func TestDescribeLoadBalancerOnEnsure(t *testing.T) {
	oscServices := newMockedFakeOSCServices(TestClusterID)
	c, _ := newOSCCloud(CloudConfig{}, oscServices)
	oscServices.lbu.(*MockedFakeLBU).expectReadLoadBalancers("aid")

	c.EnsureLoadBalancer(context.TODO(), TestClusterName, &v1.Service{ObjectMeta: metav1.ObjectMeta{Name: "myservice", UID: "id"}}, []*v1.Node{})
}

func TestBuildListener(t *testing.T) {
	tests := []struct {
		name string

		lbPort                    int64
		portName                  string
		instancePort              int64
		backendProtocolAnnotation string
		certAnnotation            string
		sslPortAnnotation         string

		expectError      bool
		lbProtocol       string
		instanceProtocol string
		certID           string
	}{
		{
			"No cert or BE protocol annotation, passthrough",
			80, "", 7999, "", "", "",
			false, "TCP", "TCP", "",
		},
		{
			"Cert annotation without BE protocol specified, SSL->TCP",
			80, "", 8000, "", "cert", "",
			false, "SSL", "TCP", "cert",
		},
		{
			"BE protocol without cert annotation, passthrough",
			443, "", 8001, "https", "", "",
			false, "TCP", "TCP", "",
		},
		{
			"Invalid cert annotation, bogus backend protocol",
			443, "", 8002, "bacon", "foo", "",
			true, "TCP", "TCP", "",
		},
		{
			"Invalid cert annotation, protocol followed by equal sign",
			443, "", 8003, "http=", "=", "",
			true, "TCP", "TCP", "",
		},
		{
			"HTTPS->HTTPS",
			443, "", 8004, "https", "cert", "",
			false, "HTTPS", "HTTPS", "cert",
		},
		{
			"HTTPS->HTTP",
			443, "", 8005, "http", "cert", "",
			false, "HTTPS", "HTTP", "cert",
		},
		{
			"SSL->SSL",
			443, "", 8006, "ssl", "cert", "",
			false, "SSL", "SSL", "cert",
		},
		{
			"SSL->TCP",
			443, "", 8007, "tcp", "cert", "",
			false, "SSL", "TCP", "cert",
		},
		{
			"Port in whitelist",
			1234, "", 8008, "tcp", "cert", "1234,5678",
			false, "SSL", "TCP", "cert",
		},
		{
			"Port not in whitelist, passthrough",
			443, "", 8009, "tcp", "cert", "1234,5678",
			false, "TCP", "TCP", "",
		},
		{
			"Named port in whitelist",
			1234, "bar", 8010, "tcp", "cert", "foo,bar",
			false, "SSL", "TCP", "cert",
		},
		{
			"Named port not in whitelist, passthrough",
			443, "", 8011, "tcp", "cert", "foo,bar",
			false, "TCP", "TCP", "",
		},
		{
			"HTTP->HTTP",
			80, "", 8012, "http", "", "",
			false, "HTTP", "HTTP", "",
		},
	}

	for _, test := range tests {
		t.Logf("Running test case %s", test.name)
		annotations := make(map[string]string)
		if test.backendProtocolAnnotation != "" {
			annotations[ServiceAnnotationLoadBalancerBEProtocol] = test.backendProtocolAnnotation
		}
		if test.certAnnotation != "" {
			annotations[ServiceAnnotationLoadBalancerCertificate] = test.certAnnotation
		}
		ports := getPortSets(test.sslPortAnnotation)
		l, err := buildListener(v1.ServicePort{
			NodePort: int32(test.instancePort),
			Port:     int32(test.lbPort),
			Name:     test.portName,
			Protocol: v1.Protocol("tcp"),
		}, annotations, ports)
		if test.expectError {
			if err == nil {
				t.Errorf("Should error for case %s", test.name)
			}
		} else {
			if err != nil {
				t.Errorf("Should succeed for case: %s, got %v", test.name, err)
			} else {
				var cert string
				if test.certID != "" {
					cert = test.certID
				}
				expected := osc.Listener{
					BackendPort:     int32(test.instancePort),
					BackendProtocol: test.instanceProtocol,
					LoadBalancerPort: int32(test.lbPort),
					LoadBalancerProtocol: test.lbProtocol,
					ServerCertificateId: cert,
				}
				if !reflect.DeepEqual(l, expected) {
					t.Errorf("Incorrect listener (%v vs expected %v) for case: %s",
						l, expected, test.name)
				}
			}
		}
	}
}

func TestProxyProtocolEnabled(t *testing.T) {
	policies := sets.NewString(ProxyProtocolPolicyName, "FooBarFoo")
	fakeBackend := osc.Listener{
		BackendPort: int32(80),
		PolicyNames:  stringSetToPointers(policies),
	}
	result := proxyProtocolEnabled(fakeBackend)
	assert.True(t, result, "expected to find %s in %s", ProxyProtocolPolicyName, policies)

	policies = sets.NewString("FooBarFoo")
	fakeBackend = osc.Listener{
		BackendPort: int32(80),
		PolicyNames: []string{
			"FooBarFoo",
		},
	}
	result = proxyProtocolEnabled(fakeBackend)
	assert.False(t, result, "did not expect to find %s in %s", ProxyProtocolPolicyName, policies)

	policies = sets.NewString()
	fakeBackend = osc.Listener{
		BackendPort: int32(80),
	}
	result = proxyProtocolEnabled(fakeBackend)
	assert.False(t, result, "did not expect to find %s in %s", ProxyProtocolPolicyName, policies)
}

func TestGetLoadBalancerAdditionalTags(t *testing.T) {
	tagTests := []struct {
		Annotations map[string]string
		Tags        map[string]string
	}{
		{
			Annotations: map[string]string{
				ServiceAnnotationLoadBalancerAdditionalTags: "Key=Val",
			},
			Tags: map[string]string{
				"Key": "Val",
			},
		},
		{
			Annotations: map[string]string{
				ServiceAnnotationLoadBalancerAdditionalTags: "Key1=Val1, Key2=Val2",
			},
			Tags: map[string]string{
				"Key1": "Val1",
				"Key2": "Val2",
			},
		},
		{
			Annotations: map[string]string{
				ServiceAnnotationLoadBalancerAdditionalTags: "Key1=, Key2=Val2",
				"anotherKey": "anotherValue",
			},
			Tags: map[string]string{
				"Key1": "",
				"Key2": "Val2",
			},
		},
		{
			Annotations: map[string]string{
				"Nothing": "Key1=, Key2=Val2, Key3",
			},
			Tags: map[string]string{},
		},
		{
			Annotations: map[string]string{
				ServiceAnnotationLoadBalancerAdditionalTags: "K=V K1=V2,Key1========, =====, ======Val, =Val, , 234,",
			},
			Tags: map[string]string{
				"K":    "V K1",
				"Key1": "",
				"234":  "",
			},
		},
	}

	for _, tagTest := range tagTests {
		result := getLoadBalancerAdditionalTags(tagTest.Annotations)
		for k, v := range result {
			if len(result) != len(tagTest.Tags) {
				t.Errorf("incorrect expected length: %v != %v", result, tagTest.Tags)
				continue
			}
			if tagTest.Tags[k] != v {
				t.Errorf("%s != %s", tagTest.Tags[k], v)
				continue
			}
		}
	}
}

func TestLBExtraSecurityGroupsAnnotation(t *testing.T) {
	oscServices := newMockedFakeOSCServices(TestClusterID)
	c, _ := newOSCCloud(CloudConfig{}, oscServices)
	c.netID = "vpc-123456"

	sg1 := map[string]string{ServiceAnnotationLoadBalancerExtraSecurityGroups: "sg-000001"}
	sg2 := map[string]string{ServiceAnnotationLoadBalancerExtraSecurityGroups: "sg-000002"}
	sg3 := map[string]string{ServiceAnnotationLoadBalancerExtraSecurityGroups: "sg-000001, sg-000002"}

	tests := []struct {
		name string

		annotations map[string]string
		expectedSGs []string
	}{
		{"No extra SG annotation", map[string]string{}, []string{}},
		{"Empty extra SGs specified", map[string]string{ServiceAnnotationLoadBalancerExtraSecurityGroups: ", ,,"}, []string{}},
		{"SG specified", sg1, []string{sg1[ServiceAnnotationLoadBalancerExtraSecurityGroups]}},
		{"Multiple SGs specified", sg3, []string{sg1[ServiceAnnotationLoadBalancerExtraSecurityGroups], sg2[ServiceAnnotationLoadBalancerExtraSecurityGroups]}},
	}

	oscServices.fcu.(*MockedFakeFCU).expectReadSecurityGroups(TestClusterID, "k8s-lbu-aid")

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			serviceName := types.NamespacedName{Namespace: "default", Name: "myservice"}

			sgList, err := c.buildLBUSecurityGroupList(serviceName, "aid", test.annotations)
			assert.NoError(t, err, "buildLBUSecurityGroupList failed")
			extraSGs := sgList[1:]
			assert.True(t, sets.NewString(test.expectedSGs...).Equal(sets.NewString(extraSGs...)),
				"Security Groups expected=%q , returned=%q", test.expectedSGs, extraSGs)
		})
	}
}

func TestLBSecurityGroupsAnnotation(t *testing.T) {
	oscServices := newMockedFakeOSCServices(TestClusterID)
	c, _ := newOSCCloud(CloudConfig{}, oscServices)
	c.netID = "vpc-123456"

	sg1 := map[string]string{ServiceAnnotationLoadBalancerSecurityGroups: "sg-000001"}
	sg2 := map[string]string{ServiceAnnotationLoadBalancerSecurityGroups: "sg-000002"}
	sg3 := map[string]string{ServiceAnnotationLoadBalancerSecurityGroups: "sg-000001, sg-000002"}

	tests := []struct {
		name string

		annotations map[string]string
		expectedSGs []string
	}{
		{"SG specified", sg1, []string{sg1[ServiceAnnotationLoadBalancerSecurityGroups]}},
		{"Multiple SGs specified", sg3, []string{sg1[ServiceAnnotationLoadBalancerSecurityGroups], sg2[ServiceAnnotationLoadBalancerSecurityGroups]}},
	}

	oscServices.fcu.(*MockedFakeFCU).expectReadSecurityGroups(TestClusterID, "k8s-lbu-aid")

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			serviceName := types.NamespacedName{Namespace: "default", Name: "myservice"}

			sgList, err := c.buildLBUSecurityGroupList(serviceName, "aid", test.annotations)
			assert.NoError(t, err, "buildLBUSecurityGroupList failed")
			assert.True(t, sets.NewString(test.expectedSGs...).Equal(sets.NewString(sgList...)),
				"Security Groups expected=%q , returned=%q", test.expectedSGs, sgList)
		})
	}
}

// Test that we can add a load balancer tag
func TestAddLoadBalancerTags(t *testing.T) {
	loadBalancerName := "test-lbu"
	oscServices := newMockedFakeOSCServices(TestClusterID)
	c, _ := newOSCCloud(CloudConfig{}, oscServices)

	want := make(map[string]string)
	want["tag1"] = "val1"

	expectedAddTagsRequest := osc.CreateTagsOpts{
	    CreateTagsRequest: optional.NewInterface(
	        osc.CreateTagsRequest{
	            ResourceIds: []string{loadBalancerName},
	            Tags: []osc.ResourceTag{
                    {
                        Key:   "tag1",
                        Value: "val1",
                    },
                },
	        }),
	}
	oscServices.lbu.(*MockedFakeLBU).On("AddTags", expectedAddTagsRequest).Return(osc.CreateTagsResponse{})

	err := c.addLoadBalancerTags(loadBalancerName, want)
	assert.Nil(t, err, "Error adding load balancer tags: %v", err)
	oscServices.lbu.(*MockedFakeLBU).AssertExpectations(t)
}

// func TestEnsureLoadBalancerHealthCheck(t *testing.T) {
//
// 	tests := []struct {
// 		name                string
// 		annotations         map[string]string
// 		overriddenFieldName string
// 		overriddenValue     int64
// 	}{
// 		{"falls back to HC defaults", map[string]string{}, "", int64(0)},
// 		{"healthy threshold override", map[string]string{ServiceAnnotationLoadBalancerHCHealthyThreshold: "7"}, "HealthyThreshold", int64(7)},
// 		{"unhealthy threshold override", map[string]string{ServiceAnnotationLoadBalancerHCUnhealthyThreshold: "7"}, "UnhealthyThreshold", int64(7)},
// 		{"timeout override", map[string]string{ServiceAnnotationLoadBalancerHCTimeout: "7"}, "Timeout", int64(7)},
// 		{"interval override", map[string]string{ServiceAnnotationLoadBalancerHCInterval: "7"}, "Interval", int64(7)},
// 	}
// 	lbName := "myLB"
// 	// this HC will always differ from the expected HC and thus it is expected an
// 	// API call will be made to update it
// 	currentHC := osc.HealthCheck{}
// 	lbuDesc := osc.LoadBalancer{LoadBalancerName: lbName, HealthCheck: currentHC}
// 	defaultHealthyThreshold := int32(2)
// 	defaultUnhealthyThreshold := int32(6)
// 	defaultTimeout := int32(5)
// 	defaultInterval := int32(10)
// 	protocol, path, port := "tcp", "", int32(8080)
// 	target := "tcp:8080"
// 	defaultHC := osc.HealthCheck{
// 		HealthyThreshold:   defaultHealthyThreshold,
// 		UnhealthyThreshold: defaultUnhealthyThreshold,
// 		Timeout:            defaultTimeout,
// 		CheckInterval:           defaultInterval,
// 		Path:             target,
// 	}
// 	for _, test := range tests {
// 		t.Run(test.name, func(t *testing.T) {
// 			oscServices := newMockedFakeOSCServices(TestClusterID)
// 			c, err := newOSCCloud(CloudConfig{}, oscServices)
// 			assert.Nil(t, err, "Error building osc cloud: %v", err)
// 			expectedHC := *defaultHC
// 			if test.overriddenFieldName != "" { // cater for test case with no overrides
// 				value := reflect.ValueOf(&test.overriddenValue)
// 				reflect.ValueOf(&expectedHC).Elem().FieldByName(test.overriddenFieldName).Set(value)
// 			}
// 			oscServices.lbu.(*MockedFakeLBU).expectConfigureHealthCheck(lbName, expectedHC, nil)
//
// 			err = c.ensureLoadBalancerHealthCheck(lbuDesc, protocol, port, path, test.annotations)
//
// 			require.Nil(t, err)
// 			oscServices.lbu.(*MockedFakeLBU).AssertExpectations(t)
// 		})
// 	}
//
// 	t.Run("does not make an API call if the current health check is the same", func(t *testing.T) {
// 		oscServices := newMockedFakeOSCServices(TestClusterID)
// 		c, err := newOSCCloud(CloudConfig{}, oscServices)
// 		assert.Nil(t, err, "Error building osc cloud: %v", err)
// 		expectedHC := *defaultHC
// 		timeout := int64(3)
// 		expectedHC.Timeout = timeout
// 		annotations := map[string]string{ServiceAnnotationLoadBalancerHCTimeout: "3"}
// 		var currentHC osc.HealthCheck
// 		currentHC = expectedHC
//
// 		// NOTE no call expectations are set on the LBU mock
// 		// test default HC
// 		lbuDesc := osc.LoadBalancer{LoadBalancerName: lbName, HealthCheck: defaultHC}
// 		err = c.ensureLoadBalancerHealthCheck(lbuDesc, protocol, port, path, map[string]string{})
// 		assert.Nil(t, err)
// 		// test HC with override
// 		lbuDesc = osc.LoadBalancer{LoadBalancerName: lbName, HealthCheck: currentHC}
// 		err = c.ensureLoadBalancerHealthCheck(lbuDesc, protocol, port, path, annotations)
// 		assert.Nil(t, err)
// 	})
//
// 	t.Run("validates resulting expected health check before making an API call", func(t *testing.T) {
// 		oscServices := newMockedFakeOSCServices(TestClusterID)
// 		c, err := newOSCCloud(CloudConfig{}, oscServices)
// 		assert.Nil(t, err, "Error building osc cloud: %v", err)
// 		expectedHC := *defaultHC
// 		invalidThreshold := int64(1)
// 		expectedHC.HealthyThreshold = invalidThreshold
// 		require.Error(t, expectedHC.Validate()) // confirm test precondition
// 		annotations := map[string]string{ServiceAnnotationLoadBalancerHCTimeout: "1"}
//
// 		// NOTE no call expectations are set on the LBU mock
// 		err = c.ensureLoadBalancerHealthCheck(lbuDesc, protocol, port, path, annotations)
//
// 		require.Error(t, err)
// 	})
//
// 	t.Run("handles invalid override values", func(t *testing.T) {
// 		oscServices := newMockedFakeOSCServices(TestClusterID)
// 		c, err := newOSCCloud(CloudConfig{}, oscServices)
// 		assert.Nil(t, err, "Error building osc cloud: %v", err)
// 		annotations := map[string]string{ServiceAnnotationLoadBalancerHCTimeout: "3.3"}
//
// 		// NOTE no call expectations are set on the LBU mock
// 		err = c.ensureLoadBalancerHealthCheck(lbuDesc, protocol, port, path, annotations)
//
// 		require.Error(t, err)
// 	})
//
// 	t.Run("returns error when updating the health check fails", func(t *testing.T) {
// 		oscServices := newMockedFakeOSCServices(TestClusterID)
// 		c, err := newOSCCloud(CloudConfig{}, oscServices)
// 		assert.Nil(t, err, "Error building osc cloud: %v", err)
// 		returnErr := fmt.Errorf("throttling error")
// 		//oscServices.lbu.(*MockedFakeLBU).expectConfigureHealthCheck(lbName, defaultHC, returnErr)
//
// 		err = c.ensureLoadBalancerHealthCheck(lbuDesc, protocol, port, path, map[string]string{})
//
// 		require.Error(t, err)
// 		oscServices.lbu.(*MockedFakeLBU).AssertExpectations(t)
// 	})
// }

func TestFindSecurityGroupForInstance(t *testing.T) {
	groups := map[string]osc.SecurityGroup{"sg123": {SecurityGroupId: "sg123"}}
	id, err := findSecurityGroupForInstance(osc.Vm{SecurityGroups: []osc.SecurityGroupLight{{SecurityGroupId: "sg123", SecurityGroupName: "my_group"}}}, groups)
	if err != nil {
		t.Error()
	}
	assert.Equal(t, id.SecurityGroupId, "sg123")
	assert.Equal(t, id.SecurityGroupName, "my_group")
}

func TestFindSecurityGroupForInstanceMultipleTagged(t *testing.T) {
	groups := map[string]osc.SecurityGroup{"sg123": {SecurityGroupId: "sg123"}}
	_, err := findSecurityGroupForInstance(osc.Vm{
		SecurityGroups: []osc.SecurityGroupLight{
			{SecurityGroupId: "sg123", SecurityGroupName: "my_group"},
			{SecurityGroupId: "sg123", SecurityGroupName: "another_group"},
		},
	}, groups)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sg123(my_group)")
	assert.Contains(t, err.Error(), "sg123(another_group)")
}

func TestRegionIsValid(t *testing.T) {
	fake := newMockedFakeOSCServices("fakeCluster")
	fake.selfInstance.Placement = osc.Placement{
		SubregionName: "pl-fake-999a",
	}

	// This is the legacy list that was removed, using this to ensure we avoid
	// region regressions if something goes wrong in the SDK
	regions := []string{
		"ap-northeast-1",
		"ap-northeast-2",
		"ap-northeast-3",
		"ap-south-1",
		"ap-southeast-1",
		"ap-southeast-2",
		"ca-central-1",
		"eu-central-1",
		"eu-west-1",
		"eu-west-2",
		"eu-west-3",
		"sa-east-1",
		"us-east-1",
		"us-east-2",
		"us-west-1",
		"us-west-2",
		"cn-north-1",
		"cn-northwest-1",
		"us-gov-west-1",
		"ap-northeast-3",

		// Ensures that we always trust what the metadata service returns
		"pl-fake-999",
	}

	for _, region := range regions {
		assert.True(t, isRegionValid(region, fake.metadata), "expected region '%s' to be valid but it was not", region)
	}

	assert.False(t, isRegionValid("pl-fake-991a", fake.metadata), "expected region 'pl-fake-991' to be invalid but it was not")
}

func TestNodeNameToProviderID(t *testing.T) {
	testNodeName := types.NodeName("ip-10-0-0-1.ec2.internal")
	testProviderID := "aws:///us-east-1c/i-02bce90670bb0c7cd"
	fakeOSC := newMockedFakeOSCServices(TestClusterID)
	c, err := newOSCCloud(CloudConfig{}, fakeOSC)
	assert.NoError(t, err)

	fakeClient := &fake.Clientset{}
	fakeInformerFactory := informers.NewSharedInformerFactory(fakeClient, 0)
	c.SetInformers(fakeInformerFactory)

	// no node name
	_, err = c.nodeNameToProviderID("")
	assert.Error(t, err)

	// informer has not synced
	c.nodeInformerHasSynced = informerNotSynced
	_, err = c.nodeNameToProviderID(testNodeName)
	assert.Error(t, err)

	// informer has synced but node not found
	c.nodeInformerHasSynced = informerSynced
	_, err = c.nodeNameToProviderID(testNodeName)
	assert.Error(t, err)

	// we are able to find the node in cache
	err = c.nodeInformer.Informer().GetStore().Add(&v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: string(testNodeName),
		},
		Spec: v1.NodeSpec{
			ProviderID: testProviderID,
		},
	})
	assert.NoError(t, err)
	_, err = c.nodeNameToProviderID(testNodeName)
	assert.NoError(t, err)
}

func informerSynced() bool {
	return true
}

func informerNotSynced() bool {
	return false
}

func newMockedFakeOSCServices(id string) *FakeOSCServices {
	s := NewFakeOSCServices(id)
	s.fcu = &MockedFakeFCU{FakeFCUImpl: s.fcu.(*FakeFCUImpl)}
	s.lbu = &MockedFakeLBU{FakeLBU: s.lbu.(*FakeLBU)}
	return s
}
