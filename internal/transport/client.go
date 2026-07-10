package transport

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"time"
)

// HTTPClient interface to allow dependency injection
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

type RequestOptions struct {
	Method  string
	URL     string
	Headers map[string]string
	Body    []byte
	Timeout time.Duration
}

type Response struct {
	StatusCode int
	Status     string
	Headers    map[string]string
	Body       []byte
}

type TransportClient struct {
	httpClient HTTPClient
}

func NewTransportClient(client HTTPClient) *TransportClient {
	if client == nil {
		client = &http.Client{}
	}
	return &TransportClient{
		httpClient: client,
	}
}

func (tc *TransportClient) Execute(ctx context.Context, opts RequestOptions) (*Response, error) {
	var bodyReader io.Reader
	if opts.Body != nil {
		bodyReader = bytes.NewReader(opts.Body)
	}

	req, err := http.NewRequestWithContext(ctx, opts.Method, opts.URL, bodyReader)
	if err != nil {
		return nil, err
	}

	for k, v := range opts.Headers {
		req.Header.Set(k, v)
	}

	// Use custom client timeout if set, but in Go timeouts are usually set on the http.Client
	// or handled via context. With http.NewRequestWithContext, context deadline handles timeout.
	var cancel context.CancelFunc
	if opts.Timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, opts.Timeout)
		req = req.WithContext(ctx)
		defer cancel()
	}

	resp, err := tc.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	headers := make(map[string]string)
	for k, vv := range resp.Header {
		if len(vv) > 0 {
			headers[k] = vv[0]
		}
	}

	return &Response{
		StatusCode: resp.StatusCode,
		Status:     resp.Status,
		Headers:    headers,
		Body:       respBody,
	}, nil
}
