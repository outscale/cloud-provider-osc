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
	"io"

	"github.com/aws/aws-sdk-go/aws/credentials"
	"gopkg.in/gcfg.v1"

	"k8s.io/cloud-provider"
	"k8s.io/klog"
)

// ********************* CCM Object Init *********************

var _ cloudprovider.Interface = (*Cloud)(nil)
var _ cloudprovider.Instances = (*Cloud)(nil)
var _ cloudprovider.LoadBalancer = (*Cloud)(nil)
var _ cloudprovider.Routes = (*Cloud)(nil)
var _ cloudprovider.Zones = (*Cloud)(nil)

// ********************* CCM entry point function *********************

// readOSCCloudConfig reads an instance of OSCCloudConfig from config reader.
func readOSCCloudConfig(config io.Reader) (*CloudConfig, error) {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("readOSCCloudConfig(%v)", config)
	var cfg CloudConfig
	var err error

	if config != nil {
		err = gcfg.ReadInto(&cfg, config)
		if err != nil {
			return nil, err
		}
	}

	return &cfg, nil
}

// newOSCCloud creates a new instance of OSCCloud.
// OSCProvider and instanceId are primarily for tests
func newOSCCloud(cfg CloudConfig, oscServices Services) (*Cloud, error) {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("newOSCCloud(%v,%v)", cfg, oscServices)
	// We have some state in the Cloud object - in particular the attaching map
	// Log so that if we are building multiple Cloud objects, it is obvious!
	klog.Infof("Starting OSC cloud provider")

	metadata, err := oscServices.Metadata()
	if err != nil {
		return nil, fmt.Errorf("error creating OSC metadata client: %q", err)
	}

	err = updateConfigZone(&cfg, metadata)
	if err != nil {
		return nil, fmt.Errorf("unable to determine OSC zone from cloud provider config or OSC instance metadata: %v", err)
	}
	zone := cfg.Global.Zone
	if len(zone) <= 1 {
		return nil, fmt.Errorf("invalid OSC zone in config file: %s", zone)
	}
	regionName, err := azToRegion(zone)
	if err != nil {
		return nil, err
	}

	if !cfg.Global.DisableStrictZoneCheck {
		if !isRegionValid(regionName, metadata) {
			return nil, fmt.Errorf("not a valid OSC zone (unknown region): %s", zone)
		}
	} else {
		klog.Warningf("Strict OSC zone checking is disabled.  Proceeding with zone: %s", zone)
	}

	klog.Infof("OSC CCM cfg.Global: %v", cfg.Global)
	klog.Infof("OSC CCM cfg: %v", cfg)

	klog.Infof("Init Services/Compute")
	fcu, err := oscServices.Compute(regionName)
	if err != nil {
		return nil, fmt.Errorf("error creating OSC FCU client: %v", err)
	}
	klog.Infof("Init Services/LoadBalancing")
	lbu, err := oscServices.LoadBalancing(regionName)
	if err != nil {
		return nil, fmt.Errorf("error creating OSC LBU client: %v", err)
	}

	oscCloud := &Cloud{
		fcu:      fcu,
		lbu:      lbu,
		metadata: metadata,
		cfg:      &cfg,
		region:   regionName,
	}
	oscCloud.instanceCache.cloud = oscCloud

//     klog.Infof("newOSCCloud osccloud %v", oscCloud)
//     klog.Infof("newOSCCloud osccloud.cfg %v", oscCloud.cfg)
//     klog.Infof("newOSCCloud cfg %v", cfg)


	tagged := cfg.Global.KubernetesClusterTag != "" || cfg.Global.KubernetesClusterID != ""

// 	klog.Infof("newOSCCloud tagged %v", tagged)
//
//     klog.Infof("Inside if cfg.Global.VPC %v", cfg.Global.VPC)
//     klog.Infof("cfg.Global.SubnetID %v", cfg.Global.SubnetID)
//     klog.Infof("cfg.Global.RoleARN %v ", cfg.Global.RoleARN)
//     klog.Infof("tagged %v", tagged)
	if cfg.Global.VPC != "" && (cfg.Global.SubnetID != "" || cfg.Global.RoleARN != "") && tagged {

		// When the master is running on a different OSC account, cloud provider or on-premise
		// build up a dummy instance and use the VPC from the nodes account
// 		klog.Info("Master is configured to run on a different osc account, different cloud provider or on-premises")
		oscCloud.selfOSCInstance = &oscInstance{
			nodeName: "master-dummy",
			netID:    cfg.Global.VPC,
			subnetID: cfg.Global.SubnetID,
		}
		oscCloud.netID = cfg.Global.VPC
	} else {
		selfOSCInstance, err := oscCloud.buildSelfOSCInstance()
		if err != nil {
			return nil, err
		}
		oscCloud.selfOSCInstance = selfOSCInstance
		oscCloud.netID = selfOSCInstance.netID
		klog.Infof("OSC CCM Instance (%v)", selfOSCInstance)
		klog.Infof("OSC CCM netID (%v)", selfOSCInstance.netID)

	}

	if cfg.Global.KubernetesClusterTag != "" || cfg.Global.KubernetesClusterID != "" {
// 	    klog.Infof("newOSCCloud cfg.Global.KubernetesClusterTag cfg.Global.KubernetesClusterID %v %v", cfg.Global.KubernetesClusterTag, cfg.Global.KubernetesClusterID)
		if err := oscCloud.tagging.init(cfg.Global.KubernetesClusterTag, cfg.Global.KubernetesClusterID); err != nil {
// 		    klog.Infof("Inside if osccloud.tagging.init")
			return nil, err
		}
	} else {
	    klog.Infof("Inside else")
		// TODO: Clean up double-API query
		info, err := oscCloud.selfOSCInstance.describeInstance()
// 		klog.Infof("after oscCloud.selfOSCInstance.describeInstance %v %q", info, err)
		if err != nil {
			return nil, err
		}
		if err := oscCloud.tagging.initFromTags(info.Tags); err != nil {
// 		    klog.Infof(" if osccloud.tagging.initfromtags %v", err)
			return nil, err
		}
	}
	klog.Infof("OSC CCM oscCloud %v", oscCloud)
	return oscCloud, nil
}

func init() {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("init()")
	registerMetrics()
	cloudprovider.RegisterCloudProvider(ProviderName, func(config io.Reader) (cloudprovider.Interface, error) {
		cfg, err := readOSCCloudConfig(config)
		if err != nil {
			return nil, fmt.Errorf("unable to read OSC cloud provider config file: %v", err)
		}

		if err = cfg.validateOverrides(); err != nil {
			return nil, fmt.Errorf("unable to validate custom endpoint overrides: %v", err)
		}

		provider := []credentials.Provider{
			&credentials.EnvProvider{},
			&credentials.SharedCredentialsProvider{},
		}

		creds := credentials.NewChainCredentials(provider)

		osc := newOSCSDKProvider(creds, cfg)
		return newOSCCloud(*cfg, osc)
	})
}
