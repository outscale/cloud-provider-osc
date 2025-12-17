/*
SPDX-FileCopyrightText: 2025 Outscale SAS <opensource@outscale.com>

SPDX-License-Identifier: BSD-3-Clause
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
