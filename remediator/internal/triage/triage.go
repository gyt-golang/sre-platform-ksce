// Package triage 实现告警智能分诊。
//
// 职责：Alertmanager webhook payload → 按 alertname/slo/severity 分组去重 → 关联 runbook →
// 评估错误预算消耗 → 定级（P0-P3）→ 生成幂等事件。
// 事件状态机：firing→active，resolved→resolved（复发用新 eventID）。
package triage

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

// Alert 是 Alertmanager webhook payload 里的单个告警。
type Alert struct {
	Status      string            `json:"status"`      // firing / resolved
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations"`
	StartsAt    time.Time         `json:"startsAt"`
	Fingerprint string            `json:"fingerprint"`
}

// WebhookPayload 是 Alertmanager webhook 请求体。
type WebhookPayload struct {
	Status           string            `json:"status"`
	GroupLabels      map[string]string `json:"groupLabels"`
	CommonLabels     map[string]string `json:"commonLabels"`
	CommonAnnotations map[string]string `json:"commonAnnotations"`
	Alerts           []Alert           `json:"alerts"`
	GroupKey         string            `json:"groupKey"`
}

// Tier 定级。
type Tier string

const (
	TierP0 Tier = "P0" // page 立即介入
	TierP1 Tier = "P1" // ticket + burn1h>6
	TierP2 Tier = "P2" // ticket
	TierP3 Tier = "P3" // info / chaos 演练
)

// Event 是分诊产出的事件。
type Event struct {
	ID            string            `json:"id"`            // 幂等 ID = hash(groupKey+startsAt)
	Tier          Tier              `json:"tier"`
	Status        string            `json:"status"`        // active / resolved
	AlertName     string            `json:"alertName"`
	SLO           string            `json:"slo"`
	Service       string            `json:"service"`
	Severity      string            `json:"severity"`
	IsChaos       bool              `json:"isChaos"`
	RunbookURL    string            `json:"runbookUrl"`
	DashboardURL  string            `json:"dashboardUrl"`
	Summary       string            `json:"summary"`
	BurnRate5m    float64           `json:"burnRate5m"`
	BurnRate1h    float64           `json:"burnRate1h"`
	BudgetImpact  string            `json:"budgetImpact"` // 错误预算消耗描述
	LLMResult     *LLMResult        `json:"llmResult,omitempty"`
	Remediations  []RemediationLog  `json:"remediations,omitempty"`
	CreatedAt     time.Time         `json:"createdAt"`
	ResolvedAt    *time.Time        `json:"resolvedAt,omitempty"`
}

// LLMResult 是 LLM 根因推断结果。
type LLMResult struct {
	RootCauseHypothesis string  `json:"rootCauseHypothesis"`
	Scenario            string  `json:"scenario"`     // A/B/C/D
	RecommendAction     string  `json:"recommendAction"`
	RecommendRollback   bool    `json:"recommendRollback"`
	Confidence          float64 `json:"confidence"`
	RawResponse         string  `json:"rawResponse,omitempty"`
}

// RemediationLog 记录一次修复动作。
type RemediationLog struct {
	Action    string    `json:"action"`     // fault_reset / scale_up / rollout_undo
	Result    string    `json:"result"`     // executed / skipped / failed
	Reason    string    `json:"reason"`
	Timestamp time.Time `json:"timestamp"`
}

// Store 事件存储（内存，定期 dump KS3 由调用方驱动）。
type Store struct {
	mu     sync.RWMutex
	events map[string]*Event // 按 ID 索引
}

func NewStore() *Store {
	return &Store{events: make(map[string]*Event)}
}

// List 返回所有事件（副本）。
func (s *Store) List() []*Event {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*Event, 0, len(s.events))
	for _, e := range s.events {
		out = append(out, e)
	}
	return out
}

// Get 按 ID 取事件。
func (s *Store) Get(id string) (*Event, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	e, ok := s.events[id]
	return e, ok
}

// Upsert 新建或更新事件。
func (s *Store) Upsert(e *Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events[e.ID] = e
}

// eventID 生成幂等事件 ID：hash(groupKey + 首告警 startsAt)。
// 同一 groupKey 的 firing/resolved 复用 ID；resolved 后复发用新 startsAt 生成新 ID。
func eventID(payload *WebhookPayload) string {
	starts := ""
	if len(payload.Alerts) > 0 {
		starts = payload.Alerts[0].StartsAt.Format(time.RFC3339Nano)
	}
	h := sha256.Sum256([]byte(payload.GroupKey + starts))
	return hex.EncodeToString(h[:])[:16]
}

// Triage 对 payload 分诊，返回事件（新建或更新）。
// burn5m/burn1h 由调用方查 Prometheus 后填入（promClient）。
func Triage(payload *WebhookPayload, burn5m, burn1h float64, store *Store) *Event {
	if len(payload.Alerts) == 0 {
		return nil
	}
	first := payload.Alerts[0]
	id := eventID(payload)

	severity := first.Labels["severity"]
	alertName := first.Labels["alertname"]
	slo := first.Labels["slo"]
	service := first.Labels["service"]
	_, isChaos := first.Labels["chaos"]

	tier := classify(severity, burn1h, isChaos)

	event := &Event{
		ID:           id,
		Tier:         tier,
		Status:       payload.Status, // firing→active, resolved→resolved
		AlertName:    alertName,
		SLO:          slo,
		Service:      service,
		Severity:     severity,
		IsChaos:      isChaos,
		RunbookURL:   first.Annotations["runbook_url"],
		DashboardURL: first.Annotations["dashboard_url"],
		Summary:      first.Annotations["summary"],
		BurnRate5m:   burn5m,
		BurnRate1h:   burn1h,
		BudgetImpact: budgetImpact(burn1h),
		CreatedAt:    time.Now(),
	}

	// 已存在则保留 LLMResult/Remediations，更新 status 与 burn。
	if existing, ok := store.Get(id); ok {
		event.LLMResult = existing.LLMResult
		event.Remediations = existing.Remediations
		event.CreatedAt = existing.CreatedAt
		if payload.Status == "resolved" {
			now := time.Now()
			event.ResolvedAt = &now
		}
	}
	store.Upsert(event)
	return event
}

// classify 定级。
func classify(severity string, burn1h float64, isChaos bool) Tier {
	if isChaos {
		return TierP3
	}
	if severity == "page" {
		return TierP0
	}
	if severity == "ticket" && burn1h > 6 {
		return TierP1
	}
	if severity == "ticket" {
		return TierP2
	}
	return TierP3
}

// budgetImpact 把 burn1h 转成人类可读的错误预算消耗描述。
func budgetImpact(burn1h float64) string {
	if burn1h <= 0 {
		return "无数据"
	}
	// burn_rate=1 表示按当前速率 30 天耗尽预算；burn1h=X 表示当前 1h 窗口消耗 X/720 的月预算。
	pct := burn1h / 720 * 100
	return fmt.Sprintf("当前 1h 窗口消耗月预算 %.2f%%（burn_rate1h=%.2f）", pct, burn1h)
}
