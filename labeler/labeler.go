/*
SPDX-FileCopyrightText: 2025 Outscale SAS <opensource@outscale.com>

SPDX-License-Identifier: BSD-3-Clause
*/
package labeler

import (
	"context"
	"errors"
	"fmt"

	"github.com/outscale/goutils/sdk/metadata"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	cloudnodeutil "k8s.io/cloud-provider/node/helpers"
	"k8s.io/klog/v2"
)

const (
	ClusterLabel = "topology.outscale.com/cluster"
	ServerLabel  = "topology.outscale.com/server"
)

func SetLabels(ctx context.Context, name string) error {
	config, err := rest.InClusterConfig() // in cluster config
	if err != nil {
		return err
	}
	k8s, err := kubernetes.NewForConfig(config)
	if err != nil {
		return err
	}
	api := k8s.CoreV1().Nodes()
	node, err := api.Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("fetch nodes: %w", err)
	}
	labels := map[string]string{}
	cluster, err := metadata.GetPlacementCluster(ctx)
	if err != nil {
		return fmt.Errorf("fetch cluster metadata: %w", err)
	}
	klog.Info("cluster: " + cluster)
	if cluster != node.Labels[ClusterLabel] {
		labels[ClusterLabel] = cluster
	}
	server, err := metadata.GetPlacementServer(ctx)
	if err != nil {
		return fmt.Errorf("fetch server metadata: %w", err)
	}
	klog.Info("server: " + server)
	if server != node.Labels[ServerLabel] {
		labels[ServerLabel] = server
	}
	if len(labels) == 0 {
		klog.Info("Labels are OK")
	}

	if !cloudnodeutil.AddOrUpdateLabelsOnNode(k8s, labels, node) {
		return errors.New("unable to update labels")
	}
	return nil
}
