package service

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bitxeno/atvloadly/internal/ipa"
)

func TestAppleIconNameSearchLooksUpTVEditions(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Query().Get("country") != "gb" {
			t.Error("store country was lost")
		}
		switch r.URL.Path {
		case "/search":
			if r.URL.Query().Get("term") != "Plex & Movies" || r.URL.Query().Get("entity") != "software" {
				t.Error("app name search parameters are incorrect")
			}
			_, _ = io.WriteString(w, `{"results":[{"trackId":123},{"trackId":456}]}`)
		case "/lookup":
			if r.URL.Query().Get("id") != "123,456" || r.URL.Query().Get("entity") != "tvSoftware" {
				t.Error("tvOS lookup did not batch the search IDs")
			}
			_, _ = io.WriteString(w, `{"results":[{"trackId":123,"trackName":"Plex TV","artistName":"Plex","artworkUrl512":"https://is1-ssl.mzstatic.com/image/thumb/test/icon.lsr/512x512bb.jpg"},{"trackId":123,"artworkUrl512":"https://is1-ssl.mzstatic.com/duplicate.jpg"},{"trackId":456,"artworkUrl512":"https://example.com/icon.jpg"}]}`)
		default:
			t.Error("unexpected endpoint")
		}
	}))
	defer server.Close()
	catalog := appleIconCatalog{client: server.Client(), baseURL: server.URL}
	results, err := catalog.search(context.Background(), " Plex & Movies ", "GB", true)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 || len(results) != 1 {
		t.Fatalf("calls=%d results=%+v", calls, results)
	}
	if results[0].Name != "Plex TV" || !strings.HasSuffix(results[0].ArtworkURL, "/1280x768bb.jpg") || !strings.Contains(results[0].StoreURL, "/gb/app/id123?platform=appleTV") {
		t.Fatalf("incorrect TV result: %+v", results[0])
	}
}

func TestAppleIconDirectLookupAndEmptySearch(t *testing.T) {
	for _, term := range []string{"123", "https://apps.apple.com/us/app/example/id123"} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/lookup" || r.URL.Query().Get("id") != "123" {
				t.Errorf("unexpected request %s", r.URL)
			}
			_, _ = io.WriteString(w, `{"results":[]}`)
		}))
		catalog := appleIconCatalog{client: server.Client(), baseURL: server.URL}
		results, err := catalog.search(context.Background(), term, "", true)
		server.Close()
		if err != nil || results == nil || len(results) != 0 {
			t.Fatalf("empty lookup: %+v %v", results, err)
		}
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search" {
			t.Fatal("empty search should not perform a lookup")
		}
		_, _ = io.WriteString(w, `{"results":[]}`)
	}))
	defer server.Close()
	catalog := appleIconCatalog{client: server.Client(), baseURL: server.URL}
	if _, err := catalog.search(context.Background(), "Nothing", "us", true); err != nil {
		t.Fatal(err)
	}
	for _, request := range [][2]string{{"x", "us"}, {"Plex", "us&country=gb"}} {
		if _, err := catalog.search(context.Background(), request[0], request[1], true); err == nil {
			t.Fatal("invalid search accepted")
		}
	}
}

func TestAppleIconCatalogFailure(t *testing.T) {
	for _, body := range []string{"rate limited", "not json"} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if body == "rate limited" {
				w.WriteHeader(http.StatusTooManyRequests)
			}
			_, _ = io.WriteString(w, body)
		}))
		catalog := appleIconCatalog{client: server.Client(), baseURL: server.URL}
		_, err := catalog.search(context.Background(), "Plex", "us", true)
		server.Close()
		if err == nil {
			t.Fatal("catalog error was ignored")
		}
	}
}

type appleIconTransport func(*http.Request) (*http.Response, error)

func (f appleIconTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestDownloadAppleIconRestrictsOriginAndValidatesImage(t *testing.T) {
	for _, raw := range []string{"http://is1-ssl.mzstatic.com/image.jpg", "https://mzstatic.com.evil.test/image.jpg", "http://127.0.0.1/secret", "file:///etc/passwd", "https://user:pass@is1.mzstatic.com/a", "https://is1.mzstatic.com:8080/a"} {
		if _, err := DownloadAppleIcon(context.Background(), raw); err == nil {
			t.Fatalf("accepted unsafe artwork URL %q", raw)
		}
	}
	original := appleIcons.client
	t.Cleanup(func() { appleIcons.client = original })
	var data bytes.Buffer
	if err := png.Encode(&data, image.NewRGBA(image.Rect(0, 0, 8, 8))); err != nil {
		t.Fatal(err)
	}
	appleIcons.client = &http.Client{Transport: appleIconTransport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(data.Bytes())), Header: make(http.Header)}, nil
	})}
	value, err := DownloadAppleIcon(context.Background(), "https://is1-ssl.mzstatic.com/image.png")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ipa.DecodeCustomIcon(value); err != nil {
		t.Fatal(err)
	}
	data.Reset()
	data.WriteString("not an image")
	if _, err := DownloadAppleIcon(context.Background(), "https://is1-ssl.mzstatic.com/image.png"); err == nil {
		t.Fatal("invalid image accepted")
	}
}
