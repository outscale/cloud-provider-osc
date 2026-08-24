/*
SPDX-FileCopyrightText: 2025 Outscale SAS <opensource@outscale.com>

SPDX-License-Identifier: BSD-3-Clause
*/
package main

import (
	"context"
	"io"
	"os"
	"os/signal"
	"sync"

	"github.com/outscale/cloud-provider-osc/ccm"
	cloudprovider "k8s.io/cloud-provider"
	"k8s.io/cloud-provider/app"
	cloudcontrollerconfig "k8s.io/cloud-provider/app/config"
	"k8s.io/cloud-provider/names"
	"k8s.io/cloud-provider/options"
	cliflag "k8s.io/component-base/cli/flag"
	_ "k8s.io/component-base/logs/json/register"
	_ "k8s.io/component-base/metrics/prometheus/clientgo" // for client metric registration
	_ "k8s.io/component-base/metrics/prometheus/version"  // for version metric registration
	"k8s.io/klog/v2"
)

func main() {
	klog.EnableContextualLogging(true)
	defer klog.Flush()

	opts, err := options.NewCloudControllerManagerOptions()
	if err != nil {
		klog.Fatalf("unable to initialize command options: %v", err)
	}

	controllerAliases := names.CCMControllerAliases()

	controllerInitializers := app.DefaultInitFuncConstructors

	fss := cliflag.NamedFlagSets{}
	ofss := fss.FlagSet("Outscale")
	oopts := ccm.Options{}
	oopts.AddFlags(ofss)

	// wg will store goroutines refs.
	// we will wait for all goroutines to stop before quitting.
	wg := sync.WaitGroup{}
	// catch interrupt signals
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	stopCh := make(chan struct{})
	go func() {
		<-c
		close(stopCh)
	}()

	command := app.NewCloudControllerManagerCommand(opts, cloudInitializer, controllerInitializers, controllerAliases, fss, stopCh)

	cloudprovider.RegisterCloudProvider(ccm.ProviderName, func(config io.Reader) (cloudprovider.Interface, error) {
		return ccm.NewProvider(context.Background(), oopts, &wg)
	})

	if err := command.Execute(); err != nil {
		os.Exit(1)
	}
	// wait for all goroutines to stop
	wg.Wait()
}

func cloudInitializer(config *cloudcontrollerconfig.CompletedConfig) cloudprovider.Interface {
	cloudConfig := config.ComponentConfig.KubeCloudShared.CloudProvider
	providerName := cloudConfig.Name

	if providerName == "" {
		providerName = ccm.ProviderName
	}

	cloud, err := cloudprovider.InitCloudProvider(providerName, cloudConfig.CloudConfigFile)
	if err != nil {
		klog.Fatalf("Cloud provider could not be initialized: %v", err)
	}
	if cloud == nil {
		klog.Fatalf("Cloud provider is nil")
	}

	if !cloud.HasClusterID() {
		if config.ComponentConfig.KubeCloudShared.AllowUntaggedCloud {
			klog.Warning("Detected a cluster without a ClusterID.  A ClusterID will be required in the future.  Please tag your cluster to avoid any future issues")
		} else {
			klog.Fatalf("no ClusterID found.  A ClusterID is required for the cloud provider to function properly.  This check can be bypassed by setting the allow-untagged-cloud option")
		}
	}

	return cloud
}
