// +build !providerless

/*
Copyright 2014 The Kubernetes Authors.

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
	"context"
	"fmt"

	"github.com/outscale/osc-sdk-go/osc"
	"k8s.io/klog"
	"github.com/antihax/optional"

	cloudprovider "k8s.io/cloud-provider"
)

func (c *Cloud) findRouteTable(ctx context.Context, clusterName string) (osc.RouteTable, error) {
	// This should be unnecessary (we already filter on TagNameKubernetesCluster,
	// and something is broken if cluster name doesn't match, but anyway...
	// TODO: All clouds should be cluster-aware by default
	var tables []osc.RouteTable

	if c.cfg.Global.RouteTableID != "" {
		request := &osc.ReadRouteTablesOpts{
            ReadRouteTablesRequest: optional.NewInterface(
                osc.ReadRouteTablesRequest{
                    Filters: osc.FiltersRouteTable{
                        RouteTableIds: []string{c.cfg.Global.RouteTableID},
                    },
                }),
        }
		response, httpRes, err := c.fcu.ReadRouteTables(ctx, request)
		if err != nil {
		    fmt.Errorf("http %q", httpRes)
			return osc.RouteTable{}, err
		}

		tables = response
	} else {
		request := &osc.ReadRouteTablesOpts{}
		response, httpRes, err := c.fcu.ReadRouteTables(ctx, request)
		if err != nil {
		    fmt.Errorf("http %q", httpRes)
			return osc.RouteTable{}, err
		}

		for _, table := range response {
			if c.tagging.hasClusterTag(table.Tags) {
				tables = append(tables, table)
			}
		}
	}

	if len(tables) == 0 {
		return osc.RouteTable{}, fmt.Errorf("unable to find route table for OSC cluster: %s", clusterName)
	}

	if len(tables) != 1 {
		return osc.RouteTable{}, fmt.Errorf("found multiple matching OSC route tables for OSC cluster: %s", clusterName)
	}
	return tables[0], nil
}

// ListRoutes implements Routes.ListRoutes
// List all routes that match the filter
func (c *Cloud) ListRoutes(ctx context.Context, clusterName string) ([]*cloudprovider.Route, error) {
	table, err := c.findRouteTable(ctx, clusterName)
	if err != nil {
		return nil, err
	}

	var routes []*cloudprovider.Route
	var instanceIDs []string

	for _, r := range table.Routes {
		instanceID := r.VmId

		if instanceID == "" {
			continue
		}

		instanceIDs = append(instanceIDs, instanceID)
	}

	instances, err := c.getInstancesByIDs(instanceIDs)
	if err != nil {
		return nil, err
	}

	for _, r := range table.Routes {
		destinationCIDR := r.DestinationIpRange
		if destinationCIDR == "" {
			continue
		}

		route := &cloudprovider.Route{
			Name:            clusterName + "-" + destinationCIDR,
			DestinationCIDR: destinationCIDR,
		}

		// Capture blackhole routes
		if r.State == "blackhole" {
			route.Blackhole = true
			routes = append(routes, route)
			continue
		}

		// Capture instance routes
		instanceID := r.VmId
		if instanceID != "" {
			instance, found := instances[instanceID]
			if found {
				route.TargetNode = mapInstanceToNodeName(instance)
				routes = append(routes, route)
			} else {
				klog.Warningf("unable to find instance ID %s in the list of instances being routed to", instanceID)
			}
		}
	}

	return routes, nil
}

// Sets the instance attribute "source-dest-check" to the specified value
func (c *Cloud) configureInstanceSourceDestCheck(ctx context.Context, instanceID string, sourceDestCheck bool) error {
	request := &osc.UpdateVmOpts{
        UpdateVmRequest: optional.NewInterface(
			osc.UpdateVmRequest{
                VmId: instanceID,
                IsSourceDestChecked: sourceDestCheck,
		}),
	}

	_, httpRes, err := c.fcu.UpdateVm(ctx, request)
	if err != nil {
		return fmt.Errorf("error configuring source-dest-check on instance %s: %q %q", instanceID, err, httpRes)
	}
	return nil
}

// CreateRoute implements Routes.CreateRoute
// Create the described route
func (c *Cloud) CreateRoute(ctx context.Context, clusterName string, nameHint string, route *cloudprovider.Route) error {
	instance, err := c.getInstanceByNodeName(route.TargetNode)
	if err != nil {
		return err
	}

	// In addition to configuring the route itself, we also need to configure the instance to accept that traffic
	// On OSC, this requires turning source-dest checks off
	err = c.configureInstanceSourceDestCheck(ctx, instance.VmId, false)
	if err != nil {
		return err
	}

	table, err := c.findRouteTable(ctx, clusterName)
	if err != nil {
		return err
	}

	var deleteRoute osc.Route
	for _, r := range table.Routes {
		destinationCIDR := r.DestinationIpRange

		if destinationCIDR != route.DestinationCIDR {
			continue
		}

		if r.State == "blackhole" {
			deleteRoute = r
		}
	}

	if deleteRoute != (osc.Route{}) {
		klog.Infof("deleting blackholed route: %s", deleteRoute.DestinationIpRange)

		request := &osc.DeleteRouteOpts{
            DeleteRouteRequest: optional.NewInterface(
                osc.DeleteRouteRequest{
                    DestinationIpRange: deleteRoute.DestinationIpRange,
                    RouteTableId: table.RouteTableId,
                }),
        }

		_, httpRes, errBlackhole := c.fcu.DeleteRoute(ctx, request)
		if err != nil {
			return fmt.Errorf("error deleting blackholed OSC route (%s): %q %q", deleteRoute.DestinationIpRange, errBlackhole, httpRes)
		}
	}

	// TODO: use ClientToken for idempotency?
	request := &osc.CreateRouteOpts{
            CreateRouteRequest: optional.NewInterface(
                osc.CreateRouteRequest{
                    VmId: instance.VmId,
                    DestinationIpRange: route.DestinationCIDR,
                    RouteTableId: table.RouteTableId,
                }),
        }

	_, httpRes, errRoute := c.fcu.CreateRoute(ctx, request)
	if err != nil {
		return fmt.Errorf("error creating OSC route (%s): %q %q", route.DestinationCIDR, errRoute, httpRes)
	}

	return nil
}

// DeleteRoute implements Routes.DeleteRoute
// Delete the specified route
func (c *Cloud) DeleteRoute(ctx context.Context, clusterName string, route *cloudprovider.Route) error {
	table, err := c.findRouteTable(ctx, clusterName)
	if err != nil {
		return err
	}

	request := &osc.DeleteRouteOpts{
            DeleteRouteRequest: optional.NewInterface(
                osc.DeleteRouteRequest{
                    DestinationIpRange: route.DestinationCIDR,
                    RouteTableId: table.RouteTableId,
                }),
    }

	_, httpRes, errDelete := c.fcu.DeleteRoute(ctx, request)
	if err != nil {
		return fmt.Errorf("error deleting OSC route (%s): %q %q", route.DestinationCIDR, errDelete, httpRes)
	}

	return nil
}
