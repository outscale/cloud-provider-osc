/*
SPDX-FileCopyrightText: 2025 Outscale SAS <opensource@outscale.com>

SPDX-License-Identifier: BSD-3-Clause
*/
package cloud

import (
	"slices"
	"strings"

	"github.com/outscale/osc-sdk-go/v2"
)

func hasTag(tags []osc.ResourceTag, k string, v ...string) bool {
	return slices.ContainsFunc(tags, func(t osc.ResourceTag) bool {
		return t.Key == k && (len(v) == 0 || t.Value == v[0])
	})
}

func countRoleTags(tags []osc.ResourceTag) int {
	cnt := 0
	for i := range tags {
		if strings.HasPrefix(tags[i].Key, RoleTagKeyPrefix) {
			cnt++
		}
	}
	return cnt
}
