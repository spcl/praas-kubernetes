package networking

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	"knative.dev/pkg/logging"
)

const (
	jsonContentType      = "application/json"
	plainTextContentType = "text/plain"
)

// HTTPClient facilitates sending http requests and parsing the results
type HTTPClient struct {
	httpClient *http.Client
}

func newHTTPClient(httpClient *http.Client) *HTTPClient {
	return &HTTPClient{
		httpClient: httpClient,
	}
}

// keepAliveTransport is a http.Transport with the default settings, but with
// keepAlive upped to allow 1000 connections.
var keepAliveTransport = func() *http.Transport {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.DisableKeepAlives = false // default, but for clarity.
	t.MaxIdleConns = 1000
	return t
}()

// client is a normal http client with HTTP Keep-Alive enabled.
// This client is used in the direct pod scraping where we want
// to take advantage of HTTP Keep-Alive to avoid connection creation overhead
// between scrapes of the same pod.
var client = &http.Client{
	Timeout:   0,
	Transport: keepAliveTransport,
}

type httpKey struct{}

func WithHTTPClient(ctx context.Context) context.Context {
	return context.WithValue(ctx, httpKey{}, newHTTPClient(client))
}

func GetHTTPClient(ctx context.Context) *HTTPClient {
	untyped := ctx.Value(httpKey{})
	if untyped == nil {
		logging.FromContext(ctx).Panic("Unable to fetch HTTP client from context.")
	}
	return untyped.(*HTTPClient)
}

func (c *HTTPClient) Do(ctx context.Context, path string, target any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, path, nil)
	if err != nil {
		return err
	}

	req.Header.Add("Accept", jsonContentType)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("GET request for URL %q returned HTTP status %v", req.URL.String(), resp.StatusCode)
	}
	// Return if we don't expect an answer
	if target == nil {
		return nil
	}

	// Get answer object
	contentType := strings.Split(resp.Header.Get("Content-Type"), ";")[0]
	if contentType != jsonContentType && contentType != plainTextContentType {
		return fmt.Errorf("unsupported Content-Type: %s", resp.Header.Get("Content-Type"))
	}
	return objFromJSON(resp.Body, target)
}

func objFromJSON(body io.Reader, target any) error {
	b := pool.Get().(*bytes.Buffer)
	b.Reset()
	defer pool.Put(b)
	_, err := b.ReadFrom(body)
	if err != nil {
		return fmt.Errorf("reading body failed: %w", err)
	}
	err = json.Unmarshal(b.Bytes(), target)
	if err != nil {
		return fmt.Errorf("unmarshalling failed: %w (%s)", err, string(b.Bytes()))
	}
	return nil
}

var pool = sync.Pool{
	New: func() interface{} {
		return new(bytes.Buffer)
	},
}