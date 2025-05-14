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
	"fmt"
	"strings"
	"time"

	elbApi "github.com/aws/aws-sdk-go/service/elb"
	. "github.com/onsi/ginkgo/v2" //nolint: staticcheck
	. "github.com/onsi/gomega"    //nolint: staticcheck
	e2eutils "github.com/outscale/cloud-provider-osc/tests/e2e/utils"
	apiv1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/test/e2e/framework"
	e2esvc "k8s.io/kubernetes/test/e2e/framework/service"
	admissionapi "k8s.io/pod-security-admission/api"
)

const (
	echoServerImage = "gcr.io/google_containers/echoserver:1.10"
	testTimeout     = 10 * time.Minute
)

// TestParam customize e2e tests and lb annotations
type TestParam struct {
	Title              string
	Annotations        map[string]string
	Cmd                string
	DeploymentMetaName string
	DeploymentName     string
	DeploymentImage    string
	WaitForDeplyment   bool
	Replicas           int32
	Ports              []apiv1.ContainerPort
	ServiceMetaName    string
	ServiceName        string
	ServicePorts       []apiv1.ServicePort
	SourceRanges       bool
	SourceRangesCidr   []string
	UpdateService      bool
}

var _ = Describe("[ccm-e2e] SVC-LB", func() {
	f := framework.NewDefaultFramework("ccm")
	f.NamespacePodSecurityEnforceLevel = admissionapi.LevelPrivileged

	var (
		cs clientset.Interface
		ns *apiv1.Namespace
	)

	BeforeEach(func() {
		cs = f.ClientSet
		ns = f.Namespace
	})

	params := []TestParam{
		{
			Title:              "Create LB",
			Cmd:                "",
			Annotations:        map[string]string{},
			DeploymentMetaName: "echoheaders",
			DeploymentName:     "echoheaders",
			DeploymentImage:    echoServerImage,
			Replicas:           1,
			Ports: []apiv1.ContainerPort{
				{
					Name:          "tcp",
					Protocol:      apiv1.ProtocolTCP,
					ContainerPort: 8080,
				},
			},
			ServiceMetaName: "echoheaders-lb-public",
			ServiceName:     "echoheaders",
			ServicePorts: []apiv1.ServicePort{
				{
					Name:       "tcp",
					Protocol:   apiv1.ProtocolTCP,
					TargetPort: intstr.FromInt(8080),
					Port:       80,
				},
			},
			SourceRanges:     false,
			SourceRangesCidr: []string{},
			UpdateService:    true,
		},
		{
			Title:              "Create LB after deployment",
			Cmd:                "",
			Annotations:        map[string]string{},
			DeploymentMetaName: "echoheaders",
			DeploymentName:     "echoheaders",
			DeploymentImage:    echoServerImage,
			WaitForDeplyment:   true,
			Replicas:           1,
			Ports: []apiv1.ContainerPort{
				{
					Name:          "tcp",
					Protocol:      apiv1.ProtocolTCP,
					ContainerPort: 8080,
				},
			},
			ServiceMetaName: "echoheaders-lb-public",
			ServiceName:     "echoheaders",
			ServicePorts: []apiv1.ServicePort{
				{
					Name:       "tcp",
					Protocol:   apiv1.ProtocolTCP,
					TargetPort: intstr.FromInt(8080),
					Port:       80,
				},
			},
			SourceRanges:     false,
			SourceRangesCidr: []string{},
			UpdateService:    true,
		},
		{
			Title: "Create LB With proxy protocol enabled",
			Cmd: "sed -i 's/listen 8080 default_server reuseport/listen 8080 default_server reuseport proxy_protocol/g' /etc/nginx/nginx.conf; " +
				"sed -i 's/listen 8443 default_server ssl http2 reuseport/listen 8443 default_server ssl http2 reuseport proxy_protocol/g' /etc/nginx/nginx.conf ; " +
				"/usr/local/bin/run.sh",
			Annotations: map[string]string{
				"service.beta.kubernetes.io/aws-load-balancer-proxy-protocol": "*",
			},
			DeploymentMetaName: "echoheaders",
			DeploymentName:     "echoheaders",
			DeploymentImage:    echoServerImage,
			Replicas:           1,
			Ports: []apiv1.ContainerPort{
				{
					Name:          "tcp",
					Protocol:      apiv1.ProtocolTCP,
					ContainerPort: 8080,
				},
			},
			ServiceMetaName: "echoheaders-lb-public",
			ServiceName:     "echoheaders",
			ServicePorts: []apiv1.ServicePort{
				{
					Name:       "tcp",
					Protocol:   apiv1.ProtocolTCP,
					TargetPort: intstr.FromInt(8080),
					Port:       80,
				},
			},
			SourceRanges:     false,
			SourceRangesCidr: []string{},
			UpdateService:    true,
		},
		{
			Title: "Create LB with hc customized",
			Cmd:   "",
			Annotations: map[string]string{
				"service.beta.kubernetes.io/aws-load-balancer-healthcheck-healthy-threshold":   "3",
				"service.beta.kubernetes.io/aws-load-balancer-healthcheck-unhealthy-threshold": "7",
				"service.beta.kubernetes.io/aws-load-balancer-healthcheck-timeout":             "6",
				"service.beta.kubernetes.io/aws-load-balancer-healthcheck-interval":            "11",
			},
			DeploymentMetaName: "echoheaders",
			DeploymentName:     "echoheaders",
			DeploymentImage:    echoServerImage,
			Replicas:           1,
			Ports: []apiv1.ContainerPort{
				{
					Name:          "tcp",
					Protocol:      apiv1.ProtocolTCP,
					ContainerPort: 8080,
				},
			},
			ServiceMetaName: "echoheaders-lb-public",
			ServiceName:     "echoheaders",
			ServicePorts: []apiv1.ServicePort{
				{
					Name:       "tcp",
					Protocol:   apiv1.ProtocolTCP,
					TargetPort: intstr.FromInt(8080),
					Port:       80,
				},
			},
			SourceRanges:     false,
			SourceRangesCidr: []string{},
			UpdateService:    true,
		},
		{
			Title:              "Create basic LB",
			Cmd:                "",
			Annotations:        map[string]string{},
			DeploymentMetaName: "basic-deployment",
			DeploymentName:     "basic",
			DeploymentImage:    echoServerImage,
			Replicas:           2,
			Ports: []apiv1.ContainerPort{
				{
					Name:          "tcp",
					Protocol:      apiv1.ProtocolTCP,
					ContainerPort: 8080,
				},
			},
			ServiceMetaName: "service-basic",
			ServiceName:     "basic",
			ServicePorts: []apiv1.ServicePort{
				{
					Name:       "tcp",
					Protocol:   apiv1.ProtocolTCP,
					TargetPort: intstr.FromInt(8080),
					Port:       80,
				},
			},
			SourceRanges:     false,
			SourceRangesCidr: []string{},
			UpdateService:    false,
		},
		{
			Title: "Create simple lb",
			Cmd:   "",
			Annotations: map[string]string{
				"service.beta.kubernetes.io/osc-load-balancer-name-length": "20",
				"service.beta.kubernetes.io/osc-load-balancer-name":        "simple-lb-test",
			},
			DeploymentMetaName: "echoheaders",
			DeploymentName:     "echoheaders",
			DeploymentImage:    echoServerImage,
			Replicas:           1,
			Ports: []apiv1.ContainerPort{
				{
					Name:          "tcp",
					Protocol:      apiv1.ProtocolTCP,
					ContainerPort: 8080,
				},
			},
			ServiceMetaName: "echoheaders-lb-advanced-public",
			ServiceName:     "echoheaders",
			ServicePorts: []apiv1.ServicePort{
				{
					Name:       "http",
					Protocol:   apiv1.ProtocolTCP,
					TargetPort: intstr.FromInt(8080),
					Port:       80,
				},
			},
			SourceRanges:     true,
			SourceRangesCidr: []string{"0.0.0.0/0"},
			UpdateService:    false,
		},
	}

	for _, param := range params {
		title := param.Title
		cmd := param.Cmd
		annotations := param.Annotations
		deploymentMetaName := param.DeploymentMetaName
		deploymentName := param.DeploymentName
		deploymentImage := param.DeploymentImage
		replicas := param.Replicas
		ports := param.Ports
		serviceMetaName := param.ServiceMetaName
		serviceName := param.ServiceName
		servicePorts := param.ServicePorts
		sourceRanges := param.SourceRanges
		sourceRangesCidr := param.SourceRangesCidr
		updateService := param.UpdateService
		wait := param.WaitForDeplyment
		It(title, func() {
			fmt.Printf("Create Simple LB :  %v\n", ns)
			fmt.Printf("Cs :  %v\n", cs)
			fmt.Printf("Params :  %v / %v / %v\n", title, cmd, annotations)

			By("Create Deployment")

			deployement := e2eutils.CreateDeployment(cs, ns, cmd, deploymentMetaName, deploymentName, deploymentImage, replicas, ports)
			defer e2eutils.DeleteDeployment(cs, ns, deployement)
			defer e2eutils.ListDeployment(cs, ns)

			if wait {
				By("Check that the pods are running")
				e2eutils.WaitForDeployementReady(cs, ns, deployement)
			}

			By("List deployment")
			e2eutils.ListDeployment(cs, ns)

			By("Create Svc")
			svc := e2eutils.CreateSvc(cs, ns, annotations, serviceMetaName, serviceName, servicePorts, sourceRanges, sourceRangesCidr)
			fmt.Printf("Created Service %q.\n", svc)
			defer e2eutils.ListSvc(cs, ns)
			defer e2eutils.DeleteSvc(cs, ns, svc)

			By("Check that the svc is ready")
			e2eutils.WaitForSvc(cs, ns, svc)

			By("Listing svc")
			e2eutils.ListSvc(cs, ns)

			By("Get Updated svc")
			count := 0
			var updatedSvc *apiv1.Service
			for count < 10 {
				updatedSvc = e2eutils.GetSvc(cs, ns, svc.GetObjectMeta().GetName())
				fmt.Printf("Ingress:  %v\n", updatedSvc.Status.LoadBalancer.Ingress)
				if len(updatedSvc.Status.LoadBalancer.Ingress) > 0 {
					break
				}
				count++
				time.Sleep(30 * time.Second)
			}
			address := updatedSvc.Status.LoadBalancer.Ingress[0].Hostname
			lbName := strings.Split(address, "-")[0]
			fmt.Printf("address:  %v\n", address)

			By("Test Connection (wait to have endpoint ready)")
			e2esvc.TestReachableHTTP(context.TODO(), address, int(servicePorts[0].Port), testTimeout)
			if updateService {
				By("Remove Instances from lbu")
				elb, err := e2eutils.ElbAPI()
				framework.ExpectNoError(err)

				lb, err := e2eutils.GetLb(elb, lbName)
				framework.ExpectNoError(err)

				lbInstances := []*elbApi.Instance{}
				for _, lbInstance := range lb.Instances {
					lbInstanceItem := &elbApi.Instance{}
					lbInstanceItem.InstanceId = lbInstance.InstanceId
					lbInstances = append(lbInstances, lbInstanceItem)
				}
				Expect(lbInstances).NotTo(BeEmpty(), "There must be a load-balancer")
				//framework.ExpectNotEqual(len(lbInstances), 0)

				err = e2eutils.RemoveLbInst(elb, lbName, lbInstances)
				framework.ExpectNoError(err)

				lb, err = e2eutils.GetLb(elb, lbName)
				framework.ExpectNoError(err)
				Expect(lb.Instances).To(BeEmpty(), "Load-balancer should have been deleted")
				//framework.ExpectEqual(len(lb.Instances), 0)

				By("Add port to force update of LB")
				port := apiv1.ServicePort{
					Name:       "tcp2",
					Protocol:   apiv1.ProtocolTCP,
					TargetPort: intstr.FromInt(8443),
					Port:       443,
				}
				svc = e2eutils.UpdateSvcPorts(cs, ns, updatedSvc, port)
				fmt.Printf("svc updated:  %v\n", svc)

				By("Test LB updated(wait to have vm registred)")
				count = 0
				for count < 10 {
					lb, err = e2eutils.GetLb(elb, lbName)
					if err == nil && len(lb.Instances) != 0 {
						break
					}
					count++
					time.Sleep(30 * time.Second)
				}
				lb, err = e2eutils.GetLb(elb, lbName)

				framework.ExpectNoError(err)
				Expect(lb.Instances).NotTo(BeEmpty())
				//framework.ExpectNotEqual(len(lb.Instances), 0)

				By("TestReachableHTTP after update")
				e2esvc.TestReachableHTTP(context.TODO(), address, int(servicePorts[0].Port), testTimeout)
			}
		})

	}
})

// Test to check that the issue 68 is solved
var _ = Describe("[ccm-e2e] SVC-LB", func() {
	f := framework.NewDefaultFramework("ccm")
	f.NamespacePodSecurityEnforceLevel = admissionapi.LevelPrivileged

	var (
		cs clientset.Interface
	)

	BeforeEach(func() {
		cs = f.ClientSet
	})

	title := "Issue #68"
	cmd := ""
	annotations := map[string]string{}
	deploymentMetaName := "echoheaders"
	deploymentImage := echoServerImage
	var replicas int32 = 1
	deploymentName := "echoheaders"
	serviceMetaName := "echoheaders-lb-public"
	serviceName := "echoheaders"
	servicePorts := []apiv1.ServicePort{
		{
			Name:       "tcp",
			Protocol:   apiv1.ProtocolTCP,
			TargetPort: intstr.FromInt(8080),
			Port:       80,
		},
	}
	sourceRanges := false
	sourceRangesCidr := []string{}
	ports := []apiv1.ContainerPort{
		{
			Name:          "tcp",
			Protocol:      apiv1.ProtocolTCP,
			ContainerPort: 8080,
		},
	}
	It(title, func() {
		nsSvc1, err := f.CreateNamespace(context.TODO(), "svc1", map[string]string{})
		framework.ExpectNoError(err)
		nsSvc2, err := f.CreateNamespace(context.TODO(), "svc2", map[string]string{})
		framework.ExpectNoError(err)

		fmt.Printf("Params :  %v / %v / %v\n", title, cmd, annotations)

		By("Create Deployment 1")

		deployementSvc1 := e2eutils.CreateDeployment(cs, nsSvc1, cmd, deploymentMetaName, deploymentName, deploymentImage, replicas, ports)
		defer e2eutils.DeleteDeployment(cs, nsSvc1, deployementSvc1)
		defer e2eutils.ListDeployment(cs, nsSvc1)

		By("Create Deployment 2")

		deployementSvc2 := e2eutils.CreateDeployment(cs, nsSvc2, cmd, deploymentMetaName, deploymentName, deploymentImage, replicas, ports)
		defer e2eutils.DeleteDeployment(cs, nsSvc1, deployementSvc2)
		defer e2eutils.ListDeployment(cs, nsSvc1)

		// By("checking that pods are running")
		// e2eutils.WaitForDeployementReady(cs, nsSvc1, deployementSvc1)
		// e2eutils.WaitForDeployementReady(cs, nsSvc2, deployementSvc2)

		By("Create Svc 1")
		svc1 := e2eutils.CreateSvc(cs, nsSvc1, annotations, serviceMetaName, serviceName, servicePorts, sourceRanges, sourceRangesCidr)
		fmt.Printf("Created Service %q.\n", svc1)
		defer e2eutils.ListSvc(cs, nsSvc1)

		By("Create Svc 2")
		svc2 := e2eutils.CreateSvc(cs, nsSvc2, annotations, serviceMetaName, serviceName, servicePorts, sourceRanges, sourceRangesCidr)
		fmt.Printf("Created Service %q.\n", svc2)
		defer e2eutils.ListSvc(cs, nsSvc2)
		defer e2eutils.DeleteSvc(cs, nsSvc2, svc2)

		By("Check that svc are ready")
		e2eutils.WaitForSvc(cs, nsSvc1, svc1)
		e2eutils.WaitForSvc(cs, nsSvc2, svc2)

		By("List deployments")
		e2eutils.ListDeployment(cs, nsSvc1)
		e2eutils.ListDeployment(cs, nsSvc2)

		By("List svc")
		e2eutils.ListSvc(cs, nsSvc1)
		e2eutils.ListSvc(cs, nsSvc2)

		By("Get Updated svc")
		addresses := [2]string{}
		lbNames := [2]string{}
		nss := []*apiv1.Namespace{nsSvc1, nsSvc2}
		svcs := []*apiv1.Service{svc1, svc2}
		for i := 0; i < 2; i++ {
			count := 0
			var updatedSvc *apiv1.Service
			for count < 10 {
				updatedSvc = e2eutils.GetSvc(cs, nss[i], svcs[i].GetObjectMeta().GetName())
				fmt.Printf("Ingress:  %v\n", updatedSvc.Status.LoadBalancer.Ingress)
				if len(updatedSvc.Status.LoadBalancer.Ingress) > 0 {
					break
				}
				count++
				time.Sleep(30 * time.Second)
			}

			addresses[i] = updatedSvc.Status.LoadBalancer.Ingress[0].Hostname
			lbNames[i] = strings.Split(addresses[i], "-")[0]
			fmt.Printf("address:  %v\n", addresses[i])
		}

		By("Test Connection (wait to have endpoint ready)")
		for i := 0; i < 2; i++ {
			e2esvc.TestReachableHTTP(context.TODO(), addresses[i], 80, testTimeout)
		}

		By("Remove SVC 1")
		e2eutils.DeleteSvc(cs, nsSvc1, svc1)

		e2eutils.WaitForDeletedSvc(cs, nsSvc1, svc1)

		fmt.Printf("Wait to have stable sg")
		time.Sleep(120 * time.Second)

		By("Test SVC 2")
		e2esvc.TestReachableHTTP(context.TODO(), addresses[1], 80, testTimeout)
	})
})
