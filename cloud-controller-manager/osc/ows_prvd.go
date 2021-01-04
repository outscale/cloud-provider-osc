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
	"sync"

    "github.com/outscale/osc-sdk-go/osc"

	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/ec2metadata"
	"github.com/aws/aws-sdk-go/aws/endpoints"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"


	"k8s.io/client-go/pkg/version"
	"k8s.io/klog"
)

// ********************* CCM awsSDKProvider Def & functions *********************

type oscSDKProvider struct {
	creds *credentials.Credentials
	cfg   oscCloudConfigProvider

	mutex          sync.Mutex
	regionDelayers map[string]*CrossRequestRetryDelay
}

func (p *oscSDKProvider) addHandlers(regionName string, h *request.Handlers) {
	h.Build.PushFrontNamed(request.NamedHandler{
		Name: "k8s/user-agent",
		Fn:   request.MakeAddToUserAgentHandler("kubernetes", version.Get().String()),
	})

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

func (p *oscSDKProvider) addAPILoggingHandlers(h *request.Handlers) {
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
func (p *oscSDKProvider) getCrossRequestRetryDelay(regionName string) *CrossRequestRetryDelay {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("getCrossRequestRetryDelay(%v)", regionName)
	p.mutex.Lock()
	defer p.mutex.Unlock()

	delayer, found := p.regionDelayers[regionName]
	if !found {
		delayer = NewCrossRequestRetryDelay()
		p.regionDelayers[regionName] = delayer
	}
	return delayer
}

func (p *oscSDKProvider) Compute(regionName string) (FCU, error) {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("Compute(%v)", regionName)
	sess, err := NewSession()
	if err != nil {
		return nil, fmt.Errorf("unable to initialize AWS session: %v", err)
	}

	p.addHandlers(regionName, &service.Handlers)

	client := &OscClient{}
	client.config = osc.NewConfiguration()
	client.config.BasePath, _ = client.config.ServerUrl(0, map[string]string{"region": useRegion})
	client.api = osc.NewAPIClient(client.config)
	client.auth = context.WithValue(context.Background(), osc.ContextAWSv4, osc.AWSv4{
		AccessKey: os.Getenv("OSC_ACCESS_KEY"),
		SecretKey: os.Getenv("OSC_SECRET_KEY"),
	})


	fcu := &oscSdkFCU{
		config: client.config,
	    auth:   client.auth,
	    api:    client.api,
	}

	return fcu, nil
}

func (p *oscSDKProvider) LoadBalancing(regionName string) (LBU, error) {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("LoadBalancing(%v)", regionName)
	sess, err := NewSession()
	if err != nil {
		return nil, fmt.Errorf("unable to initialize AWS session: %v", err)
	}
	lbuClient := lbu.New(sess)
	p.addHandlers(regionName, &lbuClient.Handlers)

	return elbClient, nil
}

func (p *oscSDKProvider) Metadata() (EC2Metadata, error) {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("Metadata()")
	sess, err := session.NewSession(&aws.Config{
		EndpointResolver: endpoints.ResolverFunc(SetupMetadataResolver()),
	})
	if err != nil {
		return nil, fmt.Errorf("unable to initialize OSC session: %v", err)
	}
	client := ec2metadata.New(sess)
	p.addAPILoggingHandlers(&client.Handlers)
	return client, nil
}
