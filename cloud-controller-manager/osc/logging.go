/*
SPDX-FileCopyrightText: 2025 Outscale SAS <opensource@outscale.com>

SPDX-License-Identifier: BSD-3-Clause
*/
package osc

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/outscale/cloud-provider-osc/cloud-controller-manager/utils"
	"github.com/rs/xid"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/runtime"
	cloudprovider "k8s.io/cloud-provider"
	"k8s.io/klog/v2"
)

type logger struct {
	cloudprovider.Instances
	cloudprovider.InstancesV2
	cloudprovider.LoadBalancer
}

const callerPrefix = "github.com/outscale/cloud-provider-osc/cloud-controller-manager/osc.logger."

func method(caller string) string {
	return strings.TrimPrefix(caller, callerPrefix)
}

func log(ctx context.Context, fn func(ctx context.Context) error, attrs ...any) error {
	attrs = append(attrs, "span_id", xid.New().String(), "version", utils.GetVersion(), "method", method(runtime.GetCaller()))
	logger := klog.Background().WithValues(attrs...)
	ctx = klog.NewContext(ctx, logger)
	start := time.Now()
	err := fn(ctx)
	dur := time.Since(start)
	if err == nil {
		logger.V(2).Info("Success", "duration", dur)
	} else {
		logger.V(2).Error(err, "Failure", "duration", dur)
	}
	return err
}

func log1[T any](ctx context.Context, fn func(ctx context.Context) (T, error), attrs ...any) (T, error) {
	attrs = append(attrs, "span_id", xid.New().String(), "version", utils.GetVersion(), "method", method(runtime.GetCaller()))
	logger := klog.Background().WithValues(attrs...)
	ctx = klog.NewContext(ctx, logger)
	start := time.Now()
	res, err := fn(ctx)
	dur := time.Since(start)
	if err == nil {
		logger.V(2).Info("Success", "duration", dur)
		logger.V(5).Info(fmt.Sprintf("Response: %+v", res))
	} else {
		logger.V(2).Error(err, "Failure", "duration", dur)
	}
	return res, err
}

func log2[T, U any](ctx context.Context, fn func(ctx context.Context) (T, U, error), attrs ...any) (T, U, error) {
	attrs = append(attrs, "span_id", xid.New().String(), "version", utils.GetVersion(), "method", method(runtime.GetCaller()))
	logger := klog.Background().WithValues(attrs...)
	ctx = klog.NewContext(ctx, logger)
	start := time.Now()
	res1, res2, err := fn(ctx)
	dur := time.Since(start)
	if err == nil {
		logger.V(2).Info("Success", "duration", dur)
		logger.V(5).Info(fmt.Sprintf("Response: %+v", res1))
	} else {
		logger.V(2).Error(err, "Failure", "duration", dur)
	}
	return res1, res2, err
}

func (l logger) NodeAddresses(ctx context.Context, name types.NodeName) ([]v1.NodeAddress, error) {
	return log1(ctx, func(ctx context.Context) ([]v1.NodeAddress, error) {
		return l.Instances.NodeAddresses(ctx, name)
	}, "name", name)
}

func (l *logger) NodeAddressesByProviderID(ctx context.Context, providerID string) ([]v1.NodeAddress, error) {
	return log1(ctx, func(ctx context.Context) ([]v1.NodeAddress, error) {
		return l.Instances.NodeAddressesByProviderID(ctx, providerID)
	}, "providerID", providerID)
}

func (l *logger) InstanceID(ctx context.Context, nodeName types.NodeName) (string, error) {
	return log1(ctx, func(ctx context.Context) (string, error) {
		return l.Instances.InstanceID(ctx, nodeName)
	}, "nodeName", nodeName)
}

func (l *logger) InstanceTypeByProviderID(ctx context.Context, providerID string) (string, error) {
	return log1(ctx, func(ctx context.Context) (string, error) {
		return l.Instances.InstanceTypeByProviderID(ctx, providerID)
	}, "providerID", providerID)
}

func (l *logger) InstanceType(ctx context.Context, nodeName types.NodeName) (string, error) {
	return log1(ctx, func(ctx context.Context) (string, error) {
		return l.Instances.InstanceType(ctx, nodeName)
	}, "nodeName", nodeName)
}

func (l *logger) CurrentNodeName(ctx context.Context, hostname string) (types.NodeName, error) {
	return log1(ctx, func(ctx context.Context) (types.NodeName, error) {
		return l.Instances.CurrentNodeName(ctx, hostname)
	}, "hostname", hostname)
}

func (l *logger) InstanceExistsByProviderID(ctx context.Context, providerID string) (bool, error) {
	return log1(ctx, func(ctx context.Context) (bool, error) {
		return l.Instances.InstanceExistsByProviderID(ctx, providerID)
	}, "providerID", providerID)
}

func (l *logger) InstanceShutdownByProviderID(ctx context.Context, providerID string) (bool, error) {
	return log1(ctx, func(ctx context.Context) (bool, error) {
		return l.Instances.InstanceShutdownByProviderID(ctx, providerID)
	}, "providerID", providerID)
}

func (l *logger) AddSSHKeyToAllInstances(ctx context.Context, user string, keyData []byte) error {
	return log(ctx, func(ctx context.Context) error {
		return l.Instances.AddSSHKeyToAllInstances(ctx, user, keyData)
	}, "user", user)
}

func (l logger) InstanceExists(ctx context.Context, node *v1.Node) (bool, error) {
	return log1(ctx, func(ctx context.Context) (bool, error) {
		return l.InstancesV2.InstanceExists(ctx, node)
	}, "nodeName", node.Name)
}

func (l logger) InstanceShutdown(ctx context.Context, node *v1.Node) (bool, error) {
	return log1(ctx, func(ctx context.Context) (bool, error) {
		return l.InstancesV2.InstanceShutdown(ctx, node)
	}, "nodeName", node.Name)
}

func (l logger) InstanceMetadata(ctx context.Context, node *v1.Node) (*cloudprovider.InstanceMetadata, error) {
	return log1(ctx, func(ctx context.Context) (*cloudprovider.InstanceMetadata, error) {
		return l.InstancesV2.InstanceMetadata(ctx, node)
	}, "nodeName", node.Name)
}

func (l logger) GetLoadBalancer(ctx context.Context, clusterName string, svc *v1.Service) (*v1.LoadBalancerStatus, bool, error) {
	return log2(ctx, func(ctx context.Context) (*v1.LoadBalancerStatus, bool, error) {
		return l.LoadBalancer.GetLoadBalancer(ctx, clusterName, svc)
	}, "serviceName", svc.Name)
}

func (l logger) GetLoadBalancerName(ctx context.Context, clusterName string, svc *v1.Service) string {
	return l.LoadBalancer.GetLoadBalancerName(ctx, clusterName, svc)
}

func (l logger) EnsureLoadBalancer(ctx context.Context, clusterName string, svc *v1.Service,
	nodes []*v1.Node) (*v1.LoadBalancerStatus, error) {
	return log1(ctx, func(ctx context.Context) (*v1.LoadBalancerStatus, error) {
		return l.LoadBalancer.EnsureLoadBalancer(ctx, clusterName, svc, nodes)
	}, "serviceName", svc.Name, "nodes", utils.Map(nodes, func(n *v1.Node) (string, bool) { return n.Name, true }))
}

func (l logger) UpdateLoadBalancer(ctx context.Context, clusterName string, svc *v1.Service, nodes []*v1.Node) error {
	return log(ctx, func(ctx context.Context) error {
		return l.LoadBalancer.UpdateLoadBalancer(ctx, clusterName, svc, nodes)
	}, "serviceName", svc.Name, "nodes", utils.Map(nodes, func(n *v1.Node) (string, bool) { return n.Name, true }))
}

func (l logger) EnsureLoadBalancerDeleted(ctx context.Context, clusterName string, svc *v1.Service) error {
	return log(ctx, func(ctx context.Context) error {
		return l.LoadBalancer.EnsureLoadBalancerDeleted(ctx, clusterName, svc)
	}, "serviceName", svc.Name)
}
