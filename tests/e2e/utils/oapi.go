package e2eutils

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elb"
	"github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	"github.com/outscale/cloud-provider-osc/cloud-controller-manager/osc/oapi"
	"github.com/outscale/osc-sdk-go/v2"
	"k8s.io/kubernetes/test/e2e/framework"
	"k8s.io/utils/ptr"
)

func OAPI() (*oapi.Client, error) {
	return oapi.NewClient(os.Getenv("OSC_REGION"))
}

// GetLoadBalancer describe an LB
func GetLoadBalancer(ctx context.Context, api *oapi.Client, name string) (*elb.LoadBalancerDescription, error) {
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

func GetLoadBalancerTags(ctx context.Context, api *oapi.Client, name string) ([]osc.ResourceTag, error) {
	if len(name) > 32 {
		name = name[:32]
	}
	response, err := api.OAPI().ReadLoadBalancers(ctx, osc.ReadLoadBalancersRequest{
		Filters: &osc.FiltersLoadBalancer{
			LoadBalancerNames: &[]string{name},
		},
	})
	if err != nil {
		return nil, err
	}

	switch len(response) {
	case 0:
		return nil, nil
	case 1:
		return response[0].GetTags(), nil
	default:
		return nil, fmt.Errorf("found multiple load balancers with name: %s", name)
	}
}

func ExpectLoadBalancer(ctx context.Context, api *oapi.Client, name string, matcher types.GomegaMatcher) {
	err := framework.Gomega().Eventually(ctx, func(ctx context.Context) (*elb.LoadBalancerDescription, error) {
		return GetLoadBalancer(ctx, api, name)
	}).
		WithTimeout(10 * time.Minute).WithPolling(10 * time.Second).
		Should(matcher)
	framework.ExpectNoError(err)
}

func ExpectLoadBalancerTags(ctx context.Context, api *oapi.Client, name string, matcher types.GomegaMatcher) {
	err := framework.Gomega().Eventually(ctx, func(ctx context.Context) ([]osc.ResourceTag, error) {
		return GetLoadBalancerTags(ctx, api, name)
	}).
		WithTimeout(10 * time.Minute).WithPolling(10 * time.Second).
		Should(matcher)
	framework.ExpectNoError(err)
}

func ExpectNoLoadBalancer(ctx context.Context, api *oapi.Client, name string) {
	_ = framework.Gomega().Eventually(ctx, func(ctx context.Context) (*elb.LoadBalancerDescription, error) {
		return GetLoadBalancer(ctx, api, name)
	}).
		WithTimeout(10 * time.Minute).WithPolling(10 * time.Second).
		Should(gomega.BeNil())
}

func GetSecurityGroups(ctx context.Context, api *oapi.Client, lb *elb.LoadBalancerDescription) ([]osc.SecurityGroup, error) {
	return api.OAPI().ReadSecurityGroups(ctx, osc.ReadSecurityGroupsRequest{
		Filters: &osc.FiltersSecurityGroup{
			SecurityGroupIds: ptr.To(aws.StringValueSlice(lb.SecurityGroups)),
		},
	})
}

func ExpectSecurityGroups(ctx context.Context, api *oapi.Client, lb *elb.LoadBalancerDescription, matcher types.GomegaMatcher) {
	err := framework.Gomega().Eventually(ctx, func(ctx context.Context) ([]osc.SecurityGroup, error) {
		return GetSecurityGroups(ctx, api, lb)
	}).
		WithTimeout(10 * time.Minute).WithPolling(10 * time.Second).
		Should(matcher)
	framework.ExpectNoError(err)
}

func DeleteBackendVMs(ctx context.Context, api *oapi.Client, lb *elb.LoadBalancerDescription) {
	c := api.OAPI().(*oapi.OscClient)
	for _, instance := range lb.Instances {
		_, _, err := c.APIClient().VmApi.DeleteVms(c.WithAuth(ctx)).DeleteVmsRequest(osc.DeleteVmsRequest{
			VmIds: []string{*instance.InstanceId},
		}).Execute()
		framework.ExpectNoError(err)
	}
}
