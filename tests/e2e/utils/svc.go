package e2eutils

import (
	"context"
	"fmt"
	"time"

	"github.com/onsi/ginkgo/v2"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/test/e2e/framework"
	e2esvc "k8s.io/kubernetes/test/e2e/framework/service"
)

// getAnnotations return Annotations
func getAnnotations() map[string]string {
	return map[string]string{
		// Tags
		"service.beta.kubernetes.io/aws-load-balancer-additional-resource-tags": "testKey1=Val1,testKey2=Val2",
		// ConnectionDraining
		"service.beta.kubernetes.io/aws-load-balancer-connection-draining-enabled": "true",
		"service.beta.kubernetes.io/aws-load-balancer-connection-draining-timeout": "30",
		// ConnectionSettings
		"service.beta.kubernetes.io/aws-load-balancer-connection-idle-timeout": "65",
	}
}

// CreateSvc create an svc
func CreateSvc(ctx context.Context, client clientset.Interface, namespace *v1.Namespace, additional map[string]string, servicePort []v1.ServicePort, sourceRangesCidr []string) *v1.Service {
	name := "echoserver"
	ginkgo.By(fmt.Sprintf("Creating service %q", name))
	svcClient := client.CoreV1().Services(namespace.Name)

	annotations := getAnnotations()
	for k, v := range additional {
		annotations[k] = v
	}

	service := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   namespace.Name,
			Annotations: annotations,
		},
		Spec: v1.ServiceSpec{
			Type: v1.ServiceTypeLoadBalancer,
			Selector: map[string]string{
				"app": name,
			},
			Ports: servicePort,
		},
	}
	service.Spec.LoadBalancerSourceRanges = sourceRangesCidr
	result, err := svcClient.Create(ctx, service, metav1.CreateOptions{})
	framework.ExpectNoError(err)
	return result
}

// DeleteSvc delete an svc
func DeleteSvc(ctx context.Context, client clientset.Interface, svc *v1.Service) {
	ginkgo.By(fmt.Sprintf("Deleting service %q", svc.Name))
	svcClient := client.CoreV1().Services(svc.Namespace)
	err := svcClient.Delete(ctx, svc.GetObjectMeta().GetName(), metav1.DeleteOptions{})
	framework.ExpectNoError(err)
}

// ListSvc list and svc
func ListSvc(ctx context.Context, client clientset.Interface, namespace *v1.Namespace) {
	svcClient := client.CoreV1().Services(namespace.Name)
	list, err := svcClient.List(ctx, metav1.ListOptions{})
	framework.ExpectNoError(err)
	fmt.Printf("svc:  %v\n", list.Items)
}

// GetSvc return an svc
func GetSvc(ctx context.Context, client clientset.Interface, namespace *v1.Namespace, name string) (result *v1.Service) {
	svcClient := client.CoreV1().Services(namespace.Name)
	result, err := svcClient.Get(ctx, name, metav1.GetOptions{})
	framework.ExpectNoError(err)
	framework.Logf("Get Svc:  %+v\n", result)
	return result
}

// WaitForSvc wait for an svc to be ready
func WaitForSvc(ctx context.Context, client clientset.Interface, svc *v1.Service) *v1.Service {
	name := svc.Name
	e2esvc.WaitForServiceUpdatedWithFinalizer(ctx, client, svc.Namespace, name, true)
	ginkgo.By("Wait for ingress")
	svcClient := client.CoreV1().Services(svc.Namespace)
	err := wait.PollUntilContextTimeout(ctx, 30*time.Second, e2esvc.GetServiceLoadBalancerCreationTimeout(ctx, client), true, func(ctx context.Context) (bool, error) {
		var err error
		svc, err = svcClient.Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		framework.Logf("ingress: %+v\n", svc.Status.LoadBalancer.Ingress)
		return len(svc.Status.LoadBalancer.Ingress) > 0, nil
	})
	framework.ExpectNoError(err)
	return svc
}

// WaitForDeletedSvc waits for an svc to be deleted
func WaitForDeletedSvc(ctx context.Context, client clientset.Interface, svc *v1.Service) {
	e2esvc.WaitForServiceDeletedWithFinalizer(ctx, client, svc.Namespace, svc.Name)
}
