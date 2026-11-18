// Package remoteimage fetches previews from user-approved HTTPS hosts.
package remoteimage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/crittylabs/critty/v2/internal/imageinfo"
)

const (
	maxBytes = 5 << 20
	timeout  = 5 * time.Second
)

var hostname = regexp.MustCompile(`^(?:[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?\.)*[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?$`)

func ValidateHosts(hosts []string) error {
	for _, entry := range hosts {
		host := strings.TrimPrefix(entry, ".")
		sample := strings.ReplaceAll(host, "*", "a")
		if len(host) > 253 || !strings.Contains(entry, ".") || !hostname.MatchString(sample) || net.ParseIP(host) != nil {
			return fmt.Errorf("remote_image_hosts requires a DNS hostname, wildcard pattern, or dot-prefixed suffix, got %q", entry)
		}
	}
	return nil
}

type Client struct {
	hosts    map[string]bool
	patterns []*regexp.Regexp
	http     http.Client
	slots    chan struct{}
}

// New creates a fetcher for validated hosts. A nil client denies every URL.
func New(hosts []string) *Client {
	if len(hosts) == 0 {
		return nil
	}
	c := &Client{hosts: make(map[string]bool, len(hosts)), slots: make(chan struct{}, 4)}
	for _, entry := range hosts {
		entry = strings.ToLower(entry)
		if !strings.Contains(entry, "*") {
			c.hosts[entry] = true
			continue
		}
		host, suffix := strings.CutPrefix(entry, ".")
		pattern := strings.ReplaceAll(regexp.QuoteMeta(host), `\*`, `[a-z0-9-]*`)
		if suffix {
			pattern = `(?:[a-z0-9-]+\.)+` + pattern
		}
		c.patterns = append(c.patterns, regexp.MustCompile("^"+pattern+"$"))
	}
	c.http = http.Client{
		Transport: &http.Transport{
			DialContext:            dialPublic,
			ForceAttemptHTTP2:      true,
			MaxIdleConns:           8,
			IdleConnTimeout:        30 * time.Second,
			MaxResponseHeaderBytes: 32 << 10,
		},
		CheckRedirect: c.checkRedirect,
	}
	return c
}

func (c *Client) Allows(rawURL string) bool {
	if c == nil {
		return false
	}
	u, err := url.Parse(strings.TrimSpace(rawURL))
	return err == nil && c.allows(u)
}

func (c *Client) allows(u *url.URL) bool {
	if c == nil || u.Scheme != "https" || u.User != nil || u.Opaque != "" {
		return false
	}
	host := u.Hostname()
	if len(host) > 253 || !hostname.MatchString(host) || net.ParseIP(host) != nil || u.Host != host && u.Host != host+":443" {
		return false
	}
	host = strings.ToLower(host)
	if c.hosts[host] {
		return true
	}
	for i := range len(host) {
		if host[i] == '.' && c.hosts[host[i:]] {
			return true
		}
	}
	for _, pattern := range c.patterns {
		if pattern.MatchString(host) {
			return true
		}
	}
	return false
}

func (c *Client) checkRedirect(req *http.Request, via []*http.Request) error {
	if !c.allows(req.URL) {
		return errors.New("redirect target is outside the image allowlist")
	}
	if len(via) > 3 {
		return errors.New("too many image redirects")
	}
	req.Header.Del("Referer")
	return nil
}

// Fetch returns the response body and whether it should be decoded as SVG.
func (c *Client) Fetch(ctx context.Context, rawURL string) ([]byte, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimSpace(rawURL), nil)
	if err != nil || !c.allows(req.URL) {
		return nil, false, errors.New("URL is outside the image allowlist")
	}
	select {
	case c.slots <- struct{}{}:
		defer func() { <-c.slots }()
	case <-ctx.Done():
		return nil, false, ctx.Err()
	}
	resp, err := c.http.Do(req)
	if err != nil {
		var requestErr *url.Error
		if errors.As(err, &requestErr) {
			// Request errors otherwise expose the full URL, including query parameters.
			err = requestErr.Err
		}
		return nil, false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("image server returned HTTP %d", resp.StatusCode)
	}
	if resp.ContentLength > maxBytes {
		return nil, false, errors.New("image exceeds 5 MiB")
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return nil, false, err
	}
	if len(data) > maxBytes {
		return nil, false, errors.New("image exceeds 5 MiB")
	}
	contentType, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	return data, contentType == "image/svg+xml" || imageinfo.SVGPath(resp.Request.URL.Path), nil
}

// dialPublic connects to the resolved address that passed the policy check.
// The transport still uses the original hostname for TLS verification.
func dialPublic(ctx context.Context, network, address string) (net.Conn, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	addresses, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host+".")
	if err != nil {
		return nil, err
	}
	err = errors.New("image host has no public address")
	dialer := net.Dialer{Timeout: 1500 * time.Millisecond}
	for _, ip := range addresses {
		if !publicIP(ip) {
			continue
		}
		conn, dialErr := dialer.DialContext(ctx, network, net.JoinHostPort(ip.Unmap().String(), port))
		if dialErr == nil {
			return conn, nil
		}
		err = dialErr
	}
	return nil, err
}

var specialUsePrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("192.88.99.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("2001::/23"),
	netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("2002::/16"),
	netip.MustParsePrefix("3fff::/20"),
}

var globalIPv6 = netip.MustParsePrefix("2000::/3")

func publicIP(ip netip.Addr) bool {
	ip = ip.Unmap()
	if !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.Is6() && !globalIPv6.Contains(ip) {
		return false
	}
	for _, prefix := range specialUsePrefixes {
		if prefix.Contains(ip) {
			return false
		}
	}
	return true
}
