//go:build !providerless
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
	"time"

	osc "github.com/outscale/osc-sdk-go/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
)

func TestMapToAWSInstanceIDs(t *testing.T) {
	tests := []struct {
		Kubernetes  KubernetesInstanceID
		Aws         InstanceID
		ExpectError bool
	}{
		{
			Kubernetes: "aws:///us-east-1a/i-12345678",
			Aws:        "i-12345678",
		},
		{
			Kubernetes: "aws:////i-12345678",
			Aws:        "i-12345678",
		},
		{
			Kubernetes: "i-12345678",
			Aws:        "i-12345678",
		},
		{
			Kubernetes: "aws:///us-east-1a/i-12345678abcdef01",
			Aws:        "i-12345678abcdef01",
		},
		{
			Kubernetes: "aws:////i-12345678abcdef01",
			Aws:        "i-12345678abcdef01",
		},
		{
			Kubernetes: "i-12345678abcdef01",
			Aws:        "i-12345678abcdef01",
		},
		{
			Kubernetes:  "vol-123456789",
			ExpectError: true,
		},
		{
			Kubernetes:  "aws:///us-east-1a/vol-12345678abcdef01",
			ExpectError: true,
		},
		{
			Kubernetes:  "aws://accountid/us-east-1a/vol-12345678abcdef01",
			ExpectError: true,
		},
		{
			Kubernetes:  "aws:///us-east-1a/vol-12345678abcdef01/suffix",
			ExpectError: true,
		},
		{
			Kubernetes:  "",
			ExpectError: true,
		},
	}

	for _, test := range tests {
		awsID, err := test.Kubernetes.MapToAWSInstanceID()
		if test.ExpectError {
			require.Error(t, err)
		} else {
			require.NoError(t, err)
			assert.Equal(t, test.Aws, awsID)
		}
	}

	for _, test := range tests {
		node := &v1.Node{}
		node.Spec.ProviderID = string(test.Kubernetes)

		awsInstanceIds, err := mapToAWSInstanceIDs([]*v1.Node{node})
		if test.ExpectError {
			require.Error(t, err)
		} else {
			require.NoError(t, err)
			require.Len(t, awsInstanceIds, 1)
			assert.Equal(t, test.Aws, awsInstanceIds[0])
		}

		awsInstanceIds = mapToAWSInstanceIDsTolerant([]*v1.Node{node})
		if test.ExpectError {
			require.Empty(t, awsInstanceIds)
		} else {
			require.Len(t, awsInstanceIds, 1)
			assert.Equal(t, test.Aws, awsInstanceIds[0])
		}
	}
}

func TestSnapshotMeetsCriteria(t *testing.T) {
	snapshot := &allInstancesSnapshot{timestamp: time.Now().Add(-3601 * time.Second)}

	assert.True(t, snapshot.MeetsCriteria(cacheCriteria{}),
		"Snapshot should always meet empty criteria")

	assert.False(t, snapshot.MeetsCriteria(cacheCriteria{MaxAge: time.Hour}),
		"Snapshot did not honor MaxAge")

	assert.False(t, snapshot.MeetsCriteria(cacheCriteria{HasInstances: []InstanceID{InstanceID("i-12345678")}}),
		"Snapshot did not honor HasInstances with missing instances")

	snapshot.instances = make(map[InstanceID]*osc.Vm)
	snapshot.instances[InstanceID("i-12345678")] = &osc.Vm{}

	assert.True(t, snapshot.MeetsCriteria(cacheCriteria{HasInstances: []InstanceID{InstanceID("i-12345678")}}),
		"Snapshot did not honor HasInstances with matching instances")

	assert.False(t, snapshot.MeetsCriteria(cacheCriteria{HasInstances: []InstanceID{InstanceID("i-12345678"), InstanceID("i-00000000")}}),
		"Snapshot did not honor HasInstances with partially matching instances")
}

func TestOlderThan(t *testing.T) {
	t1 := time.Now()
	t2 := t1.Add(time.Second)

	s1 := &allInstancesSnapshot{timestamp: t1}
	s2 := &allInstancesSnapshot{timestamp: t2}

	assert.True(t, s1.olderThan(s2), "s1 should be olderThan s2")
	assert.False(t, s2.olderThan(s1), "s2 not should be olderThan s1")
	assert.False(t, s1.olderThan(s1), "s1 not should be olderThan itself")
}

func TestSnapshotFindInstances(t *testing.T) {
	snapshot := &allInstancesSnapshot{}

	snapshot.instances = make(map[InstanceID]*osc.Vm)
	{
		id := InstanceID("i-12345678")
		idString := "i-12345678"
		snapshot.instances[id] = &osc.Vm{VmId: &idString}
	}
	{
		id := InstanceID("i-23456789")
		idString := "i-23456789"
		snapshot.instances[id] = &osc.Vm{VmId: &idString}
	}

	instances := snapshot.FindInstances([]InstanceID{InstanceID("i-12345678"), InstanceID("i-23456789"), InstanceID("i-00000000")})
	require.Len(t, instances, 2)

	for _, id := range []InstanceID{InstanceID("i-12345678"), InstanceID("i-23456789")} {
		i := instances[id]
		require.NotNil(t, i)
		assert.Equal(t, string(id), i.GetVmId())
		assert.Equal(t, snapshot.instances[id], i)
	}
}
