package e2eutils

import (
	"context"
	"fmt"

	"github.com/onsi/ginkgo/v2"
	"github.com/outscale/goutils/sdk/ptr"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/test/e2e/framework"
	e2edeployment "k8s.io/kubernetes/test/e2e/framework/deployment"
	e2epod "k8s.io/kubernetes/test/e2e/framework/pod"
	imageutils "k8s.io/kubernetes/test/utils/image"
)

// ListDeployment list deployement
func ListDeployment(ctx context.Context, client clientset.Interface, namespace *v1.Namespace) {
	deploymentsClient := client.AppsV1().Deployments(namespace.Name)
	fmt.Printf("Listing deployments in namespace %q:\n", namespace.Name)
	list, err := deploymentsClient.List(ctx, metav1.ListOptions{})
	if err != nil {
		panic(err)
	}
	for _, d := range list.Items {
		fmt.Printf(" * %s (%d replicas)\n", d.Name, *d.Spec.Replicas)
	}
}

// CreateDeployment create a deployement
func CreateDeployment(ctx context.Context, client clientset.Interface, namespace *v1.Namespace, replicas int32, ports []v1.ContainerPort) *appsv1.Deployment {
	imageName := imageutils.GetE2EImage(imageutils.Agnhost)
	name := "echoserver"
	ginkgo.By(fmt.Sprintf("Creating deployment %q", name))
	deploymentsClient := client.AppsV1().Deployments(namespace.Name)
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr.To(replicas),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": name,
				},
			},
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": name,
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name:            name,
							ImagePullPolicy: v1.PullIfNotPresent,
							Image:           imageName,
							Ports:           ports,
						},
					},
				},
			},
		},
	}
	deployment.Spec.Template.Spec.Containers[0].Args = []string{"netexec", "--http-port=8080"}

	// Create Deployment
	result, err := deploymentsClient.Create(ctx, deployment, metav1.CreateOptions{})
	framework.ExpectNoError(err)
	return result
}

// CreateProxyProtocolDeployment create a deployement
func CreateProxyProtocolDeployment(ctx context.Context, client clientset.Interface, namespace *v1.Namespace, replicas int32, ports []v1.ContainerPort) *appsv1.Deployment {
	imageName := "gcr.io/google_containers/echoserver:1.10"
	name := "echoserver"
	ginkgo.By(fmt.Sprintf("Creating deployment %q", name))
	deploymentsClient := client.AppsV1().Deployments(namespace.Name)
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr.To(replicas),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": name,
				},
			},
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": name,
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name:            name,
							ImagePullPolicy: v1.PullIfNotPresent,
							Image:           imageName,
							Ports:           ports,
							Command:         []string{"/bin/sh"},
							Args: []string{"-c", "sed -i 's/listen 8080 default_server reuseport/listen 8080 default_server reuseport proxy_protocol/g' /etc/nginx/nginx.conf; " +
								"sed -i 's/listen 8443 default_server ssl http2 reuseport/listen 8443 default_server ssl http2 reuseport proxy_protocol/g' /etc/nginx/nginx.conf ; " +
								"/usr/local/bin/run.sh"},
						},
					},
				},
			},
		},
	}

	// Create Deployment
	result, err := deploymentsClient.Create(ctx, deployment, metav1.CreateOptions{})
	framework.ExpectNoError(err)
	return result
}

// DeleteDeployment delete a Deployment
func DeleteDeployment(ctx context.Context, client clientset.Interface, deployment *appsv1.Deployment) {
	ginkgo.By(fmt.Sprintf("Deleting deployment %q", deployment.Name))
	deploymentsClient := client.AppsV1().Deployments(deployment.Namespace)
	deletePolicy := metav1.DeletePropagationForeground
	err := deploymentsClient.Delete(ctx, deployment.Name, metav1.DeleteOptions{
		PropagationPolicy: &deletePolicy,
	})
	framework.ExpectNoError(err)
}

// WaitForDeploymentReady wait for a Deployement
func WaitForDeploymentReady(ctx context.Context, client clientset.Interface, deployment *appsv1.Deployment) {
	err := e2edeployment.WaitForDeploymentComplete(client, deployment)
	framework.ExpectNoError(err)

	pods, err := e2edeployment.GetPodsForDeployment(ctx, client, deployment)
	framework.ExpectNoError(err)
	for _, pod := range pods.Items {
		ginkgo.By(fmt.Sprintf("Waiting for pod %q", pod.Name))
		err = e2epod.WaitForPodRunningInNamespace(ctx, client, &pod)
		framework.ExpectNoError(err)
	}
}
