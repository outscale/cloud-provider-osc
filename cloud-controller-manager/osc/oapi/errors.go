package oapi

import (
	"encoding/json"
	"fmt"

	"github.com/outscale/osc-sdk-go/v2"
)

// OAPIError is a wrapper for OAPI errors, with better error messages.
type OAPIError struct {
	errors []osc.Errors
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
