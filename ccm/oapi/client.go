/*
SPDX-FileCopyrightText: 2025 Outscale SAS <opensource@outscale.com>

SPDX-License-Identifier: BSD-3-Clause
*/
package oapi

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go/service/elb" //nolint:staticcheck
	"github.com/outscale/cloud-provider-osc/ccm/utils"
	"github.com/outscale/goutils/k8s/sdk"
	"github.com/outscale/osc-sdk-go/v3/pkg/osc"
)

// Clienter is the interface for Client.
type Clienter interface {
	OAPI() osc.ClientInterface
	LBU() LBU
}

// Client wraps both OAPI ans AWS clients.
type Client struct {
	elb  *elb.ELB
	oapi osc.ClientInterface
}

// NewClient builds a Client.
func NewClient(ctx context.Context, opts ...sdk.Options) (*Client, error) {
	ua := "osc-cloud-controller-manager/" + utils.GetVersion()
	prof, oapi, err := sdk.NewSDKClient(ctx, ua, opts...)
	if err != nil {
		return nil, fmt.Errorf("new client: %w", err)
	}
	sess, err := NewSession(prof)
	if err != nil {
		return nil, fmt.Errorf("new client: %w", err)
	}
	aws := elb.New(sess)
	return &Client{
		elb:  aws,
		oapi: oapi,
	}, nil
}

// OAPI returns an OAPI client.
func (c *Client) OAPI() osc.ClientInterface {
	return c.oapi
}

// LBU returns an LBU client.
func (c *Client) LBU() LBU {
	return c.elb
}
