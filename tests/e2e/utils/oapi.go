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
)

func OAPI() (*oapi.Client, error) {
	return oapi.NewClient(os.Getenv("OSC_REGION"))
}

// GetLoadBalancer describe an LB
func GetLoadBalancer(ctx context.Context, api *oapi.Client, name string) (*elb.LoadBalancerDescription, error) {
	request := &elb.DescribeLoadBalancersInput{}
	request.LoadBalancerNames = []*string{&name}

	response, err := api.LoadBalancing().DescribeLoadBalancersWithContext(ctx, request)
	if err != nil {
		if oapi.AWSErrorCode(err) == elb.ErrCodeAccessPointNotFoundException {
			return nil, nil
		}
		return nil, err
	}

	var ret *elb.LoadBalancerDescription
	for _, loadBalancer := range response.LoadBalancerDescriptions {
		if ret != nil {
			return nil, fmt.Errorf("found multiple load balancers with name: %s", name)
		}
		ret = loadBalancer
	}

	return ret, nil
}

func ExpectLoadBalancer(ctx context.Context, api *oapi.Client, name string, matcher types.GomegaMatcher) {
	err := framework.Gomega().Eventually(ctx, func(ctx context.Context) (*elb.LoadBalancerDescription, error) {
		return GetLoadBalancer(ctx, api, name)
	}).
		WithTimeout(10 * time.Minute).WithPolling(30 * time.Second).
		Should(matcher)
	framework.ExpectNoError(err)
}

func ExpectNoLoadBalancer(ctx context.Context, api *oapi.Client, name string) {
	_ = framework.Gomega().Eventually(ctx, func(ctx context.Context) (*elb.LoadBalancerDescription, error) {
		return GetLoadBalancer(ctx, api, name)
	}).
		WithTimeout(10 * time.Minute).WithPolling(30 * time.Second).
		Should(gomega.BeNil())
}

func GetSecurityGroups(ctx context.Context, api *oapi.Client, lb *elb.LoadBalancerDescription) ([]osc.SecurityGroup, error) {
	sgIds := aws.StringValueSlice(lb.SecurityGroups)
	return api.OAPI().ReadSecurityGroups(ctx, osc.ReadSecurityGroupsRequest{
		Filters: &osc.FiltersSecurityGroup{
			SecurityGroupIds: &sgIds,
		},
	})
}

func ExpectSecurityGroups(ctx context.Context, api *oapi.Client, lb *elb.LoadBalancerDescription, matcher types.GomegaMatcher) {
	err := framework.Gomega().Eventually(ctx, func(ctx context.Context) ([]osc.SecurityGroup, error) {
		return GetSecurityGroups(ctx, api, lb)
	}).
		WithTimeout(10 * time.Minute).WithPolling(30 * time.Second).
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
