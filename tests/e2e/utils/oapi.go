package e2eutils

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go/aws"         //nolint:staticcheck
	"github.com/aws/aws-sdk-go/service/elb" //nolint:staticcheck
	"github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	"github.com/outscale/cloud-provider-osc/ccm/oapi"
	"github.com/outscale/goutils/sdk/ptr"
	"github.com/outscale/osc-sdk-go/v3/pkg/osc"
	"k8s.io/kubernetes/test/e2e/framework"
)

// GetLoadBalancer describe an LB
func GetLoadBalancer(ctx context.Context, api oapi.Clienter, name string) (*elb.LoadBalancerDescription, error) {
	if len(name) > 32 {
		name = name[:32]
	}
	response, err := api.LBU().DescribeLoadBalancersWithContext(ctx, &elb.DescribeLoadBalancersInput{
		LoadBalancerNames: []*string{&name},
	})
	if err != nil {
		return nil, err
	}

	switch len(response.LoadBalancerDescriptions) {
	case 0:
		return nil, nil
	case 1:
		return response.LoadBalancerDescriptions[0], nil
	default:
		return nil, fmt.Errorf("found multiple load balancers with name: %s", name)
	}
}

func GetLoadBalancerTags(ctx context.Context, api oapi.Clienter, name string) ([]osc.ResourceTag, error) {
	if len(name) > 32 {
		name = name[:32]
	}
	resp, err := api.OAPI().ReadLoadBalancers(ctx, osc.ReadLoadBalancersRequest{
		Filters: &osc.FiltersLoadBalancer{
			LoadBalancerNames: &[]string{name},
		},
	})
	if err != nil {
		return nil, err
	}

	switch len(*resp.LoadBalancers) {
	case 0:
		return nil, nil
	case 1:
		return (*resp.LoadBalancers)[0].Tags, nil
	default:
		return nil, fmt.Errorf("found multiple load balancers with name: %s", name)
	}
}

func ExpectLoadBalancer(ctx context.Context, api oapi.Clienter, name string, matcher types.GomegaMatcher) {
	err := framework.Gomega().Eventually(ctx, func(ctx context.Context) (*elb.LoadBalancerDescription, error) {
		return GetLoadBalancer(ctx, api, name)
	}).
		WithTimeout(10 * time.Minute).WithPolling(10 * time.Second).
		Should(matcher)
	framework.ExpectNoError(err)
}

func ExpectLoadBalancerTags(ctx context.Context, api oapi.Clienter, name string, matcher types.GomegaMatcher) {
	err := framework.Gomega().Eventually(ctx, func(ctx context.Context) ([]osc.ResourceTag, error) {
		return GetLoadBalancerTags(ctx, api, name)
	}).
		WithTimeout(10 * time.Minute).WithPolling(10 * time.Second).
		Should(matcher)
	framework.ExpectNoError(err)
}

func ExpectNoLoadBalancer(ctx context.Context, api oapi.Clienter, name string) {
	_ = framework.Gomega().Eventually(ctx, func(ctx context.Context) (*elb.LoadBalancerDescription, error) {
		return GetLoadBalancer(ctx, api, name)
	}).
		WithTimeout(10 * time.Minute).WithPolling(10 * time.Second).
		Should(gomega.BeNil())
}

func GetSecurityGroups(ctx context.Context, api oapi.Clienter, lb *elb.LoadBalancerDescription) ([]osc.SecurityGroup, error) {
	resp, err := api.OAPI().ReadSecurityGroups(ctx, osc.ReadSecurityGroupsRequest{
		Filters: &osc.FiltersSecurityGroup{
			SecurityGroupIds: ptr.To(aws.StringValueSlice(lb.SecurityGroups)),
		},
	})
	if err != nil {
		return nil, err
	}
	return *resp.SecurityGroups, nil
}

func ExpectSecurityGroups(ctx context.Context, api oapi.Clienter, lb *elb.LoadBalancerDescription, matcher types.GomegaMatcher) {
	err := framework.Gomega().Eventually(ctx, func(ctx context.Context) ([]osc.SecurityGroup, error) {
		return GetSecurityGroups(ctx, api, lb)
	}).
		WithTimeout(10 * time.Minute).WithPolling(10 * time.Second).
		Should(matcher)
	framework.ExpectNoError(err)
}
