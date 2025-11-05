package cloud

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"dario.cat/mergo"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elb"
	"github.com/go-viper/mapstructure/v2"
	"github.com/outscale/cloud-provider-osc/cloud-controller-manager/osc/oapi"
	"github.com/outscale/osc-sdk-go/v2"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	controllerapi "k8s.io/cloud-provider/api"
	servicehelpers "k8s.io/cloud-provider/service/helpers"
	"k8s.io/klog/v2"
	utilnet "k8s.io/utils/net"
	"k8s.io/utils/ptr"
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
		TargetRole: "worker",
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
	// ErrEmptyPool is returned when a LoadBalancer requests an API from an empty pool.
	ErrEmptyPool = errors.New("no available IP in pool")

	ErrBelongsToSomeoneElse = errors.New("found a LBU with the same name belonging to")
)

// HealthCheck is the healcheck configuration.
type HealthCheck struct {
	Interval           int32 `annotation:"osc-load-balancer-healthcheck-interval"`
	Timeout            int32 `annotation:"osc-load-balancer-healthcheck-timeout"`
	HealthyThreshold   int32 `annotation:"osc-load-balancer-healthcheck-healthy-threshold"`
	UnhealthyThreshold int32 `annotation:"osc-load-balancer-healthcheck-unhealthy-threshold"`

	Port     int32  `annotation:"osc-load-balancer-healthcheck-port"`
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

func isPortIn(port int32, allowed []string) bool {
	if slices.Contains(allowed, "*") {
		return true
	}
	sport := strconv.FormatInt(int64(port), 10)
	return slices.Contains(allowed, sport)
}

// Listener defines a listener.
type Listener struct {
	Port        int32
	BackendPort int32
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

// LoadBalancer defines a load-balancer.
type LoadBalancer struct {
	Name                     string `annotation:"osc-load-balancer-name"`
	ServiceName              string
	Internal                 bool   `annotation:"osc-load-balancer-internal"`
	PublicIPPool             string `annotation:"osc-load-balancer-ip-pool"`
	PublicIPID               string `annotation:"osc-load-balancer-ip-id"`
	SubnetID                 string `annotation:"osc-load-balancer-subnet-id"`
	NetID                    string
	SecurityGroups           []string          `annotation:"osc-load-balancer-security-group"`
	AdditionalSecurityGroups []string          `annotation:"osc-load-balancer-extra-security-groups"`
	TargetRole               string            `annotation:"osc-load-balancer-target-role"`
	TargetNodesLabels        map[string]string `annotation:"osc-load-balancer-target-node-labels"`
	Tags                     map[string]string `annotation:"osc-load-balancer-additional-resource-tags"`
	HealthCheck              HealthCheck       `annotation:",squash"`
	ListenerDefaults         ListenerDefaults  `annotation:",squash"`
	Listeners                []Listener
	Connection               Connection `annotation:",squash"`
	SessionAffinity          string
	AccessLog                AccessLog `annotation:",squash"`
	AllowFrom                utilnet.IPNetSet
	IngressAddress           IngressAddress         `annotation:"osc-load-balancer-ingress-address"`
	IPMode                   *v1.LoadBalancerIPMode `annotation:"osc-load-balancer-ingress-ipmode"`

	lbSecurityGroup     *osc.SecurityGroup
	targetSecurityGroup *osc.SecurityGroup
}

var reName = regexp.MustCompile("^[a-zA-Z0-9-]+$")

// NewLoadBalancer creates a new LoadBalancer instance from a Kubernetes Service.
func NewLoadBalancer(svc *v1.Service, addTags map[string]string) (*LoadBalancer, error) {
	if svc.Spec.SessionAffinity != v1.ServiceAffinityNone {
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
		if port.Protocol != v1.ProtocolTCP {
			return nil, errors.New("only TCP load balancers are supported")
		}
		if port.NodePort == 0 {
			continue
		}
		lb.Listeners = append(lb.Listeners, Listener{
			Port:        port.Port,
			BackendPort: port.NodePort,
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
		lb.HealthCheck.Port = port
		lb.HealthCheck.Protocol = "http"
	case len(lb.Listeners) == 0:
	default:
		lb.HealthCheck.Port = lb.Listeners[0].BackendPort
		lb.HealthCheck.Protocol = "tcp"
	}
	err = mergo.Merge(lb, DefaultLoadBalancerConfiguration)
	if err != nil {
		return nil, fmt.Errorf("unable to set defaults: %w", err)
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
	lb, err := c.getLoadBalancer(ctx, l)
	switch {
	case err != nil:
		return false, err
	case lb == nil:
		return false, nil
	}
	if getLBUClusterID(lb.GetTags()) != c.clusterID {
		return false, fmt.Errorf("%w another cluster", ErrBelongsToSomeoneElse)
	}
	svcName := getLBUServiceName(lb.GetTags())
	if svcName != "" && svcName != l.ServiceName {
		return false, fmt.Errorf("%w another service", ErrBelongsToSomeoneElse)
	}
	return true, nil
}

func (c *Cloud) getLoadBalancer(ctx context.Context, l *LoadBalancer) (*osc.LoadBalancer, error) {
	res, err := c.api.OAPI().ReadLoadBalancers(ctx, osc.ReadLoadBalancersRequest{
		Filters: &osc.FiltersLoadBalancer{LoadBalancerNames: &[]string{l.Name}},
	})
	if err != nil {
		return nil, err
	}
	if len(res) == 0 {
		return nil, nil
	}
	return &res[0], nil
}

// GetLoadBalancer fetches a load-balancer.
func (c *Cloud) GetLoadBalancer(ctx context.Context, l *LoadBalancer) (dns, ip string, found bool, err error) {
	lb, err := c.getLoadBalancer(ctx, l)
	switch {
	case err != nil:
		return "", "", false, fmt.Errorf("unable to get LB: %w", err)
	case lb == nil:
		return "", "", false, nil
	case lb.DnsName != nil:
		return lb.GetDnsName(), lb.GetPublicIp(), true, nil
	default:
		return "", "", true, nil
	}
}

// CreateLoadBalancer creates a load-balancer.
func (c *Cloud) CreateLoadBalancer(ctx context.Context, l *LoadBalancer, backend []VM) (dns, ip string, err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("unable to create LB: %w", err)
		}
	}()
	createRequest := osc.CreateLoadBalancerRequest{
		LoadBalancerName: l.Name,
		Listeners:        l.listeners(),
	}

	if l.Internal {
		createRequest.LoadBalancerType = ptr.To("internal")
	}
	switch {
	case l.PublicIPID != "":
		createRequest.PublicIp = ptr.To(l.PublicIPID)
	case l.PublicIPPool != "":
		ip, err := c.allocateFromPool(ctx, l.PublicIPPool)
		if err != nil {
			return "", "", fmt.Errorf("allocate ip: %w", err)
		}
		createRequest.PublicIp = ip.PublicIpId
	}

	// TODO: drop public cloud code ?
	if c.Self.SubnetID == "" {
		createRequest.SubregionNames = &[]string{c.Metadata.AvailabilityZone}
	} else {
		// subnet
		err = c.ensureSubnet(ctx, l)
		if err != nil {
			return "", "", err
		}
		createRequest.Subnets = &[]string{l.SubnetID}

		// security group
		err = c.ensureSecurityGroup(ctx, l)
		if err != nil {
			return "", "", err
		}
		sgs := append(l.SecurityGroups, l.AdditionalSecurityGroups...)
		createRequest.SecurityGroups = &sgs
	}
	tags := l.Tags
	if tags == nil {
		tags = map[string]string{}
	}
	tags[ServiceNameTagKey] = l.ServiceName
	tags[clusterIDTagKey(c.clusterID)] = ResourceLifecycleOwned

	ltags := make([]osc.ResourceTag, 0, len(tags))
	for k, v := range tags {
		ltags = append(ltags, osc.ResourceTag{
			Key: k, Value: v,
		})
	}
	createRequest.Tags = &ltags
	slices.SortFunc(createRequest.GetTags(), func(a, b osc.ResourceTag) int {
		switch {
		case a.Key < b.Key:
			return -1
		case a.Key > b.Key:
			return 1
		default:
			return 0
		}
	})

	klog.FromContext(ctx).V(1).Info("Creating load balancer")
	res, err := c.api.OAPI().CreateLoadBalancer(ctx, createRequest)
	if err == nil {
		err = c.updateProxyProtocol(ctx, l, nil)
	}
	if err == nil {
		err = c.updateAttributes(ctx, l, nil)
	}
	if err == nil {
		err = c.updateHealthcheck(ctx, l, nil)
	}
	if err == nil {
		err = c.updateBackendVms(ctx, l, backend, nil)
	}
	if err == nil {
		err = c.getSecurityGroups(ctx, l, backend, nil)
	}
	if err == nil {
		err = c.updateIngressSecurityGroupRules(ctx, l, nil)
	}
	if err == nil {
		err = c.updateBackendSecurityGroupRules(ctx, l, nil)
	}
	switch {
	case err != nil:
		return "", "", err
	case !l.Internal && res.PublicIp == nil:
		return "", "", ErrLoadBalancerIsNotReady
	case res.DnsName == nil:
		return "", "", ErrLoadBalancerIsNotReady
	default:
		return res.GetDnsName(), res.GetPublicIp(), nil
	}
}

func (c *Cloud) allocateFromPool(ctx context.Context, pool string) (*osc.PublicIp, error) {
	log := klog.FromContext(ctx)
	log.V(4).Info("Fetching publicIps from pool", "pool", pool)
	pips, err := c.api.OAPI().ListPublicIpsFromPool(ctx, pool)
	if err != nil {
		return nil, fmt.Errorf("from pool: %w", err)
	}
	if len(pips) == 0 {
		return nil, ErrEmptyPool
	}
	// randomly fetch from the list, to limit the chance of allocating
	// the same IP to two concurrent requests
	off := rand.IntN(len(pips)) //nolint:gosec
	for i := range pips {
		pip := pips[(off+i)%len(pips)]
		if pip.LinkPublicIpId == nil {
			log.V(3).Info("Found publicIp in pool", "publicIpId", pip.GetPublicIpId(), "publicIp", pip.GetPublicIp())
			return &pip, nil
		}
	}
	return nil, ErrEmptyPool
}

func (c *Cloud) ensureSubnet(ctx context.Context, l *LoadBalancer) error {
	if l.SubnetID != "" {
		subnets, err := c.api.OAPI().ReadSubnets(ctx, osc.ReadSubnetsRequest{
			Filters: &osc.FiltersSubnet{
				SubnetIds: &[]string{l.SubnetID},
			},
		})
		switch {
		case err != nil:
			return fmt.Errorf("find existing subnet: %w", err)
		case len(subnets) == 0:
			return errors.New("find existing subnet: not found")
		}
		l.NetID = subnets[0].GetNetId()
		return nil
	}
	subnets, err := c.api.OAPI().ReadSubnets(ctx, osc.ReadSubnetsRequest{
		Filters: &osc.FiltersSubnet{
			TagKeys: &[]string{clusterIDTagKey(c.clusterID)},
		},
	})
	if err != nil {
		return fmt.Errorf("find subnet: %w", err)
	}
	// Find by role
	ensureByTag := func(key string) bool {
		for _, subnet := range subnets {
			if hasTag(subnet.GetTags(), key) {
				l.SubnetID = subnet.GetSubnetId()
				l.NetID = subnet.GetNetId()
				return true
			}
		}
		return false
	}
	switch {
	case !l.Internal && ensureByTag(tagNameSubnetPublicELB):
	case l.Internal && ensureByTag(tagNameSubnetInternalELB):
	case l.Internal && ensureByTag("OscK8sRole/service.internal"):
	case ensureByTag("OscK8sRole/service"):
	case ensureByTag("OscK8sRole/loadbalancer"):
	default:
		return errors.New("no subnet found with the correct tag")
	}
	return nil
}

func (c *Cloud) ensureSecurityGroup(ctx context.Context, l *LoadBalancer) error {
	if len(l.SecurityGroups) > 0 {
		return nil
	}
	sgName := "k8s-elb-" + l.Name
	sgDescription := fmt.Sprintf("Security group for Kubernetes ELB %s (%v)", l.Name, l.ServiceName)
	resp, err := c.api.OAPI().CreateSecurityGroup(ctx, osc.CreateSecurityGroupRequest{
		SecurityGroupName: sgName,
		Description:       sgDescription,
		NetId:             &l.NetID,
	})
	if err != nil {
		return fmt.Errorf("create SG: %w", err)
	}
	l.SecurityGroups = []string{resp.GetSecurityGroupId()}
	l.lbSecurityGroup = resp
	err = c.api.OAPI().CreateTags(ctx, osc.CreateTagsRequest{
		ResourceIds: l.SecurityGroups,
		Tags:        []osc.ResourceTag{{Key: clusterIDTagKey(c.clusterID), Value: ResourceLifecycleOwned}},
	})
	if err != nil {
		return fmt.Errorf("create SG: %w", err)
	}
	return nil
}

// UpdateLoadBalancer updates a load-balancer.
func (c *Cloud) UpdateLoadBalancer(ctx context.Context, l *LoadBalancer, backend []VM) (dns string, ip string, err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("unable to create LB: %w", err)
		}
	}()
	existing, err := c.getLoadBalancer(ctx, l)
	if err != nil {
		return "", "", fmt.Errorf("check LB: %w", err)
	}
	if existing == nil {
		return "", "", errors.New("existing LBU not found")
	}

	err = c.updateListeners(ctx, l, existing)
	if err == nil {
		// proxy protocol requires listeners to be set
		err = c.updateProxyProtocol(ctx, l, existing)
	}
	if err == nil {
		err = c.updateSSLCert(ctx, l, existing)
	}
	if err == nil {
		err = c.updateAttributes(ctx, l, existing)
	}
	if err == nil {
		err = c.updateHealthcheck(ctx, l, existing)
	}
	if err == nil {
		err = c.getSecurityGroups(ctx, l, backend, existing)
	}
	if err == nil {
		err = c.updateIngressSecurityGroupRules(ctx, l, existing)
	}
	if err == nil {
		err = c.updateBackendSecurityGroupRules(ctx, l, existing)
	}
	if err == nil {
		err = c.updateBackendVms(ctx, l, backend, existing)
	}

	switch {
	case err != nil:
		return "", "", err
	case !l.Internal && existing.PublicIp == nil:
		return "", "", ErrLoadBalancerIsNotReady
	case existing.DnsName == nil:
		return "", "", ErrLoadBalancerIsNotReady
	default:
		return existing.GetDnsName(), existing.GetPublicIp(), nil
	}
}

func (c *Cloud) updateProxyProtocol(ctx context.Context, l *LoadBalancer, _ *osc.LoadBalancer) error {
	elbu, err := c.api.LBU().DescribeLoadBalancersWithContext(ctx, &elb.DescribeLoadBalancersInput{
		LoadBalancerNames: []*string{aws.String(l.Name)},
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
			LoadBalancerName: aws.String(l.Name),
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
		if err != nil && oapi.AWSErrorCode(err) != elb.ErrCodeDuplicatePolicyNameException {
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
			LoadBalancerName: aws.String(l.Name),
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
	var del, delback []int32
	for _, elistener := range existing.GetListeners() {
		if !slices.ContainsFunc(expect, func(listener osc.ListenerForCreation) bool {
			return oscListenersAreEqual(elistener, listener)
		}) {
			del = append(del, elistener.GetLoadBalancerPort())
			if len(elistener.GetPolicyNames()) > 0 {
				delback = append(delback, elistener.GetBackendPort())
			}
		}
	}
	for _, port := range delback {
		klog.FromContext(ctx).V(2).Info(fmt.Sprintf("Reseting policies on backend port %d", port))
		_, err := c.api.LBU().SetLoadBalancerPoliciesForBackendServerWithContext(ctx, &elb.SetLoadBalancerPoliciesForBackendServerInput{
			LoadBalancerName: aws.String(l.Name),
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
			LoadBalancerName:  l.Name,
			LoadBalancerPorts: del,
		})
		if err != nil {
			return fmt.Errorf("delete unused listeners: %w", err)
		}
	}

	// Add new listeners
	var add []osc.ListenerForCreation
	for _, listener := range expect {
		if !slices.ContainsFunc(existing.GetListeners(), func(elistener osc.Listener) bool {
			return oscListenersAreEqual(elistener, listener)
		}) {
			add = append(add, listener)
		}
	}
	if len(add) > 0 {
		klog.FromContext(ctx).V(2).Info(fmt.Sprintf("Adding %d listeners", len(add)))
		_, err := c.api.OAPI().CreateLoadBalancerListeners(ctx, osc.CreateLoadBalancerListenersRequest{
			LoadBalancerName: l.Name,
			Listeners:        add,
		})
		if err != nil {
			return fmt.Errorf("add new listeners: %w", err)
		}
	}
	return nil
}

func oscListenersAreEqual(actual osc.Listener, expected osc.ListenerForCreation) bool {
	if !protocolsAreEqual(actual.GetLoadBalancerProtocol(), expected.LoadBalancerProtocol) {
		return false
	}
	if !protocolsAreEqual(actual.GetBackendProtocol(), expected.GetBackendProtocol()) {
		return false
	}
	if actual.GetLoadBalancerPort() != expected.LoadBalancerPort {
		return false
	}
	if actual.GetBackendPort() != expected.BackendPort {
		return false
	}
	return true
}

// protocolsAreEqual checks if two ELB protocol strings are considered the same
// Comparison is case insensitive
func protocolsAreEqual(l, r string) bool {
	return strings.EqualFold(l, r)
}

func (c *Cloud) updateSSLCert(ctx context.Context, l *LoadBalancer, existing *osc.LoadBalancer) error {
	for _, listener := range existing.GetListeners() {
		if listener.GetServerCertificateId() != l.ListenerDefaults.SSLCertificate {
			klog.FromContext(ctx).V(2).Info("Changing certificate", "port", listener.GetLoadBalancerPort())
			err := c.api.OAPI().UpdateLoadBalancer(ctx, osc.UpdateLoadBalancerRequest{
				LoadBalancerName:    l.Name,
				LoadBalancerPort:    listener.LoadBalancerPort,
				ServerCertificateId: &l.ListenerDefaults.SSLCertificate,
			})
			if err != nil {
				return fmt.Errorf("set certificate: %w", err)
			}
		}
	}
	return nil
}

func (c *Cloud) updateAttributes(ctx context.Context, l *LoadBalancer, _ *osc.LoadBalancer) error {
	existing, err := c.api.LBU().DescribeLoadBalancerAttributesWithContext(ctx, &elb.DescribeLoadBalancerAttributesInput{
		LoadBalancerName: aws.String(l.Name),
	})
	if err != nil {
		return fmt.Errorf("check LB attributes: %w", err)
	}
	expected := l.elbAttributes()
	if !accessLogAttributesAreEqual(existing.LoadBalancerAttributes, expected) {
		klog.FromContext(ctx).V(2).Info("Updating access log attributes")
		_, err := c.api.LBU().ModifyLoadBalancerAttributesWithContext(ctx, &elb.ModifyLoadBalancerAttributesInput{
			LoadBalancerName: aws.String(l.Name),
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
			LoadBalancerName: aws.String(l.Name),
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
		if expected.GetPath() == actual.GetPath() &&
			expected.HealthyThreshold == actual.HealthyThreshold &&
			expected.UnhealthyThreshold == actual.UnhealthyThreshold &&
			expected.CheckInterval == actual.CheckInterval &&
			expected.Timeout == actual.Timeout {
			return nil
		}
	}
	klog.FromContext(ctx).V(2).Info("Configuring healthcheck")
	err := c.api.OAPI().UpdateLoadBalancer(ctx, osc.UpdateLoadBalancerRequest{
		LoadBalancerName: l.Name,
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
		if existing != nil && slices.Contains(existing.GetBackendVmIds(), vm.ID) {
			continue
		}
		add = append(add, vm.ID)
	}
	if len(add) > 0 {
		klog.FromContext(ctx).V(2).Info("Adding backend instances", "count", len(add))
		err := c.api.OAPI().RegisterVmsInLoadBalancer(ctx, osc.RegisterVmsInLoadBalancerRequest{
			LoadBalancerName: l.Name,
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
	for _, i := range existing.GetBackendVmIds() {
		if slices.ContainsFunc(vms, func(vm VM) bool {
			return i == vm.ID
		}) {
			continue
		}
		remove = append(remove, i)
	}
	if len(remove) > 0 {
		klog.FromContext(ctx).V(2).Info("Removing backend instances", "count", len(remove))
		err := c.api.OAPI().DeregisterVmsInLoadBalancer(ctx, osc.DeregisterVmsInLoadBalancerRequest{
			LoadBalancerName: l.Name,
			BackendVmIds:     remove,
		})
		if err != nil {
			return fmt.Errorf("deregister instances: %w", err)
		}
	}

	return nil
}

func (c *Cloud) getSecurityGroups(ctx context.Context, l *LoadBalancer, vms []VM, existing *osc.LoadBalancer) error {
	if l.lbSecurityGroup == nil {
		var (
			lbSG []string
		)
		if existing != nil {
			lbSG = existing.GetSecurityGroups()
		} else {
			lbSG = l.SecurityGroups
		}
		srcSGID := lbSG[0]
		sgs, err := c.api.OAPI().ReadSecurityGroups(ctx, osc.ReadSecurityGroupsRequest{
			Filters: &osc.FiltersSecurityGroup{
				SecurityGroupIds: &[]string{srcSGID},
			},
		})
		if err != nil {
			return fmt.Errorf("list SGs: %w", err)
		}
		if len(sgs) == 0 {
			return errors.New("no SG found for load balancer")
		}
		l.lbSecurityGroup = &sgs[0]
	}
	if l.targetSecurityGroup == nil {
		sgIDs := sets.Set[string]{}
		for _, vm := range vms {
			for _, sg := range *vm.cloudVm.SecurityGroups {
				sgIDs.Insert(sg.GetSecurityGroupId())
			}
		}
		sgs, err := c.api.OAPI().ReadSecurityGroups(ctx, osc.ReadSecurityGroupsRequest{
			Filters: &osc.FiltersSecurityGroup{
				SecurityGroupIds: ptr.To(sets.List(sgIDs)),
			},
		})
		if err != nil {
			return fmt.Errorf("list SGs: %w", err)
		}
		if len(sgs) == 0 {
			return errors.New("no SG found for target nodes")
		}
		roleTagCount := math.MaxInt
		for _, sg := range sgs {
			if hasTag(sg.GetTags(), mainSGTagKey(c.clusterID)) {
				klog.FromContext(ctx).V(4).Info("Found security group having main tag", "securityGroupId", sg.GetSecurityGroupId())
				l.targetSecurityGroup = &sg
			}
			if hasTag(sg.GetTags(), roleTagKey(l.TargetRole)) {
				nRoleTagCount := countRoleTags(sg.GetTags())
				if nRoleTagCount < roleTagCount {
					klog.FromContext(ctx).V(4).Info("Found security group having role tag", "securityGroupId", sg.GetSecurityGroupId(), "role", l.TargetRole, "nroles", nRoleTagCount)
					l.targetSecurityGroup = &sg
					roleTagCount = nRoleTagCount
				}
			}
		}
		if l.targetSecurityGroup == nil {
			klog.FromContext(ctx).V(3).Info("No security group found by tag, using a random one", "securityGroupId", sgs[0].GetSecurityGroupId())
			l.targetSecurityGroup = &sgs[0]
		}
	}
	return nil
}

func (c *Cloud) updateIngressSecurityGroupRules(ctx context.Context, l *LoadBalancer, existing *osc.LoadBalancer) error {
	lbSG := l.lbSecurityGroup
	allowed := l.AllowFrom.StringSlice()
	// sort slice to get a deterministic order for tests
	slices.Sort(allowed)
	// Adding new rules
	for _, listener := range l.Listeners {
		var addRanges []string
		for _, allowFrom := range allowed {
			if !slices.ContainsFunc(lbSG.GetInboundRules(), func(r osc.SecurityGroupRule) bool {
				return r.GetFromPortRange() == listener.Port &&
					slices.Contains(r.GetIpRanges(), allowFrom)
			}) {
				addRanges = append(addRanges, allowFrom)
			}
		}
		if len(addRanges) == 0 {
			continue
		}
		klog.FromContext(ctx).V(2).Info("Adding rule", "from", addRanges, "to", lbSG.GetSecurityGroupId(), "port", listener.Port)
		_, err := c.api.OAPI().CreateSecurityGroupRule(ctx, osc.CreateSecurityGroupRuleRequest{
			SecurityGroupId: lbSG.GetSecurityGroupId(),
			Flow:            "Inbound",
			Rules: &[]osc.SecurityGroupRule{{
				IpProtocol:    ptr.To("tcp"),
				FromPortRange: &listener.Port,
				ToPortRange:   &listener.Port,
				IpRanges:      &addRanges,
			}},
		})
		if err != nil {
			return fmt.Errorf("add ingress rule: %w", err)
		}
	}

	// Removing rules
	for _, r := range lbSG.GetInboundRules() {
		del := false
		var delRanges []string
		if !slices.ContainsFunc(l.Listeners, func(listener Listener) bool {
			return listener.Port == r.GetFromPortRange()
		}) {
			del = true
		}
		if del {
			delRanges = r.GetIpRanges()
		} else {
			for _, ipRange := range r.GetIpRanges() {
				if !slices.Contains(allowed, ipRange) {
					delRanges = append(delRanges, ipRange)
					del = true
				}
			}
		}
		if !del {
			continue
		}
		klog.FromContext(ctx).V(2).Info("Deleting rule", "from", delRanges, "to", lbSG.GetSecurityGroupId(), "port", r.GetFromPortRange())
		_, err := c.api.OAPI().DeleteSecurityGroupRule(ctx, osc.DeleteSecurityGroupRuleRequest{
			SecurityGroupId: lbSG.GetSecurityGroupId(),
			Flow:            "Inbound",
			Rules: &[]osc.SecurityGroupRule{{
				IpProtocol:    r.IpProtocol,
				FromPortRange: r.FromPortRange,
				ToPortRange:   r.ToPortRange,
				IpRanges:      &delRanges,
			}},
		})
		if err != nil {
			return fmt.Errorf("delete ingress rule: %w", err)
		}
	}
	return nil
}

func (c *Cloud) updateBackendSecurityGroupRules(ctx context.Context, l *LoadBalancer, existing *osc.LoadBalancer) error {
	srcSGID := l.lbSecurityGroup.GetSecurityGroupId()
	destSG := l.targetSecurityGroup

	// Adding new rules
	for _, listener := range l.Listeners {
		if slices.ContainsFunc(destSG.GetInboundRules(), func(r osc.SecurityGroupRule) bool {
			return r.GetFromPortRange() == listener.BackendPort &&
				slices.ContainsFunc(r.GetSecurityGroupsMembers(), func(m osc.SecurityGroupsMember) bool {
					return m.HasSecurityGroupId() && srcSGID == m.GetSecurityGroupId()
				})
		}) {
			continue
		}
		klog.FromContext(ctx).V(2).Info("Adding rule", "from", srcSGID, "to", destSG.GetSecurityGroupId(), "port", listener.BackendPort)
		_, err := c.api.OAPI().CreateSecurityGroupRule(ctx, osc.CreateSecurityGroupRuleRequest{
			SecurityGroupId: destSG.GetSecurityGroupId(),
			Flow:            "Inbound",
			Rules: &[]osc.SecurityGroupRule{{
				IpProtocol:    ptr.To("tcp"),
				FromPortRange: &listener.BackendPort,
				ToPortRange:   &listener.BackendPort,
				SecurityGroupsMembers: &[]osc.SecurityGroupsMember{{
					SecurityGroupId: &srcSGID,
				}},
			}},
		})
		if err != nil {
			return fmt.Errorf("add backend rule: %w", err)
		}
	}

	// Removing rules
	for _, r := range destSG.GetInboundRules() {
		// ignore if rule is not from the LB SG
		if !slices.ContainsFunc(r.GetSecurityGroupsMembers(), func(m osc.SecurityGroupsMember) bool {
			return m.GetSecurityGroupId() == srcSGID
		}) {
			continue
		}
		// ignore if port is not from a lister
		if slices.ContainsFunc(l.Listeners, func(listener Listener) bool {
			return listener.BackendPort == r.GetFromPortRange()
		}) {
			continue
		}
		klog.FromContext(ctx).V(2).Info("Deleting rule", "from", srcSGID, "to", destSG.GetSecurityGroupId(), "port", r.GetFromPortRange())
		_, err := c.api.OAPI().DeleteSecurityGroupRule(ctx, osc.DeleteSecurityGroupRuleRequest{
			SecurityGroupId: destSG.GetSecurityGroupId(),
			Flow:            "Inbound",
			Rules: &[]osc.SecurityGroupRule{{
				IpProtocol:    r.IpProtocol,
				FromPortRange: r.FromPortRange,
				ToPortRange:   r.ToPortRange,
				SecurityGroupsMembers: &[]osc.SecurityGroupsMember{{
					SecurityGroupId: &srcSGID,
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
	existing, err := c.getLoadBalancer(ctx, l)
	if err != nil {
		return fmt.Errorf("check LB: %w", err)
	}
	if existing == nil {
		return nil
	}
	// remove all backend VMs
	err = c.updateBackendVms(ctx, l, nil, existing)
	if err != nil {
		return fmt.Errorf("deregister instances: %w", err)
	}
	// Tag LB SG as to be deleted (only if it has been created)
	for _, sg := range existing.GetSecurityGroups() {
		if !slices.Contains(l.SecurityGroups, sg) && !slices.Contains(l.AdditionalSecurityGroups, sg) {
			klog.FromContext(ctx).V(2).Info("Marking SG for deletion", "securityGroupId", sg)
			err = c.api.OAPI().CreateTags(ctx, osc.CreateTagsRequest{
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
	err = c.api.OAPI().DeleteLoadBalancer(ctx, osc.DeleteLoadBalancerRequest{
		LoadBalancerName: l.Name,
	})
	if err != nil {
		return fmt.Errorf("delete LBU: %w", err)
	}
	return nil
}

// RunGarbageCollector deletes LB security groups
func (c *Cloud) RunGarbageCollector(ctx context.Context) error {
	// We collect all the SG from the cluster
	// This is the list of SG we will scan to find rules linking to the SG to be deleted.
	sgs, err := c.api.OAPI().ReadSecurityGroups(ctx, osc.ReadSecurityGroupsRequest{
		Filters: &osc.FiltersSecurityGroup{
			TagKeys: &[]string{clusterIDTagKey(c.clusterID)},
		},
	})
	if err != nil {
		return fmt.Errorf("find security groups: %w", err)
	}
	// Find SG to delete
	var toDelete []string
	for _, sg := range sgs {
		if hasTag(sg.GetTags(), SGToDeleteTagKey) {
			toDelete = append(toDelete, sg.GetSecurityGroupId())
		}
	}
	klog.FromContext(ctx).V(4).Info("Security groups marked for deletion", "count", len(toDelete))
	for _, delSGID := range toDelete {
		// delete all inbound rules from this SG
		for _, sg := range sgs {
			klog.FromContext(ctx).V(2).Info("Deleting inbound rule", "from", delSGID, "to", sg.GetSecurityGroupId())
			for _, r := range sg.GetInboundRules() {
				if slices.ContainsFunc(r.GetSecurityGroupsMembers(), func(m osc.SecurityGroupsMember) bool {
					return slices.Contains(toDelete, m.GetSecurityGroupId())
				}) {
					_, err = c.api.OAPI().DeleteSecurityGroupRule(ctx, osc.DeleteSecurityGroupRuleRequest{
						SecurityGroupId: sg.GetSecurityGroupId(),
						Flow:            "Inbound",
						Rules: &[]osc.SecurityGroupRule{{
							FromPortRange: r.FromPortRange,
							ToPortRange:   r.ToPortRange,
							IpProtocol:    r.IpProtocol,
							SecurityGroupsMembers: &[]osc.SecurityGroupsMember{{
								SecurityGroupId: &delSGID,
							}},
						}},
					})
					if err != nil {
						return fmt.Errorf("delete rule from %s to %s: %w", delSGID, sg.GetSecurityGroupId(), err)
					}
				}
			}
		}
		klog.FromContext(ctx).V(2).Info("Deleting SG", "securityGroupId", delSGID)
		err = c.api.OAPI().DeleteSecurityGroup(ctx, osc.DeleteSecurityGroupRequest{
			SecurityGroupId: &delSGID,
		})
		if err != nil {
			return fmt.Errorf("delete SG %s: %w", delSGID, err)
		}
	}
	return nil
}
