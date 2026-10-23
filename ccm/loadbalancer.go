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
	lb, buildErr := c.buildLB(svc)
	if buildErr != nil {
		if !errors.Is(buildErr, cloud.ErrNonTCP) {
			return nil, false, fmt.Errorf("unable to build LB: %w", buildErr)
		}
		// Non-TCP Service: the LBU name is still derivable (see stubLB) — a
		// Service provisioned as TCP then mutated to non-TCP may own an LBU.
		stub := c.stubLB(svc)
		if stub == nil {
			klog.V(2).Infof("Service %s/%s cannot own a load balancer: %v", svc.Namespace, svc.Name, buildErr)
			return nil, false, nil
		}
		exists, err := c.cloud.LoadBalancerExists(ctx, stub)
		switch {
		case errors.Is(err, cloud.ErrBelongsToSomeoneElse):
			// Not our LBU: nothing of ours to read.
			return nil, false, nil
		case err != nil:
			return nil, false, fmt.Errorf("unable to check LB: %w", err)
		case !exists:
			klog.V(2).Infof("Service %s/%s cannot own a load balancer: %v", svc.Namespace, svc.Name, buildErr)
			return nil, false, nil
		}
		// The LBU exists but the current (non-TCP) spec cannot describe it:
		// report it as existing without a status.
		return nil, true, nil
	}
	ingresses, err := c.cloud.GetLoadBalancer(ctx, lb)
	if err != nil || len(ingresses) == 0 {
		return nil, false, err
	}
	res, err := c.loadBalancerStatus(ctx, lb, ingresses)
	if err != nil {
		return nil, false, err
	}
	return res, true, nil
}

// buildLB maps a Service to the load balancer it describes.
//
// cloud.NewLoadBalancer derives the LBU name from svc.UID/annotations BEFORE
// refusing the Service, and one refusal (instances count on a Service with
// existing status ingress) fires precisely when an LBU exists. Therefore ONLY
// the non-TCP refusal is treated as non-existent by the read, update and
// delete paths: a non-TCP Service can never reach an API call, so it can
// never have provisioned an LBU. Every other refusal is returned so callers
// keep failing loudly — the Service may own an LBU provisioned under a prior
// spec/annotation state. Provisioning still goes through
// EnsureLoadBalancer, which keeps refusing unsupported Services.
func (c *Provider) buildLB(svc *v1.Service) (*cloud.LoadBalancer, error) {
	return cloud.NewLoadBalancer(svc, c.opts.LBTags)
}

// stubLB covers the provisioned-then-mutated case: a Service provisioned as
// TCP (its LBU was created, named from svc.UID/annotations) then mutated to a
// non-TCP port. buildLB then fails with cloud.ErrNonTCP although the LBU
// still exists — treating it as non-existent would remove the finalizer and
// orphan a billable LBU.
//
// The LBU name derivation in cloud.NewLoadBalancer does not depend on the
// port protocol: forcing every port to TCP lets the full derivation run
// (annotation decoding, defaults, UID naming) and yields the name the LBU was
// provisioned with. The returned stub LB is that derived build, suitable for
// LoadBalancerExists and DeleteLoadBalancer.
//
// When the Service cannot be built even with TCP ports — a second defect
// checked after the non-TCP refusal (session affinity, invalid name/source
// range annotations, instances mismatch…) — no name can be derived and nil is
// returned: the caller keeps the non-existent behavior.
func (c *Provider) stubLB(svc *v1.Service) *cloud.LoadBalancer {
	tcpSvc := svc.DeepCopy()
	for i := range tcpSvc.Spec.Ports {
		tcpSvc.Spec.Ports[i].Protocol = v1.ProtocolTCP
	}
	lb, err := cloud.NewLoadBalancer(tcpSvc, c.opts.LBTags)
	if err != nil || len(lb.Name) == 0 {
		return nil
	}
	return lb
}

// GetLoadBalancerName implements cloudprovider.LoadBalancer
func (c *Provider) GetLoadBalancerName(ctx context.Context, clusterName string, svc *v1.Service) string {
	lb, err := cloud.NewLoadBalancer(svc, c.opts.LBTags)
	if err != nil || len(lb.Name) == 0 {
		return ""
	}
	return lb.Name[0]
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
	lb, err := c.buildLB(svc)
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

	var ingresses []cloud.Ingress
	if exists {
		ingresses, err = c.cloud.UpdateLoadBalancer(ctx, lb, vms)
	} else {
		ingresses, err = c.cloud.CreateLoadBalancer(ctx, lb, vms)
	}
	if err != nil {
		return nil, err
	}
	return c.loadBalancerStatus(ctx, lb, ingresses)
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
	lb, buildErr := c.buildLB(svc)
	if buildErr != nil {
		if !errors.Is(buildErr, cloud.ErrNonTCP) {
			return fmt.Errorf("unable to build LB: %w", buildErr)
		}
		// Non-TCP Service: nothing to update — a non-TCP spec cannot be
		// served, and the read/delete paths still resolve any existing LBU.
		klog.V(2).Infof("Service %s/%s cannot own a load balancer: %v", svc.Namespace, svc.Name, buildErr)
		return nil
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

	_, err = c.cloud.UpdateLoadBalancer(ctx, lb, vms)
	return err
}

// EnsureLoadBalancerDeleted implements cloudprovider.LoadBalancer.
func (c *Provider) EnsureLoadBalancerDeleted(ctx context.Context, clusterName string, svc *v1.Service) error {
	lb, buildErr := c.buildLB(svc)
	if buildErr != nil {
		if !errors.Is(buildErr, cloud.ErrNonTCP) {
			return fmt.Errorf("unable to build LB: %w", buildErr)
		}
		// Non-TCP Service: the LBU name is still derivable (see stubLB) —
		// resolve the stub so an existing LBU is really deleted instead of
		// orphaned; the caller's finalizer stays until this returns.
		stub := c.stubLB(svc)
		if stub == nil {
			klog.V(2).Infof("Service %s/%s cannot own a load balancer: %v", svc.Namespace, svc.Name, buildErr)
			return nil
		}
		lb = stub
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

func (c *Provider) loadBalancerStatus(ctx context.Context, lb *cloud.LoadBalancer, ingresses []cloud.Ingress) (*v1.LoadBalancerStatus, error) {
	res := &v1.LoadBalancerStatus{
		Ingress: make([]v1.LoadBalancerIngress, 0, len(ingresses)),
	}

	for _, ingress := range ingresses {
		ires := v1.LoadBalancerIngress{}
		if lb.IngressAddress.NeedIP() {
			ires.IPMode = lb.IPMode
			switch {
			case lb.Internal && ingress.Hostname != "":
				// internal LBU only have a hostname, no IP
				rip, err := c.resolveLBUHostname(ctx, ingress.Hostname)
				if err != nil {
					return res, fmt.Errorf("resolve hostname: %w", err)
				}
				ires.IP = rip
			case ingress.PublicIP != nil:
				// internet facing LBU have a public IP once the LBU has started
				ires.IP = *ingress.PublicIP
			default:
				continue
			}
		}
		switch {
		case !lb.IngressAddress.NeedHostname():
		case ingress.Hostname != "":
			// LBUs should always have a hostname, but we still ensure that's the case
			ires.Hostname = ingress.Hostname
		default:
			continue
		}
		res.Ingress = append(res.Ingress, ires)
	}
	return res, nil
}

var _ cloudprovider.LoadBalancer = (*Provider)(nil)
