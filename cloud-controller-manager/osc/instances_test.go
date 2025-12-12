/*
SPDX-FileCopyrightText: 2025 Outscale SAS <opensource@outscale.com>

SPDX-License-Identifier: BSD-3-Clause
*/
package osc_test

import (
	"context"
	"testing"

	"github.com/outscale/cloud-provider-osc/cloud-controller-manager/osc"
	"github.com/outscale/cloud-provider-osc/cloud-controller-manager/osc/cloud"
	sdk "github.com/outscale/osc-sdk-go/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
)

func TestNodeAddresses(t *testing.T) {
	t.Run("Getting the addresses of the current node", func(t *testing.T) {
		vm := cloud.FromOscVm(&sdk.Vm{
			VmId:           ptr.To("i-foo"),
			PrivateDnsName: ptr.To("10.0.0.10.eu-west-2.compute.internal"),
			PrivateIp:      ptr.To("10.0.0.10"),
		})
		c, _, _ := newAPI(t, vm, []string{"foo"})
		p := osc.NewProviderWith(c, nil)
		addrs, err := p.NodeAddresses(context.TODO(), vm.NodeName)
		require.NoError(t, err)
		assert.Equal(t, []v1.NodeAddress{
			{Type: v1.NodeInternalIP, Address: "10.0.0.10"},
			{Type: v1.NodeInternalDNS, Address: "10.0.0.10.eu-west-2.compute.internal"},
			{Type: v1.NodeHostName, Address: "10.0.0.10.eu-west-2.compute.internal"},
		}, addrs)
	})
	t.Run("Getting the addresses of the current node with public addresses", func(t *testing.T) {
		vm := cloud.FromOscVm(&sdk.Vm{
			VmId:           ptr.To("i-foo"),
			PrivateDnsName: ptr.To("10.0.0.10.eu-west-2.compute.internal"),
			PrivateIp:      ptr.To("10.0.0.10"),
			PublicDnsName:  ptr.To("ip-198-51-100-10.eu-west-2.compute.internal"),
			PublicIp:       ptr.To("198.51.100.10"),
		})
		c, _, _ := newAPI(t, vm, []string{"foo"})
		p := osc.NewProviderWith(c, nil)
		addrs, err := p.NodeAddresses(context.TODO(), vm.NodeName)
		require.NoError(t, err)
		assert.Equal(t, []v1.NodeAddress{
			{Type: v1.NodeInternalIP, Address: "10.0.0.10"},
			{Type: v1.NodeExternalIP, Address: "198.51.100.10"},
			{Type: v1.NodeInternalDNS, Address: "10.0.0.10.eu-west-2.compute.internal"},
			{Type: v1.NodeHostName, Address: "10.0.0.10.eu-west-2.compute.internal"},
			{Type: v1.NodeExternalDNS, Address: "ip-198-51-100-10.eu-west-2.compute.internal"},
		}, addrs)
	})
	t.Run("Getting the addresses of another node (without informer)", func(t *testing.T) {
		name := "10.0.0.10.eu-west-2.compute.internal"
		sdkvm := &sdk.Vm{
			VmId:           ptr.To("i-foo"),
			PrivateDnsName: ptr.To(name),
			PrivateIp:      ptr.To("10.0.0.10"),
			Tags: &[]sdk.ResourceTag{{
				Key:   cloud.TagVmNodeName,
				Value: name,
			}},
		}
		sdkself := &sdk.Vm{
			VmId:           ptr.To("i-bar"),
			PrivateDnsName: ptr.To("10.0.0.11.eu-west-2.compute.internal"),
			PrivateIp:      ptr.To("10.0.0.11"),
		}
		self := cloud.FromOscVm(sdkself)
		c, mock, _ := newAPI(t, self, []string{"foo"})
		expectVMs(mock, *sdkself, *sdkvm)
		p := osc.NewProviderWith(c, nil)
		addrs, err := p.NodeAddresses(context.TODO(), types.NodeName(name))
		require.NoError(t, err)
		assert.Equal(t, []v1.NodeAddress{
			{Type: v1.NodeInternalIP, Address: "10.0.0.10"},
			{Type: v1.NodeInternalDNS, Address: "10.0.0.10.eu-west-2.compute.internal"},
			{Type: v1.NodeHostName, Address: "10.0.0.10.eu-west-2.compute.internal"},
		}, addrs)
	})
}

func TestNodeAddressesByProviderID(t *testing.T) {
	sdkvm := &sdk.Vm{
		VmId:           ptr.To("i-foo"),
		PrivateDnsName: ptr.To("10.0.0.10.eu-west-2.compute.internal"),
		PrivateIp:      ptr.To("10.0.0.10"),
	}
	vm := cloud.FromOscVm(sdkvm)
	providerID := "aws:///eu-west-2a/i-foo"
	c, mock, _ := newAPI(t, vm, []string{"foo"})
	mock.EXPECT().
		ReadVms(gomock.Any(), gomock.Eq(sdk.ReadVmsRequest{
			Filters: &sdk.FiltersVm{
				VmIds: &[]string{"i-foo"},
			},
		})).
		Return([]sdk.Vm{*sdkvm}, nil)
	p := osc.NewProviderWith(c, nil)
	addrs, err := p.NodeAddressesByProviderID(context.TODO(), providerID)
	require.NoError(t, err)
	assert.Equal(t, []v1.NodeAddress{
		{Type: v1.NodeInternalIP, Address: "10.0.0.10"},
		{Type: v1.NodeInternalDNS, Address: "10.0.0.10.eu-west-2.compute.internal"},
		{Type: v1.NodeHostName, Address: "10.0.0.10.eu-west-2.compute.internal"},
	}, addrs)
}

func TestInstanceTypeByProviderID(t *testing.T) {
	sdkvm := &sdk.Vm{
		VmId:   ptr.To("i-foo"),
		VmType: ptr.To("tinav7.c1r1p1"),
	}
	vm := cloud.FromOscVm(sdkvm)
	providerID := "aws:///eu-west-2a/i-foo"
	c, mock, _ := newAPI(t, vm, []string{"foo"})
	mock.EXPECT().
		ReadVms(gomock.Any(), gomock.Eq(sdk.ReadVmsRequest{
			Filters: &sdk.FiltersVm{
				VmIds: &[]string{"i-foo"},
			},
		})).
		Return([]sdk.Vm{*sdkvm}, nil)
	p := osc.NewProviderWith(c, nil)
	typ, err := p.InstanceTypeByProviderID(context.TODO(), providerID)
	require.NoError(t, err)
	assert.Equal(t, *sdkvm.VmType, typ)
}

func TestInstanceID(t *testing.T) {
	t.Run("Getting the id of the current node", func(t *testing.T) {
		vm := cloud.FromOscVm(&sdk.Vm{
			VmId:      ptr.To("i-foo"),
			Placement: &sdk.Placement{SubregionName: ptr.To("eu-west-2a")},
		})
		c, _, _ := newAPI(t, vm, []string{"foo"})
		p := osc.NewProviderWith(c, nil)
		id, err := p.InstanceID(context.TODO(), vm.NodeName)
		require.NoError(t, err)
		assert.Equal(t, "/eu-west-2a/i-foo", id)
	})
	t.Run("Getting the id of another node (without informer)", func(t *testing.T) {
		name := "10.0.0.10.eu-west-2.compute.internal"
		sdkvm := &sdk.Vm{
			VmId:      ptr.To("i-foo"),
			Placement: &sdk.Placement{SubregionName: ptr.To("eu-west-2a")},
			Tags: &[]sdk.ResourceTag{{
				Key:   cloud.TagVmNodeName,
				Value: name,
			}},
		}
		sdkself := &sdk.Vm{
			VmId:           ptr.To("i-bar"),
			PrivateDnsName: ptr.To("10.0.0.11.eu-west-2.compute.internal"),
			PrivateIp:      ptr.To("10.0.0.11"),
		}
		self := cloud.FromOscVm(sdkself)
		c, mock, _ := newAPI(t, self, []string{"foo"})
		expectVMs(mock, *sdkself, *sdkvm)
		p := osc.NewProviderWith(c, nil)
		id, err := p.InstanceID(context.TODO(), types.NodeName(name))
		require.NoError(t, err)
		assert.Equal(t, "/eu-west-2a/i-foo", id)
	})
}
