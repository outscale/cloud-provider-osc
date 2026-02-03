package e2e

import (
	"context"
	"os"

	. "github.com/onsi/ginkgo/v2" //nolint: staticcheck
	"github.com/onsi/gomega"
	"github.com/outscale/cloud-provider-osc/labeler"
	e2eutils "github.com/outscale/cloud-provider-osc/tests/e2e/utils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kubernetes/test/e2e/framework"
	admissionapi "k8s.io/pod-security-admission/api"
)

var _ = Describe("[e2e][node] Adding labels to nodes", func() {
	f := framework.NewDefaultFramework("ccm")
	f.NamespacePodSecurityEnforceLevel = admissionapi.LevelPrivileged

	It("can set labels on nodes", func() {
		cl := f.ClientSet.CoreV1().Nodes()
		lst, err := cl.List(context.Background(), metav1.ListOptions{})
		framework.ExpectNoError(err)
		for _, node := range lst.Items {
			gomega.Expect(node.Labels).To(gomega.HaveKey("clilabel"))
		}
	})
})

var _ = Describe("[e2e][node] The node labeler works", func() {
	f := framework.NewDefaultFramework("ccm")
	f.NamespacePodSecurityEnforceLevel = admissionapi.LevelPrivileged

	imageName := os.Getenv("CCM_IMAGE")
	It("can addcluster/server labels on nodes", func() {
		ctx := context.Background()
		_ = e2eutils.CreateDaemonSet(ctx, f.ClientSet, imageName)
		// defer e2eutils.DeleteDaemonSet(ctx, f.ClientSet, ds) - avoid deleting to be able to dump logs
		cl := f.ClientSet.CoreV1().Nodes()
		lst, err := cl.List(ctx, metav1.ListOptions{
			LabelSelector: "!" + e2eutils.ControlPlaneLabel,
		})
		framework.ExpectNoError(err)
		for _, node := range lst.Items {
			By("Labels for " + node.Name)
			gomega.Eventually(func() map[string]string {
				n, _ := cl.Get(ctx, node.Name, metav1.GetOptions{})
				if n != nil {
					framework.Logf("labels: %v", n.Labels)
					return n.Labels
				}
				return map[string]string{}
			}).To(gomega.And(gomega.HaveKey(labeler.ClusterLabel), gomega.HaveKey(labeler.ServerLabel)))
		}
	})
})
