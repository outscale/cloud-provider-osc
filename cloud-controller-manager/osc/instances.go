/*
SPDX-FileCopyrightText: 2025 Outscale SAS <opensource@outscale.com>

SPDX-License-Identifier: BSD-3-Clause
*/
package osc

import (
	"context"
	"errors"
	"fmt"

	"github.com/outscale/cloud-provider-osc/cloud-controller-manager/osc/cloud"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	cloudprovider "k8s.io/cloud-provider"
)

// NodeAddresses returns the addresses of the specified instance.
// NodeAddresses implements cloudprovider.Instances.
func (c *Provider) NodeAddresses(ctx context.Context, name types.NodeName) ([]v1.NodeAddress, error) {
	var (
		vm  *cloud.VM
		err error
	)
	if c.self.NodeName == name || name == "" {
		vm = c.self
	} else {
		vm, err = c.getVmByNodeName(ctx, string(name))
	}
	switch {
	case errors.Is(err, cloudprovider.InstanceNotFound):
		return nil, err
	case err != nil:
		return nil, fmt.Errorf("unable to fetch vm: %w", err)
	default:
		return vm.NodeAddresses(), nil
	}
}

// NodeAddressesByProviderID returns the node addresses of an instances with the specified unique providerID
// This method will not be called from the node that is requesting this ID. i.e. metadata service
// and other local methods cannot be used here
func (c *Provider) NodeAddressesByProviderID(ctx context.Context, providerID string) ([]v1.NodeAddress, error) {
	vm, err := c.cloud.GetVMByProviderID(ctx, providerID)
	switch {
	case errors.Is(err, cloudprovider.InstanceNotFound):
		return nil, err
	case err != nil:
		return nil, fmt.Errorf("unable to fetch vm: %w", err)
	default:
		return vm.NodeAddresses(), nil
	}
}

// InstanceID returns the cloud provider ID of the node with the specified nodeName.
func (c *Provider) InstanceID(ctx context.Context, nodeName types.NodeName) (string, error) {
	// In the future it is possible to also return an endpoint as:
	// <endpoint>/<zone>/<instanceid>
	if c.self.NodeName == nodeName {
		return c.self.InstanceID(), nil
	}
	vm, err := c.getVmByNodeName(ctx, string(nodeName))
	switch {
	case errors.Is(err, cloudprovider.InstanceNotFound):
		return "", err
	case err != nil:
		return "", fmt.Errorf("unable to fetch vm: %w", err)
	default:
		return vm.InstanceID(), nil
	}
}

// InstanceTypeByProviderID returns the cloudprovider instance type of the node with the specified unique providerID
// This method will not be called from the node that is requesting this ID. i.e. metadata service
// and other local methods cannot be used here
func (c *Provider) InstanceTypeByProviderID(ctx context.Context, providerID string) (string, error) {
	vm, err := c.cloud.GetVMByProviderID(ctx, providerID)
	switch {
	case errors.Is(err, cloudprovider.InstanceNotFound):
		return "", err
	case err != nil:
		return "", fmt.Errorf("unable to fetch vm: %w", err)
	default:
		return vm.VmType, nil
	}
}

// InstanceType returns the type of the node with the specified nodeName.
func (c *Provider) InstanceType(ctx context.Context, nodeName types.NodeName) (string, error) {
	if c.self.NodeName == nodeName {
		return c.self.VmType, nil
	}
	vm, err := c.getVmByNodeName(ctx, string(nodeName))
	switch {
	case errors.Is(err, cloudprovider.InstanceNotFound):
		return "", err
	case err != nil:
		return "", fmt.Errorf("unable to fetch vm: %w", err)
	default:
		return vm.VmType, nil
	}
}

// AddSSHKeyToAllInstances is currently not implemented.
func (c *Provider) AddSSHKeyToAllInstances(ctx context.Context, user string, keyData []byte) error {
	return cloudprovider.NotImplemented
}

// CurrentNodeName returns the name of the current node
func (c *Provider) CurrentNodeName(ctx context.Context, hostname string) (types.NodeName, error) {
	return c.self.NodeName, nil
}

// InstanceExistsByProviderID returns true if the instance with the given provider id still exists.
// If false is returned with no error, the instance will be immediately deleted by the cloud controller manager.
func (c *Provider) InstanceExistsByProviderID(ctx context.Context, providerID string) (bool, error) {
	_, err := c.cloud.GetVMByProviderID(ctx, providerID)
	switch {
	case errors.Is(err, cloudprovider.InstanceNotFound):
		return false, nil
	case err != nil:
		return false, fmt.Errorf("unable to fetch vm: %w", err)
	default:
		return true, nil
	}
}

// InstanceShutdownByProviderID returns true if the instance is in safe state to detach volumes
func (c *Provider) InstanceShutdownByProviderID(ctx context.Context, providerID string) (bool, error) {
	vm, err := c.cloud.GetVMByProviderID(ctx, providerID)
	switch {
	case errors.Is(err, cloudprovider.InstanceNotFound):
		return false, nil
	case err != nil:
		return false, fmt.Errorf("unable to fetch vm: %w", err)
	default:
		return vm.IsStopped(), nil
	}
}

var _ cloudprovider.Instances = (*Provider)(nil)
