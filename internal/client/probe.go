package client

import (
	"context"
	"io"
	"net/http"
	"net/http/httptrace"
	"sort"
	"strings"
	"time"
)

// NetBaseline is the result of probing the endpoint's transport latency — the
// network floor that sits underneath every latency the load test reports.
type NetBaseline struct {
	Path      string    // endpoint actually probed (/health or /v1/models)
	Reachable bool      // at least one probe got a response
	Status    int       // HTTP status of the last probe
	ConnectMs float64   // cold TCP connect time (first probe)
	TTFBms    []float64 // per-probe time-to-first-byte (transport round trip)
}

// TTFBStats returns min / p50 / p90 of the measured round trips.
func (n *NetBaseline) TTFBStats() (min, p50, p90 float64) {
	if len(n.TTFBms) == 0 {
		return 0, 0, 0
	}
	s := append([]float64(nil), n.TTFBms...)
	sort.Float64s(s)
	pick := func(p float64) float64 {
		i := int(p * float64(len(s)))
		if i >= len(s) {
			i = len(s) - 1
		}
		return s[i]
	}
	return s[0], pick(0.50), pick(0.90)
}

// Probe measures transport latency with n requests. It prefers the lightweight
// /health endpoint (at the server root, outside /v1) and falls back to
// /v1/models on a 404. Connection reuse means only the first probe pays TCP
// connect; the rest measure the warm round trip a real request sees. Path
// detection is folded into the first probe so the cold connect time is captured
// honestly (no separate pre-check consuming the dial).
func (c *Client) Probe(ctx context.Context, n int) *NetBaseline {
	if n < 1 {
		n = 1
	}
	url, path := c.healthURL()
	b := &NetBaseline{Path: path}
	var connectMs float64
	switched := false

	for i := 0; i < n; i++ {
		if ctx.Err() != nil {
			break
		}
		connect, ttfb, status, err := c.probeOnce(ctx, url)
		if err != nil {
			continue
		}
		// /health absent — fall back to /models once and restart the probes.
		if status == http.StatusNotFound && !switched && path == "/health" {
			switched = true
			url = c.baseURL + "/models"
			path = "/v1/models"
			b.Path = path
			i = -1
			continue
		}
		b.Reachable = true
		b.Status = status
		b.TTFBms = append(b.TTFBms, float64(ttfb.Microseconds())/1000)
		if connect > 0 && connectMs == 0 {
			connectMs = float64(connect.Microseconds()) / 1000
		}
	}
	b.ConnectMs = connectMs
	return b
}

// probeOnce issues one GET and returns its TCP-connect duration (0 if the
// connection was reused), time-to-first-byte, and HTTP status.
func (c *Client) probeOnce(ctx context.Context, url string) (connect, ttfb time.Duration, status int, err error) {
	start := time.Now()
	trace := &httptrace.ClientTrace{
		ConnectDone: func(_, _ string, _ error) {
			if connect == 0 {
				connect = time.Since(start)
			}
		},
	}
	req, err := http.NewRequestWithContext(httptrace.WithClientTrace(ctx, trace), http.MethodGet, url, nil)
	if err != nil {
		return 0, 0, 0, err
	}
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	resp, err := c.http.Do(req)
	ttfb = time.Since(start)
	if err != nil {
		return 0, 0, 0, err
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	return connect, ttfb, resp.StatusCode, nil
}

// healthURL returns the server-root /health URL (vLLM/omlx expose it outside /v1).
func (c *Client) healthURL() (url, path string) {
	root := strings.TrimSuffix(c.baseURL, "/v1")
	return root + "/health", "/health"
}
