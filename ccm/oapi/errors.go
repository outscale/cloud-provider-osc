/*
SPDX-FileCopyrightText: 2025 Outscale SAS <opensource@outscale.com>

SPDX-License-Identifier: BSD-3-Clause
*/
package oapi

import "github.com/outscale/osc-sdk-go/v3/pkg/osc"

func ErrorCode(err error) string {
	if oerr := osc.AsErrorResponse(err); oerr != nil {
		for _, oe := range oerr.Errors {
			return oe.Code
		}
	}
	return ""
}
