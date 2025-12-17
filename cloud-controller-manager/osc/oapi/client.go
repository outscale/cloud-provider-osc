/*
SPDX-FileCopyrightText: 2025 Outscale SAS <opensource@outscale.com>

SPDX-License-Identifier: BSD-3-Clause
*/
package oapi

import (
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go/service/elb"
)

// Clienter is the interface for Client.
type Clienter interface {
	OAPI() OAPI
	LBU() LBU
}

// Client wraps both OAPI ans AWS clients.
type Client struct {
	elb  *elb.ELB
	oapi *OscClient
}

// NewClient builds a Client.
func NewClient(region string) (*Client, error) {
	configEnv, err := LoadConfig()
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	if configEnv.AccessKey == nil || configEnv.SecretKey == nil {
		return nil, errors.New(("OSC_ACCESS_KEY/OSC_SECRET_KEY are required"))
	}
	sess, err := NewSession(region, configEnv)
	if err != nil {
		return nil, fmt.Errorf("new client: %w", err)
	}
	aws := elb.New(sess)
	oapi, err := NewOscClient(region, configEnv)
	if err != nil {
		return nil, fmt.Errorf("new client: %w", err)
	}
	return &Client{
		elb:  aws,
		oapi: oapi,
	}, nil
}

// OAPI returns an OAPI client.
func (c *Client) OAPI() OAPI {
	return c.oapi
}

// LBU returns an LBU client.
func (c *Client) LBU() LBU {
	return c.elb
}
