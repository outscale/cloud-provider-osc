/*
Copyright 2018 The Kubernetes Authors.

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

package e2e

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2" //nolint: staticcheck
	"github.com/onsi/gomega"
	"github.com/outscale/cloud-provider-osc/cloud-controller-manager/osc/cloud"
	"github.com/outscale/cloud-provider-osc/cloud-controller-manager/osc/oapi"
	e2eutils "github.com/outscale/cloud-provider-osc/tests/e2e/utils"
	"github.com/outscale/osc-sdk-go/v2"
	"github.com/rs/xid"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/test/e2e/framework"
	e2esvc "k8s.io/kubernetes/test/e2e/framework/service"
	admissionapi "k8s.io/pod-security-admission/api"
	"k8s.io/utils/ptr"
)

const (
	testTimeout = 10 * time.Minute
)

var _ = Describe("[e2e][loadbalancer][fast] Creating a load-balancer", func() {
	f := framework.NewDefaultFramework("ccm")
	f.NamespacePodSecurityEnforceLevel = admissionapi.LevelPrivileged

	var (
		cs     clientset.Interface
		ns     *v1.Namespace
		ctx    context.Context
		lbName string
		oapi   *oapi.Client
	)

	BeforeEach(func() {
		cs = f.ClientSet
		ns = f.Namespace
		ctx = context.Background()
		lbName = "ccm-simple-" + xid.New().String()
		var err error
		oapi, err = e2eutils.OAPI()
		framework.ExpectNoError(err)
	})

	Context("When creating a simple service", func() {
		var (
			deployment *appsv1.Deployment
			svc        *v1.Service
		)
		BeforeEach(func() {
			deployment = e2eutils.CreateDeployment(ctx, cs, ns, 1, []v1.ContainerPort{
				{
					Name:          "tcp",
					Protocol:      v1.ProtocolTCP,
					ContainerPort: 8080,
				},
			})
			e2eutils.WaitForDeploymentReady(ctx, cs, deployment)

			svc = e2eutils.CreateSvc(ctx, cs, ns, map[string]string{
				"service.beta.kubernetes.io/osc-load-balancer-name": lbName,
			}, []v1.ServicePort{
				{
					Name:       "tcp",
					Protocol:   v1.ProtocolTCP,
					TargetPort: intstr.FromInt(8080),
					Port:       80,
				},
			}, nil)
		})

		AfterEach(func() {
			if deployment != nil {
				e2eutils.DeleteDeployment(ctx, cs, deployment)
				deployment = nil
			}
			if svc != nil {
				e2eutils.DeleteSvc(ctx, cs, svc)
				svc = nil
			}
		})

		It("can connect to the load-balancer", func() {
			svc = e2eutils.WaitForSvc(ctx, cs, svc)
			e2esvc.TestReachableHTTP(ctx, svc.Status.LoadBalancer.Ingress[0].Hostname, 80, testTimeout)
		})

		It("sets tags from cli args and annotations in addition to the standard ones", func() {
			e2eutils.ExpectLoadBalancerTags(ctx, oapi, lbName, gomega.And(
				gomega.ContainElement(osc.ResourceTag{Key: "annotationkey", Value: "annotationvalue"}),
				gomega.ContainElement(osc.ResourceTag{Key: "clikey", Value: "clivalue"}),
				gomega.ContainElement(osc.ResourceTag{Key: cloud.ServiceNameTagKey, Value: svc.Name}),
				gomega.ContainElement(gomega.HaveField("Key", gomega.HavePrefix(cloud.ClusterIDTagKeyPrefix))),
			))
		})

		It("can update the port", func() {
			svc = e2eutils.WaitForSvc(ctx, cs, svc)
			_, err := e2esvc.UpdateService(ctx, cs, svc.Namespace, svc.Name, func(d *v1.Service) {
				d.Spec.Ports[0].Port = 8080
			})
			framework.ExpectNoError(err)
			e2esvc.TestReachableHTTP(ctx, svc.Status.LoadBalancer.Ingress[0].Hostname, 8080, testTimeout)
		})
	})
})

var _ = Describe("[e2e][loadbalancer] Checking proxy-protocol", func() {
	f := framework.NewDefaultFramework("ccm")
	f.NamespacePodSecurityEnforceLevel = admissionapi.LevelPrivileged

	var (
		cs   clientset.Interface
		ns   *v1.Namespace
		ctx  context.Context
		oapi *oapi.Client
	)

	BeforeEach(func() {
		cs = f.ClientSet
		ns = f.Namespace
		ctx = context.Background()
		var err error
		oapi, err = e2eutils.OAPI()
		framework.ExpectNoError(err)
	})

	It("can create a service using proxy protocol", func() {
		deployment := e2eutils.CreateProxyProtocolDeployment(ctx, cs, ns, 1, []v1.ContainerPort{
			{
				Name:          "tcp",
				Protocol:      v1.ProtocolTCP,
				ContainerPort: 8080,
			},
		})
		defer e2eutils.DeleteDeployment(ctx, cs, deployment)
		e2eutils.WaitForDeploymentReady(ctx, cs, deployment)

		lbName := "ccm-proxyprotocol-" + xid.New().String()
		svc := e2eutils.CreateSvc(ctx, cs, ns, map[string]string{
			"service.beta.kubernetes.io/osc-load-balancer-proxy-protocol":   "*",
			"service.beta.kubernetes.io/osc-load-balancer-backend-protocol": "http",
			"service.beta.kubernetes.io/osc-load-balancer-name":             lbName,
		}, []v1.ServicePort{
			{
				Name:       "tcp",
				Protocol:   v1.ProtocolTCP,
				TargetPort: intstr.FromInt(8080),
				Port:       80,
			},
		}, nil)
		svc = e2eutils.WaitForSvc(ctx, cs, svc)
		e2eutils.ExpectLoadBalancer(ctx, oapi, lbName, gomega.HaveField("BackendServerDescriptions",
			gomega.ContainElement(gomega.HaveField("PolicyNames", gomega.ContainElement(ptr.To("k8s-proxyprotocol-enabled")))),
		))
		e2esvc.TestReachableHTTP(ctx, svc.Status.LoadBalancer.Ingress[0].Hostname, 80, testTimeout)
		e2eutils.DeleteSvc(ctx, cs, svc)
	})
})

var _ = Describe("[e2e][loadbalancer] Checking cleanup of resources", func() {
	f := framework.NewDefaultFramework("ccm")
	f.NamespacePodSecurityEnforceLevel = admissionapi.LevelPrivileged

	var (
		cs   clientset.Interface
		ns   *v1.Namespace
		ctx  context.Context
		oapi *oapi.Client
	)

	BeforeEach(func() {
		cs = f.ClientSet
		ns = f.Namespace
		ctx = context.Background()
		var err error
		oapi, err = e2eutils.OAPI()
		framework.ExpectNoError(err)
	})

	It("deletes the security group and the load-balancer", func() {
		deployment := e2eutils.CreateDeployment(ctx, cs, ns, 1, []v1.ContainerPort{
			{
				Name:          "tcp",
				Protocol:      v1.ProtocolTCP,
				ContainerPort: 8080,
			},
		})
		e2eutils.WaitForDeploymentReady(ctx, cs, deployment)

		lbName := "ccm-cleanup-" + xid.New().String()
		svc := e2eutils.CreateSvc(ctx, cs, ns, map[string]string{
			"service.beta.kubernetes.io/osc-load-balancer-name": lbName,
		}, []v1.ServicePort{
			{
				Name:       "tcp",
				Protocol:   v1.ProtocolTCP,
				TargetPort: intstr.FromInt(8080),
				Port:       80,
			},
		}, nil)

		svc = e2eutils.WaitForSvc(ctx, cs, svc)
		e2esvc.TestReachableHTTP(ctx, svc.Status.LoadBalancer.Ingress[0].Hostname, 80, testTimeout)
		lb, err := e2eutils.GetLoadBalancer(ctx, oapi, lbName)
		framework.ExpectNoError(err)
		e2eutils.DeleteDeployment(ctx, cs, deployment)
		e2eutils.DeleteSvc(ctx, cs, svc)
		By("Checking that load-balancer & security groups have been deleted")
		e2eutils.ExpectNoLoadBalancer(ctx, oapi, lbName)
		e2eutils.ExpectSecurityGroups(ctx, oapi, lb, gomega.BeEmpty())
	})
})

var _ = Describe("[e2e][loadbalancer] Updating backends", func() {
	f := framework.NewDefaultFramework("ccm")
	f.NamespacePodSecurityEnforceLevel = admissionapi.LevelPrivileged

	var (
		cs   clientset.Interface
		ns   *v1.Namespace
		ctx  context.Context
		oapi *oapi.Client
	)

	BeforeEach(func() {
		cs = f.ClientSet
		ns = f.Namespace
		ctx = context.Background()
		var err error
		oapi, err = e2eutils.OAPI()
		framework.ExpectNoError(err)
	})

	It("updates backends when a node is deleted", func() {
		deployment := e2eutils.CreateDeployment(ctx, cs, ns, 1, []v1.ContainerPort{
			{
				Name:          "tcp",
				Protocol:      v1.ProtocolTCP,
				ContainerPort: 8080,
			},
		})

		defer e2eutils.DeleteDeployment(ctx, cs, deployment)
		e2eutils.WaitForDeploymentReady(ctx, cs, deployment)

		lbName := "ccm-backends-" + xid.New().String()
		svc := e2eutils.CreateSvc(ctx, cs, ns, map[string]string{
			"service.beta.kubernetes.io/osc-load-balancer-name": lbName,
		}, []v1.ServicePort{
			{
				Name:       "tcp",
				Protocol:   v1.ProtocolTCP,
				TargetPort: intstr.FromInt(8080),
				Port:       80,
			},
		}, nil)
		defer e2eutils.DeleteSvc(ctx, cs, svc)
		svc = e2eutils.WaitForSvc(ctx, cs, svc)
		e2esvc.TestReachableHTTP(ctx, svc.Status.LoadBalancer.Ingress[0].Hostname, 80, testTimeout)
		lb, err := e2eutils.GetLoadBalancer(ctx, oapi, lbName)
		framework.ExpectNoError(err)
		e2eutils.DeleteWorkerNode(ctx, cs)
		By("Waiting until LB backends have changed")
		e2eutils.ExpectLoadBalancer(ctx, oapi, lbName, gomega.Not(gomega.HaveField("Instances", lb.Instances)))
		e2esvc.TestReachableHTTP(ctx, svc.Status.LoadBalancer.Ingress[0].Hostname, 80, testTimeout)
	})
})
