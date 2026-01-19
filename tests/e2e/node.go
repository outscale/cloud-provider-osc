package e2e

import (
	"context"

	. "github.com/onsi/ginkgo/v2" //nolint: staticcheck
	"github.com/onsi/gomega"
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
