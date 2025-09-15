package deploy_test

import (
	"bufio"
	"errors"
	"io"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/kubectl/pkg/scheme"
)

func getHelmSpecs(t *testing.T, vars []string) []runtime.Object {
	args := []string{"template", "--debug"}
	if len(vars) > 0 {
		args = append(args, "--set", strings.Join(vars, ","))
	}
	args = append(args, "k8s-osc-ccm")
	cmd := exec.Command("helm", args...)
	stdout, err := cmd.StdoutPipe()
	require.NoError(t, err)
	err = cmd.Start()
	require.NoError(t, err)
	var specs []runtime.Object
	decode := scheme.Codecs.UniversalDeserializer().Decode
	r := yaml.NewYAMLReader(bufio.NewReader(stdout))
	for {
		buf, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)
		spec, _, err := decode(buf, nil, nil)
		require.NoError(t, err)
		specs = append(specs, spec)
	}
	err = cmd.Wait()
	require.NoError(t, err)
	require.Len(t, specs, 5)
	return specs
}

func TestHelmTemplate(t *testing.T) {
	t.Run("The chart contains the right objets", func(t *testing.T) {
		specs := getHelmSpecs(t, nil)
		require.IsType(t, &corev1.ServiceAccount{}, specs[0])
		assert.IsType(t, &rbacv1.ClusterRole{}, specs[1])
		assert.IsType(t, &rbacv1.ClusterRoleBinding{}, specs[2])
		assert.IsType(t, &rbacv1.RoleBinding{}, specs[3])
		assert.IsType(t, &appsv1.DaemonSet{}, specs[4])
	})

	t.Run("By default, no nodeSelector is configured", func(t *testing.T) {
		specs := getHelmSpecs(t, nil)
		require.IsType(t, &appsv1.DaemonSet{}, specs[4])
		ds := specs[4].(*appsv1.DaemonSet)
		assert.Equal(t, map[string]string{}, ds.Spec.Template.Spec.NodeSelector)
	})
	t.Run("By default, affinity is configured", func(t *testing.T) {
		specs := getHelmSpecs(t, nil)
		require.IsType(t, &appsv1.DaemonSet{}, specs[4])
		ds := specs[4].(*appsv1.DaemonSet)
		assert.Equal(t, &corev1.Affinity{
			NodeAffinity: &corev1.NodeAffinity{
				RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
					NodeSelectorTerms: []corev1.NodeSelectorTerm{{
						MatchExpressions: []corev1.NodeSelectorRequirement{{
							Key:      "node-role.kubernetes.io/control-plane",
							Operator: corev1.NodeSelectorOpExists,
						}},
					}},
				},
			},
		}, ds.Spec.Template.Spec.Affinity)
	})
	t.Run("By default, the right tolerations are configured", func(t *testing.T) {
		specs := getHelmSpecs(t, nil)
		require.IsType(t, &appsv1.DaemonSet{}, specs[4])
		ds := specs[4].(*appsv1.DaemonSet)
		assert.Equal(t, []corev1.Toleration{
			{
				Key:    "node.cloudprovider.kubernetes.io/uninitialized",
				Value:  "true",
				Effect: corev1.TaintEffectNoSchedule,
			},
			{
				Key:    "node-role.kubernetes.io/control-plane",
				Effect: corev1.TaintEffectNoSchedule,
			},
		}, ds.Spec.Template.Spec.Tolerations)
	})
	t.Run("By default, auth is based on env", func(t *testing.T) {
		specs := getHelmSpecs(t, []string{"oscSecretFormat=v1"})
		require.IsType(t, &appsv1.DaemonSet{}, specs[4])
		ds := specs[4].(*appsv1.DaemonSet)
		require.Len(t, ds.Spec.Template.Spec.Containers, 1)
		assert.Equal(t, "OSC_ACCESS_KEY", ds.Spec.Template.Spec.Containers[0].Env[0].Name)
		assert.Equal(t, "OSC_SECRET_KEY", ds.Spec.Template.Spec.Containers[0].Env[1].Name)
	})
	t.Run("Using auth from a profile file mounted from a secret", func(t *testing.T) {
		specs := getHelmSpecs(t, []string{"oscCredentialsFromFile=true"})
		require.IsType(t, &appsv1.DaemonSet{}, specs[4])
		ds := specs[4].(*appsv1.DaemonSet)
		require.Len(t, ds.Spec.Template.Spec.Containers, 1)
		for _, env := range ds.Spec.Template.Spec.Containers[0].Env {
			assert.NotEqual(t, "OSC_ACCESS_KEY", env.Name)
			assert.NotEqual(t, "OSC_SECRET_KEY", env.Name)
		}
		assert.Len(t, ds.Spec.Template.Spec.Containers[0].VolumeMounts, 1)
		assert.Len(t, ds.Spec.Template.Spec.Volumes, 1)
	})
	t.Run("Using auth from a profile file not mounted from a secret", func(t *testing.T) {
		specs := getHelmSpecs(t, []string{"oscCredentialsFromFile=true", "oscSecretName="})
		require.IsType(t, &appsv1.DaemonSet{}, specs[4])
		ds := specs[4].(*appsv1.DaemonSet)
		require.Len(t, ds.Spec.Template.Spec.Containers, 1)
		for _, env := range ds.Spec.Template.Spec.Containers[0].Env {
			assert.NotEqual(t, "OSC_ACCESS_KEY", env.Name)
			assert.NotEqual(t, "OSC_SECRET_KEY", env.Name)
		}
		assert.Empty(t, ds.Spec.Template.Spec.Containers[0].VolumeMounts)
		assert.Empty(t, ds.Spec.Template.Spec.Volumes)
	})

	t.Run("By default, the OSC_ENDPOINT_API env var is not set", func(t *testing.T) {
		specs := getHelmSpecs(t, nil)
		require.IsType(t, &appsv1.DaemonSet{}, specs[4])
		ds := specs[4].(*appsv1.DaemonSet)
		require.Len(t, ds.Spec.Template.Spec.Containers, 1)
		for _, env := range ds.Spec.Template.Spec.Containers[0].Env {
			assert.NotEqual(t, "OSC_ENDPOINT_API", env.Name)
		}
	})
	t.Run("OSC_ENDPOINT_API can by set with customEndpoint", func(t *testing.T) {
		specs := getHelmSpecs(t, []string{"customEndpoint=https://api.example.com"})
		require.IsType(t, &appsv1.DaemonSet{}, specs[4])
		ds := specs[4].(*appsv1.DaemonSet)
		require.Len(t, ds.Spec.Template.Spec.Containers, 1)
		assert.Contains(t, ds.Spec.Template.Spec.Containers[0].Env, corev1.EnvVar{
			Name:  "OSC_ENDPOINT_API",
			Value: "https://api.example.com",
		})
	})

	t.Run("By default, the OSC_ENDPOINT_FCU env var is not set", func(t *testing.T) {
		specs := getHelmSpecs(t, nil)
		require.IsType(t, &appsv1.DaemonSet{}, specs[4])
		ds := specs[4].(*appsv1.DaemonSet)
		require.Len(t, ds.Spec.Template.Spec.Containers, 1)
		for _, env := range ds.Spec.Template.Spec.Containers[0].Env {
			assert.NotEqual(t, "OSC_ENDPOINT_FCU", env.Name)
		}
	})

	t.Run("By default, the OSC_ENDPOINT_LBU env var is not set", func(t *testing.T) {
		specs := getHelmSpecs(t, nil)
		require.IsType(t, &appsv1.DaemonSet{}, specs[4])
		ds := specs[4].(*appsv1.DaemonSet)
		require.Len(t, ds.Spec.Template.Spec.Containers, 1)
		for _, env := range ds.Spec.Template.Spec.Containers[0].Env {
			assert.NotEqual(t, "OSC_ENDPOINT_LBU", env.Name)
		}
	})
	t.Run("OSC_ENDPOINT_LBU can by set with customEndpointLbu", func(t *testing.T) {
		specs := getHelmSpecs(t, []string{"customEndpointLbu=https://lbu.example.com"})
		require.IsType(t, &appsv1.DaemonSet{}, specs[4])
		ds := specs[4].(*appsv1.DaemonSet)
		require.Len(t, ds.Spec.Template.Spec.Containers, 1)
		assert.Contains(t, ds.Spec.Template.Spec.Containers[0].Env, corev1.EnvVar{
			Name:  "OSC_ENDPOINT_LBU",
			Value: "https://lbu.example.com",
		})
	})

	t.Run("By default, neither HTTPS_PROXY env var nor NO_PROXY are set", func(t *testing.T) {
		specs := getHelmSpecs(t, nil)
		require.IsType(t, &appsv1.DaemonSet{}, specs[4])
		ds := specs[4].(*appsv1.DaemonSet)
		require.Len(t, ds.Spec.Template.Spec.Containers, 1)
		for _, env := range ds.Spec.Template.Spec.Containers[0].Env {
			assert.NotEqual(t, "HTTPS_PROXY", env.Name)
			assert.NotEqual(t, "NO_PROXY", env.Name)
		}
	})
	t.Run("HTTPS_PROXY can by set with httpsProxy", func(t *testing.T) {
		specs := getHelmSpecs(t, []string{"httpsProxy=https://proxy.example.com"})
		require.IsType(t, &appsv1.DaemonSet{}, specs[4])
		ds := specs[4].(*appsv1.DaemonSet)
		require.Len(t, ds.Spec.Template.Spec.Containers, 1)
		assert.Contains(t, ds.Spec.Template.Spec.Containers[0].Env, corev1.EnvVar{
			Name:  "HTTPS_PROXY",
			Value: "https://proxy.example.com",
		})
	})
	t.Run("NO_PROXY can by set with noProxy only if httpsProxy is set", func(t *testing.T) {
		{
			specs := getHelmSpecs(t, []string{"noProxy=https://direct.example.com"})
			require.IsType(t, &appsv1.DaemonSet{}, specs[4])
			ds := specs[4].(*appsv1.DaemonSet)
			require.Len(t, ds.Spec.Template.Spec.Containers, 1)
			for _, env := range ds.Spec.Template.Spec.Containers[0].Env {
				assert.NotEqual(t, "NO_PROXY", env.Name)
			}
		}
		{
			specs := getHelmSpecs(t, []string{
				"httpsProxy=https://proxy.example.com",
				"noProxy=https://direct.example.com",
			})
			require.IsType(t, &appsv1.DaemonSet{}, specs[4])
			ds := specs[4].(*appsv1.DaemonSet)
			require.Len(t, ds.Spec.Template.Spec.Containers, 1)
			assert.Contains(t, ds.Spec.Template.Spec.Containers[0].Env, corev1.EnvVar{
				Name:  "HTTPS_PROXY",
				Value: "https://proxy.example.com",
			})
			assert.Contains(t, ds.Spec.Template.Spec.Containers[0].Env, corev1.EnvVar{
				Name:  "NO_PROXY",
				Value: "https://direct.example.com",
			})
		}
	})

	t.Run("By default, no CA bundle is mounted", func(t *testing.T) {
		specs := getHelmSpecs(t, nil)
		require.IsType(t, &appsv1.DaemonSet{}, specs[4])
		ds := specs[4].(*appsv1.DaemonSet)
		require.Len(t, ds.Spec.Template.Spec.Containers, 1)
		for _, mount := range ds.Spec.Template.Spec.Containers[0].VolumeMounts {
			assert.NotEqual(t, "/etc/ssl/certs", mount.MountPath)
		}
	})
	t.Run("A custom CA bundle can be set", func(t *testing.T) {
		specs := getHelmSpecs(t, []string{"caBundle.name=foo", "caBundle.key=bar"})
		require.IsType(t, &appsv1.DaemonSet{}, specs[4])
		ds := specs[4].(*appsv1.DaemonSet)
		require.Len(t, ds.Spec.Template.Spec.Containers, 1)
		assert.Contains(t, ds.Spec.Template.Spec.Containers[0].VolumeMounts, corev1.VolumeMount{
			Name:      "ca-bundle",
			MountPath: "/etc/ssl/certs",
			ReadOnly:  true,
		})
		assert.Contains(t, ds.Spec.Template.Spec.Volumes, corev1.Volume{
			Name: "ca-bundle",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: "foo",
					Items: []corev1.KeyToPath{{
						Key:  "bar",
						Path: "ca-certificates.crt",
					}},
				},
			},
		})
	})

	t.Run("By default, no resource limits/requests are set", func(t *testing.T) {
		specs := getHelmSpecs(t, nil)
		require.IsType(t, &appsv1.DaemonSet{}, specs[4])
		ds := specs[4].(*appsv1.DaemonSet)
		require.Len(t, ds.Spec.Template.Spec.Containers, 1)
		assert.Equal(t, corev1.ResourceRequirements{}, ds.Spec.Template.Spec.Containers[0].Resources)
	})
	t.Run("Resource limits can be set", func(t *testing.T) {
		specs := getHelmSpecs(t, []string{"resources.limits.cpu=100m", "resources.limits.memory=1G"})
		require.IsType(t, &appsv1.DaemonSet{}, specs[4])
		ds := specs[4].(*appsv1.DaemonSet)
		require.Len(t, ds.Spec.Template.Spec.Containers, 1)
		assert.Equal(t, corev1.ResourceRequirements{
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("100m"),
				corev1.ResourceMemory: resource.MustParse("1G"),
			},
		}, ds.Spec.Template.Spec.Containers[0].Resources)
	})
	t.Run("Resource requests can be set", func(t *testing.T) {
		specs := getHelmSpecs(t, []string{"resources.requests.cpu=100m", "resources.requests.memory=1G"})
		require.IsType(t, &appsv1.DaemonSet{}, specs[4])
		ds := specs[4].(*appsv1.DaemonSet)
		require.Len(t, ds.Spec.Template.Spec.Containers, 1)
		assert.Equal(t, corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("100m"),
				corev1.ResourceMemory: resource.MustParse("1G"),
			},
		}, ds.Spec.Template.Spec.Containers[0].Resources)
	})

	t.Run("The container image may be changed", func(t *testing.T) {
		specs := getHelmSpecs(t, []string{"image.repository=foo.bar", "image.tag=v42"})
		require.IsType(t, &appsv1.DaemonSet{}, specs[4])
		ds := specs[4].(*appsv1.DaemonSet)
		require.Len(t, ds.Spec.Template.Spec.Containers, 1)
		assert.Equal(t, "foo.bar:v42", ds.Spec.Template.Spec.Containers[0].Image)
	})
	t.Run("The container image pull poliy may be changed", func(t *testing.T) {
		specs := getHelmSpecs(t, []string{"image.pullPolicy=foo"})
		require.IsType(t, &appsv1.DaemonSet{}, specs[4])
		ds := specs[4].(*appsv1.DaemonSet)
		require.Len(t, ds.Spec.Template.Spec.Containers, 1)
		assert.Equal(t, corev1.PullPolicy("foo"), ds.Spec.Template.Spec.Containers[0].ImagePullPolicy)
	})
	t.Run("The ccm log verbosity may be changed", func(t *testing.T) {
		specs := getHelmSpecs(t, []string{"verbose=42"})
		require.IsType(t, &appsv1.DaemonSet{}, specs[4])
		ds := specs[4].(*appsv1.DaemonSet)
		require.Len(t, ds.Spec.Template.Spec.Containers, 1)
		require.Len(t, ds.Spec.Template.Spec.Containers[0].Command, 4)
		assert.Equal(t, "-v=42", ds.Spec.Template.Spec.Containers[0].Command[3])
	})
	t.Run("Extra tags can be set", func(t *testing.T) {
		specs := getHelmSpecs(t, []string{"extraLoadBalancerTags.key1=value1", "extraLoadBalancerTags.key2=value2"})
		require.IsType(t, &appsv1.DaemonSet{}, specs[4])
		ds := specs[4].(*appsv1.DaemonSet)
		require.Len(t, ds.Spec.Template.Spec.Containers, 1)
		require.Len(t, ds.Spec.Template.Spec.Containers[0].Command, 5)
		assert.Equal(t, "--extra-loadbalancer-tags=key1=value1,key2=value2", ds.Spec.Template.Spec.Containers[0].Command[4])
	})
	t.Run("The secret containing access keys may be changed", func(t *testing.T) {
		{
			specs := getHelmSpecs(t, []string{"oscSecretName=foo"})
			require.IsType(t, &appsv1.DaemonSet{}, specs[4])
			ds := specs[4].(*appsv1.DaemonSet)
			require.Len(t, ds.Spec.Template.Spec.Containers, 1)
			for _, e := range ds.Spec.Template.Spec.Containers[0].Env {
				if e.ValueFrom == nil || e.ValueFrom.SecretKeyRef == nil {
					continue
				}
				assert.Equal(t, "foo", e.ValueFrom.SecretKeyRef.Name)
			}
		}
		{
			specs := getHelmSpecs(t, []string{"oscSecretName=foo", "oscSecretFormat=v1"})
			require.IsType(t, &appsv1.DaemonSet{}, specs[4])
			ds := specs[4].(*appsv1.DaemonSet)
			require.Len(t, ds.Spec.Template.Spec.Containers, 1)
			for _, e := range ds.Spec.Template.Spec.Containers[0].Env {
				if e.ValueFrom == nil || e.ValueFrom.SecretKeyRef == nil {
					continue
				}
				assert.Equal(t, "foo", e.ValueFrom.SecretKeyRef.Name)
			}
		}
	})
}
