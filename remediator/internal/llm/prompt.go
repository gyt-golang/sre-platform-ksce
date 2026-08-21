// Package llm 的 prompt 构造。
package llm

import (
	"fmt"
	"strings"

	"github.com/sre-demo/remediator/internal/triage"
)

// buildRootCausePrompt 构造根因推断 prompt：告警信息 + Prom 指标 + Loki 日志 + RAG runbook 知识。
// 要求 LLM 输出 JSON（rootCauseHypothesis/scenario/recommendAction/recommendRollback/confidence）。
func buildRootCausePrompt(event *triage.Event, logSummary string, snippets []RunbookSnippet) string {
	var kb strings.Builder
	for _, s := range snippets {
		kb.WriteString(fmt.Sprintf("### %s\n%s\n\n", s.Title, s.Content))
	}
	return fmt.Sprintf(`你是 SRE 根因分析专家。根据以下告警与上下文推断根因并给出处置建议。

## 告警信息
- 告警名：%s
- SLO：%s
- 严重级别：%s
- 错误预算燃烧率：burn_rate5m=%.2f, burn_rate1h=%.2f
- 预算影响：%s
- 是否混沌演练：%v

## 关联指标
- error_ratio1d: (查 Prometheus)
- latency_p99_5m: (查 Prometheus)
- request_rate1d: (查 Prometheus)

## 相关日志摘要（Loki）
%s

## Runbook 知识库（RAG 检索）
%s

## 任务
推断最可能的根因（scenario: A=故障注入残留/下游超时, B=OOM/资源不足, C=流量突增, D=坏版本上线），
给出处置建议。判断是否需要回滚（recommendRollback=true 仅当 scenario=D 且 burn_rate5m>=50）。

严格输出 JSON，不要其他文字：
{"rootCauseHypothesis":"<根因假设>","scenario":"<A|B|C|D>","recommendAction":"<处置建议>","recommendRollback":<true|false>,"confidence":<0-1的置信度>}`,
		event.AlertName, event.SLO, event.Severity,
		event.BurnRate5m, event.BurnRate1h, event.BudgetImpact, event.IsChaos,
		logSummary, kb.String())
}

// buildRCAPrompt 阶段四进化功能：事件 resolved 后生成 postmortem 草稿。
func buildRCAPrompt(event *triage.Event, actions []triage.RemediationLog) string {
	var acts strings.Builder
	for _, a := range actions {
		acts.WriteString(fmt.Sprintf("- %s: %s (%s)\n", a.Action, a.Result, a.Reason))
	}
	llmRoot := "无"
	if event.LLMResult != nil {
		llmRoot = event.LLMResult.RootCauseHypothesis
	}
	return fmt.Sprintf(`你是 SRE 复盘专家。根据以下故障事件生成 postmortem 草稿（无责复盘）。

## 事件
- 告警：%s / SLO %s / 级别 %s
- 燃烧率：5m=%.2f 1h=%.2f
- LLM 根因推断：%s
- 修复动作：
%s

## 输出 Markdown（按 postmortem/template.md 结构）
# Postmortem: %s 自动生成
## 概要
<2-3 句>
## 影响
<错误预算消耗、持续时间>
## 根因
<根因>
## 时间线
<关键节点>
## 行动项
- [ ] <至少 1 个可执行行动项>
`,
		event.AlertName, event.SLO, event.Severity,
		event.BurnRate5m, event.BurnRate1h, llmRoot, acts.String(), event.ID)
}
