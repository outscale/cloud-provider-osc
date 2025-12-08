package oapi

import (
	"slices"
	"strings"

	"github.com/outscale/osc-sdk-go/v2"
)

func IsSubnetPublic(id string, rtbls []osc.RouteTable) bool {
	for _, rt := range rtbls {
		for _, lnk := range rt.GetLinkRouteTables() {
			if lnk.GetSubnetId() == id {
				return slices.ContainsFunc(rt.GetRoutes(), func(r osc.Route) bool {
					return strings.HasPrefix(r.GetGatewayId(), "igw-")
				})
			}
		}
	}
	return false
}
