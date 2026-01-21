/*
SPDX-FileCopyrightText: 2025 Outscale SAS <opensource@outscale.com>

SPDX-License-Identifier: BSD-3-Clause
*/
package ccm

import (
	"context"
	"errors"
	"strings"

	v1 "k8s.io/api/core/v1"
	cloudprovider "k8s.io/cloud-provider"
	"k8s.io/klog/v2"
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
	labels := make(map[string]string, len(c.opts.NodeLabels))
	for k, v := range c.opts.nodeLabelTemplates {
		str := strings.Builder{}
		err := v.Execute(&str, vm)
		if err != nil {
			klog.FromContext(ctx).V(2).Error(err, "unable to compute label %q: %w", k, err)
			continue
		}
		labels[k] = str.String()
	}
	return &cloudprovider.InstanceMetadata{
		ProviderID:       vm.ProviderID(),
		InstanceType:     vm.VmType,
		NodeAddresses:    vm.NodeAddresses(),
		Zone:             vm.SubRegion,
		Region:           vm.Region,
		AdditionalLabels: labels,
	}, nil
}

var _ cloudprovider.InstancesV2 = (*Provider)(nil)
