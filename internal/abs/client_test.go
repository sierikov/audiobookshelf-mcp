package abs

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
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
