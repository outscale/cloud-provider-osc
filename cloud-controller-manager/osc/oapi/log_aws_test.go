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

package oapi

import (
	"testing"

	"github.com/aws/aws-sdk-go/service/elb"
	"github.com/stretchr/testify/assert"
	"k8s.io/utils/ptr"
)

func TestCleanAWS(t *testing.T) {
	src := &elb.RegisterInstancesWithLoadBalancerInput{
		LoadBalancerName: ptr.To("lb-foo"),
		Instances: []*elb.Instance{{
			InstanceId: ptr.To("i-foo"),
		}},
	}
	cleaned := cleanAws(src)
	assert.Equal(t, "{Instances:[{InstanceId:i-foo}],LoadBalancerName:lb-foo}", cleaned)
}
