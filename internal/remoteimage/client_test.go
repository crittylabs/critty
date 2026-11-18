package remoteimage

import (
	"context"
	"io"
	"net/http"
	"net/netip"
	"slices"
	"strings"
	"testing"
)

func TestAllowedImageURL(t *testing.T) {
	client := New([]string{"Images.Example.org", ".CDN.Example.org"})
	for _, tc := range []struct {
		url  string
		want bool
	}{
		{"https://images.example.org/photo.png", true},
		{"https://IMAGES.EXAMPLE.ORG:443/photo.png", true},
		{"http://images.example.org/photo.png", false},
		{"//images.example.org/photo.png", false},
		{"https://images.example.org:8443/photo.png", false},
		{"https://login@images.example.org/photo.png", false},
		{"https://images.example.org@other.example.org/photo.png", false},
		{"https://sub.images.example.org/photo.png", false},
		{"https://images.example.org.other.example.org/photo.png", false},
		{"https://media.cdn.example.org/photo.png", true},
		{"https://A.MEDIA.CDN.EXAMPLE.ORG:443/photo.png", true},
		{"https://cdn.example.org/photo.png", false},
		{"https://othercdn.example.org/photo.png", false},
		{"https://media.cdn.example.org.other.example.org/photo.png", false},
	} {
		t.Run(tc.url, func(t *testing.T) {
			if got := client.Allows(tc.url); got != tc.want {
				t.Errorf("Allows = %v, want %v", got, tc.want)
			}
			if New(nil).Allows(tc.url) {
				t.Error("empty allowlist allowed URL")
			}
		})
	}
}

func TestPublicImageAddresses(t *testing.T) {
	for _, tc := range []struct {
		address string
		want    bool
	}{
		{"1.1.1.1", true},
		{"2606:4700:4700::1111", true},
		{"127.0.0.1", false},
		{"172.16.0.1", false},
		{"169.254.169.254", false},
		{"100.64.0.1", false},
		{"::1", false},
		{"::ffff:192.168.0.1", false},
		{"fc00::1", false},
		{"64:ff9b::a00:1", false},
	} {
		t.Run(tc.address, func(t *testing.T) {
			if got := publicIP(netip.MustParseAddr(tc.address)); got != tc.want {
				t.Errorf("publicIP = %v, want %v", got, tc.want)
			}
		})
	}
}

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestFetchRedirects(t *testing.T) {
	for _, tc := range []struct {
		name, location, wantError string
		wantHosts                 []string
	}{
		{"permitted", "https://media.cdn.example.org/badge", "", []string{"images.example.org", "media.cdn.example.org"}},
		{"different host", "https://other.example.org/badge", "outside the image allowlist", []string{"images.example.org"}},
		{"downgrade", "http://media.cdn.example.org/badge", "outside the image allowlist", []string{"images.example.org"}},
		{"loop", "/start", "too many image redirects", []string{"images.example.org", "images.example.org", "images.example.org", "images.example.org"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := New([]string{"images.example.org", ".cdn.example.org"})
			var hosts []string
			client.http.Transport = transportFunc(func(req *http.Request) (*http.Response, error) {
				if ref := req.Referer(); ref != "" {
					t.Error("redirect sent a Referer")
				}
				hosts = append(hosts, req.URL.Hostname())
				resp := &http.Response{Request: req, Header: make(http.Header), Body: http.NoBody}
				if req.URL.Path == "/start" {
					resp.StatusCode = http.StatusFound
					resp.Header.Set("Location", tc.location)
				} else {
					resp.StatusCode = http.StatusOK
					resp.Header.Set("Content-Type", "image/svg+xml; charset=utf-8")
					resp.Body = io.NopCloser(strings.NewReader("<svg/>"))
				}
				return resp, nil
			})
			data, svg, err := client.Fetch(t.Context(), "https://images.example.org/start?token=test")
			if tc.wantError == "" {
				if err != nil || !svg || string(data) != "<svg/>" {
					t.Fatalf("Fetch = %q, SVG %v, error %v", data, svg, err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tc.wantError) || strings.Contains(err.Error(), "token=") {
				t.Fatalf("Fetch error = %v, want %q without request query", err, tc.wantError)
			}
			if !slices.Equal(hosts, tc.wantHosts) {
				t.Fatalf("contacted hosts %v, want %v", hosts, tc.wantHosts)
			}
		})
	}
}

func TestFetchResponseLimits(t *testing.T) {
	for _, tc := range []struct {
		name, want    string
		status        int
		contentLength int64
		bodySize      int
	}{
		{"non-success", "HTTP 404", http.StatusNotFound, 0, 0},
		{"declared limit", "exceeds 5 MiB", http.StatusOK, maxBytes + 1, 0},
		{"streamed limit", "exceeds 5 MiB", http.StatusOK, -1, maxBytes + 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := New([]string{"images.example.org"})
			client.http.Transport = transportFunc(func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					Request: req, StatusCode: tc.status, ContentLength: tc.contentLength,
					Body: io.NopCloser(strings.NewReader(strings.Repeat("x", tc.bodySize))),
				}, nil
			})
			if _, _, err := client.Fetch(t.Context(), "https://images.example.org/pic.png"); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Fetch error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestFetchWithEmptyAllowlist(t *testing.T) {
	if _, _, err := New(nil).Fetch(context.Background(), "https://images.example.org/pic.png"); err == nil {
		t.Fatal("empty allowlist accepted fetch")
	}
}
