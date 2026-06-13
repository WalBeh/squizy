// Package client speaks the OpenAI-compatible HTTP API: /v1/models and
// streaming /v1/chat/completions. All timing is captured client-side.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Client targets one base URL (server root including /v1).
type Client struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

// New builds a client. baseURL should include the /v1 suffix, e.g.
// http://localhost:8000/v1. Per-request timeouts come from the context, so the
// underlying http.Client has no timeout of its own.
func New(baseURL, apiKey string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		http:    &http.Client{},
	}
}

func (c *Client) newRequest(ctx context.Context, method, path string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return nil, err
	}
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req, nil
}

// ListModels returns the models advertised by /v1/models.
func (c *Client) ListModels(ctx context.Context) ([]Model, error) {
	req, err := c.newRequest(ctx, http.MethodGet, "/models", nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return nil, fmt.Errorf("models: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	var mr modelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&mr); err != nil {
		return nil, fmt.Errorf("decode models: %w", err)
	}
	return mr.Data, nil
}

// ChatParams is one streaming chat request.
type ChatParams struct {
	Model       string
	Messages    []Message
	MaxTokens   int
	Temperature *float64
}

func (c *Client) buildBody(p ChatParams) ([]byte, error) {
	r := chatRequest{
		Model:       p.Model,
		Messages:    p.Messages,
		Stream:      true,
		StreamOpts:  &streamOpts{IncludeUsage: true},
		MaxTokens:   p.MaxTokens,
		Temperature: p.Temperature,
	}
	return json.Marshal(r)
}

func (c *Client) openStream(ctx context.Context, p ChatParams) (*http.Response, error) {
	body, err := c.buildBody(p)
	if err != nil {
		return nil, err
	}
	req, err := c.newRequest(ctx, http.MethodPost, "/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "text/event-stream")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		resp.Body.Close()
		return nil, fmt.Errorf("chat: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	return resp, nil
}
