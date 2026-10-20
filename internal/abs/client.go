// Package abs is a minimal client for the Audiobookshelf HTTP API.
//
// It only speaks HTTP: one method per endpoint, returning the API's own wire
// types. It knows nothing about MCP, formatting, or how results are used.
package abs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client is an HTTP client for the Audiobookshelf API.
type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// NewClient creates a client for the Audiobookshelf instance at baseURL,
// authenticating with the given API token.
func NewClient(baseURL, token string) *Client {
	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		token:      token,
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

// pathParam escapes a caller-supplied ID for safe use in URL paths.
// Prevents path traversal (e.g. "../../admin") by percent-encoding slashes.
func pathParam(id string) string {
	return url.PathEscape(id)
}

// do sends a request with an optional JSON body and decodes a JSON response
// into out when out is non-nil. Any status other than 200 is an error.
func (c *Client) do(ctx context.Context, method, path string, query url.Values, body, out any) error {
	u, err := url.Parse(c.baseURL + path)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	if query != nil {
		u.RawQuery = query.Encode()
	}

	reqBody := io.Reader(http.NoBody)
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encoding request: %w", err)
		}
		reqBody = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, u.String(), reqBody)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ABS API %d: %s", resp.StatusCode, string(respBody))
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(respBody, out)
}

// get makes a GET request and decodes the JSON response into out.
func (c *Client) get(ctx context.Context, path string, query url.Values, out any) error {
	return c.do(ctx, http.MethodGet, path, query, nil, out)
}
