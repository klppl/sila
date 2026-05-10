package fetcher

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"
)

const defaultTimeout = 30 * time.Second

type Client struct {
	hc      *http.Client
	etag    map[string]string
	lastMod map[string]string
}

func New() *Client {
	return &Client{
		hc: &http.Client{
			Timeout: defaultTimeout,
		},
		etag:    make(map[string]string),
		lastMod: make(map[string]string),
	}
}

var (
	ErrNotModified = errors.New("not modified")
	ErrInvalidURL  = errors.New("invalid or unsafe URL")
)

var privateCIDRs = []string{
	"127.0.0.0/8",
	"10.0.0.0/8",
	"172.16.0.0/12",
	"192.168.0.0/16",
	"169.254.0.0/16",
	"::1/128",
	"fc00::/7",
	"fe80::/10",
}

var privateNets []*net.IPNet

func init() {
	for _, cidr := range privateCIDRs {
		_, n, err := net.ParseCIDR(cidr)
		if err != nil {
			panic("bad cidr: " + cidr)
		}
		privateNets = append(privateNets, n)
	}
}

func ValidateURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("%w: parse: %v", ErrInvalidURL, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("%w: scheme must be http or https", ErrInvalidURL)
	}
	if u.Host == "" {
		return fmt.Errorf("%w: empty host", ErrInvalidURL)
	}

	host := u.Hostname()
	if host == "localhost" || host == "0.0.0.0" || host == "127.0.0.1" || host == "::1" {
		return fmt.Errorf("%w: blocked host", ErrInvalidURL)
	}

	if net.ParseIP(host) != nil {
		if isPrivateIP(net.ParseIP(host)) {
			return fmt.Errorf("%w: private IP not allowed", ErrInvalidURL)
		}
		return nil
	}

	ips, err := net.LookupIP(host)
	if err != nil {
		return fmt.Errorf("%w: dns lookup: %v", ErrInvalidURL, err)
	}
	for _, ip := range ips {
		if isPrivateIP(ip) {
			return fmt.Errorf("%w: resolves to private IP", ErrInvalidURL)
		}
	}
	return nil
}

func isPrivateIP(ip net.IP) bool {
	for _, n := range privateNets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

func (c *Client) Fetch(rawURL string) (string, error) {
	if err := ValidateURL(rawURL); err != nil {
		return "", err
	}

	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", "Sila/1.0 (blocklist aggregator)")

	if etag, ok := c.etag[rawURL]; ok {
		req.Header.Set("If-None-Match", etag)
	}
	if lastMod, ok := c.lastMod[rawURL]; ok {
		req.Header.Set("If-Modified-Since", lastMod)
	}

	resp, err := c.hc.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotModified {
		return "", ErrNotModified
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	if etag := resp.Header.Get("ETag"); etag != "" {
		c.etag[rawURL] = etag
	}
	if lastMod := resp.Header.Get("Last-Modified"); lastMod != "" {
		c.lastMod[rawURL] = lastMod
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 50<<20))
	if err != nil {
		return "", fmt.Errorf("read body: %w", err)
	}
	return string(body), nil
}
