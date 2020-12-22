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

	cloudprovider "k8s.io/cloud-provider"
)

func (c *Cloud) findRouteTable(clusterName string) (osc.RouteTable, error) {
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
		response, err := c.fcu.ReadRouteTables(request)
		if err != nil {
			return nil, err
		}

		tables = response
	} else {
		request := &osc.ReadRouteTablesOpts{}
		response, err := c.fcu.ReadRouteTables(request)
		if err != nil {
			return nil, err
		}

		for _, table := range response {
			if c.tagging.hasClusterTag(table.Tags) {
				tables = append(tables, table)
			}
		}
	}

	if len(tables) == 0 {
		return nil, fmt.Errorf("unable to find route table for AWS cluster: %s", clusterName)
	}

	if len(tables) != 1 {
		return nil, fmt.Errorf("found multiple matching AWS route tables for AWS cluster: %s", clusterName)
	}
	return tables[0], nil
}

// ListRoutes implements Routes.ListRoutes
// List all routes that match the filter
func (c *Cloud) ListRoutes(ctx context.Context, clusterName string) ([]*cloudprovider.Route, error) {
	table, err := c.findRouteTable(clusterName)
	if err != nil {
		return nil, err
	}

	var routes []*cloudprovider.Route
	var instanceIDs []string

	for _, r := range table.Routes {
		instanceID := r.InstanceId

		if instanceID == "" {
			continue
		}

		instanceIDs = append(instanceIDs, &instanceID)
	}

	instances, err := c.getInstancesByIDs(instanceIDs)
	if err != nil {
		return nil, err
	}

	for _, r := range table.Routes {
		destinationCIDR := r.DestinationCidrBlock
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
		instanceID := r.InstanceId
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
func (c *Cloud) configureInstanceSourceDestCheck(instanceID string, sourceDestCheck bool) error {
	request := &osc.UpdateVmOpts{}
	request.InstanceId = instanceID
	request.SourceDestCheck = &ec2.AttributeBooleanValue{Value: aws.Bool(sourceDestCheck)}

	_, err := c.fcu.UpdateVm(request)
	if err != nil {
		return fmt.Errorf("error configuring source-dest-check on instance %s: %q", instanceID, err)
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
	// On AWS, this requires turning source-dest checks off
	err = c.configureInstanceSourceDestCheck(instance.InstanceId, false)
	if err != nil {
		return err
	}

	table, err := c.findRouteTable(clusterName)
	if err != nil {
		return err
	}

	var deleteRoute osc.Route
	for _, r := range table.Routes {
		destinationCIDR := r.DestinationCidrBlock

		if destinationCIDR != route.DestinationCIDR {
			continue
		}

		if r.State == "blackhole" {
			deleteRoute = r
		}
	}

	if deleteRoute != nil {
		klog.Infof("deleting blackholed route: %s", aws.StringValue(deleteRoute.DestinationCidrBlock))

		request := osc.DeleteRouteOpts{
            DeleteRouteRequest: optional.NewInterface(
                osc.DeleteRouteRequest{
                    DestinationIpRange: deleteRoute.DestinationCidrBlock,
                    RouteTableId: table.RouteTableId,
                }),
        }

		_, err = c.fcu.DeleteRoute(ctx, request)
		if err != nil {
			return fmt.Errorf("error deleting blackholed AWS route (%s): %q", aws.StringValue(deleteRoute.DestinationCidrBlock), err)
		}
	}

	// TODO: use ClientToken for idempotency?
	request := osc.CreateRouteOpts{
            CreateRouteRequest: optional.NewInterface(
                osc.CreateRouteRequest{
                    VmId: instance.InstanceId,
                    DestinationIpRange: route.DestinationCIDR,
                    RouteTableId: table.RouteTableId,
                }),
        }

	_, err = c.fcu.CreateRoute(ctx, request)
	if err != nil {
		return fmt.Errorf("error creating AWS route (%s): %q", route.DestinationCIDR, err)
	}

	return nil
}

// DeleteRoute implements Routes.DeleteRoute
// Delete the specified route
func (c *Cloud) DeleteRoute(ctx context.Context, clusterName string, route *cloudprovider.Route) error {
	table, err := c.findRouteTable(clusterName)
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

	_, err = c.fcu.DeleteRoute(ctx, request)
	if err != nil {
		return fmt.Errorf("error deleting AWS route (%s): %q", route.DestinationCIDR, err)
	}

	return nil
}
