package abs

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sierikov/audiobookshelf-mcp/internal/abs/abstest"
)

func TestClient_ReturnsAPIErrorWithStatusAndBody(t *testing.T) {
	srv := abstest.NewServer(t, nil)
	c := NewClient(srv.URL, abstest.Token)

	_, err := c.Libraries(context.Background())

	require.EqualError(t, err, "ABS API 404: not found\n")
}

func TestClient_SendsBearerToken(t *testing.T) {
	srv := abstest.NewServer(t, map[string]any{"/api/libraries": map[string]any{"libraries": []any{}}})

	_, err := NewClient(srv.URL, "wrong-token").Libraries(context.Background())
	require.EqualError(t, err, "ABS API 401: unauthorized\n")

	_, err = NewClient(srv.URL, abstest.Token).Libraries(context.Background())
	require.NoError(t, err)
}

func TestClient_TrimsTrailingSlashFromBaseURL(t *testing.T) {
	srv := abstest.NewServer(t, map[string]any{"/api/libraries": map[string]any{"libraries": []any{}}})

	_, err := NewClient(srv.URL+"/", abstest.Token).Libraries(context.Background())

	require.NoError(t, err)
	assert.Equal(t, "/api/libraries", srv.LastRequest().URL.Path)
}

func TestClient_EscapesPathParams(t *testing.T) {
	srv := abstest.NewServer(t, nil)

	_, _ = NewClient(srv.URL, abstest.Token).Item(context.Background(), "../admin")

	assert.Equal(t, "/api/items/..%2Fadmin", srv.LastRequest().URL.EscapedPath())
}

func TestClient_DoSendsJSONBody(t *testing.T) {
	var gotMethod, gotType string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotType = r.Header.Get("Content-Type")
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(srv.Close)

	var out struct {
		OK bool `json:"ok"`
	}
	err := NewClient(srv.URL, abstest.Token).do(context.Background(), http.MethodPatch, "/api/x", nil, map[string]any{"progress": 0.5}, &out)

	require.NoError(t, err)
	assert.Equal(t, http.MethodPatch, gotMethod)
	assert.Equal(t, "application/json", gotType)
	assert.InDelta(t, 0.5, gotBody["progress"], 0)
	assert.True(t, out.OK)
}

func TestClient_APIErrorIsStatusError(t *testing.T) {
	srv := abstest.NewServer(t, nil)

	_, err := NewClient(srv.URL, abstest.Token).Libraries(context.Background())

	var se *StatusError
	require.ErrorAs(t, err, &se)
	assert.Equal(t, http.StatusNotFound, se.StatusCode)
	assert.Equal(t, "not found\n", se.Body)
	assert.Equal(t, "ABS API 404: not found\n", se.Error())
}

func TestClient_StatusErrorTextIsShort(t *testing.T) {
	longScript := "<script>" + strings.Repeat("var a=1;", 2000) + "</script>"
	tests := []struct {
		name        string
		contentType string
		body        string
		want        string
	}{
		{
			name:        "an HTML page gives its title",
			contentType: "text/html; charset=UTF-8",
			body:        "<!DOCTYPE html><html><head><title>Cloudflare Tunnel error | example.com | Cloudflare</title>" + longScript + "</head><body>Error 1033</body></html>",
			want:        "ABS API 530: Cloudflare Tunnel error | example.com | Cloudflare",
		},
		{
			name: "an HTML page without a content type is found by its start",
			body: "\n  <!doctype html>\n<HTML><HEAD><TITLE>\n  Bad &amp; Gateway\n</TITLE></HEAD></HTML>",
			want: "ABS API 530: Bad & Gateway",
		},
		{
			name:        "an HTML page without a title",
			contentType: "text/html",
			body:        "<html><body>" + longScript + "</body></html>",
			want:        "ABS API 530: an HTML page without a title",
		},
		{
			name:        "long text is cut",
			contentType: "text/plain",
			body:        strings.Repeat("é", 400),
			want:        "ABS API 530: " + strings.Repeat("é", 300) + "…",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if tt.contentType != "" {
					w.Header().Set("Content-Type", tt.contentType)
				}
				w.WriteHeader(530)
				_, _ = w.Write([]byte(tt.body))
			}))
			t.Cleanup(srv.Close)
			c := NewClient(srv.URL, abstest.Token)

			_, err := c.Libraries(t.Context())
			require.EqualError(t, err, tt.want, "JSON request")

			_, err = c.ItemCover(t.Context(), "li_1", 400)
			require.EqualError(t, err, tt.want, "image request")
			var se *StatusError
			require.ErrorAs(t, err, &se)
			assert.Equal(t, 530, se.StatusCode)
		})
	}
}

func TestClient_StatusErrorMasksTheToken(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		body        string
		want        string
	}{
		{
			name: "across the cut",
			// The token starts at character 295, so the cut at 300 would keep
			// its first 5 characters if it were not masked first.
			body: strings.Repeat("x", 288) + "Bearer " + abstest.Token,
			want: "ABS API 401: " + strings.Repeat("x", 288) + "Bearer ***",
		},
		{
			name:        "in an HTML title",
			contentType: "text/html",
			body:        "<html><title>Denied for " + abstest.Token + "</title></html>",
			want:        "ABS API 401: Denied for ***",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if tt.contentType != "" {
					w.Header().Set("Content-Type", tt.contentType)
				}
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(tt.body))
			}))
			t.Cleanup(srv.Close)

			_, err := NewClient(srv.URL, abstest.Token).Libraries(t.Context())

			require.EqualError(t, err, tt.want)
		})
	}
}
