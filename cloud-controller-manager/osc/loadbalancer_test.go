package osc_test

import (
	"context"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go/service/elb"
	"github.com/outscale/cloud-provider-osc/cloud-controller-manager/osc"
	"github.com/outscale/cloud-provider-osc/cloud-controller-manager/osc/cloud"
	sdk "github.com/outscale/osc-sdk-go/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

func testSvc() *v1.Service {
	return &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: svcName,
			Annotations: map[string]string{
				"service.beta.kubernetes.io/osc-load-balancer-name": lbName,
			},
		},
		Spec: v1.ServiceSpec{
			Type:            v1.ServiceTypeLoadBalancer,
			SessionAffinity: v1.ServiceAffinityNone,
			Ports: []v1.ServicePort{
				{
					Name:     "tcp",
					Protocol: v1.ProtocolTCP,
					NodePort: 8080,
					Port:     80,
				},
			},
		},
	}
}

func TestGetLoadBalancer(t *testing.T) {
	t.Run("If the ingress is configured, it is returned", func(t *testing.T) {
		svc := testSvc()
		c, _, mock := newAPI(t, self, "foo")
		expectLoadBalancerDescription(mock, func(desc *elb.LoadBalancerDescription) { desc.DNSName = ptr.To("bar.example.com") })
		p := osc.NewProviderWith(c)
		status, exists, err := p.GetLoadBalancer(context.TODO(), "foo", svc)
		require.NoError(t, err)
		assert.True(t, exists)
		assert.Equal(t, &v1.LoadBalancerStatus{Ingress: []v1.LoadBalancerIngress{{Hostname: "bar.example.com"}}}, status)
	})
	t.Run("If no ingress is configured, status is empty", func(t *testing.T) {
		svc := testSvc()
		c, _, mock := newAPI(t, self, "foo")
		expectLoadBalancerDescription(mock, func(desc *elb.LoadBalancerDescription) { desc.DNSName = nil })
		p := osc.NewProviderWith(c)
		status, exists, err := p.GetLoadBalancer(context.TODO(), "foo", svc)
		require.NoError(t, err)
		assert.True(t, exists)
		assert.Equal(t, &v1.LoadBalancerStatus{}, status)
	})
	t.Run("If no load-balancer exists, false is returned", func(t *testing.T) {
		svc := testSvc()
		c, _, mock := newAPI(t, self, "foo")
		expectNoLoadBalancerDescription(mock)
		p := osc.NewProviderWith(c)
		_, exists, err := p.GetLoadBalancer(context.TODO(), "foo", svc)
		require.NoError(t, err)
		assert.False(t, exists)
	})
}

func TestGetLoadBalancerName(t *testing.T) {
	svc := testSvc()
	c, _, _ := newAPI(t, self, "foo")
	p := osc.NewProviderWith(c)
	name := p.GetLoadBalancerName(context.TODO(), "foo", svc)
	assert.Equal(t, lbName, name)
}

func TestEnsureLoadBalancer_Create(t *testing.T) {
	t.Run("Cannot create a load-balancer if a LBU with the same name already exists but from another cluster", func(t *testing.T) {
		svc := testSvc()
		c, _, lbmock := newAPI(t, self, "foo")
		expectLoadbalancerExistsAndNotOwned(lbmock)
		p := osc.NewProviderWith(c)
		_, err := p.EnsureLoadBalancer(context.TODO(), "foo", svc, []*v1.Node{&vmNode})
		require.Error(t, err)
	})
	t.Run("Cannot create a load-balancer if a LBU with the same name already exists but from another service", func(t *testing.T) {
		svc := testSvc()
		c, _, lbmock := newAPI(t, self, "foo")
		expectLoadbalancerExistsAndOwned(lbmock, func(tag *elb.Tag) {
			if *tag.Key == cloud.ServiceNameTagKey {
				tag.Value = ptr.To("baz")
			}
		})
		p := osc.NewProviderWith(c)
		_, err := p.EnsureLoadBalancer(context.TODO(), "foo", svc, []*v1.Node{&vmNode})
		require.Error(t, err)
	})
	t.Run("A public LB is created and a retryable error is returned when not ready", func(t *testing.T) {
		svc := testSvc()
		c, oapimock, lbmock := newAPI(t, self, "foo")
		expectVMs(oapimock, sdkSelf, sdkVM)
		expectNoLoadbalancer(lbmock)
		expectFindLBSubnet(oapimock)
		expectCreateSecurityGroup(oapimock)
		expectFindWorkerSGByRole(oapimock)
		expectAddIngressSGRule(oapimock, []string{"0.0.0.0/0"}, "sg-foo")
		expectAddInternalSGRule(oapimock, "sg-foo", "sg-worker")
		expectCreateLoadBalancer(lbmock)
		expectConfigureHealthCheck(lbmock)
		expectDescribeLoadBalancerAttributes(lbmock)
		expectRegisterInstances(lbmock, *sdkVM.VmId)
		p := osc.NewProviderWith(c)
		_, err := p.EnsureLoadBalancer(context.TODO(), "foo", svc, []*v1.Node{&vmNode})
		require.ErrorIs(t, err, cloud.ErrLoadBalancerIsNotReady)
	})
	t.Run("An internal LB is created and a retryable error is returned when not ready", func(t *testing.T) {
		svc := testSvc()
		svc.Annotations["service.beta.kubernetes.io/osc-load-balancer-internal"] = "true"
		c, oapimock, lbmock := newAPI(t, self, "foo")
		expectVMs(oapimock, sdkSelf, sdkVM)
		expectNoLoadbalancer(lbmock)
		expectFindLBSubnet(oapimock)
		expectCreateSecurityGroup(oapimock)
		expectFindWorkerSGByRole(oapimock)
		expectAddIngressSGRule(oapimock, []string{"0.0.0.0/0"}, "sg-foo")
		expectAddInternalSGRule(oapimock, "sg-foo", "sg-worker")
		expectCreateLoadBalancer(lbmock, func(req *elb.CreateLoadBalancerInput) {
			req.Scheme = ptr.To("internal")
			req.Subnets[0] = ptr.To("subnet-service.internal")
		})
		expectConfigureHealthCheck(lbmock)
		expectDescribeLoadBalancerAttributes(lbmock)
		expectRegisterInstances(lbmock, *sdkVM.VmId)
		p := osc.NewProviderWith(c)
		_, err := p.EnsureLoadBalancer(context.TODO(), "foo", svc, []*v1.Node{&vmNode})
		require.ErrorIs(t, err, cloud.ErrLoadBalancerIsNotReady)
	})
	t.Run("IP restriction may be configured", func(t *testing.T) {
		svc := testSvc()
		svc.Spec.LoadBalancerSourceRanges = []string{"198.51.100.0/24", "203.0.113.0/24"}
		c, oapimock, lbmock := newAPI(t, self, "foo")
		expectVMs(oapimock, sdkSelf, sdkVM)
		expectNoLoadbalancer(lbmock)
		expectFindLBSubnet(oapimock)
		expectCreateSecurityGroup(oapimock)
		expectFindWorkerSGByRole(oapimock)
		expectAddIngressSGRule(oapimock, svc.Spec.LoadBalancerSourceRanges, "sg-foo")
		expectAddInternalSGRule(oapimock, "sg-foo", "sg-worker")
		expectCreateLoadBalancer(lbmock)
		expectConfigureHealthCheck(lbmock)
		expectDescribeLoadBalancerAttributes(lbmock)
		expectRegisterInstances(lbmock, *sdkVM.VmId)
		p := osc.NewProviderWith(c)
		_, err := p.EnsureLoadBalancer(context.TODO(), "foo", svc, []*v1.Node{&vmNode})
		require.ErrorIs(t, err, cloud.ErrLoadBalancerIsNotReady)
	})
	t.Run("The LB SG can be set", func(t *testing.T) {
		svc := testSvc()
		svc.Annotations["service.beta.kubernetes.io/osc-load-balancer-security-group"] = "sg-existing"
		c, oapimock, lbmock := newAPI(t, self, "foo")
		expectVMs(oapimock, sdkSelf, sdkVM)
		expectNoLoadbalancer(lbmock)
		expectFindLBSubnet(oapimock)
		expectFindIngressSecurityGroup(oapimock, "sg-existing")
		expectFindWorkerSGByRole(oapimock)
		expectAddIngressSGRule(oapimock, []string{"0.0.0.0/0"}, "sg-existing")
		expectAddInternalSGRule(oapimock, "sg-existing", "sg-worker")
		expectCreateLoadBalancer(lbmock, func(req *elb.CreateLoadBalancerInput) {
			req.SecurityGroups = []*string{ptr.To("sg-existing")}
		})
		expectConfigureHealthCheck(lbmock)
		expectDescribeLoadBalancerAttributes(lbmock)
		expectRegisterInstances(lbmock, *sdkVM.VmId)
		p := osc.NewProviderWith(c)
		_, err := p.EnsureLoadBalancer(context.TODO(), "foo", svc, []*v1.Node{&vmNode})
		require.ErrorIs(t, err, cloud.ErrLoadBalancerIsNotReady)
	})
	t.Run("A different role may be targeted", func(t *testing.T) {
		svc := testSvc()
		svc.Annotations["service.beta.kubernetes.io/osc-load-balancer-target-role"] = "controlplane"
		c, oapimock, lbmock := newAPI(t, self, "foo")
		expectVMs(oapimock, sdkSelf, sdkVM)
		expectNoLoadbalancer(lbmock)
		expectFindLBSubnet(oapimock)
		expectCreateSecurityGroup(oapimock)
		expectFindWorkerSGByRole(oapimock)
		expectAddIngressSGRule(oapimock, []string{"0.0.0.0/0"}, "sg-foo")
		expectAddInternalSGRule(oapimock, "sg-foo", "sg-controlplane")
		expectCreateLoadBalancer(lbmock)
		expectConfigureHealthCheck(lbmock)
		expectDescribeLoadBalancerAttributes(lbmock)
		expectRegisterInstances(lbmock, *sdkVM.VmId)
		p := osc.NewProviderWith(c)
		_, err := p.EnsureLoadBalancer(context.TODO(), "foo", svc, []*v1.Node{&vmNode})
		require.ErrorIs(t, err, cloud.ErrLoadBalancerIsNotReady)
	})
	t.Run("The LB subnet can be set", func(t *testing.T) {
		svc := testSvc()
		svc.Annotations["service.beta.kubernetes.io/osc-load-balancer-subnet-id"] = "subnet-existing"
		c, oapimock, lbmock := newAPI(t, self, "foo")
		expectVMs(oapimock, sdkSelf, sdkVM)
		expectNoLoadbalancer(lbmock)
		expectCreateSecurityGroup(oapimock)
		expectFindWorkerSGByRole(oapimock)
		expectAddIngressSGRule(oapimock, []string{"0.0.0.0/0"}, "sg-foo")
		expectAddInternalSGRule(oapimock, "sg-foo", "sg-worker")
		expectCreateLoadBalancer(lbmock, func(req *elb.CreateLoadBalancerInput) {
			req.Subnets = []*string{ptr.To("subnet-existing")}
		})
		expectConfigureHealthCheck(lbmock)
		expectDescribeLoadBalancerAttributes(lbmock)
		expectRegisterInstances(lbmock, *sdkVM.VmId)
		p := osc.NewProviderWith(c)
		_, err := p.EnsureLoadBalancer(context.TODO(), "foo", svc, []*v1.Node{&vmNode})
		require.ErrorIs(t, err, cloud.ErrLoadBalancerIsNotReady)
	})
	t.Run("HTTP SSL termination can be set on the LB", func(t *testing.T) {
		svc := testSvc()
		svc.Annotations["service.beta.kubernetes.io/osc-load-balancer-ssl-cert"] = "arn:aws:service:region:account:resource"
		svc.Spec.Ports[0].Port = 443
		c, oapimock, lbmock := newAPI(t, self, "foo")
		expectVMs(oapimock, sdkSelf, sdkVM)
		expectNoLoadbalancer(lbmock)
		expectFindLBSubnet(oapimock)
		expectCreateSecurityGroup(oapimock)
		expectFindWorkerSGByRole(oapimock)
		expectAddIngressSGRule(oapimock, []string{"0.0.0.0/0"}, "sg-foo", func(req *sdk.CreateSecurityGroupRuleRequest) {
			req.GetRules()[0].FromPortRange = ptr.To[int32](443)
			req.GetRules()[0].ToPortRange = ptr.To[int32](443)
		})
		expectAddInternalSGRule(oapimock, "sg-foo", "sg-worker")
		expectCreateLoadBalancer(lbmock, func(clbi *elb.CreateLoadBalancerInput) {
			clbi.Listeners[0].LoadBalancerPort = ptr.To[int64](443)
			clbi.Listeners[0].Protocol = ptr.To("HTTPS")
			clbi.Listeners[0].InstanceProtocol = ptr.To("HTTP")
			clbi.Listeners[0].SSLCertificateId = ptr.To("arn:aws:service:region:account:resource")
		})
		expectConfigureHealthCheck(lbmock)
		expectDescribeLoadBalancerAttributes(lbmock)
		expectRegisterInstances(lbmock, *sdkVM.VmId)
		p := osc.NewProviderWith(c)
		_, err := p.EnsureLoadBalancer(context.TODO(), "foo", svc, []*v1.Node{&vmNode})
		require.ErrorIs(t, err, cloud.ErrLoadBalancerIsNotReady)
	})
	t.Run("HTTP SSL termination can be set on a single port", func(t *testing.T) {
		svc := testSvc()
		svc.Annotations["service.beta.kubernetes.io/osc-load-balancer-ssl-cert"] = "arn:aws:service:region:account:resource"
		svc.Annotations["service.beta.kubernetes.io/osc-load-balancer-ssl-ports"] = "443"
		svc.Spec.Ports = append(svc.Spec.Ports, v1.ServicePort{
			Protocol: v1.ProtocolTCP,
			NodePort: 8080,
			Port:     443,
		})
		c, oapimock, lbmock := newAPI(t, self, "foo")
		expectVMs(oapimock, sdkSelf, sdkVM)
		expectNoLoadbalancer(lbmock)
		expectFindLBSubnet(oapimock)
		expectCreateSecurityGroup(oapimock)
		expectFindWorkerSGByRole(oapimock)
		expectAddIngressSGRule(oapimock, []string{"0.0.0.0/0"}, "sg-foo")
		expectAddIngressSGRule(oapimock, []string{"0.0.0.0/0"}, "sg-foo", func(req *sdk.CreateSecurityGroupRuleRequest) {
			req.GetRules()[0].FromPortRange = ptr.To[int32](443)
			req.GetRules()[0].ToPortRange = ptr.To[int32](443)
		})
		expectAddInternalSGRule(oapimock, "sg-foo", "sg-worker")
		expectAddInternalSGRule(oapimock, "sg-foo", "sg-worker")
		expectCreateLoadBalancer(lbmock, func(req *elb.CreateLoadBalancerInput) {
			req.Listeners = append(req.Listeners, &elb.Listener{
				LoadBalancerPort: ptr.To[int64](443),
				Protocol:         ptr.To("HTTPS"),
				InstancePort:     ptr.To[int64](8080),
				InstanceProtocol: ptr.To("HTTP"),
				SSLCertificateId: ptr.To("arn:aws:service:region:account:resource"),
			})
		})
		expectConfigureHealthCheck(lbmock)
		expectDescribeLoadBalancerAttributes(lbmock)
		expectRegisterInstances(lbmock, *sdkVM.VmId)
		p := osc.NewProviderWith(c)
		_, err := p.EnsureLoadBalancer(context.TODO(), "foo", svc, []*v1.Node{&vmNode})
		require.ErrorIs(t, err, cloud.ErrLoadBalancerIsNotReady)
	})
	t.Run("TCP SSL termination can be set on the LB", func(t *testing.T) {
		svc := testSvc()
		svc.Annotations["service.beta.kubernetes.io/osc-load-balancer-ssl-cert"] = "arn:aws:service:region:account:resource"
		svc.Spec.Ports[0].Port = 465
		c, oapimock, lbmock := newAPI(t, self, "foo")
		expectVMs(oapimock, sdkSelf, sdkVM)
		expectNoLoadbalancer(lbmock)
		expectFindLBSubnet(oapimock)
		expectCreateSecurityGroup(oapimock)
		expectFindWorkerSGByRole(oapimock)
		expectAddIngressSGRule(oapimock, []string{"0.0.0.0/0"}, "sg-foo", func(req *sdk.CreateSecurityGroupRuleRequest) {
			req.GetRules()[0].FromPortRange = ptr.To[int32](465)
			req.GetRules()[0].ToPortRange = ptr.To[int32](465)
		})
		expectAddInternalSGRule(oapimock, "sg-foo", "sg-worker")
		expectCreateLoadBalancer(lbmock, func(clbi *elb.CreateLoadBalancerInput) {
			clbi.Listeners[0].LoadBalancerPort = ptr.To[int64](465)
			clbi.Listeners[0].Protocol = ptr.To("SSL")
			clbi.Listeners[0].InstanceProtocol = ptr.To("TCP")
			clbi.Listeners[0].SSLCertificateId = ptr.To("arn:aws:service:region:account:resource")
		})
		expectConfigureHealthCheck(lbmock)
		expectDescribeLoadBalancerAttributes(lbmock)
		expectRegisterInstances(lbmock, *sdkVM.VmId)
		p := osc.NewProviderWith(c)
		_, err := p.EnsureLoadBalancer(context.TODO(), "foo", svc, []*v1.Node{&vmNode})
		require.ErrorIs(t, err, cloud.ErrLoadBalancerIsNotReady)
	})
	t.Run("backend protocol can be set", func(t *testing.T) {
		svc := testSvc()
		svc.Annotations["service.beta.kubernetes.io/osc-load-balancer-backend-protocol"] = "tcp"
		c, oapimock, lbmock := newAPI(t, self, "foo")
		expectVMs(oapimock, sdkSelf, sdkVM)
		expectNoLoadbalancer(lbmock)
		expectFindLBSubnet(oapimock)
		expectCreateSecurityGroup(oapimock)
		expectFindWorkerSGByRole(oapimock)
		expectAddIngressSGRule(oapimock, []string{"0.0.0.0/0"}, "sg-foo")
		expectAddInternalSGRule(oapimock, "sg-foo", "sg-worker")
		expectCreateLoadBalancer(lbmock, func(clbi *elb.CreateLoadBalancerInput) {
			clbi.Listeners[0].Protocol = ptr.To("TCP")
			clbi.Listeners[0].InstanceProtocol = ptr.To("TCP")
		})
		expectConfigureHealthCheck(lbmock)
		expectDescribeLoadBalancerAttributes(lbmock)
		expectRegisterInstances(lbmock, *sdkVM.VmId)
		p := osc.NewProviderWith(c)
		_, err := p.EnsureLoadBalancer(context.TODO(), "foo", svc, []*v1.Node{&vmNode})
		require.ErrorIs(t, err, cloud.ErrLoadBalancerIsNotReady)
	})
	t.Run("proxy protocol can be set on all ports", func(t *testing.T) {
		svc := testSvc()
		svc.Annotations["service.beta.kubernetes.io/osc-load-balancer-proxy-protocol"] = "*"
		svc.Spec.Ports = append(svc.Spec.Ports, v1.ServicePort{
			Protocol: v1.ProtocolTCP,
			NodePort: 8443,
			Port:     443,
		})
		c, oapimock, lbmock := newAPI(t, self, "foo")
		expectVMs(oapimock, sdkSelf, sdkVM)
		expectNoLoadbalancer(lbmock)
		expectFindLBSubnet(oapimock)
		expectCreateSecurityGroup(oapimock)
		expectFindWorkerSGByRole(oapimock)
		expectAddIngressSGRule(oapimock, []string{"0.0.0.0/0"}, "sg-foo")
		expectAddIngressSGRule(oapimock, []string{"0.0.0.0/0"}, "sg-foo", func(req *sdk.CreateSecurityGroupRuleRequest) {
			req.GetRules()[0].FromPortRange = ptr.To[int32](443)
			req.GetRules()[0].ToPortRange = ptr.To[int32](443)
		})
		expectAddInternalSGRule(oapimock, "sg-foo", "sg-worker")
		expectAddInternalSGRule(oapimock, "sg-foo", "sg-worker", func(req *sdk.CreateSecurityGroupRuleRequest) {
			req.GetRules()[0].FromPortRange = ptr.To[int32](8443)
			req.GetRules()[0].ToPortRange = ptr.To[int32](8443)
		})
		expectCreateLoadBalancer(lbmock, func(req *elb.CreateLoadBalancerInput) {
			req.Listeners = append(req.Listeners, &elb.Listener{
				LoadBalancerPort: ptr.To[int64](443),
				Protocol:         ptr.To("TCP"),
				InstancePort:     ptr.To[int64](8443),
				InstanceProtocol: ptr.To("TCP"),
			})
		})
		expectConfigureHealthCheck(lbmock)
		expectConfigureLoadBalancerPolicy(lbmock, 8080, 8443)
		expectDescribeLoadBalancerAttributes(lbmock)
		expectRegisterInstances(lbmock, *sdkVM.VmId)
		p := osc.NewProviderWith(c)
		_, err := p.EnsureLoadBalancer(context.TODO(), "foo", svc, []*v1.Node{&vmNode})
		require.ErrorIs(t, err, cloud.ErrLoadBalancerIsNotReady)
	})
	t.Run("proxy protocol can be set on a single port", func(t *testing.T) {
		svc := testSvc()
		svc.Annotations["service.beta.kubernetes.io/osc-load-balancer-proxy-protocol"] = "8080"
		svc.Spec.Ports = append(svc.Spec.Ports, v1.ServicePort{
			Protocol: v1.ProtocolTCP,
			NodePort: 8443,
			Port:     443,
		})
		c, oapimock, lbmock := newAPI(t, self, "foo")
		expectVMs(oapimock, sdkSelf, sdkVM)
		expectNoLoadbalancer(lbmock)
		expectFindLBSubnet(oapimock)
		expectCreateSecurityGroup(oapimock)
		expectFindWorkerSGByRole(oapimock)
		expectAddIngressSGRule(oapimock, []string{"0.0.0.0/0"}, "sg-foo")
		expectAddIngressSGRule(oapimock, []string{"0.0.0.0/0"}, "sg-foo", func(req *sdk.CreateSecurityGroupRuleRequest) {
			req.GetRules()[0].FromPortRange = ptr.To[int32](443)
			req.GetRules()[0].ToPortRange = ptr.To[int32](443)
		})
		expectAddInternalSGRule(oapimock, "sg-foo", "sg-worker")
		expectAddInternalSGRule(oapimock, "sg-foo", "sg-worker", func(req *sdk.CreateSecurityGroupRuleRequest) {
			req.GetRules()[0].FromPortRange = ptr.To[int32](8443)
			req.GetRules()[0].ToPortRange = ptr.To[int32](8443)
		})
		expectCreateLoadBalancer(lbmock, func(req *elb.CreateLoadBalancerInput) {
			req.Listeners = append(req.Listeners, &elb.Listener{
				LoadBalancerPort: ptr.To[int64](443),
				Protocol:         ptr.To("TCP"),
				InstancePort:     ptr.To[int64](8443),
				InstanceProtocol: ptr.To("TCP"),
			})
		})
		expectConfigureHealthCheck(lbmock)
		expectConfigureLoadBalancerPolicy(lbmock, 8080)
		expectDescribeLoadBalancerAttributes(lbmock)
		expectRegisterInstances(lbmock, *sdkVM.VmId)
		p := osc.NewProviderWith(c)
		_, err := p.EnsureLoadBalancer(context.TODO(), "foo", svc, []*v1.Node{&vmNode})
		require.ErrorIs(t, err, cloud.ErrLoadBalancerIsNotReady)
	})
	t.Run("logs can be stored", func(t *testing.T) {
		svc := testSvc()
		svc.Annotations["service.beta.kubernetes.io/osc-load-balancer-access-log-enabled"] = "true"
		svc.Annotations["service.beta.kubernetes.io/osc-load-balancer-access-log-emit-interval"] = "30"
		svc.Annotations["service.beta.kubernetes.io/osc-load-balancer-access-log-oos-bucket-name"] = "bucket"
		svc.Annotations["service.beta.kubernetes.io/osc-load-balancer-access-log-oos-bucket-prefix"] = "prefix"
		c, oapimock, lbmock := newAPI(t, self, "foo")
		expectVMs(oapimock, sdkSelf, sdkVM)
		expectNoLoadbalancer(lbmock)
		expectFindLBSubnet(oapimock)
		expectCreateSecurityGroup(oapimock)
		expectFindWorkerSGByRole(oapimock)
		expectAddIngressSGRule(oapimock, []string{"0.0.0.0/0"}, "sg-foo")
		expectAddInternalSGRule(oapimock, "sg-foo", "sg-worker")
		expectCreateLoadBalancer(lbmock)
		expectConfigureHealthCheck(lbmock)
		expectDescribeLoadBalancerAttributes(lbmock)
		expectModifyLoadBalancerAttributes(lbmock, &elb.LoadBalancerAttributes{
			AccessLog: &elb.AccessLog{
				Enabled:        ptr.To(true),
				EmitInterval:   ptr.To[int64](30),
				S3BucketName:   ptr.To("bucket"),
				S3BucketPrefix: ptr.To("prefix"),
			},
		})
		expectRegisterInstances(lbmock, *sdkVM.VmId)
		p := osc.NewProviderWith(c)
		_, err := p.EnsureLoadBalancer(context.TODO(), "foo", svc, []*v1.Node{&vmNode})
		require.ErrorIs(t, err, cloud.ErrLoadBalancerIsNotReady)
	})
}

func TestEnsureLoadBalancer_Update(t *testing.T) {
	t.Run("When retrying creation, the status is properly returned when ready", func(t *testing.T) {
		svc := testSvc()
		c, oapimock, lbmock := newAPI(t, self, "foo")
		expectLoadbalancerExistsAndOwned(lbmock)
		expectVMs(oapimock, sdkSelf, sdkVM)
		expectLoadBalancerDescription(lbmock, func(desc *elb.LoadBalancerDescription) {
			desc.DNSName = ptr.To("foo.example.com")
			desc.Instances = []*elb.Instance{{InstanceId: ptr.To("i-foo")}}
		})
		expectDescribeLoadBalancerAttributes(lbmock)
		expectFindExistingIngressSecurityGroup(oapimock, "sg-foo")
		expectFindExistingWorkerSG(oapimock)
		p := osc.NewProviderWith(c)
		status, err := p.EnsureLoadBalancer(context.TODO(), "foo", svc, []*v1.Node{&vmNode})
		require.NoError(t, err)
		assert.Equal(t, &v1.LoadBalancerStatus{Ingress: []v1.LoadBalancerIngress{{Hostname: "foo.example.com"}}}, status)
	})
	t.Run("Listeners are updated", func(t *testing.T) {
		svc := testSvc()
		svc.Annotations["service.beta.kubernetes.io/osc-load-balancer-backend-protocol"] = "http"
		svc.Spec.Ports[0].Port = 8080
		c, oapimock, lbmock := newAPI(t, self, "foo")
		expectLoadbalancerExistsAndOwned(lbmock)
		expectVMs(oapimock, sdkSelf, sdkVM)
		expectLoadBalancerDescription(lbmock, func(desc *elb.LoadBalancerDescription) {
			desc.DNSName = ptr.To("foo.example.com")
			desc.Instances = []*elb.Instance{{InstanceId: ptr.To("i-foo")}}
		})
		expectDeleteListener(lbmock)
		expectCreateListener(lbmock, 8080)
		expectDescribeLoadBalancerAttributes(lbmock)
		expectFindExistingIngressSecurityGroup(oapimock, "sg-foo")
		expectFindExistingWorkerSG(oapimock)
		expectDeleteIngressSGRule(oapimock, []string{"0.0.0.0/0"}, "sg-foo")
		expectAddIngressSGRule(oapimock, []string{"0.0.0.0/0"}, "sg-foo", func(req *sdk.CreateSecurityGroupRuleRequest) {
			req.GetRules()[0].FromPortRange = ptr.To[int32](8080)
			req.GetRules()[0].ToPortRange = ptr.To[int32](8080)
		})
		p := osc.NewProviderWith(c)
		_, err := p.EnsureLoadBalancer(context.TODO(), "foo", svc, []*v1.Node{&vmNode})
		require.NoError(t, err)
	})
}

func TestUpdateLoadBalancer(t *testing.T) {
	t.Run("Listeners are updated", func(t *testing.T) {
		svc := testSvc()
		svc.Annotations["service.beta.kubernetes.io/osc-load-balancer-backend-protocol"] = "http"
		svc.Spec.Ports[0].Port = 8080
		c, oapimock, lbmock := newAPI(t, self, "foo")
		expectLoadbalancerExistsAndOwned(lbmock)
		expectVMs(oapimock, sdkSelf, sdkVM)
		expectLoadBalancerDescription(lbmock, func(desc *elb.LoadBalancerDescription) {
			desc.DNSName = ptr.To("foo.example.com")
			desc.Instances = []*elb.Instance{{InstanceId: ptr.To("i-foo")}}
		})
		expectDeleteListener(lbmock)
		expectCreateListener(lbmock, 8080)
		expectDescribeLoadBalancerAttributes(lbmock)
		expectFindExistingIngressSecurityGroup(oapimock, "sg-foo")
		expectFindExistingWorkerSG(oapimock)
		expectDeleteIngressSGRule(oapimock, []string{"0.0.0.0/0"}, "sg-foo")
		expectAddIngressSGRule(oapimock, []string{"0.0.0.0/0"}, "sg-foo", func(req *sdk.CreateSecurityGroupRuleRequest) {
			req.GetRules()[0].FromPortRange = ptr.To[int32](8080)
			req.GetRules()[0].ToPortRange = ptr.To[int32](8080)
		})
		p := osc.NewProviderWith(c)
		err := p.UpdateLoadBalancer(context.TODO(), "foo", svc, []*v1.Node{&vmNode})
		require.NoError(t, err)
	})
	t.Run("SSL Certificate is updated", func(t *testing.T) {
		svc := testSvc()
		svc.Annotations["service.beta.kubernetes.io/osc-load-balancer-ssl-cert"] = "arn:aws:service:region:account:new_resource"
		svc.Annotations["service.beta.kubernetes.io/osc-load-balancer-backend-protocol"] = "http"
		c, oapimock, lbmock := newAPI(t, self, "foo")
		expectLoadbalancerExistsAndOwned(lbmock)
		expectVMs(oapimock, sdkSelf, sdkVM)
		expectLoadBalancerDescription(lbmock, func(desc *elb.LoadBalancerDescription) {
			desc.DNSName = ptr.To("foo.example.com")
			desc.Instances = []*elb.Instance{{InstanceId: ptr.To("i-foo")}}
			desc.ListenerDescriptions[0].Listener.Protocol = ptr.To("https")
			desc.ListenerDescriptions[0].Listener.InstanceProtocol = ptr.To("http")
			desc.ListenerDescriptions[0].Listener.SSLCertificateId = ptr.To("arn:aws:service:region:account:resource")
		})
		expectDescribeLoadBalancerAttributes(lbmock)
		expectFindExistingIngressSecurityGroup(oapimock, "sg-foo")
		expectFindExistingWorkerSG(oapimock)
		lbmock.EXPECT().
			SetLoadBalancerListenerSSLCertificateWithContext(gomock.Any(), gomock.Eq(&elb.SetLoadBalancerListenerSSLCertificateInput{
				LoadBalancerName: ptr.To("lb-foo"),
				LoadBalancerPort: ptr.To[int64](80),
				SSLCertificateId: ptr.To("arn:aws:service:region:account:new_resource"),
			})).Return(&elb.SetLoadBalancerListenerSSLCertificateOutput{}, nil)
		p := osc.NewProviderWith(c)
		err := p.UpdateLoadBalancer(context.TODO(), "foo", svc, []*v1.Node{&vmNode})
		require.NoError(t, err)
	})
	t.Run("IP restriction is updated", func(t *testing.T) {
		svc := testSvc()
		svc.Spec.LoadBalancerSourceRanges = []string{"203.0.113.0/24", "198.51.100.0/24"}
		c, oapimock, lbmock := newAPI(t, self, "foo")
		expectLoadbalancerExistsAndOwned(lbmock)
		expectVMs(oapimock, sdkSelf, sdkVM)
		expectLoadBalancerDescription(lbmock, func(desc *elb.LoadBalancerDescription) {
			desc.DNSName = ptr.To("foo.example.com")
			desc.Instances = []*elb.Instance{{InstanceId: ptr.To("i-foo")}}
		})
		expectDescribeLoadBalancerAttributes(lbmock)
		expectFindExistingIngressSecurityGroup(oapimock, "sg-foo")
		expectFindExistingWorkerSG(oapimock)
		expectDeleteIngressSGRule(oapimock, []string{"0.0.0.0/0"}, "sg-foo")
		expectAddIngressSGRule(oapimock, []string{"198.51.100.0/24", "203.0.113.0/24"}, "sg-foo")
		p := osc.NewProviderWith(c)
		err := p.UpdateLoadBalancer(context.TODO(), "foo", svc, []*v1.Node{&vmNode})
		require.NoError(t, err)
	})
	t.Run("Cannot update a load-balancer if a LBU with the same name already exists but from another cluster", func(t *testing.T) {
		svc := testSvc()
		c, _, lbmock := newAPI(t, self, "foo")
		expectLoadbalancerExistsAndOwned(lbmock, func(tag *elb.Tag) {
			if strings.HasPrefix(*tag.Key, cloud.ClusterIDTagKeyPrefix) {
				tag.Key = ptr.To(cloud.ClusterIDTagKeyPrefix + "bar")
			}
		})
		p := osc.NewProviderWith(c)
		err := p.UpdateLoadBalancer(context.TODO(), "foo", svc, []*v1.Node{&vmNode})
		require.Error(t, err)
	})
	t.Run("Can update a load-balancer even if service tag is not set", func(t *testing.T) {
		svc := testSvc()
		c, oapimock, lbmock := newAPI(t, self, "foo")
		expectLoadbalancerExistsAndOwned(lbmock, func(tag *elb.Tag) {
			if *tag.Key == cloud.ServiceNameTagKey {
				tag.Key = nil
			}
		})
		expectVMs(oapimock, sdkSelf, sdkVM)
		expectLoadBalancerDescription(lbmock, func(desc *elb.LoadBalancerDescription) {
			desc.DNSName = ptr.To("foo.example.com")
			desc.Instances = []*elb.Instance{{InstanceId: ptr.To("i-foo")}}
		})
		expectDescribeLoadBalancerAttributes(lbmock)
		expectFindExistingIngressSecurityGroup(oapimock, "sg-foo")
		expectFindExistingWorkerSG(oapimock)
		p := osc.NewProviderWith(c)
		err := p.UpdateLoadBalancer(context.TODO(), "foo", svc, []*v1.Node{&vmNode})
		require.NoError(t, err)
	})
	t.Run("Nodes are added", func(t *testing.T) {
		svc := testSvc()
		c, oapimock, lbmock := newAPI(t, self, "foo")
		expectLoadbalancerExistsAndOwned(lbmock)
		expectVMs(oapimock, sdkSelf, sdkVM)
		expectLoadBalancerDescription(lbmock, func(desc *elb.LoadBalancerDescription) {
			desc.DNSName = ptr.To("foo.example.com")
		})
		expectDescribeLoadBalancerAttributes(lbmock)
		expectFindExistingIngressSecurityGroup(oapimock, "sg-foo")
		expectFindExistingWorkerSG(oapimock)
		expectRegisterInstances(lbmock, "i-foo")
		p := osc.NewProviderWith(c)
		err := p.UpdateLoadBalancer(context.TODO(), "foo", svc, []*v1.Node{&vmNode})
		require.NoError(t, err)
	})
	t.Run("Nodes are removed", func(t *testing.T) {
		svc := testSvc()
		c, oapimock, lbmock := newAPI(t, self, "foo")
		expectLoadbalancerExistsAndOwned(lbmock)
		expectVMs(oapimock, sdkSelf, sdkVM)
		expectLoadBalancerDescription(lbmock, func(desc *elb.LoadBalancerDescription) {
			desc.DNSName = ptr.To("foo.example.com")
			desc.Instances = []*elb.Instance{{InstanceId: ptr.To("i-foo")}, {InstanceId: ptr.To("i-bar")}}
		})
		expectDescribeLoadBalancerAttributes(lbmock)
		expectFindExistingIngressSecurityGroup(oapimock, "sg-foo")
		expectFindExistingWorkerSG(oapimock)
		expectDeregisterInstances(lbmock, "i-bar")
		p := osc.NewProviderWith(c)
		err := p.UpdateLoadBalancer(context.TODO(), "foo", svc, []*v1.Node{&vmNode})
		require.NoError(t, err)
	})
}

func TestEnsureLoadBalancerDeleted(t *testing.T) {
	t.Run("If the load-balancer exists, delete it", func(t *testing.T) {
		svc := testSvc()
		c, oapimock, lbmock := newAPI(t, self, "foo")
		expectLoadbalancerExistsAndOwned(lbmock)
		expectLoadBalancerDescription(lbmock, func(desc *elb.LoadBalancerDescription) {
			desc.DNSName = ptr.To("foo.example.com")
			desc.Instances = []*elb.Instance{{InstanceId: ptr.To("i-foo")}}
		})
		expectDeregisterInstances(lbmock, "i-foo")
		expectCreateTag(oapimock, "sg-foo", sdk.ResourceTag{Key: cloud.SGToDeleteTagKey})
		expectDeleteLoadBalancer(lbmock)
		p := osc.NewProviderWith(c)
		err := p.EnsureLoadBalancerDeleted(context.TODO(), "foo", svc)
		require.NoError(t, err)
	})
	t.Run("If the load-balancer has already been deleted, do nothing", func(t *testing.T) {
		svc := testSvc()
		c, _, lbmock := newAPI(t, self, "foo")
		expectNoLoadbalancer(lbmock)
		p := osc.NewProviderWith(c)
		err := p.EnsureLoadBalancerDeleted(context.TODO(), "foo", svc)
		require.NoError(t, err)
	})
}
