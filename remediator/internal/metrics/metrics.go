// Package metrics 定义 remediator 的 Prometheus 指标。
//
// 指标分四类，对应分诊→修复→LLM 推断全链路：
//   - alerts_received_total：收到 Alertmanager webhook 的告警数（按 severity）
//   - events_created_total：分诊生成的事件数（按 tier P0-P3）
//   - remediation_actions_total：自动修复动作执行数（按 action_type/result）
//   - llm_inferences_total / llm_inference_duration_seconds：LLM 根因推断调用数与延迟
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// AlertsReceived 收到的告警数。
	AlertsReceived = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "remediator_alerts_received_total",
		Help: "Alerts received from Alertmanager webhook, by severity.",
	}, []string{"severity"})

	// EventsCreated 分诊生成的事件数。
	EventsCreated = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "remediator_events_created_total",
		Help: "Triage events created, by tier (P0-P3).",
	}, []string{"tier"})

	// RemediationActions 自动修复动作执行数。
	RemediationActions = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "remediator_remediation_actions_total",
		Help: "Remediation actions executed, by action_type and result.",
	}, []string{"action_type", "result"})

	// LLMInferences LLM 根因推断调用数。
	LLMInferences = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "remediator_llm_inferences_total",
		Help: "LLM root-cause inferences, by result (success/failed/skipped).",
	}, []string{"result"})

	// LLMInferenceDuration LLM 推断延迟。
	LLMInferenceDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "remediator_llm_inference_duration_seconds",
		Help:    "LLM inference latency.",
		Buckets: []float64{0.5, 1, 2, 5, 10, 20, 30},
	}, []string{})
)
