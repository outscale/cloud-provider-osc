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
	"k8s.io/client-go/informers"
	"k8s.io/klog/v2"
)

// SetInformers implements InformerUser interface by setting up informer-fed caches for aws lib to
// leverage Kubernetes API for caching
func (c *Provider) SetInformers(informerFactory informers.SharedInformerFactory) {
	c.nodeInformer = informerFactory.Core().V1().Nodes()
}

func (c *Provider) informerHasSynced() bool {
	return c.nodeInformer != nil && c.nodeInformer.Informer().HasSynced()
}

func (c *Provider) getProviderID(ctx context.Context, nodeName string) (string, error) {
	if !c.informerHasSynced() {
		return "", errors.New("getProviderID: node informer has not synced yet")
	}

	node, err := c.nodeInformer.Lister().Get(nodeName)
	if err != nil {
		return "", fmt.Errorf("informer: %w", err)
	}
	if node.Spec.ProviderID == "" {
		klog.FromContext(ctx).V(4).Info("Warning: node has no provider ID", "node", nodeName)
	}
	return node.Spec.ProviderID, nil
}

// getVmByNodeName returns the instance with the specified node name
func (c *Provider) getVmByNodeName(ctx context.Context, nodeName string) (*cloud.VM, error) {
	providerID, err := c.getProviderID(ctx, nodeName)
	switch {
	case err != nil:
		klog.FromContext(ctx).V(4).Error(err, "Unable to find provider ID")
	case providerID != "":
		return c.cloud.GetVMByProviderID(ctx, providerID)
	}

	klog.FromContext(ctx).V(4).Info("Falling back to tag search")
	return c.cloud.GetVMByNodeName(ctx, nodeName)
}

// getVmByNodeName returns the instance with the specified node name
func (c *Provider) getVmsByNodeName(ctx context.Context, nodeNames ...string) ([]cloud.VM, error) {
	if c.informerHasSynced() {
		providerIDs := make([]string, 0, len(nodeNames))
		for _, nodeName := range nodeNames {
			providerID, err := c.getProviderID(ctx, nodeName)
			switch {
			case err != nil:
				// error ? rather than return an error and triggering an expensive backoff, we try OAPI
				klog.FromContext(ctx).V(4).Error(err, "Unable to find provider ID")
				goto NOPROVIDERID
			case providerID == "":
				// no provider id ??? need to switch back to OAPI search
				goto NOPROVIDERID
			}
			providerIDs = append(providerIDs, providerID)
		}
		return c.cloud.GetVMsByProviderID(ctx, providerIDs...)
	}
NOPROVIDERID:
	klog.FromContext(ctx).V(4).Info("Falling back to tag search")
	return c.cloud.GetVMsByNodeName(ctx, nodeNames...)
}
