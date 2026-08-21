// Package llm 调用金山云大模型 API 做根因推断 + RAG 检索 runbook 知识。
//
// 安全：LLM 只产出根因假设与处置建议（只读），不直接执行危险动作；
// 危险动作（rollout undo）由规则引擎在 LLM RecommendRollback && Confidence≥0.7 双确认后才执行。
package llm

import (
	"os"
	"strings"
)

// RunbookSnippet RAG 检索的 runbook 片段。
type RunbookSnippet struct {
	Title   string
	Content string
}

// runbookKB 是内置的 runbook 知识库（关键词→片段）。
// demo 用关键词检索；生产可接金山云向量服务做语义检索。
var runbookKB = []RunbookSnippet{
	{
		Title: "场景A：故障注入残留/下游超时",
		Content: `告警 OrdersvcHighErrorRatePage 触发，若 burn_rate5m 在 14.4-30 之间，
可能是故障注入未复位或下游服务超时。处置：调 ordersvc /admin/fault?fail=0&latency=0 复位故障注入，
观察 burn_rate5m 是否回落。恢复标准：burn_rate5m<1 持续 10 分钟。`,
	},
	{
		Title: "场景B：Pod OOM/资源不足",
		Content: `若 burn_rate5m≥30 且 Pod 出现 OOMKilled/重启，是资源不足。
处置：kubectl scale deploy/ordersvc 扩容（上限 20），或扩 HPA maxReplicas。
评估是否需调 resources limits。`,
	},
	{
		Title: "场景C：流量突增延迟告警",
		Content: `OrdersvcHighLatencyP99Ticket 触发，P99>500ms，通常是流量突增。
处置：扩容到 8 副本，查 HPA 是否已达 maxReplicas，评估限流降级。`,
	},
	{
		Title: "场景D：坏版本上线（极端燃烧）",
		Content: `若 burn_rate5m≥50，错误率极高且无外部故障注入，疑似坏版本上线。
处置：argo rollouts abort 回滚到上一个 stable ReplicaSet。
需 LLM 根因推断确认（RecommendRollback=true, Confidence≥0.7）后由规则引擎执行。`,
	},
}

// Retrieve 按告警特征关键词检索 runbook 片段，返回 top-K 拼接。
// keywords 来自告警 alertname/severity/burn_rate/日志摘要。
func Retrieve(keywords []string, topK int) []RunbookSnippet {
	if topK <= 0 {
		topK = 2
	}
	type scored struct {
		snippet RunbookSnippet
		score   int
	}
	var ss []scored
	for _, s := range runbookKB {
		score := 0
		for _, kw := range keywords {
			kw = strings.ToLower(kw)
			if kw == "" {
				continue
			}
			if strings.Contains(strings.ToLower(s.Title), kw) || strings.Contains(strings.ToLower(s.Content), kw) {
				score++
			}
		}
		if score > 0 {
			ss = append(ss, scored{s, score})
		}
	}
	// 简单按 score 降序取 topK。
	for i := 0; i < len(ss)-1; i++ {
		for j := i + 1; j < len(ss); j++ {
			if ss[j].score > ss[i].score {
				ss[i], ss[j] = ss[j], ss[i]
			}
		}
	}
	var out []RunbookSnippet
	for i := 0; i < topK && i < len(ss); i++ {
		out = append(out, ss[i].snippet)
	}
	// 无匹配时返回全部（兜底给 LLM 完整知识）。
	if len(out) == 0 {
		return runbookKB
	}
	return out
}

// LoadRunbookFromDisk 从 docs/runbook 目录加载额外知识（可选，文件存在时增强 RAG）。
func LoadRunbookFromDisk(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		data, err := os.ReadFile(dir + "/" + e.Name())
		if err != nil {
			continue
		}
		runbookKB = append(runbookKB, RunbookSnippet{
			Title:   e.Name(),
			Content: string(data),
		})
	}
}
