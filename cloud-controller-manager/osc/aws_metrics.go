// +build !providerless

/*
Copyright 2017 The Kubernetes Authors.

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
	"sync"

	"github.com/prometheus/client_golang/prometheus"

	"k8s.io/component-base/metrics"
	"k8s.io/component-base/metrics/legacyregistry"
)

var (
	oscAPIMetric = metrics.NewHistogramVec(
		&metrics.HistogramOpts{
			Name:           "cloudprovider_osc_api_request_duration_seconds",
			Help:           "Latency of OSC API calls",
			StabilityLevel: metrics.ALPHA,
		},
		[]string{"request"})

	oscAPIErrorMetric = metrics.NewCounterVec(
		&metrics.CounterOpts{
			Name:           "cloudprovider_osc_api_request_errors",
			Help:           "OSC API errors",
			StabilityLevel: metrics.ALPHA,
		},
		[]string{"request"})

	oscAPIThrottlesMetric = metrics.NewCounterVec(
		&metrics.CounterOpts{
			Name:           "cloudprovider_osc_api_throttled_requests_total",
			Help:           "OSC API throttled requests",
			StabilityLevel: metrics.ALPHA,
		},
		[]string{"operation_name"})
)

func recordOSCMetric(actionName string, timeTaken float64, err error) {
	if err != nil {
		oscAPIErrorMetric.With(prometheus.Labels{"request": actionName}).Inc()
	} else {
		oscAPIMetric.With(prometheus.Labels{"request": actionName}).Observe(timeTaken)
	}
}

func recordOSCThrottlesMetric(operation string) {
	oscAPIThrottlesMetric.With(prometheus.Labels{"operation_name": operation}).Inc()
}

var registerOnce sync.Once

func registerMetrics() {
	registerOnce.Do(func() {
		legacyregistry.MustRegister(oscAPIMetric)
		legacyregistry.MustRegister(oscAPIErrorMetric)
		legacyregistry.MustRegister(oscAPIThrottlesMetric)
	})
}
