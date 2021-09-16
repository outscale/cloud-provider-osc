module github.com/outscale-dev/cloud-provider-osc

go 1.13

replace (
	// these replacements are pinned to 59603c6e503c which is the sha associated with the 1.17.2 tag on k/k
	// as you cannot pin them to v1.17.2 directly
	k8s.io/api => k8s.io/kubernetes/staging/src/k8s.io/api v0.0.0-20200118001809-59603c6e503c
	k8s.io/apiextensions-apiserver => k8s.io/kubernetes/staging/src/k8s.io/apiextensions-apiserver v0.0.0-20200118001809-59603c6e503c
	k8s.io/apimachinery => k8s.io/kubernetes/staging/src/k8s.io/apimachinery v0.0.0-20200118001809-59603c6e503c
	k8s.io/apiserver => k8s.io/kubernetes/staging/src/k8s.io/apiserver v0.0.0-20200118001809-59603c6e503c
	k8s.io/cli-runtime => k8s.io/kubernetes/staging/src/k8s.io/cli-runtime v0.0.0-20200118001809-59603c6e503c
	k8s.io/client-go => k8s.io/kubernetes/staging/src/k8s.io/client-go v0.0.0-20200118001809-59603c6e503c
	k8s.io/cloud-provider => k8s.io/kubernetes/staging/src/k8s.io/cloud-provider v0.0.0-20200118001809-59603c6e503c
	k8s.io/cluster-bootstrap => k8s.io/kubernetes/staging/src/k8s.io/cluster-bootstrap v0.0.0-20200118001809-59603c6e503c
	k8s.io/code-generator => k8s.io/kubernetes/staging/src/k8s.io/code-generator v0.0.0-20200118001809-59603c6e503c
	k8s.io/component-base => k8s.io/kubernetes/staging/src/k8s.io/component-base v0.0.0-20200118001809-59603c6e503c
	k8s.io/cri-api => k8s.io/kubernetes/staging/src/k8s.io/cri-api v0.0.0-20200118001809-59603c6e503c
	k8s.io/csi-translation-lib => k8s.io/kubernetes/staging/src/k8s.io/csi-translation-lib v0.0.0-20200118001809-59603c6e503c
	k8s.io/kube-aggregator => k8s.io/kubernetes/staging/src/k8s.io/kube-aggregator v0.0.0-20200118001809-59603c6e503c
	k8s.io/kube-controller-manager => k8s.io/kubernetes/staging/src/k8s.io/kube-controller-manager v0.0.0-20200118001809-59603c6e503c
	k8s.io/kube-proxy => k8s.io/kubernetes/staging/src/k8s.io/kube-proxy v0.0.0-20200118001809-59603c6e503c
	k8s.io/kube-scheduler => k8s.io/kubernetes/staging/src/k8s.io/kube-scheduler v0.0.0-20200118001809-59603c6e503c
	k8s.io/kubectl => k8s.io/kubernetes/staging/src/k8s.io/kubectl v0.0.0-20200118001809-59603c6e503c
	k8s.io/kubelet => k8s.io/kubernetes/staging/src/k8s.io/kubelet v0.0.0-20200118001809-59603c6e503c
	k8s.io/legacy-cloud-providers => k8s.io/kubernetes/staging/src/k8s.io/legacy-cloud-providers v0.0.0-20200118001809-59603c6e503c
	k8s.io/metrics => k8s.io/kubernetes/staging/src/k8s.io/metrics v0.0.0-20200118001809-59603c6e503c
	k8s.io/node-api => k8s.io/kubernetes/staging/src/k8s.io/node-api v0.0.0-20200118001809-59603c6e503c
	k8s.io/sample-apiserver => k8s.io/kubernetes/staging/src/k8s.io/sample-apiserver v0.0.0-20200118001809-59603c6e503c
)

require (
	github.com/antihax/optional v1.0.0
	github.com/aws/aws-sdk-go v1.16.26
	github.com/prometheus/client_golang v1.0.0
	github.com/spf13/cobra v0.0.7
	github.com/stretchr/testify v1.4.0
	gopkg.in/gcfg.v1 v1.2.0
	k8s.io/api v0.0.0
	k8s.io/apimachinery v0.0.0
	k8s.io/apiserver v0.0.0
	k8s.io/client-go v0.0.0
	k8s.io/cloud-provider v0.0.0
	k8s.io/component-base v0.0.0
	k8s.io/klog v1.0.0
	k8s.io/kubernetes v1.17.2
	k8s.io/utils v0.0.0-20191114184206-e782cd3c129f
)
