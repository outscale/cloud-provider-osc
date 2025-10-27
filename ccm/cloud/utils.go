/*
SPDX-FileCopyrightText: 2025 Outscale SAS <opensource@outscale.com>

SPDX-License-Identifier: BSD-3-Clause
*/
package cloud

import (
	"strings"

	"github.com/outscale/goutils/k8s/tags"
	"github.com/outscale/osc-sdk-go/v3/pkg/osc"
)

func countRoleTags(t []osc.ResourceTag) int {
	cnt := 0
	for i := range t {
		if strings.HasPrefix(t[i].Key, tags.RolePrefix) {
			cnt++
		}
	}
	return cnt
}
