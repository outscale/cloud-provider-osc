/*
SPDX-FileCopyrightText: 2025 Outscale SAS <opensource@outscale.com>

SPDX-License-Identifier: BSD-3-Clause
*/
package oapi

import (
	"context"
	"errors"
	"strings"

	osc "github.com/outscale/osc-sdk-go/v3/pkg/osc"
)

func GetPublicIp(ctx context.Context, idOrIP string, c osc.ClientInterface) (ip string, err error) {
	if !strings.HasPrefix(idOrIP, "ipalloc-") {
		return idOrIP, nil
	}
	req := osc.ReadPublicIpsRequest{
		Filters: &osc.FiltersPublicIp{PublicIpIds: &[]string{idOrIP}},
	}
	resp, err := c.ReadPublicIps(ctx, req)
	switch {
	case err != nil:
		return "", err
	case len(*resp.PublicIps) == 0:
		return "", errors.New("not found")
	default:
		return (*resp.PublicIps)[0].PublicIp, nil
	}
}
