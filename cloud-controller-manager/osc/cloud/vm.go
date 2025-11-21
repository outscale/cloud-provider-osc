package cloud

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/outscale/osc-sdk-go/v2"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	cloudprovider "k8s.io/cloud-provider"
)

// TagVmNodeName is the name of the Vm tag containing the node name.
const TagVmNodeName = "OscK8sNodeName"

// VM provide Virtual Machine representation
type VM struct {
	ID               string
	NodeName         types.NodeName
	AvailabilityZone string
	Region           string
	NetID            string
	SubnetID         string
	VmType           string

	cloudVm *osc.Vm
}

// FromOscVm creates a new awsInstance object
func FromOscVm(vm *osc.Vm) *VM {
	v := &VM{
		ID:       vm.GetVmId(),
		NodeName: mapInstanceToNodeName(vm),
		VmType:   vm.GetVmType(),
		NetID:    vm.GetNetId(),
		SubnetID: vm.GetSubnetId(),

		cloudVm: vm,
	}
	if vm.Placement != nil {
		v.AvailabilityZone = vm.Placement.GetSubregionName()
		v.Region = v.AvailabilityZone[:len(v.AvailabilityZone)-1]
	}
	return v
}

// NodeAddresses maps the instance information from OSC to an array of v1.NodeAddress
func (vm *VM) NodeAddresses() []v1.NodeAddress {
	if vm == nil {
		return nil
	}

	addresses := []v1.NodeAddress{}

	// handle internal network interfaces
	if len(vm.cloudVm.GetNics()) > 0 {
		for _, networkInterface := range vm.cloudVm.GetNics() {
			// skip network interfaces that are not currently in use
			if *networkInterface.State != "in-use" {
				continue
			}

			for _, internalIP := range networkInterface.GetPrivateIps() {
				if ipAddress := internalIP.GetPrivateIp(); ipAddress != "" {
					addresses = append(addresses, v1.NodeAddress{Type: v1.NodeInternalIP, Address: ipAddress})
				}
			}
		}
	} else {
		ipAddress := vm.cloudVm.GetPrivateIp()
		if ipAddress != "" {
			addresses = append(addresses, v1.NodeAddress{Type: v1.NodeInternalIP, Address: ipAddress})
		}
	}
	publicIPAddress := vm.cloudVm.GetPublicIp()
	if publicIPAddress != "" {
		addresses = append(addresses, v1.NodeAddress{Type: v1.NodeExternalIP, Address: publicIPAddress})
	}
	privateDNSName := aws.StringValue(vm.cloudVm.PrivateDnsName)
	if privateDNSName != "" {
		addresses = append(addresses, v1.NodeAddress{Type: v1.NodeInternalDNS, Address: privateDNSName})
		addresses = append(addresses, v1.NodeAddress{Type: v1.NodeHostName, Address: privateDNSName})
	}
	publicDNSName := aws.StringValue(vm.cloudVm.PublicDnsName)
	if publicDNSName != "" {
		addresses = append(addresses, v1.NodeAddress{Type: v1.NodeExternalDNS, Address: publicDNSName})
	}

	return addresses
}

func (vm *VM) IsStopped() bool {
	return vm.cloudVm.GetState() == "stopped"
}

func (vm *VM) IsTerminated() bool {
	return vm.cloudVm.GetState() == "terminated"
}

// InstanceID returns the instance ID
func (vm *VM) InstanceID() string {
	return "/" + vm.cloudVm.Placement.GetSubregionName() + "/" + vm.cloudVm.GetVmId()
}

func (vm *VM) ClusterID() string {
	for _, t := range vm.cloudVm.GetTags() {
		if strings.HasPrefix(t.Key, ClusterIDTagKeyPrefix) {
			return strings.TrimPrefix(t.Key, ClusterIDTagKeyPrefix)
		}
	}
	return ""
}

// ProviderID returns the provider ID of an instance which is ultimately set in the node.Spec.ProviderID field.
// The well-known format for a node's providerID is:
//   - aws:///<availability-zone>/<instance-id>
func (vm *VM) ProviderID() string {
	return "aws://" + vm.InstanceID()
}

// GetVMByNodeName returns the instance with the specified node name
// Like findInstanceByNodeName, but returns error if node not found
func (c *Cloud) GetVMByNodeName(ctx context.Context, nodeName string) (*VM, error) {
	vms, err := c.GetVMsByNodeName(ctx, nodeName)
	switch {
	case err != nil:
		return nil, err
	case len(vms) == 0:
		return nil, cloudprovider.InstanceNotFound
	default:
		return &vms[0], err
	}
}

// GetVMsByNodeName returns the instances with the specified node name
func (c *Cloud) GetVMsByNodeName(ctx context.Context, nodeNames ...string) ([]VM, error) {
	sdkVMs, err := c.api.OAPI().ReadVms(ctx, osc.ReadVmsRequest{
		Filters: &osc.FiltersVm{
			TagKeys: &[]string{clusterIDTagKey(c.clusterID)},
			VmStateNames: &[]string{
				"pending",
				"running",
				"stopping",
				"stopped",
				"shutting-down",
			},
		},
	})
	switch {
	case err != nil:
		return nil, fmt.Errorf("unable to find vms by node name: %w", err)
	case len(sdkVMs) == 0:
		return nil, nil
	}
	vms := make([]VM, 0, len(nodeNames))
	for _, nodeName := range nodeNames {
		for _, sdkVM := range sdkVMs {
			if hasTag(sdkVM.GetTags(), TagVmNodeName, nodeName) ||
				mapInstanceToNodeName(&sdkVM) == types.NodeName(nodeName) {
				vms = append(vms, *FromOscVm(&sdkVM))
			}
		}
	}
	return vms, nil
}

// GetVMByProviderID returns the instance with the specified provider id
// Like findInstanceByNodeName, but returns error if node not found
func (c *Cloud) GetVMByProviderID(ctx context.Context, providerID string) (*VM, error) {
	_, vmID, err := ParseProviderID(providerID)
	if err != nil {
		return nil, fmt.Errorf("GetVmByProviderID: %w", err)
	}
	return c.GetVMByID(ctx, vmID)
}

// GetVMsByProviderID returns the instances with the specified provider id
// Like findInstanceByNodeName, but returns error if node not found
func (c *Cloud) GetVMsByProviderID(ctx context.Context, providerIDs ...string) ([]VM, error) {
	ids := make([]string, 0, len(providerIDs))
	for _, providerID := range providerIDs {
		_, id, err := ParseProviderID(providerID)
		if err != nil {
			return nil, fmt.Errorf("GetVmByProviderID: %w", err)
		}
		ids = append(ids, id)
	}
	return c.GetVMsByID(ctx, ids...)
}

// GetVMByID returns the instance with the specified id
// Like findInstanceByNodeName, but returns error if node not found
func (c *Cloud) GetVMByID(ctx context.Context, vmID string) (*VM, error) {
	vms, err := c.GetVMsByID(ctx, vmID)
	switch {
	case err != nil:
		return nil, fmt.Errorf("unable to find vm by node name: %w", err)
	case len(vms) == 0:
		return nil, cloudprovider.InstanceNotFound
	default:
		return &vms[0], nil
	}
}

// GetVMsByID returns the instances with the specified id
// Like findInstanceByNodeName, but returns error if node not found
func (c *Cloud) GetVMsByID(ctx context.Context, vmIDs ...string) ([]VM, error) {
	sdkVMs, err := c.api.OAPI().ReadVms(ctx, osc.ReadVmsRequest{
		Filters: &osc.FiltersVm{
			VmIds: &vmIDs,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("unable to find vm by node name: %w", err)
	}
	vms := make([]VM, 0, len(sdkVMs))
	for _, sdkVM := range sdkVMs {
		vms = append(vms, *FromOscVm(&sdkVM))
	}
	return vms, nil
}

// mapInstanceToNodeName maps an OSC instance to a k8s NodeName, by extracting the PrivateDNSName
func mapInstanceToNodeName(i *osc.Vm) types.NodeName {
	return types.NodeName(aws.StringValue(i.PrivateDnsName))
}

func ParseProviderID(providerID string) (subregion, vmID string, err error) {
	if !strings.HasPrefix(providerID, "aws://") {
		// Build a URL with an empty host (AZ)
		providerID = "aws:////" + providerID
	}
	url, err := url.Parse(providerID)
	if err != nil {
		return "", "", fmt.Errorf("invalid provider id %q: %w", providerID, err)
	}
	if url.Scheme != "aws" && url.Scheme != "osc" {
		return "", "", fmt.Errorf("invalid provider id %q: %w", providerID, err)
	}

	tokens := strings.Split(url.Path, "/")
	if len(tokens) < 2 {
		return "", "", fmt.Errorf("invalid provider id %q", providerID)
	}

	vmID = tokens[len(tokens)-1]
	subregion = tokens[len(tokens)-2]

	return
}
