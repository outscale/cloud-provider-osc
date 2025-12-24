/*
SPDX-FileCopyrightText: 2025 Outscale SAS <opensource@outscale.com>

SPDX-License-Identifier: BSD-3-Clause
*/
package oapi

import (
	"slices"
	"strings"

	"github.com/outscale/osc-sdk-go/v3/pkg/osc"
)

func IsSubnetPublic(id string, rtbls []osc.RouteTable) bool {
	for _, rt := range rtbls {
		for _, lnk := range rt.LinkRouteTables {
			if lnk.SubnetId == id {
				return slices.ContainsFunc(rt.Routes, func(r osc.Route) bool {
					return r.GatewayId != nil && strings.HasPrefix(*r.GatewayId, "igw-")
				})
			}
		}
	}
	return false
}
