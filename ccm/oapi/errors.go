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
