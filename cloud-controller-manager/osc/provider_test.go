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

package osc_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGarbageCollector(t *testing.T) {
	t.Run("If the load-balancer exists, delete it", func(t *testing.T) {
		c, oapimock, _ := newAPI(t, self, []string{"foo"})
		expectPurgeSecurityGroups(oapimock)
		err := c.RunGarbageCollector(context.TODO())
		require.NoError(t, err)
	})
}
