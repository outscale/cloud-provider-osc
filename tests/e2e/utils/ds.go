package e2eutils

import (
	"context"
	"fmt"

	"github.com/onsi/ginkgo/v2"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/test/e2e/framework"
)

// CreateDaemonSet create a daemonset
func CreateDaemonSet(ctx context.Context, client clientset.Interface, imageName string) *appsv1.DaemonSet {
	name := "labeler"
	ginkgo.By(fmt.Sprintf("Creating daemonset %q", name))
	dsClient := client.AppsV1().DaemonSets("kube-system")
	ds := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": name,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": name,
					},
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: "cloud-controller-manager",
					Containers: []corev1.Container{
						{
							Name:            name,
							Command:         []string{"/bin/osc-labeler", "--wait"},
							ImagePullPolicy: corev1.PullIfNotPresent,
							Image:           imageName,
							Env: []corev1.EnvVar{{
								Name: "NODE_NAME",
								ValueFrom: &corev1.EnvVarSource{
									FieldRef: &corev1.ObjectFieldSelector{
										FieldPath: "spec.nodeName",
									},
								},
							}},
						},
					},
					Resources: &corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceMemory: resource.MustParse("64M"),
							corev1.ResourceCPU:    resource.MustParse("250m"),
						},
						Limits: corev1.ResourceList{
							corev1.ResourceMemory: resource.MustParse("64M"),
							corev1.ResourceCPU:    resource.MustParse("250m"),
						},
					},
				},
			},
		},
	}
	// Create Deployment
	result, err := dsClient.Create(ctx, ds, metav1.CreateOptions{})
	framework.ExpectNoError(err)
	return result
}

// DeleteDaemonSet delete a Deployment
func DeleteDaemonSet(ctx context.Context, client clientset.Interface, ds *appsv1.DaemonSet) {
	ginkgo.By(fmt.Sprintf("Deleting daemonset %q", ds.Name))
	dsClient := client.AppsV1().DaemonSets(ds.Namespace)
	deletePolicy := metav1.DeletePropagationForeground
	err := dsClient.Delete(ctx, ds.Name, metav1.DeleteOptions{
		PropagationPolicy: &deletePolicy,
	})
	framework.ExpectNoError(err)
}
