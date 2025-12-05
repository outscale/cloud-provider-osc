package ccm_test

import (
	"testing"

	"github.com/aws/aws-sdk-go/service/elb"
	"github.com/outscale/cloud-provider-osc/ccm/cloud"
	"github.com/outscale/cloud-provider-osc/ccm/oapi"
	oapimocks "github.com/outscale/cloud-provider-osc/ccm/oapi/mocks"
	"github.com/outscale/osc-sdk-go/v3/pkg/osc"
	"go.uber.org/mock/gomock"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

type MockOAPIClient struct {
	oapi *oapimocks.MockOAPI
	lb   *oapimocks.MockLBU
}

func (m MockOAPIClient) OAPI() oapi.OAPI {
	return m.oapi
}

func (m MockOAPIClient) LBU() oapi.LBU {
	return m.lb
}

func newAPI(t *testing.T, self *cloud.VM, clusterID string) (*cloud.Cloud, *oapimocks.MockOAPI, *oapimocks.MockLBU) {
	ctrl := gomock.NewController(t)
	oa := oapimocks.NewMockOAPI(ctrl)
	lb := oapimocks.NewMockLBU(ctrl)
	c := cloud.NewWith(MockOAPIClient{oapi: oa, lb: lb}, self, clusterID)
	return c, oa, lb
}

var (
	subRegion = "eu-west-2a"

	vmNodeName = "10.0.0.10.eu-west-2.compute.internal"
	sdkVM      = osc.Vm{
		VmId:           "i-foo",
		VmType:         "tinav3.c1r1p1",
		PrivateDnsName: &vmNodeName,
		PrivateIp:      "10.0.0.10",
		Placement:      osc.Placement{SubregionName: subRegion},
		Tags: []osc.ResourceTag{{
			Key:   cloud.TagVmNodeName,
			Value: vmNodeName,
		}},
		SubnetId:       ptr.To("subnet-bar"),
		NetId:          ptr.To("net-bar"),
		SecurityGroups: []osc.SecurityGroupLight{{SecurityGroupId: "sg-worker"}, {SecurityGroupId: "sg-node"}},
		State:          osc.VmStateRunning,
	}
	vmNode = v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: vmNodeName,
		},
	}
	selfNodeName = "10.0.0.11.eu-west-2.compute.internal"
	sdkSelf      = osc.Vm{
		VmId:           "i-bar",
		VmType:         "tinav3.c1r1p1",
		PrivateDnsName: &selfNodeName,
		PrivateIp:      "10.0.0.11",
		Tags: []osc.ResourceTag{{
			Key:   cloud.TagVmNodeName,
			Value: selfNodeName,
		}},
		Placement:      osc.Placement{SubregionName: subRegion},
		NetId:          ptr.To("net-bar"),
		SubnetId:       ptr.To("subnet-bar"),
		SecurityGroups: []osc.SecurityGroupLight{{SecurityGroupId: "sg-controlplane"}, {SecurityGroupId: "sg-node"}},
		State:          osc.VmStateRunning,
	}
	self = cloud.FromOscVm(&sdkSelf)
)

func expectVMs(mock *oapimocks.MockOAPI, vms ...osc.Vm) {
	mock.EXPECT().
		ReadVms(gomock.Any(), gomock.Eq(osc.ReadVmsRequest{
			Filters: &osc.FiltersVm{
				TagKeys: &[]string{"OscK8sClusterID/foo"},
				VmStateNames: &[]osc.VmState{
					osc.VmStatePending,
					osc.VmStateRunning,
					osc.VmStateStopping,
					osc.VmStateStopped,
					osc.VmStateShuttingDown,
				},
			},
		})).
		Return(vms, nil)
}

var (
	lbName  = "lb-foo"
	svcName = "svc-foo"
)

func expectNoLoadbalancer(mock *oapimocks.MockOAPI) {
	mock.EXPECT().
		ReadLoadBalancers(gomock.Any(), gomock.Eq(osc.ReadLoadBalancersRequest{
			Filters: &osc.FiltersLoadBalancer{
				LoadBalancerNames: &[]string{lbName},
			},
		})).
		Return(nil, nil)
}

func expectLoadbalancerExistsAndOwned(mock *oapimocks.MockOAPI, updates ...func(tag *osc.ResourceTag)) {
	tags := []osc.ResourceTag{{
		Key: cloud.ClusterIDTagKeyPrefix + "foo",
	}, {
		Key:   cloud.ServiceNameTagKey,
		Value: svcName,
	}}
	for i := range tags {
		for _, update := range updates {
			update(&tags[i])
		}
	}
	mock.EXPECT().
		ReadLoadBalancers(gomock.Any(), gomock.Eq(osc.ReadLoadBalancersRequest{
			Filters: &osc.FiltersLoadBalancer{
				LoadBalancerNames: &[]string{lbName},
			},
		})).
		Return([]osc.LoadBalancer{{
			Tags: tags,
		}}, nil)
}

func expectLoadbalancerExistsAndNotOwned(mock *oapimocks.MockOAPI) {
	mock.EXPECT().
		ReadLoadBalancers(gomock.Any(), gomock.Eq(osc.ReadLoadBalancersRequest{
			Filters: &osc.FiltersLoadBalancer{
				LoadBalancerNames: &[]string{lbName},
			},
		})).
		Return([]osc.LoadBalancer{{
			Tags: []osc.ResourceTag{{
				Key: cloud.ClusterIDTagKeyPrefix + "bar",
			}, {
				Key:   cloud.ServiceNameTagKey,
				Value: "baz",
			}},
		}}, nil)
}

func expectReadLoadBalancer(mock *oapimocks.MockOAPI, updates ...func(*osc.LoadBalancer)) {
	desc := osc.LoadBalancer{
		LoadBalancerName: lbName,
		SecurityGroups:   []string{"sg-foo"},
		Subnets:          []string{"subnet-service"},
		Listeners: []osc.Listener{{
			LoadBalancerPort:     80,
			LoadBalancerProtocol: "TCP",
			BackendPort:          8080,
			BackendProtocol:      "TCP",
		}},
		HealthCheck: osc.HealthCheck{
			HealthyThreshold:   2,
			CheckInterval:      10,
			Timeout:            5,
			UnhealthyThreshold: 3,
		},
	}
	for _, update := range updates {
		update(&desc)
	}
	mock.EXPECT().
		ReadLoadBalancers(gomock.Any(), gomock.Eq(osc.ReadLoadBalancersRequest{
			Filters: &osc.FiltersLoadBalancer{
				LoadBalancerNames: &[]string{lbName},
			},
		})).
		Return([]osc.LoadBalancer{desc}, nil)
}

func expectReadLoadBalancerNoneFound(mock *oapimocks.MockOAPI) {
	mock.EXPECT().
		ReadLoadBalancers(gomock.Any(), gomock.Eq(osc.ReadLoadBalancersRequest{
			Filters: &osc.FiltersLoadBalancer{
				LoadBalancerNames: &[]string{lbName},
			},
		})).
		Return(nil, nil)
}

func expectCreateLoadBalancer(mock *oapimocks.MockOAPI, updates ...func(*osc.CreateLoadBalancerRequest)) {
	req := osc.CreateLoadBalancerRequest{
		LoadBalancerName: lbName,
		SecurityGroups:   &[]string{"sg-foo"},
		Subnets:          &[]string{"subnet-service"},
		Tags: &[]osc.ResourceTag{{
			Key:   "OscK8sClusterID/foo",
			Value: "owned",
		}, {
			Key:   "OscK8sService",
			Value: "svc-foo",
		}},
		Listeners: []osc.ListenerForCreation{{
			LoadBalancerPort:     80,
			LoadBalancerProtocol: "TCP",
			BackendPort:          8080,
			BackendProtocol:      ptr.To("TCP"),
		}},
	}
	for _, update := range updates {
		update(&req)
	}
	mock.EXPECT().
		CreateLoadBalancer(gomock.Any(), gomock.Eq(req)).
		Return(&osc.LoadBalancer{}, nil)
}

func expectDeleteLoadBalancer(mock *oapimocks.MockOAPI) {
	mock.EXPECT().
		DeleteLoadBalancer(gomock.Any(), gomock.Eq(osc.DeleteLoadBalancerRequest{
			LoadBalancerName: lbName,
		})).
		Return(nil)
}

func expectConfigureHealthCheck(mock *oapimocks.MockOAPI, hc ...*osc.HealthCheck) {
	req := osc.UpdateLoadBalancerRequest{
		LoadBalancerName: "lb-foo",
	}
	if len(hc) > 0 {
		req.HealthCheck = hc[0]
	} else {
		req.HealthCheck = &osc.HealthCheck{
			Port:               8080,
			Protocol:           "TCP",
			HealthyThreshold:   2,
			CheckInterval:      10,
			Timeout:            5,
			UnhealthyThreshold: 3,
		}
	}
	mock.EXPECT().
		UpdateLoadBalancer(gomock.Any(), gomock.Eq(req)).
		Return(nil)
}

func expectDescribeProxyProtocol(mock *oapimocks.MockLBU, set bool, ports ...int64) {
	if ports == nil {
		ports = []int64{8080}
	}
	out := &elb.DescribeLoadBalancersOutput{
		LoadBalancerDescriptions: []*elb.LoadBalancerDescription{{
			ListenerDescriptions: []*elb.ListenerDescription{},
		}},
	}
	for _, port := range ports {
		l := &elb.ListenerDescription{
			Listener: &elb.Listener{
				InstancePort: &port,
			},
		}
		if set {
			l.PolicyNames = []*string{ptr.To("k8s-proxyprotocol-enabled")}
		}
		out.LoadBalancerDescriptions[0].ListenerDescriptions = append(out.LoadBalancerDescriptions[0].ListenerDescriptions, l)
	}
	mock.EXPECT().
		DescribeLoadBalancersWithContext(gomock.Any(), gomock.Eq(&elb.DescribeLoadBalancersInput{
			LoadBalancerNames: []*string{ptr.To("lb-foo")},
		})).
		Return(out, nil)
}

func expectConfigureProxyProtocol(mock *oapimocks.MockLBU, set, need bool, ports ...int64) {
	if !set {
		mock.EXPECT().
			CreateLoadBalancerPolicyWithContext(gomock.Any(), gomock.Eq(&elb.CreateLoadBalancerPolicyInput{
				PolicyAttributes: []*elb.PolicyAttribute{{
					AttributeName:  ptr.To("ProxyProtocol"),
					AttributeValue: ptr.To("true"),
				}},
				PolicyName:       ptr.To("k8s-proxyprotocol-enabled"),
				PolicyTypeName:   ptr.To("ProxyProtocolPolicyType"),
				LoadBalancerName: ptr.To("lb-foo"),
			})).
			Return(&elb.CreateLoadBalancerPolicyOutput{}, nil)
	}
	if ports == nil {
		ports = []int64{8080}
	}
	var policies []*string
	if need {
		policies = []*string{ptr.To("k8s-proxyprotocol-enabled")}
	} else {
		policies = []*string{}
	}
	for _, port := range ports {
		mock.EXPECT().
			SetLoadBalancerPoliciesForBackendServerWithContext(gomock.Any(), gomock.Eq(&elb.SetLoadBalancerPoliciesForBackendServerInput{
				InstancePort:     ptr.To[int64](port),
				PolicyNames:      policies,
				LoadBalancerName: ptr.To("lb-foo"),
			})).
			Return(&elb.SetLoadBalancerPoliciesForBackendServerOutput{}, nil)
	}
}

func expectFindExistingSubnet(mock *oapimocks.MockOAPI, id string) {
	mock.EXPECT().
		ReadSubnets(gomock.Any(), gomock.Eq(osc.ReadSubnetsRequest{
			Filters: &osc.FiltersSubnet{
				SubnetIds: &[]string{id},
			},
		})).
		Return([]osc.Subnet{
			{SubnetId: id, NetId: "net-foo"},
		}, nil)
}

func expectFindLBSubnet(mock *oapimocks.MockOAPI) {
	mock.EXPECT().
		ReadSubnets(gomock.Any(), gomock.Eq(osc.ReadSubnetsRequest{
			Filters: &osc.FiltersSubnet{
				TagKeys: &[]string{"OscK8sClusterID/foo"},
			},
		})).
		Return([]osc.Subnet{
			{SubnetId: "subnet-service", NetId: "net-foo", Tags: []osc.ResourceTag{{Key: "OscK8sRole/service"}}},
			{SubnetId: "subnet-service.internal", NetId: "net-foo", Tags: []osc.ResourceTag{{Key: "OscK8sRole/service.internal"}}},
		}, nil)
}

func expectFindNoLBSubnet(mock *oapimocks.MockOAPI) {
	mock.EXPECT().
		ReadSubnets(gomock.Any(), gomock.Eq(osc.ReadSubnetsRequest{
			Filters: &osc.FiltersSubnet{
				TagKeys: &[]string{"OscK8sClusterID/foo"},
			},
		})).
		Return([]osc.Subnet{
			{SubnetId: "subnet-service", NetId: "net-foo", Tags: []osc.ResourceTag{}},
			{SubnetId: "subnet-service.internal", NetId: "net-foo", Tags: []osc.ResourceTag{}},
		}, nil)
}

func expectSGAlreadyExists(mock *oapimocks.MockOAPI) {
	mock.EXPECT().
		CreateSecurityGroup(gomock.Any(), gomock.Eq(osc.CreateSecurityGroupRequest{
			SecurityGroupName: "k8s-elb-lb-foo",
			Description:       "Security group for Kubernetes LB lb-foo (svc-foo)",
			NetId:             ptr.To("net-foo"),
		})).
		Return(nil, &osc.ErrorResponse{Errors: []osc.Errors{{Code: "9008"}}})
	mock.EXPECT().
		ReadSecurityGroups(gomock.Any(), gomock.Eq(osc.ReadSecurityGroupsRequest{
			Filters: &osc.FiltersSecurityGroup{
				SecurityGroupNames: &[]string{"k8s-elb-lb-foo"},
			},
		})).
		Return([]osc.SecurityGroup{{
			SecurityGroupId: "sg-foo",
			Tags:            []osc.ResourceTag{{Key: "OscK8sClusterID/foo"}},
		}}, nil)
}

func expectCreateSecurityGroup(mock *oapimocks.MockOAPI) {
	mock.EXPECT().
		CreateSecurityGroup(gomock.Any(), gomock.Eq(osc.CreateSecurityGroupRequest{
			SecurityGroupName: "k8s-elb-lb-foo",
			Description:       "Security group for Kubernetes LB lb-foo (svc-foo)",
			NetId:             ptr.To("net-foo"),
		})).
		Return(&osc.SecurityGroup{SecurityGroupId: "sg-foo"}, nil)
	mock.EXPECT().
		CreateTags(gomock.Any(), gomock.Eq(osc.CreateTagsRequest{
			ResourceIds: []string{"sg-foo"},
			Tags: []osc.ResourceTag{
				{Key: "OscK8sClusterID/foo", Value: cloud.ResourceLifecycleOwned},
			},
		})).
		Return(nil)
}

func expectFindIngressSecurityGroup(mock *oapimocks.MockOAPI, id string) {
	mock.EXPECT().
		ReadSecurityGroups(gomock.Any(), gomock.Eq(osc.ReadSecurityGroupsRequest{
			Filters: &osc.FiltersSecurityGroup{
				SecurityGroupIds: &[]string{id},
			},
		})).
		Return([]osc.SecurityGroup{{
			SecurityGroupId: id,
			InboundRules:    []osc.SecurityGroupRule{},
		}}, nil)
}

func expectFindExistingIngressSecurityGroup(mock *oapimocks.MockOAPI, id string) {
	mock.EXPECT().
		ReadSecurityGroups(gomock.Any(), gomock.Eq(osc.ReadSecurityGroupsRequest{
			Filters: &osc.FiltersSecurityGroup{
				SecurityGroupIds: &[]string{id},
			},
		})).
		Return([]osc.SecurityGroup{{
			SecurityGroupId: id,
			InboundRules: []osc.SecurityGroupRule{{
				IpProtocol:    "tcp",
				FromPortRange: 80,
				ToPortRange:   80,
				IpRanges:      []string{"0.0.0.0/0"},
			}},
		}}, nil)
}

func expectFindWorkerSGByRole(mock *oapimocks.MockOAPI) {
	mock.EXPECT().
		ReadSecurityGroups(gomock.Any(), gomock.Eq(osc.ReadSecurityGroupsRequest{
			Filters: &osc.FiltersSecurityGroup{
				SecurityGroupIds: &[]string{"sg-node", "sg-worker"},
			},
		})).
		Return([]osc.SecurityGroup{
			{SecurityGroupId: "sg-worker", Tags: []osc.ResourceTag{{Key: cloud.RoleTagKeyPrefix + "worker"}}},
			{SecurityGroupId: "sg-controlplane", Tags: []osc.ResourceTag{{Key: cloud.RoleTagKeyPrefix + "controlplane"}}},
			{SecurityGroupId: "sg-node", Tags: []osc.ResourceTag{{Key: cloud.RoleTagKeyPrefix + "worker"}, {Key: cloud.RoleTagKeyPrefix + "controlplane"}}},
		}, nil)
}

func expectFindExistingWorkerSG(mock *oapimocks.MockOAPI) {
	mock.EXPECT().
		ReadSecurityGroups(gomock.Any(), gomock.Eq(osc.ReadSecurityGroupsRequest{
			Filters: &osc.FiltersSecurityGroup{
				SecurityGroupIds: &[]string{"sg-node", "sg-worker"},
			},
		})).
		Return([]osc.SecurityGroup{
			{SecurityGroupId: "sg-worker", Tags: []osc.ResourceTag{{Key: cloud.RoleTagKeyPrefix + "worker"}},
				InboundRules: []osc.SecurityGroupRule{
					{IpProtocol: "tcp", FromPortRange: 8080, ToPortRange: 8080, SecurityGroupsMembers: []osc.SecurityGroupsMember{{
						SecurityGroupId: "sg-foo",
					}}},
				}},
			{SecurityGroupId: "sg-controlplane", Tags: []osc.ResourceTag{{Key: cloud.RoleTagKeyPrefix + "controlplane"}}},
			{SecurityGroupId: "sg-node", Tags: []osc.ResourceTag{{Key: cloud.RoleTagKeyPrefix + "worker"}, {Key: cloud.RoleTagKeyPrefix + "controlplane"}}},
		}, nil)
}

func expectAddIngressSGRule(mock *oapimocks.MockOAPI, ipRanges []string, dstSG string, updates ...func(req *osc.CreateSecurityGroupRuleRequest)) {
	req := osc.CreateSecurityGroupRuleRequest{
		SecurityGroupId: dstSG,
		Flow:            "Inbound",
		Rules: []osc.SecurityGroupRule{{
			IpProtocol:    "tcp",
			FromPortRange: 80,
			ToPortRange:   80,
			IpRanges:      ipRanges,
		}},
	}
	for _, update := range updates {
		update(&req)
	}
	mock.EXPECT().
		CreateSecurityGroupRule(gomock.Any(), gomock.Eq(req)).Return(&osc.SecurityGroup{}, nil)
}

func expectDeleteIngressSGRule(mock *oapimocks.MockOAPI, ipRanges []string, dstSG string) {
	req := osc.DeleteSecurityGroupRuleRequest{
		SecurityGroupId: dstSG,
		Flow:            "Inbound",
		Rules: []osc.SecurityGroupRule{{
			IpProtocol:    "tcp",
			FromPortRange: 80,
			ToPortRange:   80,
			IpRanges:      ipRanges,
		}},
	}
	mock.EXPECT().
		DeleteSecurityGroupRule(gomock.Any(), gomock.Eq(req)).Return(&osc.SecurityGroup{}, nil)
}

func expectAddInternalSGRule(mock *oapimocks.MockOAPI, srcSG, dstSG string, updates ...func(req *osc.CreateSecurityGroupRuleRequest)) {
	req := osc.CreateSecurityGroupRuleRequest{
		SecurityGroupId: dstSG,
		Flow:            "Inbound",
		Rules: []osc.SecurityGroupRule{{
			IpProtocol:            "tcp",
			FromPortRange:         8080,
			ToPortRange:           8080,
			SecurityGroupsMembers: []osc.SecurityGroupsMember{{SecurityGroupId: srcSG}},
		}},
	}
	for _, update := range updates {
		update(&req)
	}
	mock.EXPECT().
		CreateSecurityGroupRule(gomock.Any(), gomock.Eq(req))
}

func expectRegisterInstances(mock *oapimocks.MockOAPI, vmIds ...string) {
	mock.EXPECT().
		RegisterVmsInLoadBalancer(gomock.Any(), gomock.Eq(osc.RegisterVmsInLoadBalancerRequest{
			BackendVmIds:     vmIds,
			LoadBalancerName: "lb-foo",
		})).
		Return(nil)
}

func expectDeregisterInstances(mock *oapimocks.MockOAPI, vmIds ...string) {
	mock.EXPECT().
		DeregisterVmsInLoadBalancer(gomock.Any(), gomock.Eq(osc.DeregisterVmsInLoadBalancerRequest{
			BackendVmIds:     vmIds,
			LoadBalancerName: "lb-foo",
		})).
		Return(nil)
}

func expectDeleteListener(mock *oapimocks.MockOAPI) {
	mock.EXPECT().
		DeleteLoadBalancerListeners(gomock.Any(), gomock.Eq(osc.DeleteLoadBalancerListenersRequest{
			LoadBalancerName:  "lb-foo",
			LoadBalancerPorts: []int{80},
		})).
		Return(&osc.LoadBalancer{}, nil)
}

func expectCreateListener(mock *oapimocks.MockOAPI, port int) {
	mock.EXPECT().
		CreateLoadBalancerListeners(gomock.Any(), gomock.Eq(osc.CreateLoadBalancerListenersRequest{
			LoadBalancerName: "lb-foo",
			Listeners: []osc.ListenerForCreation{{
				LoadBalancerPort:     port,
				LoadBalancerProtocol: "TCP",
				BackendPort:          8080,
				BackendProtocol:      ptr.To("TCP"),
			}},
		})).
		Return(&osc.LoadBalancer{}, nil)
}

func expectDescribeLoadBalancerAttributes(mock *oapimocks.MockLBU) {
	mock.EXPECT().DescribeLoadBalancerAttributesWithContext(gomock.Any(), gomock.Eq(&elb.DescribeLoadBalancerAttributesInput{
		LoadBalancerName: ptr.To("lb-foo"),
	})).
		Return(&elb.DescribeLoadBalancerAttributesOutput{
			LoadBalancerAttributes: &elb.LoadBalancerAttributes{
				AccessLog: &elb.AccessLog{
					Enabled: ptr.To(false),
				},
				ConnectionDraining: &elb.ConnectionDraining{
					Enabled: ptr.To(false),
				},
				ConnectionSettings: &elb.ConnectionSettings{
					IdleTimeout: ptr.To[int64](60),
				},
			},
		}, nil)
}

func expectModifyLoadBalancerAttributes(mock *oapimocks.MockLBU, attrs *elb.LoadBalancerAttributes) {
	mock.EXPECT().ModifyLoadBalancerAttributesWithContext(gomock.Any(), gomock.Eq(&elb.ModifyLoadBalancerAttributesInput{
		LoadBalancerName:       ptr.To("lb-foo"),
		LoadBalancerAttributes: attrs,
	})).
		Return(&elb.ModifyLoadBalancerAttributesOutput{}, nil)
}

func expectCreateTag(mock *oapimocks.MockOAPI, id string, tag osc.ResourceTag) {
	mock.EXPECT().
		CreateTags(gomock.Any(), gomock.Eq(osc.CreateTagsRequest{
			ResourceIds: []string{id},
			Tags:        []osc.ResourceTag{tag},
		})).
		Return(nil)
}

func expectPurgeSecurityGroups(mock *oapimocks.MockOAPI) {
	mock.EXPECT().
		ReadSecurityGroups(gomock.Any(), gomock.Eq(osc.ReadSecurityGroupsRequest{
			Filters: &osc.FiltersSecurityGroup{
				TagKeys: &[]string{
					cloud.ClusterIDTagKeyPrefix + "foo",
				},
			},
		})).
		Return([]osc.SecurityGroup{
			{
				SecurityGroupId: "sg-foo",
				Tags:            []osc.ResourceTag{{Key: cloud.SGToDeleteTagKey}},
			},
			{
				SecurityGroupId: "sg-bar",
				InboundRules: []osc.SecurityGroupRule{
					{IpProtocol: "-1", IpRanges: []string{"0.0.0.0/0"}},
					{IpProtocol: "tcp", FromPortRange: 8080, ToPortRange: 8080, SecurityGroupsMembers: []osc.SecurityGroupsMember{{SecurityGroupId: "sg-foo"}}},
				},
			},
		}, nil)
	mock.EXPECT().
		DeleteSecurityGroupRule(gomock.Any(), gomock.Eq(osc.DeleteSecurityGroupRuleRequest{
			SecurityGroupId: "sg-bar",
			Flow:            "Inbound",
			Rules: []osc.SecurityGroupRule{{
				IpProtocol: "tcp", FromPortRange: 8080, ToPortRange: 8080,
				SecurityGroupsMembers: []osc.SecurityGroupsMember{{SecurityGroupId: "sg-foo"}},
			}},
		})).
		Return(&osc.SecurityGroup{}, nil)
	mock.EXPECT().
		DeleteSecurityGroup(gomock.Any(), gomock.Eq(osc.DeleteSecurityGroupRequest{
			SecurityGroupId: ptr.To("sg-foo"),
		})).
		Return(nil)
}

func expectPublicIPFromPool(mock *oapimocks.MockOAPI, ips []osc.PublicIp) {
	mock.EXPECT().
		ListPublicIpsFromPool(gomock.Any(), gomock.Eq("pool-foo")).
		Return(ips, nil)
}

func expectPublicIP(mock *oapimocks.MockOAPI, id string, ip *osc.PublicIp) {
	mock.EXPECT().
		GetPublicIp(gomock.Any(), gomock.Eq(id)).
		Return(ip, nil)
}
