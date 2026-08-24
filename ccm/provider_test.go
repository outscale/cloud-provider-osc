/*
SPDX-FileCopyrightText: 2025 Outscale SAS <opensource@outscale.com>

SPDX-License-Identifier: BSD-3-Clause
*/
package ccm_test

import (
	"testing"
)

func TestGarbageCollector(t *testing.T) {
	t.Run("If the load-balancer exists, delete it", func(t *testing.T) {
		c, oapimock, _ := newAPI(t, self, []string{"foo"})
		expectPurgeSecurityGroups(oapimock)
		c.RunGarbageCollector(t.Context())
	})
}
