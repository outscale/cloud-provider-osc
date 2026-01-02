/*
SPDX-FileCopyrightText: 2025 Outscale SAS <opensource@outscale.com>

SPDX-License-Identifier: BSD-3-Clause
*/
package cloud

import (
	"context"
	"fmt"
	"slices"

	"github.com/outscale/cloud-provider-osc/ccm/oapi"
	"github.com/outscale/goutils/k8s/sdk"
	"github.com/outscale/goutils/k8s/tags"
	"github.com/outscale/goutils/sdk/metadata"
	"github.com/outscale/osc-sdk-go/v3/pkg/osc"
	"github.com/samber/lo"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
)

type Cloud struct {
	api       oapi.Clienter
	Self      *VM
	clusterID []string
}

func New(ctx context.Context, clusterID string, opts ...sdk.Options) (*Cloud, error) {
	api, err := oapi.NewClient(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("init cloud: %w", err)
	}
	c := &Cloud{api: api}

	id, err := metadata.GetInstanceID(ctx)
	if err != nil {
		return nil, fmt.Errorf("get metadata: %w", err)
	}
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
		if self.SubnetID != nil {
			// alternate cluster ID (CAPOSC v0)
			subs, err := api.OAPI().ReadSubnets(ctx, osc.ReadSubnetsRequest{
				Filters: &osc.FiltersSubnet{SubnetIds: &[]string{*self.SubnetID}},
			})
			if err != nil {
				return nil, fmt.Errorf("error finding self subnets: %w", err)
			}
			if len(*subs.Subnets) == 1 {
				clusterID := tags.GetClusterID((*subs.Subnets)[0].Tags)
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
		api:       api,
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

func (c *Cloud) sameCluster(t []osc.ResourceTag) bool {
	return slices.Contains(c.clusterID, tags.GetClusterID(t))
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
	return tags.ClusterIDKey(c.clusterID[0])
}

func (c *Cloud) clusterIDTagKeys() []string {
	return lo.Map(c.clusterID, func(c string, _ int) string {
		return tags.ClusterIDKey(c)
	})
}
