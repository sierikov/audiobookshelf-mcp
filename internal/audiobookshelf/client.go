package audiobookshelf

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// sanitizePathParam escapes a user-supplied ID for safe use in URL paths.
// Prevents path traversal (e.g. "../../admin") by percent-encoding slashes.
func sanitizePathParam(id string) string {
	return url.PathEscape(id)
}

// ABSClient is an HTTP client for the Audiobookshelf API.
type ABSClient struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// NewClient creates a new Audiobookshelf API client.
func NewClient(baseURL, token string) *ABSClient {
	return &ABSClient{
		baseURL:    strings.TrimRight(baseURL, "/"),
		token:      token,
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

// get makes a GET request to the ABS API and returns the raw response body.
func (c *ABSClient) get(ctx context.Context, path string, params url.Values) ([]byte, error) {
	u, err := url.Parse(c.baseURL + path)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}
	if params != nil {
		u.RawQuery = params.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ABS API %d: %s", resp.StatusCode, string(body))
	}
	return body, nil
}

// getJSON makes a GET request and unmarshals the JSON response into out.
func (c *ABSClient) getJSON(ctx context.Context, path string, params url.Values, out any) error {
	body, err := c.get(ctx, path, params)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, out)
}
