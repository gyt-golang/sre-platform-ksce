// Package remediate 实现告警自动修复规则引擎。
//
// 编码 docs/runbook/ordersvc-high-error-rate.md 的诊断决策树：
//   场景A（下游超时/故障注入残留）→ fault_reset 调 /admin/fault 复位（安全）
//   场景B（Pod OOM/资源不足）    → scale_up 扩容（安全）
//   场景C（流量突增延迟告警）      → scale_up 扩容（安全）
//   场景D（极端燃烧）             → rollout_undo 回滚（危险，需 LLM 双确认）
//
// 安全护栏：cooldown 去重、危险动作 LLM+规则双确认、chaos 告警只分诊不修复、动作白名单、scale 上限。
package remediate

import (
	"github.com/sre-demo/remediator/internal/triage"
)

// ActionType 动作类型白名单。
type ActionType string

const (
	ActionFaultReset  ActionType = "fault_reset"  // 调 ordersvc /admin/fault 复位故障注入
	ActionScaleUp     ActionType = "scale_up"      // kubectl scale 扩容
	ActionRolloutUndo ActionType = "rollout_undo"  // argo rollouts abort 回滚（危险）
)

// Rule 一条修复规则，编码 runbook 决策树。
type Rule struct {
	ID            string     // 规则 ID（cooldown key 的一部分）
	AlertName     string     // 匹配的 alertname（空=任意）
	BurnThreshold float64    // burn5m ≥ 此值才触发（降序匹配取首）
	IsDangerous   bool       // 危险动作需 LLM RecommendRollback && Confidence≥0.7 双确认
	Action        ActionType // 执行的动作
	CooldownSec   int        // 冷却期，同事件同规则期内不重复
	ScaleTo       int        // scale_up 目标副本数
	Description   string
}

// Rules 规则表（按 BurnThreshold 降序，engine 取首个匹配）。
// 与 runbook 场景 A/B/C/D 对应。
var Rules = []Rule{
	{
		ID: "R-D1", AlertName: "OrdersvcHighErrorRatePage",
		BurnThreshold: 50.0, IsDangerous: true, Action: ActionRolloutUndo,
		CooldownSec: 1800, Description: "场景D：极端燃烧（burn5m≥50），疑似坏版本上线，回滚到 stable",
	},
	{
		ID: "R-B1", AlertName: "OrdersvcHighErrorRatePage",
		BurnThreshold: 30.0, IsDangerous: false, Action: ActionScaleUp,
		CooldownSec: 600, ScaleTo: 6, Description: "场景B：持续高错误率疑似资源不足/OOM，扩容到 6 副本",
	},
	{
		ID: "R-A1", AlertName: "OrdersvcHighErrorRatePage",
		BurnThreshold: 14.4, IsDangerous: false, Action: ActionFaultReset,
		CooldownSec: 300, Description: "场景A：错误预算高速燃烧，先复位故障注入（/admin/fault fail=0）",
	},
	{
		ID: "R-C1", AlertName: "OrdersvcHighLatencyP99Ticket",
		BurnThreshold: 0, IsDangerous: false, Action: ActionScaleUp,
		CooldownSec: 600, ScaleTo: 8, Description: "场景C：P99 延迟告警，流量突增，扩容到 8 副本",
	},
}

// MaxScaleReplicas scale 动作上限，防止无限扩容。
const MaxScaleReplicas = 20

// Match 按 alertname + burn5m 匹配首个规则（降序）。
func Match(alertName string, burn5m float64) *Rule {
	for i := range Rules {
		r := &Rules[i]
		if r.AlertName != "" && r.AlertName != alertName {
			continue
		}
		if burn5m >= r.BurnThreshold {
			return r
		}
	}
	return nil
}

// Engine 修复引擎。
type Engine struct {
	Cooldown  *Cooldown
	Executor  ActionExecutor
	DryRun    bool // true 时危险动作只记录不执行
}

// ActionExecutor 执行具体动作的接口（便于 mock 测试）。
type ActionExecutor interface {
	FaultReset() (string, error)                              // 调 /admin/fault 复位，返回响应
	ScaleUp(replicas int) (string, error)                     // kubectl scale，返回结果
	RolloutUndo() (string, error)                             // argo rollouts abort，返回结果
}

// Evaluate 对事件评估并执行修复。
// 返回 RemediationLog（含 result: executed/skipped/failed/dry_run）。
func (e *Engine) Evaluate(event *triage.Event) triage.RemediationLog {
	log := triage.RemediationLog{Timestamp: event.CreatedAt}

	// 安全护栏1：chaos 演练告警只分诊不修复。
	if event.IsChaos {
		log.Action = "none"
		log.Result = "skipped"
		log.Reason = "chaos 演练告警，只分诊不自动修复"
		return log
	}

	rule := Match(event.AlertName, event.BurnRate5m)
	if rule == nil {
		log.Action = "none"
		log.Result = "skipped"
		log.Reason = "无匹配修复规则"
		return log
	}
	log.Action = string(rule.Action)

	// 安全护栏2：cooldown 去重。
	if e.Cooldown.InCooldown(event.ID, rule.ID, rule.CooldownSec) {
		log.Result = "skipped"
		log.Reason = "cooldown 期内，跳过重复执行"
		return log
	}

	// 安全护栏3：危险动作需 LLM 双确认。
	if rule.IsDangerous {
		if event.LLMResult == nil {
			log.Result = "skipped"
			log.Reason = "危险动作但 LLM 未推断，跳过"
			return log
		}
		if !event.LLMResult.RecommendRollback || event.LLMResult.Confidence < 0.7 {
			log.Result = "skipped"
			log.Reason = "危险动作未获 LLM 双确认（RecommendRollback && Confidence≥0.7），跳过"
			return log
		}
		if e.DryRun {
			log.Result = "dry_run"
			log.Reason = "DryRun 模式，危险动作仅记录不执行"
			return log
		}
	}

	// 执行动作。
	e.Cooldown.Mark(event.ID, rule.ID)
	replicas := rule.ScaleTo
	if replicas > MaxScaleReplicas {
		replicas = MaxScaleReplicas
	}
	var msg string
	var err error
	switch rule.Action {
	case ActionFaultReset:
		msg, err = e.Executor.FaultReset()
	case ActionScaleUp:
		msg, err = e.Executor.ScaleUp(replicas)
	case ActionRolloutUndo:
		msg, err = e.Executor.RolloutUndo()
	}
	if err != nil {
		log.Result = "failed"
		log.Reason = err.Error()
	} else {
		log.Result = "executed"
		log.Reason = msg
	}
	return log
}
