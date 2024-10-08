package metrics

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
)

const (
	JSONContentType      = "application/json"
	PlainTextContentType = "text/plain"
)

type httpScrapeClient struct {
	httpClient *http.Client
}

func newHTTPScrapeClient(httpClient *http.Client) *httpScrapeClient {
	return &httpScrapeClient{
		httpClient: httpClient,
	}
}

func (c *httpScrapeClient) Do(req *http.Request) (StatEntry, error) {
	req.Header.Add("Accept", JSONContentType)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return emptyStatEntry, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return emptyStatEntry, fmt.Errorf(
			"GET request for URL %q returned HTTP status %v",
			req.URL.String(),
			resp.StatusCode,
		)
	}
	if resp.Header.Get("Content-Type") != JSONContentType && resp.Header.Get("Content-Type") != PlainTextContentType {
		return emptyStatEntry, fmt.Errorf("unsupported Content-Type: %s", resp.Header.Get("Content-Type"))
	}
	return statFromJSON(resp.Body)
}

func statFromJSON(body io.Reader) (StatEntry, error) {
	var stat StatEntry
	b := pool.Get().(*bytes.Buffer)
	b.Reset()
	defer pool.Put(b)
	_, err := b.ReadFrom(body)
	if err != nil {
		return emptyStatEntry, fmt.Errorf("reading body failed: %w", err)
	}
	err = json.Unmarshal(b.Bytes(), &stat)
	if err != nil {
		return emptyStatEntry, fmt.Errorf("unmarshalling failed: %w (%s)", err, string(b.Bytes()))
	}
	return stat, nil
}

var pool = sync.Pool{
	New: func() interface{} {
		return new(bytes.Buffer)
	},
}
