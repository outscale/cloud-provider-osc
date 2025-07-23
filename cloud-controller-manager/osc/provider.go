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
	"os"
	"time"

	"github.com/outscale/cloud-provider-osc/cloud-controller-manager/osc/cloud"
	"github.com/outscale/cloud-provider-osc/cloud-controller-manager/utils"
	"github.com/rs/xid"
	v1 "k8s.io/api/core/v1"
	informercorev1 "k8s.io/client-go/informers/core/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/record"
	cloudprovider "k8s.io/cloud-provider"
	"k8s.io/klog/v2"
)

// ProviderName is the name of this cloud provider.
const ProviderName = "osc"

// Provider is the Ouscale Cloud provider.
type Provider struct {
	cloud *cloud.Cloud

	// The vm instance that we are running on
	self *cloud.VM

	clientBuilder cloudprovider.ControllerClientBuilder
	kubeClient    clientset.Interface

	nodeInformer     informercorev1.NodeInformer
	eventBroadcaster record.EventBroadcaster
	eventRecorder    record.EventRecorder
}

// NewProvider builds a provider.
func NewProvider(ctx context.Context) (*Provider, error) {
	klog.V(2).Infof("Starting OSC cloud provider")

	c, err := cloud.New(ctx, os.Getenv(""))
	if err != nil {
		return nil, fmt.Errorf("init: %w", err)
	}

	self := c.Self
	klog.V(3).Infof("OSC CCM Instance (%s)", self.ID)
	klog.V(3).Infof("OSC CCM vpcID (%s)", self.NetID)
	return &Provider{
		cloud: c,
		self:  self,
	}, nil
}

func NewProviderWith(c *cloud.Cloud) *Provider {
	return &Provider{
		cloud: c,
		self:  c.Self,
	}
}

// Initialize passes a Kubernetes clientBuilder interface to the cloud provider
func (c *Provider) Initialize(clientBuilder cloudprovider.ControllerClientBuilder,
	_ <-chan struct{}) {
	c.clientBuilder = clientBuilder
	c.kubeClient = clientBuilder.ClientOrDie("osc-cloud-provider")
	c.eventBroadcaster = record.NewBroadcaster()
	c.eventBroadcaster.StartLogging(klog.Infof)
	c.eventBroadcaster.StartRecordingToSink(&v1core.EventSinkImpl{Interface: c.kubeClient.CoreV1().Events("")})
	c.eventRecorder = c.eventBroadcaster.NewRecorder(scheme.Scheme, v1.EventSource{Component: "osc-cloud-provider"})

	ctx := context.Background()
	c.cloud.Initialize(ctx, c.kubeClient)
	go c.garbageCollector(ctx)
}

// ProviderName returns the cloud provider ID.
func (c *Provider) ProviderName() string {
	return ProviderName
}

// LoadBalancer returns an implementation of LoadBalancer for Outscale.
func (c *Provider) LoadBalancer() (cloudprovider.LoadBalancer, bool) {
	return &logger{LoadBalancer: c}, true
}

// Instances returns an implementation of Instances for Outscale.
func (c *Provider) Instances() (cloudprovider.Instances, bool) {
	return &logger{Instances: c}, true
}

// InstancesV2 returns an implementation of InstancesV2 for Outscale.
func (c *Provider) InstancesV2() (cloudprovider.InstancesV2, bool) {
	return &logger{InstancesV2: c}, true
}

// Clusters is not implemented.
func (c *Provider) Clusters() (cloudprovider.Clusters, bool) {
	return nil, false
}

// Zones is not implemented.
func (c *Provider) Zones() (cloudprovider.Zones, bool) {
	return nil, false
}

// Routes is not implemented.
func (c *Provider) Routes() (cloudprovider.Routes, bool) {
	return nil, false
}

// HasClusterID returns true if the cluster has a clusterID
func (c *Provider) HasClusterID() bool {
	return c.self.ClusterID() != ""
}

func (c *Provider) garbageCollector(ctx context.Context) {
	logger := klog.Background().WithValues("version", utils.GetVersion(), "method", "GarbageCollector")
	logger.V(5).Info("Starting garbage collector")

	t := time.NewTicker(2 * time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			logger.V(5).Info("Collecting garbage")
			glogger := logger.WithValues("span_id", xid.New().String())
			ctx = klog.NewContext(ctx, glogger)
			err := c.cloud.RunGarbageCollector(ctx)
			if err != nil {
				logger.V(3).Error(err, "Error running garbage collector")
			}
		}
	}
}

var _ cloudprovider.Interface = (*Provider)(nil)
