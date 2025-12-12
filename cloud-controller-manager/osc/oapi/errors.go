/*
SPDX-FileCopyrightText: 2025 Outscale SAS <opensource@outscale.com>

SPDX-License-Identifier: BSD-3-Clause
*/
package oapi

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/outscale/osc-sdk-go/v2"
)

// OAPIError is a wrapper for OAPI errors, with better error messages.
type OAPIError struct {
	errors []osc.Errors
}

func NewOAPIError(errors ...osc.Errors) OAPIError {
	return OAPIError{errors: errors}
}

func (err OAPIError) Error() string {
	if len(err.errors) == 0 {
		return "unknown error"
	}
	oe := err.errors[0]
	str := oe.GetCode() + "/" + oe.GetType()
	details := oe.GetDetails()
	if details != "" {
		str += " (" + details + ")"
	}
	return str
}

func extractOAPIError(err error, body []byte) error {
	var oerr osc.ErrorResponse
	jerr := json.Unmarshal(body, &oerr)
	if jerr == nil && len(*oerr.Errors) > 0 {
		return OAPIError{errors: *oerr.Errors}
	}
	return fmt.Errorf("http error: %w", err)
}

func ErrorCode(err error) string {
	var oerr OAPIError
	if errors.As(err, &oerr) {
		for _, oerr := range oerr.errors {
			return oerr.GetCode()
		}
	}
	return ""
}
