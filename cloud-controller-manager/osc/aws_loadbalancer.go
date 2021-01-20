// +build !providerless

/*
Copyright 2014 The Kubernetes Authors.

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
	"fmt"
	"reflect"
	"strconv"
	"strings"

    "github.com/outscale/osc-sdk-go/osc"

    "github.com/aws/aws-sdk-go/aws/awserr"

	"k8s.io/klog"

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/antihax/optional"
)

const (
	// ProxyProtocolPolicyName is the tag named used for the proxy protocol
	// policy
	ProxyProtocolPolicyName = "k8s-proxyprotocol-enabled"

	// SSLNegotiationPolicyNameFormat is a format string used for the SSL
	// negotiation policy tag name
	SSLNegotiationPolicyNameFormat = "k8s-SSLNegotiationPolicy-%s"

	lbAttrLoadBalancingCrossZoneEnabled = "load_balancing.cross_zone.enabled"
	lbAttrAccessLogsS3Enabled           = "access_logs.s3.enabled"
	lbAttrAccessLogsS3Bucket            = "access_logs.s3.bucket"
	lbAttrAccessLogsS3Prefix            = "access_logs.s3.prefix"
)

var (
	// Defaults for LBU Healthcheck
	defaultHCHealthyThreshold   = int64(2)
	defaultHCUnhealthyThreshold = int64(6)
	defaultHCTimeout            = int64(5)
	defaultHCInterval           = int64(10)
)

type nlbPortMapping struct {
	FrontendPort     int64
	FrontendProtocol string

	TrafficPort     int64
	TrafficProtocol string

	HealthCheckPort     int64
	HealthCheckPath     string
	HealthCheckProtocol string

	SSLCertificateARN string
	SSLPolicy         string
}

// getLoadBalancerAdditionalTags converts the comma separated list of key-value
// pairs in the ServiceAnnotationLoadBalancerAdditionalTags annotation and returns
// it as a map.
func getLoadBalancerAdditionalTags(annotations map[string]string) map[string]string {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("getLoadBalancerAdditionalTags(%v)", annotations)
	additionalTags := make(map[string]string)
	if additionalTagsList, ok := annotations[ServiceAnnotationLoadBalancerAdditionalTags]; ok {
		additionalTagsList = strings.TrimSpace(additionalTagsList)

		// Break up list of "Key1=Val,Key2=Val2"
		tagList := strings.Split(additionalTagsList, ",")

		// Break up "Key=Val"
		for _, tagSet := range tagList {
			tag := strings.Split(strings.TrimSpace(tagSet), "=")

			// Accept "Key=val" or "Key=" or just "Key"
			if len(tag) >= 2 && len(tag[0]) != 0 {
				// There is a key and a value, so save it
				additionalTags[tag[0]] = tag[1]
			} else if len(tag) == 1 && len(tag[0]) != 0 {
				// Just "Key"
				additionalTags[tag[0]] = ""
			}
		}
	}

	return additionalTags
}

func (c *Cloud) getVpcCidrBlocks() ([]string, error) {
	debugPrintCallerFunctionName()
	vpcs, httpRes, err := c.fcu.ReadNets(&osc.ReadNetsOpts{
		ReadNetsRequest: optional.NewInterface(
			osc.ReadNetsRequest{
				Filters: osc.FiltersNet{
					NetIds: []string{c.netID},
				},
			}),
	})
	if err != nil {
	    if httpRes != nil {
			return nil, fmt.Errorf(httpRes.Status)
		}
		return nil, fmt.Errorf("error querying VPC for LBU: %q", err)
	}
	if len(vpcs.Nets) != 1 {
		return nil, fmt.Errorf("error querying VPC for LBU, got %d vpcs for %s", len(vpcs.Nets), c.netID)
	}

    //A verifier
	cidrBlocks := make([]string, 0, len(vpcs.Nets[0].IpRange))
	for _, cidr := range vpcs.Nets[0].IpRange {
		cidrBlocks = append(cidrBlocks, string(cidr))
	}
	return cidrBlocks, nil
}

// updateInstanceSecurityGroupsForNLB will adjust securityGroup's settings to allow inbound traffic into instances from clientCIDRs and portMappings.
// TIP: if either instances or clientCIDRs or portMappings are nil, then the securityGroup rules for lbName are cleared.
func (c *Cloud) updateInstanceSecurityGroupsForNLB(lbName string, instances map[InstanceID]osc.Vm, clientCIDRs []string, portMappings []nlbPortMapping) error {
	debugPrintCallerFunctionName()

	if c.cfg.Global.DisableSecurityGroupIngress {
		return nil
	}

	clusterSGs, err := c.getTaggedSecurityGroups()
	if err != nil {
		return fmt.Errorf("error querying for tagged security groups: %q", err)
	}
	// scan instances for groups we want to open
	desiredSGIDs := sets.String{}
	for _, instance := range instances {
		sg, err := findSecurityGroupForInstance(instance, clusterSGs)
		if err != nil {
			return err
		}
		if sg == (osc.SecurityGroupLight{}) {
			klog.Warningf("Ignoring instance without security group: %s", instance.VmId)
			continue
		}
		desiredSGIDs.Insert(sg.SecurityGroupId)
	}

	// TODO(@M00nF1sh): do we really needs to support SG without cluster tag at current version?
	// findSecurityGroupForInstance might return SG that are not tagged.
	{
		for sgID := range desiredSGIDs.Difference(sets.StringKeySet(clusterSGs)) {
			sg, err := c.findSecurityGroup(sgID)
			if err != nil {
				return fmt.Errorf("error finding instance group: %q", err)
			}
			clusterSGs[sgID] = sg
		}
	}

	{
		clientPorts := sets.Int64{}
		healthCheckPorts := sets.Int64{}
		for _, port := range portMappings {
			clientPorts.Insert(port.TrafficPort)
			healthCheckPorts.Insert(port.HealthCheckPort)
		}
		clientRuleAnnotation := fmt.Sprintf("%s=%s", NLBClientRuleDescription, lbName)
		healthRuleAnnotation := fmt.Sprintf("%s=%s", NLBHealthCheckRuleDescription, lbName)
		vpcCIDRs, err := c.getVpcCidrBlocks()
		if err != nil {
			return err
		}
		for sgID, sg := range clusterSGs {
			sgPerms := NewSecurityGroupRuleSet(sg.InboundRules...).Ungroup()
			if desiredSGIDs.Has(sgID) {
				if err := c.updateInstanceSecurityGroupForNLBTraffic(sgID, sgPerms, healthRuleAnnotation, "tcp", healthCheckPorts, vpcCIDRs); err != nil {
					return err
				}
				if err := c.updateInstanceSecurityGroupForNLBTraffic(sgID, sgPerms, clientRuleAnnotation, "tcp", clientPorts, clientCIDRs); err != nil {
					return err
				}
			} else {
				if err := c.updateInstanceSecurityGroupForNLBTraffic(sgID, sgPerms, healthRuleAnnotation, "tcp", nil, nil); err != nil {
					return err
				}
				if err := c.updateInstanceSecurityGroupForNLBTraffic(sgID, sgPerms, clientRuleAnnotation, "tcp", nil, nil); err != nil {
					return err
				}
			}
			if !sgPerms.Equal(NewSecurityGroupRuleSet(sg.InboundRules...).Ungroup()) {
				if err := c.updateInstanceSecurityGroupForNLBMTU(sgID, sgPerms); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// updateInstanceSecurityGroupForNLBTraffic will manage permissions set(identified by ruleDesc) on securityGroup to match desired set(allow protocol traffic from ports/cidr).
// Note: sgPerms will be updated to reflect the current permission set on SG after update.
func (c *Cloud) updateInstanceSecurityGroupForNLBTraffic(sgID string, sgPerms SecurityGroupRuleSet,
	ruleDesc string, protocol string, ports sets.Int64, cidrs []string) error {

	debugPrintCallerFunctionName()
	klog.V(10).Infof("updateInstanceSecurityGroupForNLBTraffic(%v,%v,%v,%v,%v,%v)",
		sgID, sgPerms, ruleDesc, protocol, ports, cidrs)

	desiredPerms := NewSecurityGroupRuleSet()
	for port := range ports {
		for _, cidr := range cidrs {
			desiredPerms.Insert(osc.SecurityGroupRule{
				IpProtocol: protocol,
				FromPortRange:   int32(port),
				ToPortRange:     int32(port),
				IpRanges: []string{cidr},
			})
		}
	}

	permsToGrant := desiredPerms.Difference(sgPerms)
	permsToRevoke := sgPerms.Difference(desiredPerms)
	permsToRevoke.DeleteIf(SecurityGroupRuleNotMatch{SecurityGroupRuleMatchDesc{ruleDesc}})
	if len(permsToRevoke) > 0 {
		permsToRevokeList := permsToRevoke.List()
		changed, err := c.removeSecurityGroupIngress(sgID, permsToRevokeList, false)
		if err != nil {
			klog.Warningf("Error remove traffic permission from security group: %q", err)
			return err
		}
		if !changed {
			klog.Warning("Revoking ingress was not needed; concurrent change? groupId=", sgID)
		}
		sgPerms.Delete(permsToRevokeList...)
	}
	if len(permsToGrant) > 0 {
		permsToGrantList := permsToGrant.List()
		changed, err := c.addSecurityGroupIngress(sgID, permsToGrantList, false)
		if err != nil {
			klog.Warningf("Error add traffic permission to security group: %q", err)
			return err
		}
		if !changed {
			klog.Warning("Allowing ingress was not needed; concurrent change? groupId=", sgID)
		}
		sgPerms.Insert(permsToGrantList...)
	}
	return nil
}

// Note: sgPerms will be updated to reflect the current permission set on SG after update.
func (c *Cloud) updateInstanceSecurityGroupForNLBMTU(sgID string, sgPerms SecurityGroupRuleSet) error {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("updateInstanceSecurityGroupForNLBMTU(%v,%v)", sgID, sgPerms)
	desiredPerms := NewSecurityGroupRuleSet()
	for _, perm := range sgPerms {
		for _, ipRange := range perm.IpRanges {
			if strings.Contains(ipRange, NLBClientRuleDescription) {
				desiredPerms.Insert(osc.SecurityGroupRule{
					IpProtocol: "icmp",
					FromPortRange:   3,
					ToPortRange:     4,
					IpRanges: []string{ipRange},
					//Description: NLBMtuDiscoveryRuleDescription,
				})
			}
		}
	}

	permsToGrant := desiredPerms.Difference(sgPerms)
	permsToRevoke := sgPerms.Difference(desiredPerms)
	permsToRevoke.DeleteIf(SecurityGroupRuleNotMatch{SecurityGroupRuleMatchDesc{NLBMtuDiscoveryRuleDescription}})
	if len(permsToRevoke) > 0 {
		permsToRevokeList := permsToRevoke.List()
		changed, err := c.removeSecurityGroupIngress(sgID, permsToRevokeList, false)
		if err != nil {
			klog.Warningf("Error remove MTU permission from security group: %q", err)
			return err
		}
		if !changed {
			klog.Warning("Revoking ingress was not needed; concurrent change? groupId=", sgID)
		}

		sgPerms.Delete(permsToRevokeList...)
	}
	if len(permsToGrant) > 0 {
		permsToGrantList := permsToGrant.List()
		changed, err := c.addSecurityGroupIngress(sgID, permsToGrantList, false)
		if err != nil {
			klog.Warningf("Error add MTU permission to security group: %q", err)
			return err
		}
		if !changed {
			klog.Warning("Allowing ingress was not needed; concurrent change? groupId=", sgID)
		}
		sgPerms.Insert(permsToGrantList...)
	}
	return nil
}

func (c *Cloud) ensureLoadBalancer(namespacedName types.NamespacedName, loadBalancerName string,
	listeners []osc.ListenerForCreation, subnetIDs []string, securityGroupIDs []string, internalLBU,
	proxyProtocol bool, loadBalancerAttributes osc.LoadBalancer,
	annotations map[string]string) (osc.LoadBalancer, error) {

	debugPrintCallerFunctionName()
	klog.V(10).Infof("ensureLoadBalancer(%v,%v,%v,%v,%v,%v,%v,%v,%v,)",
		namespacedName, loadBalancerName, listeners, subnetIDs, securityGroupIDs,
		internalLBU, proxyProtocol, loadBalancerAttributes, annotations)

	loadBalancer, err := c.describeLoadBalancer(loadBalancerName)
	if err != nil {
		return osc.LoadBalancer{}, err
	}

	dirty := false

	if loadBalancer.LoadBalancerName == "" {
	    request := osc.CreateLoadBalancerRequest{
				LoadBalancerName:          loadBalancerName,
                Listeners:                 listeners,
        }


		if internalLBU {
		    request.LoadBalancerType = "internal"
		}

		// We are supposed to specify one subnet per AZ.
		// TODO: What happens if we have more than one subnet per AZ?
		if subnetIDs == nil {
		    request.Subnets = nil
            request.SubregionNames = append(request.SubregionNames, c.selfOSCInstance.availabilityZone)

		} else {
		    request.Subnets = subnetIDs
		}

		if securityGroupIDs == nil || subnetIDs == nil {
		    request.SecurityGroups = nil
		} else {
		    request.SecurityGroups = securityGroupIDs
		}

		// Get additional tags set by the user
		tags := getLoadBalancerAdditionalTags(annotations)

		// Add default tags
		tags[TagNameKubernetesService] = namespacedName.String()
		tags = c.tagging.buildTags(ResourceLifecycleOwned, tags)

		for k, v := range tags {
			request.Tags = append(request.Tags, osc.ResourceTag{
				Key: k, Value: v,
			})
		}

		createRequest := osc.CreateLoadBalancerOpts{CreateLoadBalancerRequest: optional.NewInterface(request)}

		klog.Infof("Creating load balancer for %v with name: %s", namespacedName, loadBalancerName)
		klog.Infof("c.lbu.CreateLoadBalancer(createRequest): %v", createRequest)

		_, httpRes, err := c.lbu.CreateLoadBalancer(&createRequest)
		if err != nil {
		    if httpRes != nil {
			    return osc.LoadBalancer{}, fmt.Errorf(httpRes.Status)
		    }
		    klog.Infof("c.lbu.CreateLoadBalancer(createRequest) Error : %v", err)
			return osc.LoadBalancer{}, err
		}

		if proxyProtocol {
			err = c.createProxyProtocolPolicy(loadBalancerName)
			if err != nil {
				return osc.LoadBalancer{}, err
			}

			for _, listener := range listeners {
				klog.V(2).Infof("Adjusting OSC loadbalancer proxy protocol on node port %d. Setting to true", listener.BackendPort)
				err := c.setBackendPolicies(loadBalancerName, listener.BackendPort, []string{ProxyProtocolPolicyName})
				if err != nil {
					return osc.LoadBalancer{}, err
				}
			}
		}

		dirty = true
	} else {
		// TODO: Sync internal vs non-internal
		{
			// Sync subnets
			expected := sets.NewString(subnetIDs...)
			actual := stringSetFromPointers(loadBalancer.Subnets)
			additions := expected.Difference(actual)
			removals := actual.Difference(expected)
			klog.Warningf("AttachLoadBalancerToSubnets/DetachLoadBalancerFromSubnets loadBalancer: %v / expected: %v / actual %v / additions %v / removals %v",
				loadBalancer, expected, actual, additions, removals)
			if removals.Len() != 0 {
				klog.Warningf("DetachLoadBalancerFromSubnets not supported loadBalancer: %v / expected: %v / actual %v / additions %v / removals %v",
					loadBalancer, expected, actual, additions, removals)
				dirty = true
			}
			if additions.Len() != 0 {
				klog.Warningf("AttachLoadBalancerToSubnets not supported loadBalancer: %v / expected: %v / actual %v / additions %v / removals %v",
					loadBalancer, expected, actual, additions, removals)
				dirty = true
			}
		}
		{
			// Sync security groups
			expected := sets.NewString(securityGroupIDs...)
			actual := stringSetFromPointers(loadBalancer.SecurityGroups)
			if len(subnetIDs) == 0 || c.netID == "" {
				actual = sets.NewString([]string{DefaultSrcSgName}...)
			}

			klog.Infof("ApplySecurityGroupsToLoadBalancer: loadBalancer: %v expected: %v / actual %v",
				loadBalancer, expected, actual)
			if !expected.Equal(actual) {
				klog.Warningf("ApplySecurityGroupsToLoadBalancer not supported loadBalancer: %v expected: %v / actual %v",
					loadBalancer, expected, actual)
			}
		}
		{
			additions, removals := syncLbuListeners(loadBalancerName, listeners, loadBalancer.Listeners)
			if len(removals) != 0 {
				request := &osc.DeleteLoadBalancerListenersOpts{
                    DeleteLoadBalancerListenersRequest: optional.NewInterface(
                        osc.DeleteLoadBalancerListenersRequest{
                            LoadBalancerName: loadBalancerName,
                            LoadBalancerPorts: removals,
                        }),
				}

				klog.V(2).Info("Deleting removed load balancer listeners")
				if _, httpRes, err := c.lbu.DeleteLoadBalancerListeners(request); err != nil {

					return osc.LoadBalancer{}, fmt.Errorf("error deleting OSC loadbalancer listeners: %q %q", err, httpRes.Status)
				}
				dirty = true
			}

			if len(additions) != 0 {
				request := &osc.CreateLoadBalancerListenersOpts{
				    CreateLoadBalancerListenersRequest: optional.NewInterface(
                        osc.CreateLoadBalancerListenersRequest {
                            LoadBalancerName: loadBalancerName,
                            Listeners: additions,
                        }),
				}

				klog.V(2).Info("Creating added load balancer listeners")
				if _, httpRes, err := c.lbu.CreateLoadBalancerListeners(request); err != nil {
					return osc.LoadBalancer{}, fmt.Errorf("error creating OSC loadbalancer listeners: %q %q", err, httpRes.Status)
				}
				dirty = true
			}
		}

		{
			// Sync proxy protocol state for new and existing listeners

			proxyPolicies := make([]string, 0)
			if proxyProtocol {
				// Ensure the backend policy exists

				// NOTE The documentation for the OSC API indicates we could get an HTTP 400
				// back if a policy of the same name already exists. However, the aws-sdk does not
				// seem to return an error to us in these cases. Therefore, this will issue an API
				// request every time.
				err := c.createProxyProtocolPolicy(loadBalancerName)
				if err != nil {
					return osc.LoadBalancer{}, err
				}

				proxyPolicies = append(proxyPolicies, ProxyProtocolPolicyName)
			}

			foundBackends := make(map[int32]bool)
			proxyProtocolBackends := make(map[int32]bool)
			for _, backendListener := range loadBalancer.Listeners {
				foundBackends[backendListener.BackendPort] = false
				proxyProtocolBackends[backendListener.BackendPort] = proxyProtocolEnabled(backendListener)
			}

			for _, listener := range listeners {
				setPolicy := false
				instancePort := listener.BackendPort

				if currentState, ok := proxyProtocolBackends[instancePort]; !ok {
					// This is a new LBU backend so we only need to worry about
					// potentially adding a policy and not removing an
					// existing one
					setPolicy = proxyProtocol
				} else {
					foundBackends[instancePort] = true
					// This is an existing LBU backend so we need to determine
					// if the state changed
					setPolicy = (currentState != proxyProtocol)
				}

				if setPolicy {
					klog.V(2).Infof("Adjusting OSC loadbalancer proxy protocol on node port %d. Setting to %t", instancePort, proxyProtocol)
					err := c.setBackendPolicies(loadBalancerName, int32(instancePort), proxyPolicies)
					if err != nil {
						return osc.LoadBalancer{}, err
					}
					dirty = true
				}
			}

			// We now need to figure out if any backend policies need removed
			// because these old policies will stick around even if there is no
			// corresponding listener anymore
			for instancePort, found := range foundBackends {
				if !found {
					klog.V(2).Infof("Adjusting OSC loadbalancer proxy protocol on node port %d. Setting to false", instancePort)
					err := c.setBackendPolicies(loadBalancerName, int32(instancePort), []string{})
					if err != nil {
						return osc.LoadBalancer{}, err
					}
					dirty = true
				}
			}
		}

		{
			// Add additional tags
			klog.V(2).Infof("Creating additional load balancer tags for %s", loadBalancerName)
			tags := getLoadBalancerAdditionalTags(annotations)
			if len(tags) > 0 {
				err := c.addLoadBalancerTags(loadBalancerName, tags)
				if err != nil {
					return osc.LoadBalancer{}, fmt.Errorf("unable to create additional load balancer tags: %v", err)
				}
			}
		}
	}

	// Whether the LBU was new or existing, sync attributes regardless. This accounts for things
	// that cannot be specified at the time of creation and can only be modified after the fact,
	// e.g. idle connection timeout.
	{
		describeAttributesRequest := &osc.ReadLoadBalancersOpts{
		    ReadLoadBalancersRequest: optional.NewInterface(
                osc.ReadLoadBalancersRequest{
                    Filters: osc.FiltersLoadBalancer{
                        LoadBalancerNames: []string{loadBalancerName},
                    },
                }),
		}

		describeAttributesOutput, httpRes, err := c.lbu.ReadLoadBalancers(describeAttributesRequest)
		if err != nil {
		    if httpRes != nil {
                return osc.LoadBalancer{}, fmt.Errorf(httpRes.Status)
            }
			klog.Warning("Unable to retrieve load balancer attributes during attribute sync")
			return osc.LoadBalancer{}, err
		}

		foundAttributes := describeAttributesOutput.LoadBalancers[0]

		// Update attributes if they're dirty
		if !reflect.DeepEqual(loadBalancerAttributes, foundAttributes) {
			modifyAttributesRequest := &osc.UpdateLoadBalancerOpts{
			    UpdateLoadBalancerRequest: optional.NewInterface(
			        osc.UpdateLoadBalancerRequest {
                            LoadBalancerName: loadBalancerName,
			                HealthCheck: loadBalancerAttributes.HealthCheck,
			                AccessLog: loadBalancerAttributes.AccessLog,
                        }),
			}

			klog.V(2).Infof("Updating load-balancer attributes for %q with attributes (%v)",
				loadBalancerName, loadBalancerAttributes)
			_, httpRes, err = c.lbu.UpdateLoadBalancer(modifyAttributesRequest)
			if err != nil {
				return osc.LoadBalancer{}, fmt.Errorf("Unable to update load balancer attributes during attribute sync: %q httpRes: %q", err, httpRes.Status)
			}
			dirty = true
		}
	}

	if dirty {
		loadBalancer, err = c.describeLoadBalancer(loadBalancerName)
		if err != nil {
			klog.Warning("Unable to retrieve load balancer after creation/update")
			return osc.LoadBalancer{}, err
		}
	}

	return loadBalancer, nil
}

// syncLbuListeners computes a plan to reconcile the desired vs actual state of the listeners on an LBU
// NOTE: there exists an O(nlgn) implementation for this function. However, as the default limit of
//       listeners per lbu is 100, this implementation is reduced from O(m*n) => O(n).
func syncLbuListeners(loadBalancerName string, listeners []osc.ListenerForCreation, listenerDescriptions []osc.Listener) ([]osc.ListenerForCreation, []int32) {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("syncLbuListeners(%v,%v,%v)", loadBalancerName, listeners, listenerDescriptions)
	foundSet := make(map[int]bool)
	removals := []int32{}
	additions := []osc.ListenerForCreation{}

	for _, listenerDescription := range listenerDescriptions {
		actual := listenerDescription
		if actual.LoadBalancerPort == 0 {
			klog.Warning("Ignoring empty listener in OSC loadbalancer: ", loadBalancerName)
			continue
		}

		found := false
		for i, expected := range listeners {
			if expected == (osc.ListenerForCreation{}) {
				klog.Warning("Ignoring empty desired listener for loadbalancer: ", loadBalancerName)
				continue
			}
			if lbuListenersAreEqual(actual, expected) {
				// The current listener on the actual
				// lbu is in the set of desired listeners.
				foundSet[i] = true
				found = true
				break
			}
		}
		if !found {
			removals = append(removals, actual.LoadBalancerPort)
		}
	}

	for i := range listeners {
		if !foundSet[i] {
			additions = append(additions, listeners[i])
		}
	}

	return additions, removals
}

func lbuListenersAreEqual(actual osc.Listener, expected osc.ListenerForCreation) bool {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("lbuListenersAreEqual(%v,%v)", actual, expected)
	if !lbuProtocolsAreEqual(actual.LoadBalancerProtocol, expected.LoadBalancerProtocol) {
		return false
	}
	if !lbuProtocolsAreEqual(actual.BackendProtocol, expected.BackendProtocol) {
		return false
	}
	if actual.BackendPort != expected.BackendPort {
		return false
	}
	if actual.LoadBalancerPort != expected.LoadBalancerPort {
		return false
	}
	if !oscArnEquals(actual.ServerCertificateId, expected.ServerCertificateId) {
		return false
	}
	return true
}

// lbuProtocolsAreEqual checks if two LBU protocol strings are considered the same
// Comparison is case insensitive
func lbuProtocolsAreEqual(l, r string) bool {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("lbuProtocolsAreEqual(%v,%v)", l, r)
	if l == "" || r == "" {
		return l == r
	}
	return strings.EqualFold(l, r)
}

// oscArnEquals checks if two ARN strings are considered the same
// Comparison is case insensitive
func oscArnEquals(l, r string) bool {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("oscArnEquals(%v,%v)", l, r)
	if l == "" || r == "" {
		return l == r
	}
	return strings.EqualFold(l, r)
}

// getExpectedHealthCheck returns an lbu.Healthcheck for the provided target
// and using either sensible defaults or overrides via Service annotations
func (c *Cloud) getExpectedHealthCheck(target string, annotations map[string]string) (osc.HealthCheck, error) {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("getExpectedHealthCheck(%v,%v)", target, annotations)
	healthcheck := osc.HealthCheck{Path: target}
	getOrDefault := func(annotation string, defaultValue int64) (int32, error) {
		i32 := defaultValue
		var err error
		if s, ok := annotations[annotation]; ok {
			i32, err = strconv.ParseInt(s, 10, 0)
			if err != nil {
				return 0, fmt.Errorf("failed parsing health check annotation value: %v", err)
			}
		}
		return int32(i32), nil
	}
	var err error
	healthcheck.HealthyThreshold, err = getOrDefault(ServiceAnnotationLoadBalancerHCHealthyThreshold, defaultHCHealthyThreshold)
	if err != nil {
		return osc.HealthCheck{}, err
	}
	healthcheck.UnhealthyThreshold, err = getOrDefault(ServiceAnnotationLoadBalancerHCUnhealthyThreshold, defaultHCUnhealthyThreshold)
	if err != nil {
		return osc.HealthCheck{}, err
	}
	healthcheck.Timeout, err = getOrDefault(ServiceAnnotationLoadBalancerHCTimeout, defaultHCTimeout)
	if err != nil {
		return osc.HealthCheck{}, err
	}
	healthcheck.CheckInterval, err = getOrDefault(ServiceAnnotationLoadBalancerHCInterval, defaultHCInterval)
	if err != nil {
		return osc.HealthCheck{}, err
	}

	// No method Validate for osc sdk go
// 	if err = healthcheck.Validate(); err != nil {
// 		return osc.HealthCheck{}, fmt.Errorf("some of the load balancer health check parameters are invalid: %v", err)
// 	}
	return healthcheck, nil
}

// Makes sure that the health check for an LBU matches the configured health check node port
func (c *Cloud) ensureLoadBalancerHealthCheck(loadBalancer osc.LoadBalancer,
	protocol string, port int32, path string, annotations map[string]string) error {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("ensureLoadBalancerHealthCheck(%v,%v, %v, %v, %v)",
		loadBalancer, protocol, port, path, annotations)
	klog.Infof("ensureLoadBalancerHealthCheck(%v,%v, %v, %v, %v)", loadBalancer, protocol, port, path, annotations)
	name := loadBalancer.LoadBalancerName

	actual := loadBalancer.HealthCheck
	expectedTarget := protocol + ":" + strconv.FormatInt(int64(port), 10) + path
	expected, err := c.getExpectedHealthCheck(expectedTarget, annotations)
	klog.Infof("ensureLoadBalancerHealthCheck expected expectedTarget %v %v)", expected, expectedTarget)
	if err != nil {
		return fmt.Errorf("cannot update health check for load balancer %q: %q", name, err)
	}

	// comparing attributes 1 by 1 to avoid breakage in case a new field is
	// added to the HC which breaks the equality
	if expected.Path == actual.Path &&
		expected.HealthyThreshold == actual.HealthyThreshold &&
		expected.UnhealthyThreshold == actual.UnhealthyThreshold &&
		expected.CheckInterval == actual.CheckInterval &&
		expected.Timeout == actual.Timeout {
		return nil
	}


    request := &osc.UpdateLoadBalancerOpts{
            UpdateLoadBalancerRequest: optional.NewInterface(
                osc.UpdateLoadBalancerRequest{
                    HealthCheck: expected,
                    LoadBalancerName: loadBalancer.LoadBalancerName,
                }),
    }
    klog.Infof("ensureLoadBalancerHealthCheck expected.Path %v expected.Protocol %v", expected.Path, expected.Protocol)


	_, httpRes, errUpdate := c.lbu.UpdateLoadBalancer(request)

	if err != nil {
	    if httpRes != nil {
			return fmt.Errorf(httpRes.Status)
		}
		return fmt.Errorf("error configuring load balancer health check for %q: %q", name, errUpdate)
	}

	return nil
}

// Makes sure that exactly the specified hosts are registered as instances with the load balancer
func (c *Cloud) ensureLoadBalancerInstances(loadBalancerName string,
	lbInstances []string,
	instanceIDs map[InstanceID]osc.Vm) error {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("ensureLoadBalancerInstances(%v,%v, %v)", loadBalancerName, lbInstances, instanceIDs)
	expected := sets.NewString()
	for id := range instanceIDs {
		expected.Insert(string(id))
	}

	actual := sets.NewString()
	for _, lbInstance := range lbInstances {
		actual.Insert(lbInstance)
	}

	additions := expected.Difference(actual)
	removals := actual.Difference(expected)

	addInstances := []string{}
	for _, instanceID := range additions.List() {
		addInstance := osc.Vm{}
		addInstance.VmId = instanceID
		addInstances = append(addInstances, addInstance.VmId)
	}

	removeInstances := []string{}
	for _, instanceID := range removals.List() {
		removeInstance := osc.Vm{}
		removeInstance.VmId = instanceID
		removeInstances = append(removeInstances, removeInstance.VmId)
	}
	klog.V(10).Infof("ensureLoadBalancerInstances register/Deregister addInstances(%v) , removeInstances(%v)", addInstances, removeInstances)

	if len(addInstances) > 0 {
		registerRequest := &osc.RegisterVmsInLoadBalancerOpts{
            RegisterVmsInLoadBalancerRequest: optional.NewInterface(
                osc.RegisterVmsInLoadBalancerRequest{
                    BackendVmIds: addInstances,
	                LoadBalancerName: loadBalancerName,
                }),
		}

		_, httpRes, err := c.lbu.RegisterVmsInLoadBalancer(registerRequest)
		if err != nil {
		    if httpRes != nil {
                return fmt.Errorf(httpRes.Status)
            }
			return err
		}
		klog.V(1).Infof("Instances added to load-balancer %s", loadBalancerName)
	}

	if len(removeInstances) > 0 {
		deregisterRequest := &osc.DeregisterVmsInLoadBalancerOpts{
            DeregisterVmsInLoadBalancerRequest: optional.NewInterface(
                osc.DeregisterVmsInLoadBalancerRequest{
                    BackendVmIds: removeInstances,
		            LoadBalancerName: loadBalancerName,
                }),
		}



		_, httpRes, err := c.lbu.DeregisterVmsInLoadBalancer(deregisterRequest)
		if err != nil {
		    if httpRes != nil {
                return fmt.Errorf(httpRes.Status)
            }
			return err
		}
		klog.V(1).Infof("Instances removed from load-balancer %s", loadBalancerName)
	}

	return nil
}

func (c *Cloud) getLoadBalancerTLSPorts(loadBalancer osc.LoadBalancer) []int32 {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("getLoadBalancerTLSPorts(%v)", loadBalancer)
	ports := []int32{}

	for _, listenerDescription := range loadBalancer.Listeners {
		protocol := listenerDescription.LoadBalancerProtocol
		if protocol == "SSL" || protocol == "HTTPS" {
			ports = append(ports, listenerDescription.LoadBalancerPort)
		}
	}
	return ports
}

func (c *Cloud) ensureSSLNegotiationPolicy(loadBalancer osc.LoadBalancer, policyName string) error {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("ensureSSLNegotiationPolicy(%v,%v)", loadBalancer, policyName)
	klog.V(2).Info("Describing load balancer policies on load balancer")
	result, httpRes, err := c.lbu.ReadLoadBalancers(&osc.ReadLoadBalancersOpts{
	    ReadLoadBalancersRequest: optional.NewInterface(
	        osc.ReadLoadBalancersRequest{
	            Filters: osc.FiltersLoadBalancer{
	                LoadBalancerNames: []string{loadBalancer.LoadBalancerName},
	            },

	        }),
	    })

	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			//case osc.ErrCodePolicyNotFoundException:
			default:
				return fmt.Errorf("error describing security policies on load balancer: %q %q", err, httpRes.Status)
			}
		}
	}

	if len(result.LoadBalancers[0].LoadBalancerStickyCookiePolicies) > 0 {
		return nil
	}

	klog.V(2).Infof("Creating SSL negotiation policy '%s' on load balancer", fmt.Sprintf(SSLNegotiationPolicyNameFormat, policyName))
	// there is an upper limit of 98 policies on an LBU, we're pretty safe from
	// running into it
	_, httpRes, err = c.lbu.CreateLoadBalancerPolicy(
	    &osc.CreateLoadBalancerPolicyOpts{
	        CreateLoadBalancerPolicyRequest: optional.NewInterface(
	            osc.CreateLoadBalancerPolicyRequest{
	                    LoadBalancerName: loadBalancer.LoadBalancerName,
                        PolicyName:       fmt.Sprintf(SSLNegotiationPolicyNameFormat, policyName),
                        PolicyType:   "SSLNegotiationPolicyType",
	    			}),
	    })
	if err != nil {
	    if httpRes != nil {
			return fmt.Errorf(httpRes.Status)
		}
		return fmt.Errorf("error creating security policy on load balancer: %q", err)
	}
	return nil
}

func (c *Cloud) setSSLNegotiationPolicy(loadBalancerName, sslPolicyName string, port int32) error {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("setSSLNegotiationPolicy(%v,%v,%v)", loadBalancerName, sslPolicyName, port)
	policyName := fmt.Sprintf(SSLNegotiationPolicyNameFormat, sslPolicyName)
	request := &osc.CreateLoadBalancerListenersOpts{
	    CreateLoadBalancerListenersRequest: optional.NewInterface(
	        osc.CreateLoadBalancerListenersRequest{
	            LoadBalancerName: loadBalancerName,
	            //A verifier
	            //Listeners: []ListenerForCreation{}
	        }),

	}
	klog.V(2).Infof("Setting SSL negotiation policy '%s' on load balancer", policyName)
	_, httpRes, err := c.lbu.CreateLoadBalancerListeners(request)
	if err != nil {
		return fmt.Errorf("error setting SSL negotiation policy '%s' on load balancer: %q %q", policyName, err, httpRes.Status)
	}
	return nil
}

func (c *Cloud) createProxyProtocolPolicy(loadBalancerName string) error {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("createProxyProtocolPolicy(%v)", loadBalancerName)
	request := &osc.CreateLoadBalancerPolicyOpts{
	    CreateLoadBalancerPolicyRequest: optional.NewInterface(
	        osc.CreateLoadBalancerPolicyRequest{
	            LoadBalancerName: loadBalancerName,
	            PolicyName: ProxyProtocolPolicyName,
	            PolicyType: "ProxyProtocolPolicyType",

	        }),
	}
	klog.V(2).Info("Creating proxy protocol policy on load balancer")
	_, httpRes, err := c.lbu.CreateLoadBalancerPolicy(request)
	if err != nil {
	    if httpRes != nil {
			return fmt.Errorf(httpRes.Status)
		}
		return fmt.Errorf("error creating proxy protocol policy on load balancer: %q", err)
	}

	return nil
}

func (c *Cloud) setBackendPolicies(loadBalancerName string, instancePort int32, policies []string) error {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("setBackendPolicies(%v,%v,%v)", loadBalancerName, instancePort, policies)

    //A verifier
    request := &osc.CreateLoadBalancerPolicyOpts{
	    CreateLoadBalancerPolicyRequest: optional.NewInterface(
	        osc.CreateLoadBalancerPolicyRequest{
	            LoadBalancerName: loadBalancerName,
	            PolicyName: ProxyProtocolPolicyName,
	            PolicyType: "ProxyProtocolPolicyType",

	        }),
	}

	/* request := &osc.SetLoadBalancerPoliciesForBackendServerInput{
		InstancePort:     instancePort,
		LoadBalancerName: loadBalancerName,
		PolicyNames:      policies,
	} */
	if len(policies) > 0 {
		klog.V(2).Infof("Adding OSC loadbalancer backend policies on node port %d", instancePort)
	} else {
		klog.V(2).Infof("Removing OSC loadbalancer backend policies on node port %d", instancePort)
	}
	_, httpRes, err := c.lbu.CreateLoadBalancerPolicy(request)
	if err != nil {
	    if httpRes != nil {
			return fmt.Errorf(httpRes.Status)
		}
		return fmt.Errorf("error adjusting OSC loadbalancer backend policies: %q", err)
	}

	return nil
}


// A Verifier
func proxyProtocolEnabled(backend osc.Listener) bool {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("proxyProtocolEnabled(%v)", backend)
	for _, policy := range backend.PolicyNames {
		if policy == ProxyProtocolPolicyName {
			return true
		}
	}

	return false
}

// findInstancesForLBU gets the OSC instances corresponding to the Nodes, for setting up an LBU
// We ignore Nodes (with a log message) where the instanceid cannot be determined from the provider,
// and we ignore instances which are not found
func (c *Cloud) findInstancesForLBU(nodes []*v1.Node) (map[InstanceID]osc.Vm, error) {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("findInstancesForLBU(%v)", nodes)

	for _, node := range nodes {
		if node.Spec.ProviderID == "" {
			// TODO  Need to be optimize by setting providerID which is not possible actualy
			instance, _ := c.findInstanceByNodeName(types.NodeName(node.Name))
			node.Spec.ProviderID = instance.VmId
		}
	}

	// Map to instance ids ignoring Nodes where we cannot find the id (but logging)
	instanceIDs := mapToOSCInstanceIDsTolerant(nodes)

	cacheCriteria := cacheCriteria{
		// MaxAge not required, because we only care about security groups, which should not change
		HasInstances: instanceIDs, // Refresh if any of the instance ids are missing
	}
	snapshot, err := c.instanceCache.describeAllInstancesCached(cacheCriteria)
	if err != nil {
		return nil, err
	}

	instances := snapshot.FindInstances(instanceIDs)
	// We ignore instances that cannot be found

	return instances, nil
}
