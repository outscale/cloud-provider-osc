/*
SPDX-FileCopyrightText: 2025 Outscale SAS <opensource@outscale.com>

SPDX-License-Identifier: BSD-3-Clause
*/
package ccm_test

import (
	"context"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go/service/elb" //nolint:staticcheck
	"github.com/outscale/cloud-provider-osc/ccm"
	"github.com/outscale/cloud-provider-osc/ccm/cloud"
	"github.com/outscale/goutils/k8s/tags"
	"github.com/outscale/goutils/sdk/ptr"
	"github.com/outscale/osc-sdk-go/v3/pkg/osc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func testSvc() *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: svcName,
			Annotations: map[string]string{
				"service.beta.kubernetes.io/osc-load-balancer-name": lbName,
			},
		},
		Spec: corev1.ServiceSpec{
			Type:            corev1.ServiceTypeLoadBalancer,
			SessionAffinity: corev1.ServiceAffinityNone,
			Ports: []corev1.ServicePort{
				{
					Name:     "tcp",
					Protocol: corev1.ProtocolTCP,
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
		c, mock, _ := newAPI(t, self, []string{"foo"})
		expectReadLoadBalancer(mock, func(desc *osc.LoadBalancer) {
			desc.DnsName = "bar.example.com"
			desc.PublicIp = ptr.To("198.51.100.42")
		})
		p := ccm.NewProviderWith(c, staticDNSResolver{}, ccm.Options{})
		status, exists, err := p.GetLoadBalancer(t.Context(), "foo", svc)
		require.NoError(t, err)
		assert.True(t, exists)
		assert.Equal(t, &corev1.LoadBalancerStatus{Ingress: []corev1.LoadBalancerIngress{{Hostname: "bar.example.com"}}}, status)
	})
	t.Run("The ingress has only the IP when requested", func(t *testing.T) {
		svc := testSvc()
		svc.Annotations["service.beta.kubernetes.io/osc-load-balancer-ingress-address"] = "ip"
		c, mock, _ := newAPI(t, self, []string{"foo"})
		expectReadLoadBalancer(mock, func(desc *osc.LoadBalancer) {
			desc.DnsName = "bar.example.com"
			desc.PublicIp = ptr.To("198.51.100.42")
		})
		p := ccm.NewProviderWith(c, staticDNSResolver{}, ccm.Options{})
		status, exists, err := p.GetLoadBalancer(t.Context(), "foo", svc)
		require.NoError(t, err)
		assert.True(t, exists)
		assert.Equal(t, &corev1.LoadBalancerStatus{Ingress: []corev1.LoadBalancerIngress{{IP: "198.51.100.42", IPMode: ptr.To(corev1.LoadBalancerIPModeProxy)}}}, status)
	})
	t.Run("The ingress has only the hostname when requested", func(t *testing.T) {
		svc := testSvc()
		svc.Annotations["service.beta.kubernetes.io/osc-load-balancer-ingress-address"] = "hostname"
		c, mock, _ := newAPI(t, self, []string{"foo"})
		expectReadLoadBalancer(mock, func(desc *osc.LoadBalancer) {
			desc.DnsName = "bar.example.com"
			desc.PublicIp = ptr.To("198.51.100.42")
		})
		p := ccm.NewProviderWith(c, staticDNSResolver{}, ccm.Options{})
		status, exists, err := p.GetLoadBalancer(t.Context(), "foo", svc)
		require.NoError(t, err)
		assert.True(t, exists)
		assert.Equal(t, &corev1.LoadBalancerStatus{Ingress: []corev1.LoadBalancerIngress{{Hostname: "bar.example.com"}}}, status)
	})
	t.Run("The ingress has bost IP and hostname when requested", func(t *testing.T) {
		svc := testSvc()
		svc.Annotations["service.beta.kubernetes.io/osc-load-balancer-ingress-address"] = "both"
		c, mock, _ := newAPI(t, self, []string{"foo"})
		expectReadLoadBalancer(mock, func(desc *osc.LoadBalancer) {
			desc.DnsName = "bar.example.com"
			desc.PublicIp = ptr.To("198.51.100.42")
		})
		p := ccm.NewProviderWith(c, staticDNSResolver{}, ccm.Options{})
		status, exists, err := p.GetLoadBalancer(t.Context(), "foo", svc)
		require.NoError(t, err)
		assert.True(t, exists)
		assert.Equal(t, &corev1.LoadBalancerStatus{Ingress: []corev1.LoadBalancerIngress{{Hostname: "bar.example.com", IP: "198.51.100.42", IPMode: ptr.To(corev1.LoadBalancerIPModeProxy)}}}, status)
	})
	t.Run("If no ingress is configured, no ingresses are returned", func(t *testing.T) {
		svc := testSvc()
		c, mock, _ := newAPI(t, self, []string{"foo"})
		expectReadLoadBalancer(mock, func(desc *osc.LoadBalancer) { desc.DnsName = "" })
		p := ccm.NewProviderWith(c, staticDNSResolver{}, ccm.Options{})
		status, exists, err := p.GetLoadBalancer(t.Context(), "foo", svc)
		require.NoError(t, err)
		assert.True(t, exists)
		assert.Empty(t, status.Ingress)
	})
	t.Run("If no load-balancer exists, false is returned", func(t *testing.T) {
		svc := testSvc()
		c, mock, _ := newAPI(t, self, []string{"foo"})
		expectReadLoadBalancerNoneFound(mock)
		p := ccm.NewProviderWith(c, staticDNSResolver{}, ccm.Options{})
		_, exists, err := p.GetLoadBalancer(t.Context(), "foo", svc)
		require.NoError(t, err)
		assert.False(t, exists)
	})
	t.Run("The ingress ipmode is not set by when ipmode annotation in 'Proxy' but IP is not defined", func(t *testing.T) {
		svc := testSvc()
		svc.Annotations["service.beta.kubernetes.io/osc-load-balancer-ingress-address"] = "hostname"
		svc.Annotations["service.beta.kubernetes.io/osc-load-balancer-ingress-ipmode"] = "Proxy"
		c, mock, _ := newAPI(t, self, []string{"foo"})
		expectReadLoadBalancer(mock, func(desc *osc.LoadBalancer) {
			desc.DnsName = "bar.example.com"
			desc.PublicIp = ptr.To("198.51.100.42")
		})
		p := ccm.NewProviderWith(c, staticDNSResolver{}, ccm.Options{})
		status, exists, err := p.GetLoadBalancer(t.Context(), "foo", svc)
		require.NoError(t, err)
		assert.True(t, exists)
		assert.Equal(t, &corev1.LoadBalancerStatus{Ingress: []corev1.LoadBalancerIngress{{Hostname: "bar.example.com"}}}, status)
	})
	t.Run("The ingress ipmode is Proxy when IP is defined and ipmode annotation in 'VIP'", func(t *testing.T) {
		svc := testSvc()
		svc.Annotations["service.beta.kubernetes.io/osc-load-balancer-ingress-address"] = "both"
		svc.Annotations["service.beta.kubernetes.io/osc-load-balancer-ingress-ipmode"] = "VIP"
		c, mock, _ := newAPI(t, self, []string{"foo"})
		expectReadLoadBalancer(mock, func(desc *osc.LoadBalancer) {
			desc.DnsName = "bar.example.com"
			desc.PublicIp = ptr.To("198.51.100.42")
		})
		p := ccm.NewProviderWith(c, staticDNSResolver{}, ccm.Options{})
		status, exists, err := p.GetLoadBalancer(t.Context(), "foo", svc)
		require.NoError(t, err)
		assert.True(t, exists)
		assert.Equal(t, &corev1.LoadBalancerStatus{Ingress: []corev1.LoadBalancerIngress{{Hostname: "bar.example.com", IP: "198.51.100.42", IPMode: ptr.To(corev1.LoadBalancerIPModeVIP)}}}, status)
	})
}

func TestGetLoadBalancerName(t *testing.T) {
	svc := testSvc()
	c, _, _ := newAPI(t, self, []string{"foo"})
	p := ccm.NewProviderWith(c, staticDNSResolver{}, ccm.Options{})
	name := p.GetLoadBalancerName(t.Context(), "foo", svc)
	assert.Equal(t, lbName, name)
}

func TestEnsureLoadBalancer_Create(t *testing.T) {
	t.Run("Cannot create a load-balancer if a LBU with the same name already exists but from another cluster", func(t *testing.T) {
		svc := testSvc()
		c, oapimock, _ := newAPI(t, self, []string{"foo"})
		expectLoadbalancerExistsAndNotOwned(oapimock)
		p := ccm.NewProviderWith(c, staticDNSResolver{}, ccm.Options{})
		_, err := p.EnsureLoadBalancer(t.Context(), "foo", svc, []*corev1.Node{&vmNode})
		require.Error(t, err)
	})
	t.Run("Cannot create a load-balancer if a LBU with the same name already exists but from another service", func(t *testing.T) {
		svc := testSvc()
		c, oapimock, _ := newAPI(t, self, []string{"foo"})
		expectLoadbalancerExistsAndOwned(oapimock, func(tag *osc.ResourceTag) {
			if tag.Key == tags.ServiceName {
				tag.Value = "baz"
			}
		})
		p := ccm.NewProviderWith(c, staticDNSResolver{}, ccm.Options{})
		_, err := p.EnsureLoadBalancer(t.Context(), "foo", svc, []*corev1.Node{&vmNode})
		require.Error(t, err)
	})
	t.Run("Cannot create a load-balancer if no subnet is found", func(t *testing.T) {
		svc := testSvc()
		c, oapimock, _ := newAPI(t, self, []string{"foo"})
		expectVMs(oapimock, sdkSelf, sdkVM)
		expectNoLoadbalancer(oapimock)
		expectFindNoLBSubnetWithRole(oapimock)
		expectFindNoPublicRouteTables(oapimock)
		p := ccm.NewProviderWith(c, staticDNSResolver{}, ccm.Options{})
		_, err := p.EnsureLoadBalancer(t.Context(), "foo", svc, []*corev1.Node{&vmNode})
		require.Error(t, err)
	})
	t.Run("A public LB is created, and a retryable error is returned if it is not ready", func(t *testing.T) {
		svc := testSvc()
		c, oapimock, lbmock := newAPI(t, self, []string{"foo"})
		expectVMs(oapimock, sdkSelf, sdkVM)
		expectNoLoadbalancer(oapimock)
		expectFindLBSubnetWithRole(oapimock)
		expectCreateSecurityGroup(oapimock)
		expectFindWorkerSGByRole(oapimock)
		expectAddIngressSGRule(oapimock, []string{"0.0.0.0/0"}, "sg-foo")
		expectAddInternalSGRule(oapimock, "sg-foo", "sg-worker")
		expectCreateLoadBalancer(oapimock)
		expectConfigureHealthCheck(oapimock)
		expectDescribeProxyProtocol(lbmock, false)
		expectDescribeLoadBalancerAttributes(lbmock)
		expectRegisterInstances(oapimock, sdkVM.VmId)
		p := ccm.NewProviderWith(c, staticDNSResolver{}, ccm.Options{})
		_, err := p.EnsureLoadBalancer(t.Context(), "foo", svc, []*corev1.Node{&vmNode})
		require.ErrorIs(t, err, cloud.ErrLoadBalancerIsNotReady)
	})
	t.Run("A public LB is created, and a security group is reused if already created", func(t *testing.T) {
		svc := testSvc()
		c, oapimock, lbmock := newAPI(t, self, []string{"foo"})
		expectVMs(oapimock, sdkSelf, sdkVM)
		expectNoLoadbalancer(oapimock)
		expectFindLBSubnetWithRole(oapimock)
		expectSGAlreadyExists(oapimock)
		expectFindWorkerSGByRole(oapimock)
		expectAddIngressSGRule(oapimock, []string{"0.0.0.0/0"}, "sg-foo")
		expectAddInternalSGRule(oapimock, "sg-foo", "sg-worker")
		expectCreateLoadBalancer(oapimock)
		expectConfigureHealthCheck(oapimock)
		expectDescribeProxyProtocol(lbmock, false)
		expectDescribeLoadBalancerAttributes(lbmock)
		expectRegisterInstances(oapimock, sdkVM.VmId)
		p := ccm.NewProviderWith(c, staticDNSResolver{}, ccm.Options{})
		_, err := p.EnsureLoadBalancer(t.Context(), "foo", svc, []*corev1.Node{&vmNode})
		require.ErrorIs(t, err, cloud.ErrLoadBalancerIsNotReady)
	})
	t.Run("A subnet is found, event if subnets are not tagged by owner", func(t *testing.T) {
		svc := testSvc()
		c, oapimock, lbmock := newAPI(t, self, []string{"foo"})
		expectVMs(oapimock, sdkSelf, sdkVM)
		expectNoLoadbalancer(oapimock)
		expectFindLBSubnetWithRoleWithNetFallback(oapimock)
		expectSGAlreadyExists(oapimock)
		expectFindWorkerSGByRole(oapimock)
		expectAddIngressSGRule(oapimock, []string{"0.0.0.0/0"}, "sg-foo")
		expectAddInternalSGRule(oapimock, "sg-foo", "sg-worker")
		expectCreateLoadBalancer(oapimock, func(clbr *osc.CreateLoadBalancerRequest) {
			clbr.Subnets = &[]string{"subnet-service"}
		})
		expectConfigureHealthCheck(oapimock)
		expectDescribeProxyProtocol(lbmock, false)
		expectDescribeLoadBalancerAttributes(lbmock)
		expectRegisterInstances(oapimock, sdkVM.VmId)
		p := ccm.NewProviderWith(c, staticDNSResolver{}, ccm.Options{})
		_, err := p.EnsureLoadBalancer(t.Context(), "foo", svc, []*corev1.Node{&vmNode})
		require.Error(t, err)
	})
	t.Run("A public LB is created, in a public subnet if subnets have no role", func(t *testing.T) {
		svc := testSvc()
		c, oapimock, lbmock := newAPI(t, self, []string{"foo"})
		expectVMs(oapimock, sdkSelf, sdkVM)
		expectNoLoadbalancer(oapimock)
		expectFindNoLBSubnetWithRole(oapimock)
		expectFindRouteTables(oapimock)
		expectSGAlreadyExists(oapimock)
		expectFindWorkerSGByRole(oapimock)
		expectAddIngressSGRule(oapimock, []string{"0.0.0.0/0"}, "sg-foo")
		expectAddInternalSGRule(oapimock, "sg-foo", "sg-worker")
		expectCreateLoadBalancer(oapimock, func(clbr *osc.CreateLoadBalancerRequest) {
			clbr.Subnets = &[]string{"subnet-public"}
		})
		expectConfigureHealthCheck(oapimock)
		expectDescribeProxyProtocol(lbmock, false)
		expectDescribeLoadBalancerAttributes(lbmock)
		expectRegisterInstances(oapimock, sdkVM.VmId)
		p := ccm.NewProviderWith(c, staticDNSResolver{}, ccm.Options{})
		_, err := p.EnsureLoadBalancer(t.Context(), "foo", svc, []*corev1.Node{&vmNode})
		require.Error(t, err)
	})
	t.Run("An internal LB is created, in a private subnet if subnets have no role", func(t *testing.T) {
		svc := testSvc()
		svc.Annotations["service.beta.kubernetes.io/osc-load-balancer-internal"] = "true"
		c, oapimock, lbmock := newAPI(t, self, []string{"foo"})
		expectVMs(oapimock, sdkSelf, sdkVM)
		expectNoLoadbalancer(oapimock)
		expectFindNoLBSubnetWithRole(oapimock)
		expectFindRouteTables(oapimock)
		expectSGAlreadyExists(oapimock)
		expectFindWorkerSGByRole(oapimock)
		expectAddIngressSGRule(oapimock, []string{"0.0.0.0/0"}, "sg-foo")
		expectAddInternalSGRule(oapimock, "sg-foo", "sg-worker")
		expectCreateLoadBalancer(oapimock, func(clbr *osc.CreateLoadBalancerRequest) {
			clbr.LoadBalancerType = ptr.To("internal")
			clbr.Subnets = &[]string{"subnet-private"}
		})
		expectConfigureHealthCheck(oapimock)
		expectDescribeProxyProtocol(lbmock, false)
		expectDescribeLoadBalancerAttributes(lbmock)
		expectRegisterInstances(oapimock, sdkVM.VmId)
		p := ccm.NewProviderWith(c, staticDNSResolver{}, ccm.Options{})
		_, err := p.EnsureLoadBalancer(t.Context(), "foo", svc, []*corev1.Node{&vmNode})
		require.Error(t, err)
	})
	t.Run("A public LB is created with a predefined public IP (by ID)", func(t *testing.T) {
		svc := testSvc()
		svc.Annotations["service.beta.kubernetes.io/osc-load-balancer-ip-id"] = "ipalloc-foo"
		c, oapimock, lbmock := newAPI(t, self, []string{"foo"})
		expectVMs(oapimock, sdkSelf, sdkVM)
		expectNoLoadbalancer(oapimock)
		expectPublicIP(oapimock, "ipalloc-foo", &osc.PublicIp{PublicIp: "1.2.3.4"})
		expectFindLBSubnetWithRole(oapimock)
		expectCreateSecurityGroup(oapimock)
		expectFindWorkerSGByRole(oapimock)
		expectAddIngressSGRule(oapimock, []string{"0.0.0.0/0"}, "sg-foo")
		expectAddInternalSGRule(oapimock, "sg-foo", "sg-worker")
		expectCreateLoadBalancer(oapimock, func(clbr *osc.CreateLoadBalancerRequest) {
			clbr.PublicIp = ptr.To("1.2.3.4")
		})
		expectConfigureHealthCheck(oapimock)
		expectDescribeProxyProtocol(lbmock, false)
		expectDescribeLoadBalancerAttributes(lbmock)
		expectRegisterInstances(oapimock, sdkVM.VmId)
		p := ccm.NewProviderWith(c, staticDNSResolver{}, ccm.Options{})
		_, err := p.EnsureLoadBalancer(t.Context(), "foo", svc, []*corev1.Node{&vmNode})
		require.ErrorIs(t, err, cloud.ErrLoadBalancerIsNotReady)
	})
	t.Run("A public LB is created with a predefined public IP (by IP)", func(t *testing.T) {
		svc := testSvc()
		svc.Annotations["service.beta.kubernetes.io/osc-load-balancer-ip-id"] = "1.2.3.4"
		c, oapimock, lbmock := newAPI(t, self, []string{"foo"})
		expectVMs(oapimock, sdkSelf, sdkVM)
		expectNoLoadbalancer(oapimock)
		expectFindLBSubnetWithRole(oapimock)
		expectCreateSecurityGroup(oapimock)
		expectFindWorkerSGByRole(oapimock)
		expectAddIngressSGRule(oapimock, []string{"0.0.0.0/0"}, "sg-foo")
		expectAddInternalSGRule(oapimock, "sg-foo", "sg-worker")
		expectCreateLoadBalancer(oapimock, func(clbr *osc.CreateLoadBalancerRequest) {
			clbr.PublicIp = ptr.To("1.2.3.4")
		})
		expectConfigureHealthCheck(oapimock)
		expectDescribeProxyProtocol(lbmock, false)
		expectDescribeLoadBalancerAttributes(lbmock)
		expectRegisterInstances(oapimock, sdkVM.VmId)
		p := ccm.NewProviderWith(c, staticDNSResolver{}, ccm.Options{})
		_, err := p.EnsureLoadBalancer(t.Context(), "foo", svc, []*corev1.Node{&vmNode})
		require.ErrorIs(t, err, cloud.ErrLoadBalancerIsNotReady)
	})
	t.Run("A public LB is created with a public IP from a pool", func(t *testing.T) {
		svc := testSvc()
		svc.Annotations["service.beta.kubernetes.io/osc-load-balancer-ip-pool"] = "pool-foo"
		c, oapimock, lbmock := newAPI(t, self, []string{"foo"})
		expectVMs(oapimock, sdkSelf, sdkVM)
		expectNoLoadbalancer(oapimock)
		expectPublicIPFromPool(oapimock, []osc.PublicIp{
			{PublicIpId: "ipalloc-foo", PublicIp: "1.2.3.4"},
		})
		expectFindLBSubnetWithRole(oapimock)
		expectCreateSecurityGroup(oapimock)
		expectFindWorkerSGByRole(oapimock)
		expectAddIngressSGRule(oapimock, []string{"0.0.0.0/0"}, "sg-foo")
		expectAddInternalSGRule(oapimock, "sg-foo", "sg-worker")
		expectCreateLoadBalancer(oapimock, func(clbr *osc.CreateLoadBalancerRequest) {
			clbr.PublicIp = ptr.To("1.2.3.4")
		})
		expectConfigureHealthCheck(oapimock)
		expectDescribeProxyProtocol(lbmock, false)
		expectDescribeLoadBalancerAttributes(lbmock)
		expectRegisterInstances(oapimock, sdkVM.VmId)
		p := ccm.NewProviderWith(c, staticDNSResolver{}, ccm.Options{})
		_, err := p.EnsureLoadBalancer(t.Context(), "foo", svc, []*corev1.Node{&vmNode})
		require.ErrorIs(t, err, cloud.ErrLoadBalancerIsNotReady)
	})
	t.Run("If pool is empty, no LB is created", func(t *testing.T) {
		svc := testSvc()
		svc.Annotations["service.beta.kubernetes.io/osc-load-balancer-ip-pool"] = "pool-foo"
		c, oapimock, _ := newAPI(t, self, []string{"foo"})
		expectVMs(oapimock, sdkSelf, sdkVM)
		expectNoLoadbalancer(oapimock)
		expectFindLBSubnetWithRole(oapimock)
		expectPublicIPFromPool(oapimock, nil)
		p := ccm.NewProviderWith(c, staticDNSResolver{}, ccm.Options{})
		_, err := p.EnsureLoadBalancer(t.Context(), "foo", svc, []*corev1.Node{&vmNode})
		require.Error(t, err)
	})
	t.Run("If all IPs have been allocated, no LB is created", func(t *testing.T) {
		svc := testSvc()
		svc.Annotations["service.beta.kubernetes.io/osc-load-balancer-ip-pool"] = "pool-foo"
		c, oapimock, _ := newAPI(t, self, []string{"foo"})
		expectVMs(oapimock, sdkSelf, sdkVM)
		expectNoLoadbalancer(oapimock)
		expectFindLBSubnetWithRole(oapimock)
		expectPublicIPFromPool(oapimock, []osc.PublicIp{
			{PublicIpId: "ip-foo", LinkPublicIpId: ptr.To("ipassoc-foo"), PublicIp: "1.2.3.4"},
		})
		p := ccm.NewProviderWith(c, staticDNSResolver{}, ccm.Options{})
		_, err := p.EnsureLoadBalancer(t.Context(), "foo", svc, []*corev1.Node{&vmNode})
		require.Error(t, err)
	})
	t.Run("An internal LB is created and a retryable error is returned when not ready", func(t *testing.T) {
		svc := testSvc()
		svc.Annotations["service.beta.kubernetes.io/osc-load-balancer-internal"] = "true"
		c, oapimock, lbmock := newAPI(t, self, []string{"foo"})
		expectVMs(oapimock, sdkSelf, sdkVM)
		expectNoLoadbalancer(oapimock)
		expectFindLBSubnetWithRole(oapimock)
		expectCreateSecurityGroup(oapimock)
		expectFindWorkerSGByRole(oapimock)
		expectAddIngressSGRule(oapimock, []string{"0.0.0.0/0"}, "sg-foo")
		expectAddInternalSGRule(oapimock, "sg-foo", "sg-worker")
		expectCreateLoadBalancer(oapimock, func(req *osc.CreateLoadBalancerRequest) {
			req.LoadBalancerType = ptr.To("internal")
			req.Subnets = &[]string{"subnet-service.internal"}
		})
		expectConfigureHealthCheck(oapimock)
		expectDescribeProxyProtocol(lbmock, false)
		expectDescribeLoadBalancerAttributes(lbmock)
		expectRegisterInstances(oapimock, sdkVM.VmId)
		p := ccm.NewProviderWith(c, staticDNSResolver{}, ccm.Options{})
		_, err := p.EnsureLoadBalancer(t.Context(), "foo", svc, []*corev1.Node{&vmNode})
		require.ErrorIs(t, err, cloud.ErrLoadBalancerIsNotReady)
	})
	t.Run("IP restriction may be configured", func(t *testing.T) {
		svc := testSvc()
		svc.Spec.LoadBalancerSourceRanges = []string{"198.51.100.0/24", "203.0.113.0/24"}
		c, oapimock, lbmock := newAPI(t, self, []string{"foo"})
		expectVMs(oapimock, sdkSelf, sdkVM)
		expectNoLoadbalancer(oapimock)
		expectFindLBSubnetWithRole(oapimock)
		expectCreateSecurityGroup(oapimock)
		expectFindWorkerSGByRole(oapimock)
		expectAddIngressSGRule(oapimock, svc.Spec.LoadBalancerSourceRanges, "sg-foo")
		expectAddInternalSGRule(oapimock, "sg-foo", "sg-worker")
		expectCreateLoadBalancer(oapimock)
		expectConfigureHealthCheck(oapimock)
		expectDescribeProxyProtocol(lbmock, false)
		expectDescribeLoadBalancerAttributes(lbmock)
		expectRegisterInstances(oapimock, sdkVM.VmId)
		p := ccm.NewProviderWith(c, staticDNSResolver{}, ccm.Options{})
		_, err := p.EnsureLoadBalancer(t.Context(), "foo", svc, []*corev1.Node{&vmNode})
		require.ErrorIs(t, err, cloud.ErrLoadBalancerIsNotReady)
	})
	t.Run("The LB SG can be set", func(t *testing.T) {
		svc := testSvc()
		svc.Annotations["service.beta.kubernetes.io/osc-load-balancer-security-group"] = "sg-existing"
		c, oapimock, lbmock := newAPI(t, self, []string{"foo"})
		expectVMs(oapimock, sdkSelf, sdkVM)
		expectNoLoadbalancer(oapimock)
		expectFindLBSubnetWithRole(oapimock)
		expectFindIngressSecurityGroup(oapimock, "sg-existing")
		expectFindWorkerSGByRole(oapimock)
		expectAddIngressSGRule(oapimock, []string{"0.0.0.0/0"}, "sg-existing")
		expectAddInternalSGRule(oapimock, "sg-existing", "sg-worker")
		expectCreateLoadBalancer(oapimock, func(req *osc.CreateLoadBalancerRequest) {
			req.SecurityGroups = &[]string{"sg-existing"}
		})
		expectConfigureHealthCheck(oapimock)
		expectDescribeProxyProtocol(lbmock, false)
		expectDescribeLoadBalancerAttributes(lbmock)
		expectRegisterInstances(oapimock, sdkVM.VmId)
		p := ccm.NewProviderWith(c, staticDNSResolver{}, ccm.Options{})
		_, err := p.EnsureLoadBalancer(t.Context(), "foo", svc, []*corev1.Node{&vmNode})
		require.ErrorIs(t, err, cloud.ErrLoadBalancerIsNotReady)
	})
	t.Run("A different role may be targeted", func(t *testing.T) {
		svc := testSvc()
		svc.Annotations["service.beta.kubernetes.io/osc-load-balancer-target-role"] = "controlplane"
		c, oapimock, lbmock := newAPI(t, self, []string{"foo"})
		expectVMs(oapimock, sdkSelf, sdkVM)
		expectNoLoadbalancer(oapimock)
		expectFindLBSubnetWithRole(oapimock)
		expectCreateSecurityGroup(oapimock)
		expectFindWorkerSGByRole(oapimock)
		expectAddIngressSGRule(oapimock, []string{"0.0.0.0/0"}, "sg-foo")
		expectAddInternalSGRule(oapimock, "sg-foo", "sg-controlplane")
		expectCreateLoadBalancer(oapimock)
		expectConfigureHealthCheck(oapimock)
		expectDescribeProxyProtocol(lbmock, false)
		expectDescribeLoadBalancerAttributes(lbmock)
		expectRegisterInstances(oapimock, sdkVM.VmId)
		p := ccm.NewProviderWith(c, staticDNSResolver{}, ccm.Options{})
		_, err := p.EnsureLoadBalancer(t.Context(), "foo", svc, []*corev1.Node{&vmNode})
		require.ErrorIs(t, err, cloud.ErrLoadBalancerIsNotReady)
	})
	t.Run("The LB subnet can be set", func(t *testing.T) {
		svc := testSvc()
		svc.Annotations["service.beta.kubernetes.io/osc-load-balancer-subnet-id"] = "subnet-existing"
		c, oapimock, lbmock := newAPI(t, self, []string{"foo"})
		expectVMs(oapimock, sdkSelf, sdkVM)
		expectNoLoadbalancer(oapimock)
		expectFindExistingSubnet(oapimock, "subnet-existing")
		expectCreateSecurityGroup(oapimock)
		expectFindWorkerSGByRole(oapimock)
		expectAddIngressSGRule(oapimock, []string{"0.0.0.0/0"}, "sg-foo")
		expectAddInternalSGRule(oapimock, "sg-foo", "sg-worker")
		expectCreateLoadBalancer(oapimock, func(req *osc.CreateLoadBalancerRequest) {
			req.Subnets = &[]string{"subnet-existing"}
		})
		expectConfigureHealthCheck(oapimock)
		expectDescribeProxyProtocol(lbmock, false)
		expectDescribeLoadBalancerAttributes(lbmock)
		expectRegisterInstances(oapimock, sdkVM.VmId)
		p := ccm.NewProviderWith(c, staticDNSResolver{}, ccm.Options{})
		_, err := p.EnsureLoadBalancer(t.Context(), "foo", svc, []*corev1.Node{&vmNode})
		require.ErrorIs(t, err, cloud.ErrLoadBalancerIsNotReady)
	})
	t.Run("HTTP SSL termination can be set on the LB", func(t *testing.T) {
		svc := testSvc()
		svc.Annotations["service.beta.kubernetes.io/osc-load-balancer-ssl-cert"] = "arn:aws:service:region:account:resource"
		svc.Annotations["service.beta.kubernetes.io/osc-load-balancer-backend-protocol"] = "http"
		svc.Spec.Ports[0].Port = 443
		c, oapimock, lbmock := newAPI(t, self, []string{"foo"})
		expectVMs(oapimock, sdkSelf, sdkVM)
		expectNoLoadbalancer(oapimock)
		expectFindLBSubnetWithRole(oapimock)
		expectCreateSecurityGroup(oapimock)
		expectFindWorkerSGByRole(oapimock)
		expectAddIngressSGRule(oapimock, []string{"0.0.0.0/0"}, "sg-foo", func(req *osc.CreateSecurityGroupRuleRequest) {
			req.Rules[0].FromPortRange = 443
			req.Rules[0].ToPortRange = 443
		})
		expectAddInternalSGRule(oapimock, "sg-foo", "sg-worker")
		expectCreateLoadBalancer(oapimock, func(clbi *osc.CreateLoadBalancerRequest) {
			clbi.Listeners[0].LoadBalancerPort = 443
			clbi.Listeners[0].LoadBalancerProtocol = "HTTPS"
			clbi.Listeners[0].BackendProtocol = ptr.To("HTTP")
			clbi.Listeners[0].ServerCertificateId = ptr.To("arn:aws:service:region:account:resource")
		})
		expectConfigureHealthCheck(oapimock)
		expectDescribeProxyProtocol(lbmock, false)
		expectDescribeLoadBalancerAttributes(lbmock)
		expectRegisterInstances(oapimock, sdkVM.VmId)
		p := ccm.NewProviderWith(c, staticDNSResolver{}, ccm.Options{})
		_, err := p.EnsureLoadBalancer(t.Context(), "foo", svc, []*corev1.Node{&vmNode})
		require.ErrorIs(t, err, cloud.ErrLoadBalancerIsNotReady)
	})
	t.Run("HTTP SSL termination can be set on a single port", func(t *testing.T) {
		svc := testSvc()
		svc.Annotations["service.beta.kubernetes.io/osc-load-balancer-ssl-cert"] = "arn:aws:service:region:account:resource"
		svc.Annotations["service.beta.kubernetes.io/osc-load-balancer-ssl-ports"] = "443"
		svc.Annotations["service.beta.kubernetes.io/osc-load-balancer-backend-protocol"] = "http"
		svc.Spec.Ports = append(svc.Spec.Ports, corev1.ServicePort{
			Protocol: corev1.ProtocolTCP,
			NodePort: 8080,
			Port:     443,
		})
		c, oapimock, lbmock := newAPI(t, self, []string{"foo"})
		expectVMs(oapimock, sdkSelf, sdkVM)
		expectNoLoadbalancer(oapimock)
		expectFindLBSubnetWithRole(oapimock)
		expectCreateSecurityGroup(oapimock)
		expectFindWorkerSGByRole(oapimock)
		expectAddIngressSGRule(oapimock, []string{"0.0.0.0/0"}, "sg-foo")
		expectAddIngressSGRule(oapimock, []string{"0.0.0.0/0"}, "sg-foo", func(req *osc.CreateSecurityGroupRuleRequest) {
			req.Rules[0].FromPortRange = 443
			req.Rules[0].ToPortRange = 443
		})
		expectAddInternalSGRule(oapimock, "sg-foo", "sg-worker")
		expectAddInternalSGRule(oapimock, "sg-foo", "sg-worker")
		expectCreateLoadBalancer(oapimock, func(req *osc.CreateLoadBalancerRequest) {
			req.Listeners[0].LoadBalancerProtocol = "HTTP"
			req.Listeners[0].BackendProtocol = ptr.To("HTTP")
			req.Listeners = append(req.Listeners, osc.ListenerForCreation{
				LoadBalancerPort:     443,
				LoadBalancerProtocol: "HTTPS",
				BackendPort:          8080,
				BackendProtocol:      ptr.To("HTTP"),
				ServerCertificateId:  ptr.To("arn:aws:service:region:account:resource"),
			})
		})
		expectConfigureHealthCheck(oapimock)
		expectDescribeProxyProtocol(lbmock, false)
		expectDescribeLoadBalancerAttributes(lbmock)
		expectRegisterInstances(oapimock, sdkVM.VmId)
		p := ccm.NewProviderWith(c, staticDNSResolver{}, ccm.Options{})
		_, err := p.EnsureLoadBalancer(t.Context(), "foo", svc, []*corev1.Node{&vmNode})
		require.ErrorIs(t, err, cloud.ErrLoadBalancerIsNotReady)
	})
	t.Run("TCP SSL termination can be set on the LB", func(t *testing.T) {
		svc := testSvc()
		svc.Annotations["service.beta.kubernetes.io/osc-load-balancer-ssl-cert"] = "arn:aws:service:region:account:resource"
		svc.Spec.Ports[0].Port = 465
		c, oapimock, lbmock := newAPI(t, self, []string{"foo"})
		expectVMs(oapimock, sdkSelf, sdkVM)
		expectNoLoadbalancer(oapimock)
		expectFindLBSubnetWithRole(oapimock)
		expectCreateSecurityGroup(oapimock)
		expectFindWorkerSGByRole(oapimock)
		expectAddIngressSGRule(oapimock, []string{"0.0.0.0/0"}, "sg-foo", func(req *osc.CreateSecurityGroupRuleRequest) {
			req.Rules[0].FromPortRange = 465
			req.Rules[0].ToPortRange = 465
		})
		expectAddInternalSGRule(oapimock, "sg-foo", "sg-worker")
		expectCreateLoadBalancer(oapimock, func(clbi *osc.CreateLoadBalancerRequest) {
			clbi.Listeners[0].LoadBalancerPort = 465
			clbi.Listeners[0].LoadBalancerProtocol = "SSL"
			clbi.Listeners[0].BackendProtocol = ptr.To("TCP")
			clbi.Listeners[0].ServerCertificateId = ptr.To("arn:aws:service:region:account:resource")
		})
		expectConfigureHealthCheck(oapimock)
		expectDescribeProxyProtocol(lbmock, false)
		expectDescribeLoadBalancerAttributes(lbmock)
		expectRegisterInstances(oapimock, sdkVM.VmId)
		p := ccm.NewProviderWith(c, staticDNSResolver{}, ccm.Options{})
		_, err := p.EnsureLoadBalancer(t.Context(), "foo", svc, []*corev1.Node{&vmNode})
		require.ErrorIs(t, err, cloud.ErrLoadBalancerIsNotReady)
	})
	t.Run("backend protocol can be set", func(t *testing.T) {
		svc := testSvc()
		svc.Annotations["service.beta.kubernetes.io/osc-load-balancer-backend-protocol"] = "tcp"
		c, oapimock, lbmock := newAPI(t, self, []string{"foo"})
		expectVMs(oapimock, sdkSelf, sdkVM)
		expectNoLoadbalancer(oapimock)
		expectFindLBSubnetWithRole(oapimock)
		expectCreateSecurityGroup(oapimock)
		expectFindWorkerSGByRole(oapimock)
		expectAddIngressSGRule(oapimock, []string{"0.0.0.0/0"}, "sg-foo")
		expectAddInternalSGRule(oapimock, "sg-foo", "sg-worker")
		expectCreateLoadBalancer(oapimock, func(clbi *osc.CreateLoadBalancerRequest) {
			clbi.Listeners[0].LoadBalancerProtocol = "TCP"
			clbi.Listeners[0].BackendProtocol = ptr.To("TCP")
		})
		expectConfigureHealthCheck(oapimock)
		expectDescribeProxyProtocol(lbmock, false)
		expectDescribeLoadBalancerAttributes(lbmock)
		expectRegisterInstances(oapimock, sdkVM.VmId)
		p := ccm.NewProviderWith(c, staticDNSResolver{}, ccm.Options{})
		_, err := p.EnsureLoadBalancer(t.Context(), "foo", svc, []*corev1.Node{&vmNode})
		require.ErrorIs(t, err, cloud.ErrLoadBalancerIsNotReady)
	})
	t.Run("proxy protocol can be set on all ports", func(t *testing.T) {
		svc := testSvc()
		svc.Annotations["service.beta.kubernetes.io/osc-load-balancer-proxy-protocol"] = "*"
		svc.Spec.Ports = append(svc.Spec.Ports, corev1.ServicePort{
			Protocol: corev1.ProtocolTCP,
			NodePort: 8443,
			Port:     443,
		})
		c, oapimock, lbmock := newAPI(t, self, []string{"foo"})
		expectVMs(oapimock, sdkSelf, sdkVM)
		expectNoLoadbalancer(oapimock)
		expectFindLBSubnetWithRole(oapimock)
		expectCreateSecurityGroup(oapimock)
		expectFindWorkerSGByRole(oapimock)
		expectAddIngressSGRule(oapimock, []string{"0.0.0.0/0"}, "sg-foo")
		expectAddIngressSGRule(oapimock, []string{"0.0.0.0/0"}, "sg-foo", func(req *osc.CreateSecurityGroupRuleRequest) {
			req.Rules[0].FromPortRange = 443
			req.Rules[0].ToPortRange = 443
		})
		expectAddInternalSGRule(oapimock, "sg-foo", "sg-worker")
		expectAddInternalSGRule(oapimock, "sg-foo", "sg-worker", func(req *osc.CreateSecurityGroupRuleRequest) {
			req.Rules[0].FromPortRange = 8443
			req.Rules[0].ToPortRange = 8443
		})
		expectCreateLoadBalancer(oapimock, func(req *osc.CreateLoadBalancerRequest) {
			req.Listeners = append(req.Listeners, osc.ListenerForCreation{
				LoadBalancerPort:     443,
				LoadBalancerProtocol: "TCP",
				BackendPort:          8443,
				BackendProtocol:      ptr.To("TCP"),
			})
		})
		expectConfigureHealthCheck(oapimock)
		expectDescribeProxyProtocol(lbmock, false, 8080, 8443)
		expectConfigureProxyProtocol(lbmock, false, true, 8080, 8443)
		expectDescribeLoadBalancerAttributes(lbmock)
		expectRegisterInstances(oapimock, sdkVM.VmId)
		p := ccm.NewProviderWith(c, staticDNSResolver{}, ccm.Options{})
		_, err := p.EnsureLoadBalancer(t.Context(), "foo", svc, []*corev1.Node{&vmNode})
		require.ErrorIs(t, err, cloud.ErrLoadBalancerIsNotReady)
	})
	t.Run("proxy protocol can be set on a single port", func(t *testing.T) {
		svc := testSvc()
		svc.Annotations["service.beta.kubernetes.io/osc-load-balancer-proxy-protocol"] = "8080"
		svc.Spec.Ports = append(svc.Spec.Ports, corev1.ServicePort{
			Protocol: corev1.ProtocolTCP,
			NodePort: 8443,
			Port:     443,
		})
		c, oapimock, lbmock := newAPI(t, self, []string{"foo"})
		expectVMs(oapimock, sdkSelf, sdkVM)
		expectNoLoadbalancer(oapimock)
		expectFindLBSubnetWithRole(oapimock)
		expectCreateSecurityGroup(oapimock)
		expectFindWorkerSGByRole(oapimock)
		expectAddIngressSGRule(oapimock, []string{"0.0.0.0/0"}, "sg-foo")
		expectAddIngressSGRule(oapimock, []string{"0.0.0.0/0"}, "sg-foo", func(req *osc.CreateSecurityGroupRuleRequest) {
			req.Rules[0].FromPortRange = 443
			req.Rules[0].ToPortRange = 443
		})
		expectAddInternalSGRule(oapimock, "sg-foo", "sg-worker")
		expectAddInternalSGRule(oapimock, "sg-foo", "sg-worker", func(req *osc.CreateSecurityGroupRuleRequest) {
			req.Rules[0].FromPortRange = 8443
			req.Rules[0].ToPortRange = 8443
		})
		expectCreateLoadBalancer(oapimock, func(req *osc.CreateLoadBalancerRequest) {
			req.Listeners = append(req.Listeners, osc.ListenerForCreation{
				LoadBalancerPort:     443,
				LoadBalancerProtocol: "TCP",
				BackendPort:          8443,
				BackendProtocol:      ptr.To("TCP"),
			})
		})
		expectConfigureHealthCheck(oapimock)
		expectDescribeProxyProtocol(lbmock, false)
		expectConfigureProxyProtocol(lbmock, false, true)
		expectDescribeLoadBalancerAttributes(lbmock)
		expectRegisterInstances(oapimock, sdkVM.VmId)
		p := ccm.NewProviderWith(c, staticDNSResolver{}, ccm.Options{})
		_, err := p.EnsureLoadBalancer(t.Context(), "foo", svc, []*corev1.Node{&vmNode})
		require.ErrorIs(t, err, cloud.ErrLoadBalancerIsNotReady)
	})
	t.Run("logs can be stored", func(t *testing.T) {
		svc := testSvc()
		svc.Annotations["service.beta.kubernetes.io/osc-load-balancer-access-log-enabled"] = "true"
		svc.Annotations["service.beta.kubernetes.io/osc-load-balancer-access-log-emit-interval"] = "30"
		svc.Annotations["service.beta.kubernetes.io/osc-load-balancer-access-log-oos-bucket-name"] = "bucket"
		svc.Annotations["service.beta.kubernetes.io/osc-load-balancer-access-log-oos-bucket-prefix"] = "prefix"
		c, oapimock, lbmock := newAPI(t, self, []string{"foo"})
		expectVMs(oapimock, sdkSelf, sdkVM)
		expectNoLoadbalancer(oapimock)
		expectFindLBSubnetWithRole(oapimock)
		expectCreateSecurityGroup(oapimock)
		expectFindWorkerSGByRole(oapimock)
		expectAddIngressSGRule(oapimock, []string{"0.0.0.0/0"}, "sg-foo")
		expectAddInternalSGRule(oapimock, "sg-foo", "sg-worker")
		expectCreateLoadBalancer(oapimock)
		expectConfigureHealthCheck(oapimock)
		expectDescribeProxyProtocol(lbmock, false)
		expectDescribeLoadBalancerAttributes(lbmock)
		expectModifyLoadBalancerAttributes(lbmock, &elb.LoadBalancerAttributes{
			AccessLog: &elb.AccessLog{
				Enabled:        ptr.To(true),
				EmitInterval:   ptr.To[int64](30),
				S3BucketName:   ptr.To("bucket"),
				S3BucketPrefix: ptr.To("prefix"),
			},
		})
		expectRegisterInstances(oapimock, sdkVM.VmId)
		p := ccm.NewProviderWith(c, staticDNSResolver{}, ccm.Options{})
		_, err := p.EnsureLoadBalancer(t.Context(), "foo", svc, []*corev1.Node{&vmNode})
		require.ErrorIs(t, err, cloud.ErrLoadBalancerIsNotReady)
	})
	t.Run("Nodes can be filtered", func(t *testing.T) {
		svc := testSvc()
		svc.Annotations["service.beta.kubernetes.io/osc-load-balancer-target-node-labels"] = "key=val"
		vmNode1 := vmNode
		vmNode1.Labels = map[string]string{"key": "val"}
		vmNode2 := vmNode
		vmNode2.Name = "10.0.0.11.eu-west-2.compute.internal"
		c, oapimock, lbmock := newAPI(t, self, []string{"foo"})
		expectVMs(oapimock, sdkSelf, sdkVM)
		expectNoLoadbalancer(oapimock)
		expectFindLBSubnetWithRole(oapimock)
		expectCreateSecurityGroup(oapimock)
		expectFindWorkerSGByRole(oapimock)
		expectAddIngressSGRule(oapimock, []string{"0.0.0.0/0"}, "sg-foo")
		expectAddInternalSGRule(oapimock, "sg-foo", "sg-worker")
		expectCreateLoadBalancer(oapimock)
		expectConfigureHealthCheck(oapimock)
		expectDescribeProxyProtocol(lbmock, false)
		expectDescribeLoadBalancerAttributes(lbmock)
		expectRegisterInstances(oapimock, sdkVM.VmId)
		p := ccm.NewProviderWith(c, staticDNSResolver{}, ccm.Options{})
		_, err := p.EnsureLoadBalancer(t.Context(), "foo", svc, []*corev1.Node{&vmNode1, &vmNode2})
		require.ErrorIs(t, err, cloud.ErrLoadBalancerIsNotReady)
	})
	t.Run("HTTP health checks can be configured", func(t *testing.T) {
		svc := testSvc()
		svc.Annotations["service.beta.kubernetes.io/osc-load-balancer-healthcheck-healthy-threshold"] = "42"
		svc.Annotations["service.beta.kubernetes.io/osc-load-balancer-healthcheck-unhealthy-threshold"] = "43"
		svc.Annotations["service.beta.kubernetes.io/osc-load-balancer-healthcheck-timeout"] = "44"
		svc.Annotations["service.beta.kubernetes.io/osc-load-balancer-healthcheck-interval"] = "45"
		svc.Annotations["service.beta.kubernetes.io/osc-load-balancer-healthcheck-port"] = "46"
		svc.Annotations["service.beta.kubernetes.io/osc-load-balancer-healthcheck-protocol"] = "http"
		svc.Annotations["service.beta.kubernetes.io/osc-load-balancer-healthcheck-path"] = "/healthz"
		c, oapimock, lbmock := newAPI(t, self, []string{"foo"})
		expectVMs(oapimock, sdkSelf, sdkVM)
		expectNoLoadbalancer(oapimock)
		expectFindLBSubnetWithRole(oapimock)
		expectCreateSecurityGroup(oapimock)
		expectFindWorkerSGByRole(oapimock)
		expectAddIngressSGRule(oapimock, []string{"0.0.0.0/0"}, "sg-foo")
		expectAddInternalSGRule(oapimock, "sg-foo", "sg-worker")
		expectCreateLoadBalancer(oapimock)
		expectConfigureHealthCheck(oapimock, &osc.HealthCheck{
			HealthyThreshold:   42,
			UnhealthyThreshold: 43,
			Timeout:            44,
			CheckInterval:      45,
			Port:               46,
			Protocol:           "HTTP",
			Path:               ptr.To("/healthz"),
		})
		expectDescribeProxyProtocol(lbmock, false)
		expectDescribeLoadBalancerAttributes(lbmock)
		expectRegisterInstances(oapimock, sdkVM.VmId)
		p := ccm.NewProviderWith(c, staticDNSResolver{}, ccm.Options{})
		_, err := p.EnsureLoadBalancer(t.Context(), "foo", svc, []*corev1.Node{&vmNode})
		require.ErrorIs(t, err, cloud.ErrLoadBalancerIsNotReady)
	})
	t.Run("HTTPs health checks can be configured", func(t *testing.T) {
		svc := testSvc()
		svc.Annotations["service.beta.kubernetes.io/osc-load-balancer-healthcheck-healthy-threshold"] = "42"
		svc.Annotations["service.beta.kubernetes.io/osc-load-balancer-healthcheck-unhealthy-threshold"] = "43"
		svc.Annotations["service.beta.kubernetes.io/osc-load-balancer-healthcheck-timeout"] = "44"
		svc.Annotations["service.beta.kubernetes.io/osc-load-balancer-healthcheck-interval"] = "45"
		svc.Annotations["service.beta.kubernetes.io/osc-load-balancer-healthcheck-port"] = "46"
		svc.Annotations["service.beta.kubernetes.io/osc-load-balancer-healthcheck-protocol"] = "https"
		svc.Annotations["service.beta.kubernetes.io/osc-load-balancer-healthcheck-path"] = "/healthz"
		c, oapimock, lbmock := newAPI(t, self, []string{"foo"})
		expectVMs(oapimock, sdkSelf, sdkVM)
		expectNoLoadbalancer(oapimock)
		expectFindLBSubnetWithRole(oapimock)
		expectCreateSecurityGroup(oapimock)
		expectFindWorkerSGByRole(oapimock)
		expectAddIngressSGRule(oapimock, []string{"0.0.0.0/0"}, "sg-foo")
		expectAddInternalSGRule(oapimock, "sg-foo", "sg-worker")
		expectCreateLoadBalancer(oapimock)
		expectConfigureHealthCheck(oapimock, &osc.HealthCheck{
			HealthyThreshold:   42,
			UnhealthyThreshold: 43,
			Timeout:            44,
			CheckInterval:      45,
			Port:               46,
			Protocol:           "HTTPS",
			Path:               ptr.To("/healthz"),
		})
		expectDescribeProxyProtocol(lbmock, false)
		expectDescribeLoadBalancerAttributes(lbmock)
		expectRegisterInstances(oapimock, sdkVM.VmId)
		p := ccm.NewProviderWith(c, staticDNSResolver{}, ccm.Options{})
		_, err := p.EnsureLoadBalancer(t.Context(), "foo", svc, []*corev1.Node{&vmNode})
		require.ErrorIs(t, err, cloud.ErrLoadBalancerIsNotReady)
	})
}

type staticDNSResolver struct{}

func (staticDNSResolver) LookupHost(ctx context.Context, hostname string) ([]string, error) {
	return []string{"10.0.0.1"}, nil
}

func TestEnsureLoadBalancer_Update(t *testing.T) {
	t.Run("When retrying creation, the status is properly returned when ready with only the hostname", func(t *testing.T) {
		svc := testSvc()
		c, oapimock, lbmock := newAPI(t, self, []string{"foo"})
		expectLoadbalancerExistsAndOwned(oapimock)
		expectVMs(oapimock, sdkSelf, sdkVM)
		expectReadLoadBalancer(oapimock, func(desc *osc.LoadBalancer) {
			desc.DnsName = "foo.example.com"
			desc.BackendVmIds = []string{"i-foo"}
			desc.PublicIp = ptr.To("198.51.100.42")
		})
		expectDescribeProxyProtocol(lbmock, false)
		expectDescribeLoadBalancerAttributes(lbmock)
		expectFindExistingIngressSecurityGroup(oapimock, "sg-foo")
		expectFindExistingWorkerSG(oapimock)
		p := ccm.NewProviderWith(c, staticDNSResolver{}, ccm.Options{})
		status, err := p.EnsureLoadBalancer(t.Context(), "foo", svc, []*corev1.Node{&vmNode})
		require.NoError(t, err)
		assert.Equal(t, &corev1.LoadBalancerStatus{Ingress: []corev1.LoadBalancerIngress{{Hostname: "foo.example.com"}}}, status)
	})
	t.Run("When retrying creation, the status is properly returned when ready, with only the IP", func(t *testing.T) {
		svc := testSvc()
		svc.Annotations["service.beta.kubernetes.io/osc-load-balancer-ingress-address"] = "ip"
		c, oapimock, lbmock := newAPI(t, self, []string{"foo"})
		expectLoadbalancerExistsAndOwned(oapimock)
		expectVMs(oapimock, sdkSelf, sdkVM)
		expectReadLoadBalancer(oapimock, func(desc *osc.LoadBalancer) {
			desc.DnsName = "foo.example.com"
			desc.BackendVmIds = []string{"i-foo"}
			desc.PublicIp = ptr.To("198.51.100.42")
		})
		expectDescribeProxyProtocol(lbmock, false)
		expectDescribeLoadBalancerAttributes(lbmock)
		expectFindExistingIngressSecurityGroup(oapimock, "sg-foo")
		expectFindExistingWorkerSG(oapimock)
		p := ccm.NewProviderWith(c, staticDNSResolver{}, ccm.Options{})
		status, err := p.EnsureLoadBalancer(t.Context(), "foo", svc, []*corev1.Node{&vmNode})
		require.NoError(t, err)
		assert.Equal(t, &corev1.LoadBalancerStatus{Ingress: []corev1.LoadBalancerIngress{{IP: "198.51.100.42", IPMode: ptr.To(corev1.LoadBalancerIPModeProxy)}}}, status)
	})
	t.Run("When retrying creation, the status is properly returned when ready, with only the nostname", func(t *testing.T) {
		svc := testSvc()
		svc.Annotations["service.beta.kubernetes.io/osc-load-balancer-ingress-address"] = "hostname"
		c, oapimock, lbmock := newAPI(t, self, []string{"foo"})
		expectLoadbalancerExistsAndOwned(oapimock)
		expectVMs(oapimock, sdkSelf, sdkVM)
		expectReadLoadBalancer(oapimock, func(desc *osc.LoadBalancer) {
			desc.DnsName = "foo.example.com"
			desc.BackendVmIds = []string{"i-foo"}
			desc.PublicIp = ptr.To("198.51.100.42")
		})
		expectDescribeProxyProtocol(lbmock, false)
		expectDescribeLoadBalancerAttributes(lbmock)
		expectFindExistingIngressSecurityGroup(oapimock, "sg-foo")
		expectFindExistingWorkerSG(oapimock)
		p := ccm.NewProviderWith(c, staticDNSResolver{}, ccm.Options{})
		status, err := p.EnsureLoadBalancer(t.Context(), "foo", svc, []*corev1.Node{&vmNode})
		require.NoError(t, err)
		assert.Equal(t, &corev1.LoadBalancerStatus{Ingress: []corev1.LoadBalancerIngress{{Hostname: "foo.example.com"}}}, status)
	})
	t.Run("When retrying creation, the status is properly returned when ready, with both IP and hostname", func(t *testing.T) {
		svc := testSvc()
		svc.Annotations["service.beta.kubernetes.io/osc-load-balancer-ingress-address"] = "both"
		c, oapimock, lbmock := newAPI(t, self, []string{"foo"})
		expectLoadbalancerExistsAndOwned(oapimock)
		expectVMs(oapimock, sdkSelf, sdkVM)
		expectReadLoadBalancer(oapimock, func(desc *osc.LoadBalancer) {
			desc.DnsName = "foo.example.com"
			desc.BackendVmIds = []string{"i-foo"}
			desc.PublicIp = ptr.To("198.51.100.42")
		})
		expectDescribeProxyProtocol(lbmock, false)
		expectDescribeLoadBalancerAttributes(lbmock)
		expectFindExistingIngressSecurityGroup(oapimock, "sg-foo")
		expectFindExistingWorkerSG(oapimock)
		p := ccm.NewProviderWith(c, staticDNSResolver{}, ccm.Options{})
		status, err := p.EnsureLoadBalancer(t.Context(), "foo", svc, []*corev1.Node{&vmNode})
		require.NoError(t, err)
		assert.Equal(t, &corev1.LoadBalancerStatus{Ingress: []corev1.LoadBalancerIngress{{Hostname: "foo.example.com", IP: "198.51.100.42", IPMode: ptr.To(corev1.LoadBalancerIPModeProxy)}}}, status)
	})
	t.Run("When retrying creation on an internal LBU, the status is properly returned when ready, with a resolved IP", func(t *testing.T) {
		svc := testSvc()
		svc.Annotations["service.beta.kubernetes.io/osc-load-balancer-internal"] = "true"
		svc.Annotations["service.beta.kubernetes.io/osc-load-balancer-ingress-address"] = "ip"
		c, oapimock, lbmock := newAPI(t, self, []string{"foo"})
		expectLoadbalancerExistsAndOwned(oapimock)
		expectVMs(oapimock, sdkSelf, sdkVM)
		expectReadLoadBalancer(oapimock, func(desc *osc.LoadBalancer) {
			desc.LoadBalancerType = "internal"
			desc.DnsName = "foo.example.com"
			desc.BackendVmIds = []string{"i-foo"}
		})
		expectDescribeProxyProtocol(lbmock, false)
		expectDescribeLoadBalancerAttributes(lbmock)
		expectFindExistingIngressSecurityGroup(oapimock, "sg-foo")
		expectFindExistingWorkerSG(oapimock)
		p := ccm.NewProviderWith(c, staticDNSResolver{}, ccm.Options{})
		status, err := p.EnsureLoadBalancer(t.Context(), "foo", svc, []*corev1.Node{&vmNode})
		require.NoError(t, err)
		assert.Equal(t, &corev1.LoadBalancerStatus{Ingress: []corev1.LoadBalancerIngress{{IP: "10.0.0.1", IPMode: ptr.To(corev1.LoadBalancerIPModeProxy)}}}, status)
	})
	t.Run("Listeners are updated", func(t *testing.T) {
		svc := testSvc()
		svc.Spec.Ports[0].Port = 8080
		c, oapimock, lbmock := newAPI(t, self, []string{"foo"})
		expectLoadbalancerExistsAndOwned(oapimock)
		expectVMs(oapimock, sdkSelf, sdkVM)
		expectReadLoadBalancer(oapimock, func(desc *osc.LoadBalancer) {
			desc.DnsName = "foo.example.com"
			desc.BackendVmIds = []string{"i-foo"}
			desc.PublicIp = ptr.To("198.51.100.42")
		})
		expectDeleteListener(oapimock)
		expectCreateListener(oapimock, 8080)
		expectDescribeProxyProtocol(lbmock, false)
		expectDescribeLoadBalancerAttributes(lbmock)
		expectFindExistingIngressSecurityGroup(oapimock, "sg-foo")
		expectFindExistingWorkerSG(oapimock)
		expectDeleteIngressSGRule(oapimock, []string{"0.0.0.0/0"}, "sg-foo")
		expectAddIngressSGRule(oapimock, []string{"0.0.0.0/0"}, "sg-foo", func(req *osc.CreateSecurityGroupRuleRequest) {
			req.Rules[0].FromPortRange = 8080
			req.Rules[0].ToPortRange = 8080
		})
		p := ccm.NewProviderWith(c, staticDNSResolver{}, ccm.Options{})
		_, err := p.EnsureLoadBalancer(t.Context(), "foo", svc, []*corev1.Node{&vmNode})
		require.NoError(t, err)
	})
	t.Run("Proxy protocol can be set", func(t *testing.T) {
		svc := testSvc()
		svc.Annotations["service.beta.kubernetes.io/osc-load-balancer-proxy-protocol"] = "*"
		c, oapimock, lbmock := newAPI(t, self, []string{"foo"})
		expectLoadbalancerExistsAndOwned(oapimock)
		expectVMs(oapimock, sdkSelf, sdkVM)
		expectReadLoadBalancer(oapimock, func(desc *osc.LoadBalancer) {
			desc.DnsName = "foo.example.com"
			desc.BackendVmIds = []string{"i-foo"}
			desc.PublicIp = ptr.To("198.51.100.42")
		})
		expectDescribeProxyProtocol(lbmock, false)
		expectConfigureProxyProtocol(lbmock, false, true)
		expectDescribeLoadBalancerAttributes(lbmock)
		expectFindExistingIngressSecurityGroup(oapimock, "sg-foo")
		expectFindExistingWorkerSG(oapimock)
		p := ccm.NewProviderWith(c, staticDNSResolver{}, ccm.Options{})
		_, err := p.EnsureLoadBalancer(t.Context(), "foo", svc, []*corev1.Node{&vmNode})
		require.NoError(t, err)
	})
	t.Run("Proxy protocol is not changed", func(t *testing.T) {
		svc := testSvc()
		svc.Annotations["service.beta.kubernetes.io/osc-load-balancer-proxy-protocol"] = "*"
		c, oapimock, lbmock := newAPI(t, self, []string{"foo"})
		expectLoadbalancerExistsAndOwned(oapimock)
		expectVMs(oapimock, sdkSelf, sdkVM)
		expectReadLoadBalancer(oapimock, func(desc *osc.LoadBalancer) {
			desc.DnsName = "foo.example.com"
			desc.BackendVmIds = []string{"i-foo"}
			desc.PublicIp = ptr.To("198.51.100.42")
		})
		expectDescribeProxyProtocol(lbmock, true)
		expectDescribeLoadBalancerAttributes(lbmock)
		expectFindExistingIngressSecurityGroup(oapimock, "sg-foo")
		expectFindExistingWorkerSG(oapimock)
		p := ccm.NewProviderWith(c, staticDNSResolver{}, ccm.Options{})
		_, err := p.EnsureLoadBalancer(t.Context(), "foo", svc, []*corev1.Node{&vmNode})
		require.NoError(t, err)
	})
	t.Run("Proxy protocol can be disabled", func(t *testing.T) {
		svc := testSvc()
		c, oapimock, lbmock := newAPI(t, self, []string{"foo"})
		expectLoadbalancerExistsAndOwned(oapimock)
		expectVMs(oapimock, sdkSelf, sdkVM)
		expectReadLoadBalancer(oapimock, func(desc *osc.LoadBalancer) {
			desc.DnsName = "foo.example.com"
			desc.BackendVmIds = []string{"i-foo"}
			desc.PublicIp = ptr.To("198.51.100.42")
		})
		expectDescribeProxyProtocol(lbmock, true)
		expectConfigureProxyProtocol(lbmock, true, false)
		expectDescribeLoadBalancerAttributes(lbmock)
		expectFindExistingIngressSecurityGroup(oapimock, "sg-foo")
		expectFindExistingWorkerSG(oapimock)
		p := ccm.NewProviderWith(c, staticDNSResolver{}, ccm.Options{})
		_, err := p.EnsureLoadBalancer(t.Context(), "foo", svc, []*corev1.Node{&vmNode})
		require.NoError(t, err)
	})
}

func TestUpdateLoadBalancer(t *testing.T) {
	t.Run("Listeners are updated", func(t *testing.T) {
		svc := testSvc()
		svc.Spec.Ports[0].Port = 8080
		c, oapimock, lbmock := newAPI(t, self, []string{"foo"})
		expectLoadbalancerExistsAndOwned(oapimock)
		expectVMs(oapimock, sdkSelf, sdkVM)
		expectReadLoadBalancer(oapimock, func(desc *osc.LoadBalancer) {
			desc.DnsName = "foo.example.com"
			desc.BackendVmIds = []string{"i-foo"}
			desc.PublicIp = ptr.To("198.51.100.42")
		})
		expectDeleteListener(oapimock)
		expectCreateListener(oapimock, 8080)
		expectDescribeProxyProtocol(lbmock, false)
		expectDescribeLoadBalancerAttributes(lbmock)
		expectFindExistingIngressSecurityGroup(oapimock, "sg-foo")
		expectFindExistingWorkerSG(oapimock)
		expectDeleteIngressSGRule(oapimock, []string{"0.0.0.0/0"}, "sg-foo")
		expectAddIngressSGRule(oapimock, []string{"0.0.0.0/0"}, "sg-foo", func(req *osc.CreateSecurityGroupRuleRequest) {
			req.Rules[0].FromPortRange = 8080
			req.Rules[0].ToPortRange = 8080
		})
		p := ccm.NewProviderWith(c, staticDNSResolver{}, ccm.Options{})
		err := p.UpdateLoadBalancer(t.Context(), "foo", svc, []*corev1.Node{&vmNode})
		require.NoError(t, err)
	})
	t.Run("SSL Certificate is updated", func(t *testing.T) {
		svc := testSvc()
		svc.Annotations["service.beta.kubernetes.io/osc-load-balancer-ssl-cert"] = "arn:aws:service:region:account:new_resource"
		svc.Annotations["service.beta.kubernetes.io/osc-load-balancer-backend-protocol"] = "http"
		c, oapimock, lbmock := newAPI(t, self, []string{"foo"})
		expectLoadbalancerExistsAndOwned(oapimock)
		expectVMs(oapimock, sdkSelf, sdkVM)
		expectReadLoadBalancer(oapimock, func(desc *osc.LoadBalancer) {
			desc.DnsName = "foo.example.com"
			desc.BackendVmIds = []string{"i-foo"}
			desc.PublicIp = ptr.To("198.51.100.42")
			desc.Listeners[0].LoadBalancerProtocol = "https"
			desc.Listeners[0].BackendProtocol = "http"
			desc.Listeners[0].ServerCertificateId = ptr.To("arn:aws:service:region:account:resource")
		})
		expectDescribeProxyProtocol(lbmock, false)
		expectDescribeLoadBalancerAttributes(lbmock)
		expectFindExistingIngressSecurityGroup(oapimock, "sg-foo")
		expectFindExistingWorkerSG(oapimock)
		oapimock.EXPECT().
			UpdateLoadBalancer(gomock.Any(), gomock.Eq(osc.UpdateLoadBalancerRequest{
				LoadBalancerName:    "lb-foo",
				ServerCertificateId: ptr.To("arn:aws:service:region:account:new_resource"),
			})).Return(&osc.UpdateLoadBalancerResponse{}, nil)
		p := ccm.NewProviderWith(c, staticDNSResolver{}, ccm.Options{})
		err := p.UpdateLoadBalancer(t.Context(), "foo", svc, []*corev1.Node{&vmNode})
		require.NoError(t, err)
	})
	t.Run("IP restriction is updated", func(t *testing.T) {
		svc := testSvc()
		svc.Spec.LoadBalancerSourceRanges = []string{"203.0.113.0/24", "198.51.100.0/24"}
		c, oapimock, lbmock := newAPI(t, self, []string{"foo"})
		expectLoadbalancerExistsAndOwned(oapimock)
		expectVMs(oapimock, sdkSelf, sdkVM)
		expectReadLoadBalancer(oapimock, func(desc *osc.LoadBalancer) {
			desc.DnsName = "foo.example.com"
			desc.BackendVmIds = []string{"i-foo"}
			desc.PublicIp = ptr.To("198.51.100.42")
		})
		expectDescribeProxyProtocol(lbmock, false)
		expectDescribeLoadBalancerAttributes(lbmock)
		expectFindExistingIngressSecurityGroup(oapimock, "sg-foo")
		expectFindExistingWorkerSG(oapimock)
		expectDeleteIngressSGRule(oapimock, []string{"0.0.0.0/0"}, "sg-foo")
		expectAddIngressSGRule(oapimock, []string{"198.51.100.0/24", "203.0.113.0/24"}, "sg-foo")
		p := ccm.NewProviderWith(c, staticDNSResolver{}, ccm.Options{})
		err := p.UpdateLoadBalancer(t.Context(), "foo", svc, []*corev1.Node{&vmNode})
		require.NoError(t, err)
	})
	t.Run("Cannot update a load-balancer if a LBU with the same name already exists but from another cluster", func(t *testing.T) {
		svc := testSvc()
		c, oapimock, _ := newAPI(t, self, []string{"foo"})
		expectLoadbalancerExistsAndOwned(oapimock, func(tag *osc.ResourceTag) {
			if strings.HasPrefix(tag.Key, tags.ClusterIDPrefix) {
				tag.Key = tags.ClusterIDKey("bar")
			}
		})
		p := ccm.NewProviderWith(c, staticDNSResolver{}, ccm.Options{})
		err := p.UpdateLoadBalancer(t.Context(), "foo", svc, []*corev1.Node{&vmNode})
		require.Error(t, err)
	})
	t.Run("Can update a load-balancer even if service tag is not set", func(t *testing.T) {
		svc := testSvc()
		c, oapimock, lbmock := newAPI(t, self, []string{"foo"})
		expectLoadbalancerExistsAndOwned(oapimock, func(tag *osc.ResourceTag) {
			if tag.Key == tags.ServiceName {
				tag.Key = ""
			}
		})
		expectVMs(oapimock, sdkSelf, sdkVM)
		expectReadLoadBalancer(oapimock, func(desc *osc.LoadBalancer) {
			desc.DnsName = "foo.example.com"
			desc.BackendVmIds = []string{"i-foo"}
			desc.PublicIp = ptr.To("198.51.100.42")
		})
		expectDescribeProxyProtocol(lbmock, false)
		expectDescribeLoadBalancerAttributes(lbmock)
		expectFindExistingIngressSecurityGroup(oapimock, "sg-foo")
		expectFindExistingWorkerSG(oapimock)
		p := ccm.NewProviderWith(c, staticDNSResolver{}, ccm.Options{})
		err := p.UpdateLoadBalancer(t.Context(), "foo", svc, []*corev1.Node{&vmNode})
		require.NoError(t, err)
	})
	t.Run("Nodes are added", func(t *testing.T) {
		svc := testSvc()
		c, oapimock, lbmock := newAPI(t, self, []string{"foo"})
		expectLoadbalancerExistsAndOwned(oapimock)
		expectVMs(oapimock, sdkSelf, sdkVM)
		expectReadLoadBalancer(oapimock, func(desc *osc.LoadBalancer) {
			desc.DnsName = "foo.example.com"
			desc.PublicIp = ptr.To("198.51.100.42")
		})
		expectDescribeProxyProtocol(lbmock, false)
		expectDescribeLoadBalancerAttributes(lbmock)
		expectFindExistingIngressSecurityGroup(oapimock, "sg-foo")
		expectFindExistingWorkerSG(oapimock)
		expectRegisterInstances(oapimock, "i-foo")
		p := ccm.NewProviderWith(c, staticDNSResolver{}, ccm.Options{})
		err := p.UpdateLoadBalancer(t.Context(), "foo", svc, []*corev1.Node{&vmNode})
		require.NoError(t, err)
	})
	t.Run("Nodes are removed", func(t *testing.T) {
		svc := testSvc()
		c, oapimock, lbmock := newAPI(t, self, []string{"foo"})
		expectLoadbalancerExistsAndOwned(oapimock)
		expectVMs(oapimock, sdkSelf, sdkVM)
		expectReadLoadBalancer(oapimock, func(desc *osc.LoadBalancer) {
			desc.DnsName = "foo.example.com"
			desc.BackendVmIds = []string{"i-foo", "i-bar"}
			desc.PublicIp = ptr.To("198.51.100.42")
		})
		expectDescribeProxyProtocol(lbmock, false)
		expectDescribeLoadBalancerAttributes(lbmock)
		expectFindExistingIngressSecurityGroup(oapimock, "sg-foo")
		expectFindExistingWorkerSG(oapimock)
		expectDeregisterInstances(oapimock, "i-bar")
		p := ccm.NewProviderWith(c, staticDNSResolver{}, ccm.Options{})
		err := p.UpdateLoadBalancer(t.Context(), "foo", svc, []*corev1.Node{&vmNode})
		require.NoError(t, err)
	})
}

func TestEnsureLoadBalancerDeleted(t *testing.T) {
	t.Run("If the load-balancer exists, delete it", func(t *testing.T) {
		svc := testSvc()
		c, oapimock, _ := newAPI(t, self, []string{"foo"})
		expectLoadbalancerExistsAndOwned(oapimock)
		expectReadLoadBalancer(oapimock, func(desc *osc.LoadBalancer) {
			desc.DnsName = "foo.example.com"
			desc.BackendVmIds = []string{"i-foo"}
		})
		expectDeregisterInstances(oapimock, "i-foo")
		expectCreateTag(oapimock, "sg-foo", osc.ResourceTag{Key: cloud.SGToDeleteTagKey})
		expectDeleteLoadBalancer(oapimock)
		p := ccm.NewProviderWith(c, staticDNSResolver{}, ccm.Options{})
		err := p.EnsureLoadBalancerDeleted(t.Context(), "foo", svc)
		require.NoError(t, err)
	})
	t.Run("If the load-balancer has already been deleted, do nothing", func(t *testing.T) {
		svc := testSvc()
		c, oapimock, _ := newAPI(t, self, []string{"foo"})
		expectNoLoadbalancer(oapimock)
		p := ccm.NewProviderWith(c, staticDNSResolver{}, ccm.Options{})
		err := p.EnsureLoadBalancerDeleted(t.Context(), "foo", svc)
		require.NoError(t, err)
	})
	t.Run("If the load-balancer belongs to someone else, do nothing", func(t *testing.T) {
		svc := testSvc()
		c, oapimock, _ := newAPI(t, self, []string{"foo"})
		expectLoadbalancerExistsAndNotOwned(oapimock)
		p := ccm.NewProviderWith(c, staticDNSResolver{}, ccm.Options{})
		err := p.EnsureLoadBalancerDeleted(t.Context(), "foo", svc)
		require.NoError(t, err)
	})
}
