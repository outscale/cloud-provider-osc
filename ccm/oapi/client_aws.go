/*
SPDX-FileCopyrightText: 2025 Outscale SAS <opensource@outscale.com>

SPDX-License-Identifier: BSD-3-Clause
*/
package oapi

import (
	"fmt"

	"github.com/aws/aws-sdk-go/aws"             //nolint:staticcheck
	"github.com/aws/aws-sdk-go/aws/awserr"      //nolint:staticcheck
	"github.com/aws/aws-sdk-go/aws/credentials" //nolint:staticcheck
	"github.com/aws/aws-sdk-go/aws/request"     //nolint:staticcheck
	"github.com/aws/aws-sdk-go/aws/session"     //nolint:staticcheck
	"github.com/outscale/cloud-provider-osc/ccm/utils"
	"github.com/outscale/osc-sdk-go/v3/pkg/profile"
)

// NewSession create a new AWS client session, using OSC credentials.
func NewSession(prof *profile.Profile) (*session.Session, error) {
	awsConfig := &aws.Config{
		Region: aws.String(prof.Region),
		Credentials: credentials.NewCredentials(&credentials.StaticProvider{
			Value: credentials.Value{
				AccessKeyID:     prof.AccessKey,
				SecretAccessKey: prof.SecretKey,
				ProviderName:    "osc",
			},
		}),
		CredentialsChainVerboseErrors: aws.Bool(true),
		EndpointResolver:              ServiceResolver(prof.Region),
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
