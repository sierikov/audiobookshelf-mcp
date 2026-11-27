package gemini

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testKey = "k-123"
	// testCheckPath is what CheckKey reads; testProPath and testStandardPath
	// are the models' generateContent paths.
	testCheckPath    = "/v1/models/gemini-3-pro-image"
	testProPath      = "/v1/models/gemini-3-pro-image:generateContent"
	testStandardPath = "/v1/models/gemini-nano-banana-2.1:generateContent"
)

// newTestClient returns a client for a server running h.
func newTestClient(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return NewClient(srv.URL, testKey)
}

// replyJSON answers with the status and v as the JSON body.
func replyJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// inlinePart is a response part that holds an image.
func inlinePart(data []byte, mimeType string, thought bool) map[string]any {
	p := map[string]any{"inlineData": map[string]any{
		"mimeType": mimeType,
		"data":     base64.StdEncoding.EncodeToString(data),
	}}
	if thought {
		p["thought"] = true
	}
	return p
}

// candidateReply is a response with one candidate made of parts.
func candidateReply(finishReason string, parts ...any) map[string]any {
	return map[string]any{"candidates": []any{map[string]any{
		"content":      map[string]any{"parts": parts},
		"finishReason": finishReason,
	}}}
}

// errorReply is the body Google sends with an error status.
func errorReply(code int, status, message string, details ...any) map[string]any {
	e := map[string]any{"code": code, "message": message, "status": status}
	if details != nil {
		e["details"] = details
	}
	return map[string]any{"error": e}
}

func TestNewClient_TrimsTrailingSlashFromBaseURL(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		replyJSON(w, http.StatusOK, map[string]any{})
	}))
	t.Cleanup(srv.Close)

	require.NoError(t, NewClient(srv.URL+"/", testKey).CheckKey(t.Context()))

	assert.Equal(t, testCheckPath, gotPath)
}

func TestGenerateImage_SendsTheRequestGoogleDocuments(t *testing.T) {
	cover := []byte("cover-bytes")
	back := []byte("back-bytes")
	tests := []struct {
		name        string
		aspectRatio string
		imageConfig string
	}{
		{name: "with an aspect ratio", aspectRatio: "2:3", imageConfig: `{"aspectRatio":"2:3","imageSize":"2K"}`},
		{name: "without one", aspectRatio: "", imageConfig: `{"imageSize":"2K"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotMethod, gotPath, gotKey, gotQuery, gotType string
			var gotBody []byte
			c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				gotMethod, gotPath, gotQuery = r.Method, r.URL.Path, r.URL.RawQuery
				gotKey, gotType = r.Header.Get("x-goog-api-key"), r.Header.Get("Content-Type")
				gotBody, _ = io.ReadAll(r.Body)
				replyJSON(w, http.StatusOK, candidateReply("STOP", inlinePart([]byte("made"), "image/png", false)))
			})

			_, err := c.GenerateImage(t.Context(), Request{
				Model:  ModelPro,
				Prompt: "A quiet harbour at dusk",
				Images: []Image{
					{Data: cover, MIMEType: "image/jpeg"},
					{Data: back, MIMEType: "image/png"},
				},
				AspectRatio: tt.aspectRatio,
			})

			require.NoError(t, err)
			assert.Equal(t, http.MethodPost, gotMethod)
			assert.Equal(t, testProPath, gotPath)
			assert.Equal(t, "k-123", gotKey)
			assert.Empty(t, gotQuery, "the key must not travel in the URL")
			assert.Equal(t, "application/json", gotType)
			want := fmt.Sprintf(`{
				"contents": [{"parts": [
					{"text": "A quiet harbour at dusk"},
					{"inline_data": {"mime_type": "image/jpeg", "data": %q}},
					{"inline_data": {"mime_type": "image/png", "data": %q}}
				]}],
				"generationConfig": {"responseModalities": ["IMAGE"], "imageConfig": %s}
			}`, base64.StdEncoding.EncodeToString(cover), base64.StdEncoding.EncodeToString(back), tt.imageConfig)
			assert.JSONEq(t, want, string(gotBody))
		})
	}
}

func TestGenerateImage_PostsToTheModelAskedFor(t *testing.T) {
	tests := []struct {
		name      string
		model     Model
		wantPath  string
		wantImage string
	}{
		{name: "pro", model: ModelPro, wantPath: testProPath, wantImage: `"imageSize":"2K"`},
		{name: "standard", model: ModelStandard, wantPath: testStandardPath, wantImage: `"imageSize":"1K"`},
		{name: "a model that is neither", model: "gemini-next", wantPath: "/v1/models/gemini-next:generateContent", wantImage: `"imageSize":"1K"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotPath string
			var gotBody []byte
			c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				gotPath = r.URL.Path
				gotBody, _ = io.ReadAll(r.Body)
				replyJSON(w, http.StatusOK, candidateReply("STOP", inlinePart([]byte("made"), "image/png", false)))
			})

			_, err := c.GenerateImage(t.Context(), Request{Model: tt.model, Prompt: "p"})

			require.NoError(t, err)
			assert.Equal(t, tt.wantPath, gotPath)
			assert.Contains(t, string(gotBody), tt.wantImage)
		})
	}
}

func TestGenerateImage_WithoutAModelNothingIsSent(t *testing.T) {
	var calls atomic.Int32
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		replyJSON(w, http.StatusOK, candidateReply("STOP", inlinePart([]byte("made"), "image/png", false)))
	})

	_, err := c.GenerateImage(t.Context(), Request{Prompt: "p"})

	require.ErrorIs(t, err, ErrNoModel)
	assert.Zero(t, calls.Load(), "no request may reach the server")
}

func TestGenerateImage_ReturnsTheLastImageThatIsNotAThought(t *testing.T) {
	final := []byte("the-final-cover")
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		reply := candidateReply("STOP",
			inlinePart([]byte("a-thought"), "image/png", true),
			inlinePart([]byte("an-earlier-image"), "image/png", false),
			map[string]any{"text": "Here is the cover."},
			inlinePart(final, "image/webp", false),
		)
		reply["usageMetadata"] = map[string]any{
			"promptTokenCount":     40,
			"candidatesTokenCount": 1120,
			"thoughtsTokenCount":   300,
			"totalTokenCount":      1460,
		}
		replyJSON(w, http.StatusOK, reply)
	})

	res, err := c.GenerateImage(t.Context(), Request{Model: ModelPro, Prompt: "p"})

	require.NoError(t, err)
	assert.Equal(t, Image{Data: final, MIMEType: "image/webp"}, res.Image)
	assert.Equal(t, Usage{PromptTokens: 40, OutputTokens: 1120, ThoughtTokens: 300}, res.Usage)
}

func TestGenerateImage_RefusalsAreBlockedErrors(t *testing.T) {
	tests := []struct {
		name       string
		reply      map[string]any
		wantReason string
		wantUsage  Usage
	}{
		{
			name: "a blocked prompt",
			reply: map[string]any{
				"promptFeedback": map[string]any{"blockReason": "PROHIBITED_CONTENT"},
				"usageMetadata":  map[string]any{"promptTokenCount": 12},
			},
			wantReason: "PROHIBITED_CONTENT",
			wantUsage:  Usage{PromptTokens: 12},
		},
		{
			name: "an image stopped for safety",
			reply: func() map[string]any {
				r := candidateReply("IMAGE_SAFETY")
				r["usageMetadata"] = map[string]any{
					"promptTokenCount":   15,
					"thoughtsTokenCount": 250,
				}
				return r
			}(),
			wantReason: "IMAGE_SAFETY",
			wantUsage:  Usage{PromptTokens: 15, ThoughtTokens: 250},
		},
		{
			name: "a normal stop with only text",
			reply: func() map[string]any {
				r := candidateReply("STOP", map[string]any{"text": "I cannot draw that."})
				r["usageMetadata"] = map[string]any{
					"promptTokenCount":     20,
					"candidatesTokenCount": 9,
					"thoughtsTokenCount":   100,
				}
				return r
			}(),
			wantReason: "NO_IMAGE",
			wantUsage:  Usage{PromptTokens: 20, OutputTokens: 9, ThoughtTokens: 100},
		},
		{
			name: "a normal stop with only a thought image",
			reply: func() map[string]any {
				r := candidateReply("STOP", inlinePart([]byte("a-thought"), "image/png", true))
				r["usageMetadata"] = map[string]any{"promptTokenCount": 21}
				return r
			}(),
			wantReason: "NO_IMAGE",
			wantUsage:  Usage{PromptTokens: 21},
		},
		{
			name: "no candidates and no reason",
			reply: map[string]any{
				"usageMetadata": map[string]any{"promptTokenCount": 7},
			},
			wantReason: "NO_IMAGE",
			wantUsage:  Usage{PromptTokens: 7},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
				replyJSON(w, http.StatusOK, tt.reply)
			})

			res, err := c.GenerateImage(t.Context(), Request{Model: ModelPro, Prompt: "p"})

			var blocked *BlockedError
			require.ErrorAs(t, err, &blocked)
			assert.Equal(t, tt.wantReason, blocked.Reason)
			assert.Equal(t, tt.wantUsage, res.Usage, "Google bills a refusal too")
			assert.Empty(t, res.Image.Data)
		})
	}
}

func TestGenerateImage_ErrorBody(t *testing.T) {
	const spendCap = "Your project has exceeded its monthly spending cap."
	tests := []struct {
		name       string
		status     int
		body       any
		want       StatusError
		wantString string
		wantStops  bool
	}{
		{
			name:       "a spending cap",
			status:     http.StatusTooManyRequests,
			body:       errorReply(429, "RESOURCE_EXHAUSTED", spendCap),
			want:       StatusError{Code: 429, Status: "RESOURCE_EXHAUSTED", Message: spendCap},
			wantString: "Gemini API 429 RESOURCE_EXHAUSTED: " + spendCap,
			wantStops:  true,
		},
		{
			name:   "a bad key",
			status: http.StatusBadRequest,
			body: errorReply(400, "INVALID_ARGUMENT", "API key not valid. Please pass a valid API key.",
				map[string]any{"@type": "type.googleapis.com/google.rpc.ErrorInfo", "reason": "API_KEY_INVALID"}),
			want: StatusError{
				Code: 400, Status: "INVALID_ARGUMENT", Reason: "API_KEY_INVALID",
				Message: "API key not valid. Please pass a valid API key.",
			},
			wantStops: true,
		},
		{
			name:   "the reason is in a later detail",
			status: http.StatusBadRequest,
			body: errorReply(400, "INVALID_ARGUMENT", "bad",
				map[string]any{"@type": "type.googleapis.com/google.rpc.BadRequest"},
				map[string]any{"reason": "API_KEY_INVALID"}),
			want:      StatusError{Code: 400, Status: "INVALID_ARGUMENT", Reason: "API_KEY_INVALID", Message: "bad"},
			wantStops: true,
		},
		{
			name:      "empty prepaid credit",
			status:    http.StatusPaymentRequired,
			body:      errorReply(402, "FAILED_PRECONDITION", "Prepaid credit is empty."),
			want:      StatusError{Code: 402, Status: "FAILED_PRECONDITION", Message: "Prepaid credit is empty."},
			wantStops: true,
		},
		{
			name:      "unauthorized",
			status:    http.StatusUnauthorized,
			body:      errorReply(401, "UNAUTHENTICATED", "No credentials."),
			want:      StatusError{Code: 401, Status: "UNAUTHENTICATED", Message: "No credentials."},
			wantStops: true,
		},
		{
			name:      "permission denied",
			status:    http.StatusForbidden,
			body:      errorReply(403, "PERMISSION_DENIED", "Not allowed."),
			want:      StatusError{Code: 403, Status: "PERMISSION_DENIED", Message: "Not allowed."},
			wantStops: true,
		},
		{
			name:       "a server error",
			status:     http.StatusInternalServerError,
			body:       errorReply(500, "INTERNAL", "Internal error encountered."),
			want:       StatusError{Code: 500, Status: "INTERNAL", Message: "Internal error encountered."},
			wantString: "Gemini API 500 INTERNAL: Internal error encountered.",
		},
		{
			name:   "a bad request that is not about the key",
			status: http.StatusBadRequest,
			body:   errorReply(400, "INVALID_ARGUMENT", "Unsupported aspect ratio."),
			want:   StatusError{Code: 400, Status: "INVALID_ARGUMENT", Message: "Unsupported aspect ratio."},
		},
		{
			name:       "a body that is not JSON",
			status:     http.StatusBadGateway,
			body:       "Bad Gateway\n",
			want:       StatusError{Code: 502, Message: "Bad Gateway"},
			wantString: "Gemini API 502: Bad Gateway",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
				if s, ok := tt.body.(string); ok {
					w.WriteHeader(tt.status)
					_, _ = w.Write([]byte(s))
					return
				}
				replyJSON(w, tt.status, tt.body)
			})

			_, err := c.GenerateImage(t.Context(), Request{Model: ModelPro, Prompt: "p"})

			var se *StatusError
			require.ErrorAs(t, err, &se)
			assert.Equal(t, tt.want, *se)
			assert.Equal(t, tt.wantStops, se.StopsBatch())
			if tt.wantString != "" {
				assert.EqualError(t, err, tt.wantString)
			}
		})
	}
}

func TestGenerateImage_ErrorsNeverCarryTheKey(t *testing.T) {
	tests := []struct {
		name        string
		status      int
		body        string
		wantMessage string
	}{
		{
			name:        "echoed in the message",
			status:      http.StatusBadRequest,
			body:        `{"error":{"code":400,"status":"INVALID_ARGUMENT","message":"API key k-123 is not valid."}}`,
			wantMessage: "API key *** is not valid.",
		},
		{
			name:   "across the cut",
			status: http.StatusBadRequest,
			// The key starts at character 297, so a cut at 300 would keep its
			// first 3 characters if it were not masked first.
			body:        `{"error":{"code":400,"status":"INVALID_ARGUMENT","message":"` + strings.Repeat("x", 297) + `k-123 and more"}}`,
			wantMessage: strings.Repeat("x", 297) + "***…",
		},
		{
			name:        "in a body that is not JSON",
			status:      http.StatusBadGateway,
			body:        "bad gateway for key k-123",
			wantMessage: "bad gateway for key ***",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			})

			_, err := c.GenerateImage(t.Context(), Request{Model: ModelPro, Prompt: "p"})

			var se *StatusError
			require.ErrorAs(t, err, &se)
			assert.Equal(t, tt.wantMessage, se.Message)
			assert.NotContains(t, err.Error(), "k-1")
		})
	}

	t.Run("a closed connection", func(t *testing.T) {
		c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
			conn, _, err := w.(http.Hijacker).Hijack()
			if err == nil {
				_ = conn.Close()
			}
		})

		_, err := c.GenerateImage(t.Context(), Request{Model: ModelPro, Prompt: "p"})

		require.Error(t, err)
		assert.True(t, strings.HasPrefix(err.Error(), "Gemini request failed: "), err.Error())
		assert.NotContains(t, err.Error(), "k-123")
		assert.NotContains(t, err.Error(), c.baseURL, "the URL must not be printed")
	})
}

func TestGenerateImage_CutsALongErrorMessage(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		replyJSON(w, http.StatusInternalServerError, errorReply(500, "INTERNAL", strings.Repeat("é", 400)))
	})

	_, err := c.GenerateImage(t.Context(), Request{Model: ModelPro, Prompt: "p"})

	var se *StatusError
	require.ErrorAs(t, err, &se)
	assert.Equal(t, strings.Repeat("é", 300)+"…", se.Message)
	assert.Equal(t, 301, utf8.RuneCountInString(se.Message))
}

func TestGenerateImage_LimitsTheResponse(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(bytes.Repeat([]byte{' '}, 33<<20))
	})

	_, err := c.GenerateImage(t.Context(), Request{Model: ModelPro, Prompt: "p"})

	require.EqualError(t, err, "response from Gemini is larger than 32 MiB")
}

func TestGenerateImage_AnUnreadableResponseIsAnError(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		replyJSON(w, http.StatusOK, candidateReply("STOP", map[string]any{
			"inlineData": map[string]any{"mimeType": "image/png", "data": "***not base64***"},
		}))
	})

	_, err := c.GenerateImage(t.Context(), Request{Model: ModelPro, Prompt: "p"})

	require.ErrorContains(t, err, "decoding Gemini response")
	require.NotErrorAs(t, err, new(*BlockedError))
}

func TestGenerateImage_ACancelledCallStaysACancellation(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		replyJSON(w, http.StatusOK, candidateReply("STOP", inlinePart([]byte("x"), "image/png", false)))
	})
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := c.GenerateImage(ctx, Request{Model: ModelPro, Prompt: "p"})

	require.ErrorIs(t, err, context.Canceled)
	assert.NotContains(t, err.Error(), c.baseURL, "the URL must not be printed")
}

func TestCheckKey(t *testing.T) {
	t.Run("a good key", func(t *testing.T) {
		var gotMethod, gotPath, gotKey, gotQuery string
		c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			gotMethod, gotPath, gotQuery = r.Method, r.URL.Path, r.URL.RawQuery
			gotKey = r.Header.Get("x-goog-api-key")
			replyJSON(w, http.StatusOK, map[string]any{"name": "models/gemini-3-pro-image"})
		})

		require.NoError(t, c.CheckKey(t.Context()))

		assert.Equal(t, http.MethodGet, gotMethod)
		assert.Equal(t, testCheckPath, gotPath)
		assert.Equal(t, "k-123", gotKey)
		assert.Empty(t, gotQuery, "the key must not travel in the URL")
	})

	t.Run("a bad key", func(t *testing.T) {
		c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
			replyJSON(w, http.StatusBadRequest, errorReply(400, "INVALID_ARGUMENT", "API key not valid.",
				map[string]any{"reason": "API_KEY_INVALID"}))
		})

		err := c.CheckKey(t.Context())

		var se *StatusError
		require.ErrorAs(t, err, &se)
		assert.Equal(t, 400, se.Code)
		assert.Equal(t, "API_KEY_INVALID", se.Reason)
		assert.True(t, se.StopsBatch())
	})
}

// TestClient_DoesNotFollowRedirects keeps the key on Google's host: a redirect
// is an error, and the place it points to is never asked.
func TestClient_DoesNotFollowRedirects(t *testing.T) {
	calls := map[string]func(*Client) error{
		"GenerateImage": func(c *Client) error {
			_, err := c.GenerateImage(t.Context(), Request{Model: ModelPro, Prompt: "p"})
			return err
		},
		"CheckKey": func(c *Client) error { return c.CheckKey(t.Context()) },
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			var elsewhereCalls atomic.Int32
			var elsewhereKey atomic.Value
			elsewhere := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				elsewhereCalls.Add(1)
				elsewhereKey.Store(r.Header.Get("x-goog-api-key"))
				replyJSON(w, http.StatusOK, map[string]any{})
			}))
			t.Cleanup(elsewhere.Close)
			c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Location", elsewhere.URL+r.URL.Path)
				w.WriteHeader(http.StatusTemporaryRedirect)
			})

			err := call(c)

			var se *StatusError
			require.ErrorAs(t, err, &se)
			assert.Equal(t, http.StatusTemporaryRedirect, se.Code)
			assert.Zero(t, elsewhereCalls.Load(), "the redirect target must never be asked")
			assert.Nil(t, elsewhereKey.Load(), "the key must never reach the redirect target")
		})
	}
}
