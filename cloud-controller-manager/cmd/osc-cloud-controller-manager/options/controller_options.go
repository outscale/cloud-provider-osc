/*
Copyright 2020 The Kubernetes Authors.

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

package options

import (
	"flag"

	cliflag "k8s.io/component-base/cli/flag"
)

// ControllerOptions contains options and configuration settings for the controller service.
type ControllerOptions struct {
	// ExtraVolumeTags is a map of tags that will be attached to each dynamically provisioned
	// volume.
	ExtraVolumeTags map[string]string
}

func (s *ControllerOptions) AddFlags(fs *flag.FlagSet) {
	fs.Var(cliflag.NewMapStringString(&s.ExtraVolumeTags), "extra-volume-tags", "Extra volume tags to attach to each dynamically provisioned volume. It is a comma separated list of key value pairs like '<key1>=<value1>,<key2>=<value2>'")
}

// Validate checks for errors from user input
func ValidateExtraVolumeTags(tags map[string]string) error {
	if len(tags) > cloud.MaxNumTagsPerResource {
		return fmt.Errorf("Too many volume tags (actual: %d, limit: %d)", len(tags), cloud.MaxNumTagsPerResource)
	}

	for k, v := range tags {
		if len(k) > cloud.MaxTagKeyLength {
			return fmt.Errorf("Volume tag key too long (actual: %d, limit: %d)", len(k), cloud.MaxTagKeyLength)
		}
		if len(v) > cloud.MaxTagValueLength {
			return fmt.Errorf("Volume tag value too long (actual: %d, limit: %d)", len(v), cloud.MaxTagValueLength)
		}
		if k == cloud.VolumeNameTagKey {
			return fmt.Errorf("Volume tag key '%s' is reserved", cloud.VolumeNameTagKey)
		}
		if strings.HasPrefix(k, cloud.KubernetesTagKeyPrefix) {
			return fmt.Errorf("Volume tag key prefix '%s' is reserved", cloud.KubernetesTagKeyPrefix)
		}
		if strings.HasPrefix(k, cloud.OscTagKeyPrefix) {
			return fmt.Errorf("Volume tag key prefix '%s' is reserved", cloud.OscTagKeyPrefix)
		}
	}

	return nil
}
