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

package oapi

import (
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/outscale/cloud-provider-osc/ccm/utils"
	"github.com/outscale/osc-sdk-go/v3/pkg/profile"
)

// NewSession create a new AWS client session, using OSC credentials.
func NewSession(region string, prof *profile.Profile) (*session.Session, error) {
	awsConfig := &aws.Config{
		Region: aws.String(region),
		Credentials: credentials.NewCredentials(&credentials.StaticProvider{
			Value: credentials.Value{
				AccessKeyID:     prof.AccessKey,
				SecretAccessKey: prof.SecretKey,
				ProviderName:    "osc",
			},
		}),
		CredentialsChainVerboseErrors: aws.Bool(true),
		EndpointResolver:              ServiceResolver(region),
		// FIXME: required for mTLS, use client from
		// HTTPClient:                    cfg.HTTPClient,
	}
	sess, err := session.NewSession(awsConfig)
	if err != nil {
		return nil, fmt.Errorf("unable to initialize AWS session: %w", err)
	}
	addHandlers(&sess.Handlers)
	return sess, nil
}

func addHandlers(h *request.Handlers) {
	h.Build.PushFrontNamed(request.NamedHandler{
		Name: "cloud-provider-osc/user-agent",
		Fn:   request.MakeAddToUserAgentHandler("osc-cloud-controller-manager", utils.GetVersion()),
	})

	// h.Sign.PushFrontNamed(request.NamedHandler{
	// 	Name: "k8s/logger",
	// 	Fn:   awsHandlerLogger,
	// })

	h.Send.PushBackNamed(request.NamedHandler{
		Name: "k8s/api-log-request",
		Fn:   awsLogRequestLogger,
	})

	h.CompleteAttempt.PushFrontNamed(request.NamedHandler{
		Name: "k8s/api-log-response",
		Fn:   awsLogResponseHandlerLogger,
	})
}

func AWSErrorCode(err error) string {
	if awsError, ok := err.(awserr.Error); ok {
		return awsError.Code()
	}
	return ""
}
