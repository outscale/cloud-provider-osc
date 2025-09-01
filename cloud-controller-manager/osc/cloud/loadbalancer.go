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
	}
	// ErrLoadBalancerIsNotReady is returned by CreateLoadBalancer/UpdateLoadBalancer when the LB is not ready yet.
	ErrLoadBalancerIsNotReady = controllerapi.NewRetryError("load balancer is not ready", 30*time.Second)
)

// HealthCheck is the healcheck configuration.
type HealthCheck struct {
	Interval           int `annotation:"osc-load-balancer-healthcheck-interval"`
	Timeout            int `annotation:"osc-load-balancer-healthcheck-timeout"`
	HealthyThreshold   int `annotation:"osc-load-balancer-healthcheck-healthy-threshold"`
	UnhealthyThreshold int `annotation:"osc-load-balancer-healthcheck-unhealthy-threshold"`

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

// LoadBalancer defines a load-balancer.
type LoadBalancer struct {
	Name                     string `annotation:"osc-load-balancer-name"`
	ServiceName              string
	Internal                 bool              `annotation:"osc-load-balancer-internal"`
	IPPool                   string            `annotation:"osc-load-balancer-ip-pool"`
	SubnetID                 string            `annotation:"osc-load-balancer-subnet-id"`
	SecurityGroups           []string          `annotation:"osc-load-balancer-security-group"`
	AdditionalSecurityGroups []string          `annotation:"osc-load-balancer-extra-security-groups"`
	TargetRole               string            `annotation:"osc-load-balancer-target-role"`
	Tags                     map[string]string `annotation:"osc-load-balancer-additional-resource-tags"`
	HealthCheck              HealthCheck       `annotation:",squash"`
	ListenerDefaults         ListenerDefaults  `annotation:",squash"`
	Listeners                []Listener
	Connection               Connection `annotation:",squash"`
	SessionAffinity          string
	AccessLog                AccessLog `annotation:",squash"`
	AllowFrom                utilnet.IPNetSet

	lbSecurityGroup     *osc.SecurityGroup
	targetSecurityGroup *osc.SecurityGroup
}

var reName = regexp.MustCompile("^[a-zA-Z0-9-]+$")

// NewLoadBalancer creates a new LoadBalancer instance from a Kubernetes Service.
func NewLoadBalancer(svc *v1.Service) (*LoadBalancer, error) {
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

func (l *LoadBalancer) elbListener() []*elb.Listener {
	awsLb := make([]*elb.Listener, 0, len(l.Listeners))
	for _, lstnr := range l.Listeners {
		elbl := elb.Listener{
			LoadBalancerPort: aws.Int64(int64(lstnr.Port)),
			InstancePort:     aws.Int64(int64(lstnr.BackendPort)),
		}
		var protocol string
		backendProtocol := strings.ToLower(l.ListenerDefaults.BackendProtocol)
		if l.ListenerDefaults.SSLCertificate != "" && isPortIn(lstnr.Port, l.ListenerDefaults.SSLPorts) {
			switch {
			case backendProtocol == "http":
				protocol = "https"
			case backendProtocol == "" && lstnr.Port == 443:
				protocol = "https"
				backendProtocol = "http"
			case backendProtocol == "":
				protocol = "ssl"
				backendProtocol = "tcp"
			default:
				protocol = "ssl"
			}
			elbl.SSLCertificateId = aws.String(l.ListenerDefaults.SSLCertificate)
		} else {
			switch {
			case backendProtocol == "http":
				protocol = "http"
			case backendProtocol == "" && lstnr.Port == 80:
				protocol = "http"
				backendProtocol = "http"
			case backendProtocol == "":
				protocol = "tcp"
				backendProtocol = "tcp"
			default:
				protocol = "tcp"
			}
		}

		elbl.Protocol = aws.String(strings.ToUpper(protocol))
		elbl.InstanceProtocol = aws.String(strings.ToUpper(backendProtocol))
		awsLb = append(awsLb, &elbl)
	}
	return awsLb
}

func (l *LoadBalancer) elbHealthCheck() *elb.HealthCheck {
	if l.HealthCheck.Port == 0 {
		return nil
	}
	hc := &elb.HealthCheck{
		Interval:           aws.Int64(int64(l.HealthCheck.Interval)),
		Timeout:            aws.Int64(int64(l.HealthCheck.Timeout)),
		HealthyThreshold:   aws.Int64(int64(l.HealthCheck.HealthyThreshold)),
		UnhealthyThreshold: aws.Int64(int64(l.HealthCheck.UnhealthyThreshold)),
	}
	protocol := strings.ToUpper(l.HealthCheck.Protocol)
	switch protocol {
	case "":
	case "HTTP":
		hc.Target = aws.String(protocol + ":" + strconv.FormatInt(int64(l.HealthCheck.Port), 10) + l.HealthCheck.Path)
	default:
		hc.Target = aws.String(protocol + ":" + strconv.FormatInt(int64(l.HealthCheck.Port), 10))
	}
	return hc
}

// LoadBalancerExists checks if a load-balancer exists.
func (c *Cloud) LoadBalancerExists(ctx context.Context, l *LoadBalancer) (bool, error) {
	tags, err := c.api.LoadBalancing().DescribeTagsWithContext(ctx, &elb.DescribeTagsInput{
		LoadBalancerNames: []*string{&l.Name},
	})
	if err != nil {
		if oapi.AWSErrorCode(err) == elb.ErrCodeAccessPointNotFoundException {
			return false, nil
		}
		return false, err
	}
	if getLBUClusterID(tags) != c.clusterID {
		return false, errors.New("found a LBU with the same name belonging to another cluster")
	}
	svcName := getLBUServiceName(tags)
	if svcName != "" && svcName != l.ServiceName {
		return false, errors.New("found a LBU with the same name belonging to another service")
	}
	return true, nil
}

func (c *Cloud) getLoadBalancer(ctx context.Context, l *LoadBalancer) (*elb.LoadBalancerDescription, error) {
	res, err := c.api.LoadBalancing().DescribeLoadBalancersWithContext(ctx, &elb.DescribeLoadBalancersInput{
		LoadBalancerNames: []*string{&l.Name},
	})
	if err != nil {
		return nil, err
	}
	if len(res.LoadBalancerDescriptions) == 0 {
		return nil, nil
	}
	return res.LoadBalancerDescriptions[0], nil
}

// GetLoadBalancer fetches a load-balancer.
func (c *Cloud) GetLoadBalancer(ctx context.Context, l *LoadBalancer) (dns string, found bool, err error) {
	lb, err := c.getLoadBalancer(ctx, l)
	switch {
	case err != nil:
		return "", false, fmt.Errorf("unable to get LB: %w", err)
	case lb == nil:
		return "", false, nil
	case lb.DNSName != nil:
		return *lb.DNSName, true, nil
	default:
		return "", true, nil
	}
}

// CreateLoadBalancer creates a load-balancer.
func (c *Cloud) CreateLoadBalancer(ctx context.Context, l *LoadBalancer, backend []VM) (dns string, err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("unable to create LB: %w", err)
		}
	}()
	createRequest := &elb.CreateLoadBalancerInput{
		LoadBalancerName: aws.String(l.Name),
		Listeners:        l.elbListener(),
	}

	if l.Internal {
		createRequest.Scheme = aws.String("internal")
	}

	// TODO: drop public cloud code ?
	if c.Self.SubnetID == "" {
		createRequest.AvailabilityZones = []*string{aws.String(c.Metadata.AvailabilityZone)}
	} else {
		// subnet
		err = c.ensureSubnet(ctx, l)
		if err != nil {
			return "", err
		}
		createRequest.Subnets = []*string{&l.SubnetID}

		// security group
		err = c.ensureSecurityGroup(ctx, l)
		if err != nil {
			return "", err
		}
		createRequest.SecurityGroups = aws.StringSlice(append(l.SecurityGroups, l.AdditionalSecurityGroups...))
	}
	tags := l.Tags
	if tags == nil {
		tags = map[string]string{}
	}
	tags[ServiceNameTagKey] = l.ServiceName
	tags[clusterIDTagKey(c.clusterID)] = ResourceLifecycleOwned
	for k, v := range tags {
		createRequest.Tags = append(createRequest.Tags, &elb.Tag{
			Key: aws.String(k), Value: aws.String(v),
		})
	}
	slices.SortFunc(createRequest.Tags, func(a, b *elb.Tag) int {
		switch {
		case *a.Key < *b.Key:
			return -1
		case *a.Key > *b.Key:
			return 1
		default:
			return 0
		}
	})

	klog.FromContext(ctx).V(1).Info("Creating load balancer")
	res, err := c.api.LoadBalancing().CreateLoadBalancerWithContext(ctx, createRequest)
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
		return "", err
	case res.DNSName != nil:
		return *res.DNSName, nil
	default:
		return "", ErrLoadBalancerIsNotReady
	}
}

func (c *Cloud) ensureSubnet(ctx context.Context, l *LoadBalancer) error {
	if l.SubnetID != "" {
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
		NetId:             &c.Self.NetID,
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
func (c *Cloud) UpdateLoadBalancer(ctx context.Context, l *LoadBalancer, backend []VM) (dns string, err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("unable to create LB: %w", err)
		}
	}()
	existing, err := c.getLoadBalancer(ctx, l)
	if err != nil {
		return "", fmt.Errorf("check LB: %w", err)
	}
	if existing == nil {
		return "", errors.New("existing LBU not found")
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
		return "", err
	case existing.DNSName != nil:
		return *existing.DNSName, nil
	default:
		return "", ErrLoadBalancerIsNotReady
	}
}

func (c *Cloud) updateProxyProtocol(ctx context.Context, l *LoadBalancer, existing *elb.LoadBalancerDescription) error {
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
		_, err := c.api.LoadBalancing().CreateLoadBalancerPolicyWithContext(ctx, request)
		if err != nil && oapi.AWSErrorCode(err) != "ErrCodeDuplicatePolicyNameException" {
			return fmt.Errorf("create proxy protocol policy: %w", err)
		}
		policies = []*string{aws.String(proxyProtocolPolicyName)}
	} else {
		policies = []*string{}
	}

	for _, listener := range l.Listeners {
		if !isPortIn(listener.BackendPort, l.ListenerDefaults.ProxyProtocol) {
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
		_, err := c.api.LoadBalancing().SetLoadBalancerPoliciesForBackendServerWithContext(ctx, request)
		if err != nil {
			return fmt.Errorf("set proxy protocol policy: %w", err)
		}
	}
	return nil
}

func (c *Cloud) updateListeners(ctx context.Context, l *LoadBalancer, existing *elb.LoadBalancerDescription) error {
	expect := l.elbListener()

	// Remove unused listeners
	var del, delback []*int64
	for _, elistener := range existing.ListenerDescriptions {
		if !slices.ContainsFunc(expect, func(listener *elb.Listener) bool {
			return elbListenersAreEqual(listener, elistener.Listener)
		}) {
			del = append(del, elistener.Listener.LoadBalancerPort)
			if len(elistener.PolicyNames) > 0 {
				delback = append(delback, elistener.Listener.InstancePort)
			}
		}
	}
	for _, port := range delback {
		klog.FromContext(ctx).V(2).Info(fmt.Sprintf("Reseting policies on backend port %d", *port))
		_, err := c.api.LoadBalancing().SetLoadBalancerPoliciesForBackendServerWithContext(ctx, &elb.SetLoadBalancerPoliciesForBackendServerInput{
			LoadBalancerName: aws.String(l.Name),
			InstancePort:     port,
			PolicyNames:      []*string{},
		})
		if err != nil {
			return fmt.Errorf("unset backend policy: %w", err)
		}
	}
	if len(del) > 0 {
		klog.FromContext(ctx).V(2).Info(fmt.Sprintf("Deleting %d listeners", len(del)))
		_, err := c.api.LoadBalancing().DeleteLoadBalancerListenersWithContext(ctx, &elb.DeleteLoadBalancerListenersInput{
			LoadBalancerName:  aws.String(l.Name),
			LoadBalancerPorts: del,
		})
		if err != nil {
			return fmt.Errorf("delete unused listeners: %w", err)
		}
	}

	// Add new listeners
	var add []*elb.Listener
	for _, listener := range expect {
		if !slices.ContainsFunc(existing.ListenerDescriptions, func(elistener *elb.ListenerDescription) bool {
			return elbListenersAreEqual(listener, elistener.Listener)
		}) {
			add = append(add, listener)
		}
	}
	if len(add) > 0 {
		klog.FromContext(ctx).V(2).Info(fmt.Sprintf("Adding %d listeners", len(add)))
		_, err := c.api.LoadBalancing().CreateLoadBalancerListenersWithContext(ctx, &elb.CreateLoadBalancerListenersInput{
			LoadBalancerName: aws.String(l.Name),
			Listeners:        add,
		})
		if err != nil {
			return fmt.Errorf("add new listeners: %w", err)
		}
	}
	return nil
}

func elbListenersAreEqual(actual, expected *elb.Listener) bool {
	if !elbProtocolsAreEqual(actual.Protocol, expected.Protocol) {
		return false
	}
	if !elbProtocolsAreEqual(actual.InstanceProtocol, expected.InstanceProtocol) {
		return false
	}
	if aws.Int64Value(actual.InstancePort) != aws.Int64Value(expected.InstancePort) {
		return false
	}
	if aws.Int64Value(actual.LoadBalancerPort) != aws.Int64Value(expected.LoadBalancerPort) {
		return false
	}
	return true
}

// elbProtocolsAreEqual checks if two ELB protocol strings are considered the same
// Comparison is case insensitive
func elbProtocolsAreEqual(l, r *string) bool {
	if l == nil || r == nil {
		return l == r
	}
	return strings.EqualFold(aws.StringValue(l), aws.StringValue(r))
}

func (c *Cloud) updateSSLCert(ctx context.Context, l *LoadBalancer, existing *elb.LoadBalancerDescription) error {
	for _, listener := range existing.ListenerDescriptions {
		if aws.StringValue(listener.Listener.SSLCertificateId) != l.ListenerDefaults.SSLCertificate {
			klog.FromContext(ctx).V(2).Info("Changing certificate", "port", *listener.Listener.LoadBalancerPort)
			_, err := c.api.LoadBalancing().SetLoadBalancerListenerSSLCertificateWithContext(ctx, &elb.SetLoadBalancerListenerSSLCertificateInput{
				LoadBalancerName: &l.Name,
				LoadBalancerPort: listener.Listener.LoadBalancerPort,
				SSLCertificateId: &l.ListenerDefaults.SSLCertificate,
			})
			if err != nil {
				return fmt.Errorf("set certificate: %w", err)
			}
		}
	}
	return nil
}

func (c *Cloud) updateAttributes(ctx context.Context, l *LoadBalancer, _ *elb.LoadBalancerDescription) error {
	existing, err := c.api.LoadBalancing().DescribeLoadBalancerAttributesWithContext(ctx, &elb.DescribeLoadBalancerAttributesInput{
		LoadBalancerName: aws.String(l.Name),
	})
	if err != nil {
		return fmt.Errorf("check LB attributes: %w", err)
	}
	expected := l.elbAttributes()
	if !accessLogAttributesAreEqual(existing.LoadBalancerAttributes, expected) {
		klog.FromContext(ctx).V(2).Info("Updating access log attribute")
		_, err := c.api.LoadBalancing().ModifyLoadBalancerAttributesWithContext(ctx, &elb.ModifyLoadBalancerAttributesInput{
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
		klog.FromContext(ctx).V(2).Info("Updating access log attribute")
		_, err := c.api.LoadBalancing().ModifyLoadBalancerAttributesWithContext(ctx, &elb.ModifyLoadBalancerAttributesInput{
			LoadBalancerName: aws.String(l.Name),
			LoadBalancerAttributes: &elb.LoadBalancerAttributes{
				ConnectionDraining: expected.ConnectionDraining,
				ConnectionSettings: expected.ConnectionSettings,
			},
		})
		if err != nil {
			return fmt.Errorf("update access log attribute: %w", err)
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

func (c *Cloud) updateHealthcheck(ctx context.Context, l *LoadBalancer, existing *elb.LoadBalancerDescription) error {
	expected := l.elbHealthCheck()
	switch {
	case (existing == nil) != (expected == nil):
	case existing != nil:
		actual := existing.HealthCheck
		if aws.StringValue(expected.Target) == aws.StringValue(actual.Target) &&
			aws.Int64Value(expected.HealthyThreshold) == aws.Int64Value(actual.HealthyThreshold) &&
			aws.Int64Value(expected.UnhealthyThreshold) == aws.Int64Value(actual.UnhealthyThreshold) &&
			aws.Int64Value(expected.Interval) == aws.Int64Value(actual.Interval) &&
			aws.Int64Value(expected.Timeout) == aws.Int64Value(actual.Timeout) {
			return nil
		}
	}
	klog.FromContext(ctx).V(2).Info("Configuring healthcheck")
	_, err := c.api.LoadBalancing().ConfigureHealthCheckWithContext(ctx, &elb.ConfigureHealthCheckInput{
		LoadBalancerName: &l.Name,
		HealthCheck:      expected,
	})
	if err != nil {
		return fmt.Errorf("configure health check: %w", err)
	}

	return nil
}

func (c *Cloud) updateBackendVms(ctx context.Context, l *LoadBalancer, vms []VM, existing *elb.LoadBalancerDescription) error {
	// in most cases, there will be no change, preallocating would waste an alloc
	var add []*elb.Instance //nolint:prealloc
	for _, vm := range vms {
		if existing != nil && slices.ContainsFunc(existing.Instances, func(i *elb.Instance) bool {
			return aws.StringValue(i.InstanceId) == vm.ID
		}) {
			continue
		}
		add = append(add, &elb.Instance{InstanceId: &vm.ID})
	}
	if len(add) > 0 {
		klog.FromContext(ctx).V(2).Info("Adding backend instances", "count", len(add))
		_, err := c.api.LoadBalancing().RegisterInstancesWithLoadBalancerWithContext(ctx, &elb.RegisterInstancesWithLoadBalancerInput{
			LoadBalancerName: &l.Name,
			Instances:        add,
		})
		if err != nil {
			return fmt.Errorf("register instances: %w", err)
		}
	}
	if existing == nil {
		return nil
	}
	var remove []*elb.Instance //nolint:prealloc
	for _, i := range existing.Instances {
		if slices.ContainsFunc(vms, func(vm VM) bool {
			return aws.StringValue(i.InstanceId) == vm.ID
		}) {
			continue
		}
		remove = append(remove, i)
	}
	if len(remove) > 0 {
		klog.FromContext(ctx).V(2).Info("Removing backend instances", "count", len(remove))
		_, err := c.api.LoadBalancing().DeregisterInstancesFromLoadBalancerWithContext(ctx, &elb.DeregisterInstancesFromLoadBalancerInput{
			LoadBalancerName: &l.Name,
			Instances:        remove,
		})
		if err != nil {
			return fmt.Errorf("deregister instances: %w", err)
		}
	}

	return nil
}

func (c *Cloud) getSecurityGroups(ctx context.Context, l *LoadBalancer, vms []VM, existing *elb.LoadBalancerDescription) error {
	if l.lbSecurityGroup == nil {
		var (
			lbSG []string
		)
		if existing != nil {
			lbSG = aws.StringValueSlice(existing.SecurityGroups)
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

func (c *Cloud) updateIngressSecurityGroupRules(ctx context.Context, l *LoadBalancer, existing *elb.LoadBalancerDescription) error {
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

func (c *Cloud) updateBackendSecurityGroupRules(ctx context.Context, l *LoadBalancer, existing *elb.LoadBalancerDescription) error {
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
	for _, sg := range existing.SecurityGroups {
		if !slices.Contains(l.SecurityGroups, *sg) && !slices.Contains(l.AdditionalSecurityGroups, *sg) {
			klog.FromContext(ctx).V(2).Info("Marking SG for deletion", "securityGroupId", sg)
			err = c.api.OAPI().CreateTags(ctx, osc.CreateTagsRequest{
				ResourceIds: []string{*sg},
				Tags:        []osc.ResourceTag{{Key: SGToDeleteTagKey}},
			})
			if err != nil {
				return fmt.Errorf("mark SG for deletion: %w", err)
			}
		}
	}

	// Delete the load balancer itself
	klog.FromContext(ctx).V(2).Info("Deleting load-balancer")
	_, err = c.api.LoadBalancing().DeleteLoadBalancerWithContext(ctx, &elb.DeleteLoadBalancerInput{
		LoadBalancerName: aws.String(l.Name),
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
