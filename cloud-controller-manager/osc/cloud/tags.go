package cloud

import (
	"strings"

	"github.com/aws/aws-sdk-go/service/elb"
)

const (
	// ClusterIDTagKeyPrefix is the tag key prefix we use to differentiate multiple
	// logically independent clusters running in the same AZ.
	// The tag key = ClusterIDTagKeyPrefix + clusterID
	// The tag value is an ownership value
	ClusterIDTagKeyPrefix = "OscK8sClusterID/"

	// SGToDeleteTagKey is a tag key that is added to all SG requiring to be deleted
	SGToDeleteTagKey = "OscK8sToDelete"

	// MainSGTagKeyPrefix The main sg Tag
	// The tag key = OscK8sMainSG/clusterId
	MainSGTagKeyPrefix = "OscK8sMainSG/"

	// RoleTagKeyPrefix is the prefix of tag key storing the role.
	RoleTagKeyPrefix = "OscK8sRole/"

	// ServiceNameTagKey is the tag key storing the service name.
	ServiceNameTagKey = "OscK8sService"
)

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

func getLBUClusterID(lbus *elb.DescribeTagsOutput) string {
	for _, lbu := range lbus.TagDescriptions {
		for _, t := range lbu.Tags {
			if t.Key != nil && strings.HasPrefix(*t.Key, ClusterIDTagKeyPrefix) {
				return strings.TrimPrefix(*t.Key, ClusterIDTagKeyPrefix)
			}
		}
	}
	return ""
}

func getLBUServiceName(lbus *elb.DescribeTagsOutput) string {
	for _, lbu := range lbus.TagDescriptions {
		for _, t := range lbu.Tags {
			if t.Key != nil && *t.Key == ServiceNameTagKey && t.Value != nil {
				return *t.Value
			}
		}
	}
	return ""
}

func clusterIDTagKey(clusterID string) string {
	return ClusterIDTagKeyPrefix + clusterID
}

func mainSGTagKey(clusterID string) string {
	return MainSGTagKeyPrefix + clusterID
}

func roleTagKey(role string) string {
	return RoleTagKeyPrefix + role
}
