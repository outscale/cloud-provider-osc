package e2eutils

import (
	"context"
	"fmt"

	"github.com/onsi/ginkgo/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/test/e2e/framework"
)

const ControlPlaneLabel = "node-role.kubernetes.io/control-plane"

// DeleteWorkerNode deletes a worker node
func DeleteWorkerNode(ctx context.Context, client clientset.Interface) {
	cl := client.CoreV1().Nodes()
	lst, err := cl.List(ctx, metav1.ListOptions{})
	framework.ExpectNoError(err)
	for _, node := range lst.Items {
		if _, found := node.Labels[ControlPlaneLabel]; found {
			continue
		}
		ginkgo.By(fmt.Sprintf("Deleting worker node %q", node.Name))
		err = cl.Delete(ctx, node.Name, metav1.DeleteOptions{})
		framework.ExpectNoError(err)
		return
	}
}
