package service

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sierikov/audiobookshelf-mcp/internal/abs"
)

// Bodies that start the way real images do, which is all the fetcher looks at.
var (
	jpegBytes = []byte("\xff\xd8\xff\xe0\x00\x10JFIF\x00 not a whole picture")
	pngBytes  = []byte("\x89PNG\r\n\x1a\n not a whole picture")
	webpBytes = []byte("RIFF\x24\x00\x00\x00WEBPVP8 not a whole picture")
	gifBytes  = []byte("GIF89a not a whole picture")
)

func serve(t *testing.T, h http.Handler) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv
}

// respond answers every request with body under contentType. An empty
// contentType sends no Content-Type header at all.
func respond(contentType string, body []byte) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		if contentType == "" {
			w.Header()["Content-Type"] = nil // stops the server from guessing one
		} else {
			w.Header().Set("Content-Type", contentType)
		}
		_, _ = w.Write(body)
	}
}

// redirectsThenImage sends a client through hops redirects (/hop/0 to
// /hop/1 and so on) and then serves a JPEG.
func redirectsThenImage(hops int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		n, err := strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/hop/"))
		if err != nil || n >= hops {
			respond("image/jpeg", jpegBytes)(w, r)
			return
		}
		http.Redirect(w, r, fmt.Sprintf("/hop/%d", n+1), http.StatusFound)
	}
}

func TestImageFetcher_ReturnsTheImageWithItsType(t *testing.T) {
	atLimit := append(bytes.Clone(jpegBytes), make([]byte, abs.MaxImageBytes-len(jpegBytes))...)
	tests := []struct {
		name        string
		contentType string
		body        []byte
		want        abs.Image
	}{
		{"jpeg", "image/jpeg", jpegBytes, abs.Image{Data: jpegBytes, MIMEType: "image/jpeg"}},
		{"png", "image/png", pngBytes, abs.Image{Data: pngBytes, MIMEType: "image/png"}},
		{"webp", "image/webp", webpBytes, abs.Image{Data: webpBytes, MIMEType: "image/webp"}},
		{"gif", "image/gif", gifBytes, abs.Image{Data: gifBytes, MIMEType: "image/gif"}},
		{"webp declared as jpeg, as FantLab does", "image/jpeg", webpBytes, abs.Image{Data: webpBytes, MIMEType: "image/webp"}},
		{"type with parameters, in capitals", "IMAGE/PNG; charset=binary", pngBytes, abs.Image{Data: pngBytes, MIMEType: "image/png"}},
		{"exactly the size limit", "image/jpeg", atLimit, abs.Image{Data: atLimit, MIMEType: "image/jpeg"}},
		{"declared as one type, is another", "image/jpeg", pngBytes, abs.Image{Data: pngBytes, MIMEType: "image/png"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := serve(t, respond(tt.contentType, tt.body))

			got, err := NewImageFetcher().Fetch(t.Context(), srv.URL+"/cover")

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestImageFetcher_Refusals checks every way a candidate can misbehave, as
// the Problem text the covers service shows for it.
func TestImageFetcher_Refusals(t *testing.T) {
	tooBig := append(bytes.Clone(jpegBytes), make([]byte, 3<<20)...)
	oneOver := append(bytes.Clone(jpegBytes), make([]byte, abs.MaxImageBytes-len(jpegBytes)+1)...)
	tests := []struct {
		name        string
		handler     http.Handler
		wantProblem string
	}{
		{"html page", respond("text/html; charset=utf-8", []byte("<html>no cover here</html>")), "not an image"},
		{"svg", respond("image/svg+xml", []byte("<svg xmlns='http://www.w3.org/2000/svg'/>")), "not an image"},
		{"no content type", respond("", pngBytes), "not an image"},
		{"image type without an image", respond("image/jpeg", []byte("<html>no cover here</html>")), "not an image"},
		{"empty image", respond("image/jpeg", nil), "not an image"},
		{"3 MiB", respond("image/jpeg", tooBig), "over 2 MiB"},
		{"one byte over the limit", respond("image/jpeg", oneOver), "over 2 MiB"},
		{"not found", http.NotFoundHandler(), "HTTP 404"},
		{"server error", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "boom", http.StatusInternalServerError)
		}), "HTTP 500"},
		{"redirect to ftp", http.RedirectHandler("ftp://example.com/cover.jpg", http.StatusFound), "not an http or https URL"},
		{"redirect to file", http.RedirectHandler("file:///etc/passwd", http.StatusFound), "not an http or https URL"},
		{"redirect to a user name and password", http.RedirectHandler("http://user:pass@example.com/cover.jpg", http.StatusFound), "not an http or https URL"},
		{"endless redirects", http.RedirectHandler("/again", http.StatusFound), "too many redirects"},
		{"four redirects", redirectsThenImage(4), "too many redirects"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := serve(t, tt.handler)

			got, err := NewImageFetcher().Fetch(t.Context(), srv.URL+"/hop/0")

			require.Error(t, err)
			assert.Equal(t, tt.wantProblem, problemOf(err))
			assert.Equal(t, abs.Image{}, got)
		})
	}
}

func TestImageFetcher_FollowsThreeRedirects(t *testing.T) {
	srv := serve(t, redirectsThenImage(3))

	got, err := NewImageFetcher().Fetch(t.Context(), srv.URL+"/hop/0")

	require.NoError(t, err)
	assert.Equal(t, abs.Image{Data: jpegBytes, MIMEType: "image/jpeg"}, got)
}

func TestImageFetcher_SlowServerTimesOut(t *testing.T) {
	srv := serve(t, http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(10 * time.Second):
		}
	}))

	_, err := newImageFetcher(50*time.Millisecond).Fetch(t.Context(), srv.URL+"/cover")

	require.Error(t, err)
	assert.Equal(t, "timed out", problemOf(err))
}

func TestImageFetcher_ServerStallingMidBodyTimesOut(t *testing.T) {
	srv := serve(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write(jpegBytes)
		w.(http.Flusher).Flush()
		select {
		case <-r.Context().Done():
		case <-time.After(10 * time.Second):
		}
	}))

	_, err := newImageFetcher(100*time.Millisecond).Fetch(t.Context(), srv.URL+"/cover")

	require.Error(t, err)
	assert.Equal(t, "timed out", problemOf(err))
}

func TestImageFetcher_SendsNoCredentials(t *testing.T) {
	var mu sync.Mutex
	var seen []http.Header
	srv := serve(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, r.Header.Clone())
		mu.Unlock()
		redirectsThenImage(1)(w, r)
	}))

	_, err := NewImageFetcher().Fetch(t.Context(), srv.URL+"/hop/0")

	require.NoError(t, err)
	mu.Lock()
	defer mu.Unlock()
	require.Len(t, seen, 2, "the redirect is followed")
	for _, h := range seen {
		assert.Empty(t, h.Values("Authorization"))
		assert.Empty(t, h.Values("Cookie"))
	}
}

func TestImageFetcher_RefusesANonWebURLWithoutAnyRequest(t *testing.T) {
	for _, u := range []string{
		"ftp://example.com/c.jpg", "file:///etc/passwd", "", "http:///c.jpg", "//example.com/c.jpg", "javascript:alert(1)",
		"http://user:pass@example.com/c.jpg", "https://trusted.example@evil.example/c.jpg", "https://example.com/c\u2028.jpg",
	} {
		t.Run(u, func(t *testing.T) {
			_, err := NewImageFetcher().Fetch(t.Context(), u)

			require.ErrorIs(t, err, ErrNotWebURL)
		})
	}
}

// TestImageFetcher_NeverSendsUserInfo checks that a user name and password in
// a URL, which Go would turn into an Authorization header, never leave: not in
// the URL asked for, and not in a redirect.
func TestImageFetcher_NeverSendsUserInfo(t *testing.T) {
	var mu sync.Mutex
	var paths, authorizations []string
	var srv *httptest.Server
	srv = serve(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.URL.Path)
		authorizations = append(authorizations, r.Header.Get("Authorization"))
		mu.Unlock()
		if r.URL.Path == "/start" {
			http.Redirect(w, r, strings.Replace(srv.URL, "http://", "http://user:pass@", 1)+"/cover", http.StatusFound)
			return
		}
		respond("image/jpeg", jpegBytes)(w, r)
	}))
	withUserInfo := strings.Replace(srv.URL, "http://", "http://user:pass@", 1)

	tests := []struct {
		name, url string
		wantPaths []string
	}{
		{"in the URL", withUserInfo + "/cover", nil},
		{"in a redirect", srv.URL + "/start", []string{"/start"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mu.Lock()
			paths, authorizations = nil, nil
			mu.Unlock()

			_, err := NewImageFetcher().Fetch(t.Context(), tt.url)

			require.Error(t, err)
			assert.Equal(t, "not an http or https URL", problemOf(err))
			mu.Lock()
			defer mu.Unlock()
			assert.Equal(t, tt.wantPaths, paths, "no request goes to the URL with the password")
			for _, a := range authorizations {
				assert.Empty(t, a)
			}
		})
	}
}
