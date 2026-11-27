// Package gemini is a minimal client for Google's Gemini image API.
//
// It only speaks HTTP: one method per call, returning plain Go types. It knows
// nothing about MCP, Audiobookshelf, or how a made image is used. The API key
// travels only in a request header and never appears in an error.
package gemini

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

// DefaultBaseURL is the address of Google's Gemini API.
const DefaultBaseURL = "https://generativelanguage.googleapis.com"

// Model is the ID of an image model, as the Gemini API names it in a path.
type Model string

// The image models a request can use.
const (
	// ModelPro is Nano Banana Pro, which makes 2K images.
	ModelPro Model = "gemini-3-pro-image"
	// ModelStandard is Nano Banana 2.1, which makes 1K images.
	ModelStandard Model = "gemini-nano-banana-2.1"
)

// ErrNoModel is the error for a [Request] that names no model. Nothing is sent.
var ErrNoModel = errors.New("no image model given")

const (
	// checkPath is the path of a model resource, which a GET describes: the
	// Pro model, for the key check.
	checkPath = "/v1/models/" + string(ModelPro)
	// sizeStandard is the size class asked of every model but the Pro one: 1K
	// is 1024×1024 at 1:1.
	sizeStandard = "1K"
	// sizePro is the size class asked of the Pro model: 2K is 2048×2048 at
	// 1:1, at the same price as 1K, where the standard model charges more for
	// 2K.
	sizePro = "2K"
	// maxResponse is the most of a successful response that is read.
	maxResponse = 32 << 20
	// maxErrorRead is how much of an error response is read.
	maxErrorRead = 64 << 10
	// maxErrorText is how many characters of an error message are kept.
	maxErrorText = 300
	// reasonNoImage is the [BlockedError] reason for an answer without an image
	// that Google gave no reason for.
	reasonNoImage = "NO_IMAGE"
)

// Client is an HTTP client for the Gemini image API.
type Client struct {
	baseURL    string
	key        string
	httpClient *http.Client
}

// NewClient creates a client for the Gemini API at baseURL, authenticating
// with the given API key. An image takes a while to make, so a call may last
// up to two minutes. The client follows no redirect: the key is meant for
// Google's host alone, so a 3xx answer is a [*StatusError] and the place it
// points to is never asked.
func NewClient(baseURL, key string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		key:     key,
		httpClient: &http.Client{
			Timeout: 2 * time.Minute,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

// Image is an image with its MIME type, such as "image/png".
type Image struct {
	// Data is the encoded image, not base64.
	Data     []byte
	MIMEType string
}

// Request is what to make an image from.
type Request struct {
	// Model is the image model to use. It is required: an empty one is an
	// [ErrNoModel] error, and nothing is sent.
	Model Model
	// Prompt says what to draw.
	Prompt string
	// Images are the pictures the prompt refers to, such as the current cover.
	Images []Image
	// AspectRatio is the shape of the result, such as "2:3". Empty leaves it
	// to the model, which matches the input image.
	AspectRatio string
}

// Usage is how many tokens a call used, which is what Google bills.
type Usage struct {
	PromptTokens  int
	OutputTokens  int
	ThoughtTokens int
}

// Result is a made image and what making it used.
type Result struct {
	Image Image
	Usage Usage
}

// StatusError is the error for an API response with a status other than 200.
// Callers match it with [errors.As] to tell, say, a spending cap from a bad key.
type StatusError struct {
	// Code is the HTTP status code.
	Code int
	// Status is Google's name for the error, such as "RESOURCE_EXHAUSTED".
	Status string
	// Reason is Google's machine-readable cause, such as "API_KEY_INVALID".
	// It is empty when the error has none.
	Reason string
	// Message is the error's text, with the key masked and cut short.
	Message string
}

// Error implements the error interface.
func (e *StatusError) Error() string {
	if e.Status == "" {
		return fmt.Sprintf("Gemini API %d: %s", e.Code, e.Message)
	}
	return fmt.Sprintf("Gemini API %d %s: %s", e.Code, e.Status, e.Message)
}

// StopsBatch reports whether the error says that every further request would
// fail too: a bad key, a missing permission, empty credit, or a spending limit,
// quota or rate limit.
func (e *StatusError) StopsBatch() bool {
	switch e.Code {
	case http.StatusPaymentRequired, http.StatusTooManyRequests,
		http.StatusUnauthorized, http.StatusForbidden:
		return true
	}
	return e.Reason == "API_KEY_INVALID"
}

// BlockedError is the error for a refusal: Google answered with status 200 but
// made no image. The result that comes with it still holds the usage, since a
// refusal is billed too.
type BlockedError struct {
	// Reason is why there is no image: a block reason such as
	// "PROHIBITED_CONTENT", a finish reason such as "IMAGE_SAFETY", or
	// "NO_IMAGE" when the model answered with text only.
	Reason string
}

// Error implements the error interface.
func (e *BlockedError) Error() string {
	return "Gemini made no image: " + e.Reason
}

// GenerateImage makes an image with the model r names from a prompt and
// optional input images; without a model it fails with [ErrNoModel] before
// anything is sent. The image is 2K for [ModelPro] and 1K for any other model.
// An answer without an image is a [*BlockedError], and the returned Usage is
// filled even then. Any status other than 200 is a [*StatusError].
func (c *Client) GenerateImage(ctx context.Context, r Request) (Result, error) {
	if r.Model == "" {
		return Result{}, ErrNoModel
	}
	body, err := json.Marshal(newGenerateRequest(r))
	if err != nil {
		return Result{}, fmt.Errorf("encoding request: %w", err)
	}
	resp, err := c.send(ctx, http.MethodPost, "/v1/models/"+string(r.Model)+":generateContent", body)
	if err != nil {
		return Result{}, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return Result{}, c.statusError(resp)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponse+1))
	if err != nil {
		return Result{}, fmt.Errorf("reading Gemini response: %w", err)
	}
	if len(raw) > maxResponse {
		return Result{}, fmt.Errorf("response from Gemini is larger than %d MiB", maxResponse>>20)
	}
	var out generateResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return Result{}, fmt.Errorf("decoding Gemini response: %w", err)
	}
	return out.result()
}

// CheckKey checks that the API key works, by reading the description of
// [ModelPro], without making an image and without a charge. The check can't
// see whether the key's project has credit. A key that does not work is a
// [*StatusError].
func (c *Client) CheckKey(ctx context.Context) error {
	resp, err := c.send(ctx, http.MethodGet, checkPath, nil)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return c.statusError(resp)
	}
	return nil
}

// send sends a request with an optional JSON body and returns the response.
// The caller closes the response body and checks the status. A transport
// error is returned without the [*url.Error] around it, which would print the
// URL, and so it still matches [context.Canceled] and the like.
func (c *Client) send(ctx context.Context, method, path string, body []byte) (*http.Response, error) {
	reqBody := io.Reader(http.NoBody)
	if body != nil {
		reqBody = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reqBody)
	if err != nil {
		return nil, fmt.Errorf("invalid Gemini request: %w", unwrapURLError(err))
	}
	req.Header.Set("x-goog-api-key", c.key)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		//nolint:staticcheck // ST1005: Gemini is a proper noun.
		return nil, fmt.Errorf("Gemini request failed: %w", unwrapURLError(err))
	}
	return resp, nil
}

// unwrapURLError returns the error inside a [*url.Error].
func unwrapURLError(err error) error {
	if ue, ok := errors.AsType[*url.Error](err); ok {
		return ue.Err
	}
	return err
}

// errorBody is the body of an error response.
type errorBody struct {
	Error struct {
		Message string `json:"message"`
		Status  string `json:"status"`
		Details []struct {
			Reason string `json:"reason"`
		} `json:"details"`
	} `json:"error"`
}

// reason returns the first reason among the error's details.
func (b *errorBody) reason() string {
	for _, d := range b.Error.Details {
		if d.Reason != "" {
			return d.Reason
		}
	}
	return ""
}

// statusError reads a response with a status other than 200 into a
// [*StatusError]. A body that is not Google's error JSON becomes the message
// itself. The key, should the response echo it, becomes "***" first, so the
// cut cannot leave part of it behind.
func (c *Client) statusError(resp *http.Response) error {
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxErrorRead))
	if err != nil {
		return fmt.Errorf("reading Gemini response: %w", err)
	}
	// A body that is not JSON leaves the fields empty.
	var body errorBody
	_ = json.Unmarshal(raw, &body)

	message := cmp.Or(body.Error.Message, strings.TrimSpace(string(raw)), http.StatusText(resp.StatusCode))
	return &StatusError{
		Code:    resp.StatusCode,
		Status:  c.tidy(body.Error.Status),
		Reason:  c.tidy(body.reason()),
		Message: c.tidy(message),
	}
}

// tidy masks the key in text, then cuts the text to maxErrorText characters.
func (c *Client) tidy(text string) string {
	if c.key != "" {
		text = strings.ReplaceAll(text, c.key, "***")
	}
	if utf8.RuneCountInString(text) > maxErrorText {
		text = string([]rune(text)[:maxErrorText]) + "…"
	}
	return text
}

// The wire types use Google's own JSON names, which differ between a request
// (inline_data) and a response (inlineData). A byte slice is base64 in JSON.

// generateRequest is the body of a generateContent call.
type generateRequest struct {
	Contents         []content        `json:"contents"`
	GenerationConfig generationConfig `json:"generationConfig"`
}

// content is one turn of the conversation; its parts are textPart and
// inlineDataPart values.
type content struct {
	Parts []any `json:"parts"`
}

// textPart is a part that holds text.
type textPart struct {
	Text string `json:"text"`
}

// inlineDataPart is a part that holds an input image.
type inlineDataPart struct {
	InlineData requestBlob `json:"inline_data"`
}

// requestBlob is an input image.
type requestBlob struct {
	MIMEType string `json:"mime_type"`
	Data     []byte `json:"data"`
}

// generationConfig asks for an image and not for text.
type generationConfig struct {
	ResponseModalities []string    `json:"responseModalities"`
	ImageConfig        imageConfig `json:"imageConfig"`
}

// imageConfig is the shape and size of the image.
type imageConfig struct {
	AspectRatio string `json:"aspectRatio,omitempty"`
	ImageSize   string `json:"imageSize"`
}

// imageSize is the size class asked for with model: 2K for [ModelPro], 1K for
// any other.
func imageSize(model Model) string {
	if model == ModelPro {
		return sizePro
	}
	return sizeStandard
}

// newGenerateRequest builds the body for r: the prompt first, then the images.
func newGenerateRequest(r Request) generateRequest {
	parts := make([]any, 0, 1+len(r.Images))
	parts = append(parts, textPart{Text: r.Prompt})
	for _, img := range r.Images {
		parts = append(parts, inlineDataPart{InlineData: requestBlob{MIMEType: img.MIMEType, Data: img.Data}})
	}
	return generateRequest{
		Contents: []content{{Parts: parts}},
		GenerationConfig: generationConfig{
			ResponseModalities: []string{"IMAGE"},
			ImageConfig:        imageConfig{AspectRatio: r.AspectRatio, ImageSize: imageSize(r.Model)},
		},
	}
}

// generateResponse is the body of a successful generateContent call.
type generateResponse struct {
	Candidates     []candidate `json:"candidates"`
	PromptFeedback struct {
		BlockReason string `json:"blockReason"`
	} `json:"promptFeedback"`
	UsageMetadata struct {
		PromptTokenCount     int `json:"promptTokenCount"`
		CandidatesTokenCount int `json:"candidatesTokenCount"`
		ThoughtsTokenCount   int `json:"thoughtsTokenCount"`
	} `json:"usageMetadata"`
}

// candidate is one answer of the model.
type candidate struct {
	Content struct {
		Parts []responsePart `json:"parts"`
	} `json:"content"`
	FinishReason string `json:"finishReason"`
}

// responsePart is one part of an answer: text, an image, or either of them as
// a thought, which is the model thinking aloud and not part of the result.
type responsePart struct {
	Thought    bool          `json:"thought"`
	InlineData *responseBlob `json:"inlineData"`
}

// responseBlob is a made image.
type responseBlob struct {
	MIMEType string `json:"mimeType"`
	Data     []byte `json:"data"`
}

// result turns the response into the made image, or into a [*BlockedError]
// when there is none. The usage is filled either way.
func (g *generateResponse) result() (Result, error) {
	res := Result{Usage: Usage{
		PromptTokens:  g.UsageMetadata.PromptTokenCount,
		OutputTokens:  g.UsageMetadata.CandidatesTokenCount,
		ThoughtTokens: g.UsageMetadata.ThoughtsTokenCount,
	}}
	if reason := g.PromptFeedback.BlockReason; reason != "" {
		return res, &BlockedError{Reason: reason}
	}
	if len(g.Candidates) == 0 {
		return res, &BlockedError{Reason: reasonNoImage}
	}
	first := g.Candidates[0]
	if reason := first.FinishReason; reason != "" && reason != "STOP" {
		return res, &BlockedError{Reason: reason}
	}
	for _, p := range slices.Backward(first.Content.Parts) {
		if p.Thought || p.InlineData == nil || len(p.InlineData.Data) == 0 {
			continue
		}
		res.Image = Image{Data: p.InlineData.Data, MIMEType: p.InlineData.MIMEType}
		return res, nil
	}
	return res, &BlockedError{Reason: reasonNoImage}
}
