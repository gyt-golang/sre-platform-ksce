// Package loki 查询 Loki 日志，供 LLM 根因推断取日志上下文。
package loki

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client Loki HTTP API 客户端。
type Client struct {
	baseURL string
	http    *http.Client
}

func New(baseURL string) *Client {
	return &Client{baseURL: baseURL, http: &http.Client{Timeout: 15 * time.Second}}
}

// queryResult Loki log query 响应（stream 型）。
type queryResult struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string `json:"resultType"`
		Result     []struct {
			Values [][]any `json:"values"` // [[ts, "line"], ...]
		} `json:"result"`
	} `json:"data"`
}

// Tail 查询最近窗口的日志，返回拼接的日志摘要（截断防超长）。
// query 是 LogQL，如 `{namespace="sre-demo",container="ordersvc"} |= "error" | json`
func (c *Client) Tail(ctx context.Context, query string, limit int) (string, error) {
	u := fmt.Sprintf("%s/loki/api/v1/query?query=%s&limit=%d",
		c.baseURL, url.QueryEscape(query), limit)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("loki query: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var qr queryResult
	if err := json.Unmarshal(body, &qr); err != nil {
		return "", fmt.Errorf("unmarshal loki response: %w", err)
	}
	var lines []string
	for _, stream := range qr.Data.Result {
		for _, v := range stream.Values {
			if len(v) >= 2 {
				if line, ok := v[1].(string); ok {
					lines = append(lines, line)
				}
			}
		}
	}
	summary := strings.Join(lines, "\n")
	// 截断防 prompt 超长（LLM 上下文限制）。
	if len(summary) > 4000 {
		summary = summary[:4000] + "\n...(truncated)"
	}
	return summary, nil
}
