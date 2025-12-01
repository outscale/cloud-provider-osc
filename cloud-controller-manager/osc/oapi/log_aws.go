/*
Copyright 2015 The Kubernetes Authors.

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
	"fmt"
	"unicode"

	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/outscale/cloud-provider-osc/cloud-controller-manager/utils"
	"k8s.io/klog/v2"
)

// func awsHandlerLogger(req *request.Request) {
// 	_, call := awsServiceAndName(req)
// 	logger := klog.FromContext(req.HTTPRequest.Context())
// 	if logger.V(5).Enabled() {
// 		logger.Info("LBU request: "+cleanAws(req.Params), "LBU", call)
// 	}
// }

func awsLogRequestLogger(req *request.Request) {
	_, call := awsServiceAndName(req)
	logger := klog.FromContext(req.HTTPRequest.Context())
	if logger.V(5).Enabled() {
		logger.Info("LBU request: "+cleanAws(req.Params), "LBU", call)
	}
}

func awsLogResponseHandlerLogger(req *request.Request) {
	_, call := awsServiceAndName(req)
	logger := klog.FromContext(req.HTTPRequest.Context())
	switch {
	case req.Error != nil && req.HTTPResponse == nil:
		logger.V(3).Error(req.Error, "LBU error", "LBU", call)
	case req.HTTPResponse == nil:
	case req.HTTPResponse.StatusCode > 299:
		logger.V(3).Info("LBU error response: "+cleanAws(req.Data), "LBU", call, "http_status", req.HTTPResponse.Status)
	case logger.V(5).Enabled(): // no error
		logger.Info("LBU response: "+cleanAws(req.Data), "LBU", call)
	}
}

func awsServiceAndName(req *request.Request) (string, string) {
	service := req.ClientInfo.ServiceName

	name := "?"
	if req.Operation != nil {
		name = req.Operation.Name
	}
	return service, name
}

// cleanAws cleans a aws log
// - merges all multiple unicode spaces (\n, \r, \t, ...) into a single ascii space.
// - removes all spaces after unicode punctuations [ ] { } : , etc.
// - removes all "
func cleanAws(i any) string {
	str := fmt.Sprintf("%v", i)
	var prev rune
	return string(utils.Map([]rune(str), func(r rune) (rune, bool) {
		defer func() {
			prev = r
		}()
		switch {
		case unicode.IsSpace(r) && (unicode.IsPunct(prev) || unicode.IsSpace(prev)):
			return ' ', false
		case unicode.IsSpace(r):
			return ' ', true
		case r == '"':
			return ' ', false
		default:
			return r, true
		}
	}))
}
