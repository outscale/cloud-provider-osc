package osc

import (
	"context"
	"errors"

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
		return "", err
	}
	if node.Spec.ProviderID == "" {
		return "", errors.New("getProviderID: node has no providerID")
	}
	return node.Spec.ProviderID, nil
}

// getVmByNodeName returns the instance with the specified node name
func (c *Provider) getVmByNodeName(ctx context.Context, nodeName string) (*cloud.VM, error) {
	providerID, err := c.getProviderID(ctx, nodeName)
	if err == nil {
		return c.cloud.GetVMByProviderID(ctx, providerID)
	}

	klog.FromContext(ctx).V(4).Info("Unable to find provider ID, falling back to tag search")
	return c.cloud.GetVMByNodeName(ctx, nodeName)
}

// getVmByNodeName returns the instance with the specified node name
func (c *Provider) getVmsByNodeName(ctx context.Context, nodeNames ...string) ([]cloud.VM, error) {
	if c.informerHasSynced() {
		providerIDs := make([]string, 0, len(nodeNames))
		for _, nodeName := range nodeNames {
			providerID, err := c.getProviderID(ctx, nodeName)
			if err != nil {
				return nil, err
			}
			providerIDs = append(providerIDs, providerID)
		}
		return c.cloud.GetVMsByProviderID(ctx, providerIDs...)
	}

	klog.FromContext(ctx).V(4).Info("Unable to find provider ID, falling back to tag search")
	return c.cloud.GetVMsByNodeName(ctx, nodeNames...)
}
