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

// 	"github.com/aws/aws-sdk-go/aws"
// 	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/stretchr/testify/assert"

    "github.com/outscale/osc-sdk-go/osc"

	"k8s.io/api/core/v1"
)

func TestMapToOSCInstanceIDs(t *testing.T) {
	tests := []struct {
		Kubernetes  KubernetesInstanceID
		Osc         InstanceID
		ExpectError bool
	}{
		{
			Kubernetes: "aws:///us-east-1a/i-12345678",
			Osc:        "i-12345678",
		},
		{
			Kubernetes: "aws:////i-12345678",
			Osc:        "i-12345678",
		},
		{
			Kubernetes: "i-12345678",
			Osc:        "i-12345678",
		},
		{
			Kubernetes: "aws:///us-east-1a/i-12345678abcdef01",
			Osc:        "i-12345678abcdef01",
		},
		{
			Kubernetes: "aws:////i-12345678abcdef01",
			Osc:        "i-12345678abcdef01",
		},
		{
			Kubernetes: "i-12345678abcdef01",
			Osc:        "i-12345678abcdef01",
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
		oscID, err := test.Kubernetes.MapToOSCInstanceID()
		if err != nil {
			if !test.ExpectError {
				t.Errorf("unexpected error parsing %s: %v", test.Kubernetes, err)
			}
		} else {
			if test.ExpectError {
				t.Errorf("expected error parsing %s", test.Kubernetes)
			} else if test.Osc != oscID {
				t.Errorf("unexpected value parsing %s, got %s", test.Kubernetes, oscID)
			}
		}
	}

	for _, test := range tests {
		node := &v1.Node{}
		node.Spec.ProviderID = string(test.Kubernetes)

		oscInstanceIds, err := mapToOSCInstanceIDs([]*v1.Node{node})
		if err != nil {
			if !test.ExpectError {
				t.Errorf("unexpected error parsing %s: %v", test.Kubernetes, err)
			}
		} else {
			if test.ExpectError {
				t.Errorf("expected error parsing %s", test.Kubernetes)
			} else if len(oscInstanceIds) != 1 {
				t.Errorf("unexpected value parsing %s, got %s", test.Kubernetes, oscInstanceIds)
			} else if oscInstanceIds[0] != test.Osc {
				t.Errorf("unexpected value parsing %s, got %s", test.Kubernetes, oscInstanceIds)
			}
		}

		oscInstanceIds = mapToOSCInstanceIDsTolerant([]*v1.Node{node})
		if test.ExpectError {
			if len(oscInstanceIds) != 0 {
				t.Errorf("unexpected results parsing %s: %s", test.Kubernetes, oscInstanceIds)
			}
		} else {
			if len(oscInstanceIds) != 1 {
				t.Errorf("unexpected value parsing %s, got %s", test.Kubernetes, oscInstanceIds)
			} else if oscInstanceIds[0] != test.Osc {
				t.Errorf("unexpected value parsing %s, got %s", test.Kubernetes, oscInstanceIds)
			}
		}
	}
}

func TestSnapshotMeetsCriteria(t *testing.T) {
	snapshot := &allInstancesSnapshot{timestamp: time.Now().Add(-3601 * time.Second)}

	if !snapshot.MeetsCriteria(cacheCriteria{}) {
		t.Errorf("Snapshot should always meet empty criteria")
	}

	if snapshot.MeetsCriteria(cacheCriteria{MaxAge: time.Hour}) {
		t.Errorf("Snapshot did not honor MaxAge")
	}

	if snapshot.MeetsCriteria(cacheCriteria{HasInstances: []InstanceID{InstanceID("i-12345678")}}) {
		t.Errorf("Snapshot did not honor HasInstances with missing instances")
	}

	snapshot.instances = make(map[InstanceID]osc.Vm)
	snapshot.instances[InstanceID("i-12345678")] = osc.Vm{}

	if !snapshot.MeetsCriteria(cacheCriteria{HasInstances: []InstanceID{InstanceID("i-12345678")}}) {
		t.Errorf("Snapshot did not honor HasInstances with matching instances")
	}

	if snapshot.MeetsCriteria(cacheCriteria{HasInstances: []InstanceID{InstanceID("i-12345678"), InstanceID("i-00000000")}}) {
		t.Errorf("Snapshot did not honor HasInstances with partially matching instances")
	}
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

	snapshot.instances = make(map[InstanceID]osc.Vm)
	{
		id := InstanceID("i-12345678")
		snapshot.instances[id] = osc.Vm{VmId: id.oscString()}
	}
	{
		id := InstanceID("i-23456789")
		snapshot.instances[id] = osc.Vm{VmId: id.oscString()}
	}

	instances := snapshot.FindInstances([]InstanceID{InstanceID("i-12345678"), InstanceID("i-23456789"), InstanceID("i-00000000")})
	if len(instances) != 2 {
		t.Errorf("findInstances returned %d results, expected 2", len(instances))
	}

	for _, id := range []InstanceID{InstanceID("i-12345678"), InstanceID("i-23456789")} {
		i := instances[id]
		if i.VmId == "" {
			t.Errorf("findInstances did not return %s", id)
			continue
		}
		if i.VmId != string(id) {
			t.Errorf("findInstances did not return expected instanceId for %s", id)
		}
		if i.VmId != snapshot.instances[id].VmId {
			t.Errorf("findInstances did not return expected instance (reference equality) for %s", id)
		}
	}
}
