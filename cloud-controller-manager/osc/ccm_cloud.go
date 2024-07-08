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
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/elb"
	"github.com/outscale/osc-sdk-go/v2"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/informers"
	informercorev1 "k8s.io/client-go/informers/core/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"

	cloudprovider "k8s.io/cloud-provider"
	servicehelpers "k8s.io/cloud-provider/service/helpers"
)

// ********************* CCM Cloud Object Def *********************

// Cloud is an implementation of Interface, LoadBalancer and Instances for Amazon Web Services.
type Cloud struct {
	compute      Compute
	loadBalancer LoadBalancer
	metadata     EC2Metadata
	cfg          *CloudConfig
	region       string
	vpcID        string

	instances cloudprovider.InstancesV2

	tagging resourceTagging

	// The AWS instance that we are running on
	// Note that we cache some state in awsInstance (mountpoints), so we must preserve the instance
	selfAWSInstance *VM

	instanceCache instanceCache

	clientBuilder cloudprovider.ControllerClientBuilder
	kubeClient    clientset.Interface

	nodeInformer informercorev1.NodeInformer
	// Extract the function out to make it easier to test
	nodeInformerHasSynced cache.InformerSynced
	eventBroadcaster      record.EventBroadcaster
	eventRecorder         record.EventRecorder
}

// ********************* CCM Cloud Object functions *********************

// ********************* CCM Cloud Context functions *********************
// Builds the awsInstance for the EC2 instance on which we are running.
// This is called when the AWSCloud is initialized, and should not be called otherwise (because the awsInstance for the local instance is a singleton with drive mapping state)
func (c *Cloud) buildSelfAWSInstance() (*VM, error) {
	debugPrintCallerFunctionName()
	klog.V(5).Infof("buildSelfAWSInstance()")
	if c.selfAWSInstance != nil {
		panic("do not call buildSelfAWSInstance directly")
	}
	instanceID, err := c.metadata.GetMetadata("instance-id")
	if err != nil {
		return nil, fmt.Errorf("error fetching instance-id from ec2 metadata service: %q", err)
	}

	// We want to fetch the hostname via the EC2 metadata service
	// (`GetMetadata("local-hostname")`): But see #11543 - we need to use
	// the EC2 API to get the privateDnsName in case of a private DNS zone
	// e.g. mydomain.io, because the metadata service returns the wrong
	// hostname.  Once we're doing that, we might as well get all our
	// information from the instance returned by the EC2 API - it is a
	// single API call to get all the information, and it means we don't
	// have two code paths.
	instance, err := c.getInstanceByID(instanceID)
	if err != nil {
		return nil, fmt.Errorf("error finding instance %s: %q", instanceID, err)
	}
	return newAWSInstance(c.compute, instance), nil
}

// SetInformers implements InformerUser interface by setting up informer-fed caches for aws lib to
// leverage Kubernetes API for caching
func (c *Cloud) SetInformers(informerFactory informers.SharedInformerFactory) {
	debugPrintCallerFunctionName()
	klog.V(5).Infof("SetInformers(%v)", informerFactory)
	klog.Infof("Setting up informers for Cloud")
	c.nodeInformer = informerFactory.Core().V1().Nodes()
	c.nodeInformerHasSynced = c.nodeInformer.Informer().HasSynced
}

// AddSSHKeyToAllInstances is currently not implemented.
func (c *Cloud) AddSSHKeyToAllInstances(ctx context.Context, user string, keyData []byte) error {
	debugPrintCallerFunctionName()
	klog.V(5).Infof("AddSSHKeyToAllInstances(%v,%v)", user, keyData)
	return cloudprovider.NotImplemented
}

// CurrentNodeName returns the name of the current node
func (c *Cloud) CurrentNodeName(ctx context.Context, hostname string) (types.NodeName, error) {
	debugPrintCallerFunctionName()
	klog.V(5).Infof("CurrentNodeName(%v)", hostname)
	return c.selfAWSInstance.nodeName, nil
}

// Initialize passes a Kubernetes clientBuilder interface to the cloud provider
func (c *Cloud) Initialize(clientBuilder cloudprovider.ControllerClientBuilder,
	stop <-chan struct{}) {
	debugPrintCallerFunctionName()
	klog.V(5).Infof("Initialize(%v,%v)", clientBuilder, stop)
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
	klog.V(5).Infof("Clusters()")
	return nil, false
}

// ProviderName returns the cloud provider ID.
func (c *Cloud) ProviderName() string {
	debugPrintCallerFunctionName()
	klog.V(5).Infof("ProviderName")
	return ProviderName
}

// LoadBalancer returns an implementation of LoadBalancer for Amazon Web Services.
func (c *Cloud) LoadBalancer() (cloudprovider.LoadBalancer, bool) {
	debugPrintCallerFunctionName()
	klog.V(5).Infof("LoadBalancer()")
	return c, true
}

// Instances returns an implementation of Instances for Amazon Web Services.
func (c *Cloud) Instances() (cloudprovider.Instances, bool) {
	debugPrintCallerFunctionName()
	klog.V(5).Infof("Instances()")
	return c, true
}

// InstancesV2 is an implementation for instances and should only be implemented by external cloud providers.
// Implementing InstancesV2 is behaviorally identical to Instances but is optimized to significantly reduce
// API calls to the cloud provider when registering and syncing nodes.
// Also returns true if the interface is supported, false otherwise.
func (c *Cloud) InstancesV2() (cloudprovider.InstancesV2, bool) {
	return c.instances, true
}

// Zones returns an implementation of Zones for Amazon Web Services.
func (c *Cloud) Zones() (cloudprovider.Zones, bool) {
	debugPrintCallerFunctionName()
	return c, true
}

// Routes returns an implementation of Routes for Amazon Web Services.
func (c *Cloud) Routes() (cloudprovider.Routes, bool) {
	debugPrintCallerFunctionName()
	return c, false
}

// HasClusterID returns true if the cluster has a clusterID
func (c *Cloud) HasClusterID() bool {
	debugPrintCallerFunctionName()
	return len(c.tagging.clusterID()) > 0
}

// NodeAddresses is an implementation of Instances.NodeAddresses.
func (c *Cloud) NodeAddresses(ctx context.Context, name types.NodeName) ([]v1.NodeAddress, error) {
	debugPrintCallerFunctionName()
	klog.V(5).Infof("NodeAddresses(%v)", name)
	if c.selfAWSInstance.nodeName == name || len(name) == 0 {
		addresses := []v1.NodeAddress{}

		macs, err := c.metadata.GetMetadata("network/interfaces/macs/")
		if err != nil {
			return nil, fmt.Errorf("error querying AWS metadata for %q: %q", "network/interfaces/macs", err)
		}

		for _, macID := range strings.Split(macs, "\n") {
			if macID == "" {
				continue
			}
			macPath := path.Join("network/interfaces/macs/", macID, "local-ipv4s")
			internalIPs, err := c.metadata.GetMetadata(macPath)
			if err != nil {
				return nil, fmt.Errorf("error querying AWS metadata for %q: %q", macPath, err)
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
			// but the AWS client masks all failures with the same error description.
			klog.V(4).Info("Could not determine public IP from AWS metadata.")
		} else {
			addresses = append(addresses, v1.NodeAddress{Type: v1.NodeExternalIP, Address: externalIP})
		}

		localHostname, err := c.metadata.GetMetadata("local-hostname")
		if err != nil || len(localHostname) == 0 {
			//TODO: It would be nice to be able to determine the reason for the failure,
			// but the AWS client masks all failures with the same error description.
			klog.V(4).Info("Could not determine private DNS from AWS metadata.")
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
			// but the AWS client masks all failures with the same error description.
			klog.V(4).Info("Could not determine public DNS from AWS metadata.")
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
	klog.V(5).Infof("NodeAddressesByProviderID(%v)", providerID)
	instanceID, err := KubernetesInstanceID(providerID).MapToAWSInstanceID()
	if err != nil {
		return nil, err
	}

	instance, err := describeInstance(c.compute, instanceID)
	if err != nil {
		return nil, err
	}

	return extractNodeAddresses(instance)
}

// InstanceExistsByProviderID returns true if the instance with the given provider id still exists.
// If false is returned with no error, the instance will be immediately deleted by the cloud controller manager.
func (c *Cloud) InstanceExistsByProviderID(ctx context.Context, providerID string) (bool, error) {
	debugPrintCallerFunctionName()
	klog.V(5).Infof("InstanceExistsByProviderID(%v)", providerID)
	instanceID, err := KubernetesInstanceID(providerID).MapToAWSInstanceID()
	if err != nil {
		return false, err
	}

	request := &osc.ReadVmsRequest{
		Filters: &osc.FiltersVm{
			VmIds: &[]string{string(instanceID)},
		},
	}

	instances, err := c.compute.ReadVms(request)
	if err != nil {
		return false, err
	}
	if len(instances) == 0 {
		return false, nil
	}
	if len(instances) > 1 {
		return false, fmt.Errorf("multiple instances found for instance: %s", instanceID)
	}

	state := instances[0].State
	if *state == "terminated" {
		klog.Warningf("the instance %s is terminated", instanceID)
		return false, nil
	}

	return true, nil
}

// InstanceShutdownByProviderID returns true if the instance is in safe state to detach volumes
func (c *Cloud) InstanceShutdownByProviderID(ctx context.Context, providerID string) (bool, error) {
	debugPrintCallerFunctionName()
	klog.V(5).Infof("InstanceShutdownByProviderID(%v)", providerID)
	instanceID, err := KubernetesInstanceID(providerID).MapToAWSInstanceID()
	if err != nil {
		return false, err
	}

	request := &osc.ReadVmsRequest{
		Filters: &osc.FiltersVm{
			VmIds: &[]string{string(instanceID)},
		},
	}

	instances, err := c.compute.ReadVms(request)
	if err != nil {
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
	if instance.State != nil {
		// valid state for detaching volumes
		if *instance.State == "stopped" {
			return true, nil
		}
	}
	return false, nil
}

// InstanceID returns the cloud provider ID of the node with the specified nodeName.
func (c *Cloud) InstanceID(ctx context.Context, nodeName types.NodeName) (string, error) {
	debugPrintCallerFunctionName()
	klog.V(5).Infof("InstanceID(%v)", nodeName)
	// In the future it is possible to also return an endpoint as:
	// <endpoint>/<zone>/<instanceid>
	if c.selfAWSInstance.nodeName == nodeName {
		return "/" + c.selfAWSInstance.availabilityZone + "/" + c.selfAWSInstance.vmID, nil
	}
	inst, err := c.getInstanceByNodeName(nodeName)
	if err != nil {
		if err == cloudprovider.InstanceNotFound {
			// The Instances interface requires that we return InstanceNotFound (without wrapping)
			return "", err
		}
		return "", fmt.Errorf("getInstanceByNodeName failed for %q with %q", nodeName, err)
	}
	return "/" + inst.Placement.GetSubregionName() + "/" + inst.GetVmId(), nil
}

// InstanceTypeByProviderID returns the cloudprovider instance type of the node with the specified unique providerID
// This method will not be called from the node that is requesting this ID. i.e. metadata service
// and other local methods cannot be used here
func (c *Cloud) InstanceTypeByProviderID(ctx context.Context, providerID string) (string, error) {
	debugPrintCallerFunctionName()
	klog.V(5).Infof("InstanceTypeByProviderID(%v)", providerID)
	instanceID, err := KubernetesInstanceID(providerID).MapToAWSInstanceID()
	if err != nil {
		return "", err
	}

	instance, err := describeInstance(c.compute, instanceID)
	if err != nil {
		return "", err
	}

	return instance.GetVmType(), nil
}

// InstanceType returns the type of the node with the specified nodeName.
func (c *Cloud) InstanceType(ctx context.Context, nodeName types.NodeName) (string, error) {
	debugPrintCallerFunctionName()
	klog.V(5).Infof("InstanceType(%v)", nodeName)
	if c.selfAWSInstance.nodeName == nodeName {
		return c.selfAWSInstance.instanceType, nil
	}
	inst, err := c.getInstanceByNodeName(nodeName)
	if err != nil {
		return "", fmt.Errorf("getInstanceByNodeName failed for %q with %q", nodeName, err)
	}
	return inst.GetVmType(), nil
}

// GetZone implements Zones.GetZone
func (c *Cloud) GetZone(ctx context.Context) (cloudprovider.Zone, error) {
	debugPrintCallerFunctionName()
	return cloudprovider.Zone{
		FailureDomain: c.selfAWSInstance.availabilityZone,
		Region:        c.region,
	}, nil
}

// GetZoneByProviderID implements Zones.GetZoneByProviderID
// This is particularly useful in external cloud providers where the kubelet
// does not initialize node data.
func (c *Cloud) GetZoneByProviderID(ctx context.Context, providerID string) (cloudprovider.Zone, error) {
	debugPrintCallerFunctionName()
	klog.V(5).Infof("GetZoneByProviderID(%v)", providerID)
	instanceID, err := KubernetesInstanceID(providerID).MapToAWSInstanceID()
	if err != nil {
		return cloudprovider.Zone{}, err
	}
	instance, err := c.getInstanceByID(string(instanceID))
	if err != nil {
		return cloudprovider.Zone{}, err
	}

	zone := cloudprovider.Zone{
		FailureDomain: instance.Placement.GetSubregionName(),
		Region:        c.region,
	}

	return zone, nil
}

// GetZoneByNodeName implements Zones.GetZoneByNodeName
// This is particularly useful in external cloud providers where the kubelet
// does not initialize node data.
func (c *Cloud) GetZoneByNodeName(ctx context.Context, nodeName types.NodeName) (cloudprovider.Zone, error) {
	debugPrintCallerFunctionName()
	klog.V(5).Infof("GetZoneByNodeName(%v)", nodeName)
	instance, err := c.getInstanceByNodeName(nodeName)
	if err != nil {
		return cloudprovider.Zone{}, err
	}
	zone := cloudprovider.Zone{
		FailureDomain: instance.Placement.GetSubregionName(),
		Region:        c.region,
	}

	return zone, nil

}

// Retrieves instance's vpc id from metadata
func (c *Cloud) findVPCID() (string, error) {
	debugPrintCallerFunctionName()
	klog.V(5).Infof("findVPCID()")
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
		vpcID, err := c.metadata.GetMetadata(url)
		if err != nil {
			continue
		}
		return vpcID, nil
	}
	return "", fmt.Errorf("could not find VPC ID in instance metadata")
}

// ********************* CCM Cloud Resource LBU Functions  *********************

func (c *Cloud) addLoadBalancerTags(loadBalancerName string, requested map[string]string) error {
	debugPrintCallerFunctionName()
	klog.V(5).Infof("addLoadBalancerTags(%v,%v)", loadBalancerName, requested)
	var tags []*elb.Tag
	for k, v := range requested {
		tag := &elb.Tag{
			Key:   aws.String(k),
			Value: aws.String(v),
		}
		tags = append(tags, tag)
	}

	request := &elb.AddTagsInput{}
	request.LoadBalancerNames = []*string{&loadBalancerName}
	request.Tags = tags

	_, err := c.loadBalancer.AddTags(request)
	if err != nil {
		return fmt.Errorf("error adding tags to load balancer: %v", err)
	}
	return nil
}

// Gets the current load balancer state
func (c *Cloud) describeLoadBalancer(name string) (*elb.LoadBalancerDescription, error) {
	debugPrintCallerFunctionName()
	klog.V(5).Infof("describeLoadBalancer(%v)", name)
	request := &elb.DescribeLoadBalancersInput{}
	request.LoadBalancerNames = []*string{&name}

	response, err := c.loadBalancer.DescribeLoadBalancers(request)
	if err != nil {
		if awsError, ok := err.(awserr.Error); ok {
			if awsError.Code() == "LoadBalancerNotFound" {
				return nil, nil
			}
		}
		return nil, err
	}

	var ret *elb.LoadBalancerDescription
	for _, loadBalancer := range response.LoadBalancerDescriptions {
		if ret != nil {
			klog.Errorf("Found multiple load balancers with name: %s", name)
		}
		ret = loadBalancer
	}
	return ret, nil
}

// Retrieves the specified security group from the AWS API, or returns nil if not found
func (c *Cloud) findSecurityGroup(securityGroupID string) (*osc.SecurityGroup, error) {
	debugPrintCallerFunctionName()
	klog.V(5).Infof("findSecurityGroup(%v)", securityGroupID)
	readSecurityGroupsRequest := osc.ReadSecurityGroupsRequest{
		Filters: &osc.FiltersSecurityGroup{
			SecurityGroupIds: &[]string{
				securityGroupID,
			},
		},
	}
	// We don't apply our tag filters because we are retrieving by ID

	groups, err := c.compute.ReadSecurityGroups(&readSecurityGroupsRequest)
	if err != nil {
		klog.Warningf("Error retrieving security group: %q", err)
		return nil, err
	}

	if len(groups) == 0 {
		return nil, nil
	}
	if len(groups) != 1 {
		// This should not be possible - ids should be unique
		return nil, fmt.Errorf("multiple security groups found with same id %q", securityGroupID)
	}
	group := groups[0]
	return &group, nil
}

// Makes sure the security group ingress is exactly the specified permissions
// Returns true if and only if changes were made
// The security group must already exist
func (c *Cloud) setSecurityGroupIngress(securityGroupID string, permissions IPRulesSet) (bool, error) {
	debugPrintCallerFunctionName()
	klog.V(5).Infof("setSecurityGroupIngress(%v,%v)", securityGroupID, permissions)
	// We do not want to make changes to the Global defined SG
	if securityGroupID == c.cfg.Global.ElbSecurityGroup {
		return false, nil
	}

	group, err := c.findSecurityGroup(securityGroupID)
	if err != nil {
		klog.Warningf("Error retrieving security group %q", err)
		return false, err
	}

	if group == nil {
		return false, fmt.Errorf("security group not found: %s", securityGroupID)
	}

	klog.V(2).Infof("Existing security group ingress: %s %v", securityGroupID, group.GetInboundRules())

	actual := NewIPRulesSet(group.GetInboundRules()...)

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
	// But this is only used on the ELB security group currently, so it
	// would require (ports * CIDRS) > 100.  Also, it isn't obvious exactly
	// how removing single permissions from compound rules works, and we
	// don't want to accidentally open more than intended while we're
	// applying changes.
	if add.Len() != 0 {
		klog.V(2).Infof("Adding security group ingress: %s %v", securityGroupID, add.List())

		list := add.List()
		request := osc.CreateSecurityGroupRuleRequest{
			Flow:            "Inbound",
			SecurityGroupId: securityGroupID,
			Rules:           &list,
		}

		_, err = c.compute.CreateSecurityGroupRule(&request)
		if err != nil {
			return false, fmt.Errorf("error authorizing security group ingress: %q", err)
		}
	}
	if remove.Len() != 0 {
		klog.V(2).Infof("Remove security group ingress: %s %v", securityGroupID, remove.List())

		list := remove.List()
		request := osc.DeleteSecurityGroupRuleRequest{
			Flow:            "Inbound",
			SecurityGroupId: securityGroupID,
			Rules:           &list,
		}

		_, err = c.compute.DeleteSecurityGroupRule(&request)
		if err != nil {
			return false, fmt.Errorf("error revoking security group ingress: %q", err)
		}
	}

	return true, nil
}

// Makes sure the security group includes the specified permissions
// Returns true if and only if changes were made
// The security group must already exist
func (c *Cloud) addSecurityGroupRules(securityGroupID string, addPermissions *[]osc.SecurityGroupRule, isPublicCloud bool) (bool, error) {
	debugPrintCallerFunctionName()
	klog.V(5).Infof("addSecurityGroupRules(%v,%v,%v)", securityGroupID, addPermissions, isPublicCloud)
	// We do not want to make changes to the Global defined SG
	if securityGroupID == c.cfg.Global.ElbSecurityGroup {
		return false, nil
	}

	group, err := c.findSecurityGroup(securityGroupID)
	if err != nil {
		klog.Warningf("Error retrieving security group: %q", err)
		return false, err
	}

	if group == nil {
		return false, fmt.Errorf("security group not found: %s", securityGroupID)
	}

	klog.Infof("Existing security group ingress: %s %v", securityGroupID, group.GetInboundRules())

	changes := []osc.SecurityGroupRule{}
	for _, addPermission := range *addPermissions {
		hasUserID := false
		for _, member := range addPermission.GetSecurityGroupsMembers() {
			if member.HasAccountId() {
				hasUserID = true
			}
		}

		found := false
		for _, groupPermission := range group.GetInboundRules() {
			if ruleExists(&addPermission, &groupPermission, hasUserID) {
				found = true
				break
			}
		}

		if !found {
			changes = append(changes, addPermission)
		}
	}

	if len(changes) == 0 && !isPublicCloud {
		return false, nil
	}

	klog.Infof("Adding security group ingress: %s %v isPublic %v)", securityGroupID, changes, isPublicCloud)

	request := osc.CreateSecurityGroupRuleRequest{
		Flow:            "Inbound",
		SecurityGroupId: securityGroupID,
	}
	if !isPublicCloud {
		request.SetRules(changes)
	} else {
		request.SetSecurityGroupNameToLink(DefaultSrcSgName)
		request.SetSecurityGroupAccountIdToLink(DefaultSgOwnerID)
	}
	_, err = c.compute.CreateSecurityGroupRule(&request)
	if err != nil {
		ignore := false
		if isPublicCloud {
			if strings.Contains(err.Error(), "Conflict") {
				klog.V(2).Infof("Ignoring Duplicate for security group (%s), assuming is used by other public LB", securityGroupID)
				ignore = true

			}
		}
		if !ignore {
			klog.Warningf("Error authorizing security group ingress %q", err)
			return false, fmt.Errorf("error authorizing security group ingress: %q", err)
		}
	}

	return true, nil
}

// Makes sure the security group no longer includes the specified permissions
// Returns true if and only if changes were made
// If the security group no longer exists, will return (false, nil)
func (c *Cloud) removeSecurityGroupRules(securityGroupID string, removePermissions *[]osc.SecurityGroupRule, isPublicCloud bool) (bool, error) {
	debugPrintCallerFunctionName()
	klog.V(5).Infof("removeSecurityGroupRules(%v,%v)", securityGroupID, removePermissions)
	// We do not want to make changes to the Global defined SG
	if securityGroupID == c.cfg.Global.ElbSecurityGroup {
		return false, nil
	}

	group, err := c.findSecurityGroup(securityGroupID)
	if err != nil {
		klog.Warningf("Error retrieving security group: %q", err)
		return false, err
	}

	if group == nil {
		klog.Warning("Security group not found: ", securityGroupID)
		return false, nil
	}

	changes := []osc.SecurityGroupRule{}
	for _, removePermission := range *removePermissions {
		hasUserID := false
		for _, member := range removePermission.GetSecurityGroupsMembers() {
			if member.HasAccountId() {
				hasUserID = true
			}
		}

		for _, groupPermission := range group.GetInboundRules() {
			if ruleExists(&removePermission, &groupPermission, hasUserID) {
				changes = append(changes, removePermission)
				break
			}
		}

	}

	if len(changes) == 0 && !isPublicCloud {
		return false, nil
	}

	klog.Infof("Removing security group ingress: %s %v", securityGroupID, changes)

	request := osc.DeleteSecurityGroupRuleRequest{
		Flow:            "Inbound",
		SecurityGroupId: securityGroupID,
	}
	if !isPublicCloud {
		request.SetRules(changes)
	} else {
		request.SetSecurityGroupNameToUnlink(DefaultSrcSgName)
		request.SetSecurityGroupAccountIdToUnlink(DefaultSgOwnerID)
	}

	_, err = c.compute.DeleteSecurityGroupRule(&request)
	if err != nil {
		klog.Warningf("Error revoking security group ingress: %q", err)
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
	klog.V(5).Infof("ensureSecurityGroup (%v,%v,%v)", name, description, additionalTags)

	groupID := ""
	attempt := 0
	for {
		attempt++

		// Note that we do _not_ add our tag filters; group-name + vpc-id is the EC2 primary key.
		// However, we do check that it matches our tags.
		// If it doesn't have any tags, we tag it; this is how we recover if we failed to tag before.
		// If it has a different cluster's tags, that is an error.
		// This shouldn't happen because name is expected to be globally unique (UUID derived)
		request := osc.ReadSecurityGroupsRequest{
			Filters: &osc.FiltersSecurityGroup{
				SecurityGroupNames: &[]string{name},
			},
		}

		if c.vpcID != "" {
			request.Filters.NetIds = &[]string{c.vpcID}
		}

		securityGroups, err := c.compute.ReadSecurityGroups(&request)
		if err != nil {
			return "", err
		}

		if len(securityGroups) >= 1 {
			if len(securityGroups) > 1 {
				klog.Warningf("Found multiple security groups with name: %q", name)
			}
			err := c.tagging.readRepairClusterTags(
				c.compute, securityGroups[0].GetSecurityGroupId(),
				ResourceLifecycleOwned, nil, securityGroups[0].Tags)
			if err != nil {
				return "", err
			}

			return securityGroups[0].GetSecurityGroupId(), nil
		}

		createRequest := osc.CreateSecurityGroupRequest{}
		if c.vpcID != "" {
			createRequest.SetNetId(c.vpcID)
		}
		createRequest.SetSecurityGroupName(name)
		createRequest.SetDescription(description)

		createResponse, err := c.compute.CreateSecurityGroup(&createRequest)
		if err != nil {
			ignore := false
			if strings.Contains(err.Error(), "Conflict") && attempt < MaxReadThenCreateRetries {
				klog.V(2).Infof("Got InvalidGroup.Duplicate while creating security group (race?); will retry")
				ignore = true
			}
			if !ignore {
				klog.Errorf("Error creating security group: %q", err)
				return "", err
			}
			time.Sleep(1 * time.Second)
		} else {
			groupID = createResponse.SecurityGroup.GetSecurityGroupId()
			break
		}
	}
	if groupID == "" {
		return "", fmt.Errorf("created security group, but id was not returned: %s", name)
	}

	err := c.tagging.createTags(c.compute, groupID, ResourceLifecycleOwned, additionalTags)
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
func (c *Cloud) findSubnets() ([]*osc.Subnet, error) {
	debugPrintCallerFunctionName()
	klog.V(5).Infof("findSubnets()")
	request := osc.ReadSubnetsRequest{}
	if c.vpcID != "" {
		request.SetFilters(osc.FiltersSubnet{
			NetIds: &[]string{
				c.vpcID,
			},
		})

		subnets, err := c.compute.DescribeSubnets(&request)
		if err != nil {
			return nil, fmt.Errorf("error describing subnets: %q", err)
		}

		var matches []*osc.Subnet
		for _, subnet := range subnets {
			if c.tagging.hasClusterTag(subnet.Tags) {
				subnetRef := subnet
				matches = append(matches, &subnetRef)
			}
		}

		if len(matches) != 0 {
			return matches, nil
		}
	}

	if c.selfAWSInstance.subnetID != "" {
		// Fall back to the current instance subnets, if nothing is tagged
		klog.Warningf("No tagged subnets found; will fall-back to the current subnet only.  This is likely to be an error in a future version of k8s.")
		request = osc.ReadSubnetsRequest{}
		request.SetFilters(osc.FiltersSubnet{
			SubnetIds: &[]string{
				c.selfAWSInstance.subnetID,
			},
		})
		subnets, err := c.compute.DescribeSubnets(&request)
		if err != nil {
			return nil, fmt.Errorf("error describing subnets: %q", err)
		}

		var matches []*osc.Subnet
		for _, subnet := range subnets {
			subnetRef := subnet
			matches = append(matches, &subnetRef)
		}
		return matches, nil

	}

	return []*osc.Subnet{}, nil

}

// Finds the subnets to use for an ELB we are creating.
// Normal (Internet-facing) ELBs must use public subnets, so we skip private subnets.
// Internal ELBs can use public or private subnets, but if we have a private subnet we should prefer that.
func (c *Cloud) findELBSubnets(internalELB bool) ([]string, error) {
	debugPrintCallerFunctionName()
	klog.V(5).Infof("findELBSubnets(%v)", internalELB)

	subnets, err := c.findSubnets()
	if err != nil {
		return nil, err
	}
	var rt []osc.RouteTable
	if c.vpcID != "" {
		readRequest := osc.ReadRouteTablesRequest{
			Filters: &osc.FiltersRouteTable{
				NetIds: &[]string{c.vpcID},
			},
		}
		rt, err = c.compute.ReadRouteTables(&readRequest)
		if err != nil {
			return nil, fmt.Errorf("error describe route table: %q", err)
		}
	}

	// Try to break the tie using a tag
	var tagName string
	if internalELB {
		tagName = TagNameSubnetInternalELB
	} else {
		tagName = TagNameSubnetPublicELB
	}

	subnetsByAZ := make(map[string]*osc.Subnet)
	for _, subnet := range subnets {
		az := subnet.GetSubregionName()
		id := subnet.GetSubnetId()
		if az == "" || id == "" {
			klog.Warningf("Ignoring subnet with empty az/id: %v", subnet)
			continue
		}

		isPublic, err := isSubnetPublic(&rt, id)
		if err != nil {
			return nil, err
		}
		if !internalELB && !isPublic {
			klog.V(2).Infof("Ignoring private subnet for public ELB %q", id)
			continue
		}

		existing := subnetsByAZ[az]
		_, subnetHasTag := findTag(subnet.Tags, tagName)
		if existing == nil {
			if subnetHasTag {
				subnetsByAZ[az] = subnet
			} else if isPublic && !internalELB {
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
		if strings.Compare(existing.GetSubnetId(), subnet.GetSubnetId()) > 0 {
			klog.Warningf("Found multiple subnets in AZ %q; choosing %q between subnets %q and %q", az, *subnet.SubnetId, *existing.SubnetId, *subnet.SubnetId)
			subnetsByAZ[az] = subnet
			continue
		}

		klog.Warningf("Found multiple subnets in AZ %q; choosing %q between subnets %q and %q", az, *existing.SubnetId, *existing.SubnetId, *subnet.SubnetId)
		continue
	}

	var azNames []string
	for key := range subnetsByAZ {
		azNames = append(azNames, key)
	}

	sort.Strings(azNames)

	var subnetIDs []string
	for _, key := range azNames {
		subnetIDs = append(subnetIDs, aws.StringValue(subnetsByAZ[key].SubnetId))
	}

	return subnetIDs, nil
}

// buildELBSecurityGroupList returns list of SecurityGroups which should be
// attached to ELB created by a service. List always consist of at least
// 1 member which is an SG created for this service or a SG from the Global config.
// Extra groups can be specified via annotation, as can extra tags for any
// new groups. The annotation "ServiceAnnotationLoadBalancerSecurityGroups" allows for
// setting the security groups specified.
func (c *Cloud) buildELBSecurityGroupList(serviceName types.NamespacedName, loadBalancerName string, annotations map[string]string) ([]string, error) {
	debugPrintCallerFunctionName()
	klog.V(5).Infof("buildELBSecurityGroupList(%v,%v,%v)", serviceName, loadBalancerName, annotations)
	var err error
	var securityGroupID string

	if c.cfg.Global.ElbSecurityGroup != "" {
		securityGroupID = c.cfg.Global.ElbSecurityGroup
	} else {
		// Create a security group for the load balancer
		sgName := "k8s-elb-" + loadBalancerName
		sgDescription := fmt.Sprintf("Security group for Kubernetes ELB %s (%v)", loadBalancerName, serviceName)
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
	klog.V(5).Infof("EnsureLoadBalancer(%v, %v, %v)", clusterName, apiService, nodes)
	klog.V(5).Infof("EnsureLoadBalancer.annotations(%v)", apiService.Annotations)
	annotations := apiService.Annotations
	if apiService.Spec.SessionAffinity != v1.ServiceAffinityNone {
		// ELB supports sticky sessions, but only when configured for HTTP/HTTPS
		return nil, fmt.Errorf("unsupported load balancer affinity: %v", apiService.Spec.SessionAffinity)
	}

	if len(apiService.Spec.Ports) == 0 {
		return nil, fmt.Errorf("requested load balancer with no ports")
	}

	// Figure out what mappings we want on the load balancer
	listeners := []*elb.Listener{}

	sslPorts := getPortSets(annotations[ServiceAnnotationLoadBalancerSSLPorts])

	for _, port := range apiService.Spec.Ports {
		if port.Protocol != v1.ProtocolTCP {
			return nil, fmt.Errorf("Only TCP LoadBalancer is supported for AWS ELB")
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
		return nil, fmt.Errorf("LoadBalancerIP cannot be specified for AWS ELB")
	}

	instances, err := c.findInstancesForELB(nodes)
	klog.V(5).Infof("Debug OSC: c.findInstancesForELB(nodes) : %v", instances)
	if err != nil {
		return nil, err
	}

	sourceRanges, err := servicehelpers.GetLoadBalancerSourceRanges(apiService)
	klog.V(5).Infof("Debug OSC:  servicehelpers.GetLoadBalancerSourceRanges : %v", sourceRanges)
	if err != nil {
		return nil, err
	}

	// Determine if this is tagged as an Internal ELB
	internalELB := false
	internalAnnotation := apiService.Annotations[ServiceAnnotationLoadBalancerInternal]
	if internalAnnotation == "false" {
		internalELB = false
	} else if internalAnnotation != "" {
		internalELB = true
	}
	klog.V(5).Infof("Debug OSC:  internalELB : %v", internalELB)

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
	loadBalancerAttributes := &elb.LoadBalancerAttributes{
		ConnectionDraining: &elb.ConnectionDraining{Enabled: aws.Bool(false)},
		ConnectionSettings: &elb.ConnectionSettings{IdleTimeout: aws.Int64(60)},
	}

	if annotations[ServiceAnnotationLoadBalancerAccessLogS3BucketName] != "" &&
		annotations[ServiceAnnotationLoadBalancerAccessLogS3BucketPrefix] != "" {

		loadBalancerAttributes.AccessLog = &elb.AccessLog{Enabled: aws.Bool(false)}

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
			loadBalancerAttributes.AccessLog.Enabled = &accessLogEnabled
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
			loadBalancerAttributes.AccessLog.EmitInterval = &accessLogEmitInterval
		}

		// Determine if access log s3 bucket name has been specified
		accessLogS3BucketNameAnnotation := annotations[ServiceAnnotationLoadBalancerAccessLogS3BucketName]
		if accessLogS3BucketNameAnnotation != "" {
			loadBalancerAttributes.AccessLog.S3BucketName = &accessLogS3BucketNameAnnotation
		}

		// Determine if access log s3 bucket prefix has been specified
		accessLogS3BucketPrefixAnnotation := annotations[ServiceAnnotationLoadBalancerAccessLogS3BucketPrefix]
		if accessLogS3BucketPrefixAnnotation != "" {
			loadBalancerAttributes.AccessLog.S3BucketPrefix = &accessLogS3BucketPrefixAnnotation
		}
		klog.V(5).Infof("Debug OSC:  loadBalancerAttributes.AccessLog : %v", loadBalancerAttributes.AccessLog)
	}

	// Determine if connection draining enabled/disabled has been specified
	connectionDrainingEnabledAnnotation := annotations[ServiceAnnotationLoadBalancerConnectionDrainingEnabled]
	if connectionDrainingEnabledAnnotation != "" {
		connectionDrainingEnabled, err := strconv.ParseBool(connectionDrainingEnabledAnnotation)
		if err != nil {
			return nil, fmt.Errorf("error parsing service annotation: %s=%s",
				ServiceAnnotationLoadBalancerConnectionDrainingEnabled,
				connectionDrainingEnabledAnnotation,
			)
		}
		loadBalancerAttributes.ConnectionDraining.Enabled = &connectionDrainingEnabled
	}

	// Determine if connection draining timeout has been specified
	connectionDrainingTimeoutAnnotation := annotations[ServiceAnnotationLoadBalancerConnectionDrainingTimeout]
	if connectionDrainingTimeoutAnnotation != "" {
		connectionDrainingTimeout, err := strconv.ParseInt(connectionDrainingTimeoutAnnotation, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("error parsing service annotation: %s=%s",
				ServiceAnnotationLoadBalancerConnectionDrainingTimeout,
				connectionDrainingTimeoutAnnotation,
			)
		}
		loadBalancerAttributes.ConnectionDraining.Timeout = &connectionDrainingTimeout
	}

	// Determine if connection idle timeout has been specified
	connectionIdleTimeoutAnnotation := annotations[ServiceAnnotationLoadBalancerConnectionIdleTimeout]
	if connectionIdleTimeoutAnnotation != "" {
		connectionIdleTimeout, err := strconv.ParseInt(connectionIdleTimeoutAnnotation, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("error parsing service annotation: %s=%s",
				ServiceAnnotationLoadBalancerConnectionIdleTimeout,
				connectionIdleTimeoutAnnotation,
			)
		}
		loadBalancerAttributes.ConnectionSettings.IdleTimeout = &connectionIdleTimeout
	}

	// Find the subnets that the ELB will live in
	subnetIDs, err := c.findELBSubnets(internalELB)
	klog.V(2).Infof("Debug OSC:  c.findELBSubnets(internalELB) : %v", subnetIDs)

	if err != nil {
		klog.Errorf("Error listing subnets in VPC: %q", err)
		return nil, err
	}

	// Bail out early if there are no subnets
	if len(subnetIDs) == 0 {
		klog.Warningf("could not find any suitable subnets for creating the ELB")
	}

	if len(subnetIDs) > 0 && annotations[ServiceAnnotationLoadBalancerSubnetID] != "" {
		targetSubnet := annotations[ServiceAnnotationLoadBalancerSubnetID]

		if Contains(subnetIDs, targetSubnet) {
			klog.V(2).Infof("User subnet found, override list of subnets (%v) to ([%v]) ", subnetIDs, targetSubnet)
			subnetIDs = []string{targetSubnet}
		} else {
			return nil, fmt.Errorf("user subnet specified in the annotation %v=%v was not found (%v)", ServiceAnnotationLoadBalancerSubnetID, targetSubnet, subnetIDs)
		}
	} else if len(subnetIDs) > 1 {
		// OAPI does not support multiple subnets
		current := subnetIDs[0]
		for _, subnet := range subnetIDs {
			if strings.Compare(current, subnet) > 0 {
				current = subnet
				continue
			}
		}
		klog.V(2).Infof("LB does not support multiple subnets and the user does not request a specific subnet. Taking the first lexicography subnet of (%v) -> %v", subnetIDs, current)
		subnetIDs = []string{current}
	}

	loadBalancerName := c.GetLoadBalancerName(ctx, clusterName, apiService)
	serviceName := types.NamespacedName{Namespace: apiService.Namespace, Name: apiService.Name}

	klog.V(5).Infof("Debug OSC:  loadBalancerName : %v", loadBalancerName)
	klog.V(5).Infof("Debug OSC:  serviceName : %v", serviceName)
	klog.V(5).Infof("Debug OSC:  serviceName : %v", annotations)

	var securityGroupIDs []string

	if len(subnetIDs) == 0 || c.vpcID == "" {
		securityGroupIDs = []string{DefaultSrcSgName}
	} else {
		securityGroupIDs, err = c.buildELBSecurityGroupList(serviceName, loadBalancerName, annotations)
	}

	klog.V(5).Infof("Debug OSC:  ensured securityGroupIDs : %v", securityGroupIDs)

	if err != nil {
		return nil, err
	}
	if len(securityGroupIDs) == 0 {
		return nil, fmt.Errorf("[BUG] ELB can't have empty list of Security Groups to be assigned, this is a Kubernetes bug, please report")
	}

	if len(subnetIDs) > 0 && c.vpcID != "" {
		oscSGRanges := []string{}
		for _, sourceRange := range sourceRanges.StringSlice() {
			oscSGRanges = append(oscSGRanges, sourceRange)
		}

		permissions := NewIPRulesSet()
		for _, port := range apiService.Spec.Ports {

			protocol := strings.ToLower(string(port.Protocol))

			permission := osc.SecurityGroupRule{}
			permission.SetFromPortRange(port.Port)
			permission.SetToPortRange(port.Port)
			permission.SetIpRanges(oscSGRanges)
			permission.SetIpProtocol(protocol)

			permissions.Insert(permission)
		}

		// Allow ICMP fragmentation packets, important for MTU discovery
		{
			fromPort := int32(3)
			toPort := int32(4)
			permission := osc.SecurityGroupRule{
				IpProtocol:    aws.String("icmp"),
				FromPortRange: &fromPort,
				ToPortRange:   &toPort,
				IpRanges:      &oscSGRanges,
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
		internalELB,
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
			if listener.InstancePort == nil {
				continue
			}
			tcpHealthCheckPort = int32(*listener.InstancePort)
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

	err = c.ensureLoadBalancerInstances(aws.StringValue(loadBalancer.LoadBalancerName), loadBalancer.Instances, instances)
	if err != nil {
		klog.Warningf("Error registering instances with the load balancer: %q", err)
		return nil, err
	}

	klog.V(1).Infof("Loadbalancer %s (%v) has DNS name %s", loadBalancerName, serviceName, aws.StringValue(loadBalancer.DNSName))

	// TODO: Wait for creation?

	status := toStatus(loadBalancer)
	return status, nil
}

// GetLoadBalancer is an implementation of LoadBalancer.GetLoadBalancer
func (c *Cloud) GetLoadBalancer(ctx context.Context, clusterName string, service *v1.Service) (*v1.LoadBalancerStatus, bool, error) {
	debugPrintCallerFunctionName()
	klog.V(5).Infof("GetLoadBalancer(%v,%v)", clusterName, service)
	loadBalancerName := c.GetLoadBalancerName(ctx, clusterName, service)

	lb, err := c.describeLoadBalancer(loadBalancerName)
	if err != nil {
		return nil, false, err
	}

	if lb == nil {
		return nil, false, nil
	}

	status := toStatus(lb)
	return status, true, nil
}

// GetLoadBalancerName is an implementation of LoadBalancer.GetLoadBalancerName
func (c *Cloud) GetLoadBalancerName(ctx context.Context, clusterName string, service *v1.Service) string {
	debugPrintCallerFunctionName()
	klog.V(5).Infof("GetLoadBalancerName(%v,%v)", clusterName, service)

	//The unique name of the load balancer (32 alphanumeric or hyphen characters maximum, but cannot start or end with a hyphen).
	ret := strings.Replace(string(service.UID), "-", "", -1)

	if s, ok := service.Annotations[ServiceAnnotationLoadBalancerName]; ok {
		re := regexp.MustCompile("^[a-zA-Z0-9-]+$")
		fmt.Println("e.MatchString(s): ", s, re.MatchString(s))
		if len(s) <= 0 || !re.MatchString(s) {
			klog.Warningf("Ignoring %v annotation, empty string or does not respect lb name constraints: %v", ServiceAnnotationLoadBalancerName, s)
		} else {
			ret = s
		}
	}

	nameLength := LbNameMaxLength
	if s, ok := service.Annotations[ServiceAnnotationLoadBalancerNameLength]; ok {
		var err error
		nameLength, err = strconv.ParseInt(s, 10, 0)
		if err != nil || nameLength > LbNameMaxLength {
			klog.Warningf("Ignoring %v annotation, failed parsing %v value %v or value greater than %v ", ServiceAnnotationLoadBalancerNameLength, s, err, LbNameMaxLength)
			nameLength = LbNameMaxLength
		}
	}
	if int64(len(ret)) > nameLength {
		ret = ret[:nameLength]
	}
	return strings.Trim(ret, "-")
}

// Return all the security groups that are tagged as being part of our cluster
func (c *Cloud) getTaggedSecurityGroups() (map[string]osc.SecurityGroup, error) {
	debugPrintCallerFunctionName()
	klog.V(5).Infof("getTaggedSecurityGroups()")
	request := osc.ReadSecurityGroupsRequest{
		Filters: &osc.FiltersSecurityGroup{
			TagKeys: &[]string{c.tagging.clusterTagKey()},
			Tags:    &[]string{fmt.Sprintf("%s%s=%s", TagNameMainSG, c.tagging.clusterID(), "True")},
		},
	}

	groups, err := c.compute.ReadSecurityGroups(&request)
	if err != nil {
		return nil, fmt.Errorf("error querying security groups: %q", err)
	}

	m := make(map[string]osc.SecurityGroup)
	for _, group := range groups {
		if !c.tagging.hasClusterTag(group.Tags) {
			continue
		}

		id := group.GetSecurityGroupId()
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
func (c *Cloud) updateInstanceSecurityGroupsForLoadBalancer(lb *elb.LoadBalancerDescription,
	instances map[InstanceID]*osc.Vm,
	securityGroupIDs []string) error {
	debugPrintCallerFunctionName()
	klog.V(5).Infof("updateInstanceSecurityGroupsForLoadBalancer(%v, %v, %v)", lb, instances, securityGroupIDs)

	if c.cfg.Global.DisableSecurityGroupIngress {
		return nil
	}

	// Determine the load balancer security group id
	loadBalancerSecurityGroupID := ""
	securityGroupsItem := []string{}
	if len(lb.SecurityGroups) > 0 {
		for _, securityGroup := range lb.SecurityGroups {
			securityGroupsItem = append(securityGroupsItem, *securityGroup)
		}
	} else if len(securityGroupIDs) > 0 {
		securityGroupsItem = securityGroupIDs
	}

	for _, securityGroup := range securityGroupsItem {
		if securityGroup == "" {
			continue
		}
		if loadBalancerSecurityGroupID != "" {
			// We create LBs with one SG
			klog.Warningf("Multiple security groups for load balancer: %q", aws.StringValue(lb.LoadBalancerName))
		}
		loadBalancerSecurityGroupID = securityGroup
	}

	if loadBalancerSecurityGroupID == "" {
		return fmt.Errorf("could not determine security group for load balancer: %s", aws.StringValue(lb.LoadBalancerName))
	}

	klog.V(5).Infof("loadBalancerSecurityGroupID(%v)", loadBalancerSecurityGroupID)

	// Get the actual list of groups that allow ingress from the load-balancer
	var actualGroups []osc.SecurityGroup
	{
		describeRequest := osc.ReadSecurityGroupsRequest{
			Filters: &osc.FiltersSecurityGroup{},
		}
		if loadBalancerSecurityGroupID != DefaultSrcSgName {
			describeRequest.Filters.InboundRuleSecurityGroupIds = &[]string{loadBalancerSecurityGroupID}
		} else {
			describeRequest.Filters.InboundRuleSecurityGroupNames = &[]string{loadBalancerSecurityGroupID}
		}

		response, err := c.compute.ReadSecurityGroups(&describeRequest)
		if err != nil {
			return fmt.Errorf("error querying security groups for ELB: %q", err)
		}
		for _, sg := range response {
			if !c.tagging.hasClusterTag(sg.Tags) {
				continue
			}
			actualGroups = append(actualGroups, sg)
		}
	}

	klog.V(5).Infof("actualGroups(%v)", actualGroups)

	taggedSecurityGroups, err := c.getTaggedSecurityGroups()
	if err != nil {
		return fmt.Errorf("error querying for tagged security groups: %q", err)
	}
	klog.V(5).Infof("taggedSecurityGroups(%v)", taggedSecurityGroups)

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

		if securityGroup == nil {
			klog.Warning("Ignoring instance without security group: ", instance.GetVmId())
			continue
		}
		id := securityGroup.GetSecurityGroupId()
		if id == "" {
			klog.Warningf("found security group without id: %v", securityGroup)
			continue
		}

		instanceSecurityGroupIds[id] = true
	}

	klog.V(5).Infof("instanceSecurityGroupIds(%v)", instanceSecurityGroupIds)

	// Compare to actual groups
	for _, actualGroup := range actualGroups {
		actualGroupID := actualGroup.GetSecurityGroupId()
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

	klog.V(5).Infof("instanceSecurityGroupIds(%v)", instanceSecurityGroupIds)
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
			sourceGroupID := osc.SecurityGroupsMember{
				SecurityGroupId: &loadBalancerSecurityGroupID,
			}

			allProtocols := "-1"
			toPort := int32(-1)
			fromPort := int32(-1)

			permission := osc.SecurityGroupRule{
				IpProtocol:            &allProtocols,
				SecurityGroupsMembers: &[]osc.SecurityGroupsMember{sourceGroupID},
				FromPortRange:         &fromPort,
				ToPortRange:           &toPort,
			}
			permissions = append(permissions, permission)
		}

		if add {
			changed, err := c.addSecurityGroupRules(instanceSecurityGroupID, &permissions, isPublicCloud)
			if err != nil {
				return err
			}
			if !changed {
				klog.Warning("Allowing ingress was not needed; concurrent change? groupId=", instanceSecurityGroupID)
			}
		} else {
			changed, err := c.removeSecurityGroupRules(instanceSecurityGroupID, &permissions, isPublicCloud)
			if err != nil {
				return err
			}
			if !changed {
				klog.Warning("Revoking ingress was not needed; concurrent change? groupId=", instanceSecurityGroupID)
			}
		}
	}

	return nil
}

// EnsureLoadBalancerDeleted implements LoadBalancer.EnsureLoadBalancerDeleted.
func (c *Cloud) EnsureLoadBalancerDeleted(ctx context.Context, clusterName string, service *v1.Service) error {
	debugPrintCallerFunctionName()
	klog.V(5).Infof("EnsureLoadBalancerDeleted(%v, %v)", clusterName, service)
	loadBalancerName := c.GetLoadBalancerName(ctx, clusterName, service)

	lb, err := c.describeLoadBalancer(loadBalancerName)
	if err != nil {
		return err
	}

	if lb == nil {
		klog.Info("Load balancer already deleted: ", loadBalancerName)
		return nil
	}

	loadBalancerSGs := []string{}
	if len(lb.SecurityGroups) == 0 && c.vpcID == "" {
		loadBalancerSGs = append(loadBalancerSGs, DefaultSrcSgName)
	} else {
		loadBalancerSGs = aws.StringValueSlice(lb.SecurityGroups)
	}

	{
		// De-register the load balancer security group from the instances security group
		err = c.ensureLoadBalancerInstances(aws.StringValue(lb.LoadBalancerName),
			lb.Instances,
			map[InstanceID]*osc.Vm{})
		if err != nil {
			klog.Errorf("ensureLoadBalancerInstances deregistering load balancer %v,%v,%v : %q",
				aws.StringValue(lb.LoadBalancerName),
				lb.Instances,
				nil, err)
		}

		// De-authorize the load balancer security group from the instances security group
		// Due to limit	tion of public cloud, we skip the deletion in the public cloud
		if c.vpcID != "" {
			err = c.updateInstanceSecurityGroupsForLoadBalancer(lb, nil, loadBalancerSGs)
			if err != nil {
				klog.Errorf("Error deregistering load balancer from instance security groups: %q", err)
				return err
			}
		} else {
			klog.V(2).Info("Ignore deletion of LoadBalancer SG rule in the Node SG in Public cloud")
		}
	}

	{
		// Delete the load balancer itself
		request := &elb.DeleteLoadBalancerInput{}
		request.LoadBalancerName = lb.LoadBalancerName

		_, err = c.loadBalancer.DeleteLoadBalancer(request)
		if err != nil {
			// TODO: Check if error was because load balancer was concurrently deleted
			klog.Errorf("Error deleting load balancer: %q", err)
			return err
		}
	}

	{
		// Delete the security group(s) for the load balancer
		// Note that this is annoying: the load balancer disappears from the API immediately, but it is still
		// deleting in the background.  We get a DependencyViolation until the load balancer has deleted itself

		describeRequest := osc.ReadSecurityGroupsRequest{
			Filters: &osc.FiltersSecurityGroup{
				SecurityGroupIds: &loadBalancerSGs,
			},
		}
		response, err := c.compute.ReadSecurityGroups(&describeRequest)
		if err != nil {
			return fmt.Errorf("error querying security groups for ELB: %q", err)
		}

		// Collect the security groups to delete
		securityGroupIDs := map[string]struct{}{}

		for _, sg := range response {
			sgID := sg.GetSecurityGroupId()

			if sgID == c.cfg.Global.ElbSecurityGroup {
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
				request := osc.DeleteSecurityGroupRequest{
					SecurityGroupId: &securityGroupID,
				}
				_, err := c.compute.DeleteSecurityGroup(&request)
				if err == nil {
					delete(securityGroupIDs, securityGroupID)
				} else {
					ignore := false
					if strings.Contains(err.Error(), "Conflict") {
						klog.V(2).Infof("Ignoring Conflict while deleting load-balancer security group (%s), assuming because LB is in process of deleting", securityGroupID)
						ignore = true
					}
					if !ignore {
						return fmt.Errorf("error while deleting load balancer security group (%s): %q", securityGroupID, err)
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

				return fmt.Errorf("timed out deleting ELB: %s. Could not delete security groups %v", service.Name, strings.Join(ids, ","))
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
	klog.V(5).Infof("UpdateLoadBalancer(%v, %v, %s)", clusterName, service, nodes)
	instances, err := c.findInstancesForELB(nodes)
	if err != nil {
		return err
	}

	loadBalancerName := c.GetLoadBalancerName(ctx, clusterName, service)
	lb, err := c.describeLoadBalancer(loadBalancerName)
	if err != nil {
		return err
	}

	if lb == nil {
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

	err = c.ensureLoadBalancerInstances(aws.StringValue(lb.LoadBalancerName), lb.Instances, instances)
	if err != nil {
		return nil
	}

	securityGroupsItem := []string{}
	if len(lb.SecurityGroups) == 0 && c.vpcID == "" {
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
func (c *Cloud) getInstanceByID(instanceID string) (*osc.Vm, error) {
	debugPrintCallerFunctionName()
	klog.V(5).Infof("getInstanceByID(%v)", instanceID)
	instances, err := c.getInstancesByIDs(&[]string{instanceID})
	if err != nil {
		return nil, err
	}

	if len(instances) == 0 {
		return nil, cloudprovider.InstanceNotFound
	}
	if len(instances) > 1 {
		return nil, fmt.Errorf("multiple instances found for instance: %s", instanceID)
	}

	return instances[instanceID], nil
}

func (c *Cloud) getInstancesByIDs(instanceIDs *[]string) (map[string]*osc.Vm, error) {
	debugPrintCallerFunctionName()
	klog.V(5).Infof("getInstancesByIDs(%v)", instanceIDs)

	instancesByID := make(map[string]*osc.Vm)
	if instanceIDs == nil || len(*instanceIDs) == 0 {
		return instancesByID, nil
	}

	request := &osc.ReadVmsRequest{
		Filters: &osc.FiltersVm{
			VmIds: instanceIDs,
		},
	}

	instances, err := c.compute.ReadVms(request)
	if err != nil {
		return nil, err
	}

	for _, instance := range instances {
		instanceRef := instance
		instanceID := instance.GetVmId()
		if instanceID == "" {
			continue
		}

		instancesByID[instanceID] = &instanceRef
	}

	return instancesByID, nil
}

func (c *Cloud) getInstancesByNodeNames(nodeNames []string, states ...string) ([]*osc.Vm, error) {
	debugPrintCallerFunctionName()
	klog.V(5).Infof("getInstancesByNodeNames(%v, %v)", nodeNames, states)

	names := nodeNames
	oscInstances := []*osc.Vm{}

	filters := osc.FiltersVm{
		VmStateNames: &[]string{
			"pending",
			"running",
			"stopping",
			"stopped",
			"shutting-down",
		},
	}

	instances, err := c.describeInstances(&filters)
	if err != nil {
		klog.V(2).Infof("Failed to describe instances %v", nodeNames)
		return nil, err
	}

	for _, instance := range instances {
		if Contains(names, instance.GetPrivateDnsName()) &&
			(len(states) == 0 || Contains(states, instance.GetState())) {
			oscInstances = append(oscInstances, instance)
		}
	}

	if len(oscInstances) == 0 {
		klog.V(3).Infof("Failed to find any instances %v", nodeNames)
		return nil, nil
	}
	return oscInstances, nil
}

// TODO: Move to instanceCache
func (c *Cloud) describeInstances(filters *osc.FiltersVm) ([]*osc.Vm, error) {
	debugPrintCallerFunctionName()
	klog.V(5).Infof("describeInstances(%v)", filters)

	request := &osc.ReadVmsRequest{
		Filters: filters,
	}

	response, err := c.compute.ReadVms(request)
	if err != nil {
		return nil, err
	}

	var matches []*osc.Vm
	for _, instance := range response {
		if c.tagging.hasClusterTag(instance.Tags) {
			instanceRef := instance
			matches = append(matches, &instanceRef)
		}
	}
	return matches, nil
}

// Returns the instance with the specified node name
// Returns nil if it does not exist
func (c *Cloud) findInstanceByNodeName(nodeName types.NodeName) (*osc.Vm, error) {
	debugPrintCallerFunctionName()
	klog.V(5).Infof("findInstanceByNodeName(%v)", nodeName)

	privateDNSName := mapNodeNameToPrivateDNSName(nodeName)
	filters := osc.FiltersVm{
		TagKeys: &[]string{
			c.tagging.clusterTagKey(),
		},
		Tags: &[]string{
			fmt.Sprintf("%s=%s", TagNameClusterNode, privateDNSName),
		},
		VmStateNames: &[]string{
			"pending",
			"running",
			"stopping",
			"stopped",
			"shutting-down",
		},
	}

	instances, err := c.describeInstances(&filters)

	if err != nil {
		return nil, err
	}

	if len(instances) == 0 {
		return nil, nil
	}
	if len(instances) > 1 {
		return nil, fmt.Errorf("multiple instances found for name: %s", nodeName)
	}

	if *instances[0].State == "terminated" {
		// We only want alive instances but oAPI does not have a filter for that
		return nil, nil
	}

	return instances[0], nil
}

// Returns the instance with the specified node name
// Like findInstanceByNodeName, but returns error if node not found
func (c *Cloud) getInstanceByNodeName(nodeName types.NodeName) (*osc.Vm, error) {
	debugPrintCallerFunctionName()
	klog.V(5).Infof("getInstanceByNodeName(%v)", nodeName)

	var instance *osc.Vm

	// we leverage node cache to try to retrieve node's provider id first, as
	// get instance by provider id is way more efficient than by filters in
	// aws context
	vmID, err := c.nodeNameToProviderID(nodeName)
	if err != nil {
		klog.V(3).Infof("Unable to convert node name %q to aws instanceID, fall back to findInstanceByNodeName: %v", nodeName, err)
		instance, err = c.findInstanceByNodeName(nodeName)
		// we need to set provider id for next calls

	} else {
		instance, err = c.getInstanceByID(string(vmID))
	}
	if err == nil && instance == nil {
		return nil, cloudprovider.InstanceNotFound
	}
	return instance, err
}

func (c *Cloud) nodeNameToProviderID(nodeName types.NodeName) (InstanceID, error) {
	debugPrintCallerFunctionName()
	klog.V(5).Infof("nodeNameToProviderID(%v)", nodeName)
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

	return KubernetesInstanceID(node.Spec.ProviderID).MapToAWSInstanceID()
}
