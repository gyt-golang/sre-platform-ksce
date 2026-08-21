// Package v1 包含 SLO CRD 的 API 类型定义。
//
// SLO CR 是 SLO 规则的单一事实源：spec 映射 observability/slo-spec.yaml 结构，
// controller reconcile 时从 spec 派生 Prometheus recording rules（5 窗口）与 alert rules（Page/Ticket/Budget），
// 双写 PrometheusRule CRD + ConfigMap（裸 Prometheus 降级兼容）。
package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SLOSpec 定义 SLO 规则的期望状态，映射 slo-spec.yaml。
type SLOSpec struct {
	// Version 指定 SLO spec 格式版本，固定 "prometheus/v1"。
	Version string `json:"version"`
	// Service 是目标服务名，作为派生 recording rule 名前缀（如 ordersvc:request_rate5m）。
	Service string `json:"service"`
	// Labels 是全局 labels，合并进所有派生 alert 的 labels。
	Labels map[string]string `json:"labels,omitempty"`
	// SLOs 是 SLO 条目列表，每条独立派生 recording + alert rules。
	SLOs []SLOItem `json:"slos"`
}

// SLOItem 单个 SLO 定义。
type SLOItem struct {
	// Name 是 SLO 标识，作为 alert 的 slo label 值（如 availability-99.9）。
	Name string `json:"name"`
	// Objective 是 SLO 目标百分比（0-100），如 99.9。燃烧率分母 = 1 - objective/100。
	Objective float64 `json:"objective"`
	// Description 是 SLO 的人类可读说明。
	Description string `json:"description,omitempty"`
	// SLI 定义服务等级指标，events 或 raw 二选一。
	SLI SLISpec `json:"sli"`
	// Alerting 定义派生告警的名称、labels 与各档阈值。
	Alerting AlertingSpec `json:"alerting"`
}

// SLISpec 支持 events（error/total 双 query 计算错误率）或 raw（直接给 error_ratio query）。
type SLISpec struct {
	Events *EventsSLI `json:"events,omitempty"`
	Raw    *RawSLI    `json:"raw,omitempty"`
}

// EventsSLI 用 error_query / total_query 计算 error_ratio = error/total。
// 两个 query 都必须含 {{.window}} 占位符，controller 渲染成 5m/30m/1h/6h/1d。
type EventsSLI struct {
	// ErrorQuery 是错误事件 query，label 必须用 code=（不是 status=，metrics.go HTTPRequestsTotal label 是 code）。
	ErrorQuery string `json:"error_query"`
	// TotalQuery 是总事件 query。
	TotalQuery string `json:"total_query"`
}

// RawSLI 直接给出 error_ratio query（如延迟 SLO：1 - ≤阈值bucket/count）。
type RawSLI struct {
	// ErrorRatioQuery 必须含 {{.window}} 占位符。
	ErrorRatioQuery string `json:"error_ratio_query"`
}

// AlertingSpec 定义派生告警。
type AlertingSpec struct {
	// Name 是告警名前缀（如 OrdersvcHighErrorRate），controller 拼接 Page/Ticket 后缀。
	Name string `json:"name"`
	// Labels 是该 SLO 所有告警共用的 labels（slo/team）。
	Labels map[string]string `json:"labels,omitempty"`
	// PageAlert 定义 Page 级告警（立即介入），不配则不生成。
	PageAlert *AlertTier `json:"page_alert,omitempty"`
	// TicketAlert 定义 Ticket 级告警（工作时间处理），不配则不生成。
	TicketAlert *AlertTier `json:"ticket_alert,omitempty"`
}

// AlertTier 单档告警配置。
type AlertTier struct {
	// Disable 为 true 时不生成该档告警。
	Disable bool `json:"disable,omitempty"`
	// Labels 是该档额外 labels（如 severity: page/ticket）。
	Labels map[string]string `json:"labels,omitempty"`
	// BurnThreshold 是多窗口燃烧率阈值，Page 默认 14.4，Ticket 默认 6。
	BurnThreshold *float64 `json:"burnThreshold,omitempty"`
	// Windows 是多窗口组合，Page 默认 ["5m","1h"]，Ticket 默认 ["30m","6h"]。
	Windows []string `json:"windows,omitempty"`
	// For 是告警持续时间，Page 默认 "2m"，Ticket 默认 "15m"。
	For string `json:"for,omitempty"`
}

// SLOStatus 记录 reconcile 结果。
type SLOStatus struct {
	// Conditions 记录同步状态（RulesGenerated / PrometheusRuleSyncFailed）。
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	// GeneratedRules 列出派生的 rule 名，便于核对。
	GeneratedRules []string `json:"generatedRules,omitempty"`
	// LastReconcileAt 是上次 reconcile 时间。
	LastReconcileAt *metav1.Time `json:"lastReconcileAt,omitempty"`
	// PrometheusRuleName 是派生的 PrometheusRule CR 名（可能因 CRD 未装而未生成）。
	PrometheusRuleName string `json:"prometheusRuleName,omitempty"`
	// ConfigMapName 是降级写出的 ConfigMap 名（裸 Prometheus 加载）。
	ConfigMapName string `json:"configMapName,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=slo
// +kubebuilder:printcolumn:name="Service",type=string,JSONPath=`.spec.service`
// +kubebuilder:printcolumn:name="SLOs",type=integer,JSONPath=`.status.generatedRules`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// SLO 是 SLO 规则的 CRD 实例，由 SLO Operator reconcile 派生 Prometheus 规则。
type SLO struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec   SLOSpec   `json:"spec,omitempty"`
	Status SLOStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// SLOList 是 SLO 列表。
type SLOList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items []SLO `json:"items"`
}
