// +build !providerless

/*
Copyright 2017 The Kubernetes Authors.

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
	"testing"

	"github.com/outscale/osc-sdk-go/osc"


	//"github.com/aws/aws-sdk-go/aws"
	//"github.com/aws/aws-sdk-go/service/elb"
	"github.com/stretchr/testify/assert"
)

func TestLbuProtocolsAreEqual(t *testing.T) {
	grid := []struct {
		L        string
		R        string
		Expected bool
	}{
		{
			L:        "http",
			R:        "http",
			Expected: true,
		},
		{
			L:        "HTTP",
			R:        "http",
			Expected: true,
		},
		{
			L:        "HTTP",
			R:        "TCP",
			Expected: false,
		},
		{
			L:        "",
			R:        "TCP",
			Expected: false,
		},
		{
			L:        "",
			R:        "",
			Expected: true,
		},
// A verifier (string cannot be nill)
// 		{
// 			L:        nil,
// 			R:        "",
// 			Expected: false,
// 		},
// 		{
// 			L:        "",
// 			R:        nil,
// 			Expected: false,
// 		},
// 		{
// 			L:        nil,
// 			R:        nil,
// 			Expected: true,
// 		},
	}
	for _, g := range grid {
		actual := lbuProtocolsAreEqual(g.L, g.R)
		if actual != g.Expected {
			t.Errorf("unexpected result from protocolsEquals(%v, %v)", g.L, g.R)
		}
	}
}

func TestOSCARNEquals(t *testing.T) {
	grid := []struct {
		L        string
		R        string
		Expected bool
	}{
		{
			L:        "arn:aws:acm:us-east-1:123456789012:certificate/12345678-1234-1234-1234-123456789012",
			R:        "arn:aws:acm:us-east-1:123456789012:certificate/12345678-1234-1234-1234-123456789012",
			Expected: true,
		},
		{
			L:        "ARN:AWS:ACM:US-EAST-1:123456789012:CERTIFICATE/12345678-1234-1234-1234-123456789012",
			R:        "arn:aws:acm:us-east-1:123456789012:certificate/12345678-1234-1234-1234-123456789012",
			Expected: true,
		},
		{
			L:        "arn:aws:acm:us-east-1:123456789012:certificate/12345678-1234-1234-1234-123456789012",
			R:        "",
			Expected: false,
		},
		{
			L:        "",
			R:        "",
			Expected: true,
		},
// 		{
// 			L:        nil,
// 			R:        "",
// 			Expected: false,
// 		},
// 		{
// 			L:        "",
// 			R:        nil,
// 			Expected: false,
// 		},
// 		{
// 			L:        nil,
// 			R:        nil,
// 			Expected: true,
// 		},
	}
	for _, g := range grid {
		actual := oscArnEquals(g.L, g.R)
		if actual != g.Expected {
			t.Errorf("unexpected result from oscArnEquals(%v, %v)", g.L, g.R)
		}
	}
}

func TestSyncLbuListeners(t *testing.T) {
	tests := []struct {
		name                 string
		loadBalancerName     string
		listeners            []osc.ListenerForCreation
		listenerDescriptions []osc.Listener
		toCreate             []osc.ListenerForCreation
		toDelete             []int32
	}{
		{
			name:             "no edge cases",
			loadBalancerName: "lb_one",
			listeners: []osc.ListenerForCreation{
				{BackendPort: 443, BackendProtocol: "HTTP", LoadBalancerPort: 443, LoadBalancerProtocol: "HTTP", ServerCertificateId: "abc-123"},
				{BackendPort: 80, BackendProtocol: "TCP", LoadBalancerPort: 80, LoadBalancerProtocol: "TCP", ServerCertificateId: "def-456"},
				{BackendPort: 8443, BackendProtocol: "TCP", LoadBalancerPort: 8443, LoadBalancerProtocol: "TCP", ServerCertificateId: "def-456"},
			},
			listenerDescriptions: []osc.Listener{
				{BackendPort: 80, BackendProtocol: "TCP", LoadBalancerPort: 80, LoadBalancerProtocol: "TCP"},
				{BackendPort: 8443, BackendProtocol: "TCP", LoadBalancerPort: 8443, LoadBalancerProtocol: "TCP", ServerCertificateId: "def-456"},
			},
			toDelete: []int32{
				80,
			},
			toCreate: []osc.ListenerForCreation{
				{BackendPort: 443, BackendProtocol: "HTTP", LoadBalancerPort: 443, LoadBalancerProtocol: "HTTP", ServerCertificateId: "abc-123"},
				{BackendPort: 80, BackendProtocol: "TCP", LoadBalancerPort: 80, LoadBalancerProtocol: "TCP", ServerCertificateId: "def-456"},
			},
		},
		{
			name:             "no listeners to delete",
			loadBalancerName: "lb_two",
			listeners: []osc.ListenerForCreation{
				{BackendPort: 443, BackendProtocol: "HTTP", LoadBalancerPort: 443, LoadBalancerProtocol: "HTTP", ServerCertificateId: "abc-123"},
				{BackendPort: 80, BackendProtocol: "TCP", LoadBalancerPort: 80, LoadBalancerProtocol: "TCP", ServerCertificateId: "def-456"},
			},
			listenerDescriptions: []osc.Listener{
				{BackendPort: 443, BackendProtocol: "HTTP", LoadBalancerPort: 443, LoadBalancerProtocol: "HTTP", ServerCertificateId: "abc-123"},
			},
			toCreate: []osc.ListenerForCreation{
				{BackendPort: 80, BackendProtocol: "TCP", LoadBalancerPort: 80, LoadBalancerProtocol: "TCP", ServerCertificateId: "def-456"},
			},
			toDelete: []int32{},
		},
		{
			name:             "no listeners to create",
			loadBalancerName: "lb_three",
			listeners: []osc.ListenerForCreation{
				{BackendPort: 443, BackendProtocol: "HTTP", LoadBalancerPort: 443, LoadBalancerProtocol: "HTTP", ServerCertificateId: "abc-123"},
			},
			listenerDescriptions: []osc.Listener{
				{BackendPort: 80, BackendProtocol: "TCP", LoadBalancerPort: 80, LoadBalancerProtocol: "TCP"},
				{BackendPort: 443, BackendProtocol: "HTTP", LoadBalancerPort: 443, LoadBalancerProtocol: "HTTP", ServerCertificateId: "abc-123"},
			},
			toDelete: []int32{
				80,
			},
			toCreate: []osc.ListenerForCreation{},
		},
		{
			name:             "nil actual listener",
			loadBalancerName: "lb_four",
			listeners: []osc.ListenerForCreation{
				{BackendPort: 443, BackendProtocol: "HTTP", LoadBalancerPort: 443, LoadBalancerProtocol: "HTTP"},
			},
			listenerDescriptions: []osc.Listener{
				{BackendPort: 443, BackendProtocol: "HTTP", LoadBalancerPort: 443, LoadBalancerProtocol: "HTTP", ServerCertificateId: "abc-123"},
				{},
			},
			toDelete: []int32{
				443,
			},
			toCreate: []osc.ListenerForCreation{
				{BackendPort: 443, BackendProtocol: "HTTP", LoadBalancerPort: 443, LoadBalancerProtocol: "HTTP"},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			additions, removals := syncLbuListeners(test.loadBalancerName, test.listeners, test.listenerDescriptions)
			assert.Equal(t, additions, test.toCreate)
			assert.Equal(t, removals, test.toDelete)
		})
	}
}

func TestLbuListenersAreEqual(t *testing.T) {
	tests := []struct {
		name             string
		actual osc.ListenerForCreation
		expected osc.Listener
		equal            bool
	}{
		{
			name:     "should be equal",
			expected: osc.Listener{BackendPort: 80, BackendProtocol: "TCP", LoadBalancerPort: 80, LoadBalancerProtocol: "TCP"},
			actual:   osc.ListenerForCreation{BackendPort: 80, BackendProtocol: "TCP", LoadBalancerPort: 80, LoadBalancerProtocol: "TCP"},
			equal:    true,
		},
		{
			name:     "instance port should be different",
			expected: osc.Listener{BackendPort: 443, BackendProtocol: "TCP", LoadBalancerPort: 80, LoadBalancerProtocol: "TCP"},
			actual:   osc.ListenerForCreation{BackendPort: 80, BackendProtocol: "TCP", LoadBalancerPort: 80, LoadBalancerProtocol: "TCP"},
			equal:    false,
		},
		{
			name:     "instance loadBalancerProtocol should be different",
			expected: osc.Listener{BackendPort: 80, BackendProtocol: "HTTP", LoadBalancerPort: 80, LoadBalancerProtocol: "TCP"},
			actual:   osc.ListenerForCreation{BackendPort: 80, BackendProtocol: "TCP", LoadBalancerPort: 80, LoadBalancerProtocol: "TCP"},
			equal:    false,
		},
		{
			name:     "load balancer port should be different",
			expected: osc.Listener{BackendPort: 443, BackendProtocol: "TCP", LoadBalancerPort: 443, LoadBalancerProtocol: "TCP"},
			actual:   osc.ListenerForCreation{BackendPort: 443, BackendProtocol: "TCP", LoadBalancerPort: 80, LoadBalancerProtocol: "TCP"},
			equal:    false,
		},
		{
			name:     "loadBalancerProtocol should be different",
			expected: osc.Listener{BackendPort: 443, BackendProtocol: "TCP", LoadBalancerPort: 80, LoadBalancerProtocol: "TCP"},
			actual:   osc.ListenerForCreation{BackendPort: 443, BackendProtocol: "TCP", LoadBalancerPort: 80, LoadBalancerProtocol: "HTTP"},
			equal:    false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.equal, lbuListenersAreEqual(test.expected, test.actual))
		})
	}
}
