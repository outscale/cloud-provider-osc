/*
SPDX-FileCopyrightText: 2025 Outscale SAS <opensource@outscale.com>

SPDX-License-Identifier: BSD-3-Clause
*/
package main

import (
	"context"
	"os"
	"os/signal"
	"runtime"

	"github.com/outscale/cloud-provider-osc/labeler"
	"github.com/spf13/pflag"
	"k8s.io/component-base/logs"
	logsv1 "k8s.io/component-base/logs/api/v1"
	_ "k8s.io/component-base/logs/json/register"
	"k8s.io/klog/v2"
)

func main() {
	fs := pflag.CommandLine
	logOptions := logs.NewOptions()
	logsv1.AddFlags(logOptions, fs)
	opts := Options{}
	opts.AddFlags(fs)
	pflag.Parse()

	ctx := context.Background()
	hostname := os.Getenv("NODE_NAME")

	err := labeler.SetLabels(ctx, hostname)
	if err != nil {
		klog.Error(err, "Unable to set labels")
		klog.Flush()
		os.Exit(1)
	}
	if opts.Wait {
		c := make(chan os.Signal, 1)
		signal.Notify(c)
		runtime.GC()
		<-c
	}
	klog.Flush()
}
