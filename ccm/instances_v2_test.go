/*
SPDX-FileCopyrightText: 2025 Outscale SAS <opensource@outscale.com>

SPDX-License-Identifier: BSD-3-Clause
*/
package ccm_test

import (
	"context"
	"testing"

	"github.com/outscale/cloud-provider-osc/ccm"
	"github.com/outscale/osc-sdk-go/v3/pkg/osc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	cloudprovider "k8s.io/cloud-provider"
)

func TestInstanceExists(t *testing.T) {
	t.Run("If the instance exists, return true", func(t *testing.T) {
		c, mock, _ := newAPI(t, self, []string{"foo"})
		expectVMs(mock, sdkSelf, sdkVM)
		p := ccm.NewProviderWith(c, nil)
		exists, err := p.InstanceExists(context.TODO(), &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: vmNodeName}})
		require.NoError(t, err)
		assert.True(t, exists)
	})
	t.Run("If the instance does not exists, return false", func(t *testing.T) {
		c, mock, _ := newAPI(t, self, []string{"foo"})
		expectVMs(mock, sdkSelf)
		p := ccm.NewProviderWith(c, nil)
		exists, err := p.InstanceExists(context.TODO(), &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: vmNodeName}})
		require.NoError(t, err)
		assert.False(t, exists)
	})
	t.Run("If the instance is terminated, return false", func(t *testing.T) {
		sdkTerminated := sdkVM
		sdkTerminated.State = osc.VmStateTerminated
		c, mock, _ := newAPI(t, self, []string{"foo"})
		expectVMs(mock, sdkSelf, sdkTerminated)
		p := ccm.NewProviderWith(c, nil)
		exists, err := p.InstanceExists(context.TODO(), &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: vmNodeName}})
		require.NoError(t, err)
		assert.False(t, exists)
	})
}

func TestInstanceShutdown(t *testing.T) {
	t.Run("If the instance is running, return false", func(t *testing.T) {
		c, mock, _ := newAPI(t, self, []string{"foo"})
		expectVMs(mock, sdkSelf, sdkVM)
		p := ccm.NewProviderWith(c, nil)
		shut, err := p.InstanceShutdown(context.TODO(), &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: vmNodeName}})
		require.NoError(t, err)
		assert.False(t, shut)
	})
	t.Run("If the instance is stopped, return true", func(t *testing.T) {
		sdkVM := sdkVM
		sdkVM.State = osc.VmStateStopped
		c, mock, _ := newAPI(t, self, []string{"foo"})
		expectVMs(mock, sdkSelf, sdkVM)
		p := ccm.NewProviderWith(c, nil)
		shut, err := p.InstanceShutdown(context.TODO(), &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: vmNodeName}})
		require.NoError(t, err)
		assert.True(t, shut)
	})
}

func TestInstanceMetadata(t *testing.T) {
	c, mock, _ := newAPI(t, self, []string{"foo"})
	expectVMs(mock, sdkSelf, sdkVM)
	p := ccm.NewProviderWith(c, nil)
	meta, err := p.InstanceMetadata(context.TODO(), &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: vmNodeName}})
	require.NoError(t, err)
	assert.Equal(t, &cloudprovider.InstanceMetadata{
		ProviderID:   "aws:///eu-west-2a/i-foo",
		InstanceType: "tinav3.c1r1p1",
		Zone:         "eu-west-2a",
		Region:       "eu-west-2",
		NodeAddresses: []v1.NodeAddress{
			{Type: v1.NodeInternalIP, Address: "10.0.0.10"},
			{Type: v1.NodeInternalDNS, Address: "10.0.0.10.eu-west-2.compute.internal"},
			{Type: v1.NodeHostName, Address: "10.0.0.10.eu-west-2.compute.internal"},
		},
	}, meta)
}
