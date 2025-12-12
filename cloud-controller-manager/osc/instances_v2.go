/*
SPDX-FileCopyrightText: 2025 Outscale SAS <opensource@outscale.com>

SPDX-License-Identifier: BSD-3-Clause
*/
package osc

import (
	"context"
	"errors"

	v1 "k8s.io/api/core/v1"
	cloudprovider "k8s.io/cloud-provider"
)

// InstanceExists indicates whether a given node exists according to the cloud provider
func (c *Provider) InstanceExists(ctx context.Context, node *v1.Node) (bool, error) {
	vm, err := c.getVmByNodeName(ctx, node.Name)
	switch {
	case errors.Is(err, cloudprovider.InstanceNotFound):
		return false, nil
	case err != nil:
		return false, err
	case vm.IsTerminated():
		return false, nil
	default:
		return true, nil
	}
}

// InstanceShutdown returns true if the instance is shutdown according to the cloud provider.
func (c *Provider) InstanceShutdown(ctx context.Context, node *v1.Node) (bool, error) {
	vm, err := c.getVmByNodeName(ctx, node.Name)
	switch {
	case err != nil:
		return false, err
	default:
		return vm.IsStopped(), nil
	}
}

// InstanceMetadata returns the instance's metadata.
func (c *Provider) InstanceMetadata(ctx context.Context, node *v1.Node) (*cloudprovider.InstanceMetadata, error) {
	vm, err := c.getVmByNodeName(ctx, node.Name)
	if err != nil {
		return nil, err
	}
	return &cloudprovider.InstanceMetadata{
		ProviderID:    vm.ProviderID(),
		InstanceType:  vm.VmType,
		NodeAddresses: vm.NodeAddresses(),
		Zone:          vm.AvailabilityZone,
		Region:        vm.Region,
	}, nil
}

var _ cloudprovider.InstancesV2 = (*Provider)(nil)
