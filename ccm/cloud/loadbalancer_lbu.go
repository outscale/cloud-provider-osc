/*
SPDX-FileCopyrightText: 2025 Outscale SAS <opensource@outscale.com>

SPDX-License-Identifier: BSD-3-Clause
*/
package cloud

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/aws/aws-sdk-go/aws"         //nolint:staticcheck
	"github.com/aws/aws-sdk-go/service/elb" //nolint:staticcheck
	"github.com/outscale/cloud-provider-osc/ccm/oapi"
	"github.com/outscale/goutils/k8s/tags"
	"github.com/outscale/goutils/sdk/ptr"
	"github.com/outscale/osc-sdk-go/v3/pkg/osc"
	"github.com/samber/lo"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog/v2"
)

type cloudLBU struct {
	*Cloud
}

func lbuToStatus(l *osc.LoadBalancer) *lbStatus {
	if l == nil {
		return nil
	}
	return &lbStatus{
		host:           l.DnsName,
		ip:             l.PublicIp,
		tags:           l.Tags,
		securityGroups: l.SecurityGroups,
	}
}

func (c cloudLBU) getLBU(ctx context.Context, l *LoadBalancer) (*osc.LoadBalancer, error) {
	res, err := c.api.OAPI().ReadLoadBalancers(ctx, osc.ReadLoadBalancersRequest{
		Filters: &osc.FiltersLoadBalancer{LoadBalancerNames: &[]string{l.Name}},
	})
	if err != nil {
		return nil, err
	}
	if len(*res.LoadBalancers) == 0 {
		return nil, ErrNotFound
	}
	return &(*res.LoadBalancers)[0], nil
}

func (c cloudLBU) getLoadBalancer(ctx context.Context, l *LoadBalancer) (*lbStatus, error) {
	lb, err := c.getLBU(ctx, l)
	if err != nil {
		return nil, err
	}
	return lbuToStatus(lb), nil
}

// CreateLoadBalancer creates a load-balancer.
func (c cloudLBU) createLoadBalancer(ctx context.Context, l *LoadBalancer, backend []VM) (*lbStatus, error) {
	createRequest := osc.CreateLoadBalancerRequest{
		LoadBalancerName: l.Name,
		Listeners:        l.listeners(),
	}

	if l.Internal {
		createRequest.LoadBalancerType = ptr.To("internal")
	}
	if l.publicIP != "" {
		createRequest.PublicIp = &l.publicIP
	}
	createRequest.Subnets = &[]string{l.SubnetID}
	sgs := append(l.SecurityGroups, l.AdditionalSecurityGroups...)
	createRequest.SecurityGroups = &sgs
	stags := l.Tags
	if stags == nil {
		stags = map[string]string{}
	}
	stags[tags.ServiceName] = l.ServiceName
	stags[c.clusterIDTagKey()] = tags.ResourceLifecycleOwned

	dtags := lo.MapToSlice(stags, func(k, v string) osc.ResourceTag {
		return osc.ResourceTag{Key: k, Value: v}
	})
	createRequest.Tags = &dtags
	slices.SortFunc(*createRequest.Tags, func(a, b osc.ResourceTag) int {
		switch {
		case a.Key < b.Key:
			return -1
		case a.Key > b.Key:
			return 1
		default:
			return 0
		}
	})

	klog.FromContext(ctx).V(1).Info("Creating load balancer")
	res, err := c.api.OAPI().CreateLoadBalancer(ctx, createRequest)
	if err == nil {
		err = c.updateProxyProtocol(ctx, l, nil)
	}
	if err == nil {
		err = c.updateAttributes(ctx, l, nil)
	}
	if err == nil {
		err = c.updateHealthcheck(ctx, l, nil)
	}
	if err == nil {
		err = c.updateBackendVms(ctx, l, backend, nil)
	}
	if err == nil {
		err = c.updateBackendSecurityGroupRules(ctx, l, backend, nil)
	}
	return lbuToStatus(res.LoadBalancer), nil
}

// UpdateLoadBalancer updates a load-balancer.
func (c cloudLBU) updateLoadBalancer(ctx context.Context, l *LoadBalancer, backend []VM) (st *lbStatus, err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("unable to create LB: %w", err)
		}
	}()
	existing, err := c.getLBU(ctx, l)
	if err != nil {
		return nil, fmt.Errorf("check LB: %w", err)
	}
	if existing == nil {
		return nil, errors.New("existing LBU not found")
	}

	err = c.updateListeners(ctx, l, existing)
	if err == nil {
		// proxy protocol requires listeners to be set
		err = c.updateProxyProtocol(ctx, l, existing)
	}
	if err == nil {
		err = c.updateSSLCert(ctx, l, existing)
	}
	if err == nil {
		err = c.updateAttributes(ctx, l, existing)
	}
	if err == nil {
		err = c.updateHealthcheck(ctx, l, existing)
	}
	if err == nil {
		err = c.updateBackendSecurityGroupRules(ctx, l, backend, existing)
	}
	if err == nil {
		err = c.updateBackendVms(ctx, l, backend, existing)
	}

	return lbuToStatus(existing), nil
}

func (c cloudLBU) updateProxyProtocol(ctx context.Context, l *LoadBalancer, _ *osc.LoadBalancer) error {
	elbu, err := c.api.LBU().DescribeLoadBalancersWithContext(ctx, &elb.DescribeLoadBalancersInput{
		LoadBalancerNames: []*string{aws.String(l.Name)},
	})
	if err != nil {
		return fmt.Errorf("check proxy protocol: %w", err)
	}
	if len(elbu.LoadBalancerDescriptions) == 0 {
		return nil
	}
	existing := elbu.LoadBalancerDescriptions[0]

	set := false
	if existing != nil {
		set = slices.ContainsFunc(existing.ListenerDescriptions, func(p *elb.ListenerDescription) bool {
			return slices.ContainsFunc(p.PolicyNames, func(name *string) bool {
				return *name == proxyProtocolPolicyName
			})
		})
	}
	need := slices.ContainsFunc(l.Listeners, func(lstnr Listener) bool {
		return isPortIn(lstnr.BackendPort, l.ListenerDefaults.ProxyProtocol)
	})
	if need == set {
		return nil
	}
	var policies []*string
	if need {
		request := &elb.CreateLoadBalancerPolicyInput{
			LoadBalancerName: aws.String(l.Name),
			PolicyName:       aws.String(proxyProtocolPolicyName),
			PolicyTypeName:   aws.String("ProxyProtocolPolicyType"),
			PolicyAttributes: []*elb.PolicyAttribute{
				{
					AttributeName:  aws.String("ProxyProtocol"),
					AttributeValue: aws.String("true"),
				},
			},
		}
		klog.FromContext(ctx).V(2).Info("Creating proxy protocol policy")
		_, err := c.api.LBU().CreateLoadBalancerPolicyWithContext(ctx, request)
		switch {
		case err == nil:
		case oapi.AWSErrorCode(err) == elb.ErrCodeDuplicatePolicyNameException:
			klog.FromContext(ctx).V(4).Info("Policy already exists")
		default:
			return fmt.Errorf("create proxy protocol policy: %w", err)
		}
		policies = []*string{aws.String(proxyProtocolPolicyName)}
	} else {
		policies = []*string{}
	}

	for _, listener := range l.Listeners {
		if need && !isPortIn(listener.BackendPort, l.ListenerDefaults.ProxyProtocol) {
			continue
		}
		request := &elb.SetLoadBalancerPoliciesForBackendServerInput{
			LoadBalancerName: aws.String(l.Name),
			InstancePort:     aws.Int64(int64(listener.BackendPort)),
			PolicyNames:      policies,
		}
		if len(policies) > 0 {
			klog.FromContext(ctx).V(2).Info("Adding policy on backend", "port", listener.BackendPort)
		} else {
			klog.FromContext(ctx).V(2).Info("Removing policies from backend", "port", listener.BackendPort)
		}
		_, err := c.api.LBU().SetLoadBalancerPoliciesForBackendServerWithContext(ctx, request)
		if err != nil {
			return fmt.Errorf("set proxy protocol policy: %w", err)
		}
	}
	return nil
}

func (c cloudLBU) updateListeners(ctx context.Context, l *LoadBalancer, existing *osc.LoadBalancer) error {
	expect := l.listeners()

	// Remove unused listeners
	var del, delback []int
	for _, elistener := range existing.Listeners {
		if !slices.ContainsFunc(expect, func(listener osc.ListenerForCreation) bool {
			return oscListenersAreEqual(elistener, listener)
		}) {
			del = append(del, elistener.LoadBalancerPort)
			if len(elistener.PolicyNames) > 0 {
				delback = append(delback, elistener.BackendPort)
			}
		}
	}
	for _, port := range delback {
		klog.FromContext(ctx).V(2).Info(fmt.Sprintf("Reseting policies on backend port %d", port))
		_, err := c.api.LBU().SetLoadBalancerPoliciesForBackendServerWithContext(ctx, &elb.SetLoadBalancerPoliciesForBackendServerInput{
			LoadBalancerName: aws.String(l.Name),
			InstancePort:     aws.Int64(int64(port)),
			PolicyNames:      []*string{},
		})
		if err != nil {
			return fmt.Errorf("unset backend policy: %w", err)
		}
	}
	if len(del) > 0 {
		klog.FromContext(ctx).V(2).Info(fmt.Sprintf("Deleting %d listeners", len(del)))
		_, err := c.api.OAPI().DeleteLoadBalancerListeners(ctx, osc.DeleteLoadBalancerListenersRequest{
			LoadBalancerName:  l.Name,
			LoadBalancerPorts: del,
		})
		if err != nil {
			return fmt.Errorf("delete unused listeners: %w", err)
		}
	}

	// Add new listeners
	var add []osc.ListenerForCreation
	for _, listener := range expect {
		if !slices.ContainsFunc(existing.Listeners, func(elistener osc.Listener) bool {
			return oscListenersAreEqual(elistener, listener)
		}) {
			add = append(add, listener)
		}
	}
	if len(add) > 0 {
		klog.FromContext(ctx).V(2).Info(fmt.Sprintf("Adding %d listeners", len(add)))
		_, err := c.api.OAPI().CreateLoadBalancerListeners(ctx, osc.CreateLoadBalancerListenersRequest{
			LoadBalancerName: l.Name,
			Listeners:        add,
		})
		if err != nil {
			return fmt.Errorf("add new listeners: %w", err)
		}
	}
	return nil
}

func oscListenersAreEqual(actual osc.Listener, expected osc.ListenerForCreation) bool {
	if !protocolsAreEqual(actual.LoadBalancerProtocol, expected.LoadBalancerProtocol) {
		return false
	}
	if !protocolsAreEqual(actual.BackendProtocol, *expected.BackendProtocol) {
		return false
	}
	if actual.LoadBalancerPort != expected.LoadBalancerPort {
		return false
	}
	if actual.BackendPort != expected.BackendPort {
		return false
	}
	return true
}

// protocolsAreEqual checks if two ELB protocol strings are considered the same
// Comparison is case insensitive
func protocolsAreEqual(l, r string) bool {
	return strings.EqualFold(l, r)
}

func certificateAreEqual(l *string, r string) bool {
	var ls string
	if l != nil {
		ls = *l
	}
	return ls == r
}

func (c cloudLBU) updateSSLCert(ctx context.Context, l *LoadBalancer, existing *osc.LoadBalancer) error {
	for _, listener := range existing.Listeners {
		if !certificateAreEqual(listener.ServerCertificateId, l.ListenerDefaults.SSLCertificate) {
			klog.FromContext(ctx).V(2).Info("Changing certificate", "port", listener.LoadBalancerPort)
			_, err := c.api.OAPI().UpdateLoadBalancer(ctx, osc.UpdateLoadBalancerRequest{
				LoadBalancerName:    l.Name,
				ServerCertificateId: &l.ListenerDefaults.SSLCertificate,
			})
			if err != nil {
				return fmt.Errorf("set certificate: %w", err)
			}
		}
	}
	return nil
}

func (c cloudLBU) updateAttributes(ctx context.Context, l *LoadBalancer, _ *osc.LoadBalancer) error {
	existing, err := c.api.LBU().DescribeLoadBalancerAttributesWithContext(ctx, &elb.DescribeLoadBalancerAttributesInput{
		LoadBalancerName: aws.String(l.Name),
	})
	if err != nil {
		return fmt.Errorf("check LB attributes: %w", err)
	}
	expected := l.elbAttributes()
	if !accessLogAttributesAreEqual(existing.LoadBalancerAttributes, expected) {
		klog.FromContext(ctx).V(2).Info("Updating access log attributes")
		_, err := c.api.LBU().ModifyLoadBalancerAttributesWithContext(ctx, &elb.ModifyLoadBalancerAttributesInput{
			LoadBalancerName: aws.String(l.Name),
			LoadBalancerAttributes: &elb.LoadBalancerAttributes{
				AccessLog: expected.AccessLog,
			},
		})
		if err != nil {
			return fmt.Errorf("update access log attribute: %w", err)
		}
	}
	if !connectionAttributesAreEqual(existing.LoadBalancerAttributes, expected) {
		klog.FromContext(ctx).V(2).Info("Updating connection attributes")
		_, err := c.api.LBU().ModifyLoadBalancerAttributesWithContext(ctx, &elb.ModifyLoadBalancerAttributesInput{
			LoadBalancerName: aws.String(l.Name),
			LoadBalancerAttributes: &elb.LoadBalancerAttributes{
				ConnectionDraining: expected.ConnectionDraining,
				ConnectionSettings: expected.ConnectionSettings,
			},
		})
		if err != nil {
			return fmt.Errorf("update connection attribute: %w", err)
		}
	}
	return nil
}

func accessLogAttributesAreEqual(actual, expected *elb.LoadBalancerAttributes) bool {
	switch {
	case (actual.AccessLog == nil) != (expected.AccessLog == nil):
		return false
	case expected.AccessLog == nil:
	case !equal(actual.AccessLog.Enabled, expected.AccessLog.Enabled):
		return false
	case aws.BoolValue(expected.AccessLog.Enabled):
		if !equal(actual.AccessLog.EmitInterval, expected.AccessLog.EmitInterval) ||
			!equal(actual.AccessLog.S3BucketName, expected.AccessLog.S3BucketName) ||
			!equal(actual.AccessLog.S3BucketPrefix, expected.AccessLog.S3BucketPrefix) {
			return false
		}
	}
	return true
}

func connectionAttributesAreEqual(actual, expected *elb.LoadBalancerAttributes) bool {
	switch {
	case (actual.ConnectionDraining == nil) != (expected.ConnectionDraining == nil):
		return false
	case expected.ConnectionDraining == nil:
	case !equal(actual.ConnectionDraining.Enabled, expected.ConnectionDraining.Enabled):
		return false
	case aws.BoolValue(expected.ConnectionDraining.Enabled):
		if !equal(actual.ConnectionDraining.Timeout, expected.ConnectionDraining.Timeout) {
			return false
		}
	}
	switch {
	case (actual.ConnectionSettings == nil) != (expected.ConnectionSettings == nil):
		return false
	case expected.ConnectionSettings == nil:
	case !equal(actual.ConnectionSettings.IdleTimeout, expected.ConnectionSettings.IdleTimeout):
		return false
	}
	return true
}

func equal[T comparable](a, b *T) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func (c cloudLBU) updateHealthcheck(ctx context.Context, l *LoadBalancer, existing *osc.LoadBalancer) error {
	expected := l.healthCheck()
	switch {
	case (existing == nil) != (expected == nil):
	case existing != nil:
		actual := existing.HealthCheck
		if expected.Path == actual.Path &&
			expected.HealthyThreshold == actual.HealthyThreshold &&
			expected.UnhealthyThreshold == actual.UnhealthyThreshold &&
			expected.CheckInterval == actual.CheckInterval &&
			expected.Timeout == actual.Timeout {
			return nil
		}
	}
	klog.FromContext(ctx).V(2).Info("Configuring healthcheck")
	_, err := c.api.OAPI().UpdateLoadBalancer(ctx, osc.UpdateLoadBalancerRequest{
		LoadBalancerName: l.Name,
		HealthCheck:      expected,
	})
	if err != nil {
		return fmt.Errorf("configure health check: %w", err)
	}

	return nil
}

func (c cloudLBU) updateBackendVms(ctx context.Context, l *LoadBalancer, vms []VM, existing *osc.LoadBalancer) error {
	// in most cases, there will be no change, preallocating would waste an alloc
	var add []string //nolint:prealloc
	for _, vm := range vms {
		if existing != nil && slices.Contains(existing.BackendVmIds, vm.ID) {
			continue
		}
		add = append(add, vm.ID)
	}
	if len(add) > 0 {
		klog.FromContext(ctx).V(2).Info("Adding backend instances", "count", len(add))
		_, err := c.api.OAPI().RegisterVmsInLoadBalancer(ctx, osc.RegisterVmsInLoadBalancerRequest{
			LoadBalancerName: l.Name,
			BackendVmIds:     add,
		})
		if err != nil {
			return fmt.Errorf("register instances: %w", err)
		}
	}
	if existing == nil {
		return nil
	}
	var remove []string //nolint:prealloc
	for _, i := range existing.BackendVmIds {
		if slices.ContainsFunc(vms, func(vm VM) bool {
			return i == vm.ID
		}) {
			continue
		}
		remove = append(remove, i)
	}
	if len(remove) > 0 {
		klog.FromContext(ctx).V(2).Info("Removing backend instances", "count", len(remove))
		_, err := c.api.OAPI().DeregisterVmsInLoadBalancer(ctx, osc.DeregisterVmsInLoadBalancerRequest{
			LoadBalancerName: l.Name,
			BackendVmIds:     remove,
		})
		if err != nil {
			return fmt.Errorf("deregister instances: %w", err)
		}
	}

	return nil
}

func (c cloudLBU) loadBackendSecurityGroup(ctx context.Context, l *LoadBalancer, vms []VM) (*osc.SecurityGroup, error) {
	sgIDs := sets.Set[string]{}
	for _, vm := range vms {
		for _, sg := range vm.cloudVm.SecurityGroups {
			sgIDs.Insert(sg.SecurityGroupId)
		}
	}
	resp, err := c.api.OAPI().ReadSecurityGroups(ctx, osc.ReadSecurityGroupsRequest{
		Filters: &osc.FiltersSecurityGroup{
			SecurityGroupIds: ptr.To(sets.List(sgIDs)),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("list SGs: %w", err)
	}
	if len(*resp.SecurityGroups) == 0 {
		return nil, errors.New("no SG found for target nodes")
	}

	var targetSG *osc.SecurityGroup
	roleTagCount := math.MaxInt

	for _, sg := range *resp.SecurityGroups {
		if tags.Has(sg.Tags, c.mainSGTagKey()) {
			klog.FromContext(ctx).V(4).Info("Found security group having main tag", "securityGroupId", sg.SecurityGroupId)
			targetSG = &sg
		}
		if tags.HasRole(sg.Tags, l.TargetRole) {
			nRoleTagCount := countRoleTags(sg.Tags)
			if nRoleTagCount < roleTagCount {
				klog.FromContext(ctx).V(4).Info("Found security group having role tag", "securityGroupId", sg.SecurityGroupId, "role", l.TargetRole, "nroles", nRoleTagCount)
				targetSG = &sg
				roleTagCount = nRoleTagCount
			}
		}
	}
	if targetSG == nil {
		targetSG = &(*resp.SecurityGroups)[0]
		klog.FromContext(ctx).V(3).Info("No security group found by tag, using a random one", "securityGroupId", targetSG.SecurityGroupId)
	}
	return targetSG, nil
}

func (c cloudLBU) updateBackendSecurityGroupRules(ctx context.Context, l *LoadBalancer, vms []VM, existing *osc.LoadBalancer) error {
	targetSG, err := c.loadBackendSecurityGroup(ctx, l, vms)
	if err != nil {
		return err
	}
	srcSGID := existing.SecurityGroups[0]

	// Adding new rules
	for _, listener := range l.Listeners {
		if slices.ContainsFunc(targetSG.InboundRules, func(r osc.SecurityGroupRule) bool {
			return r.FromPortRange == listener.BackendPort &&
				slices.ContainsFunc(r.SecurityGroupsMembers, func(m osc.SecurityGroupsMember) bool {
					return srcSGID == m.SecurityGroupId
				})
		}) {
			continue
		}
		klog.FromContext(ctx).V(2).Info("Adding rule", "from", srcSGID, "to", targetSG.SecurityGroupId, "port", listener.BackendPort)
		_, err := c.api.OAPI().CreateSecurityGroupRule(ctx, osc.CreateSecurityGroupRuleRequest{
			SecurityGroupId: targetSG.SecurityGroupId,
			Flow:            "Inbound",
			Rules: []osc.SecurityGroupRule{{
				IpProtocol:    "tcp",
				FromPortRange: listener.BackendPort,
				ToPortRange:   listener.BackendPort,
				SecurityGroupsMembers: []osc.SecurityGroupsMember{{
					SecurityGroupId: srcSGID,
				}},
			}},
		})
		if err != nil {
			return fmt.Errorf("add backend rule: %w", err)
		}
	}

	// Removing rules
	for _, r := range targetSG.InboundRules {
		// ignore if rule is not from the LB SG
		if !slices.ContainsFunc(r.SecurityGroupsMembers, func(m osc.SecurityGroupsMember) bool {
			return m.SecurityGroupId == srcSGID
		}) {
			continue
		}
		// ignore if port is not from a lister
		if slices.ContainsFunc(l.Listeners, func(listener Listener) bool {
			return listener.BackendPort == r.FromPortRange
		}) {
			continue
		}
		klog.FromContext(ctx).V(2).Info("Deleting rule", "from", srcSGID, "to", targetSG.SecurityGroupId, "port", r.FromPortRange)
		_, err := c.api.OAPI().DeleteSecurityGroupRule(ctx, osc.DeleteSecurityGroupRuleRequest{
			SecurityGroupId: targetSG.SecurityGroupId,
			Flow:            "Inbound",
			Rules: []osc.SecurityGroupRule{{
				IpProtocol:    r.IpProtocol,
				FromPortRange: r.FromPortRange,
				ToPortRange:   r.ToPortRange,
				SecurityGroupsMembers: []osc.SecurityGroupsMember{{
					SecurityGroupId: srcSGID,
				}},
			}},
		})
		if err != nil {
			return fmt.Errorf("delete backend rule: %w", err)
		}
	}
	return nil
}

// DeleteLoadBalancer deletes a load balancer.
func (c cloudLBU) deleteLoadBalancer(ctx context.Context, l *LoadBalancer) error {
	klog.FromContext(ctx).V(2).Info("Deleting load-balancer")
	_, err := c.api.OAPI().DeleteLoadBalancer(ctx, osc.DeleteLoadBalancerRequest{
		LoadBalancerName: l.Name,
	})
	if err != nil {
		return fmt.Errorf("delete LBU: %w", err)
	}
	return nil
}
