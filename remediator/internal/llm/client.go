// Package llm 的金山云大模型 API 客户端。
//
// 调用：POST {LLM_API_URL}/chat/completions
// 鉴权：Authorization: Bearer ${LLM_API_KEY}（环境变量注入，占位符 __LLM_API_KEY__ sed 替换，绝不入库）
// 模型：glm-5.1（环境变量 LLM_MODEL 可覆盖）
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/sre-demo/remediator/internal/metrics"
	"github.com/sre-demo/remediator/internal/triage"
)

// Client 金山云大模型客户端。
type Client struct {
	apiURL string
	apiKey string
	model  string
	http   *http.Client
}

// New 从环境变量创建客户端。apiKey 缺失时返回 nil 行为的降级客户端（LLM 层失败不阻断分诊自愈）。
func New() *Client {
	return &Client{
		apiURL: getenv("LLM_API_URL", "https://kspmas.ksyun.com/v1"),
		apiKey: getenv("LLM_API_KEY", ""),
		model:  getenv("LLM_MODEL", "glm-5.1"),
		http:   &http.Client{Timeout: 30 * time.Second},
	}
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// chatRequest OpenAI 兼容的 chat completions 请求体（金山云大模型 API 兼容此格式）。
type chatRequest struct {
	Model       string    `json:"model"`
	Messages    []message `json:"messages"`
	Temperature float64   `json:"temperature"`
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Choices []struct {
		Message message `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// chat 调用大模型，返回文本响应。
func (c *Client) chat(ctx context.Context, prompt string) (string, error) {
	if c.apiKey == "" {
		return "", fmt.Errorf("LLM_API_KEY 未配置，LLM 推断降级跳过")
	}
	body, _ := json.Marshal(chatRequest{
		Model: c.model,
		Messages: []message{
			{Role: "user", Content: prompt},
		},
		Temperature: 0.3,
	})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, c.apiURL+"/chat/completions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("llm http: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("llm status %d: %s", resp.StatusCode, string(respBody))
	}
	var cr chatResponse
	if err := json.Unmarshal(respBody, &cr); err != nil {
		return "", fmt.Errorf("unmarshal llm response: %w", err)
	}
	if cr.Error != nil {
		return "", fmt.Errorf("llm api error: %s", cr.Error.Message)
	}
	if len(cr.Choices) == 0 {
		return "", fmt.Errorf("llm empty choices")
	}
	return cr.Choices[0].Message.Content, nil
}

// Infer 根因推断：组装 prompt → 调 LLM → 解析 JSON → 返回 LLMResult。
// 失败不阻断（返回 nil），分诊与规则引擎安全动作仍正常工作。
func (c *Client) Infer(ctx context.Context, event *triage.Event, logSummary string) *triage.LLMResult {
	if c.apiKey == "" {
		metrics.LLMInferences.WithLabelValues("skipped").Inc()
		return nil
	}
	start := time.Now()
	keywords := []string{event.AlertName, event.Severity, "burn"}
	if event.IsChaos {
		keywords = append(keywords, "chaos")
	}
	snippets := Retrieve(keywords, 2)
	prompt := buildRootCausePrompt(event, logSummary, snippets)

	raw, err := c.chat(ctx, prompt)
	metrics.LLMInferenceDuration.WithLabelValues().Observe(time.Since(start).Seconds())
	if err != nil {
		metrics.LLMInferences.WithLabelValues("failed").Inc()
		return nil
	}
	result := parseLLMResult(raw)
	if result == nil {
		metrics.LLMInferences.WithLabelValues("failed").Inc()
		return nil
	}
	result.RawResponse = raw
	metrics.LLMInferences.WithLabelValues("success").Inc()
	return result
}

// GenerateRCA 阶段四：生成 postmortem 草稿。
func (c *Client) GenerateRCA(ctx context.Context, event *triage.Event, actions []triage.RemediationLog) (string, error) {
	if c.apiKey == "" {
		return "", fmt.Errorf("LLM_API_KEY 未配置")
	}
	return c.chat(ctx, buildRCAPrompt(event, actions))
}

// parseLLMResult 从 LLM 文本响应提取 JSON（容忍前后多余文字）。
func parseLLMResult(raw string) *triage.LLMResult {
	// 提取首个 {...} JSON 块。
	re := regexp.MustCompile(`\{[\s\S]*\}`)
	match := re.FindString(strings.TrimSpace(raw))
	if match == "" {
		return nil
	}
	var r triage.LLMResult
	if err := json.Unmarshal([]byte(match), &r); err != nil {
		return nil
	}
	return &r
}
