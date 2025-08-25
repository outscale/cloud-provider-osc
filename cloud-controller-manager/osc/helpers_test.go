package osc_test

import (
	"testing"

	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/elb"
	"github.com/outscale/cloud-provider-osc/cloud-controller-manager/osc"
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
	lb   *oapimocks.MockLoadBalancer
}

func (m MockOAPIClient) OAPI() oapi.OAPI {
	return m.oapi
}

func (m MockOAPIClient) LoadBalancing() oapi.LoadBalancer {
	return m.lb
}

func newAPI(t *testing.T, self *cloud.VM, clusterID string) (*cloud.Cloud, *oapimocks.MockOAPI, *oapimocks.MockLoadBalancer) {
	ctrl := gomock.NewController(t)
	oa := oapimocks.NewMockOAPI(ctrl)
	lb := oapimocks.NewMockLoadBalancer(ctrl)
	c := cloud.NewWith(MockOAPIClient{oapi: oa, lb: lb}, self, clusterID)
	return c, oa, lb
}

type awsErrorInterface awserr.Error

type awsError struct {
	awsErrorInterface
	code string
}

func (e *awsError) Error() string {
	return e.code
}

func (e *awsError) Code() string {
	return e.code
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
		NetId:          ptr.To("net-foo"),
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
		SubnetId:       ptr.To("subnet-foo"),
		NetId:          ptr.To("net-foo"),
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

func expectNoLoadbalancer(mock *oapimocks.MockLoadBalancer) {
	mock.EXPECT().
		DescribeTagsWithContext(gomock.Any(), gomock.Eq(&elb.DescribeTagsInput{
			LoadBalancerNames: []*string{&lbName},
		})).
		Return(nil, &awsError{code: elb.ErrCodeAccessPointNotFoundException})
}

func expectLoadbalancerExistsAndOwned(mock *oapimocks.MockLoadBalancer, updates ...func(tag *elb.Tag)) {
	tags := []*elb.Tag{{
		Key: ptr.To(cloud.ClusterIDTagKeyPrefix + "foo"),
	}, {
		Key:   ptr.To[string](cloud.ServiceNameTagKey),
		Value: ptr.To(svcName),
	}}
	for _, tag := range tags {
		for _, update := range updates {
			update(tag)
		}
	}
	mock.EXPECT().
		DescribeTagsWithContext(gomock.Any(), gomock.Eq(&elb.DescribeTagsInput{
			LoadBalancerNames: []*string{&lbName},
		})).
		Return(&elb.DescribeTagsOutput{TagDescriptions: []*elb.TagDescription{{
			LoadBalancerName: &lbName,
			Tags:             tags,
		}}}, nil)
}

func expectLoadbalancerExistsAndNotOwned(mock *oapimocks.MockLoadBalancer) {
	mock.EXPECT().
		DescribeTagsWithContext(gomock.Any(), gomock.Eq(&elb.DescribeTagsInput{
			LoadBalancerNames: []*string{&lbName},
		})).
		Return(&elb.DescribeTagsOutput{TagDescriptions: []*elb.TagDescription{{
			LoadBalancerName: &lbName,
			Tags: []*elb.Tag{{
				Key: ptr.To(cloud.ClusterIDTagKeyPrefix + "bar"),
			}, {
				Key:   ptr.To[string](cloud.ServiceNameTagKey),
				Value: ptr.To("baz"),
			}}},
		}}, nil)
}

func expectLoadBalancerDescription(mock *oapimocks.MockLoadBalancer, updates ...func(*elb.LoadBalancerDescription)) {
	desc := &elb.LoadBalancerDescription{
		LoadBalancerName: &lbName,
		SecurityGroups:   []*string{ptr.To("sg-foo")},
		Subnets:          []*string{ptr.To("subnet-service")},
		ListenerDescriptions: []*elb.ListenerDescription{{Listener: &elb.Listener{
			LoadBalancerPort: ptr.To[int64](80),
			Protocol:         ptr.To("HTTP"),
			InstancePort:     ptr.To[int64](8080),
			InstanceProtocol: ptr.To("HTTP"),
		}}},
		HealthCheck: &elb.HealthCheck{
			HealthyThreshold:   ptr.To[int64](2),
			Interval:           ptr.To[int64](10),
			Target:             ptr.To("TCP:8080"),
			Timeout:            ptr.To[int64](5),
			UnhealthyThreshold: ptr.To[int64](3),
		},
	}
	for _, update := range updates {
		update(desc)
	}
	mock.EXPECT().
		DescribeLoadBalancersWithContext(gomock.Any(), gomock.Eq(&elb.DescribeLoadBalancersInput{
			LoadBalancerNames: []*string{&lbName},
		})).
		Return(&elb.DescribeLoadBalancersOutput{
			LoadBalancerDescriptions: []*elb.LoadBalancerDescription{desc},
		}, nil)
}

func expectNoLoadBalancerDescription(mock *oapimocks.MockLoadBalancer) {
	mock.EXPECT().
		DescribeLoadBalancersWithContext(gomock.Any(), gomock.Eq(&elb.DescribeLoadBalancersInput{
			LoadBalancerNames: []*string{&lbName},
		})).
		Return(&elb.DescribeLoadBalancersOutput{
			LoadBalancerDescriptions: []*elb.LoadBalancerDescription{},
		}, nil)
}

func expectCreateLoadBalancer(mock *oapimocks.MockLoadBalancer, updates ...func(*elb.CreateLoadBalancerInput)) {
	req := &elb.CreateLoadBalancerInput{
		LoadBalancerName: &lbName,
		SecurityGroups:   []*string{ptr.To("sg-foo")},
		Subnets:          []*string{ptr.To("subnet-service")},
		Tags: []*elb.Tag{{
			Key:   ptr.To("OscK8sClusterID/foo"),
			Value: ptr.To("owned"),
		}, {
			Key:   ptr.To("OscK8sService"),
			Value: ptr.To("svc-foo"),
		}},
		Listeners: []*elb.Listener{{
			LoadBalancerPort: ptr.To[int64](80),
			Protocol:         ptr.To("HTTP"),
			InstancePort:     ptr.To[int64](8080),
			InstanceProtocol: ptr.To("HTTP"),
		}},
	}
	for _, update := range updates {
		update(req)
	}
	mock.EXPECT().
		CreateLoadBalancerWithContext(gomock.Any(), gomock.Eq(req)).
		Return(&elb.CreateLoadBalancerOutput{}, nil)
}

func expectDeleteLoadBalancer(mock *oapimocks.MockLoadBalancer) {
	mock.EXPECT().
		DeleteLoadBalancerWithContext(gomock.Any(), gomock.Eq(&elb.DeleteLoadBalancerInput{
			LoadBalancerName: &lbName,
		})).
		Return(&elb.DeleteLoadBalancerOutput{}, nil)
}

func expectConfigureHealthCheck(mock *oapimocks.MockLoadBalancer) {
	mock.EXPECT().
		ConfigureHealthCheckWithContext(gomock.Any(), gomock.Eq(&elb.ConfigureHealthCheckInput{
			HealthCheck: &elb.HealthCheck{
				HealthyThreshold:   ptr.To[int64](2),
				Interval:           ptr.To[int64](10),
				Target:             ptr.To("TCP:8080"),
				Timeout:            ptr.To[int64](5),
				UnhealthyThreshold: ptr.To[int64](3),
			},
			LoadBalancerName: ptr.To("lb-foo"),
		})).
		Return(&elb.ConfigureHealthCheckOutput{}, nil)
}

func expectConfigureLoadBalancerPolicy(mock *oapimocks.MockLoadBalancer, ports ...int64) {
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
	if ports == nil {
		ports = []int64{8080}
	}
	for _, port := range ports {
		mock.EXPECT().
			SetLoadBalancerPoliciesForBackendServerWithContext(gomock.Any(), gomock.Eq(&elb.SetLoadBalancerPoliciesForBackendServerInput{
				InstancePort:     ptr.To[int64](port),
				PolicyNames:      []*string{ptr.To("k8s-proxyprotocol-enabled")},
				LoadBalancerName: ptr.To("lb-foo"),
			})).
			Return(&elb.SetLoadBalancerPoliciesForBackendServerOutput{}, nil)
	}
}

func expectFindLBSubnet(mock *oapimocks.MockOAPI) {
	mock.EXPECT().
		ReadSubnets(gomock.Any(), gomock.Eq(sdk.ReadSubnetsRequest{
			Filters: &sdk.FiltersSubnet{
				TagKeys: &[]string{"OscK8sClusterID/foo"},
			},
		})).
		Return([]sdk.Subnet{
			{SubnetId: ptr.To("subnet-service"), Tags: &[]sdk.ResourceTag{{Key: "OscK8sRole/service"}}},
			{SubnetId: ptr.To("subnet-service.internal"), Tags: &[]sdk.ResourceTag{{Key: "OscK8sRole/service.internal"}}},
		}, nil)
}

func expectCreateSecurityGroup(mock *oapimocks.MockOAPI) {
	mock.EXPECT().
		CreateSecurityGroup(gomock.Any(), gomock.Eq(sdk.CreateSecurityGroupRequest{
			SecurityGroupName: "k8s-elb-lb-foo",
			Description:       "Security group for Kubernetes ELB lb-foo (svc-foo)",
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

func expectRegisterInstances(mock *oapimocks.MockLoadBalancer, vmIds ...string) {
	instances := osc.Map(vmIds, func(vmId string) *elb.Instance { return &elb.Instance{InstanceId: &vmId} })
	mock.EXPECT().
		RegisterInstancesWithLoadBalancerWithContext(gomock.Any(), gomock.Eq(&elb.RegisterInstancesWithLoadBalancerInput{
			Instances:        instances,
			LoadBalancerName: ptr.To("lb-foo"),
		})).
		Return(&elb.RegisterInstancesWithLoadBalancerOutput{}, nil)
}

func expectDeregisterInstances(mock *oapimocks.MockLoadBalancer, vmIds ...string) {
	instances := osc.Map(vmIds, func(vmId string) *elb.Instance { return &elb.Instance{InstanceId: &vmId} })
	mock.EXPECT().
		DeregisterInstancesFromLoadBalancerWithContext(gomock.Any(), gomock.Eq(&elb.DeregisterInstancesFromLoadBalancerInput{
			Instances:        instances,
			LoadBalancerName: ptr.To("lb-foo"),
		})).
		Return(&elb.DeregisterInstancesFromLoadBalancerOutput{}, nil)
}

func expectDeleteListener(mock *oapimocks.MockLoadBalancer) {
	mock.EXPECT().
		DeleteLoadBalancerListenersWithContext(gomock.Any(), gomock.Eq(&elb.DeleteLoadBalancerListenersInput{
			LoadBalancerName:  ptr.To("lb-foo"),
			LoadBalancerPorts: []*int64{ptr.To[int64](80)},
		})).
		Return(&elb.DeleteLoadBalancerListenersOutput{}, nil)
}

func expectCreateListener(mock *oapimocks.MockLoadBalancer, port int64) {
	mock.EXPECT().
		CreateLoadBalancerListenersWithContext(gomock.Any(), gomock.Eq(&elb.CreateLoadBalancerListenersInput{
			LoadBalancerName: ptr.To("lb-foo"),
			Listeners: []*elb.Listener{{
				LoadBalancerPort: ptr.To(port),
				Protocol:         ptr.To("HTTP"),
				InstancePort:     ptr.To[int64](8080),
				InstanceProtocol: ptr.To("HTTP"),
			}},
		})).
		Return(&elb.CreateLoadBalancerListenersOutput{}, nil)
}

func expectDescribeLoadBalancerAttributes(mock *oapimocks.MockLoadBalancer) {
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

func expectModifyLoadBalancerAttributes(mock *oapimocks.MockLoadBalancer, attrs *elb.LoadBalancerAttributes) {
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
