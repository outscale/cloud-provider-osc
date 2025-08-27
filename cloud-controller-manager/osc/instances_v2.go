/*
Copyright 2020 The Kubernetes Authors.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package osc

import (
	"context"

	v1 "k8s.io/api/core/v1"
	cloudprovider "k8s.io/cloud-provider"
)

// InstanceExists indicates whether a given node exists according to the cloud provider
func (c *Provider) InstanceExists(ctx context.Context, node *v1.Node) (bool, error) {
	_, err := c.getVmByNodeName(ctx, node.Name)

	switch {
	case err == cloudprovider.InstanceNotFound:
		return false, nil
	case err != nil:
		return false, err
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
