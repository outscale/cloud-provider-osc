package cloud

import (
	"context"
	"fmt"

	"github.com/outscale/cloud-provider-osc/cloud-controller-manager/osc/oapi"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
)

type Cloud struct {
	api       oapi.Clienter
	Metadata  oapi.Metadata
	Self      *VM
	clusterID string
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
	c := &Cloud{api: api, Metadata: metadata, clusterID: clusterID}

	id := c.Metadata.InstanceID
	self, err := c.GetVMByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("error finding self: %w", err)
	}
	c.Self = self
	if c.clusterID == "" {
		c.clusterID = self.ClusterID()
	}
	return c, nil
}

func NewWith(api oapi.Clienter, self *VM, clusterID string) *Cloud {
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
	if c.clusterID != "" {
		return
	}
	logger := klog.FromContext(ctx)
	logger.V(5).Info("Inferring cluster ID from kube-system ns")
	ns, err := kube.CoreV1().Namespaces().Get(ctx, "kube-system", metav1.GetOptions{})
	if err != nil {
		logger.V(3).Error(err, "Unable to infer cluster ID")
		return
	}
	c.clusterID = string(ns.UID)
	logger.V(3).Info("Inferred cluster ID: " + c.clusterID)
}
