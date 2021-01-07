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
	"context"
	"fmt"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"reflect"

	_nethttp "net/http"

    "github.com/outscale/osc-sdk-go/osc"

    "github.com/antihax/optional"

    "github.com/aws/aws-sdk-go/aws/awserr"

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/informers"
	informercorev1 "k8s.io/client-go/informers/core/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/cloud-provider"
	servicehelpers "k8s.io/cloud-provider/service/helpers"
	"k8s.io/klog"
)

// ********************* CCM Cloud Object Def *********************

// Cloud is an implementation of Interface, LoadBalancer and Instances for Amazon Web Services.
type Cloud struct {
	fcu      FCU
	lbu      LBU
	metadata EC2Metadata
	cfg      *CloudConfig
	region   string
	netID    string

	tagging oscTagging

	// The OSC instance that we are running on
	// Note that we cache some state in oscInstance (mountpoints), so we must preserve the instance
	selfOSCInstance *oscInstance

	instanceCache instanceCache

	clientBuilder cloudprovider.ControllerClientBuilder
	kubeClient    clientset.Interface

	nodeInformer informercorev1.NodeInformer
	// Extract the function out to make it easier to test
	nodeInformerHasSynced cache.InformerSynced

	eventBroadcaster record.EventBroadcaster
	eventRecorder    record.EventRecorder
}

// ********************* CCM Cloud Object functions *********************

// ********************* CCM Cloud Context functions *********************
// Builds the oscInstance for the OSC instance on which we are running.
// This is called when the OSCCloud is initialized, and should not be called otherwise (because the oscInstance for the local instance is a singleton with drive mapping state)
func (c *Cloud) buildSelfOSCInstance() (*oscInstance, error) {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("buildSelfOSCInstance()")
	if c.selfOSCInstance != nil {
		panic("do not call buildSelfOSCInstance directly")
	}
	instanceID, err := c.metadata.GetMetadata("instance-id")
	if err != nil {
		return nil, fmt.Errorf("error fetching instance-id from osc metadata service: %q", err)
	}

	// We want to fetch the hostname via the OSC metadata service
	// (`GetMetadata("local-hostname")`): But see #11543 - we need to use
	// the OSC API to get the privateDnsName in case of a private DNS zone
	// e.g. mydomain.io, because the metadata service returns the wrong
	// hostname.  Once we're doing that, we might as well get all our
	// information from the instance returned by the OSC API - it is a
	// single API call to get all the information, and it means we don't
	// have two code paths.
	instance, err := c.getInstanceByID(instanceID)
	if err != nil {
		return nil, fmt.Errorf("error finding instance %s: %q", instanceID, err)
	}
	return newOSCInstance(c.fcu, instance), nil
}

// SetInformers implements InformerUser interface by setting up informer-fed caches for osc lib to
// leverage Kubernetes API for caching
func (c *Cloud) SetInformers(informerFactory informers.SharedInformerFactory) {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("SetInformers(%v)", informerFactory)
	klog.Infof("Setting up informers for Cloud")
	c.nodeInformer = informerFactory.Core().V1().Nodes()
	c.nodeInformerHasSynced = c.nodeInformer.Informer().HasSynced
}

// AddSSHKeyToAllInstances is currently not implemented.
func (c *Cloud) AddSSHKeyToAllInstances(ctx context.Context, user string, keyData []byte) error {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("AddSSHKeyToAllInstances(%v,%v)", user, keyData)
	return cloudprovider.NotImplemented
}

// CurrentNodeName returns the name of the current node
func (c *Cloud) CurrentNodeName(ctx context.Context, hostname string) (types.NodeName, error) {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("CurrentNodeName(%v)", hostname)
	return c.selfOSCInstance.nodeName, nil
}

// Initialize passes a Kubernetes clientBuilder interface to the cloud provider
func (c *Cloud) Initialize(clientBuilder cloudprovider.ControllerClientBuilder,
	stop <-chan struct{}) {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("Initialize(%v,%v)", clientBuilder, stop)
	c.clientBuilder = clientBuilder
	c.kubeClient = clientBuilder.ClientOrDie("aws-cloud-provider")
	c.eventBroadcaster = record.NewBroadcaster()
	c.eventBroadcaster.StartLogging(klog.Infof)
	c.eventBroadcaster.StartRecordingToSink(&v1core.EventSinkImpl{Interface: c.kubeClient.CoreV1().Events("")})
	c.eventRecorder = c.eventBroadcaster.NewRecorder(scheme.Scheme, v1.EventSource{Component: "aws-cloud-provider"})
}

// Clusters returns the list of clusters.
func (c *Cloud) Clusters() (cloudprovider.Clusters, bool) {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("Clusters()")
	return nil, false
}

// ProviderName returns the cloud provider ID.
func (c *Cloud) ProviderName() string {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("ProviderName")
	return ProviderName
}

// LoadBalancer returns an implementation of LoadBalancer for Amazon Web Services.
func (c *Cloud) LoadBalancer() (cloudprovider.LoadBalancer, bool) {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("LoadBalancer()")
	return c, true
}

// Instances returns an implementation of Instances for Amazon Web Services.
func (c *Cloud) Instances() (cloudprovider.Instances, bool) {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("Instances()")
	return c, true
}

// Zones returns an implementation of Zones for Amazon Web Services.
func (c *Cloud) Zones() (cloudprovider.Zones, bool) {
	debugPrintCallerFunctionName()
	return c, true
}

// Routes returns an implementation of Routes for Amazon Web Services.
func (c *Cloud) Routes() (cloudprovider.Routes, bool) {
	debugPrintCallerFunctionName()
	return c, true
}

// HasClusterID returns true if the cluster has a clusterID
func (c *Cloud) HasClusterID() bool {
	debugPrintCallerFunctionName()
	return len(c.tagging.clusterID()) > 0
}

// NodeAddresses is an implementation of Instances.NodeAddresses.
func (c *Cloud) NodeAddresses(ctx context.Context, name types.NodeName) ([]v1.NodeAddress, error) {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("NodeAddresses(%v)", name)
	if c.selfOSCInstance.nodeName == name || len(name) == 0 {
		addresses := []v1.NodeAddress{}

		macs, err := c.metadata.GetMetadata("network/interfaces/macs/")
		if err != nil {
			return nil, fmt.Errorf("error querying OSC metadata for %q: %q", "network/interfaces/macs", err)
		}

		for _, macID := range strings.Split(macs, "\n") {
			if macID == "" {
				continue
			}
			macPath := path.Join("network/interfaces/macs/", macID, "local-ipv4s")
			internalIPs, err := c.metadata.GetMetadata(macPath)
			if err != nil {
				return nil, fmt.Errorf("error querying OSC metadata for %q: %q", macPath, err)
			}
			for _, internalIP := range strings.Split(internalIPs, "\n") {
				if internalIP == "" {
					continue
				}
				addresses = append(addresses, v1.NodeAddress{Type: v1.NodeInternalIP, Address: internalIP})
			}
		}

		externalIP, err := c.metadata.GetMetadata("public-ipv4")
		if err != nil {
			//TODO: It would be nice to be able to determine the reason for the failure,
			// but the OSC client masks all failures with the same error description.
			klog.V(4).Info("Could not determine public IP from OSC metadata.")
		} else {
			addresses = append(addresses, v1.NodeAddress{Type: v1.NodeExternalIP, Address: externalIP})
		}

		localHostname, err := c.metadata.GetMetadata("local-hostname")
		if err != nil || len(localHostname) == 0 {
			//TODO: It would be nice to be able to determine the reason for the failure,
			// but the OSC client masks all failures with the same error description.
			klog.V(4).Info("Could not determine private DNS from OSC metadata.")
		} else {
			hostname, internalDNS := parseMetadataLocalHostname(localHostname)
			addresses = append(addresses, v1.NodeAddress{Type: v1.NodeHostName, Address: hostname})
			for _, d := range internalDNS {
				addresses = append(addresses, v1.NodeAddress{Type: v1.NodeInternalDNS, Address: d})
			}
		}

		externalDNS, err := c.metadata.GetMetadata("public-hostname")
		if err != nil || len(externalDNS) == 0 {
			//TODO: It would be nice to be able to determine the reason for the failure,
			// but the OSC client masks all failures with the same error description.
			klog.V(4).Info("Could not determine public DNS from OSC metadata.")
		} else {
			addresses = append(addresses, v1.NodeAddress{Type: v1.NodeExternalDNS, Address: externalDNS})
		}

		return addresses, nil
	}

	instance, err := c.getInstanceByNodeName(name)
	if err != nil {
		return nil, fmt.Errorf("getInstanceByNodeName failed for %q with %q", name, err)
	}
	return extractNodeAddresses(instance)
}

// NodeAddressesByProviderID returns the node addresses of an instances with the specified unique providerID
// This method will not be called from the node that is requesting this ID. i.e. metadata service
// and other local methods cannot be used here
func (c *Cloud) NodeAddressesByProviderID(ctx context.Context, providerID string) ([]v1.NodeAddress, error) {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("NodeAddressesByProviderID(%v)", providerID)
	instanceID, err := KubernetesInstanceID(providerID).MapToOSCInstanceID()
	if err != nil {
		return nil, err
	}

	instance, err := describeInstance(c.fcu, instanceID)
	if err != nil {
		return nil, err
	}

	return extractNodeAddresses(instance)
}

// InstanceExistsByProviderID returns true if the instance with the given provider id still exists.
// If false is returned with no error, the instance will be immediately deleted by the cloud controller manager.
func (c *Cloud) InstanceExistsByProviderID(ctx context.Context, providerID string) (bool, error) {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("InstanceExistsByProviderID(%v)", providerID)
	instanceID, err := KubernetesInstanceID(providerID).MapToOSCInstanceID()
	if err != nil {
		return false, err
	}

	request := &osc.ReadVmsOpts{
		ReadVmsRequest: optional.NewInterface(
			osc.ReadVmsRequest{
				Filters: osc.FiltersVm{
					VmIds: []string{string(instanceID)},
				},
			}),
	}

	instances, httpRes, err := c.fcu.ReadVms(request)
	if err != nil {
	    fmt.Errorf("http %q", httpRes)
		return false, err
	}
	if len(instances) == 0 {
		return false, nil
	}
	if len(instances) > 1 {
		return false, fmt.Errorf("multiple instances found for instance: %s", instanceID)
	}

	state := instances[0].State
	if state == "terminated" {
		klog.Warningf("the instance %s is terminated", instanceID)
		return false, nil
	}

	return true, nil
}

// InstanceShutdownByProviderID returns true if the instance is in safe state to detach volumes
func (c *Cloud) InstanceShutdownByProviderID(ctx context.Context, providerID string) (bool, error) {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("InstanceShutdownByProviderID(%v)", providerID)
	instanceID, err := KubernetesInstanceID(providerID).MapToOSCInstanceID()
	if err != nil {
		return false, err
	}

	request := &osc.ReadVmsOpts{
		ReadVmsRequest: optional.NewInterface(
			osc.ReadVmsRequest{
				Filters: osc.FiltersVm{
					VmIds: []string{string(instanceID)},
				},
			}),
	}

	instances, httpRes, err := c.fcu.ReadVms(request)
	if err != nil {
	    fmt.Errorf("http %q", httpRes)
		return false, err
	}
	if len(instances) == 0 {
		klog.Warningf("the instance %s does not exist anymore", providerID)
		// returns false, because otherwise node is not deleted from cluster
		// false means that it will continue to check InstanceExistsByProviderID
		return false, nil
	}
	if len(instances) > 1 {
		return false, fmt.Errorf("multiple instances found for instance: %s", instanceID)
	}

	instance := instances[0]
	if instance.State != "" {
		state := instance.State
		// valid state for detaching volumes
		if state == "stopped" {
			return true, nil
		}
	}
	return false, nil
}

// InstanceID returns the cloud provider ID of the node with the specified nodeName.
func (c *Cloud) InstanceID(ctx context.Context, nodeName types.NodeName) (string, error) {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("InstanceID(%v)", nodeName)
	// In the future it is possible to also return an endpoint as:
	// <endpoint>/<zone>/<instanceid>
	if c.selfOSCInstance.nodeName == nodeName {
		return "/" + c.selfOSCInstance.availabilityZone + "/" + c.selfOSCInstance.oscID, nil
	}
	inst, err := c.getInstanceByNodeName(nodeName)
	if err != nil {
		if err == cloudprovider.InstanceNotFound {
			// The Instances interface requires that we return InstanceNotFound (without wrapping)
			return "", err
		}
		return "", fmt.Errorf("getInstanceByNodeName failed for %q with %q", nodeName, err)
	}
	return "/" + inst.Placement.SubregionName + "/" + inst.VmId, nil
}

// InstanceTypeByProviderID returns the cloudprovider instance type of the node with the specified unique providerID
// This method will not be called from the node that is requesting this ID. i.e. metadata service
// and other local methods cannot be used here
func (c *Cloud) InstanceTypeByProviderID(ctx context.Context, providerID string) (string, error) {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("InstanceTypeByProviderID(%v)", providerID)
	instanceID, err := KubernetesInstanceID(providerID).MapToOSCInstanceID()
	if err != nil {
		return "", err
	}

	instance, err := describeInstance(c.fcu, instanceID)
	if err != nil {
		return "", err
	}

	return instance.VmType, nil
}

// InstanceType returns the type of the node with the specified nodeName.
func (c *Cloud) InstanceType(ctx context.Context, nodeName types.NodeName) (string, error) {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("InstanceType(%v)", nodeName)
	if c.selfOSCInstance.nodeName == nodeName {
		return c.selfOSCInstance.instanceType, nil
	}
	inst, err := c.getInstanceByNodeName(nodeName)
	if err != nil {
		return "", fmt.Errorf("getInstanceByNodeName failed for %q with %q", nodeName, err)
	}
	return inst.VmType, nil
}

// GetZone implements Zones.GetZone
func (c *Cloud) GetZone(ctx context.Context) (cloudprovider.Zone, error) {
	debugPrintCallerFunctionName()
	return cloudprovider.Zone{
		FailureDomain: c.selfOSCInstance.availabilityZone,
		Region:        c.region,
	}, nil
}

// GetZoneByProviderID implements Zones.GetZoneByProviderID
// This is particularly useful in external cloud providers where the kubelet
// does not initialize node data.
func (c *Cloud) GetZoneByProviderID(ctx context.Context, providerID string) (cloudprovider.Zone, error) {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("GetZoneByProviderID(%v)", providerID)
	instanceID, err := KubernetesInstanceID(providerID).MapToOSCInstanceID()
	if err != nil {
		return cloudprovider.Zone{}, err
	}
	instance, err := c.getInstanceByID(string(instanceID))
	if err != nil {
		return cloudprovider.Zone{}, err
	}

	zone := cloudprovider.Zone{
		FailureDomain: instance.Placement.SubregionName,
		Region:        c.region,
	}

	return zone, nil
}

// GetZoneByNodeName implements Zones.GetZoneByNodeName
// This is particularly useful in external cloud providers where the kubelet
// does not initialize node data.
func (c *Cloud) GetZoneByNodeName(ctx context.Context, nodeName types.NodeName) (cloudprovider.Zone, error) {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("GetZoneByNodeName(%v)", nodeName)
	instance, err := c.getInstanceByNodeName(nodeName)
	if err != nil {
		return cloudprovider.Zone{}, err
	}
	zone := cloudprovider.Zone{
		FailureDomain: instance.Placement.SubregionName,
		Region:        c.region,
	}

	return zone, nil

}

// Retrieves instance's vpc id from metadata
func (c *Cloud) findVPCID() (string, error) {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("findVPCID()")
	macs, err := c.metadata.GetMetadata("network/interfaces/macs/")
	if err != nil {
		return "", fmt.Errorf("could not list interfaces of the instance: %q", err)
	}

	// loop over interfaces, first vpc id returned wins
	for _, macPath := range strings.Split(macs, "\n") {
		if len(macPath) == 0 {
			continue
		}
		url := fmt.Sprintf("network/interfaces/macs/%svpc-id", macPath)
		netID, err := c.metadata.GetMetadata(url)
		if err != nil {
			continue
		}
		return netID, nil
	}
	return "", fmt.Errorf("could not find VPC ID in instance metadata")
}

// ********************* CCM Cloud Resource LBU Functions  *********************

func (c *Cloud) addLoadBalancerTags(loadBalancerName string, requested map[string]string) error {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("addLoadBalancerTags(%v,%v)", loadBalancerName, requested)
	var tags []osc.ResourceTag
	for k, v := range requested {
		tag := osc.ResourceTag{
			Key:   k,
			Value: v,
		}
		tags = append(tags, tag)
	}

	request := &osc.CreateLoadBalancerTagsOpts{
		CreateLoadBalancerTagsRequest: optional.NewInterface(
			osc.CreateLoadBalancerTagsRequest{
				LoadBalancerNames: []string{loadBalancerName},
				Tags: tags,
			}),
	}

	_, httpRes, err := c.lbu.CreateLoadBalancerTags(request)
	if err != nil {
		return fmt.Errorf("error adding tags to load balancer: %v %q", err, httpRes)
	}
	return nil
}

// Gets the current load balancer state
func (c *Cloud) describeLoadBalancer(name string) (osc.LoadBalancer, error) {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("describeLoadBalancer(%v)", name)

    request := &osc.ReadLoadBalancersOpts{
		ReadLoadBalancersRequest: optional.NewInterface(
			osc.ReadLoadBalancersRequest{
			    Filters: osc.FiltersLoadBalancer{
				    LoadBalancerNames: []string{name},
				},
			}),
	}

	response, httpRes, err := c.lbu.ReadLoadBalancers(request)
	if err != nil {
	    fmt.Errorf("http %q", httpRes)
		if awsError, ok := err.(awserr.Error); ok {
			if awsError.Code() == "LoadBalancerNotFound" {
				return osc.LoadBalancer{}, nil
			}
		}
		return osc.LoadBalancer{}, err
	}

// CHECK LoadBalancerDescription returned
// 	var ret *elb.LoadBalancerDescription
// 	for _, loadBalancer := range response.LoadBalancerDescriptions {c.findSecurityGroup
// 		if ret != nil {
// 			klog.Errorf("Found multiple load balancers with name: %s", name)
// 		}
// 		ret = loadBalancer
// 	}
	return response.LoadBalancers[0], nil
}

// Retrieves the specified security group from the OSC API, or returns nil if not found
func (c *Cloud) findSecurityGroup(securityGroupID string) (osc.SecurityGroup, error) {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("findSecurityGroup(%v)", securityGroupID)

	request := &osc.ReadSecurityGroupsOpts{
		ReadSecurityGroupsRequest: optional.NewInterface(
			osc.ReadSecurityGroupsRequest{
				Filters: osc.FiltersSecurityGroup{
					SecurityGroupIds: []string{securityGroupID},
				},
			}),
	}
	// We don't apply our tag filters because we are retrieving by ID

	groups, httpRes, err := c.fcu.ReadSecurityGroups(request)
	if err != nil {
		klog.Warningf("Error retrieving security group: %q %q", err, httpRes)
		return osc.SecurityGroup{}, err
	}

	if len(groups) == 0 {
		return osc.SecurityGroup{}, nil
	}
	if len(groups) != 1 {
		// This should not be possible - ids should be unique
		return osc.SecurityGroup{}, fmt.Errorf("multiple security groups found with same id %q", securityGroupID)
	}
	group := groups[0]
	return group, nil
}

// Makes sure the security group ingress is exactly the specified permissions
// Returns true if and only if changes were made
// The security group must already exist
func (c *Cloud) setSecurityGroupIngress(securityGroupID string, permissions SecurityGroupRuleSet) (bool, error) {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("setSecurityGroupIngress(%v,%v)", securityGroupID, permissions)
	// We do not want to make changes to the Global defined SG
	if securityGroupID == c.cfg.Global.LbuSecurityGroup {
		return false, nil
	}

	group, err := c.findSecurityGroup(securityGroupID)
	if err != nil {
		klog.Warningf("Error retrieving security group %q", err)
		return false, err
	}

	if group.SecurityGroupId == "" {
		return false, fmt.Errorf("security group not found: %s", securityGroupID)
	}

	klog.V(2).Infof("Existing security group ingress: %s %v", securityGroupID, group.InboundRules)

	actual := NewSecurityGroupRuleSet(group.InboundRules...)

	// OSC groups rules together, for example combining:
	//
	// { Port=80, Range=[A] } and { Port=80, Range=[B] }
	//
	// into { Port=80, Range=[A,B] }
	//
	// We have to ungroup them, because otherwise the logic becomes really
	// complicated, and also because if we have Range=[A,B] and we try to
	// add Range=[A] then OSC complains about a duplicate rule.
	permissions = permissions.Ungroup()
	actual = actual.Ungroup()

	remove := actual.Difference(permissions)
	add := permissions.Difference(actual)

	if add.Len() == 0 && remove.Len() == 0 {
		return false, nil
	}

	// TODO: There is a limit in VPC of 100 rules per security group, so we
	// probably should try grouping or combining to fit under this limit.
	// But this is only used on the LBU security group currently, so it
	// would require (ports * CIDRS) > 100.  Also, it isn't obvious exactly
	// how removing single permissions from compound rules works, and we
	// don't want to accidentally open more than intended while we're
	// applying changes.
	if add.Len() != 0 {
		klog.V(2).Infof("Adding security group ingress: %s %v", securityGroupID, add.List())

		request := &osc.CreateSecurityGroupRuleOpts{
            CreateSecurityGroupRuleRequest: optional.NewInterface(
                osc.CreateSecurityGroupRuleRequest{
                    SecurityGroupId: securityGroupID,
                    Rules: add.List(),
                }),
	}


		_, httpRes, errSGRule := c.fcu.CreateSecurityGroupRule(request)
		if err != nil {
			return false, fmt.Errorf("error authorizing security group ingress: %q %q", errSGRule, httpRes)
		}
	}
	if remove.Len() != 0 {
		klog.V(2).Infof("Remove security group ingress: %s %v", securityGroupID, remove.List())

		request := &osc.DeleteSecurityGroupRuleOpts{
            DeleteSecurityGroupRuleRequest: optional.NewInterface(
                osc.DeleteSecurityGroupRuleRequest{
                    SecurityGroupId: securityGroupID,
                    Rules: remove.List(),
                }),
        }

        _, httpRes, errDeleteSGRule := c.fcu.DeleteSecurityGroupRule(request)
		if err != nil {
			return false, fmt.Errorf("error revoking security group ingress: %q %q", errDeleteSGRule, httpRes)
		}
	}

	return true, nil
}

// Makes sure the security group includes the specified permissions
// Returns true if and only if changes were made
// The security group must already exist
func (c *Cloud) addSecurityGroupIngress(securityGroupID string, addRules []osc.SecurityGroupRule, isPublicCloud bool) (bool, error) {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("addSecurityGroupIngress(%v,%v,%v)", securityGroupID, addRules, isPublicCloud)
	// We do not want to make changes to the Global defined SG
	if securityGroupID == c.cfg.Global.LbuSecurityGroup {
		return false, nil
	}

	group, err := c.findSecurityGroup(securityGroupID)
	if err != nil {
		klog.Warningf("Error retrieving security group: %q", err)
		return false, err
	}

	if group.SecurityGroupId == "" {
		return false, fmt.Errorf("security group not found: %s", securityGroupID)
	}

	klog.Infof("Existing security group ingress: %s %v", securityGroupID, group.InboundRules)

	changes := []osc.SecurityGroupRule{}
	for _, addRule := range addRules {
		hasUserID := false
		for i := range addRule.SecurityGroupsMembers {
			if addRule.SecurityGroupsMembers[i].SecurityGroupId != "" {
				hasUserID = true
			}
		}

		found := false
		for _, groupRule := range group.InboundRules {
			if securityGroupRuleExists(addRule, groupRule, hasUserID) {
				found = true
				break
			}
		}

		if !found {
			changes = append(changes, addRule)
		}
	}

	if len(changes) == 0 && !isPublicCloud {
		return false, nil
	}

	klog.Infof("Adding security group ingress: %s %v isPublic %v)", securityGroupID, changes, isPublicCloud)

	request := &osc.CreateSecurityGroupRuleOpts{
	    CreateSecurityGroupRuleRequest: optional.NewInterface(
			osc.CreateSecurityGroupRuleRequest{
                Rules: changes,
                SecurityGroupNameToLink: DefaultSrcSgName,
                SecurityGroupAccountIdToLink: DefaultSgOwnerID,
			}),
	}
	_, httpRes, errCreateSGRule := c.fcu.CreateSecurityGroupRule(request)

	if err != nil {
		ignore := false
		if isPublicCloud {
			if awsError, ok := err.(awserr.Error); ok {
				if awsError.Code() == "InvalidPermission.Duplicate" {
					klog.V(2).Infof("Ignoring InvalidPermission.Duplicate for security group (%s), assuming is used by other public LB", securityGroupID)
					ignore = true
				}
			}
		}
		if !ignore {
			klog.Warningf("Error authorizing security group ingress %q %q", errCreateSGRule, httpRes)
			return false, fmt.Errorf("error authorizing security group ingress: %q %q", errCreateSGRule, httpRes)
		}
	}

	return true, nil
}

// Makes sure the security group no longer includes the specified permissions
// Returns true if and only if changes were made
// If the security group no longer exists, will return (false, nil)
func (c *Cloud) removeSecurityGroupIngress(securityGroupID string, removeRules []osc.SecurityGroupRule, isPublicCloud bool) (bool, error) {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("removeSecurityGroupIngress(%v,%v)", securityGroupID, removeRules)
	// We do not want to make changes to the Global defined SG
	if securityGroupID == c.cfg.Global.LbuSecurityGroup {
		return false, nil
	}

	group, err := c.findSecurityGroup(securityGroupID)
	if err != nil {
		klog.Warningf("Error retrieving security group: %q", err)
		return false, err
	}

	if group.SecurityGroupId == "" {
		klog.Warning("Security group not found: ", securityGroupID)
		return false, nil
	}

	changes := []osc.SecurityGroupRule{}
	for _, removeRule := range removeRules {
		hasUserID := false
		for i := range removeRule.SecurityGroupsMembers {
			if removeRule.SecurityGroupsMembers[i].SecurityGroupId != "" {
				hasUserID = true
			}
		}

		var found osc.SecurityGroupRule
		for _, groupRule := range group.InboundRules {
			if securityGroupRuleExists(removeRule, groupRule, hasUserID) {
				found = removeRule
				break
			}
		}

		if !reflect.DeepEqual(found, osc.SecurityGroupRule{}) {
			changes = append(changes, found)
		}
	}

	if len(changes) == 0 && !isPublicCloud {
		return false, nil
	}

	klog.Infof("Removing security group ingress: %s %v", securityGroupID, changes)

	request := &osc.DeleteSecurityGroupRuleOpts{
		DeleteSecurityGroupRuleRequest: optional.NewInterface(
			osc.DeleteSecurityGroupRuleRequest{
	            Rules: changes,
                SecurityGroupId: securityGroupID,
                SecurityGroupNameToUnlink: DefaultSrcSgName,
                SecurityGroupAccountIdToUnlink: DefaultSgOwnerID,
			}),
	}

	_, httpRes, errDeleteSGRule := c.fcu.DeleteSecurityGroupRule(request)


	if err != nil {
		klog.Warningf("Error revoking security group ingress: %q %q", errDeleteSGRule, httpRes)
		return false, err
	}

	return true, nil
}

// Makes sure the security group exists.
// For multi-cluster isolation, name must be globally unique, for example derived from the service UUID.
// Additional tags can be specified
// Returns the security group id or error
func (c *Cloud) ensureSecurityGroup(name string, description string, additionalTags map[string]string) (string, error) {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("ensureSecurityGroup (%v,%v,%v)", name, description, additionalTags)

	groupID := ""
	attempt := 0
	for {
		attempt++

		// Note that we do _not_ add our tag filters; group-name + vpc-id is the OSC primary key.
		// However, we do check that it matches our tags.
		// If it doesn't have any tags, we tag it; this is how we recover if we failed to tag before.
		// If it has a different cluster's tags, that is an error.
		// This shouldn't happen because name is expected to be globally unique (UUID derived)

        request := osc.ReadSecurityGroupsRequest{}

		if c.netID != "" {
		    request.Filters = osc.FiltersSecurityGroup{SecurityGroupIds: []string{c.netID}}
		}


        requestOpts := &osc.ReadSecurityGroupsOpts{ReadSecurityGroupsRequest: optional.NewInterface(request)}
		securityGroups, httpRes, err := c.fcu.ReadSecurityGroups(requestOpts)
		if err != nil {
		    fmt.Errorf("http %q", httpRes)
			return "", err
		}

		if len(securityGroups) >= 1 {
			if len(securityGroups) > 1 {
				klog.Warningf("Found multiple security groups with name: %q", name)
			}
			err := c.tagging.readRepairClusterTags(
				c.fcu, securityGroups[0].SecurityGroupId,
				ResourceLifecycleOwned, nil, securityGroups[0].Tags)
			if err != nil {
				return "", err
			}

			return securityGroups[0].SecurityGroupId, nil
		}

		createRequest := &osc.CreateSecurityGroupOpts{
            CreateSecurityGroupRequest: optional.NewInterface(
                osc.CreateSecurityGroupRequest{
                    NetId: c.netID,
                    SecurityGroupName: name,
                    Description:       description,
                }),
	    }

		createResponse, httpRes, err := c.fcu.CreateSecurityGroup(createRequest)
		if err != nil {
			ignore := false
			switch err := err.(type) {
			case awserr.Error:
				if err.Code() == "InvalidGroup.Duplicate" && attempt < MaxReadThenCreateRetries {
					klog.V(2).Infof("Got InvalidGroup.Duplicate while creating security group (race?); will retry")
					ignore = true
				}
			}
			if !ignore {
				klog.Errorf("Error creating security group: %q %q", err, httpRes)
				return "", err
			}
			time.Sleep(1 * time.Second)
		} else {
			groupID = createResponse.SecurityGroup.SecurityGroupId
			break
		}
	}
	if groupID == "" {
		return "", fmt.Errorf("created security group, but id was not returned: %s", name)
	}

	err := c.tagging.createTags(c.fcu, groupID, ResourceLifecycleOwned, additionalTags)
	if err != nil {
		// If we retry, ensureClusterTags will recover from this - it
		// will add the missing tags.  We could delete the security
		// group here, but that doesn't feel like the right thing, as
		// the caller is likely to retry the create
		return "", fmt.Errorf("error tagging security group: %q", err)
	}
	return groupID, nil
}

// Finds the subnets associated with the cluster, by matching tags.
// For maximal backwards compatibility, if no subnets are tagged, it will fall-back to the current subnet.
// However, in future this will likely be treated as an error.
func (c *Cloud) findSubnets() ([]osc.Subnet, error) {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("findSubnets()")
	request := &osc.ReadSubnetsOpts{}
	var err error
	var subnets []osc.Subnet
	err = nil
	subnets = []osc.Subnet{}



	if c.netID != "" {
		//request.Filters = []*ec2.Filter{newEc2Filter("vpc-id", c.vpcID)}
        request = &osc.ReadSubnetsOpts{
            ReadSubnetsRequest: optional.NewInterface(
                osc.ReadSubnetsRequest{
                    Filters: osc.FiltersSubnet{
                        NetIds: []string{c.netID},
                },
            }),
        }

		subnets, httpRes, err := c.fcu.ReadSubnets(request)
		if err != nil {
			return []osc.Subnet{}, fmt.Errorf("error describing subnets: %q %q", err, httpRes)
		}

		var matches []osc.Subnet
		for _, subnet := range subnets {
			if c.tagging.hasClusterTag(subnet.Tags) {
				matches = append(matches, subnet)
			}
		}

		if len(matches) != 0 {
			return matches, nil
		}
	}

	if c.selfOSCInstance.subnetID != "" {
		// Fall back to the current instance subnets, if nothing is tagged
		klog.Warningf("No tagged subnets found; will fall-back to the current subnet only.  This is likely to be an error in a future version of k8s.")

		request := &osc.ReadSubnetsOpts{
		    ReadSubnetsRequest: optional.NewInterface(
			    osc.ReadSubnetsRequest{
				    Filters: osc.FiltersSubnet{
					    SubnetIds: []string{c.selfOSCInstance.subnetID},
				    },
			    }),
	}

		subnets, httpRes, errRead := c.fcu.ReadSubnets(request)
		if err != nil {
			return nil, fmt.Errorf("error describing subnets: %q %q", errRead, httpRes)
		}
		return subnets, nil

	}

	return subnets, nil

}

// Finds the subnets to use for an LBU we are creating.
// Normal (Internet-facing) LBUs must use public subnets, so we skip private subnets.
// Internal LBUs can use public or private subnets, but if we have a private subnet we should prefer that.
func (c *Cloud) findLBUSubnets(internalLBU bool) ([]string, error) {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("findLBUSubnets(%v)", internalLBU)

	subnets, err := c.findSubnets()
	if err != nil {
		return nil, err
	}
	var rt []osc.RouteTable
	var httpRes *_nethttp.Response
	if c.netID != "" {
// 		vpcIDFilter := newEc2Filter("vpc-id", c.vpcID)
// 		rRequest := &ec2.DescribeRouteTablesInput{}
// 		rRequest.Filters = []*ec2.Filter{vpcIDFilter}
// 		rt, err = c.ec2.DescribeRouteTables(rRequest)

        rRequest := &osc.ReadRouteTablesOpts{
            ReadRouteTablesRequest: optional.NewInterface(
                osc.ReadRouteTablesRequest{
                    Filters: osc.FiltersRouteTable{
                        NetIds: []string{c.netID},
                    },
                }),
	    }

	    rt, httpRes, err = c.fcu.ReadRouteTables(rRequest)

		if err != nil {
			return nil, fmt.Errorf("error describe route table: %q %q", err, httpRes)
		}
	}

	// Try to break the tie using a tag
	var tagName string
	if internalLBU {
		tagName = TagNameSubnetInternalLBU
	} else {
		tagName = TagNameSubnetPublicLBU
	}

	subnetsByAZ := make(map[string]osc.Subnet)
	for _, subnet := range subnets {
		az := subnet.SubregionName
		id := subnet.SubnetId
		if az == "" || id == "" {
			klog.Warningf("Ignoring subnet with empty az/id: %v", subnet)
			continue
		}

		isPublic, err := isSubnetPublic(rt, id)
		if err != nil {
			return nil, err
		}
		if !internalLBU && !isPublic {
			klog.V(2).Infof("Ignoring private subnet for public LBU %q", id)
			continue
		}

		existing := subnetsByAZ[az]
		_, subnetHasTag := findTag(subnet.Tags, tagName)
		if reflect.DeepEqual(existing, osc.Subnet{}) {
			if subnetHasTag {
				subnetsByAZ[az] = subnet
			} else if isPublic && !internalLBU {
				subnetsByAZ[az] = subnet
			}
			continue
		}

		_, existingHasTag := findTag(existing.Tags, tagName)

		if existingHasTag != subnetHasTag {
			if subnetHasTag {
				subnetsByAZ[az] = subnet
			}
			continue
		}

		// If we have two subnets for the same AZ we arbitrarily choose the one that is first lexicographically.
		// TODO: Should this be an error.
		if strings.Compare(existing.SubnetId, subnet.SubnetId) > 0 {
			klog.Warningf("Found multiple subnets in AZ %q; choosing %q between subnets %q and %q", az, subnet.SubnetId, existing.SubnetId, subnet.SubnetId)
			subnetsByAZ[az] = subnet
			continue
		}

		klog.Warningf("Found multiple subnets in AZ %q; choosing %q between subnets %q and %q", az, existing.SubnetId, existing.SubnetId, subnet.SubnetId)
		continue
	}

	var azNames []string
	for key := range subnetsByAZ {
		azNames = append(azNames, key)
	}

	sort.Strings(azNames)

	var subnetIDs []string
	for _, key := range azNames {
		subnetIDs = append(subnetIDs, subnetsByAZ[key].SubnetId)
	}

	return subnetIDs, nil
}

// buildLBUSecurityGroupList returns list of SecurityGroups which should be
// attached to LBU created by a service. List always consist of at least
// 1 member which is an SG created for this service or a SG from the Global config.
// Extra groups can be specified via annotation, as can extra tags for any
// new groups. The annotation "ServiceAnnotationLoadBalancerSecurityGroups" allows for
// setting the security groups specified.
func (c *Cloud) buildLBUSecurityGroupList(serviceName types.NamespacedName, loadBalancerName string, annotations map[string]string) ([]string, error) {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("buildLBUSecurityGroupList(%v,%v,%v)", serviceName, loadBalancerName, annotations)
	var err error
	var securityGroupID string

	if c.cfg.Global.LbuSecurityGroup != "" {
		securityGroupID = c.cfg.Global.LbuSecurityGroup
	} else {
		// Create a security group for the load balancer
		sgName := "k8s-elb-" + loadBalancerName
		sgDescription := fmt.Sprintf("Security group for Kubernetes LBU %s (%v)", loadBalancerName, serviceName)
		securityGroupID, err = c.ensureSecurityGroup(sgName, sgDescription, getLoadBalancerAdditionalTags(annotations))
		if err != nil {
			klog.Errorf("Error creating load balancer security group: %q", err)
			return nil, err
		}
	}

	sgList := []string{}

	for _, extraSG := range strings.Split(annotations[ServiceAnnotationLoadBalancerSecurityGroups], ",") {
		extraSG = strings.TrimSpace(extraSG)
		if len(extraSG) > 0 {
			sgList = append(sgList, extraSG)
		}
	}

	// If no Security Groups have been specified with the ServiceAnnotationLoadBalancerSecurityGroups annotation, we add the default one.
	if len(sgList) == 0 {
		sgList = append(sgList, securityGroupID)
	}

	for _, extraSG := range strings.Split(annotations[ServiceAnnotationLoadBalancerExtraSecurityGroups], ",") {
		extraSG = strings.TrimSpace(extraSG)
		if len(extraSG) > 0 {
			sgList = append(sgList, extraSG)
		}
	}

	return sgList, nil
}

// EnsureLoadBalancer implements LoadBalancer.EnsureLoadBalancer
func (c *Cloud) EnsureLoadBalancer(ctx context.Context, clusterName string, apiService *v1.Service,
	nodes []*v1.Node) (*v1.LoadBalancerStatus, error) {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("EnsureLoadBalancer(%v, %v, %v)", clusterName, apiService, nodes)
	klog.V(10).Infof("EnsureLoadBalancer.annotations(%v)", apiService.Annotations)
	annotations := apiService.Annotations
	if apiService.Spec.SessionAffinity != v1.ServiceAffinityNone {
		// LBU supports sticky sessions, but only when configured for HTTP/HTTPS
		return nil, fmt.Errorf("unsupported load balancer affinity: %v", apiService.Spec.SessionAffinity)
	}

	if len(apiService.Spec.Ports) == 0 {
		return nil, fmt.Errorf("requested load balancer with no ports")
	}

	// Figure out what mappings we want on the load balancer
	listeners := []osc.ListenerForCreation{}

	sslPorts := getPortSets(annotations[ServiceAnnotationLoadBalancerSSLPorts])

	for _, port := range apiService.Spec.Ports {
		if port.Protocol != v1.ProtocolTCP {
			return nil, fmt.Errorf("Only TCP LoadBalancer is supported for OSC LBU")
		}
		if port.NodePort == 0 {
			klog.Errorf("Ignoring port without NodePort defined: %v", port)
			continue
		}

		listener, err := buildListener(port, annotations, sslPorts)
		if err != nil {
			return nil, err
		}
		listeners = append(listeners, listener)
	}

	if apiService.Spec.LoadBalancerIP != "" {
		return nil, fmt.Errorf("LoadBalancerIP cannot be specified for OSC LBU")
	}

	instances, err := c.findInstancesForLBU(nodes)
	klog.V(10).Infof("Debug OSC: c.findInstancesForLBU(nodes) : %v", instances)
	if err != nil {
		return nil, err
	}

	sourceRanges, err := servicehelpers.GetLoadBalancerSourceRanges(apiService)
	klog.V(10).Infof("Debug OSC:  servicehelpers.GetLoadBalancerSourceRanges : %v", sourceRanges)
	if err != nil {
		return nil, err
	}

	// Determine if this is tagged as an Internal LBU
	internalLBU := false
	internalAnnotation := apiService.Annotations[ServiceAnnotationLoadBalancerInternal]
	if internalAnnotation == "false" {
		internalLBU = false
	} else if internalAnnotation != "" {
		internalLBU = true
	}
	klog.V(10).Infof("Debug OSC:  internalLBU : %v", internalLBU)

	// Determine if we need to set the Proxy protocol policy
	proxyProtocol := false
	proxyProtocolAnnotation := apiService.Annotations[ServiceAnnotationLoadBalancerProxyProtocol]
	if proxyProtocolAnnotation != "" {
		if proxyProtocolAnnotation != "*" {
			return nil, fmt.Errorf("annotation %q=%q detected, but the only value supported currently is '*'", ServiceAnnotationLoadBalancerProxyProtocol, proxyProtocolAnnotation)
		}
		proxyProtocol = true
	}

	// Some load balancer attributes are required, so defaults are set. These can be overridden by annotations.
	loadBalancerAttributes := osc.LoadBalancer{
	// A verifier
// 		ConnectionDraining: &elb.ConnectionDraining{Enabled: aws.Bool(false)},
// 		ConnectionSettings: &elb.ConnectionSettings{IdleTimeout: aws.Int64(60)},
	}

	if annotations[ServiceAnnotationLoadBalancerAccessLogOsuBucketName] != "" &&
		annotations[ServiceAnnotationLoadBalancerAccessLogOsuBucketPrefix] != "" {

		loadBalancerAttributes.AccessLog = osc.AccessLog{IsEnabled: false}

		// Determine if access log enabled/disabled has been specified
		accessLogEnabledAnnotation := annotations[ServiceAnnotationLoadBalancerAccessLogEnabled]
		if accessLogEnabledAnnotation != "" {
			accessLogEnabled, err := strconv.ParseBool(accessLogEnabledAnnotation)
			if err != nil {
				return nil, fmt.Errorf("error parsing service annotation: %s=%s",
					ServiceAnnotationLoadBalancerAccessLogEnabled,
					accessLogEnabledAnnotation,
				)
			}
			loadBalancerAttributes.AccessLog.IsEnabled = accessLogEnabled
		}
		// Determine if an access log emit interval has been specified
		accessLogEmitIntervalAnnotation := annotations[ServiceAnnotationLoadBalancerAccessLogEmitInterval]
		if accessLogEmitIntervalAnnotation != "" {
			accessLogEmitInterval, err := strconv.ParseInt(accessLogEmitIntervalAnnotation, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("error parsing service annotation: %s=%s",
					ServiceAnnotationLoadBalancerAccessLogEmitInterval,
					accessLogEmitIntervalAnnotation,
				)
			}
			loadBalancerAttributes.AccessLog.PublicationInterval = int32(accessLogEmitInterval)
		}

		// Determine if access log Osu bucket name has been specified
		accessLogOsuBucketNameAnnotation := annotations[ServiceAnnotationLoadBalancerAccessLogOsuBucketName]
		if accessLogOsuBucketNameAnnotation != "" {
			loadBalancerAttributes.AccessLog.OsuBucketName = accessLogOsuBucketNameAnnotation
		}

		// Determine if access log Osu bucket prefix has been specified
		accessLogOsuBucketPrefixAnnotation := annotations[ServiceAnnotationLoadBalancerAccessLogOsuBucketPrefix]
		if accessLogOsuBucketPrefixAnnotation != "" {
			loadBalancerAttributes.AccessLog.OsuBucketPrefix = accessLogOsuBucketPrefixAnnotation
		}
		klog.V(10).Infof("Debug OSC:  loadBalancerAttributes.AccessLog : %v", loadBalancerAttributes.AccessLog)
	}

// A verifier

	// Determine if connection draining enabled/disabled has been specified
// 	connectionDrainingEnabledAnnotation := annotations[ServiceAnnotationLoadBalancerConnectionDrainingEnabled]
// 	if connectionDrainingEnabledAnnotation != "" {
// 		connectionDrainingEnabled, err := strconv.ParseBool(connectionDrainingEnabledAnnotation)
// 		if err != nil {
// 			return nil, fmt.Errorf("error parsing service annotation: %s=%s",
// 				ServiceAnnotationLoadBalancerConnectionDrainingEnabled,
// 				connectionDrainingEnabledAnnotation,
// 			)
// 		}
// 		loadBalancerAttributes.ConnectionDraining.Enabled = connectionDrainingEnabled
// 	}

	// Determine if connection draining timeout has been specified
// 	connectionDrainingTimeoutAnnotation := annotations[ServiceAnnotationLoadBalancerConnectionDrainingTimeout]
// 	if connectionDrainingTimeoutAnnotation != "" {
// 		connectionDrainingTimeout, err := strconv.ParseInt(connectionDrainingTimeoutAnnotation, 10, 64)
// 		if err != nil {
// 			return nil, fmt.Errorf("error parsing service annotation: %s=%s",
// 				ServiceAnnotationLoadBalancerConnectionDrainingTimeout,
// 				connectionDrainingTimeoutAnnotation,
// 			)
// 		}
// 		loadBalancerAttributes.ConnectionDraining.Timeout = &connectionDrainingTimeout
// 	}

	// Determine if connection idle timeout has been specified
// 	connectionIdleTimeoutAnnotation := annotations[ServiceAnnotationLoadBalancerConnectionIdleTimeout]
// 	if connectionIdleTimeoutAnnotation != "" {
// 		connectionIdleTimeout, err := strconv.ParseInt(connectionIdleTimeoutAnnotation, 10, 64)
// 		if err != nil {
// 			return nil, fmt.Errorf("error parsing service annotation: %s=%s",
// 				ServiceAnnotationLoadBalancerConnectionIdleTimeout,
// 				connectionIdleTimeoutAnnotation,
// 			)
// 		}
// 		loadBalancerAttributes.ConnectionSettings.IdleTimeout = &connectionIdleTimeout
// 	}

	// Find the subnets that the LBU will live in
	subnetIDs, err := c.findLBUSubnets(internalLBU)
	klog.V(2).Infof("Debug OSC:  c.findLBUSubnets(internalLBU) : %v", subnetIDs)

	if err != nil {
		klog.Errorf("Error listing subnets in VPC: %q", err)
		return nil, err
	}

	// Bail out early if there are no subnets
	if len(subnetIDs) == 0 {
		klog.Warningf("could not find any suitable subnets for creating the LBU")
	}

	loadBalancerName := c.GetLoadBalancerName(ctx, clusterName, apiService)
	serviceName := types.NamespacedName{Namespace: apiService.Namespace, Name: apiService.Name}

	klog.V(10).Infof("Debug OSC:  loadBalancerName : %v", loadBalancerName)
	klog.V(10).Infof("Debug OSC:  serviceName : %v", serviceName)
	klog.V(10).Infof("Debug OSC:  serviceName : %v", annotations)

	var securityGroupIDs []string

	if len(subnetIDs) == 0 || c.netID == "" {
		securityGroupIDs = []string{DefaultSrcSgName}
	} else {
		securityGroupIDs, err = c.buildLBUSecurityGroupList(serviceName, loadBalancerName, annotations)
	}

	klog.V(10).Infof("Debug OSC:  ensured securityGroupIDs : %v", securityGroupIDs)

	if err != nil {
		return nil, err
	}
	if len(securityGroupIDs) == 0 {
		return nil, fmt.Errorf("[BUG] LBU can't have empty list of Security Groups to be assigned, this is a Kubernetes bug, please report")
	}

	if len(subnetIDs) > 0 && c.netID != "" {
		oscSourceRanges := []string{}
		for _, sourceRange := range sourceRanges.StringSlice() {
			oscSourceRanges = append(oscSourceRanges, sourceRange)
		}

		permissions := NewSecurityGroupRuleSet()
		for _, port := range apiService.Spec.Ports {

			protocol := strings.ToLower(string(port.Protocol))

			permission := osc.SecurityGroupRule{}
			permission.FromPortRange = int32(port.Port)
			permission.ToPortRange = int32(port.Port)
			permission.IpRanges = oscSourceRanges
			permission.IpProtocol = protocol

			permissions.Insert(permission)
		}

		// Allow ICMP fragmentation packets, important for MTU discovery
		{
			permission := osc.SecurityGroupRule{
				IpProtocol: "icmp",
				FromPortRange:   3,
				ToPortRange:     4,
				IpRanges:   oscSourceRanges,
			}

			permissions.Insert(permission)
		}
		_, err = c.setSecurityGroupIngress(securityGroupIDs[0], permissions)
		if err != nil {
			return nil, err
		}
	}

	// Build the load balancer itself
	loadBalancer, err := c.ensureLoadBalancer(
	    serviceName,
		loadBalancerName,
		listeners,
		subnetIDs,
		securityGroupIDs,
		internalLBU,
		proxyProtocol,
		loadBalancerAttributes,
		annotations,
	)
	if err != nil {
		return nil, err
	}

	if sslPolicyName, ok := annotations[ServiceAnnotationLoadBalancerSSLNegotiationPolicy]; ok {
		err := c.ensureSSLNegotiationPolicy(loadBalancer, sslPolicyName)
		if err != nil {
			return nil, err
		}

		for _, port := range c.getLoadBalancerTLSPorts(loadBalancer) {
			err := c.setSSLNegotiationPolicy(loadBalancerName, sslPolicyName, port)
			if err != nil {
				return nil, err
			}
		}
	}

	if path, healthCheckNodePort := servicehelpers.GetServiceHealthCheckPathPort(apiService); path != "" {
		klog.V(4).Infof("service %v (%v) needs health checks on :%d%s)", apiService.Name, loadBalancerName, healthCheckNodePort, path)
		err = c.ensureLoadBalancerHealthCheck(loadBalancer, "HTTP", healthCheckNodePort, path, annotations)
		if err != nil {
			return nil, fmt.Errorf("Failed to ensure health check for localized service %v on node port %v: %q", loadBalancerName, healthCheckNodePort, err)
		}
	} else {
		klog.V(4).Infof("service %v does not need custom health checks", apiService.Name)
		// We only configure a TCP health-check on the first port
		var tcpHealthCheckPort int32
		for _, listener := range listeners {
			if listener.BackendPort == 0 {
				continue
			}
			tcpHealthCheckPort = int32(listener.BackendPort)
			break
		}
		annotationProtocol := strings.ToLower(annotations[ServiceAnnotationLoadBalancerBEProtocol])
		var hcProtocol string
		if annotationProtocol == "https" || annotationProtocol == "ssl" {
			hcProtocol = "SSL"
		} else {
			hcProtocol = "TCP"
		}
		// there must be no path on TCP health check
		err = c.ensureLoadBalancerHealthCheck(loadBalancer, hcProtocol, tcpHealthCheckPort, "", annotations)
		if err != nil {
			return nil, err
		}
	}

	err = c.updateInstanceSecurityGroupsForLoadBalancer(loadBalancer, instances, securityGroupIDs)
	if err != nil {
		klog.Warningf("Error opening ingress rules for the load balancer to the instances: %q", err)
		return nil, err
	}

	err = c.ensureLoadBalancerInstances(loadBalancer.LoadBalancerName, loadBalancer.BackendVmIds, instances)
	if err != nil {
		klog.Warningf("Error registering instances with the load balancer: %q", err)
		return nil, err
	}

	klog.V(1).Infof("Loadbalancer %s (%v) has DNS name %s", loadBalancerName, serviceName, loadBalancer.DnsName)

	// TODO: Wait for creation?

	status := toStatus(loadBalancer)
	return status, nil
}

// GetLoadBalancer is an implementation of LoadBalancer.GetLoadBalancer
func (c *Cloud) GetLoadBalancer(ctx context.Context, clusterName string, service *v1.Service) (*v1.LoadBalancerStatus, bool, error) {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("GetLoadBalancer(%v,%v)", clusterName, service)
	loadBalancerName := c.GetLoadBalancerName(ctx, clusterName, service)

	lb, err := c.describeLoadBalancer(loadBalancerName)
	if err != nil {
		return nil, false, err
	}

	if reflect.DeepEqual(lb, osc.LoadBalancer{}) {
		return nil, false, nil
	}

	status := toStatus(lb)
	return status, true, nil
}

// GetLoadBalancerName is an implementation of LoadBalancer.GetLoadBalancerName
func (c *Cloud) GetLoadBalancerName(ctx context.Context, clusterName string, service *v1.Service) string {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("GetLoadBalancerName(%v,%v)", clusterName, service)
	// TODO: replace DefaultLoadBalancerName to generate more meaningful loadbalancer names.
	return cloudprovider.DefaultLoadBalancerName(service)
}

// Return all the security groups that are tagged as being part of our cluster
func (c *Cloud) getTaggedSecurityGroups() (map[string]osc.SecurityGroup, error) {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("getTaggedSecurityGroups()")
	request := &osc.ReadSecurityGroupsOpts{
	    ReadSecurityGroupsRequest: optional.NewInterface(
	        osc.ReadSecurityGroupsRequest{
                Filters: osc.FiltersSecurityGroup{
                    Tags: []string{["c.tagging.clusterTagKey()=[ResourceLifecycleOwned, ResourceLifecycleShared]"], ["TagNameMainSG+c.tagging.clusterID()=\"True\""],},
                    // A verifier
//                 newTagFilter(c.tagging.clusterTagKey(), []string{ResourceLifecycleOwned, ResourceLifecycleShared}...),
//                 newtagFilter(TagNameMainSG+c.tagging.clusterID(), "True"),
             },
	    }),
	}
	groups, httpRes, err := c.fcu.ReadSecurityGroups(request)
	if err != nil {
		return nil, fmt.Errorf("error querying security groups: %q %q", err, httpRes)
	}

	m := make(map[string]osc.SecurityGroup)
	for _, group := range groups {
		if !c.tagging.hasClusterTag(group.Tags) {
			continue
		}

		id := group.SecurityGroupId
		if id == "" {
			klog.Warningf("Ignoring group without id: %v", group)
			continue
		}
		m[id] = group
	}
	return m, nil
}

// Open security group ingress rules on the instances so that the load balancer can talk to them
// Will also remove any security groups ingress rules for the load balancer that are _not_ needed for allInstances
func (c *Cloud) updateInstanceSecurityGroupsForLoadBalancer(lb osc.LoadBalancer,
	instances map[InstanceID]osc.Vm,
	securityGroupIDs []string) error {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("updateInstanceSecurityGroupsForLoadBalancer(%v, %v, %v)", lb, instances, securityGroupIDs)

	if c.cfg.Global.DisableSecurityGroupIngress {
		return nil
	}

	// Determine the load balancer security group id
	loadBalancerSecurityGroupID := ""
	securityGroupsItem := []string{}
	if len(lb.SecurityGroups) > 0 {
		for _, securityGroup := range lb.SecurityGroups {
			securityGroupsItem = append(securityGroupsItem, securityGroup)
		}
	} else if len(securityGroupIDs) > 0 {
		securityGroupsItem = securityGroupIDs
	}

	for _, securityGroup := range securityGroupsItem {
		if securityGroup == ""{
			continue
		}
		if loadBalancerSecurityGroupID != "" {
			// We create LBs with one SG
			klog.Warningf("Multiple security groups for load balancer: %q", lb.LoadBalancerName)
		}
		loadBalancerSecurityGroupID = securityGroup
	}

	if loadBalancerSecurityGroupID == "" {
		return fmt.Errorf("could not determine security group for load balancer: %s", lb.LoadBalancerName)
	}

	klog.V(10).Infof("loadBalancerSecurityGroupID(%v)", loadBalancerSecurityGroupID)

	// Get the actual list of groups that allow ingress from the load-balancer
	var actualGroups []osc.SecurityGroup
	{

		reqFilters := osc.FiltersSecurityGroup{}
		// A verifier
		if loadBalancerSecurityGroupID != DefaultSrcSgName {
		    reqFilters.SecurityGroupIds= []string{loadBalancerSecurityGroupID}

		} else {
			reqFilters.SecurityGroupNames= []string{loadBalancerSecurityGroupID}
		}
		describeRequest := &osc.ReadSecurityGroupsOpts{
		    ReadSecurityGroupsRequest: optional.NewInterface(
                osc.ReadSecurityGroupsRequest{
                    Filters: reqFilters,
                }),
		}
		response, httpRes, err := c.fcu.ReadSecurityGroups(describeRequest)
		if err != nil {
			return fmt.Errorf("error querying security groups for LBU: %q %q", err, httpRes)
		}
		for _, sg := range response {
			if !c.tagging.hasClusterTag(sg.Tags) {
				continue
			}
			actualGroups = append(actualGroups, sg)
		}
	}

	klog.V(10).Infof("actualGroups(%v)", actualGroups)

	taggedSecurityGroups, err := c.getTaggedSecurityGroups()
	if err != nil {
		return fmt.Errorf("error querying for tagged security groups: %q", err)
	}
	klog.V(10).Infof("taggedSecurityGroups(%v)", taggedSecurityGroups)

	// Open the firewall from the load balancer to the instance
	// We don't actually have a trivial way to know in advance which security group the instance is in
	// (it is probably the node security group, but we don't easily have that).
	// However, we _do_ have the list of security groups on the instance records.

	// Map containing the changes we want to make; true to add, false to remove
	instanceSecurityGroupIds := map[string]bool{}

	// Scan instances for groups we want open
	for _, instance := range instances {
		securityGroup, err := findSecurityGroupForInstance(instance, taggedSecurityGroups)
		if err != nil {
			return err
		}

		if securityGroup == (osc.SecurityGroupLight{}) {
			klog.Warning("Ignoring instance without security group: ", instance.VmId)
			continue
		}
		id := securityGroup.SecurityGroupId
		if id == "" {
			klog.Warningf("found security group without id: %v", securityGroup)
			continue
		}

		instanceSecurityGroupIds[id] = true
	}

	klog.V(10).Infof("instanceSecurityGroupIds(%v)", instanceSecurityGroupIds)

	// Compare to actual groups
	for _, actualGroup := range actualGroups {
		actualGroupID := actualGroup.SecurityGroupId
		if actualGroupID == "" {
			klog.Warning("Ignoring group without ID: ", actualGroup)
			continue
		}

		adding, found := instanceSecurityGroupIds[actualGroupID]
		if found && adding {
			// We don't need to make a change; the permission is already in place
			delete(instanceSecurityGroupIds, actualGroupID)
		} else {
			// This group is not needed by allInstances; delete it
			instanceSecurityGroupIds[actualGroupID] = false
		}
	}

	klog.V(10).Infof("instanceSecurityGroupIds(%v)", instanceSecurityGroupIds)
	for instanceSecurityGroupID, add := range instanceSecurityGroupIds {
		if add {
			klog.V(2).Infof("Adding rule for traffic from the load balancer (%s) to instances (%s)", loadBalancerSecurityGroupID, instanceSecurityGroupID)
		} else {
			klog.V(2).Infof("Removing rule for traffic from the load balancer (%s) to instance (%s)", loadBalancerSecurityGroupID, instanceSecurityGroupID)
		}
		isPublicCloud := (loadBalancerSecurityGroupID == DefaultSrcSgName)
		permissions := []osc.SecurityGroupRule{}
		if !isPublicCloud {
			// This setting is applied when we are in a vpc
			sourceGroupID := osc.SecurityGroup{}
			sourceGroupID.SecurityGroupId = loadBalancerSecurityGroupID

			allProtocols := "-1"

			permission := osc.SecurityGroupRule{}
			permission.IpProtocol = allProtocols
			// A verifier
			//permission.SecurityGroupsMembers = []osc.SecurityGroupsMember{sourceGroupID.SecurityGroupId}
			permissions = []osc.SecurityGroupRule{permission}
		}

		if add {
			changed, err := c.addSecurityGroupIngress(instanceSecurityGroupID, permissions, isPublicCloud)
			if err != nil {
				return err
			}
			if !changed {
				klog.Warning("Allowing ingress was not needed; concurrent change? SecurityGroupId=", instanceSecurityGroupID)
			}
		} else {
			changed, err := c.removeSecurityGroupIngress(instanceSecurityGroupID, permissions, isPublicCloud)
			if err != nil {
				return err
			}
			if !changed {
				klog.Warning("Revoking ingress was not needed; concurrent change? SecurityGroupId=", instanceSecurityGroupID)
			}
		}
	}

	return nil
}

// EnsureLoadBalancerDeleted implements LoadBalancer.EnsureLoadBalancerDeleted.
func (c *Cloud) EnsureLoadBalancerDeleted(ctx context.Context, clusterName string, service *v1.Service) error {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("EnsureLoadBalancerDeleted(%v, %v)", clusterName, service)
	loadBalancerName := c.GetLoadBalancerName(ctx, clusterName, service)

	lb, err := c.describeLoadBalancer(loadBalancerName)
	if err != nil {
		return err
	}

	if reflect.DeepEqual(lb, osc.LoadBalancer{}) {
		klog.Info("Load balancer already deleted: ", loadBalancerName)
		return nil
	}

	securityGroupsItem := []string{}
	if len(lb.SecurityGroups) == 0 && c.netID == "" {
		securityGroupsItem = append(securityGroupsItem, DefaultSrcSgName)
	}

	{
		// De-register the load balancer security group from the instances security group
		err = c.ensureLoadBalancerInstances(lb.LoadBalancerName,
			lb.BackendVmIds,
			map[InstanceID]osc.Vm{})
		if err != nil {
			klog.Errorf("ensureLoadBalancerInstances deregistering load balancer %v,%v,%v : %q",
				lb.LoadBalancerName,
				lb.BackendVmIds,
				nil, err)
		}

		// De-authorize the load balancer security group from the instances security group
		err = c.updateInstanceSecurityGroupsForLoadBalancer(lb, nil, securityGroupsItem)
		if err != nil {
			klog.Errorf("Error deregistering load balancer from instance security groups: %q", err)
			return err
		}
	}

	{
		// Delete the load balancer itself
		request := &osc.DeleteLoadBalancerOpts{
            DeleteLoadBalancerRequest: optional.NewInterface(
                osc.DeleteLoadBalancerRequest{
                    LoadBalancerName: lb.LoadBalancerName,
                }),
		}

		_, httpRes, errDeleteLB := c.lbu.DeleteLoadBalancer(request)
		if err != nil {
			// TODO: Check if error was because load balancer was concurrently deleted
			klog.Errorf("Error deleting load balancer: %q %q", errDeleteLB, httpRes)
			return err
		}
	}

	{
		// Delete the security group(s) for the load balancer
		// Note that this is annoying: the load balancer disappears from the API immediately, but it is still
		// deleting in the background.  We get a DependencyViolation until the load balancer has deleted itself

		var loadBalancerSGs = securityGroupsItem

 		describeRequest := &osc.ReadSecurityGroupsOpts{
		    ReadSecurityGroupsRequest: optional.NewInterface(
                osc.ReadSecurityGroupsRequest{
                    Filters: osc.FiltersSecurityGroup{
					    SecurityGroupNames: loadBalancerSGs,
				    },
                }),
		}

		response, httpRes, err := c.fcu.ReadSecurityGroups(describeRequest)
		if err != nil {
			return fmt.Errorf("error querying security groups for LBU: %q %q", err, httpRes)
		}

		// Collect the security groups to delete
		securityGroupIDs := map[string]struct{}{}

		for _, sg := range response {
			sgID := sg.SecurityGroupId

			if sgID == c.cfg.Global.LbuSecurityGroup {
				//We don't want to delete a security group that was defined in the Cloud Configuration.
				continue
			}
			if sgID == "" {
				klog.Warningf("Ignoring empty security group in %s", service.Name)
				continue
			}

			if !c.tagging.hasClusterTag(sg.Tags) {
				klog.Warningf("Ignoring security group with no cluster tag in %s", service.Name)
				continue
			}

			securityGroupIDs[sgID] = struct{}{}
		}

		// Loop through and try to delete them
		timeoutAt := time.Now().Add(time.Second * 600)
		for {
			for securityGroupID := range securityGroupIDs {
				request := &osc.DeleteSecurityGroupOpts{
				    DeleteSecurityGroupRequest: optional.NewInterface(
                        osc.DeleteSecurityGroupRequest{
                            SecurityGroupId: securityGroupID,
                        }),
				}
				_, httpRes, err := c.fcu.DeleteSecurityGroup(request)
				if err == nil {
					delete(securityGroupIDs, securityGroupID)
				} else {
					ignore := false
					if awsError, ok := err.(awserr.Error); ok {
						if awsError.Code() == "DependencyViolation" || awsError.Code() == "InvalidGroup.InUse" {
							klog.V(2).Infof("Ignoring DependencyViolation or  InvalidGroup.InUse while deleting load-balancer security group (%s), assuming because LB is in process of deleting", securityGroupID)
							ignore = true
						}
					}
					if !ignore {
						return fmt.Errorf("error while deleting load balancer security group (%s): %q %q", securityGroupID, err, httpRes)
					}
				}
			}

			if len(securityGroupIDs) == 0 {
				klog.V(2).Info("Deleted all security groups for load balancer: ", service.Name)
				break
			}

			if time.Now().After(timeoutAt) {
				ids := []string{}
				for id := range securityGroupIDs {
					ids = append(ids, id)
				}

				return fmt.Errorf("timed out deleting LBU: %s. Could not delete security groups %v", service.Name, strings.Join(ids, ","))
			}

			klog.V(2).Info("Waiting for load-balancer to delete so we can delete security groups: ", service.Name)

			time.Sleep(10 * time.Second)
		}
	}

	return nil
}

// UpdateLoadBalancer implements LoadBalancer.UpdateLoadBalancer
func (c *Cloud) UpdateLoadBalancer(ctx context.Context, clusterName string, service *v1.Service, nodes []*v1.Node) error {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("UpdateLoadBalancer(%v, %v, %s)", clusterName, service, nodes)
	instances, err := c.findInstancesForLBU(nodes)
	if err != nil {
		return err
	}

	loadBalancerName := c.GetLoadBalancerName(ctx, clusterName, service)
	lb, err := c.describeLoadBalancer(loadBalancerName)
	if err != nil {
		return err
	}

	if reflect.DeepEqual(lb, osc.LoadBalancer{}) {
		return fmt.Errorf("Load balancer not found")
	}

	if sslPolicyName, ok := service.Annotations[ServiceAnnotationLoadBalancerSSLNegotiationPolicy]; ok {
		err := c.ensureSSLNegotiationPolicy(lb, sslPolicyName)
		if err != nil {
			return err
		}
		for _, port := range c.getLoadBalancerTLSPorts(lb) {
			err := c.setSSLNegotiationPolicy(loadBalancerName, sslPolicyName, port)
			if err != nil {
				return err
			}
		}
	}

	err = c.ensureLoadBalancerInstances(lb.LoadBalancerName, lb.BackendVmIds, instances)
	if err != nil {
		return nil
	}

	securityGroupsItem := []string{}
	if len(lb.SecurityGroups) == 0 && c.netID == "" {
		securityGroupsItem = append(securityGroupsItem, DefaultSrcSgName)
	}

	err = c.updateInstanceSecurityGroupsForLoadBalancer(lb, instances, securityGroupsItem)
	if err != nil {
		return err
	}

	return nil
}

// ********************* CCM Node Resource Functions  *********************

// Returns the instance with the specified ID
func (c *Cloud) getInstanceByID(instanceID string) (osc.Vm, error) {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("getInstanceByID(%v)", instanceID)
	instances, err := c.getInstancesByIDs([]string{instanceID})
	if err != nil {
		return osc.Vm{}, err
	}

	if len(instances) == 0 {
		return osc.Vm{}, cloudprovider.InstanceNotFound
	}
	if len(instances) > 1 {
		return osc.Vm{}, fmt.Errorf("multiple instances found for instance: %s", instanceID)
	}

	return instances[instanceID], nil
}

func (c *Cloud) getInstancesByIDs(instanceIDs []string) (map[string]osc.Vm, error) {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("getInstancesByIDs(%v)", instanceIDs)

	instancesByID := make(map[string]osc.Vm)
	if len(instanceIDs) == 0 {
		return instancesByID, nil
	}

	request := &osc.ReadVmsOpts{
		ReadVmsRequest: optional.NewInterface(
			osc.ReadVmsRequest{
			    Filters: osc.FiltersVm{
			        VmIds: instanceIDs,
			    },
			}),
    }
	instances, httpRes, err := c.fcu.ReadVms(request)
	if err != nil {
	    fmt.Errorf("http %q", httpRes)
		return nil, err
	}

	for _, instance := range instances {
		instanceID := instance.VmId
		if instanceID == "" {
			continue
		}

		instancesByID[instanceID] = instance
	}

	return instancesByID, nil
}

func (c *Cloud) getInstancesByNodeNames(nodeNames []string, states ...string) ([]osc.Vm, error) {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("getInstancesByNodeNames(%v, %v)", nodeNames, states)

	names := nodeNames
	oscInstances := []osc.Vm{}

	for i := 0; i < len(names); i += filterNodeLimit {
		end := i + filterNodeLimit
		if end > len(names) {
			end = len(names)
		}

		nameSlice := names[i:end]

        // A verifier
		filters := osc.FiltersVm{
		    Tags: nameSlice,
		 }

// 		if len(states) > 0 {
// 			filters = append(filters, newVmFilter("instance-state-name", states...))
// 		}

		instances, err := c.describeInstances(filters)
		if err != nil {
			klog.V(2).Infof("Failed to describe instances %v", nodeNames)
			return []osc.Vm{}, err
		}
		oscInstances = append(oscInstances, instances...)
	}

	if len(oscInstances) == 0 {
		klog.V(3).Infof("Failed to find any instances %v", nodeNames)
		return []osc.Vm{}, nil
	}
	return oscInstances, nil
}

// TODO: Move to instanceCache
func (c *Cloud) describeInstances(filters osc.FiltersVm) ([]osc.Vm, error) {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("describeInstances(%v)", filters)


	request := &osc.ReadVmsOpts{
		ReadVmsRequest: optional.NewInterface(
			osc.ReadVmsRequest{
				Filters: filters,
			}),
    }
	response, httpRes, err := c.fcu.ReadVms(request)
	if err != nil {
	    fmt.Errorf("http %q", httpRes)
		return []osc.Vm{}, err
	}

	var matches []osc.Vm
	for _, instance := range response {
		if c.tagging.hasClusterTag(instance.Tags) {
			matches = append(matches, instance)
		}
	}
	return matches, nil
}

// Returns the instance with the specified node name
// Returns nil if it does not exist
func (c *Cloud) findInstanceByNodeName(nodeName types.NodeName) (osc.Vm, error) {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("findInstanceByNodeName(%v)", nodeName)

	privateDNSName := mapNodeNameToPrivateDNSName(nodeName)

	filters := osc.FiltersVm{
	        Tags: []string{TagNameClusterNode:privateDNSName, c.tagging.clusterTagKey():[ResourceLifecycleOwned, ResourceLifecycleShared]},
//             Tags: []string{
//                 {
//                     c.tagging.clusterTagKey(): ResourceLifecycleOwned,
//                 },
//                 {
//                     c.tagging.clusterTagKey(): ResourceLifecycleShared,
//                 },
//                 {
//                     [TagNameClusterNode: privateDNSName],
//                 },
//             },

// 		newVmFilter("tag:"+TagNameClusterNode, privateDNSName),
// 		// exclude instances in "terminated" state
// 		newVmFilter("instance-state-name", aliveFilter...),
// 		newVmFilter("tag:"+c.tagging.clusterTagKey(),
// 			[]string{ResourceLifecycleOwned, ResourceLifecycleShared}...),
	}

	instances, err := c.describeInstances(filters)

	if err != nil {
		return osc.Vm{}, err
	}

	if len(instances) == 0 {
		return osc.Vm{}, nil
	}
	if len(instances) > 1 {
		return osc.Vm{}, fmt.Errorf("multiple instances found for name: %s", nodeName)
	}

	return instances[0], nil
}

// Returns the instance with the specified node name
// Like findInstanceByNodeName, but returns error if node not found
func (c *Cloud) getInstanceByNodeName(nodeName types.NodeName) (osc.Vm, error) {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("getInstanceByNodeName(%v)", nodeName)

	var instance osc.Vm

	// we leverage node cache to try to retrieve node's provider id first, as
	// get instance by provider id is way more efficient than by filters in
	// osc context
	oscID, err := c.nodeNameToProviderID(nodeName)
	if err != nil {
		klog.V(3).Infof("Unable to convert node name %q to osc instanceID, fall back to findInstanceByNodeName: %v", nodeName, err)
		instance, err = c.findInstanceByNodeName(nodeName)
		// we need to set provider id for next calls

	} else {
		instance, err = c.getInstanceByID(string(oscID))
	}
	if err == nil && reflect.DeepEqual(instance,osc.Vm{}) {
		return osc.Vm{}, cloudprovider.InstanceNotFound
	}
	return instance, err
}

func (c *Cloud) getFullInstance(nodeName types.NodeName) (*oscInstance, osc.Vm, error) {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("getFullInstance(%v)", nodeName)
	if nodeName == "" {
		instance, err := c.getInstanceByID(c.selfOSCInstance.oscID)
		return c.selfOSCInstance, instance, err
	}
	instance, err := c.getInstanceByNodeName(nodeName)
	if err != nil {
		return nil, osc.Vm{}, err
	}
	oscInstance := newOSCInstance(c.fcu, instance)
	return oscInstance, instance, err
}

func (c *Cloud) nodeNameToProviderID(nodeName types.NodeName) (InstanceID, error) {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("nodeNameToProviderID(%v)", nodeName)
	if len(nodeName) == 0 {
		return "", fmt.Errorf("no nodeName provided")
	}

	if c.nodeInformerHasSynced == nil || !c.nodeInformerHasSynced() {
		return "", fmt.Errorf("node informer has not synced yet")
	}

	node, err := c.nodeInformer.Lister().Get(string(nodeName))
	if err != nil {
		return "", err
	}
	if len(node.Spec.ProviderID) == 0 {
		return "", fmt.Errorf("node has no providerID")
	}

	return KubernetesInstanceID(node.Spec.ProviderID).MapToOSCInstanceID()
}
