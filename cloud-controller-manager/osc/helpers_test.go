package osc_test

import (
	"testing"

	"github.com/aws/aws-sdk-go/service/elb"
	"github.com/outscale/cloud-provider-osc/cloud-controller-manager/osc/cloud"
	"github.com/outscale/cloud-provider-osc/cloud-controller-manager/osc/oapi"
	oapimocks "github.com/outscale/cloud-provider-osc/cloud-controller-manager/osc/oapi/mocks"
	sdk "github.com/outscale/osc-sdk-go/v2"
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
	sdkVM      = sdk.Vm{
		VmId:           ptr.To("i-foo"),
		PrivateDnsName: &vmNodeName,
		PrivateIp:      ptr.To("10.0.0.10"),
		Placement:      &sdk.Placement{SubregionName: &subRegion},
		Tags: &[]sdk.ResourceTag{{
			Key:   cloud.TagVmNodeName,
			Value: vmNodeName,
		}},
		SubnetId:       ptr.To("subnet-bar"),
		NetId:          ptr.To("net-bar"),
		SecurityGroups: &[]sdk.SecurityGroupLight{{SecurityGroupId: ptr.To("sg-worker")}, {SecurityGroupId: ptr.To("sg-node")}},
		State:          ptr.To("running"),
	}
	vmNode = v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: vmNodeName,
		},
	}
	selfNodeName = "10.0.0.11.eu-west-2.compute.internal"
	sdkSelf      = sdk.Vm{
		VmId:           ptr.To("i-bar"),
		PrivateDnsName: &selfNodeName,
		PrivateIp:      ptr.To("10.0.0.11"),
		Tags: &[]sdk.ResourceTag{{
			Key:   cloud.TagVmNodeName,
			Value: selfNodeName,
		}},
		Placement:      &sdk.Placement{SubregionName: &subRegion},
		SubnetId:       ptr.To("subnet-bar"),
		NetId:          ptr.To("net-bar"),
		SecurityGroups: &[]sdk.SecurityGroupLight{{SecurityGroupId: ptr.To("sg-controlplane")}, {SecurityGroupId: ptr.To("sg-node")}},
		State:          ptr.To("running"),
	}
	self = cloud.FromOscVm(&sdkSelf)
)

func expectVMs(mock *oapimocks.MockOAPI, vms ...sdk.Vm) {
	mock.EXPECT().
		ReadVms(gomock.Any(), gomock.Eq(sdk.ReadVmsRequest{
			Filters: &sdk.FiltersVm{
				TagKeys: &[]string{"OscK8sClusterID/foo"},
				VmStateNames: &[]string{
					"pending",
					"running",
					"stopping",
					"stopped",
					"shutting-down",
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
		ReadLoadBalancers(gomock.Any(), gomock.Eq(sdk.ReadLoadBalancersRequest{
			Filters: &sdk.FiltersLoadBalancer{
				LoadBalancerNames: &[]string{lbName},
			},
		})).
		Return(nil, nil)
}

func expectLoadbalancerExistsAndOwned(mock *oapimocks.MockOAPI, updates ...func(tag *sdk.ResourceTag)) {
	tags := []sdk.ResourceTag{{
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
		ReadLoadBalancers(gomock.Any(), gomock.Eq(sdk.ReadLoadBalancersRequest{
			Filters: &sdk.FiltersLoadBalancer{
				LoadBalancerNames: &[]string{lbName},
			},
		})).
		Return([]sdk.LoadBalancer{{
			Tags: &tags,
		}}, nil)
}

func expectLoadbalancerExistsAndNotOwned(mock *oapimocks.MockOAPI) {
	mock.EXPECT().
		ReadLoadBalancers(gomock.Any(), gomock.Eq(sdk.ReadLoadBalancersRequest{
			Filters: &sdk.FiltersLoadBalancer{
				LoadBalancerNames: &[]string{lbName},
			},
		})).
		Return([]sdk.LoadBalancer{{
			Tags: &[]sdk.ResourceTag{{
				Key: cloud.ClusterIDTagKeyPrefix + "bar",
			}, {
				Key:   cloud.ServiceNameTagKey,
				Value: "baz",
			}},
		}}, nil)
}

func expectReadLoadBalancer(mock *oapimocks.MockOAPI, updates ...func(*sdk.LoadBalancer)) {
	desc := sdk.LoadBalancer{
		LoadBalancerName: &lbName,
		SecurityGroups:   &[]string{"sg-foo"},
		Subnets:          &[]string{"subnet-service"},
		Listeners: &[]sdk.Listener{{
			LoadBalancerPort:     ptr.To[int32](80),
			LoadBalancerProtocol: ptr.To("TCP"),
			BackendPort:          ptr.To[int32](8080),
			BackendProtocol:      ptr.To("TCP"),
		}},
		HealthCheck: &sdk.HealthCheck{
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
		ReadLoadBalancers(gomock.Any(), gomock.Eq(sdk.ReadLoadBalancersRequest{
			Filters: &sdk.FiltersLoadBalancer{
				LoadBalancerNames: &[]string{lbName},
			},
		})).
		Return([]sdk.LoadBalancer{desc}, nil)
}

func expectReadLoadBalancerNoneFound(mock *oapimocks.MockOAPI) {
	mock.EXPECT().
		ReadLoadBalancers(gomock.Any(), gomock.Eq(sdk.ReadLoadBalancersRequest{
			Filters: &sdk.FiltersLoadBalancer{
				LoadBalancerNames: &[]string{lbName},
			},
		})).
		Return(nil, nil)
}

func expectCreateLoadBalancer(mock *oapimocks.MockOAPI, updates ...func(*sdk.CreateLoadBalancerRequest)) {
	req := sdk.CreateLoadBalancerRequest{
		LoadBalancerName: lbName,
		SecurityGroups:   &[]string{"sg-foo"},
		Subnets:          &[]string{"subnet-service"},
		Tags: &[]sdk.ResourceTag{{
			Key:   "OscK8sClusterID/foo",
			Value: "owned",
		}, {
			Key:   "OscK8sService",
			Value: "svc-foo",
		}},
		Listeners: []sdk.ListenerForCreation{{
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
		Return(&sdk.LoadBalancer{}, nil)
}

func expectDeleteLoadBalancer(mock *oapimocks.MockOAPI) {
	mock.EXPECT().
		DeleteLoadBalancer(gomock.Any(), gomock.Eq(sdk.DeleteLoadBalancerRequest{
			LoadBalancerName: lbName,
		})).
		Return(nil)
}

func expectConfigureHealthCheck(mock *oapimocks.MockOAPI, hc ...*sdk.HealthCheck) {
	req := sdk.UpdateLoadBalancerRequest{
		LoadBalancerName: "lb-foo",
	}
	if len(hc) > 0 {
		req.HealthCheck = hc[0]
	} else {
		req.HealthCheck = &sdk.HealthCheck{
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
		ReadSubnets(gomock.Any(), gomock.Eq(sdk.ReadSubnetsRequest{
			Filters: &sdk.FiltersSubnet{
				SubnetIds: &[]string{id},
			},
		})).
		Return([]sdk.Subnet{
			{SubnetId: ptr.To(id), NetId: ptr.To("net-foo")},
		}, nil)
}

func expectFindLBSubnet(mock *oapimocks.MockOAPI) {
	mock.EXPECT().
		ReadSubnets(gomock.Any(), gomock.Eq(sdk.ReadSubnetsRequest{
			Filters: &sdk.FiltersSubnet{
				TagKeys: &[]string{"OscK8sClusterID/foo"},
			},
		})).
		Return([]sdk.Subnet{
			{SubnetId: ptr.To("subnet-service"), NetId: ptr.To("net-foo"), Tags: &[]sdk.ResourceTag{{Key: "OscK8sRole/service"}}},
			{SubnetId: ptr.To("subnet-service.internal"), NetId: ptr.To("net-foo"), Tags: &[]sdk.ResourceTag{{Key: "OscK8sRole/service.internal"}}},
		}, nil)
}

func expectFindNoLBSubnet(mock *oapimocks.MockOAPI) {
	mock.EXPECT().
		ReadSubnets(gomock.Any(), gomock.Eq(sdk.ReadSubnetsRequest{
			Filters: &sdk.FiltersSubnet{
				TagKeys: &[]string{"OscK8sClusterID/foo"},
			},
		})).
		Return([]sdk.Subnet{
			{SubnetId: ptr.To("subnet-service"), NetId: ptr.To("net-foo"), Tags: &[]sdk.ResourceTag{}},
			{SubnetId: ptr.To("subnet-service.internal"), NetId: ptr.To("net-foo"), Tags: &[]sdk.ResourceTag{}},
		}, nil)
}

func expectSGAlreadyExists(mock *oapimocks.MockOAPI) {
	mock.EXPECT().
		CreateSecurityGroup(gomock.Any(), gomock.Eq(sdk.CreateSecurityGroupRequest{
			SecurityGroupName: "k8s-elb-lb-foo",
			Description:       "Security group for Kubernetes LB lb-foo (svc-foo)",
			NetId:             ptr.To("net-foo"),
		})).
		Return(nil, oapi.NewOAPIError(sdk.Errors{Code: ptr.To("9008")}))
	mock.EXPECT().
		ReadSecurityGroups(gomock.Any(), gomock.Eq(sdk.ReadSecurityGroupsRequest{
			Filters: &sdk.FiltersSecurityGroup{
				SecurityGroupNames: &[]string{"k8s-elb-lb-foo"},
			},
		})).
		Return([]sdk.SecurityGroup{{
			SecurityGroupId: ptr.To("sg-foo"),
			Tags:            &[]sdk.ResourceTag{{Key: "OscK8sClusterID/foo"}},
		}}, nil)
}

func expectCreateSecurityGroup(mock *oapimocks.MockOAPI) {
	mock.EXPECT().
		CreateSecurityGroup(gomock.Any(), gomock.Eq(sdk.CreateSecurityGroupRequest{
			SecurityGroupName: "k8s-elb-lb-foo",
			Description:       "Security group for Kubernetes LB lb-foo (svc-foo)",
			NetId:             ptr.To("net-foo"),
		})).
		Return(&sdk.SecurityGroup{SecurityGroupId: ptr.To("sg-foo")}, nil)
	mock.EXPECT().
		CreateTags(gomock.Any(), gomock.Eq(sdk.CreateTagsRequest{
			ResourceIds: []string{"sg-foo"},
			Tags: []sdk.ResourceTag{
				{Key: "OscK8sClusterID/foo", Value: cloud.ResourceLifecycleOwned},
			},
		})).
		Return(nil)
}

func expectFindIngressSecurityGroup(mock *oapimocks.MockOAPI, id string) {
	mock.EXPECT().
		ReadSecurityGroups(gomock.Any(), gomock.Eq(sdk.ReadSecurityGroupsRequest{
			Filters: &sdk.FiltersSecurityGroup{
				SecurityGroupIds: &[]string{id},
			},
		})).
		Return([]sdk.SecurityGroup{{SecurityGroupId: ptr.To(id)}}, nil)
}

func expectFindExistingIngressSecurityGroup(mock *oapimocks.MockOAPI, id string) {
	mock.EXPECT().
		ReadSecurityGroups(gomock.Any(), gomock.Eq(sdk.ReadSecurityGroupsRequest{
			Filters: &sdk.FiltersSecurityGroup{
				SecurityGroupIds: &[]string{id},
			},
		})).
		Return([]sdk.SecurityGroup{{
			SecurityGroupId: ptr.To(id),
			InboundRules: &[]sdk.SecurityGroupRule{{
				IpProtocol:    ptr.To("tcp"),
				FromPortRange: ptr.To[int32](80),
				ToPortRange:   ptr.To[int32](80),
				IpRanges:      &[]string{"0.0.0.0/0"},
			}},
		}}, nil)
}

func expectFindWorkerSGByRole(mock *oapimocks.MockOAPI) {
	mock.EXPECT().
		ReadSecurityGroups(gomock.Any(), gomock.Eq(sdk.ReadSecurityGroupsRequest{
			Filters: &sdk.FiltersSecurityGroup{
				SecurityGroupIds: &[]string{"sg-node", "sg-worker"},
			},
		})).
		Return([]sdk.SecurityGroup{
			{SecurityGroupId: ptr.To("sg-worker"), Tags: &[]sdk.ResourceTag{{Key: cloud.RoleTagKeyPrefix + "worker"}}},
			{SecurityGroupId: ptr.To("sg-controlplane"), Tags: &[]sdk.ResourceTag{{Key: cloud.RoleTagKeyPrefix + "controlplane"}}},
			{SecurityGroupId: ptr.To("sg-node"), Tags: &[]sdk.ResourceTag{{Key: cloud.RoleTagKeyPrefix + "worker"}, {Key: cloud.RoleTagKeyPrefix + "controlplane"}}},
		}, nil)
}

func expectFindExistingWorkerSG(mock *oapimocks.MockOAPI) {
	mock.EXPECT().
		ReadSecurityGroups(gomock.Any(), gomock.Eq(sdk.ReadSecurityGroupsRequest{
			Filters: &sdk.FiltersSecurityGroup{
				SecurityGroupIds: &[]string{"sg-node", "sg-worker"},
			},
		})).
		Return([]sdk.SecurityGroup{
			{SecurityGroupId: ptr.To("sg-worker"), Tags: &[]sdk.ResourceTag{{Key: cloud.RoleTagKeyPrefix + "worker"}},
				InboundRules: &[]sdk.SecurityGroupRule{
					{IpProtocol: ptr.To("tcp"), FromPortRange: ptr.To[int32](8080), ToPortRange: ptr.To[int32](8080), SecurityGroupsMembers: &[]sdk.SecurityGroupsMember{{
						SecurityGroupId: ptr.To("sg-foo"),
					}}},
				}},
			{SecurityGroupId: ptr.To("sg-controlplane"), Tags: &[]sdk.ResourceTag{{Key: cloud.RoleTagKeyPrefix + "controlplane"}}},
			{SecurityGroupId: ptr.To("sg-node"), Tags: &[]sdk.ResourceTag{{Key: cloud.RoleTagKeyPrefix + "worker"}, {Key: cloud.RoleTagKeyPrefix + "controlplane"}}},
		}, nil)
}

func expectAddIngressSGRule(mock *oapimocks.MockOAPI, ipRanges []string, dstSG string, updates ...func(req *sdk.CreateSecurityGroupRuleRequest)) {
	req := sdk.CreateSecurityGroupRuleRequest{
		SecurityGroupId: dstSG,
		Flow:            "Inbound",
		Rules: &[]sdk.SecurityGroupRule{{
			IpProtocol:    ptr.To("tcp"),
			FromPortRange: ptr.To[int32](80),
			ToPortRange:   ptr.To[int32](80),
			IpRanges:      &ipRanges,
		}},
	}
	for _, update := range updates {
		update(&req)
	}
	mock.EXPECT().
		CreateSecurityGroupRule(gomock.Any(), gomock.Eq(req)).Return(&sdk.SecurityGroup{}, nil)
}

func expectDeleteIngressSGRule(mock *oapimocks.MockOAPI, ipRanges []string, dstSG string) {
	req := sdk.DeleteSecurityGroupRuleRequest{
		SecurityGroupId: dstSG,
		Flow:            "Inbound",
		Rules: &[]sdk.SecurityGroupRule{{
			IpProtocol:    ptr.To("tcp"),
			FromPortRange: ptr.To[int32](80),
			ToPortRange:   ptr.To[int32](80),
			IpRanges:      &ipRanges,
		}},
	}
	mock.EXPECT().
		DeleteSecurityGroupRule(gomock.Any(), gomock.Eq(req)).Return(&sdk.SecurityGroup{}, nil)
}

func expectAddInternalSGRule(mock *oapimocks.MockOAPI, srcSG, dstSG string, updates ...func(req *sdk.CreateSecurityGroupRuleRequest)) {
	req := sdk.CreateSecurityGroupRuleRequest{
		SecurityGroupId: dstSG,
		Flow:            "Inbound",
		Rules: &[]sdk.SecurityGroupRule{{
			IpProtocol:            ptr.To("tcp"),
			FromPortRange:         ptr.To[int32](8080),
			ToPortRange:           ptr.To[int32](8080),
			SecurityGroupsMembers: &[]sdk.SecurityGroupsMember{{SecurityGroupId: &srcSG}},
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
		RegisterVmsInLoadBalancer(gomock.Any(), gomock.Eq(sdk.RegisterVmsInLoadBalancerRequest{
			BackendVmIds:     vmIds,
			LoadBalancerName: "lb-foo",
		})).
		Return(nil)
}

func expectDeregisterInstances(mock *oapimocks.MockOAPI, vmIds ...string) {
	mock.EXPECT().
		DeregisterVmsInLoadBalancer(gomock.Any(), gomock.Eq(sdk.DeregisterVmsInLoadBalancerRequest{
			BackendVmIds:     vmIds,
			LoadBalancerName: "lb-foo",
		})).
		Return(nil)
}

func expectDeleteListener(mock *oapimocks.MockOAPI) {
	mock.EXPECT().
		DeleteLoadBalancerListeners(gomock.Any(), gomock.Eq(sdk.DeleteLoadBalancerListenersRequest{
			LoadBalancerName:  "lb-foo",
			LoadBalancerPorts: []int32{80},
		})).
		Return(&sdk.LoadBalancer{}, nil)
}

func expectCreateListener(mock *oapimocks.MockOAPI, port int32) {
	mock.EXPECT().
		CreateLoadBalancerListeners(gomock.Any(), gomock.Eq(sdk.CreateLoadBalancerListenersRequest{
			LoadBalancerName: "lb-foo",
			Listeners: []sdk.ListenerForCreation{{
				LoadBalancerPort:     port,
				LoadBalancerProtocol: "TCP",
				BackendPort:          8080,
				BackendProtocol:      ptr.To("TCP"),
			}},
		})).
		Return(&sdk.LoadBalancer{}, nil)
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

func expectCreateTag(mock *oapimocks.MockOAPI, id string, tag sdk.ResourceTag) {
	mock.EXPECT().
		CreateTags(gomock.Any(), gomock.Eq(sdk.CreateTagsRequest{
			ResourceIds: []string{id},
			Tags:        []sdk.ResourceTag{tag},
		})).
		Return(nil)
}

func expectPurgeSecurityGroups(mock *oapimocks.MockOAPI) {
	mock.EXPECT().
		ReadSecurityGroups(gomock.Any(), gomock.Eq(sdk.ReadSecurityGroupsRequest{
			Filters: &sdk.FiltersSecurityGroup{
				TagKeys: &[]string{
					cloud.ClusterIDTagKeyPrefix + "foo",
				},
			},
		})).
		Return([]sdk.SecurityGroup{
			{
				SecurityGroupId: ptr.To("sg-foo"),
				Tags:            &[]sdk.ResourceTag{{Key: cloud.SGToDeleteTagKey}},
			},
			{
				SecurityGroupId: ptr.To("sg-bar"),
				InboundRules: &[]sdk.SecurityGroupRule{
					{IpProtocol: ptr.To("-1"), IpRanges: &[]string{"0.0.0.0/0"}},
					{IpProtocol: ptr.To("tcp"), FromPortRange: ptr.To[int32](8080), ToPortRange: ptr.To[int32](8080), SecurityGroupsMembers: &[]sdk.SecurityGroupsMember{{SecurityGroupId: ptr.To("sg-foo")}}},
				},
			},
		}, nil)
	mock.EXPECT().
		DeleteSecurityGroupRule(gomock.Any(), gomock.Eq(sdk.DeleteSecurityGroupRuleRequest{
			SecurityGroupId: "sg-bar",
			Flow:            "Inbound",
			Rules: &[]sdk.SecurityGroupRule{{
				IpProtocol: ptr.To("tcp"), FromPortRange: ptr.To[int32](8080), ToPortRange: ptr.To[int32](8080),
				SecurityGroupsMembers: &[]sdk.SecurityGroupsMember{{SecurityGroupId: ptr.To("sg-foo")}},
			}},
		})).
		Return(&sdk.SecurityGroup{}, nil)
	mock.EXPECT().
		DeleteSecurityGroup(gomock.Any(), gomock.Eq(sdk.DeleteSecurityGroupRequest{
			SecurityGroupId: ptr.To("sg-foo"),
		})).
		Return(nil)
}

func expectPublicIPFromPool(mock *oapimocks.MockOAPI, ips []sdk.PublicIp) {
	mock.EXPECT().
		ListPublicIpsFromPool(gomock.Any(), gomock.Eq("pool-foo")).
		Return(ips, nil)
}
