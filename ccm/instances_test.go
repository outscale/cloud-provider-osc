/*
SPDX-FileCopyrightText: 2025 Outscale SAS <opensource@outscale.com>

SPDX-License-Identifier: BSD-3-Clause
*/
package ccm_test

import (
	"context"
	"testing"

	"github.com/outscale/cloud-provider-osc/ccm"
	"github.com/outscale/cloud-provider-osc/ccm/cloud"
	"github.com/outscale/goutils/k8s/tags"
	"github.com/outscale/goutils/sdk/ptr"
	"github.com/outscale/osc-sdk-go/v3/pkg/osc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestNodeAddresses(t *testing.T) {
	t.Run("Getting the addresses of the current node", func(t *testing.T) {
		vm := cloud.FromOscVm(&osc.Vm{
			VmId:           "i-foo",
			VmType:         "tinav3.c1r1p1",
			PrivateDnsName: ptr.To("10.0.0.10.eu-west-2.compute.internal"),
			PrivateIp:      "10.0.0.10",
			NetId:          ptr.To("net-foo"),
			SubnetId:       ptr.To("subnet-foo"),
			Placement:      osc.Placement{SubregionName: "eu-west-2a"},
		})
		c, _, _ := newAPI(t, vm, []string{"foo"})
		p := ccm.NewProviderWith(c, nil)
		addrs, err := p.NodeAddresses(context.TODO(), vm.NodeName)
		require.NoError(t, err)
		assert.Equal(t, []v1.NodeAddress{
			{Type: v1.NodeInternalIP, Address: "10.0.0.10"},
			{Type: v1.NodeInternalDNS, Address: "10.0.0.10.eu-west-2.compute.internal"},
			{Type: v1.NodeHostName, Address: "10.0.0.10.eu-west-2.compute.internal"},
		}, addrs)
	})
	t.Run("Getting the addresses of the current node with public addresses", func(t *testing.T) {
		vm := cloud.FromOscVm(&osc.Vm{
			VmId:           "i-foo",
			VmType:         "tinav3.c1r1p1",
			PrivateDnsName: ptr.To("10.0.0.10.eu-west-2.compute.internal"),
			PrivateIp:      "10.0.0.10",
			PublicDnsName:  ptr.To("ip-198-51-100-10.eu-west-2.compute.internal"),
			PublicIp:       ptr.To("198.51.100.10"),
			NetId:          ptr.To("net-foo"),
			SubnetId:       ptr.To("subnet-foo"),
			Placement:      osc.Placement{SubregionName: "eu-west-2a"},
		})
		c, _, _ := newAPI(t, vm, []string{"foo"})
		p := ccm.NewProviderWith(c, nil)
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
		sdkvm := &osc.Vm{
			VmId:           "i-foo",
			VmType:         "tinav3.c1r1p1",
			PrivateDnsName: ptr.To(name),
			PrivateIp:      "10.0.0.10",
			Tags: []osc.ResourceTag{{
				Key:   tags.VmNodeName,
				Value: name,
			}},
			NetId:     ptr.To("net-foo"),
			SubnetId:  ptr.To("subnet-foo"),
			Placement: osc.Placement{SubregionName: "eu-west-2a"},
		}
		sdkself := &osc.Vm{
			VmId:           "i-bar",
			VmType:         "tinav3.c1r1p1",
			PrivateDnsName: ptr.To("10.0.0.11.eu-west-2.compute.internal"),
			PrivateIp:      "10.0.0.11",
			NetId:          ptr.To("net-foo"),
			SubnetId:       ptr.To("subnet-foo"),
			Placement:      osc.Placement{SubregionName: "eu-west-2a"},
		}
		self := cloud.FromOscVm(sdkself)
		c, mock, _ := newAPI(t, self, []string{"foo"})
		expectVMs(mock, *sdkself, *sdkvm)
		p := ccm.NewProviderWith(c, nil)
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
	sdkvm := &osc.Vm{
		VmId:           "i-foo",
		VmType:         "tinav3.c1r1p1",
		PrivateDnsName: ptr.To("10.0.0.10.eu-west-2.compute.internal"),
		PrivateIp:      "10.0.0.10",
		NetId:          ptr.To("net-foo"),
		SubnetId:       ptr.To("subnet-foo"),
		Placement:      osc.Placement{SubregionName: "eu-west-2a"},
	}
	vm := cloud.FromOscVm(sdkvm)
	providerID := "aws:///eu-west-2a/i-foo"
	c, mock, _ := newAPI(t, vm, []string{"foo"})
	mock.EXPECT().
		ReadVms(gomock.Any(), gomock.Eq(osc.ReadVmsRequest{
			Filters: &osc.FiltersVm{
				VmIds: &[]string{"i-foo"},
			},
		})).
		Return(&osc.ReadVmsResponse{Vms: &[]osc.Vm{*sdkvm}}, nil)
	p := ccm.NewProviderWith(c, nil)
	addrs, err := p.NodeAddressesByProviderID(context.TODO(), providerID)
	require.NoError(t, err)
	assert.Equal(t, []v1.NodeAddress{
		{Type: v1.NodeInternalIP, Address: "10.0.0.10"},
		{Type: v1.NodeInternalDNS, Address: "10.0.0.10.eu-west-2.compute.internal"},
		{Type: v1.NodeHostName, Address: "10.0.0.10.eu-west-2.compute.internal"},
	}, addrs)
}

func TestInstanceTypeByProviderID(t *testing.T) {
	sdkvm := &osc.Vm{
		VmId:           "i-foo",
		VmType:         "tinav7.c1r1p1",
		NetId:          ptr.To("net-foo"),
		SubnetId:       ptr.To("subnet-foo"),
		PrivateDnsName: ptr.To("10.0.0.10.eu-west-2.compute.internal"),
		Placement:      osc.Placement{SubregionName: "eu-west-2a"},
	}
	vm := cloud.FromOscVm(sdkvm)
	providerID := "aws:///eu-west-2a/i-foo"
	c, mock, _ := newAPI(t, vm, []string{"foo"})
	mock.EXPECT().
		ReadVms(gomock.Any(), gomock.Eq(osc.ReadVmsRequest{
			Filters: &osc.FiltersVm{
				VmIds: &[]string{"i-foo"},
			},
		})).
		Return(&osc.ReadVmsResponse{Vms: &[]osc.Vm{*sdkvm}}, nil)
	p := ccm.NewProviderWith(c, nil)
	typ, err := p.InstanceTypeByProviderID(context.TODO(), providerID)
	require.NoError(t, err)
	assert.Equal(t, sdkvm.VmType, typ)
}

func TestInstanceID(t *testing.T) {
	t.Run("Getting the id of the current node", func(t *testing.T) {
		vm := cloud.FromOscVm(&osc.Vm{
			VmId:           "i-foo",
			VmType:         "tinav7.c1r1p1",
			Placement:      osc.Placement{SubregionName: "eu-west-2a"},
			NetId:          ptr.To("net-foo"),
			SubnetId:       ptr.To("subnet-foo"),
			PrivateDnsName: ptr.To("10.0.0.10.eu-west-2.compute.internal"),
		})
		c, _, _ := newAPI(t, vm, []string{"foo"})
		p := ccm.NewProviderWith(c, nil)
		id, err := p.InstanceID(context.TODO(), vm.NodeName)
		require.NoError(t, err)
		assert.Equal(t, "/eu-west-2a/i-foo", id)
	})
	t.Run("Getting the id of another node (without informer)", func(t *testing.T) {
		name := "10.0.0.10.eu-west-2.compute.internal"
		sdkvm := &osc.Vm{
			VmId:      "i-foo",
			VmType:    "tinav7.c1r1p1",
			Placement: osc.Placement{SubregionName: "eu-west-2a"},
			Tags: []osc.ResourceTag{{
				Key:   tags.VmNodeName,
				Value: name,
			}},
			NetId:          ptr.To("net-foo"),
			SubnetId:       ptr.To("subnet-foo"),
			PrivateDnsName: ptr.To(name),
		}
		sdkself := &osc.Vm{
			VmId:           "i-bar",
			VmType:         "tinav7.c1r1p1",
			PrivateDnsName: ptr.To("10.0.0.11.eu-west-2.compute.internal"),
			PrivateIp:      "10.0.0.11",
			NetId:          ptr.To("net-foo"),
			SubnetId:       ptr.To("subnet-foo"),
			Placement:      osc.Placement{SubregionName: "eu-west-2a"},
		}
		self := cloud.FromOscVm(sdkself)
		c, mock, _ := newAPI(t, self, []string{"foo"})
		expectVMs(mock, *sdkself, *sdkvm)
		p := ccm.NewProviderWith(c, nil)
		id, err := p.InstanceID(context.TODO(), types.NodeName(name))
		require.NoError(t, err)
		assert.Equal(t, "/eu-west-2a/i-foo", id)
	})
}
