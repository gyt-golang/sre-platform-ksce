// Package controller 的窗口常量与 {{.window}} 模板渲染。
package controller

// RecordingWindows 是派生 recording rules 的固定 5 窗口。
// 与现有 prometheus.yaml 对齐，slo-report.py 依赖其中的 1d 派生 error_ratio1d/burn_rate1d/request_rate1d。
var RecordingWindows = []string{"5m", "30m", "1h", "6h", "1d"}

// 默认告警参数（与现有 prometheus.yaml 对齐）。
const (
	DefaultPageBurnThreshold   = 14.4 // 1h 消耗 2% 月预算
	DefaultTicketBurnThreshold = 6.0
	DefaultPageFor             = "2m"
	DefaultTicketFor           = "15m"
	BudgetBurnThreshold        = 1.0 // 1d 燃烧率 >1 表示按当前速率会耗尽预算
	BudgetFor                  = "30m"
)

// AlertNameBudget 统一的预算耗尽告警名。
// 修复历史不一致：ConfigMap 版叫 Exhausting(for 1h)，CRD 版叫 AtRisk(for 30m)，统一为 Exhausting for 30m。
const AlertNameBudget = "OrdersvcErrorBudgetExhausting"
