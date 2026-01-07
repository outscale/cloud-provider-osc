/*
SPDX-FileCopyrightText: 2025 Outscale SAS <opensource@outscale.com>

SPDX-License-Identifier: BSD-3-Clause
*/
package cloud

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	//nolint:staticcheck
	//nolint:staticcheck
	"github.com/outscale/cloud-provider-osc/ccm/oapi"
	"github.com/outscale/goutils/k8s/role"
	"github.com/outscale/goutils/k8s/sdk"
	"github.com/outscale/goutils/k8s/tags"
	"github.com/outscale/goutils/sdk/ptr"
	"github.com/outscale/osc-sdk-go/v3/pkg/osc"
	controllerapi "k8s.io/cloud-provider/api"
	"k8s.io/klog/v2"
)

const (
	// proxyProtocolPolicyName is the name used for the proxy protocol policy
	proxyProtocolPolicyName = "k8s-proxyprotocol-enabled"

	// TagNameSubnetInternalELB is the tag name used on a subnet to designate that
	// it should be used for internal ELBs
	tagNameSubnetInternalELB = "kubernetes.io/role/internal-elb"

	// TagNameSubnetPublicELB is the tag name used on a subnet to designate that
	// it should be used for internet ELBs
	tagNameSubnetPublicELB = "kubernetes.io/role/elb"
)

var (
	// ErrLoadBalancerIsNotReady is returned by CreateLoadBalancer/UpdateLoadBalancer when the LB is not ready yet.
	ErrLoadBalancerIsNotReady = controllerapi.NewRetryError("load balancer is not ready", 30*time.Second)

	ErrBelongsToSomeoneElse = errors.New("found a LBU with the same name belonging to")

	ErrNotFound = errors.New("not found")
)

type lbImplementation interface {
	getLoadBalancer(ctx context.Context, l *LoadBalancer) (*lbStatus, error)
	createLoadBalancer(ctx context.Context, l *LoadBalancer, backend []VM) (*lbStatus, error)
	updateLoadBalancer(ctx context.Context, l *LoadBalancer, backend []VM) (st *lbStatus, err error)
	deleteLoadBalancer(ctx context.Context, l *LoadBalancer) error
}

func (c *Cloud) byType(l *LoadBalancer) lbImplementation {
	if l.Type == VIP {
		return cloudVIP{c}
	}
	return cloudLBU{c}
}

// LoadBalancerExists checks if a load-balancer exists.
func (c *Cloud) LoadBalancerExists(ctx context.Context, l *LoadBalancer) (bool, error) {
	lb, err := c.byType(l).getLoadBalancer(ctx, l)
	switch {
	case errors.Is(err, ErrNotFound):
		return false, nil
	case err != nil:
		return false, err
	}
	if !c.sameCluster(lb.tags) {
		return false, fmt.Errorf("%w another cluster", ErrBelongsToSomeoneElse)
	}
	svcName := tags.GetServiceName(lb.tags)
	if svcName != "" && svcName != l.ServiceName {
		return false, fmt.Errorf("%w another service", ErrBelongsToSomeoneElse)
	}
	return true, nil
}

// GetLoadBalancer fetches a load-balancer.
func (c *Cloud) GetLoadBalancer(ctx context.Context, l *LoadBalancer) (dns string, ip *string, found bool, err error) {
	lb, err := c.byType(l).getLoadBalancer(ctx, l)
	switch {
	case errors.Is(err, ErrNotFound):
		return "", nil, false, nil
	case err != nil:
		return "", nil, false, fmt.Errorf("unable to get LB: %w", err)
	default:
		return lb.host, lb.ip, true, nil
	}
}

// CreateLoadBalancer creates a load-balancer.
func (c *Cloud) CreateLoadBalancer(ctx context.Context, l *LoadBalancer, backend []VM) (dns string, ip *string, err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("unable to create LB: %w", err)
		}
	}()
	switch {
	case l.PublicIPID != "":
		ip, err := oapi.GetPublicIp(ctx, l.PublicIPID, c.api.OAPI())
		if err != nil {
			return "", nil, fmt.Errorf("get public ip: %w", err)
		}
		l.publicIP = ip
	case l.PublicIPPool != "":
		ip, err := sdk.AllocateIPFromPool(ctx, l.PublicIPPool, c.api.OAPI())
		if err != nil {
			return "", nil, err
		}
		l.PublicIPID = ip.PublicIpId
		l.publicIP = ip.PublicIp
	}

	// subnet
	err = c.ensureSubnet(ctx, l)
	if err != nil {
		return "", nil, err
	}
	// security group
	err = c.ensureSecurityGroup(ctx, l)
	if err != nil {
		return "", nil, err
	}

	klog.FromContext(ctx).V(1).Info("Creating load balancer")
	st, err := c.byType(l).createLoadBalancer(ctx, l, backend)
	if err == nil {
		err = c.updateIngressSecurityGroupRules(ctx, l, st)
	}
	switch {
	case err != nil:
		return "", nil, err
	case !l.Internal && st.ip == nil:
		return "", nil, ErrLoadBalancerIsNotReady
	case st.host == "":
		return "", nil, ErrLoadBalancerIsNotReady
	default:
		return st.host, st.ip, nil
	}
}

func (c *Cloud) ensureSubnet(ctx context.Context, l *LoadBalancer) error {
	if l.SubnetID != "" {
		resp, err := c.api.OAPI().ReadSubnets(ctx, osc.ReadSubnetsRequest{
			Filters: &osc.FiltersSubnet{
				SubnetIds: &[]string{l.SubnetID},
			},
		})
		switch {
		case err != nil:
			return fmt.Errorf("find existing subnet: %w", err)
		case len(*resp.Subnets) == 0:
			return errors.New("find existing subnet: not found")
		}
		l.NetID = (*resp.Subnets)[0].NetId
		return nil
	}
	resp, err := c.api.OAPI().ReadSubnets(ctx, osc.ReadSubnetsRequest{
		Filters: &osc.FiltersSubnet{
			TagKeys: ptr.To(c.clusterIDTagKeys()),
		},
	})
	if err != nil {
		return fmt.Errorf("find subnet: %w", err)
	}
	// Find by role
	ensureByTag := func(key string) bool {
		for _, subnet := range *resp.Subnets {
			if tags.Has(subnet.Tags, key) {
				l.SubnetID = subnet.SubnetId
				l.NetID = subnet.NetId
				return true
			}
		}
		return false
	}
	switch {
	case !l.Internal && ensureByTag(tagNameSubnetPublicELB):
	case l.Internal && ensureByTag(tagNameSubnetInternalELB):
	case l.Internal && ensureByTag(tags.RoleKey(role.InternalService)):
	case ensureByTag(tags.RoleKey(role.Service)):
	case ensureByTag(tags.RoleKey(role.LoadBalancer)):
	default:
		return c.discoverSubnet(ctx, l, *resp.Subnets)
	}
	return nil
}

// discoverSubnet tries to find a public or private subnet for the LB.
func (c *Cloud) discoverSubnet(ctx context.Context, l *LoadBalancer, subnets []osc.Subnet) error {
	resp, err := c.api.OAPI().ReadRouteTables(ctx, osc.ReadRouteTablesRequest{
		Filters: &osc.FiltersRouteTable{
			NetIds: &[]string{subnets[0].NetId},
		},
	})
	if err != nil {
		return fmt.Errorf("discover subnet: %w", err)
	}

	// find a public or private subnet, depending on LB type
	var discovered *osc.Subnet
	for _, subnet := range subnets {
		if oapi.IsSubnetPublic(subnet.SubnetId, *resp.RouteTables) == !l.Internal {
			// take the first, in lexical order
			if discovered == nil || tags.Must(tags.GetName(subnet.Tags)) < tags.Must(tags.GetName(discovered.Tags)) {
				discovered = &subnet
			}
		}
	}
	if discovered == nil {
		return errors.New("discover subnet: none found")
	}
	l.SubnetID = discovered.SubnetId
	l.NetID = discovered.NetId
	return nil
}

func (c *Cloud) ensureSecurityGroup(ctx context.Context, l *LoadBalancer) error {
	if len(l.SecurityGroups) > 0 {
		return nil
	}
	sgName := "k8s-elb-" + l.Name
	sgDescription := fmt.Sprintf("Security group for Kubernetes LB %s (%s)", l.Name, l.ServiceName)
	var sg *osc.SecurityGroup
	resp, err := c.api.OAPI().CreateSecurityGroup(ctx, osc.CreateSecurityGroupRequest{
		SecurityGroupName: sgName,
		Description:       sgDescription,
		NetId:             &l.NetID,
	})
	switch {
	case oapi.ErrorCode(err) == "9008": // ErrorDuplicateGroup: the SecurityGroupName '{group_name}' already exists.
		// the SG might have been created by a previous request, try to load it
		resp, err := c.api.OAPI().ReadSecurityGroups(ctx, osc.ReadSecurityGroupsRequest{
			Filters: &osc.FiltersSecurityGroup{
				SecurityGroupNames: &[]string{sgName},
			},
		})
		switch {
		case err != nil:
			return fmt.Errorf("read security groups: %w", err)
		case len(*resp.SecurityGroups) == 0: // this has a tiny chance of occurring, but we would not want the CCM to panic
			return errors.New("duplicate SG but none found")
		default:
			sg = &(*resp.SecurityGroups)[0]
		}
	case err != nil:
		return fmt.Errorf("create SG: %w", err)
	default:
		sg = resp.SecurityGroup
	}
	// check clusterID tag
	switch {
	case c.sameCluster(sg.Tags): // existing SG with valid tag => noop
	case tags.GetClusterID(sg.Tags) == "": // new SG or existing SG without tag => create
		_, err = c.api.OAPI().CreateTags(ctx, osc.CreateTagsRequest{
			ResourceIds: []string{sg.SecurityGroupId},
			Tags:        []osc.ResourceTag{{Key: c.clusterIDTagKey(), Value: tags.ResourceLifecycleOwned}},
		})
		if err != nil {
			return fmt.Errorf("create SG: %w", err)
		}
	default: // existing SG with invalid tag/belonging to another cluster
		return errors.New("a segurity group of the same name already exists")
	}
	l.SecurityGroups = []string{sg.SecurityGroupId}
	return nil
}

// UpdateLoadBalancer updates a load-balancer.
func (c *Cloud) UpdateLoadBalancer(ctx context.Context, l *LoadBalancer, backend []VM) (dns string, ip *string, err error) {
	st, err := c.byType(l).updateLoadBalancer(ctx, l, backend)
	if err == nil {
		err = c.updateIngressSecurityGroupRules(ctx, l, st)
	}
	switch {
	case err != nil:
		return "", nil, err
	case !l.Internal && st.ip == nil:
		return "", nil, ErrLoadBalancerIsNotReady
	case st.host == "":
		return "", nil, ErrLoadBalancerIsNotReady
	default:
		return st.host, st.ip, nil
	}
}

func (c *Cloud) loadSecurityGroup(ctx context.Context, l *LoadBalancer, existing *lbStatus) (*osc.SecurityGroup, error) {
	var lbSG []string
	if existing != nil {
		lbSG = existing.securityGroups
	} else {
		lbSG = l.SecurityGroups
	}
	srcSGID := lbSG[0]
	resp, err := c.api.OAPI().ReadSecurityGroups(ctx, osc.ReadSecurityGroupsRequest{
		Filters: &osc.FiltersSecurityGroup{
			SecurityGroupIds: &[]string{srcSGID},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("list SGs: %w", err)
	}
	if len(*resp.SecurityGroups) == 0 {
		return nil, errors.New("no SG found for load balancer")
	}
	return &(*resp.SecurityGroups)[0], nil
}

// FIXME: lbSecurityGroup is not set
func (c *Cloud) updateIngressSecurityGroupRules(ctx context.Context, l *LoadBalancer, existing *lbStatus) error {
	lbSG, err := c.loadSecurityGroup(ctx, l, existing)
	if err != nil {
		return err
	}
	allowed := l.AllowFrom.StringSlice()
	// sort slice to get a deterministic order for tests
	slices.Sort(allowed)
	// Adding new rules
	for _, listener := range l.Listeners {
		var addRanges []string
		for _, allowFrom := range allowed {
			if !slices.ContainsFunc(lbSG.InboundRules, func(r osc.SecurityGroupRule) bool {
				return r.FromPortRange == listener.Port && slices.Contains(r.IpRanges, allowFrom)
			}) {
				addRanges = append(addRanges, allowFrom)
			}
		}
		if len(addRanges) == 0 {
			continue
		}
		klog.FromContext(ctx).V(2).Info("Adding rule", "from", addRanges, "to", lbSG.SecurityGroupId, "port", listener.Port)
		_, err := c.api.OAPI().CreateSecurityGroupRule(ctx, osc.CreateSecurityGroupRuleRequest{
			SecurityGroupId: lbSG.SecurityGroupId,
			Flow:            "Inbound",
			Rules: []osc.SecurityGroupRule{{
				IpProtocol:    "tcp",
				FromPortRange: listener.Port,
				ToPortRange:   listener.Port,
				IpRanges:      addRanges,
			}},
		})
		if err != nil {
			return fmt.Errorf("add ingress rule: %w", err)
		}
	}

	// Removing rules
	for _, r := range lbSG.InboundRules {
		del := false
		delRule := osc.SecurityGroupRule{
			IpProtocol:    r.IpProtocol,
			FromPortRange: r.FromPortRange,
			ToPortRange:   r.ToPortRange,
		}
		if !slices.ContainsFunc(l.Listeners, func(listener Listener) bool {
			return listener.Port == r.FromPortRange
		}) {
			del = true
		}
		if del {
			delRule.IpRanges = r.IpRanges
		} else if len(r.IpRanges) > 0 {
			delRule.IpRanges = []string{}
			for _, ipRange := range r.IpRanges {
				if !slices.Contains(allowed, ipRange) {
					delRule.IpRanges = append(delRule.IpRanges, ipRange)
					del = true
				}
			}
		}
		if !del {
			continue
		}
		klog.FromContext(ctx).V(2).Info("Deleting rule", "from", delRule.IpRanges, "to", lbSG.SecurityGroupId, "port", r.FromPortRange)
		_, err := c.api.OAPI().DeleteSecurityGroupRule(ctx, osc.DeleteSecurityGroupRuleRequest{
			SecurityGroupId: lbSG.SecurityGroupId,
			Flow:            "Inbound",
			Rules:           []osc.SecurityGroupRule{delRule},
		})
		if err != nil {
			return fmt.Errorf("delete ingress rule: %w", err)
		}
	}
	return nil
}

// DeleteLoadBalancer deletes a load balancer.
func (c *Cloud) DeleteLoadBalancer(ctx context.Context, l *LoadBalancer) error {
	existing, err := c.byType(l).getLoadBalancer(ctx, l)
	if err != nil {
		return fmt.Errorf("check LB: %w", err)
	}
	if existing == nil {
		return nil
	}
	// Tag LB SG as to be deleted (only if it has been created)
	for _, sg := range existing.securityGroups {
		if !slices.Contains(l.SecurityGroups, sg) && !slices.Contains(l.AdditionalSecurityGroups, sg) {
			klog.FromContext(ctx).V(2).Info("Marking SG for deletion", "securityGroupId", sg)
			_, err = c.api.OAPI().CreateTags(ctx, osc.CreateTagsRequest{
				ResourceIds: []string{sg},
				Tags:        []osc.ResourceTag{{Key: SGToDeleteTagKey}},
			})
			if err != nil {
				return fmt.Errorf("mark SG for deletion: %w", err)
			}
		}
	}

	err = c.byType(l).deleteLoadBalancer(ctx, l)
	if err != nil {
		return fmt.Errorf("delete LBU: %w", err)
	}
	return nil
}

// RunGarbageCollector deletes LB security groups
func (c *Cloud) RunGarbageCollector(ctx context.Context) error {
	// We collect all the SG from the cluster
	// This is the list of SG we will scan to find rules linking to the SG to be deleted.
	resp, err := c.api.OAPI().ReadSecurityGroups(ctx, osc.ReadSecurityGroupsRequest{
		Filters: &osc.FiltersSecurityGroup{
			TagKeys: ptr.To(c.clusterIDTagKeys()),
		},
	})
	if err != nil {
		return fmt.Errorf("find security groups: %w", err)
	}
	// Find SG to delete
	var toDelete []string
	for _, sg := range *resp.SecurityGroups {
		if tags.Has(sg.Tags, SGToDeleteTagKey) {
			toDelete = append(toDelete, sg.SecurityGroupId)
		}
	}
	klog.FromContext(ctx).V(4).Info("Security groups marked for deletion", "count", len(toDelete))
	for _, delSGID := range toDelete {
		// delete all inbound rules from this SG
		for _, sg := range *resp.SecurityGroups {
			klog.FromContext(ctx).V(2).Info("Deleting inbound rule", "from", delSGID, "to", sg.SecurityGroupId)
			for _, r := range sg.InboundRules {
				if slices.ContainsFunc(r.SecurityGroupsMembers, func(m osc.SecurityGroupsMember) bool {
					return slices.Contains(toDelete, m.SecurityGroupId)
				}) {
					_, err = c.api.OAPI().DeleteSecurityGroupRule(ctx, osc.DeleteSecurityGroupRuleRequest{
						SecurityGroupId: sg.SecurityGroupId,
						Flow:            "Inbound",
						Rules: []osc.SecurityGroupRule{{
							FromPortRange: r.FromPortRange,
							ToPortRange:   r.ToPortRange,
							IpProtocol:    r.IpProtocol,
							SecurityGroupsMembers: []osc.SecurityGroupsMember{{
								SecurityGroupId: delSGID,
							}},
						}},
					})
					if err != nil {
						return fmt.Errorf("delete rule from %s to %s: %w", delSGID, sg.SecurityGroupId, err)
					}
				}
			}
		}
		klog.FromContext(ctx).V(2).Info("Deleting SG", "securityGroupId", delSGID)
		_, err = c.api.OAPI().DeleteSecurityGroup(ctx, osc.DeleteSecurityGroupRequest{
			SecurityGroupId: &delSGID,
		})
		if err != nil {
			return fmt.Errorf("delete SG %s: %w", delSGID, err)
		}
	}
	return nil
}
