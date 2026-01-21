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
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2" //nolint: staticcheck
	"github.com/onsi/gomega"
	"github.com/outscale/cloud-provider-osc/ccm/oapi"
	e2eutils "github.com/outscale/cloud-provider-osc/tests/e2e/utils"
	"github.com/outscale/goutils/k8s/sdk"
	"github.com/outscale/goutils/k8s/tags"
	"github.com/outscale/goutils/sdk/ptr"
	"github.com/outscale/osc-sdk-go/v3/pkg/osc"
	"github.com/rs/xid"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/test/e2e/framework"
	e2esvc "k8s.io/kubernetes/test/e2e/framework/service"
	admissionapi "k8s.io/pod-security-admission/api"
)

const (
	testTimeout = 10 * time.Minute
)

var _ = Describe("[e2e][loadbalancer][fast] Creating a load-balancer", func() {
	f := framework.NewDefaultFramework("ccm")
	f.NamespacePodSecurityEnforceLevel = admissionapi.LevelPrivileged

	var (
		cs  clientset.Interface
		ns  *v1.Namespace
		ctx context.Context
		api *oapi.Client
	)

	BeforeEach(func() {
		cs = f.ClientSet
		ns = f.Namespace
		ctx = context.Background()
		var err error
		api, err = oapi.NewClient(ctx)
		framework.ExpectNoError(err)
	})

	Context("When creating a simple service", func() {
		var (
			lbName     string
			deployment *appsv1.Deployment
			svc        *v1.Service
		)
		BeforeEach(func() {
			lbName = "ccm-simple-" + xid.New().String()
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

		It("ingress is configured and can connect to the load-balancer", func() {
			svc = e2eutils.WaitForSvc(ctx, cs, svc)
			gomega.Expect(svc.Status.LoadBalancer.Ingress, gomega.Not(gomega.BeEmpty()))
			gomega.Expect(svc.Status.LoadBalancer.Ingress[0].Hostname, gomega.Not(gomega.BeEmpty()))
			gomega.Expect(svc.Status.LoadBalancer.Ingress[0].IP, gomega.Not(gomega.BeEmpty()))
			e2esvc.TestReachableHTTP(ctx, svc.Status.LoadBalancer.Ingress[0].Hostname, 80, testTimeout)

			By("Checking tags from cli args and annotations in addition to the standard ones")
			e2eutils.ExpectLoadBalancerTags(ctx, api, lbName, gomega.And(
				gomega.ContainElement(osc.ResourceTag{Key: "annotationkey", Value: "annotationvalue"}),
				gomega.ContainElement(osc.ResourceTag{Key: "clikey", Value: "clivalue"}),
				gomega.ContainElement(osc.ResourceTag{Key: tags.ServiceName, Value: svc.Name}),
				gomega.ContainElement(gomega.HaveField("Key", gomega.HavePrefix(tags.ClusterIDPrefix))),
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

var _ = Describe("[e2e][loadbalancer] Configuring a multi-az LBU", func() {
	f := framework.NewDefaultFramework("ccm")
	f.NamespacePodSecurityEnforceLevel = admissionapi.LevelPrivileged

	var (
		cs     clientset.Interface
		ns     *v1.Namespace
		ctx    context.Context
		lbName string
	)

	BeforeEach(func() {
		cs = f.ClientSet
		ns = f.Namespace
		ctx = context.Background()
		lbName = "ccm-multiaz-" + xid.New().String()
	})
	It("can connect to the service", func() {
		deployment := e2eutils.CreateDeployment(ctx, cs, ns, 1, []v1.ContainerPort{
			{
				Name:          "tcp",
				Protocol:      v1.ProtocolTCP,
				ContainerPort: 8080,
			},
		})
		defer e2eutils.DeleteDeployment(ctx, cs, deployment)
		e2eutils.WaitForDeploymentReady(ctx, cs, deployment)

		region := os.Getenv("OSC_REGION")
		svc := e2eutils.CreateSvc(ctx, cs, ns, map[string]string{
			"service.beta.kubernetes.io/osc-load-balancer-name":       "a-" + lbName + ",b-" + lbName,
			"service.beta.kubernetes.io/osc-load-balancer-instances":  "2",
			"service.beta.kubernetes.io/osc-load-balancer-subregions": region + "a," + region + "a", // the CI env is single AZ
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
		gomega.Expect(svc.Status.LoadBalancer.Ingress).To(gomega.HaveLen(2))
		gomega.Expect(svc.Status.LoadBalancer.Ingress[0].Hostname).NotTo(gomega.Equal(svc.Status.LoadBalancer.Ingress[1].Hostname))
		e2esvc.TestReachableHTTP(ctx, svc.Status.LoadBalancer.Ingress[0].Hostname, 80, testTimeout)
		e2esvc.TestReachableHTTP(ctx, svc.Status.LoadBalancer.Ingress[1].Hostname, 80, testTimeout)
	})
})

var _ = Describe("[e2e][loadbalancer] Setting a public IP", func() {
	f := framework.NewDefaultFramework("ccm")
	f.NamespacePodSecurityEnforceLevel = admissionapi.LevelPrivileged

	var (
		cs     clientset.Interface
		ns     *v1.Namespace
		ctx    context.Context
		lbName string
	)

	BeforeEach(func() {
		cs = f.ClientSet
		ns = f.Namespace
		ctx = context.Background()
		lbName = "ccm-public-" + xid.New().String()
	})

	Context("When creating service with a public ip from a pool", func() {
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
				"service.beta.kubernetes.io/osc-load-balancer-name":           lbName,
				"service.beta.kubernetes.io/osc-load-balancer-public-ip-pool": "ccm",
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

		It("can connect to the service", func() {
			svc = e2eutils.WaitForSvc(ctx, cs, svc)
			e2esvc.TestReachableHTTP(ctx, svc.Status.LoadBalancer.Ingress[0].Hostname, 80, testTimeout)
		})
	})

	Context("When creating service with a preconfigured public ip", func() {
		var (
			api        *oapi.Client
			deployment *appsv1.Deployment
			pip        *osc.PublicIp
		)
		BeforeEach(func() {
			var err error
			api, err = oapi.NewClient(ctx)
			framework.ExpectNoError(err)
			pip, err = sdk.AllocateIPFromPool(ctx, "ccm", api.OAPI())
			framework.ExpectNoError(err)
			deployment = e2eutils.CreateDeployment(ctx, cs, ns, 1, []v1.ContainerPort{
				{
					Name:          "tcp",
					Protocol:      v1.ProtocolTCP,
					ContainerPort: 8080,
				},
			})
			e2eutils.WaitForDeploymentReady(ctx, cs, deployment)
		})

		AfterEach(func() {
			if deployment != nil {
				e2eutils.DeleteDeployment(ctx, cs, deployment)
				deployment = nil
			}
		})

		It("setting public ip by id with 'both' ingress address works", func() {
			svc := e2eutils.CreateSvc(ctx, cs, ns, map[string]string{
				"service.beta.kubernetes.io/osc-load-balancer-name":            lbName,
				"service.beta.kubernetes.io/osc-load-balancer-public-ip-id":    pip.PublicIpId,
				"service.beta.kubernetes.io/osc-load-balancer-ingress-address": "both",
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
			gomega.Expect(svc.Status.LoadBalancer.Ingress[0].IP, gomega.Equal(pip.PublicIp))
			gomega.Expect(svc.Status.LoadBalancer.Ingress[0].Hostname, gomega.Not(gomega.BeEmpty()))
			gomega.Expect(svc.Status.LoadBalancer.Ingress[0].IPMode, gomega.Equal("Proxy"))
		})

		It("setting public ip by IP with 'IP' ingress address works", func() {
			svc := e2eutils.CreateSvc(ctx, cs, ns, map[string]string{
				"service.beta.kubernetes.io/osc-load-balancer-name":            lbName,
				"service.beta.kubernetes.io/osc-load-balancer-public-ip-id":    pip.PublicIp,
				"service.beta.kubernetes.io/osc-load-balancer-ingress-address": "ip",
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
			e2esvc.TestReachableHTTP(ctx, svc.Status.LoadBalancer.Ingress[0].IP, 80, testTimeout)
			gomega.Expect(svc.Status.LoadBalancer.Ingress[0].IP, gomega.Equal(pip.PublicIp))
			gomega.Expect(svc.Status.LoadBalancer.Ingress[0].Hostname, gomega.BeEmpty())
			gomega.Expect(svc.Status.LoadBalancer.Ingress[0].IPMode, gomega.Equal("Proxy"))
		})
	})
})

var _ = Describe("[e2e][loadbalancer] Fixing annotation errors", func() {
	f := framework.NewDefaultFramework("ccm")
	f.NamespacePodSecurityEnforceLevel = admissionapi.LevelPrivileged

	var (
		cs     clientset.Interface
		ns     *v1.Namespace
		ctx    context.Context
		lbName string
	)

	BeforeEach(func() {
		cs = f.ClientSet
		ns = f.Namespace
		ctx = context.Background()
		lbName = "ccm-fix-" + xid.New().String()
	})

	Context("When creating service with a bad annotation", func() {
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
				"service.beta.kubernetes.io/osc-load-balancer-name":         lbName,
				"service.beta.kubernetes.io/osc-load-balancer-public-ip-id": "invalid",
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

		It("works after removing to bad annotation", func() {
			e2esvc.WaitForServiceUpdatedWithFinalizer(ctx, cs, svc.Namespace, svc.Name, true)
			svc, err := e2esvc.UpdateService(ctx, cs, svc.Namespace, svc.Name, func(d *v1.Service) {
				delete(svc.Annotations, "service.beta.kubernetes.io/osc-load-balancer-public-ip-id")
			})
			framework.ExpectNoError(err)
			svc = e2eutils.WaitForSvc(ctx, cs, svc)
			e2esvc.TestReachableHTTP(ctx, svc.Status.LoadBalancer.Ingress[0].Hostname, 80, testTimeout)
		})
	})
})

var _ = Describe("[e2e][loadbalancer] Creating an internal load-balancer", func() {
	f := framework.NewDefaultFramework("ccm")
	f.NamespacePodSecurityEnforceLevel = admissionapi.LevelPrivileged

	var (
		cs     clientset.Interface
		ns     *v1.Namespace
		ctx    context.Context
		lbName string
	)

	BeforeEach(func() {
		cs = f.ClientSet
		ns = f.Namespace
		ctx = context.Background()
		lbName = "ccm-internal-" + xid.New().String()
	})

	Context("When creating an internal service", func() {
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
				"service.beta.kubernetes.io/osc-load-balancer-name":     lbName,
				"service.beta.kubernetes.io/osc-load-balancer-internal": "true",
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

		It("ingress is configured and a private IP is returned", func() {
			svc = e2eutils.WaitForSvc(ctx, cs, svc)
			gomega.Expect(svc.Status.LoadBalancer.Ingress, gomega.Not(gomega.BeEmpty()))
			gomega.Expect(svc.Status.LoadBalancer.Ingress[0].Hostname, gomega.Not(gomega.BeEmpty()))
			gomega.Expect(svc.Status.LoadBalancer.Ingress[0].IP, gomega.Not(gomega.BeEmpty()))
			gomega.Expect(svc.Status.LoadBalancer.Ingress[0].IP, gomega.Or(
				// 172.16.0.0/12 should be tested, but a /12 has too many string prefixes
				gomega.HavePrefix("10."),
				gomega.HavePrefix("192.168."),
			))
		})
	})
})

var _ = Describe("[e2e][loadbalancer] Checking proxy-protocol", func() {
	f := framework.NewDefaultFramework("ccm")
	f.NamespacePodSecurityEnforceLevel = admissionapi.LevelPrivileged

	var (
		cs  clientset.Interface
		ns  *v1.Namespace
		ctx context.Context
		api *oapi.Client
	)

	BeforeEach(func() {
		cs = f.ClientSet
		ns = f.Namespace
		ctx = context.Background()
		var err error
		api, err = oapi.NewClient(ctx)
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
		defer e2eutils.DeleteSvc(ctx, cs, svc)
		svc = e2eutils.WaitForSvc(ctx, cs, svc)
		e2eutils.ExpectLoadBalancer(ctx, api, lbName, gomega.HaveField("BackendServerDescriptions",
			gomega.ContainElement(gomega.HaveField("PolicyNames", gomega.ContainElement(ptr.To("k8s-proxyprotocol-enabled")))),
		))
		e2esvc.TestReachableHTTP(ctx, svc.Status.LoadBalancer.Ingress[0].Hostname, 80, testTimeout)
	})
})

var _ = Describe("[e2e][loadbalancer] Checking cleanup of resources", func() {
	f := framework.NewDefaultFramework("ccm")
	f.NamespacePodSecurityEnforceLevel = admissionapi.LevelPrivileged

	var (
		cs  clientset.Interface
		ns  *v1.Namespace
		ctx context.Context
		api *oapi.Client
	)

	BeforeEach(func() {
		cs = f.ClientSet
		ns = f.Namespace
		ctx = context.Background()
		var err error
		api, err = oapi.NewClient(ctx)
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
		lb, err := e2eutils.GetLoadBalancer(ctx, api, lbName)
		framework.ExpectNoError(err)
		e2eutils.DeleteDeployment(ctx, cs, deployment)
		e2eutils.DeleteSvc(ctx, cs, svc)
		By("Checking that load-balancer & security groups have been deleted")
		e2eutils.ExpectNoLoadBalancer(ctx, api, lbName)
		e2eutils.ExpectSecurityGroups(ctx, api, lb, gomega.BeEmpty())
	})
})

var _ = Describe("[e2e][loadbalancer] Updating backends", func() {
	f := framework.NewDefaultFramework("ccm")
	f.NamespacePodSecurityEnforceLevel = admissionapi.LevelPrivileged

	var (
		cs  clientset.Interface
		ns  *v1.Namespace
		ctx context.Context
		api *oapi.Client
	)

	BeforeEach(func() {
		cs = f.ClientSet
		ns = f.Namespace
		ctx = context.Background()
		var err error
		api, err = oapi.NewClient(ctx)
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
		lb, err := e2eutils.GetLoadBalancer(ctx, api, lbName)
		framework.ExpectNoError(err)
		e2eutils.DeleteWorkerNode(ctx, cs)
		By("Waiting until LB backends have changed")
		e2eutils.ExpectLoadBalancer(ctx, api, lbName, gomega.Not(gomega.HaveField("Instances", lb.Instances)))
		e2esvc.TestReachableHTTP(ctx, svc.Status.LoadBalancer.Ingress[0].Hostname, 80, testTimeout)
	})
})
