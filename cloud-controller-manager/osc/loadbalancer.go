package osc

import (
	"context"
	"errors"
	"fmt"

	"github.com/outscale/cloud-provider-osc/cloud-controller-manager/osc/cloud"
	v1 "k8s.io/api/core/v1"
	cloudprovider "k8s.io/cloud-provider"
)

// GetLoadBalancer implements cloudprovider.LoadBalancer
func (c *Provider) GetLoadBalancer(ctx context.Context, clusterName string, svc *v1.Service) (*v1.LoadBalancerStatus, bool, error) {
	lb, err := cloud.NewLoadBalancer(svc)
	if err != nil {
		return nil, false, fmt.Errorf("unable to build LB: %w", err)
	}
	dns, found, err := c.cloud.GetLoadBalancer(ctx, lb)
	switch {
	case err != nil || !found:
		return nil, found, err
	case dns != "":
		return &v1.LoadBalancerStatus{Ingress: []v1.LoadBalancerIngress{{Hostname: dns}}}, true, nil
	default:
		return &v1.LoadBalancerStatus{}, true, nil
	}
}

// GetLoadBalancerName implements cloudprovider.LoadBalancer
func (c *Provider) GetLoadBalancerName(ctx context.Context, clusterName string, svc *v1.Service) string {
	lb, err := cloud.NewLoadBalancer(svc)
	if err != nil {
		return ""
	}
	return lb.Name
}

// EnsureLoadBalancer implements cloudprovider.LoadBalancer
func (c *Provider) EnsureLoadBalancer(ctx context.Context, clusterName string, svc *v1.Service,
	nodes []*v1.Node) (*v1.LoadBalancerStatus, error) {
	lb, err := cloud.NewLoadBalancer(svc)
	if err != nil {
		return nil, fmt.Errorf("unable to build LB: %w", err)
	}
	// Figure out what mappings we want on the load balancer

	exists, err := c.cloud.LoadBalancerExists(ctx, lb)
	if err != nil {
		return nil, fmt.Errorf("unable to check LB: %w", err)
	}

	vms, err := c.getVmsByNodeName(ctx, Map(nodes, func(node *v1.Node) string { return node.Name })...)
	if err != nil {
		return nil, err
	}

	var (
		dns string
	)
	if exists {
		dns, err = c.cloud.UpdateLoadBalancer(ctx, lb, vms)
	} else {
		dns, err = c.cloud.CreateLoadBalancer(ctx, lb, vms) // Note: no DNS is expected to be returned after creation
	}
	switch {
	case err != nil:
		return nil, err
	case dns != "":
		return &v1.LoadBalancerStatus{Ingress: []v1.LoadBalancerIngress{{Hostname: dns}}}, nil
	default:
		return &v1.LoadBalancerStatus{}, nil
	}
}

// UpdateLoadBalancer implements cloudprovider.LoadBalancer
func (c *Provider) UpdateLoadBalancer(ctx context.Context, clusterName string, svc *v1.Service, nodes []*v1.Node) error {
	lb, err := cloud.NewLoadBalancer(svc)
	if err != nil {
		return fmt.Errorf("unable to build LB: %w", err)
	}
	exists, err := c.cloud.LoadBalancerExists(ctx, lb)
	switch {
	case err != nil:
		return fmt.Errorf("unable to check LB: %w", err)
	case !exists:
		return errors.New("LB does not exist")
	}
	vms, err := c.getVmsByNodeName(ctx, Map(nodes, func(node *v1.Node) string { return node.Name })...)
	if err != nil {
		return err
	}

	_, err = c.cloud.UpdateLoadBalancer(ctx, lb, vms)
	return err
}

// EnsureLoadBalancerDeleted implements cloudprovider.LoadBalancer.
func (c *Provider) EnsureLoadBalancerDeleted(ctx context.Context, clusterName string, svc *v1.Service) error {
	lb, err := cloud.NewLoadBalancer(svc)
	if err != nil {
		return fmt.Errorf("unable to build LB: %w", err)
	}
	exists, err := c.cloud.LoadBalancerExists(ctx, lb)
	switch {
	case err != nil:
		return fmt.Errorf("unable to check LB: %w", err)
	case !exists:
		return nil
	default:
		return c.cloud.DeleteLoadBalancer(ctx, lb)
	}
}

var _ cloudprovider.LoadBalancer = (*Provider)(nil)
