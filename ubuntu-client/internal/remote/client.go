package remote

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/frankichen/fydeRhythm/ubuntu-client/internal/config"
	"github.com/frankichen/fydeRhythm/ubuntu-client/internal/model"
)

type Client struct {
	baseURL  string
	token    string
	deviceID string
	http     *http.Client
}

func New(cfg config.Config) *Client {
	return &Client{
		baseURL:  strings.TrimRight(cfg.ServerURL, "/"),
		token:    cfg.APIToken,
		deviceID: cfg.DeviceID,
		http:     &http.Client{Timeout: 20 * time.Second},
	}
}

func (c *Client) Health(ctx context.Context) error {
	var result map[string]any
	return c.do(ctx, http.MethodGet, "/healthz", nil, &result, false)
}

func (c *Client) Snapshot(ctx context.Context) (model.Snapshot, error) {
	var result model.Snapshot
	err := c.do(ctx, http.MethodGet, "/api/v1/sync/snapshot", nil, &result, true)
	return result, err
}

func (c *Client) Pull(ctx context.Context, since int64, limit int) (model.PullResult, error) {
	q := url.Values{}
	q.Set("since", strconv.FormatInt(since, 10))
	q.Set("limit", strconv.Itoa(limit))
	var result model.PullResult
	err := c.do(ctx, http.MethodGet, "/api/v1/sync/pull?"+q.Encode(), nil, &result, true)
	return result, err
}

func (c *Client) Push(ctx context.Context, req model.PushRequest) (model.PushResult, error) {
	var result model.PushResult
	err := c.do(ctx, http.MethodPost, "/api/v1/sync/push", req, &result, true)
	return result, err
}

func (c *Client) do(ctx context.Context, method, path string, body any, out any, auth bool) error {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if auth {
		req.Header.Set("Authorization", "Bearer "+c.token)
		req.Header.Set("X-Device-ID", c.deviceID)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		message := strings.TrimSpace(string(data))
		if len(message) > 500 {
			message = message[:500]
		}
		return fmt.Errorf("服务器返回 HTTP %d: %s", resp.StatusCode, message)
	}
	if out != nil && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			return fmt.Errorf("解析服务器响应: %w", err)
		}
	}
	return nil
}
