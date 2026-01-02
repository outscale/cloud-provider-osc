/*
SPDX-FileCopyrightText: 2025 Outscale SAS <opensource@outscale.com>

SPDX-License-Identifier: BSD-3-Clause
*/
package ccm

import (
	"context"
	"errors"
	"fmt"

	"github.com/outscale/cloud-provider-osc/ccm/cloud"
	v1 "k8s.io/api/core/v1"
	cloudprovider "k8s.io/cloud-provider"
	"k8s.io/klog/v2"
)

// GetLoadBalancer implements cloudprovider.LoadBalancer
func (c *Provider) GetLoadBalancer(ctx context.Context, clusterName string, svc *v1.Service) (*v1.LoadBalancerStatus, bool, error) {
	lb, err := cloud.NewLoadBalancer(svc, c.opts.ExtraTags)
	if err != nil {
		return nil, false, fmt.Errorf("unable to build LB: %w", err)
	}
	dns, ip, found, err := c.cloud.GetLoadBalancer(ctx, lb)
	switch {
	case err != nil || !found:
		return nil, found, err
	case dns != "" || ip != nil:
		i, err := c.ingressStatus(ctx, lb, dns, ip)
		if err != nil {
			return nil, true, err
		}
		return &v1.LoadBalancerStatus{Ingress: []v1.LoadBalancerIngress{i}}, true, nil
	default:
		return &v1.LoadBalancerStatus{}, true, nil
	}
}

// GetLoadBalancerName implements cloudprovider.LoadBalancer
func (c *Provider) GetLoadBalancerName(ctx context.Context, clusterName string, svc *v1.Service) string {
	lb, err := cloud.NewLoadBalancer(svc, c.opts.ExtraTags)
	if err != nil {
		return ""
	}
	return lb.Name
}

func (c *Provider) resolveLBUHostname(ctx context.Context, hostname string) (string, error) {
	logger := klog.FromContext(ctx)
	logger.V(5).Info("Resolving LBU hostname", "hostname", hostname)
	ips, err := c.resolver.LookupHost(ctx, hostname)
	if err != nil {
		return "", fmt.Errorf("resolving internal hostname: %w", err)
	}
	if len(ips) == 0 {
		logger.V(4).Info("No IP found for hostname", "hostname", hostname)
		return "", nil
	}
	ip := ips[0]
	logger.V(4).Info("Resolved LBU hostname", "hostname", hostname, "hostname", ip)
	return ip, nil
}

// EnsureLoadBalancer implements cloudprovider.LoadBalancer
func (c *Provider) EnsureLoadBalancer(ctx context.Context, clusterName string, svc *v1.Service,
	nodes []*v1.Node) (*v1.LoadBalancerStatus, error) {
	lb, err := cloud.NewLoadBalancer(svc, c.opts.ExtraTags)
	if err != nil {
		return nil, fmt.Errorf("unable to build LB: %w", err)
	}

	exists, err := c.cloud.LoadBalancerExists(ctx, lb)
	if err != nil {
		return nil, fmt.Errorf("unable to check LB: %w", err)
	}

	nodes = c.filterTargetNodes(lb, nodes)
	vms, err := c.getVmsByNodeName(ctx, Map(nodes, func(node *v1.Node) string { return node.Name })...)
	if err != nil {
		return nil, err
	}

	var (
		dns string
		ip  *string
	)
	if exists {
		dns, ip, err = c.cloud.UpdateLoadBalancer(ctx, lb, vms)
	} else {
		dns, ip, err = c.cloud.CreateLoadBalancer(ctx, lb, vms) // Note: no DNS is expected to be returned after creation
	}
	switch {
	case err != nil:
		return nil, err
	case dns != "" || ip != nil:
		i, err := c.ingressStatus(ctx, lb, dns, ip)
		if err != nil {
			return nil, err
		}
		return &v1.LoadBalancerStatus{Ingress: []v1.LoadBalancerIngress{i}}, nil
	default:
		return &v1.LoadBalancerStatus{}, nil
	}
}

func (c *Provider) filterTargetNodes(l *cloud.LoadBalancer, nodes []*v1.Node) []*v1.Node {
	if len(l.TargetNodesLabels) == 0 {
		return nodes
	}

	targetNodes := make([]*v1.Node, 0, len(nodes))
LOOPNODES:
	for _, node := range nodes {
		for k, v := range l.TargetNodesLabels {
			if lv, found := node.Labels[k]; !found || lv != v {
				continue LOOPNODES
			}
		}
		targetNodes = append(targetNodes, node)
	}
	return targetNodes
}

// UpdateLoadBalancer implements cloudprovider.LoadBalancer
func (c *Provider) UpdateLoadBalancer(ctx context.Context, clusterName string, svc *v1.Service, nodes []*v1.Node) error {
	lb, err := cloud.NewLoadBalancer(svc, c.opts.ExtraTags)
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
	nodes = c.filterTargetNodes(lb, nodes)
	vms, err := c.getVmsByNodeName(ctx, Map(nodes, func(node *v1.Node) string { return node.Name })...)
	if err != nil {
		return err
	}

	_, _, err = c.cloud.UpdateLoadBalancer(ctx, lb, vms)
	return err
}

// EnsureLoadBalancerDeleted implements cloudprovider.LoadBalancer.
func (c *Provider) EnsureLoadBalancerDeleted(ctx context.Context, clusterName string, svc *v1.Service) error {
	lb, err := cloud.NewLoadBalancer(svc, c.opts.ExtraTags)
	if err != nil {
		return fmt.Errorf("unable to build LB: %w", err)
	}
	exists, err := c.cloud.LoadBalancerExists(ctx, lb)
	switch {
	case errors.Is(err, cloud.ErrBelongsToSomeoneElse):
		return nil
	case err != nil:
		return fmt.Errorf("unable to check LB: %w", err)
	case !exists:
		return nil
	default:
		return c.cloud.DeleteLoadBalancer(ctx, lb)
	}
}

func (c *Provider) ingressStatus(ctx context.Context, lb *cloud.LoadBalancer, dns string, ip *string) (v1.LoadBalancerIngress, error) {
	var (
		res v1.LoadBalancerIngress
	)
	if lb.IngressAddress.NeedIP() {
		if ip == nil && dns != "" {
			rip, err := c.resolveLBUHostname(ctx, dns)
			if err != nil {
				return res, fmt.Errorf("resolve hostname: %w", err)
			}
			ip = &rip
		}
		if ip != nil {
			res.IP = *ip
		}
		res.IPMode = lb.IPMode
	}
	if lb.IngressAddress.NeedHostname() && dns != "" {
		res.Hostname = dns
	}
	return res, nil
}

var _ cloudprovider.LoadBalancer = (*Provider)(nil)
