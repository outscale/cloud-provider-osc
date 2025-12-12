/*
SPDX-FileCopyrightText: 2025 Outscale SAS <opensource@outscale.com>

SPDX-License-Identifier: BSD-3-Clause
*/
package oapi

import (
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/outscale/cloud-provider-osc/cloud-controller-manager/utils"
	"github.com/outscale/osc-sdk-go/v2"
)

// NewSession create a new AWS client session, using OSC credentials.
func NewSession(region string, config *osc.ConfigEnv) (*session.Session, error) {
	cfg, err := config.Configuration()
	if err != nil {
		return nil, fmt.Errorf("configuration: %w", err)
	}
	awsConfig := &aws.Config{
		Region: aws.String(region),
		Credentials: credentials.NewCredentials(&credentials.StaticProvider{
			Value: credentials.Value{
				AccessKeyID:     *config.AccessKey,
				SecretAccessKey: *config.SecretKey,
				ProviderName:    "osc",
			},
		}),
		CredentialsChainVerboseErrors: aws.Bool(true),
		EndpointResolver:              ServiceResolver(region),
		HTTPClient:                    cfg.HTTPClient,
	}
	sess, err := session.NewSession(awsConfig)
	if err != nil {
		return nil, fmt.Errorf("unable to initialize NewSession session: %w", err)
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
