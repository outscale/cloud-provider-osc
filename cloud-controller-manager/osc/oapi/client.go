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
	oapi, err := NewOscClient(configEnv)
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
