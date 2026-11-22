// Package extract fetches a show page and turns it into a draft production.
package extract

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"syscall"
	"time"
)

// UserAgent identifies every request the app makes to outside sites.
const UserAgent = "strapontin (personal theatre log)"

// MaxBody caps the size of a fetched page or image.
const MaxBody = 5 << 20

// ErrForbiddenAddress is returned for URLs that resolve to private or local addresses.
var ErrForbiddenAddress = errors.New("address not allowed")

// Fetcher downloads pages and images on behalf of a member.
type Fetcher struct {
	client *http.Client
}

// NewFetcher returns a fetcher that refuses private and loopback addresses.
// allowPrivate lifts that rule, for tests against a local server only.
func NewFetcher(allowPrivate bool) *Fetcher {
	dialer := &net.Dialer{
		Timeout: 5 * time.Second,
		// Control runs after DNS resolution, so a hostname pointing to a
		// private address is refused too.
		Control: func(network, address string, _ syscall.RawConn) error {
			if allowPrivate {
				return nil
			}
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return err
			}
			ip := net.ParseIP(host)
			if ip == nil || !publicIP(ip) {
				return ErrForbiddenAddress
			}
			return nil
		},
	}
	transport := &http.Transport{
		DialContext:           dialer.DialContext,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
		MaxIdleConns:          10,
		IdleConnTimeout:       30 * time.Second,
	}
	return &Fetcher{client: &http.Client{
		Transport: transport,
		Timeout:   10 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("too many redirects")
			}
			return checkURL(req.URL)
		},
	}}
}

func publicIP(ip net.IP) bool {
	return !(ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsInterfaceLocalMulticast() || ip.IsMulticast() ||
		// Carrier-grade NAT (100.64.0.0/10) is not public either.
		(ip.To4() != nil && ip.To4()[0] == 100 && ip.To4()[1]&0xc0 == 64))
}

func checkURL(u *url.URL) error {
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("unsupported scheme %q", u.Scheme)
	}
	if u.Host == "" || u.User != nil {
		return errors.New("invalid URL")
	}
	return nil
}

// ParseURL validates a URL typed or pasted by a member.
func ParseURL(raw string) (*url.URL, error) {
	raw = strings.TrimSpace(raw)
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	if err := checkURL(u); err != nil {
		return nil, err
	}
	u.Fragment = ""
	return u, nil
}

// Response is a fetched body with what the snapshot records about it.
type Response struct {
	URL         string // after redirects
	Status      int
	ContentType string
	Body        []byte
}

// Get downloads a URL with the size, time and redirect limits.
func (f *Fetcher) Get(ctx context.Context, rawURL string) (*Response, error) {
	u, err := ParseURL(rawURL)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Accept-Language", "fr")
	resp, err := f.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, MaxBody+1))
	if err != nil {
		return nil, err
	}
	if len(body) > MaxBody {
		return nil, errors.New("response too large")
	}
	return &Response{URL: resp.Request.URL.String(), Status: resp.StatusCode, ContentType: resp.Header.Get("Content-Type"), Body: body}, nil
}
