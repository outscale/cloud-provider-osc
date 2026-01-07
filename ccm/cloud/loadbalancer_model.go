/*
SPDX-FileCopyrightText: 2025 Outscale SAS <opensource@outscale.com>

SPDX-License-Identifier: BSD-3-Clause
*/
package cloud

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"dario.cat/mergo"
	"github.com/aws/aws-sdk-go/aws"         //nolint:staticcheck
	"github.com/aws/aws-sdk-go/service/elb" //nolint:staticcheck
	"github.com/go-viper/mapstructure/v2"
	"github.com/outscale/goutils/k8s/role"
	"github.com/outscale/goutils/sdk/ptr"
	"github.com/outscale/osc-sdk-go/v3/pkg/osc"
	corev1 "k8s.io/api/core/v1"
	servicehelpers "k8s.io/cloud-provider/service/helpers"
	utilnet "k8s.io/utils/net"
)

var (
	// DefaultLoadBalancerConfiguration is the default LoadBalancher configuration.
	DefaultLoadBalancerConfiguration = LoadBalancer{
		Type:         LBU,
		DeviceNumber: 1,
		TargetRole:   role.Worker,
		HealthCheck: HealthCheck{
			HealthyThreshold:   2,
			UnhealthyThreshold: 3,
			Timeout:            5,
			Interval:           10,
		},
		Connection: Connection{
			IdleTimeout: 60,
		},
		ListenerDefaults: ListenerDefaults{
			SSLPorts: []string{"*"},
		},
		IngressAddress: Hostname,
	}
)

// HealthCheck is the healcheck configuration.
type HealthCheck struct {
	Interval           int `annotation:"osc-load-balancer-healthcheck-interval"`
	Timeout            int `annotation:"osc-load-balancer-healthcheck-timeout"`
	HealthyThreshold   int `annotation:"osc-load-balancer-healthcheck-healthy-threshold"`
	UnhealthyThreshold int `annotation:"osc-load-balancer-healthcheck-unhealthy-threshold"`

	Port     int    `annotation:"osc-load-balancer-healthcheck-port"`
	Protocol string `annotation:"osc-load-balancer-healthcheck-protocol"`
	Path     string `annotation:"osc-load-balancer-healthcheck-path"`
}

// Connection defines connection handling parameters.
type Connection struct {
	ConnectionDraining        bool  `annotation:"osc-load-balancer-connection-draining-enabled"`
	ConnectionDrainingTimeout int64 `annotation:"osc-load-balancer-connection-draining-timeout"`

	IdleTimeout int64 `annotation:"osc-load-balancer-connection-idle-timeout"`
}

// ListenerDefaults defines some default values for all listeners.
type ListenerDefaults struct {
	BackendProtocol string   `annotation:"osc-load-balancer-backend-protocol"`
	ProxyProtocol   []string `annotation:"osc-load-balancer-proxy-protocol"`
	SSLCertificate  string   `annotation:"osc-load-balancer-ssl-cert"`
	SSLPorts        []string `annotation:"osc-load-balancer-ssl-ports"`
}

func isPortIn(port int, allowed []string) bool {
	if slices.Contains(allowed, "*") {
		return true
	}
	sport := strconv.Itoa(port)
	return slices.Contains(allowed, sport)
}

// Listener defines a listener.
type Listener struct {
	Port        int
	BackendPort int
}

// AccessLog defines the access log config.
type AccessLog struct {
	Enabled      bool   `annotation:"osc-load-balancer-access-log-enabled"`
	EmitInterval int64  `annotation:"osc-load-balancer-access-log-emit-interval"`
	BucketName   string `annotation:"osc-load-balancer-access-log-oos-bucket-name"`
	BucketPrefix string `annotation:"osc-load-balancer-access-log-oos-bucket-prefix"`
}

type IngressAddress string

const (
	Hostname IngressAddress = "hostname"
	IP       IngressAddress = "ip"
	Both     IngressAddress = "both"
)

func (i IngressAddress) NeedHostname() bool {
	return i == Hostname || i == Both
}

func (i IngressAddress) NeedIP() bool {
	return i == IP || i == Both
}

type LoadBalancerType string

const (
	LBU LoadBalancerType = "LBU"
	VIP LoadBalancerType = "VIP"
)

// LoadBalancer defines a load-balancer.
type LoadBalancer struct {
	Name                     string `annotation:"osc-load-balancer-name"`
	ServiceUID               string
	ServiceName              string
	Type                     LoadBalancerType `annotation:"osc-load-balancer-type"`
	Internal                 bool             `annotation:"osc-load-balancer-internal"`
	DeviceNumber             int              `annotation:"osc-load-balancer-device-number"`
	PublicIPPool             string           `annotation:"osc-load-balancer-ip-pool"`
	PublicIPID               string           `annotation:"osc-load-balancer-ip-id"`
	publicIP                 string
	SubnetID                 string `annotation:"osc-load-balancer-subnet-id"`
	NetID                    string
	SecurityGroups           []string          `annotation:"osc-load-balancer-security-group"`
	AdditionalSecurityGroups []string          `annotation:"osc-load-balancer-extra-security-groups"`
	TargetRole               role.Role         `annotation:"osc-load-balancer-target-role"`
	TargetNodesLabels        map[string]string `annotation:"osc-load-balancer-target-node-labels"`
	Tags                     map[string]string `annotation:"osc-load-balancer-additional-resource-tags"`
	HealthCheck              HealthCheck       `annotation:",squash"`
	ListenerDefaults         ListenerDefaults  `annotation:",squash"`
	Listeners                []Listener
	Connection               Connection `annotation:",squash"`
	SessionAffinity          string
	AccessLog                AccessLog `annotation:",squash"`
	AllowFrom                utilnet.IPNetSet
	IngressAddress           IngressAddress             `annotation:"osc-load-balancer-ingress-address"`
	IPMode                   *corev1.LoadBalancerIPMode `annotation:"osc-load-balancer-ingress-ipmode"`
}

var reName = regexp.MustCompile("^[a-zA-Z0-9-]+$")

// NewLoadBalancer creates a new LoadBalancer instance from a Kubernetes Service.
func NewLoadBalancer(svc *corev1.Service, addTags map[string]string) (*LoadBalancer, error) {
	if svc.Spec.SessionAffinity != corev1.ServiceAffinityNone {
		return nil, fmt.Errorf("unsupported SessionAffinity %q", svc.Spec.SessionAffinity)
	}
	if len(svc.Spec.Ports) == 0 {
		return nil, errors.New("service has no ports")
	}
	if svc.Spec.LoadBalancerIP != "" {
		return nil, errors.New("LoadBalancerIP cannot be specified")
	}

	lb := &LoadBalancer{
		ServiceUID:  string(svc.UID),
		ServiceName: svc.Name,
	}
	err := lb.decodeAnnotations(svc.Annotations)
	if err != nil {
		return nil, err
	}
	if lb.Name == "" {
		lb.Name = strings.ReplaceAll(string(svc.UID), "-", "")
	} else {
		if !reName.MatchString(lb.Name) {
			return nil, fmt.Errorf("invalid load balancer name %q, only letters, numbers and - are allowed", lb.Name)
		}
		if len(lb.Name) > 32 {
			lb.Name = lb.Name[:32]
		}
	}
	for k, v := range addTags {
		if _, found := lb.Tags[k]; !found {
			lb.Tags[k] = v
		}
	}

	for _, port := range svc.Spec.Ports {
		if port.Protocol != corev1.ProtocolTCP {
			return nil, errors.New("only TCP load balancers are supported")
		}
		if port.NodePort == 0 {
			continue
		}
		lb.Listeners = append(lb.Listeners, Listener{
			Port:        int(port.Port),
			BackendPort: int(port.NodePort),
		})
	}

	lb.AllowFrom, err = servicehelpers.GetLoadBalancerSourceRanges(svc)
	if err != nil {
		return nil, err
	}

	path, port := servicehelpers.GetServiceHealthCheckPathPort(svc)
	switch {
	case lb.HealthCheck.Port != 0 && lb.HealthCheck.Protocol != "":
	case lb.HealthCheck.Port != 0:
		if lb.HealthCheck.Path != "" {
			lb.HealthCheck.Protocol = "http"
		} else {
			lb.HealthCheck.Protocol = "tcp"
		}
	case path != "":
		lb.HealthCheck.Path = path
		lb.HealthCheck.Port = int(port)
		lb.HealthCheck.Protocol = "http"
	case len(lb.Listeners) == 0:
	default:
		lb.HealthCheck.Port = lb.Listeners[0].BackendPort
		lb.HealthCheck.Protocol = "tcp"
	}
	// set defaults
	err = mergo.Merge(lb, DefaultLoadBalancerConfiguration)
	if err != nil {
		return nil, fmt.Errorf("unable to set defaults: %w", err)
	}
	if lb.IngressAddress.NeedIP() && lb.IPMode == nil {
		lb.IPMode = ptr.To(corev1.LoadBalancerIPModeProxy)
	}

	return lb, nil
}

var mappings = map[string]string{
	"service.beta.kubernetes.io/aws-load-balancer-security-groups": "service.beta.kubernetes.io/osc-load-balancer-security-group",
}

// decodeAnnotations decodes annotations to the LoadBalancer struct.
func (l *LoadBalancer) decodeAnnotations(annotations map[string]string) error {
	// compatibility mappings
	for k, nk := range mappings {
		if v, ok := annotations[k]; ok {
			annotations[nk] = v
		}
	}

	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		MatchName:        matchAnnotationName,
		DecodeHook:       mapstructure.ComposeDecodeHookFunc(mapstructure.StringToSliceHookFunc(","), stringToMapHookFunc()),
		TagName:          "annotation",
		WeaklyTypedInput: true,
		Result:           &l,
		Squash:           true,
	})
	if err == nil {
		err = decoder.Decode(annotations)
	}
	if err != nil {
		return fmt.Errorf("unable to decode annotations: %w", err)
	}
	return nil
}

func (l *LoadBalancer) elbAttributes() *elb.LoadBalancerAttributes {
	attrs := &elb.LoadBalancerAttributes{
		ConnectionSettings: &elb.ConnectionSettings{},
		ConnectionDraining: &elb.ConnectionDraining{Enabled: aws.Bool(l.Connection.ConnectionDraining)},
		AccessLog:          &elb.AccessLog{Enabled: aws.Bool(l.AccessLog.Enabled)},
	}
	if l.Connection.ConnectionDraining && l.Connection.ConnectionDrainingTimeout > 0 {
		attrs.ConnectionDraining.Timeout = aws.Int64(l.Connection.ConnectionDrainingTimeout)
	}
	if l.Connection.IdleTimeout > 0 {
		attrs.ConnectionSettings.IdleTimeout = aws.Int64(l.Connection.IdleTimeout)
	}
	if l.AccessLog.Enabled {
		if l.AccessLog.EmitInterval > 0 {
			attrs.AccessLog.EmitInterval = aws.Int64(l.AccessLog.EmitInterval)
		}
		if l.AccessLog.BucketName != "" {
			attrs.AccessLog.S3BucketName = aws.String(l.AccessLog.BucketName)
		}
		if l.AccessLog.BucketPrefix != "" {
			attrs.AccessLog.S3BucketPrefix = aws.String(l.AccessLog.BucketPrefix)
		}
	}
	return attrs
}

func (l *LoadBalancer) listeners() []osc.ListenerForCreation {
	lst := make([]osc.ListenerForCreation, 0, len(l.Listeners))
	for _, lstnr := range l.Listeners {
		olstnr := osc.ListenerForCreation{
			LoadBalancerPort: lstnr.Port,
			BackendPort:      lstnr.BackendPort,
		}
		var protocol string
		backendProtocol := strings.ToLower(l.ListenerDefaults.BackendProtocol)
		if l.ListenerDefaults.SSLCertificate != "" && isPortIn(lstnr.Port, l.ListenerDefaults.SSLPorts) {
			switch backendProtocol {
			case "http", "https":
				protocol = "https"
			case "":
				protocol = "ssl"
				backendProtocol = "tcp"
			default:
				protocol = "ssl"
			}
			olstnr.ServerCertificateId = ptr.To(l.ListenerDefaults.SSLCertificate)
		} else {
			switch backendProtocol {
			case "http":
				protocol = "http"
			case "":
				protocol = "tcp"
				backendProtocol = "tcp"
			default:
				protocol = "tcp"
			}
		}

		olstnr.LoadBalancerProtocol = strings.ToUpper(protocol)
		olstnr.BackendProtocol = ptr.To(strings.ToUpper(backendProtocol))
		lst = append(lst, olstnr)
	}
	return lst
}

func (l *LoadBalancer) healthCheck() *osc.HealthCheck {
	if l.HealthCheck.Port == 0 {
		return nil
	}
	hc := &osc.HealthCheck{
		Port:               l.HealthCheck.Port,
		Protocol:           strings.ToUpper(l.HealthCheck.Protocol),
		CheckInterval:      l.HealthCheck.Interval,
		Timeout:            l.HealthCheck.Timeout,
		HealthyThreshold:   l.HealthCheck.HealthyThreshold,
		UnhealthyThreshold: l.HealthCheck.UnhealthyThreshold,
	}
	switch hc.Protocol {
	case "HTTP", "HTTPS":
		if l.HealthCheck.Path != "" {
			hc.Path = ptr.To(l.HealthCheck.Path)
		}
	default:
	}
	return hc
}

type lbStatus struct {
	host           string
	ip             *string
	tags           []osc.ResourceTag
	securityGroups []string
}
