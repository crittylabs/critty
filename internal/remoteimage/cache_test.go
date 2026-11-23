package remoteimage

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFetchReusesValidatedImageAcrossClients(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	const url = "https://images.example.org/start?token=example"
	requests := 0
	newClient := func(hosts ...string) *Client {
		client := New(hosts, DefaultCacheMaxBytes)
		client.http.Transport = transportFunc(func(req *http.Request) (*http.Response, error) {
			requests++
			resp := &http.Response{Request: req, Header: make(http.Header), Body: http.NoBody}
			if req.URL.Path == "/start" {
				resp.StatusCode = http.StatusFound
				resp.Header.Set("Location", "https://cdn.example.org/asset.svg")
			} else {
				resp.StatusCode = http.StatusOK
				resp.Header.Set("Cache-Control", "max-age=300")
				resp.Body = io.NopCloser(strings.NewReader("<svg/>"))
			}
			return resp, nil
		})
		return client
	}
	fetch := func(client *Client, wantRequests int) Image {
		t.Helper()
		image, err := client.Fetch(t.Context(), url)
		if err != nil || !image.SVG || string(image.Data) != "<svg/>" {
			t.Fatalf("Fetch = %q, SVG %v, error %v", image.Data, image.SVG, err)
		}
		if requests != wantRequests {
			t.Fatalf("network requests = %d, want %d", requests, wantRequests)
		}
		return image
	}
	client := newClient("images.example.org", "cdn.example.org")
	fetch(client, 2)
	fetch(client, 4).Cache(t.Context())
	other := newClient("CDN.EXAMPLE.ORG", "images.example.org")
	fetch(other, 4)
	restricted := newClient("images.example.org")
	if _, err := restricted.Fetch(t.Context(), url); err == nil || requests != 5 {
		t.Fatalf("changed allowlist reused a redirect: requests = %d, error %v", requests, err)
	}
	other.Refresh()
	fetch(other, 7).Cache(t.Context())
	fetch(other, 7)
}

func TestFetchSkipsUncacheableResponses(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	for _, directive := range []string{"no-store", "no-cache", "max-age=0", "max-age=60"} {
		t.Run(directive, func(t *testing.T) {
			client := New([]string{"images.example.org"}, DefaultCacheMaxBytes)
			requests := 0
			client.http.Transport = transportFunc(func(req *http.Request) (*http.Response, error) {
				requests++
				return &http.Response{
					Request: req, StatusCode: http.StatusOK,
					Header: http.Header{"Cache-Control": {directive}, "Age": {"60"}},
					Body:   io.NopCloser(strings.NewReader("image")),
				}, nil
			})
			for range 2 {
				image, err := client.Fetch(t.Context(), "https://images.example.org/pic.png")
				if err != nil {
					t.Fatal(err)
				}
				image.Cache(t.Context())
			}
			if requests != 2 {
				t.Fatalf("network requests = %d, want 2", requests)
			}
		})
	}
}

func TestFetchAppliesConfiguredCacheLimit(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	const first = "https://images.example.org/first.png"
	const second = "https://images.example.org/second.png"
	const capacity = cacheHeaderSize + 1
	requests := 0
	newClient := func(limit int64) *Client {
		client := New([]string{"images.example.org"}, limit)
		client.http.Transport = transportFunc(func(req *http.Request) (*http.Response, error) {
			requests++
			return &http.Response{
				Request: req, StatusCode: http.StatusOK,
				Body: io.NopCloser(strings.NewReader("x")),
			}, nil
		})
		return client
	}
	fetch := func(client *Client, url string, wantRequests int) {
		t.Helper()
		image, err := client.Fetch(t.Context(), url)
		if err != nil {
			t.Fatal(err)
		}
		image.Cache(t.Context())
		if requests != wantRequests {
			t.Fatalf("network requests = %d, want %d", requests, wantRequests)
		}
	}
	larger := newClient(2 * capacity)
	fetch(larger, first, 1)
	fetch(larger, second, 2)
	fetch(larger, second, 2)

	disabled := newClient(0)
	fetch(disabled, second, 3)
	fetch(disabled, second, 4)

	smaller := newClient(capacity)
	fetch(smaller, second, 4)
	files, err := filepath.Glob(filepath.Join(smaller.cache.dir, "*.image"))
	if err != nil || len(files) != 1 {
		t.Fatalf("after reducing limit, cache contains %d files, error %v", len(files), err)
	}
	fetch(smaller, first, 5)
}

func TestDiskCacheKeepsRecentlyReadImagesWithinBudget(t *testing.T) {
	cache := diskCache{dir: t.TempDir(), limit: 2 * (cacheHeaderSize + 1)}
	data := []byte("x")
	now := time.Now()
	key := func(n int) string { return fmt.Sprintf("%064x.image", n) }
	put := func(n int) {
		t.Helper()
		if err := cache.put(t.Context(), key(n), cacheHeader{fetched: now.UnixNano()}, data); err != nil {
			t.Fatal(err)
		}
	}
	put(1)
	put(2)
	firstRead := now.Add(time.Minute)
	if _, ok := cache.get(t.Context(), key(1), firstRead, 0); !ok {
		t.Fatal("first image did not survive a disk read")
	}
	put(3)
	for n, want := range map[int]bool{1: true, 2: false, 3: true} {
		if _, ok := cache.get(t.Context(), key(n), firstRead, 0); ok != want {
			t.Errorf("cache hit for image %d = %t, want %t", n, ok, want)
		}
	}
	var bytes int64
	files, err := filepath.Glob(filepath.Join(cache.dir, "*.image"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range files {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		bytes += info.Size()
	}
	if bytes != cache.limit {
		t.Fatalf("cache size = %d, want %d", bytes, cache.limit)
	}
}
