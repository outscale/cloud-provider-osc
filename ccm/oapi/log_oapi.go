package oapi

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
	"time"

	"k8s.io/klog/v2"
)

const maxResponseLength = 500

func clean(buf []byte) string {
	return strings.ReplaceAll(string(buf), `"`, ``)
}

func truncatedBody(body string) string {
	str := []rune(body)
	if len(str) > maxResponseLength {
		return string(str[:maxResponseLength/2]) + " [truncated] " + string(str[len(str)-maxResponseLength/2:])
	}
	return string(str)
}

type OAPILogger struct{}

func callName(r *http.Request) string {
	return path.Base(r.URL.RawPath)
}

func requestBody(req *http.Request) ([]byte, error) {
	body, err := req.GetBody()
	if err != nil {
		return nil, err
	}
	buf, err := io.ReadAll(body)
	if err != nil {
		return nil, err
	}
	return buf, nil
}

func (OAPILogger) Request(ctx context.Context, req any)   {}
func (OAPILogger) Response(ctx context.Context, resp any) {}

func (l OAPILogger) RequestHttp(ctx context.Context, req *http.Request) {
	logger := klog.FromContext(ctx).WithCallDepth(1)
	if !logger.V(5).Enabled() {
		return
	}
	body, err := requestBody(req)
	if err != nil {
		l.Error(ctx, fmt.Errorf("log request: %w", err))
		return
	}
	logger.Info("OAPI request: "+clean(body), "OAPI", callName(req))
}

func responseBody(httpResp *http.Response) ([]byte, error) {
	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, err
	}
	httpResp.Body = io.NopCloser(bytes.NewBuffer(body))
	return body, nil
}

func (l OAPILogger) ResponseHttp(ctx context.Context, resp *http.Response, d time.Duration) {
	logger := klog.FromContext(ctx).WithCallDepth(1)
	call := callName(resp.Request)
	if resp.StatusCode < 300 && !logger.V(5).Enabled() {
		return
	}
	body, err := responseBody(resp)
	if err != nil {
		l.Error(ctx, fmt.Errorf("log response: %w", err))
		return
	}
	sbody := truncatedBody(clean(body))
	switch {
	case resp.StatusCode > 299:
		logger.V(3).Info("OAPI error response: "+sbody, "OAPI", call, "http_status", resp.Status, "duration", d)
	case logger.V(5).Enabled(): // no error
		logger.Info("OAPI response: "+sbody, "OAPI", call, "duration", d)
	}
}

func (OAPILogger) Error(ctx context.Context, err error) {
	logger := klog.FromContext(ctx).WithCallDepth(1)
	logger.V(3).Error(err, "OAPI error")
}
