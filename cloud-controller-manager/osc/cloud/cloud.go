package cloud

import (
	"context"
	"fmt"
	"slices"

	"github.com/outscale/cloud-provider-osc/cloud-controller-manager/osc/oapi"
	"github.com/outscale/cloud-provider-osc/cloud-controller-manager/utils"
	"github.com/outscale/osc-sdk-go/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
)

type Cloud struct {
	api       oapi.Clienter
	Metadata  oapi.Metadata
	Self      *VM
	clusterID []string
}

func New(ctx context.Context, clusterID string) (*Cloud, error) {
	metadata, err := oapi.FetchMetadata()
	if err != nil {
		return nil, fmt.Errorf("init cloud: %w", err)
	}

	api, err := oapi.NewClient(metadata.Region)
	if err != nil {
		return nil, fmt.Errorf("init cloud: %w", err)
	}
	if err := api.OAPI().CheckCredentials(ctx); err != nil {
		return nil, fmt.Errorf("init cloud: %w", err)
	}
	c := &Cloud{api: api, Metadata: metadata}

	id := c.Metadata.InstanceID
	self, err := c.GetVMByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("error finding self: %w", err)
	}
	c.Self = self
	if clusterID != "" {
		c.clusterID = []string{clusterID}
	} else {
		// primary cluster ID (CAPOSC v1)
		c.clusterID = []string{self.ClusterID()}
		if self.SubnetID != "" {
			// alternate cluster ID (CAPOSC v0)
			subs, err := api.OAPI().ReadSubnets(ctx, osc.ReadSubnetsRequest{
				Filters: &osc.FiltersSubnet{SubnetIds: &[]string{self.SubnetID}},
			})
			if err != nil {
				return nil, fmt.Errorf("error finding self subnets: %w", err)
			}
			if len(subs) == 1 {
				clusterID := getClusterIDFromTags(subs[0].GetTags())
				if clusterID != "" && !slices.Contains(c.clusterID, clusterID) {
					c.clusterID = append(c.clusterID, clusterID)
				}
			}
		}
	}
	if len(c.clusterID) > 0 {
		klog.FromContext(ctx).V(3).Info(fmt.Sprintf("Allowed cluster IDs: %v", c.clusterID))
	}
	return c, nil
}

func NewWith(api oapi.Clienter, self *VM, clusterID []string) *Cloud {
	return &Cloud{
		api: api,
		Metadata: oapi.Metadata{
			InstanceID:       self.ID,
			Region:           self.Region,
			AvailabilityZone: self.AvailabilityZone,
		},
		Self:      self,
		clusterID: clusterID,
	}
}

func (c *Cloud) Initialize(ctx context.Context, kube clientset.Interface) {
	if len(c.clusterID) > 0 {
		return
	}
	logger := klog.FromContext(ctx)
	logger.V(5).Info("Inferring cluster ID from kube-system ns")
	ns, err := kube.CoreV1().Namespaces().Get(ctx, "kube-system", metav1.GetOptions{})
	if err != nil {
		logger.V(3).Error(err, "Unable to infer cluster ID")
		return
	}
	c.clusterID = []string{string(ns.UID)}
	logger.V(3).Info("Inferred cluster ID: " + string(ns.UID))
}

func (c *Cloud) sameCluster(tags []osc.ResourceTag) bool {
	return slices.Contains(c.clusterID, getClusterIDFromTags(tags))
}

func (c *Cloud) mainSGTagKey() string {
	if len(c.clusterID) == 0 {
		return ""
	}
	return mainSGTagKey(c.clusterID[0])
}

func (c *Cloud) clusterIDTagKey() string {
	if len(c.clusterID) == 0 {
		return ""
	}
	return clusterIDTagKey(c.clusterID[0])
}

func (c *Cloud) clusterIDTagKeys() []string {
	return utils.Map(c.clusterID, func(c string) (string, bool) {
		return clusterIDTagKey(c), true
	})
}
