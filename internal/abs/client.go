// Package abs is a minimal client for the Audiobookshelf HTTP API.
//
// It only speaks HTTP: one method per endpoint, returning the API's own wire
// types. It knows nothing about MCP, formatting, or how results are used.
package abs

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"mime"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
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

// StatusError is the error for an API response with a status other than 200.
// Callers match it with [errors.As] to tell, say, a 404 from a 403.
type StatusError struct {
	StatusCode int
	Body       string
}

// Error implements the error interface.
func (e *StatusError) Error() string {
	return fmt.Sprintf("ABS API %d: %s", e.StatusCode, e.Body)
}

const (
	// maxErrorRead is how much of an error response is read.
	maxErrorRead = 64 << 10
	// maxErrorText is how many characters of it a [StatusError] keeps.
	maxErrorText = 300
)

// statusError reads a response with a status other than 200 into a
// [*StatusError] with a short Body, since the text ends up in the tool
// error the model reads. A proxy in front of Audiobookshelf answers with
// a whole HTML page when the server is down; of an HTML page only its title
// is kept, and any text longer than maxErrorText characters is cut. The
// client's token, should the response echo it, becomes "***" first, so the
// cut cannot leave part of it behind.
func (c *Client) statusError(resp *http.Response) error {
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxErrorRead))
	if err != nil {
		return fmt.Errorf("reading response: %w", err)
	}
	text := string(raw)
	if c.token != "" {
		text = strings.ReplaceAll(text, c.token, "***")
	}
	mediaType, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if mediaType == "text/html" || isHTMLPage(text) {
		text = htmlTitle(text)
	}
	if utf8.RuneCountInString(text) > maxErrorText {
		text = string([]rune(text)[:maxErrorText]) + "…"
	}
	return &StatusError{StatusCode: resp.StatusCode, Body: text}
}

// isHTMLPage reports whether text starts like an HTML document.
func isHTMLPage(text string) bool {
	start := strings.ToLower(strings.TrimSpace(text))
	return strings.HasPrefix(start, "<!doctype html") || strings.HasPrefix(start, "<html")
}

// titleElement matches a page's title element, in any letter case.
var titleElement = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title`)

// htmlTitle returns the text of a page's title element, unescaped and with
// its whitespace collapsed.
func htmlTitle(page string) string {
	var title string
	if m := titleElement.FindStringSubmatch(page); m != nil {
		title = strings.Join(strings.Fields(html.UnescapeString(m[1])), " ")
	}
	return cmp.Or(title, "an HTML page without a title")
}

// formBody is a request body that is already encoded as multipart/form-data;
// contentType carries its boundary.
type formBody struct {
	contentType string
	data        []byte
}

// send sends a request and returns the response. A nil body sends none, a
// [formBody] is sent as it is, and anything else is encoded as JSON.
// The caller closes the response body and checks the status.
func (c *Client) send(ctx context.Context, method, path string, query url.Values, body any) (*http.Response, error) {
	u, err := url.Parse(c.baseURL + path)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}
	if query != nil {
		u.RawQuery = query.Encode()
	}

	reqBody := io.Reader(http.NoBody)
	var contentType string
	switch b := body.(type) {
	case nil:
	case formBody:
		reqBody = bytes.NewReader(b.data)
		contentType = b.contentType
	default:
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("encoding request: %w", err)
		}
		reqBody = bytes.NewReader(encoded)
		contentType = "application/json"
	}

	req, err := http.NewRequestWithContext(ctx, method, u.String(), reqBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	return resp, nil
}

// do sends a request with an optional body (see [Client.send]) and decodes a JSON response
// into out when out is non-nil. Any status other than 200 is a [*StatusError].
func (c *Client) do(ctx context.Context, method, path string, query url.Values, body, out any) error {
	resp, err := c.send(ctx, method, path, query, body)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return c.statusError(resp)
	}
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading response: %w", err)
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

// getImage makes a GET request for an image and reads at most limit bytes of
// it; a longer body is an error, so limit should be a whole number of MiB. The
// MIME type comes from the Content-Type header and is empty when there is
// none. Any status other than 200 is a [*StatusError].
func (c *Client) getImage(ctx context.Context, path string, query url.Values, limit int64) (Image, error) {
	resp, err := c.send(ctx, http.MethodGet, path, query, nil)
	if err != nil {
		return Image{}, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return Image{}, c.statusError(resp)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return Image{}, fmt.Errorf("reading image: %w", err)
	}
	if int64(len(data)) > limit {
		return Image{}, fmt.Errorf("image is larger than %d MiB", limit>>20)
	}
	mimeType, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	return Image{Data: data, MIMEType: mimeType}, nil
}
