// Package prom 查询 Prometheus，供分诊评估错误预算消耗与 LLM 推断取上下文。
package prom

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// Client Prometheus HTTP API 客户端。
type Client struct {
	baseURL string
	http    *http.Client
}

func New(baseURL string) *Client {
	return &Client{baseURL: baseURL, http: &http.Client{Timeout: 10 * time.Second}}
}

// QueryResult Prometheus instant query 响应。
type QueryResult struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string `json:"resultType"`
		Result     []struct {
			Value []any `json:"value"` // [timestamp, "value"]
		} `json:"result"`
	} `json:"data"`
}

// Query 执行 instant query，返回首个结果的 float 值。
func (c *Client) Query(ctx context.Context, promql string) (float64, error) {
	u := c.baseURL + "/api/v1/query?query=" + url.QueryEscape(promql)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, fmt.Errorf("prom query: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var qr QueryResult
	if err := json.Unmarshal(body, &qr); err != nil {
		return 0, fmt.Errorf("unmarshal prom response: %w", err)
	}
	if len(qr.Data.Result) == 0 {
		return 0, nil // 无数据返回 0（如无流量）
	}
	if len(qr.Data.Result[0].Value) < 2 {
		return 0, nil
	}
	valStr, ok := qr.Data.Result[0].Value[1].(string)
	if !ok {
		return 0, nil
	}
	return strconv.ParseFloat(valStr, 64)
}

// BurnRates 查 ordersvc:burn_rate5m 与 burn_rate1h，供分诊定级。
func (c *Client) BurnRates(ctx context.Context, service string) (burn5m, burn1h float64, err error) {
	burn5m, err = c.Query(ctx, service+":burn_rate5m")
	if err != nil {
		return 0, 0, err
	}
	burn1h, err = c.Query(ctx, service+":burn_rate1h")
	return burn5m, burn1h, err
}
