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
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/outscale/osc-sdk-go/v2"
	"k8s.io/klog/v2"

	"k8s.io/apimachinery/pkg/util/wait"
)

// TagNameKubernetesClusterPrefix is the tag name we use to differentiate multiple
// logically independent clusters running in the same AZ.
// The tag key = TagNameKubernetesClusterPrefix + clusterID
// The tag value is an ownership value
const TagNameKubernetesClusterPrefix = "OscK8sClusterID/"

// TagNameKubernetesClusterLegacy is the legacy tag name we use to differentiate multiple
// logically independent clusters running in the same AZ.  The problem with it was that it
// did not allow shared resources.
const TagNameKubernetesClusterLegacy = "project"

// ResourceLifecycle is the cluster lifecycle state used in tagging
type ResourceLifecycle string

const (
	// ResourceLifecycleOwned is the value we use when tagging resources to indicate
	// that the resource is considered owned and managed by the cluster,
	// and in particular that the lifecycle is tied to the lifecycle of the cluster.
	ResourceLifecycleOwned = "owned"
	// ResourceLifecycleShared is the value we use when tagging resources to indicate
	// that the resource is shared between multiple clusters, and should not be destroyed
	// if the cluster is destroyed.
	ResourceLifecycleShared = "shared"
)

type resourceTagging struct {
	// ClusterID is our cluster identifier: we tag AWS resources with this value,
	// and thus we can run two independent clusters in the same VPC or subnets.
	// This gives us similar functionality to GCE projects.
	ClusterID string

	// usesLegacyTags is true if we are using the legacy TagNameKubernetesClusterLegacy tags
	usesLegacyTags bool
}

func tagNameKubernetesCluster() string {
	debugPrintCallerFunctionName()
	klog.V(5).Infof("tagNameKubernetesCluster()")
	val, ok := os.LookupEnv("TAG_NAME_KUBERNETES_CLUSTER")
	if !ok {
		return TagNameKubernetesClusterLegacy
	}
	return val
}

// Extracts the legacy & new cluster ids from the given tags, if they are present
// If duplicate tags are found, returns an error
func findClusterIDs(tags *[]osc.ResourceTag) (string, string, error) {
	debugPrintCallerFunctionName()
	klog.V(5).Infof("findClusterIDs(%v)", tags)
	legacyClusterID := ""
	newClusterID := ""

	for _, tag := range *tags {
		tagKey := tag.GetKey()
		if strings.HasPrefix(tagKey, TagNameKubernetesClusterPrefix) {
			id := strings.TrimPrefix(tagKey, TagNameKubernetesClusterPrefix)
			if newClusterID != "" {
				return "", "", fmt.Errorf("Found multiple cluster tags with prefix %s (%q and %q)", TagNameKubernetesClusterPrefix, newClusterID, id)
			}
			newClusterID = id
		}

		if tagKey == tagNameKubernetesCluster() {
			id := tag.GetValue()
			if legacyClusterID != "" {
				return "", "", fmt.Errorf("Found multiple %s tags (%q and %q)", tagNameKubernetesCluster(), legacyClusterID, id)
			}
			legacyClusterID = id
		}
	}

	return legacyClusterID, newClusterID, nil
}

func (t *resourceTagging) init(legacyClusterID string, clusterID string) error {
	debugPrintCallerFunctionName()
	klog.V(5).Infof("init(%v,%v)", legacyClusterID, clusterID)
	if legacyClusterID != "" {
		if clusterID != "" && legacyClusterID != clusterID {
			return fmt.Errorf("clusterID tags did not match: %q vs %q", clusterID, legacyClusterID)
		}
		t.usesLegacyTags = true
		clusterID = legacyClusterID
	}

	t.ClusterID = clusterID

	if clusterID != "" {
		klog.Infof("AWS cloud filtering on ClusterID: %v", clusterID)
	} else {
		return fmt.Errorf("AWS cloud failed to find ClusterID")
	}

	return nil
}

// Extracts a clusterID from the given tags, if one is present
// If no clusterID is found, returns "", nil
// If multiple (different) clusterIDs are found, returns an error
func (t *resourceTagging) initFromTags(tags *[]osc.ResourceTag) error {
	debugPrintCallerFunctionName()
	klog.V(5).Infof("initFromTags(%v)", tags)
	legacyClusterID, newClusterID, err := findClusterIDs(tags)
	if err != nil {
		return err
	}

	if legacyClusterID == "" && newClusterID == "" {
		klog.Errorf("Tag %q nor %q not found; Kubernetes may behave unexpectedly.", tagNameKubernetesCluster(), TagNameKubernetesClusterPrefix+"...")
	}

	return t.init(legacyClusterID, newClusterID)
}

func (t *resourceTagging) clusterTagKey() string {
	debugPrintCallerFunctionName()
	klog.V(5).Infof("clusterTagKey()")
	return TagNameKubernetesClusterPrefix + t.ClusterID
}

// To delete after last call to this function
func (t *resourceTagging) hasClusterAWSTag(tags []*ec2.Tag) bool {
	debugPrintCallerFunctionName()
	klog.V(5).Infof("hasClusterAWSTag(%v)", tags)
	// if the clusterID is not configured -- we consider all instances.
	if len(t.ClusterID) == 0 {
		return true
	}
	clusterTagKey := t.clusterTagKey()
	for _, tag := range tags {
		if aws.StringValue(tag.Key) == clusterTagKey {
			return true
		}
	}
	return false
}

func (t *resourceTagging) hasClusterTag(tags *[]osc.ResourceTag) bool {
	debugPrintCallerFunctionName()
	klog.V(5).Infof("hasClusterTag(%v)", tags)
	// if the clusterID is not configured -- we consider all instances.
	if len(t.ClusterID) == 0 {
		return true
	}
	clusterTagKey := t.clusterTagKey()
	for _, tag := range *tags {
		if tag.GetKey() == clusterTagKey {
			return true
		}
	}
	return false
}

// Ensure that a resource has the correct tags
// If it has no tags, we assume that this was a problem caused by an error in between creation and tagging,
// and we add the tags.  If it has a different cluster's tags, that is an error.
func (t *resourceTagging) readRepairClusterTags(client Compute, resourceID string, lifecycle ResourceLifecycle, additionalTags map[string]string, observedTags *[]osc.ResourceTag) error {
	debugPrintCallerFunctionName()
	klog.V(5).Infof("readRepairClusterTags(%v, %v, %v, %v, %v)",
		client, resourceID, lifecycle, additionalTags, observedTags)
	actualTagMap := make(map[string]string)
	if observedTags == nil {
		return errors.New("Got an nil Tags")
	}
	for _, tag := range *observedTags {
		actualTagMap[tag.GetKey()] = tag.GetValue()
	}

	expectedTags := t.buildTags(lifecycle, additionalTags)

	addTags := make(map[string]string)
	for k, expected := range expectedTags {
		actual := actualTagMap[k]
		if actual == expected {
			continue
		}
		if actual == "" {
			klog.Warningf("Resource %q was missing expected cluster tag %q.  Will add (with value %q)", resourceID, k, expected)
			addTags[k] = expected
		} else {
			return fmt.Errorf("resource %q has tag belonging to another cluster: %q=%q (expected %q)", resourceID, k, actual, expected)
		}
	}

	if len(addTags) == 0 {
		return nil
	}

	if err := t.createTags(client, resourceID, lifecycle, addTags); err != nil {
		return fmt.Errorf("error adding missing tags to resource %q: %q", resourceID, err)
	}

	return nil
}

// createTags calls EC2 CreateTags, but adds retry-on-failure logic
// We retry mainly because if we create an object, we cannot tag it until it is "fully created" (eventual consistency)
// The error code varies though (depending on what we are tagging), so we simply retry on all errors
func (t *resourceTagging) createTags(client Compute, resourceID string, lifecycle ResourceLifecycle, additionalTags map[string]string) error {
	debugPrintCallerFunctionName()
	klog.V(5).Infof("createTags(%v,%v,%v,%v)", client, resourceID, lifecycle, additionalTags)

	tags := t.buildTags(lifecycle, additionalTags)

	if tags == nil || len(tags) == 0 {
		return nil
	}

	var oscTags []osc.ResourceTag
	for k, v := range tags {
		tag := osc.ResourceTag{
			Key:   k,
			Value: v,
		}
		oscTags = append(oscTags, tag)
	}

	backoff := wait.Backoff{
		Duration: createTagInitialDelay,
		Factor:   createTagFactor,
		Steps:    createTagSteps,
	}
	request := osc.CreateTagsRequest{
		ResourceIds: []string{
			resourceID,
		},
		Tags: oscTags,
	}
	var lastErr error
	err := wait.ExponentialBackoff(backoff, func() (bool, error) {
		_, err := client.CreateTags(&request)
		if err == nil {
			return true, nil
		}

		// We could check that the error is retryable, but the error code changes based on what we are tagging
		// SecurityGroup: InvalidGroup.NotFound
		klog.V(2).Infof("Failed to create tags; will retry.  Error was %q", err)
		lastErr = err
		return false, nil
	})
	if err == wait.ErrWaitTimeout {
		// return real CreateTags error instead of timeout
		err = lastErr
	}
	return err
}

// Add additional filters, to match on our tags
// This lets us run multiple k8s clusters in a single EC2 AZ
func (t *resourceTagging) addFilters(filters []*ec2.Filter) []*ec2.Filter {
	debugPrintCallerFunctionName()
	klog.V(5).Infof("addFilters(%v)", filters)
	// if there are no clusterID configured - no filtering by special tag names
	// should be applied to revert to legacy behaviour.
	if len(t.ClusterID) == 0 {
		if len(filters) == 0 {
			// We can't pass a zero-length Filters to AWS (it's an error)
			// So if we end up with no filters; just return nil
			return nil
		}
		return filters
	}

	f := newEc2Filter("tag-key", t.clusterTagKey())
	filters = append(filters, f)
	return filters
}

// Add additional filters, to match on our tags. This uses the tag for legacy
// 1.5 -> 1.6 clusters and exists for backwards compatibility
//
// This lets us run multiple k8s clusters in a single EC2 AZ
func (t *resourceTagging) addLegacyFilters(filters []*ec2.Filter) []*ec2.Filter {
	debugPrintCallerFunctionName()
	klog.V(5).Infof("addLegacyFilters(%v)", filters)
	// if there are no clusterID configured - no filtering by special tag names
	// should be applied to revert to legacy behaviour.
	if len(t.ClusterID) == 0 {
		if len(filters) == 0 {
			// We can't pass a zero-length Filters to AWS (it's an error)
			// So if we end up with no filters; just return nil
			return nil
		}
		return filters
	}

	f := newEc2Filter(fmt.Sprintf("tag:%s", tagNameKubernetesCluster()), t.ClusterID)

	// We can't pass a zero-length Filters to AWS (it's an error)
	// So if we end up with no filters; we need to return nil
	filters = append(filters, f)
	return filters
}

func (t *resourceTagging) buildTags(lifecycle ResourceLifecycle, additionalTags map[string]string) map[string]string {
	debugPrintCallerFunctionName()
	klog.V(5).Infof("buildTags(%v,%v)", lifecycle, additionalTags)
	tags := make(map[string]string)
	for k, v := range additionalTags {
		tags[k] = v
	}

	// no clusterID is a sign of misconfigured cluster, but we can't be tagging the resources with empty
	// strings
	if len(t.ClusterID) == 0 {
		return tags
	}

	// We only create legacy tags if we are using legacy tags, i.e. if we have seen a legacy tag on our instance
	if t.usesLegacyTags {
		tags[tagNameKubernetesCluster()] = t.ClusterID
	}
	tags[t.clusterTagKey()] = string(lifecycle)

	return tags
}

func (t *resourceTagging) clusterID() string {
	debugPrintCallerFunctionName()
	klog.V(5).Infof("clusterID()")
	return t.ClusterID
}
