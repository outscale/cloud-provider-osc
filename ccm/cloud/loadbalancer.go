/*
SPDX-FileCopyrightText: 2025 Outscale SAS <opensource@outscale.com>

SPDX-License-Identifier: BSD-3-Clause
*/
package cloud

import (
	"context"
	"errors"
	"fmt"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"dario.cat/mergo"
	"github.com/aws/aws-sdk-go/aws"         //nolint:staticcheck
	"github.com/aws/aws-sdk-go/service/elb" //nolint:staticcheck
	"github.com/go-viper/mapstructure/v2"
	"github.com/outscale/cloud-provider-osc/ccm/oapi"
	"github.com/outscale/goutils/k8s/role"
	"github.com/outscale/goutils/k8s/sdk"
	"github.com/outscale/goutils/k8s/tags"
	"github.com/outscale/goutils/sdk/ptr"
	"github.com/outscale/osc-sdk-go/v3/pkg/osc"
	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	controllerapi "k8s.io/cloud-provider/api"
	servicehelpers "k8s.io/cloud-provider/service/helpers"
	"k8s.io/klog/v2"
	utilnet "k8s.io/utils/net"
)

const (
	// proxyProtocolPolicyName is the name used for the proxy protocol policy
	proxyProtocolPolicyName = "k8s-proxyprotocol-enabled"

	// TagNameSubnetInternalELB is the tag name used on a subnet to designate that
	// it should be used for internal ELBs
	tagNameSubnetInternalELB = "kubernetes.io/role/internal-elb"

	// TagNameSubnetPublicELB is the tag name used on a subnet to designate that
	// it should be used for internet ELBs
	tagNameSubnetPublicELB = "kubernetes.io/role/elb"
)

var (
	// DefaultLoadBalancerConfiguration is the default LoadBalancher configuration.
	DefaultLoadBalancerConfiguration = LoadBalancer{
		Instances:  1,
		TargetRole: role.Worker,
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
	// ErrLoadBalancerIsNotReady is returned by CreateLoadBalancer/UpdateLoadBalancer when the LB is not ready yet.
	ErrLoadBalancerIsNotReady = controllerapi.NewRetryError("load balancer is not ready", 30*time.Second)

	ErrBelongsToSomeoneElse = errors.New("found a LBU with the same name belonging to")
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

type IngressType string

const (
	Hostname IngressType = "hostname"
	IP       IngressType = "ip"
	Both     IngressType = "both"
)

func (i IngressType) NeedHostname() bool {
	return i == Hostname || i == Both
}

func (i IngressType) NeedIP() bool {
	return i == IP || i == Both
}

type Ingress struct {
	Hostname string
	PublicIP *string
}

// LoadBalancer defines a load-balancer.
type LoadBalancer struct {
	Name                     []string `annotation:"osc-load-balancer-name"`
	ServiceName              string
	Internal                 bool     `annotation:"osc-load-balancer-internal"`
	Instances                int      `annotation:"osc-load-balancer-instances"`
	SubRegions               []string `annotation:"osc-load-balancer-subregions"`
	PublicIPPool             string   `annotation:"osc-load-balancer-ip-pool"`
	PublicIPID               []string `annotation:"osc-load-balancer-ip-id"`
	SubnetID                 []string `annotation:"osc-load-balancer-subnet-id"`
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
	IngressAddress           IngressType                `annotation:"osc-load-balancer-ingress-address"`
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
		ServiceName: svc.Name,
	}
	err := lb.decodeAnnotations(svc.Annotations)
	if err != nil {
		return nil, err
	}
	// set defaults
	err = mergo.Merge(lb, DefaultLoadBalancerConfiguration)
	if err != nil {
		return nil, fmt.Errorf("unable to set defaults: %w", err)
	}

	// forbid changing number of instances on a configured service
	if len(svc.Status.LoadBalancer.Ingress) > 0 && len(svc.Status.LoadBalancer.Ingress) != lb.Instances {
		return nil, errors.New("instances cannot be changed on a configured service")
	}

	switch {
	case lb.Instances == 1 && len(lb.Name) == 0:
		// autogenerated name, single instance
		lb.Name = []string{strings.ReplaceAll(string(svc.UID), "-", "")}
	case len(lb.Name) == 0:
		// autogenerated name, multiple instances (add -a/b/c)
		lb.Name = make([]string, 0, lb.Instances)
		base := strings.ReplaceAll(string(svc.UID), "-", "") + "-"
		for i := range lb.Instances {
			lb.Name = append(lb.Name, base+string([]byte{'a' + byte(i)}))
		}
	case len(lb.Name) < lb.Instances:
		// configured names, not enough names
		return nil, fmt.Errorf("missing name: %d expected, %d found", lb.Instances, len(lb.Name))
	default:
		// configured names, validate format & duplicates
		for i, name := range lb.Name {
			if !reName.MatchString(name) {
				return nil, fmt.Errorf("invalid name %q, only letters, numbers and - are allowed", name)
			}
			if lo.Count(lb.Name, name) > 1 {
				return nil, fmt.Errorf("duplicate name %q", name)
			}
			if len(name) > 32 {
				name = name[:32]
				if slices.Contains(lb.Name, name) {
					return nil, fmt.Errorf("duplicate name %q after truncation to 32 chars", name)
				}
				lb.Name[i] = name
			}
		}
	}
	if lb.Instances > 1 {
		switch len(lb.SubRegions) {
		case 0:
		case lb.Instances:
		default:
			return nil, fmt.Errorf("missing subregion: %d expected, %d found", lb.Instances, len(lb.SubRegions))
		}
		switch len(lb.PublicIPID) {
		case 0:
		case lb.Instances:
		default:
			return nil, fmt.Errorf("missing public ip id: %d expected, %d found", lb.Instances, len(lb.PublicIPID))
		}
		switch len(lb.SubnetID) {
		case 0:
		case lb.Instances:
		default:
			return nil, fmt.Errorf("missing subnet id: %d expected, %d found", lb.Instances, len(lb.PublicIPID))
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

// LoadBalancerExists checks if a load-balancer exists.
func (c *Cloud) LoadBalancerExists(ctx context.Context, l *LoadBalancer) (bool, error) {
	lbus, err := c.getLBUs(ctx, l)
	switch {
	case err != nil:
		return false, err
	case len(lbus) == 0:
		return false, nil
	}
	if !c.sameCluster(lbus[0].Tags) {
		return false, fmt.Errorf("%w another cluster", ErrBelongsToSomeoneElse)
	}
	svcName := tags.GetServiceName(lbus[0].Tags)
	if svcName != "" && svcName != l.ServiceName {
		return false, fmt.Errorf("%w another service", ErrBelongsToSomeoneElse)
	}
	return len(lbus) == l.Instances, nil
}

func (c *Cloud) getLBUs(ctx context.Context, l *LoadBalancer) ([]osc.LoadBalancer, error) {
	res, err := c.api.OAPI().ReadLoadBalancers(ctx, osc.ReadLoadBalancersRequest{
		Filters: &osc.FiltersLoadBalancer{LoadBalancerNames: &l.Name},
	})
	if err != nil {
		return nil, err
	}
	return *res.LoadBalancers, nil
}

// GetLoadBalancer fetches a load-balancer.
func (c *Cloud) GetLoadBalancer(ctx context.Context, l *LoadBalancer) (ingresses []Ingress, err error) {
	lbus, err := c.getLBUs(ctx, l)
	if err != nil {
		return nil, fmt.Errorf("unable to get LB: %w", err)
	}
	return lo.Map(lbus, func(lb osc.LoadBalancer, _ int) Ingress {
		return Ingress{Hostname: lb.DnsName, PublicIP: lb.PublicIp}
	}), nil
}

// CreateLoadBalancer creates a load-balancer.
func (c *Cloud) CreateLoadBalancer(ctx context.Context, l *LoadBalancer, backend []VM) (ingresses []Ingress, err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("unable to create LB: %w", err)
		}
	}()

	err = c.ensureSubnet(ctx, l)
	if err != nil {
		return nil, err
	}

	ingresses = make([]Ingress, 0, l.Instances)
	for i := range l.Instances {
		createRequest := osc.CreateLoadBalancerRequest{
			LoadBalancerName: l.Name[i],
			Listeners:        l.listeners(),
			Subnets:          &[]string{l.SubnetID[i]},
		}

		if l.Internal {
			createRequest.LoadBalancerType = ptr.To("internal")
		}
		switch {
		case len(l.PublicIPID) > 0:
			ip, err := oapi.GetPublicIp(ctx, l.PublicIPID[i], c.api.OAPI())
			if err != nil {
				return nil, fmt.Errorf("get public ip: %w", err)
			}
			createRequest.PublicIp = &ip
		case l.PublicIPPool != "":
			ip, err := sdk.AllocateIPFromPool(ctx, l.PublicIPPool, c.api.OAPI())
			if err != nil {
				return nil, err
			}
			createRequest.PublicIp = &ip.PublicIp
		}

		// security group
		var lbSG *osc.SecurityGroup
		if len(l.SecurityGroups) == 0 {
			lbSG, err = c.ensureSecurityGroup(ctx, l)
		} else {
			lbSG, err = c.getLBSecurityGroup(ctx, l.SecurityGroups[0])
		}
		if err == nil {
			err = c.updateIngressSecurityGroupRules(ctx, l, lbSG)
		}
		if err != nil {
			return nil, fmt.Errorf("ingress rules: %w", err)
		}
		backendSG, err := c.getBackendSecurityGroup(ctx, l, backend)
		if err == nil {
			err = c.updateBackendSecurityGroupRules(ctx, l, lbSG, backendSG)
		}
		if err != nil {
			return nil, fmt.Errorf("backend rules: %w", err)
		}

		sgs := append(l.SecurityGroups, l.AdditionalSecurityGroups...)
		createRequest.SecurityGroups = &sgs
		stags := l.Tags
		if stags == nil {
			stags = map[string]string{}
		}
		stags[tags.ServiceName] = l.ServiceName
		stags[c.clusterIDTagKey()] = tags.ResourceLifecycleOwned

		dtags := lo.MapToSlice(stags, func(k, v string) osc.ResourceTag {
			return osc.ResourceTag{Key: k, Value: v}
		})
		createRequest.Tags = &dtags
		slices.SortFunc(*createRequest.Tags, func(a, b osc.ResourceTag) int {
			switch {
			case a.Key < b.Key:
				return -1
			case a.Key > b.Key:
				return 1
			default:
				return 0
			}
		})

		klog.FromContext(ctx).V(1).Info("Creating load balancer",
			"name", createRequest.LoadBalancerName, "subnet", (*createRequest.Subnets)[0])
		res, err := c.api.OAPI().CreateLoadBalancer(ctx, createRequest)
		if err == nil {
			err = c.updateProxyProtocol(ctx, l, res.LoadBalancer)
		}
		if err == nil {
			err = c.updateAttributes(ctx, l, res.LoadBalancer)
		}
		if err == nil {
			err = c.updateHealthcheck(ctx, l, res.LoadBalancer)
		}
		if err == nil {
			err = c.updateBackendVms(ctx, l, backend, res.LoadBalancer)
		}
		switch {
		case err != nil:
			return nil, err
		case !l.Internal && res.LoadBalancer.PublicIp == nil:
		case res.LoadBalancer.DnsName == "":
		default:
			ingresses = append(ingresses, Ingress{
				Hostname: res.LoadBalancer.DnsName,
				PublicIP: res.LoadBalancer.PublicIp,
			})
		}
	}
	if len(ingresses) < l.Instances {
		return nil, ErrLoadBalancerIsNotReady
	}
	return ingresses, nil
}

func (c *Cloud) ensureSubnet(ctx context.Context, l *LoadBalancer) error {
	if len(l.SubnetID) > 0 {
		resp, err := c.api.OAPI().ReadSubnets(ctx, osc.ReadSubnetsRequest{
			Filters: &osc.FiltersSubnet{
				SubnetIds: &l.SubnetID,
			},
		})
		switch {
		case err != nil:
			return fmt.Errorf("find existing subnet: %w", err)
		case len(*resp.Subnets) < len(l.SubnetID):
			return fmt.Errorf("not enough subnets found: %d expected, %d found", len(l.SubnetID), len(*resp.Subnets))
		}
		l.NetID = (*resp.Subnets)[0].NetId
		return nil
	}
	resp, err := c.api.OAPI().ReadSubnets(ctx, osc.ReadSubnetsRequest{
		Filters: &osc.FiltersSubnet{
			TagKeys: ptr.To(c.clusterIDTagKeys()),
		},
	})
	if err == nil && len(*resp.Subnets) == 0 {
		resp, err = c.api.OAPI().ReadSubnets(ctx, osc.ReadSubnetsRequest{
			Filters: &osc.FiltersSubnet{
				NetIds: &[]string{*c.Self.NetID},
			},
		})
	}
	if err != nil {
		return fmt.Errorf("find subnet: %w", err)
	}
	if len(*resp.Subnets) == 0 {
		return errors.New("no subnet found")
	}
	azs := l.SubRegions
	if len(azs) == 0 {
		for i := range l.Instances {
			azs = append(azs, c.Self.Region+string([]byte{'a' + byte(i)}))
		}
	}

	// Find by role

	l.SubnetID = make([]string, l.Instances)
	ensureByTag := func(key, subregion string, i int) bool {
		for _, subnet := range *resp.Subnets {
			if tags.Has(subnet.Tags, key) && subnet.SubregionName == subregion {
				l.SubnetID[i] = subnet.SubnetId
				l.NetID = subnet.NetId
				return true
			}
		}
		return false
	}

	for i := range l.Instances {
		switch {
		case !l.Internal && ensureByTag(tagNameSubnetPublicELB, azs[i], i):
		case l.Internal && ensureByTag(tagNameSubnetInternalELB, azs[i], i):
		case l.Internal && ensureByTag(tags.RoleKey(role.InternalService), azs[i], i):
		case ensureByTag(tags.RoleKey(role.Service), azs[i], i):
		case ensureByTag(tags.RoleKey(role.LoadBalancer), azs[i], i):
		default:
			discovered, err := c.discoverSubnet(ctx, l, *resp.Subnets, azs[i])
			if err != nil {
				return err
			}
			l.SubnetID[i] = discovered.SubnetId
			l.NetID = discovered.NetId
		}
	}
	return nil
}

// discoverSubnet tries to find a public or private subnet for the LB.
func (c *Cloud) discoverSubnet(ctx context.Context, l *LoadBalancer, subnets []osc.Subnet, subregion string) (*osc.Subnet, error) {
	resp, err := c.api.OAPI().ReadRouteTables(ctx, osc.ReadRouteTablesRequest{
		Filters: &osc.FiltersRouteTable{
			NetIds: &[]string{subnets[0].NetId},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("discover subnet: %w", err)
	}

	// find a public or private subnet, depending on LB type
	var discovered *osc.Subnet
	for _, subnet := range subnets {
		if oapi.IsSubnetPublic(subnet.SubnetId, *resp.RouteTables) == !l.Internal {
			switch {
			case discovered == nil:
				// we ensure that a subnet is selected even if no subregion matches
				discovered = &subnet
			case subnet.SubregionName != subregion:
				// subregion does not match, and we alredy picked one
			case discovered.SubregionName != subregion:
				// replace already picked by one from the right subregion
				discovered = &subnet
			case tags.Must(tags.GetName(subnet.Tags)) < tags.Must(tags.GetName(discovered.Tags)):
				// multiple matches in the right subregion, take the first, in lexical order
				discovered = &subnet
			}
		}
	}
	if discovered == nil {
		return nil, errors.New("discover subnet: none found")
	}
	return discovered, nil
}

func (c *Cloud) ensureSecurityGroup(ctx context.Context, l *LoadBalancer) (*osc.SecurityGroup, error) {
	sgName := "k8s-elb-" + l.Name[0]
	sgDescription := fmt.Sprintf("Security group for Kubernetes service %s", l.ServiceName)
	var sg *osc.SecurityGroup
	resp, err := c.api.OAPI().CreateSecurityGroup(ctx, osc.CreateSecurityGroupRequest{
		SecurityGroupName: sgName,
		Description:       sgDescription,
		NetId:             &l.NetID,
	})
	switch {
	case oapi.ErrorCode(err) == "9008": // ErrorDuplicateGroup: the SecurityGroupName '{group_name}' already exists.
		// the SG might have been created by a previous request, try to load it
		resp, err := c.api.OAPI().ReadSecurityGroups(ctx, osc.ReadSecurityGroupsRequest{
			Filters: &osc.FiltersSecurityGroup{
				SecurityGroupNames: &[]string{sgName},
			},
		})
		switch {
		case err != nil:
			return nil, fmt.Errorf("read security groups: %w", err)
		case len(*resp.SecurityGroups) == 0: // this has a tiny chance of occurring, but we would not want the CCM to panic
			return nil, errors.New("duplicate SG but none found")
		default:
			sg = &(*resp.SecurityGroups)[0]
		}
	case err != nil:
		return nil, fmt.Errorf("create SG: %w", err)
	default:
		sg = resp.SecurityGroup
	}
	// check clusterID tag
	switch {
	case c.sameCluster(sg.Tags): // existing SG with valid tag => noop
	case tags.GetClusterID(sg.Tags) == "": // new SG or existing SG without tag => create
		_, err = c.api.OAPI().CreateTags(ctx, osc.CreateTagsRequest{
			ResourceIds: []string{sg.SecurityGroupId},
			Tags:        []osc.ResourceTag{{Key: c.clusterIDTagKey(), Value: tags.ResourceLifecycleOwned}},
		})
		if err != nil {
			return nil, fmt.Errorf("create SG: %w", err)
		}
	default: // existing SG with invalid tag/belonging to another cluster
		return nil, errors.New("a segurity group of the same name already exists")
	}
	l.SecurityGroups = []string{sg.SecurityGroupId}
	return sg, nil
}

// UpdateLoadBalancer updates a load-balancer.
func (c *Cloud) UpdateLoadBalancer(ctx context.Context, l *LoadBalancer, backend []VM) (ingresses []Ingress, err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("unable to create LB: %w", err)
		}
	}()
	existings, err := c.getLBUs(ctx, l)
	if err != nil {
		return nil, fmt.Errorf("check LB: %w", err)
	}
	if len(existings) == 0 {
		return nil, errors.New("existing LBU not found")
	}

	lbSG, err := c.getLBSecurityGroup(ctx, existings[0].SecurityGroups[0])
	if err == nil {
		err = c.updateIngressSecurityGroupRules(ctx, l, lbSG)
	}
	if err != nil {
		return nil, fmt.Errorf("ingress rules: %w", err)
	}
	backendSG, err := c.getBackendSecurityGroup(ctx, l, backend)
	if err == nil {
		err = c.updateBackendSecurityGroupRules(ctx, l, lbSG, backendSG)
	}
	if err != nil {
		return nil, fmt.Errorf("backend rules: %w", err)
	}

	ingresses = make([]Ingress, 0, len(existings))
	for _, existing := range existings {
		err = c.updateListeners(ctx, l, &existing)
		if err == nil {
			// proxy protocol requires listeners to be set
			err = c.updateProxyProtocol(ctx, l, &existing)
		}
		if err == nil {
			err = c.updateSSLCert(ctx, l, &existing)
		}
		if err == nil {
			err = c.updateAttributes(ctx, l, &existing)
		}
		if err == nil {
			err = c.updateHealthcheck(ctx, l, &existing)
		}
		if err == nil {
			err = c.updateBackendVms(ctx, l, backend, &existing)
		}
		switch {
		case err != nil:
			return nil, err
		case !l.Internal && existing.PublicIp == nil:
		case existing.DnsName == "":
		default:
			ingresses = append(ingresses, Ingress{
				Hostname: existing.DnsName,
				PublicIP: existing.PublicIp,
			})
		}
	}
	if len(ingresses) < l.Instances {
		return nil, ErrLoadBalancerIsNotReady
	}
	return ingresses, nil
}

func (c *Cloud) updateProxyProtocol(ctx context.Context, l *LoadBalancer, oscExisting *osc.LoadBalancer) error {
	name := oscExisting.LoadBalancerName
	elbu, err := c.api.LBU().DescribeLoadBalancersWithContext(ctx, &elb.DescribeLoadBalancersInput{
		LoadBalancerNames: []*string{aws.String(name)},
	})
	if err != nil {
		return fmt.Errorf("check proxy protocol: %w", err)
	}
	if len(elbu.LoadBalancerDescriptions) == 0 {
		return nil
	}
	existing := elbu.LoadBalancerDescriptions[0]

	set := false
	if existing != nil {
		set = slices.ContainsFunc(existing.ListenerDescriptions, func(p *elb.ListenerDescription) bool {
			return slices.ContainsFunc(p.PolicyNames, func(name *string) bool {
				return *name == proxyProtocolPolicyName
			})
		})
	}
	need := slices.ContainsFunc(l.Listeners, func(lstnr Listener) bool {
		return isPortIn(lstnr.BackendPort, l.ListenerDefaults.ProxyProtocol)
	})
	if need == set {
		return nil
	}
	var policies []*string
	if need {
		request := &elb.CreateLoadBalancerPolicyInput{
			LoadBalancerName: aws.String(name),
			PolicyName:       aws.String(proxyProtocolPolicyName),
			PolicyTypeName:   aws.String("ProxyProtocolPolicyType"),
			PolicyAttributes: []*elb.PolicyAttribute{
				{
					AttributeName:  aws.String("ProxyProtocol"),
					AttributeValue: aws.String("true"),
				},
			},
		}
		klog.FromContext(ctx).V(2).Info("Creating proxy protocol policy")
		_, err := c.api.LBU().CreateLoadBalancerPolicyWithContext(ctx, request)
		switch {
		case err == nil:
		case oapi.AWSErrorCode(err) == elb.ErrCodeDuplicatePolicyNameException:
			klog.FromContext(ctx).V(4).Info("Policy already exists")
		default:
			return fmt.Errorf("create proxy protocol policy: %w", err)
		}
		policies = []*string{aws.String(proxyProtocolPolicyName)}
	} else {
		policies = []*string{}
	}

	for _, listener := range l.Listeners {
		if need && !isPortIn(listener.BackendPort, l.ListenerDefaults.ProxyProtocol) {
			continue
		}
		request := &elb.SetLoadBalancerPoliciesForBackendServerInput{
			LoadBalancerName: aws.String(name),
			InstancePort:     aws.Int64(int64(listener.BackendPort)),
			PolicyNames:      policies,
		}
		if len(policies) > 0 {
			klog.FromContext(ctx).V(2).Info("Adding policy on backend", "port", listener.BackendPort)
		} else {
			klog.FromContext(ctx).V(2).Info("Removing policies from backend", "port", listener.BackendPort)
		}
		_, err := c.api.LBU().SetLoadBalancerPoliciesForBackendServerWithContext(ctx, request)
		if err != nil {
			return fmt.Errorf("set proxy protocol policy: %w", err)
		}
	}
	return nil
}

func (c *Cloud) updateListeners(ctx context.Context, l *LoadBalancer, existing *osc.LoadBalancer) error {
	expect := l.listeners()

	// Remove unused listeners
	var del, delback []int
	for _, elistener := range existing.Listeners {
		if !slices.ContainsFunc(expect, func(listener osc.ListenerForCreation) bool {
			return oscListenersAreEqual(elistener, listener)
		}) {
			del = append(del, elistener.LoadBalancerPort)
			if len(elistener.PolicyNames) > 0 {
				delback = append(delback, elistener.BackendPort)
			}
		}
	}
	for _, port := range delback {
		klog.FromContext(ctx).V(2).Info(fmt.Sprintf("Reseting policies on backend port %d", port))
		_, err := c.api.LBU().SetLoadBalancerPoliciesForBackendServerWithContext(ctx, &elb.SetLoadBalancerPoliciesForBackendServerInput{
			LoadBalancerName: aws.String(existing.LoadBalancerName),
			InstancePort:     aws.Int64(int64(port)),
			PolicyNames:      []*string{},
		})
		if err != nil {
			return fmt.Errorf("unset backend policy: %w", err)
		}
	}
	if len(del) > 0 {
		klog.FromContext(ctx).V(2).Info(fmt.Sprintf("Deleting %d listeners", len(del)))
		_, err := c.api.OAPI().DeleteLoadBalancerListeners(ctx, osc.DeleteLoadBalancerListenersRequest{
			LoadBalancerName:  existing.LoadBalancerName,
			LoadBalancerPorts: del,
		})
		if err != nil {
			return fmt.Errorf("delete unused listeners: %w", err)
		}
	}

	// Add new listeners
	var add []osc.ListenerForCreation
	for _, listener := range expect {
		if !slices.ContainsFunc(existing.Listeners, func(elistener osc.Listener) bool {
			return oscListenersAreEqual(elistener, listener)
		}) {
			add = append(add, listener)
		}
	}
	if len(add) > 0 {
		klog.FromContext(ctx).V(2).Info(fmt.Sprintf("Adding %d listeners", len(add)))
		_, err := c.api.OAPI().CreateLoadBalancerListeners(ctx, osc.CreateLoadBalancerListenersRequest{
			LoadBalancerName: existing.LoadBalancerName,
			Listeners:        add,
		})
		if err != nil {
			return fmt.Errorf("add new listeners: %w", err)
		}
	}
	return nil
}

func oscListenersAreEqual(actual osc.Listener, expected osc.ListenerForCreation) bool {
	if !protocolsAreEqual(actual.LoadBalancerProtocol, expected.LoadBalancerProtocol) {
		return false
	}
	if !protocolsAreEqual(actual.BackendProtocol, *expected.BackendProtocol) {
		return false
	}
	if actual.LoadBalancerPort != expected.LoadBalancerPort {
		return false
	}
	if actual.BackendPort != expected.BackendPort {
		return false
	}
	return true
}

// protocolsAreEqual checks if two ELB protocol strings are considered the same
// Comparison is case insensitive
func protocolsAreEqual(l, r string) bool {
	return strings.EqualFold(l, r)
}

func certificateAreEqual(l *string, r string) bool {
	var ls string
	if l != nil {
		ls = *l
	}
	return ls == r
}

func (c *Cloud) updateSSLCert(ctx context.Context, l *LoadBalancer, existing *osc.LoadBalancer) error {
	for _, listener := range existing.Listeners {
		if !certificateAreEqual(listener.ServerCertificateId, l.ListenerDefaults.SSLCertificate) {
			klog.FromContext(ctx).V(2).Info("Changing certificate", "port", listener.LoadBalancerPort)
			_, err := c.api.OAPI().UpdateLoadBalancer(ctx, osc.UpdateLoadBalancerRequest{
				LoadBalancerName:    existing.LoadBalancerName,
				ServerCertificateId: &l.ListenerDefaults.SSLCertificate,
			})
			if err != nil {
				return fmt.Errorf("set certificate: %w", err)
			}
		}
	}
	return nil
}

func (c *Cloud) updateAttributes(ctx context.Context, l *LoadBalancer, oscExisting *osc.LoadBalancer) error {
	existing, err := c.api.LBU().DescribeLoadBalancerAttributesWithContext(ctx, &elb.DescribeLoadBalancerAttributesInput{
		LoadBalancerName: aws.String(oscExisting.LoadBalancerName),
	})
	if err != nil {
		return fmt.Errorf("check LB attributes: %w", err)
	}
	expected := l.elbAttributes()
	if !accessLogAttributesAreEqual(existing.LoadBalancerAttributes, expected) {
		klog.FromContext(ctx).V(2).Info("Updating access log attributes")
		_, err := c.api.LBU().ModifyLoadBalancerAttributesWithContext(ctx, &elb.ModifyLoadBalancerAttributesInput{
			LoadBalancerName: aws.String(oscExisting.LoadBalancerName),
			LoadBalancerAttributes: &elb.LoadBalancerAttributes{
				AccessLog: expected.AccessLog,
			},
		})
		if err != nil {
			return fmt.Errorf("update access log attribute: %w", err)
		}
	}
	if !connectionAttributesAreEqual(existing.LoadBalancerAttributes, expected) {
		klog.FromContext(ctx).V(2).Info("Updating connection attributes")
		_, err := c.api.LBU().ModifyLoadBalancerAttributesWithContext(ctx, &elb.ModifyLoadBalancerAttributesInput{
			LoadBalancerName: aws.String(oscExisting.LoadBalancerName),
			LoadBalancerAttributes: &elb.LoadBalancerAttributes{
				ConnectionDraining: expected.ConnectionDraining,
				ConnectionSettings: expected.ConnectionSettings,
			},
		})
		if err != nil {
			return fmt.Errorf("update connection attribute: %w", err)
		}
	}
	return nil
}

func accessLogAttributesAreEqual(actual, expected *elb.LoadBalancerAttributes) bool {
	switch {
	case (actual.AccessLog == nil) != (expected.AccessLog == nil):
		return false
	case expected.AccessLog == nil:
	case !equal(actual.AccessLog.Enabled, expected.AccessLog.Enabled):
		return false
	case aws.BoolValue(expected.AccessLog.Enabled):
		if !equal(actual.AccessLog.EmitInterval, expected.AccessLog.EmitInterval) ||
			!equal(actual.AccessLog.S3BucketName, expected.AccessLog.S3BucketName) ||
			!equal(actual.AccessLog.S3BucketPrefix, expected.AccessLog.S3BucketPrefix) {
			return false
		}
	}
	return true
}

func connectionAttributesAreEqual(actual, expected *elb.LoadBalancerAttributes) bool {
	switch {
	case (actual.ConnectionDraining == nil) != (expected.ConnectionDraining == nil):
		return false
	case expected.ConnectionDraining == nil:
	case !equal(actual.ConnectionDraining.Enabled, expected.ConnectionDraining.Enabled):
		return false
	case aws.BoolValue(expected.ConnectionDraining.Enabled):
		if !equal(actual.ConnectionDraining.Timeout, expected.ConnectionDraining.Timeout) {
			return false
		}
	}
	switch {
	case (actual.ConnectionSettings == nil) != (expected.ConnectionSettings == nil):
		return false
	case expected.ConnectionSettings == nil:
	case !equal(actual.ConnectionSettings.IdleTimeout, expected.ConnectionSettings.IdleTimeout):
		return false
	}
	return true
}

func equal[T comparable](a, b *T) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func (c *Cloud) updateHealthcheck(ctx context.Context, l *LoadBalancer, existing *osc.LoadBalancer) error {
	expected := l.healthCheck()
	switch {
	case (existing == nil) != (expected == nil):
	case existing != nil:
		actual := existing.HealthCheck
		if expected.Path == actual.Path &&
			expected.HealthyThreshold == actual.HealthyThreshold &&
			expected.UnhealthyThreshold == actual.UnhealthyThreshold &&
			expected.CheckInterval == actual.CheckInterval &&
			expected.Timeout == actual.Timeout {
			return nil
		}
	}
	klog.FromContext(ctx).V(2).Info("Configuring healthcheck")
	_, err := c.api.OAPI().UpdateLoadBalancer(ctx, osc.UpdateLoadBalancerRequest{
		LoadBalancerName: existing.LoadBalancerName,
		HealthCheck:      expected,
	})
	if err != nil {
		return fmt.Errorf("configure health check: %w", err)
	}

	return nil
}

func (c *Cloud) updateBackendVms(ctx context.Context, l *LoadBalancer, vms []VM, existing *osc.LoadBalancer) error {
	// in most cases, there will be no change, preallocating would waste an alloc
	var add []string //nolint:prealloc
	for _, vm := range vms {
		if existing != nil && slices.Contains(existing.BackendVmIds, vm.ID) {
			continue
		}
		add = append(add, vm.ID)
	}
	if len(add) > 0 {
		klog.FromContext(ctx).V(2).Info("Adding backend instances", "count", len(add))
		_, err := c.api.OAPI().RegisterVmsInLoadBalancer(ctx, osc.RegisterVmsInLoadBalancerRequest{
			LoadBalancerName: existing.LoadBalancerName,
			BackendVmIds:     add,
		})
		if err != nil {
			return fmt.Errorf("register instances: %w", err)
		}
	}
	if existing == nil {
		return nil
	}
	var remove []string //nolint:prealloc
	for _, i := range existing.BackendVmIds {
		if slices.ContainsFunc(vms, func(vm VM) bool {
			return i == vm.ID
		}) {
			continue
		}
		remove = append(remove, i)
	}
	if len(remove) > 0 {
		klog.FromContext(ctx).V(2).Info("Removing backend instances", "count", len(remove))
		_, err := c.api.OAPI().DeregisterVmsInLoadBalancer(ctx, osc.DeregisterVmsInLoadBalancerRequest{
			LoadBalancerName: existing.LoadBalancerName,
			BackendVmIds:     remove,
		})
		if err != nil {
			return fmt.Errorf("deregister instances: %w", err)
		}
	}

	return nil
}

func (c *Cloud) getLBSecurityGroup(ctx context.Context, id string) (*osc.SecurityGroup, error) {
	resp, err := c.api.OAPI().ReadSecurityGroups(ctx, osc.ReadSecurityGroupsRequest{
		Filters: &osc.FiltersSecurityGroup{
			SecurityGroupIds: &[]string{id},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("list SGs: %w", err)
	}
	if len(*resp.SecurityGroups) == 0 {
		return nil, errors.New("no SG found for load balancer")
	}
	return &(*resp.SecurityGroups)[0], nil
}

func (c *Cloud) getBackendSecurityGroup(ctx context.Context, l *LoadBalancer, vms []VM) (backendSG *osc.SecurityGroup, err error) {
	sgIDs := sets.Set[string]{}
	for _, vm := range vms {
		for _, sg := range vm.cloudVm.SecurityGroups {
			sgIDs.Insert(sg.SecurityGroupId)
		}
	}
	resp, err := c.api.OAPI().ReadSecurityGroups(ctx, osc.ReadSecurityGroupsRequest{
		Filters: &osc.FiltersSecurityGroup{
			SecurityGroupIds: ptr.To(sets.List(sgIDs)),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("list SGs: %w", err)
	}
	if len(*resp.SecurityGroups) == 0 {
		return nil, errors.New("no SG found for target nodes")
	}
	roleTagCount := math.MaxInt
	for _, sg := range *resp.SecurityGroups {
		if tags.Has(sg.Tags, c.mainSGTagKey()) {
			klog.FromContext(ctx).V(4).Info("Found security group having main tag", "securityGroupId", sg.SecurityGroupId)
			backendSG = &sg
		}
		if tags.HasRole(sg.Tags, l.TargetRole) {
			nRoleTagCount := countRoleTags(sg.Tags)
			if nRoleTagCount < roleTagCount {
				klog.FromContext(ctx).V(4).Info("Found security group having role tag", "securityGroupId", sg.SecurityGroupId, "role", l.TargetRole, "nroles", nRoleTagCount)
				backendSG = &sg
				roleTagCount = nRoleTagCount
			}
		}
	}
	if backendSG == nil {
		backendSG = &(*resp.SecurityGroups)[0]
		klog.FromContext(ctx).V(3).Info("No security group found by tag, using a random one", "securityGroupId", backendSG.SecurityGroupId)
	}
	return
}

func (c *Cloud) updateIngressSecurityGroupRules(ctx context.Context, l *LoadBalancer, lbSG *osc.SecurityGroup) error {
	allowed := l.AllowFrom.StringSlice()
	// sort slice to get a deterministic order for tests
	slices.Sort(allowed)
	// Adding new rules
	for _, listener := range l.Listeners {
		var addRanges []string
		for _, allowFrom := range allowed {
			if !slices.ContainsFunc(lbSG.InboundRules, func(r osc.SecurityGroupRule) bool {
				return r.FromPortRange == listener.Port && slices.Contains(r.IpRanges, allowFrom)
			}) {
				addRanges = append(addRanges, allowFrom)
			}
		}
		if len(addRanges) == 0 {
			continue
		}
		klog.FromContext(ctx).V(2).Info("Adding ingress rule", "from", addRanges, "to", lbSG.SecurityGroupId, "port", listener.Port)
		_, err := c.api.OAPI().CreateSecurityGroupRule(ctx, osc.CreateSecurityGroupRuleRequest{
			SecurityGroupId: lbSG.SecurityGroupId,
			Flow:            "Inbound",
			Rules: []osc.SecurityGroupRule{{
				IpProtocol:    "tcp",
				FromPortRange: listener.Port,
				ToPortRange:   listener.Port,
				IpRanges:      addRanges,
			}},
		})
		if err != nil {
			return fmt.Errorf("add ingress rule: %w", err)
		}
	}

	// Removing rules
	for _, r := range lbSG.InboundRules {
		del := false
		delRule := osc.SecurityGroupRule{
			IpProtocol:    r.IpProtocol,
			FromPortRange: r.FromPortRange,
			ToPortRange:   r.ToPortRange,
		}
		if !slices.ContainsFunc(l.Listeners, func(listener Listener) bool {
			return listener.Port == r.FromPortRange
		}) {
			del = true
		}
		if del {
			delRule.IpRanges = r.IpRanges
		} else if len(r.IpRanges) > 0 {
			delRule.IpRanges = []string{}
			for _, ipRange := range r.IpRanges {
				if !slices.Contains(allowed, ipRange) {
					delRule.IpRanges = append(delRule.IpRanges, ipRange)
					del = true
				}
			}
		}
		if !del {
			continue
		}
		klog.FromContext(ctx).V(2).Info("Deleting ingress rule", "from", delRule.IpRanges, "to", lbSG.SecurityGroupId, "port", r.FromPortRange)
		_, err := c.api.OAPI().DeleteSecurityGroupRule(ctx, osc.DeleteSecurityGroupRuleRequest{
			SecurityGroupId: lbSG.SecurityGroupId,
			Flow:            "Inbound",
			Rules:           []osc.SecurityGroupRule{delRule},
		})
		if err != nil {
			return fmt.Errorf("delete ingress rule: %w", err)
		}
	}
	return nil
}

func (c *Cloud) updateBackendSecurityGroupRules(ctx context.Context, l *LoadBalancer, srcSG, destSG *osc.SecurityGroup) error {
	srcSGID := srcSG.SecurityGroupId

	// Adding new rules
	for _, listener := range l.Listeners {
		if slices.ContainsFunc(destSG.InboundRules, func(r osc.SecurityGroupRule) bool {
			return r.FromPortRange == listener.BackendPort &&
				slices.ContainsFunc(r.SecurityGroupsMembers, func(m osc.SecurityGroupsMember) bool {
					return srcSGID == m.SecurityGroupId
				})
		}) {
			continue
		}
		klog.FromContext(ctx).V(2).Info("Adding backend rule", "from", srcSGID, "to", destSG.SecurityGroupId, "port", listener.BackendPort)
		_, err := c.api.OAPI().CreateSecurityGroupRule(ctx, osc.CreateSecurityGroupRuleRequest{
			SecurityGroupId: destSG.SecurityGroupId,
			Flow:            "Inbound",
			Rules: []osc.SecurityGroupRule{{
				IpProtocol:    "tcp",
				FromPortRange: listener.BackendPort,
				ToPortRange:   listener.BackendPort,
				SecurityGroupsMembers: []osc.SecurityGroupsMember{{
					SecurityGroupId: srcSGID,
				}},
			}},
		})
		if err != nil {
			return fmt.Errorf("add backend rule: %w", err)
		}
	}

	// Removing rules
	for _, r := range destSG.InboundRules {
		// ignore if rule is not from the LB SG
		if !slices.ContainsFunc(r.SecurityGroupsMembers, func(m osc.SecurityGroupsMember) bool {
			return m.SecurityGroupId == srcSGID
		}) {
			continue
		}
		// ignore if port is not from a lister
		if slices.ContainsFunc(l.Listeners, func(listener Listener) bool {
			return listener.BackendPort == r.FromPortRange
		}) {
			continue
		}
		klog.FromContext(ctx).V(2).Info("Deleting backend rule", "from", srcSGID, "to", destSG.SecurityGroupId, "port", r.FromPortRange)
		_, err := c.api.OAPI().DeleteSecurityGroupRule(ctx, osc.DeleteSecurityGroupRuleRequest{
			SecurityGroupId: destSG.SecurityGroupId,
			Flow:            "Inbound",
			Rules: []osc.SecurityGroupRule{{
				IpProtocol:    r.IpProtocol,
				FromPortRange: r.FromPortRange,
				ToPortRange:   r.ToPortRange,
				SecurityGroupsMembers: []osc.SecurityGroupsMember{{
					SecurityGroupId: srcSGID,
				}},
			}},
		})
		if err != nil {
			return fmt.Errorf("delete backend rule: %w", err)
		}
	}
	return nil
}

// DeleteLoadBalancer deletes a load balancer.
func (c *Cloud) DeleteLoadBalancer(ctx context.Context, l *LoadBalancer) error {
	existings, err := c.getLBUs(ctx, l)
	if err != nil {
		return fmt.Errorf("check LB: %w", err)
	}
	for _, existing := range existings {
		// remove all backend VMs
		err = c.updateBackendVms(ctx, l, nil, &existing)
		if err != nil {
			return fmt.Errorf("deregister instances: %w", err)
		}
		// Tag LB SG as to be deleted (only if it has been created)
		for _, sg := range existing.SecurityGroups {
			if !slices.Contains(l.SecurityGroups, sg) && !slices.Contains(l.AdditionalSecurityGroups, sg) {
				klog.FromContext(ctx).V(2).Info("Marking SG for deletion", "securityGroupId", sg)
				_, err = c.api.OAPI().CreateTags(ctx, osc.CreateTagsRequest{
					ResourceIds: []string{sg},
					Tags:        []osc.ResourceTag{{Key: SGToDeleteTagKey}},
				})
				if err != nil {
					return fmt.Errorf("mark SG for deletion: %w", err)
				}
			}
		}

		// Delete the load balancer itself
		klog.FromContext(ctx).V(2).Info("Deleting load-balancer")
		_, err = c.api.OAPI().DeleteLoadBalancer(ctx, osc.DeleteLoadBalancerRequest{
			LoadBalancerName: existing.LoadBalancerName,
		})
		if err != nil {
			return fmt.Errorf("delete LBU: %w", err)
		}
	}
	return nil
}

// RunGarbageCollector deletes LB security groups
func (c *Cloud) RunGarbageCollector(ctx context.Context) error {
	// We collect all the SG from the cluster
	// This is the list of SG we will scan to find rules linking to the SG to be deleted.
	resp, err := c.api.OAPI().ReadSecurityGroups(ctx, osc.ReadSecurityGroupsRequest{
		Filters: &osc.FiltersSecurityGroup{
			TagKeys: ptr.To(c.clusterIDTagKeys()),
		},
	})
	if err != nil {
		return fmt.Errorf("find security groups: %w", err)
	}
	// Find SG to delete
	var toDelete []string
	for _, sg := range *resp.SecurityGroups {
		if tags.Has(sg.Tags, SGToDeleteTagKey) {
			toDelete = append(toDelete, sg.SecurityGroupId)
		}
	}
	klog.FromContext(ctx).V(4).Info("Security groups marked for deletion", "count", len(toDelete))
	for _, delSGID := range toDelete {
		// delete all inbound rules from this SG
		for _, sg := range *resp.SecurityGroups {
			klog.FromContext(ctx).V(2).Info("Deleting inbound rule", "from", delSGID, "to", sg.SecurityGroupId)
			for _, r := range sg.InboundRules {
				if slices.ContainsFunc(r.SecurityGroupsMembers, func(m osc.SecurityGroupsMember) bool {
					return slices.Contains(toDelete, m.SecurityGroupId)
				}) {
					_, err = c.api.OAPI().DeleteSecurityGroupRule(ctx, osc.DeleteSecurityGroupRuleRequest{
						SecurityGroupId: sg.SecurityGroupId,
						Flow:            "Inbound",
						Rules: []osc.SecurityGroupRule{{
							FromPortRange: r.FromPortRange,
							ToPortRange:   r.ToPortRange,
							IpProtocol:    r.IpProtocol,
							SecurityGroupsMembers: []osc.SecurityGroupsMember{{
								SecurityGroupId: delSGID,
							}},
						}},
					})
					if err != nil {
						return fmt.Errorf("delete rule from %s to %s: %w", delSGID, sg.SecurityGroupId, err)
					}
				}
			}
		}
		klog.FromContext(ctx).V(2).Info("Deleting SG", "securityGroupId", delSGID)
		_, err = c.api.OAPI().DeleteSecurityGroup(ctx, osc.DeleteSecurityGroupRequest{
			SecurityGroupId: &delSGID,
		})
		if err != nil {
			return fmt.Errorf("delete SG %s: %w", delSGID, err)
		}
	}
	return nil
}
