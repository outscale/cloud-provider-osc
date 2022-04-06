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
	"github.com/aws/aws-sdk-go/service/ec2"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
)

// ********************* CCM awsInstance Object & functions *********************

type awsInstance struct {
	ec2 Compute

	// id in AWS
	awsID string

	// node name in k8s
	nodeName types.NodeName

	// availability zone the instance resides in
	availabilityZone string

	// ID of VPC the instance resides in
	vpcID string

	// ID of subnet the instance resides in
	subnetID string

	// instance type
	instanceType string
}

// Gets the full information about this instance from the EC2 API
func (i *awsInstance) describeInstance() (*ec2.Instance, error) {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("describeInstance")
	return describeInstance(i.ec2, InstanceID(i.awsID))
}
