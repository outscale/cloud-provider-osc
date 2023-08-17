/*
Copyright 2014 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package osc

import (
	"fmt"
	"os"
	"sync"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/ec2metadata"
	"github.com/aws/aws-sdk-go/aws/endpoints"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/elb"

	"github.com/outscale-dev/cloud-provider-osc/cloud-controller-manager/utils"

	"context"

	osc "github.com/outscale/osc-sdk-go/v2"

	"k8s.io/klog/v2"
)

// ********************* CCM awsSDKProvider Def & functions *********************

type awsSDKProvider struct {
	creds *credentials.Credentials
	cfg   awsCloudConfigProvider

	mutex          sync.Mutex
	regionDelayers map[string]*CrossRequestRetryDelay
}

func addOscUserAgent(h *request.Handlers) {
	// addUserAgent is a named handler that will add information to requests made by the AWS SDK.
	var addUserAgent = request.NamedHandler{
		Name: "cloud-provider-osc/user-agent",
		Fn:   request.MakeAddToUserAgentHandler("osc-cloud-controller-manager", utils.GetVersion()),
	}

	h.Build.PushFrontNamed(addUserAgent)
}

func (p *awsSDKProvider) addHandlers(regionName string, h *request.Handlers) {
	addOscUserAgent(h)

	h.Sign.PushFrontNamed(request.NamedHandler{
		Name: "k8s/logger",
		Fn:   awsHandlerLogger,
	})

	delayer := p.getCrossRequestRetryDelay(regionName)
	if delayer != nil {
		h.Sign.PushFrontNamed(request.NamedHandler{
			Name: "k8s/delay-presign",
			Fn:   delayer.BeforeSign,
		})

		h.AfterRetry.PushFrontNamed(request.NamedHandler{
			Name: "k8s/delay-afterretry",
			Fn:   delayer.AfterRetry,
		})
	}

	p.addAPILoggingHandlers(h)
}

func (p *awsSDKProvider) addAPILoggingHandlers(h *request.Handlers) {
	debugPrintCallerFunctionName()
	h.Send.PushBackNamed(request.NamedHandler{
		Name: "k8s/api-request",
		Fn:   awsSendHandlerLogger,
	})

	h.ValidateResponse.PushFrontNamed(request.NamedHandler{
		Name: "k8s/api-validate-response",
		Fn:   awsValidateResponseHandlerLogger,
	})
}

// Get a CrossRequestRetryDelay, scoped to the region, not to the request.
// This means that when we hit a limit on a call, we will delay _all_ calls to the API.
// We do this to protect the AWS account from becoming overloaded and effectively locked.
// We also log when we hit request limits.
// Note that this delays the current goroutine; this is bad behaviour and will
// likely cause k8s to become slow or unresponsive for cloud operations.
// However, this throttle is intended only as a last resort.  When we observe
// this throttling, we need to address the root cause (e.g. add a delay to a
// controller retry loop)
func (p *awsSDKProvider) getCrossRequestRetryDelay(regionName string) *CrossRequestRetryDelay {
	debugPrintCallerFunctionName()
	klog.V(5).Infof("getCrossRequestRetryDelay(%v)", regionName)
	p.mutex.Lock()
	defer p.mutex.Unlock()

	delayer, found := p.regionDelayers[regionName]
	if !found {
		delayer = NewCrossRequestRetryDelay()
		p.regionDelayers[regionName] = delayer
	}
	return delayer
}

func NewOscClient(regionName string) (context.Context, *osc.APIClient, error) {
	configEnv := osc.NewConfigEnv()
	config, err := configEnv.Configuration()
	if err != nil {
		return nil, nil, err
	}
	config.Debug = true
	config.UserAgent = fmt.Sprintf("osc-cloud-controller-manager/%v", utils.GetVersion())
	client := osc.NewAPIClient(config)
	ctx := context.WithValue(context.Background(), osc.ContextAWSv4, osc.AWSv4{
		AccessKey: os.Getenv("OSC_ACCESS_KEY"),
		SecretKey: os.Getenv("OSC_SECRET_KEY"),
	})
	ctx = context.WithValue(ctx, osc.ContextServerIndex, 0)
	ctx = context.WithValue(ctx, osc.ContextServerVariables, map[string]string{"region": regionName})
	return ctx, client, err
}
func (p *awsSDKProvider) Compute(regionName string) (Compute, error) {
	debugPrintCallerFunctionName()
	klog.V(5).Infof("Compute(%v)", regionName)
	// osc config
	ctx, client, err := NewOscClient(regionName)
	if err != nil {
		return nil, err
	}

	sdk := &oscSdkCompute{
		client: client,
		ctx:    ctx,
	}

	return sdk, nil
}

func (p *awsSDKProvider) LoadBalancing(regionName string) (LoadBalancer, error) {
	debugPrintCallerFunctionName()
	klog.V(5).Infof("LoadBalancing(%v)", regionName)
	sess, err := NewSession(nil)
	if err != nil {
		return nil, fmt.Errorf("unable to initialize AWS session: %v", err)
	}
	elbClient := elb.New(sess)
	p.addHandlers(regionName, &elbClient.Handlers)

	return elbClient, nil
}

func (p *awsSDKProvider) Metadata() (EC2Metadata, error) {
	debugPrintCallerFunctionName()
	klog.V(5).Infof("Metadata()")
	awsConfig := &aws.Config{
		EndpointResolver: endpoints.ResolverFunc(SetupMetadataResolver()),
	}
	awsConfig.WithLogLevel(aws.LogDebugWithSigning | aws.LogDebugWithHTTPBody | aws.LogDebugWithRequestRetries | aws.LogDebugWithRequestErrors)
	sess := session.Must(session.NewSession(awsConfig))

	addOscUserAgent(&sess.Handlers)

	client := ec2metadata.New(sess)
	p.addAPILoggingHandlers(&client.Handlers)
	metadata, err := NewMetadataService(client)
	if err != nil {
		return nil, fmt.Errorf("could not get metadata from AWS: %v", err)
	}
	return metadata.(EC2Metadata), err
}
