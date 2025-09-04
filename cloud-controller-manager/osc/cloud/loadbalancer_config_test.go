package cloud_test

import (
	"testing"

	"github.com/outscale/cloud-provider-osc/cloud-controller-manager/osc/cloud"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilnet "k8s.io/utils/net"
)

func ipNetSet(nets ...string) utilnet.IPNetSet {
	set, _ := utilnet.ParseIPNets(nets...)
	return set
}

func TestNewLoadBalancer(t *testing.T) {
	tcs := []struct {
		name string
		svc  *v1.Service
		tags map[string]string
		lb   *cloud.LoadBalancer
		err  bool
	}{{
		name: "a simple LBU with default values",
		svc: &v1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name: "foo",
				UID:  "012-3456-789",
			},
			Spec: v1.ServiceSpec{
				SessionAffinity: v1.ServiceAffinityNone,
				Ports:           []v1.ServicePort{{Protocol: v1.ProtocolTCP, Port: 42, NodePort: 43}},
			},
		},
		lb: &cloud.LoadBalancer{
			Name:        "0123456789",
			ServiceName: "foo",
			Listeners: []cloud.Listener{{
				Port:        42,
				BackendPort: 43,
			}},
			ListenerDefaults: cloud.ListenerDefaults{
				SSLPorts: []string{"*"},
			},
			TargetRole: cloud.DefaultLoadBalancerConfiguration.TargetRole,
			HealthCheck: cloud.HealthCheck{
				Port:               43,
				Protocol:           "tcp",
				Interval:           cloud.DefaultLoadBalancerConfiguration.HealthCheck.Interval,
				Timeout:            cloud.DefaultLoadBalancerConfiguration.HealthCheck.Timeout,
				HealthyThreshold:   cloud.DefaultLoadBalancerConfiguration.HealthCheck.HealthyThreshold,
				UnhealthyThreshold: cloud.DefaultLoadBalancerConfiguration.HealthCheck.UnhealthyThreshold,
			},
			AllowFrom:  ipNetSet("0.0.0.0/0"),
			Connection: cloud.Connection{IdleTimeout: 60},
		},
	}, {
		name: "Annotations are loaded",
		svc: &v1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name: "foo",
				UID:  "012-3456-789",
				Annotations: map[string]string{
					"service.beta.kubernetes.io/osc-load-balancer-name":                            "name-foo",
					"service.beta.kubernetes.io/osc-load-balancer-internal":                        "true",
					"service.beta.kubernetes.io/osc-load-balancer-ip-pool":                         "pool-foo",
					"service.beta.kubernetes.io/osc-load-balancer-ip-id":                           "ip-foo",
					"service.beta.kubernetes.io/osc-load-balancer-subnet-id":                       "subnet-foo",
					"service.beta.kubernetes.io/osc-load-balancer-security-group":                  "sg-foo",
					"service.beta.kubernetes.io/osc-load-balancer-extra-security-groups":           "sg-bar,sg-baz",
					"service.beta.kubernetes.io/osc-load-balancer-additional-resource-tags":        "foo=bar,foobar=barbar",
					"service.beta.kubernetes.io/osc-load-balancer-target-role":                     "role-foo",
					"service.beta.kubernetes.io/osc-load-balancer-proxy-protocol":                  "*",
					"service.beta.kubernetes.io/osc-load-balancer-backend-protocol":                "protocol-foo",
					"service.beta.kubernetes.io/osc-load-balancer-ssl-cert":                        "orn:foo",
					"service.beta.kubernetes.io/osc-load-balancer-ssl-ports":                       "443,8443",
					"service.beta.kubernetes.io/osc-load-balancer-healthcheck-healthy-threshold":   "42",
					"service.beta.kubernetes.io/osc-load-balancer-healthcheck-unhealthy-threshold": "43",
					"service.beta.kubernetes.io/osc-load-balancer-healthcheck-timeout":             "44",
					"service.beta.kubernetes.io/osc-load-balancer-healthcheck-interval":            "45",
					"service.beta.kubernetes.io/osc-load-balancer-healthcheck-port":                "46",
					"service.beta.kubernetes.io/osc-load-balancer-healthcheck-protocol":            "ssl",
					"service.beta.kubernetes.io/osc-load-balancer-healthcheck-path":                "/healthz",
					"service.beta.kubernetes.io/osc-load-balancer-access-log-enabled":              "true",
					"service.beta.kubernetes.io/osc-load-balancer-access-log-emit-interval":        "60",
					"service.beta.kubernetes.io/osc-load-balancer-access-log-oos-bucket-name":      "bucket-foo",
					"service.beta.kubernetes.io/osc-load-balancer-access-log-oos-bucket-prefix":    "prefix-foo",
					"service.beta.kubernetes.io/osc-load-balancer-connection-draining-enabled":     "true",
					"service.beta.kubernetes.io/osc-load-balancer-connection-draining-timeout":     "47",
					"service.beta.kubernetes.io/osc-load-balancer-connection-idle-timeout":         "48",
					"service.beta.kubernetes.io/load-balancer-source-ranges":                       "192.0.2.0/24,198.51.100.0/24",
				},
			},
			Spec: v1.ServiceSpec{
				SessionAffinity: v1.ServiceAffinityNone,
				Ports:           []v1.ServicePort{{Protocol: v1.ProtocolTCP, Port: 42, NodePort: 43}},
			},
		},
		lb: &cloud.LoadBalancer{
			Name:                     "name-foo",
			ServiceName:              "foo",
			Internal:                 true,
			PublicIPPool:             "pool-foo",
			PublicIPID:               "ip-foo",
			SubnetID:                 "subnet-foo",
			SecurityGroups:           []string{"sg-foo"},
			AdditionalSecurityGroups: []string{"sg-bar", "sg-baz"},
			TargetRole:               "role-foo",
			Listeners: []cloud.Listener{{
				Port:        42,
				BackendPort: 43,
			}},
			ListenerDefaults: cloud.ListenerDefaults{
				SSLCertificate:  "orn:foo",
				SSLPorts:        []string{"443", "8443"},
				BackendProtocol: "protocol-foo",
				ProxyProtocol:   []string{"*"},
			},
			HealthCheck: cloud.HealthCheck{
				Interval:           45,
				Timeout:            44,
				HealthyThreshold:   42,
				UnhealthyThreshold: 43,
				Port:               46,
				Protocol:           "ssl",
				Path:               "/healthz",
			},
			Connection: cloud.Connection{
				ConnectionDraining:        true,
				ConnectionDrainingTimeout: 47,
				IdleTimeout:               48,
			},
			AccessLog: cloud.AccessLog{
				Enabled:      true,
				EmitInterval: 60,
				BucketName:   "bucket-foo",
				BucketPrefix: "prefix-foo",
			},
			Tags: map[string]string{
				"foo":    "bar",
				"foobar": "barbar",
			},
			AllowFrom: ipNetSet("192.0.2.0/24", "198.51.100.0/24"),
		},
	}, {
		name: "Tags are merged with the global config",
		tags: map[string]string{
			"foobar": "bazbar",
			"foobaz": "bazbaz",
		},
		svc: &v1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name: "foo",
				UID:  "012-3456-789",
				Annotations: map[string]string{
					"service.beta.kubernetes.io/osc-load-balancer-additional-resource-tags": "foo=bar,foobar=barbar",
				},
			},
			Spec: v1.ServiceSpec{
				SessionAffinity: v1.ServiceAffinityNone,
				Ports:           []v1.ServicePort{{Protocol: v1.ProtocolTCP, Port: 42, NodePort: 43}},
			},
		},
		lb: &cloud.LoadBalancer{
			Name:        "0123456789",
			ServiceName: "foo",
			Listeners: []cloud.Listener{{
				Port:        42,
				BackendPort: 43,
			}},
			ListenerDefaults: cloud.ListenerDefaults{
				SSLPorts: []string{"*"},
			},
			TargetRole: cloud.DefaultLoadBalancerConfiguration.TargetRole,
			HealthCheck: cloud.HealthCheck{
				Port:               43,
				Protocol:           "tcp",
				Interval:           cloud.DefaultLoadBalancerConfiguration.HealthCheck.Interval,
				Timeout:            cloud.DefaultLoadBalancerConfiguration.HealthCheck.Timeout,
				HealthyThreshold:   cloud.DefaultLoadBalancerConfiguration.HealthCheck.HealthyThreshold,
				UnhealthyThreshold: cloud.DefaultLoadBalancerConfiguration.HealthCheck.UnhealthyThreshold,
			},
			AllowFrom:  ipNetSet("0.0.0.0/0"),
			Connection: cloud.Connection{IdleTimeout: 60},
			Tags: map[string]string{
				"foo":    "bar",
				"foobar": "barbar",
				"foobaz": "bazbaz",
			},
		},
	}, {
		name: "AWS annotations are loaded",
		svc: &v1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name: "foo",
				UID:  "012-3456-789",
				Annotations: map[string]string{
					"service.beta.kubernetes.io/aws-load-balancer-name":                            "name-foo",
					"service.beta.kubernetes.io/aws-load-balancer-internal":                        "true",
					"service.beta.kubernetes.io/aws-load-balancer-subnet-id":                       "subnet-foo",
					"service.beta.kubernetes.io/aws-load-balancer-security-groups":                 "sg-foo",
					"service.beta.kubernetes.io/aws-load-balancer-extra-security-groups":           "sg-bar,sg-baz",
					"service.beta.kubernetes.io/aws-load-balancer-additional-resource-tags":        "foo=bar,foobar=barbar",
					"service.beta.kubernetes.io/aws-load-balancer-target-role":                     "role-foo",
					"service.beta.kubernetes.io/aws-load-balancer-proxy-protocol":                  "*",
					"service.beta.kubernetes.io/aws-load-balancer-backend-protocol":                "protocol-foo",
					"service.beta.kubernetes.io/aws-load-balancer-ssl-cert":                        "orn:foo",
					"service.beta.kubernetes.io/aws-load-balancer-ssl-ports":                       "443,8443",
					"service.beta.kubernetes.io/aws-load-balancer-healthcheck-healthy-threshold":   "42",
					"service.beta.kubernetes.io/aws-load-balancer-healthcheck-unhealthy-threshold": "43",
					"service.beta.kubernetes.io/aws-load-balancer-healthcheck-timeout":             "44",
					"service.beta.kubernetes.io/aws-load-balancer-healthcheck-interval":            "45",
					"service.beta.kubernetes.io/aws-load-balancer-healthcheck-port":                "46",
					"service.beta.kubernetes.io/aws-load-balancer-healthcheck-protocol":            "ssl",
					"service.beta.kubernetes.io/aws-load-balancer-healthcheck-path":                "/healthz",
					"service.beta.kubernetes.io/aws-load-balancer-access-log-enabled":              "true",
					"service.beta.kubernetes.io/aws-load-balancer-access-log-emit-interval":        "60",
					"service.beta.kubernetes.io/aws-load-balancer-access-log-s3-bucket-name":       "bucket-foo",
					"service.beta.kubernetes.io/aws-load-balancer-access-log-s3-bucket-prefix":     "prefix-foo",
					"service.beta.kubernetes.io/aws-load-balancer-connection-draining-enabled":     "true",
					"service.beta.kubernetes.io/aws-load-balancer-connection-draining-timeout":     "47",
					"service.beta.kubernetes.io/aws-load-balancer-connection-idle-timeout":         "48",
					"service.beta.kubernetes.io/load-balancer-source-ranges":                       "192.0.2.0/24,198.51.100.0/24",
				},
			},
			Spec: v1.ServiceSpec{
				SessionAffinity: v1.ServiceAffinityNone,
				Ports:           []v1.ServicePort{{Protocol: v1.ProtocolTCP, Port: 42, NodePort: 43}},
			},
		},
		lb: &cloud.LoadBalancer{
			Name:                     "name-foo",
			ServiceName:              "foo",
			Internal:                 true,
			SubnetID:                 "subnet-foo",
			SecurityGroups:           []string{"sg-foo"},
			AdditionalSecurityGroups: []string{"sg-bar", "sg-baz"},
			TargetRole:               "role-foo",
			Listeners: []cloud.Listener{{
				Port:        42,
				BackendPort: 43,
			}},
			ListenerDefaults: cloud.ListenerDefaults{
				SSLCertificate:  "orn:foo",
				SSLPorts:        []string{"443", "8443"},
				BackendProtocol: "protocol-foo",
				ProxyProtocol:   []string{"*"},
			},
			HealthCheck: cloud.HealthCheck{
				Interval:           45,
				Timeout:            44,
				HealthyThreshold:   42,
				UnhealthyThreshold: 43,
				Port:               46,
				Protocol:           "ssl",
				Path:               "/healthz",
			},
			Connection: cloud.Connection{
				ConnectionDraining:        true,
				ConnectionDrainingTimeout: 47,
				IdleTimeout:               48,
			},
			AccessLog: cloud.AccessLog{
				Enabled:      true,
				EmitInterval: 60,
				BucketName:   "bucket-foo",
				BucketPrefix: "prefix-foo",
			},
			Tags: map[string]string{
				"foo":    "bar",
				"foobar": "barbar",
			},
			AllowFrom: ipNetSet("192.0.2.0/24", "198.51.100.0/24"),
		},
	}, {
		name: "Source ranges can be set in the spec",
		svc: &v1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name: "foo",
				UID:  "012-3456-789",
			},
			Spec: v1.ServiceSpec{
				SessionAffinity:          v1.ServiceAffinityNone,
				Ports:                    []v1.ServicePort{{Protocol: v1.ProtocolTCP, Port: 42, NodePort: 43}},
				LoadBalancerSourceRanges: []string{"192.0.2.0/24", "198.51.100.0/24"},
			},
		},
		lb: &cloud.LoadBalancer{
			Name:        "0123456789",
			ServiceName: "foo",
			Listeners: []cloud.Listener{{
				Port:        42,
				BackendPort: 43,
			}},
			ListenerDefaults: cloud.ListenerDefaults{
				SSLPorts: []string{"*"},
			},
			TargetRole: cloud.DefaultLoadBalancerConfiguration.TargetRole,
			HealthCheck: cloud.HealthCheck{
				Port:               43,
				Protocol:           "tcp",
				Interval:           cloud.DefaultLoadBalancerConfiguration.HealthCheck.Interval,
				Timeout:            cloud.DefaultLoadBalancerConfiguration.HealthCheck.Timeout,
				HealthyThreshold:   cloud.DefaultLoadBalancerConfiguration.HealthCheck.HealthyThreshold,
				UnhealthyThreshold: cloud.DefaultLoadBalancerConfiguration.HealthCheck.UnhealthyThreshold,
			},
			AllowFrom:  ipNetSet("192.0.2.0/24", "198.51.100.0/24"),
			Connection: cloud.Connection{IdleTimeout: 60},
		},
	}}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			tags := tc.tags
			if tags == nil {
				tags = map[string]string{}
			}
			lb, err := cloud.NewLoadBalancer(tc.svc, tags)
			if tc.err {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.lb, lb)
			}
		})
	}
}
