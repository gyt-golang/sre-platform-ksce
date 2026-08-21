// Package controller 的规则生成器：SLO CR → Prometheus recording/alert rules。
//
// 这是 Operator 的核心逻辑。从 SLO CR 的 spec 派生：
//   - recording rules：每个 SLO × 5 窗口，命名 ordersvc:{request_rate,error_rate,error_ratio,burn_rate}{window}
//     严格保住 slo-report.py 依赖的 4 个名：error_ratio1d / burn_rate1d / latency_p99_5m / request_rate1d
//   - alert rules：Page(多窗口燃烧率>14.4) / Ticket(>6) / Budget(1d>1)
//
// 关键修复：SLI query 强制用 code=（metrics.go HTTPRequestsTotal label 是 code，不是 status）。
package controller

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"text/template"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	intstr "k8s.io/apimachinery/pkg/util/intstr"

	v1 "github.com/sre-demo/slo-operator/api/v1"
)

// dur 把 "2m" 等字符串转成 monitoringv1.Duration 指针（monitoringv1.Duration 底层就是 string）。
func dur(s string) *monitoringv1.Duration {
	if s == "" {
		return nil
	}
	d := monitoringv1.Duration(s)
	return &d
}

// ruleGenerator 从 SLO CR 派生规则。
type ruleGenerator struct{}

func newRuleGenerator() *ruleGenerator { return &ruleGenerator{} }

// renderQuery 把 SLI query 的 {{.window}} 占位符替换为具体窗口。
func renderQuery(tmplStr, window string) (string, error) {
	t, err := template.New("sli").Parse(tmplStr)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, map[string]string{"window": window}); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// allowedErrorRatio 从 objective 计算允许错误率（燃烧率分母）。
// 99.9 → 0.001。
func allowedErrorRatio(objective float64) string {
	return strconv.FormatFloat(1.0-objective/100.0, 'g', -1, 64)
}

// buildRecordingRules 为每个 SLO × 5 窗口生成 recording rules。
// 保住 ordersvc:request_rate1d / error_ratio1d / burn_rate1d / latency_p99_5m。
func (g *ruleGenerator) buildRecordingRules(slo *v1.SLO) []monitoringv1.Rule {
	var rules []monitoringv1.Rule
	svc := slo.Spec.Service
	for _, item := range slo.Spec.SLOs {
		allowed := allowedErrorRatio(item.Objective)

		for _, w := range RecordingWindows {
			if item.SLI.Events != nil {
				errQ, _ := renderQuery(item.SLI.Events.ErrorQuery, w)
				totalQ, _ := renderQuery(item.SLI.Events.TotalQuery, w)
				// request_rate{w} —— 保住 request_rate1d
				rules = append(rules, monitoringv1.Rule{
					Record: svc + ":request_rate" + w,
					Expr:   intstr.FromString(totalQ),
				})
				// error_rate{w}
				rules = append(rules, monitoringv1.Rule{
					Record: svc + ":error_rate" + w,
					Expr:   intstr.FromString(errQ),
				})
				// error_ratio{w} = error_rate{w} / request_rate{w} —— 保住 error_ratio1d
				rules = append(rules, monitoringv1.Rule{
					Record: svc + ":error_ratio" + w,
					Expr:   intstr.FromString(fmt.Sprintf("%s:error_rate%s / %s:request_rate%s", svc, w, svc, w)),
				})
				// burn_rate{w} = error_ratio{w} / allowed —— 保住 burn_rate1d
				rules = append(rules, monitoringv1.Rule{
					Record: svc + ":burn_rate" + w,
					Expr:   intstr.FromString(fmt.Sprintf("%s:error_ratio%s / %s", svc, w, allowed)),
				})
			}
			if item.SLI.Raw != nil {
				rq, _ := renderQuery(item.SLI.Raw.ErrorRatioQuery, w)
				// raw 型用带 SLO 名的独立 recording（避免与 events 型 error_ratio 冲突）
				rules = append(rules, monitoringv1.Rule{
					Record: svc + ":error_ratio_" + sanitize(item.Name) + w,
					Expr:   intstr.FromString(rq),
				})
				rules = append(rules, monitoringv1.Rule{
					Record: svc + ":burn_rate_" + sanitize(item.Name) + w,
					Expr:   intstr.FromString(fmt.Sprintf("%s:error_ratio_%s%s / %s", svc, sanitize(item.Name), w, allowed)),
				})
			}
		}

		// 延迟 SLO 额外生成 latency_p99_5m（保住 slo-report.py 依赖）。
		// 当 raw SLI query 含 histogram_quantile，认为是延迟型 SLO。
		if item.SLI.Raw != nil && strings.Contains(item.SLI.Raw.ErrorRatioQuery, "histogram_quantile") {
			rules = append(rules, monitoringv1.Rule{
				Record: svc + ":latency_p99_5m",
				Expr: intstr.FromString(`histogram_quantile(0.99,
  sum by (path, le) (rate(ordersvc_http_request_duration_seconds_bucket[5m])))`),
			})
		}
	}
	return rules
}

// buildAlertRules 生成 Page / Ticket / Budget 三档告警。
func (g *ruleGenerator) buildAlertRules(slo *v1.SLO) []monitoringv1.Rule {
	var rules []monitoringv1.Rule
	svc := slo.Spec.Service
	for _, item := range slo.Spec.SLOs {
		if item.Alerting.PageAlert != nil && !item.Alerting.PageAlert.Disable {
			thr := defaultFloat(item.Alerting.PageAlert.BurnThreshold, DefaultPageBurnThreshold)
			ws := nonEmptyWindows(item.Alerting.PageAlert.Windows, []string{"5m", "1h"})
			forDur := nonEmptyStr(item.Alerting.PageAlert.For, DefaultPageFor)
			rules = append(rules, monitoringv1.Rule{
				Alert: item.Alerting.Name + "Page",
				Expr:  intstr.FromString(buildMultiWindowBurnExpr(svc, item, ws, thr)),
				For:   dur(forDur),
				Labels: mergeLabels(slo.Spec.Labels, item.Alerting.Labels,
					item.Alerting.PageAlert.Labels, map[string]string{"service": svc, "slo": item.Name}),
				Annotations: defaultAnnotations(item, "page", svc),
			})
		}
		if item.Alerting.TicketAlert != nil && !item.Alerting.TicketAlert.Disable {
			thr := defaultFloat(item.Alerting.TicketAlert.BurnThreshold, DefaultTicketBurnThreshold)
			ws := nonEmptyWindows(item.Alerting.TicketAlert.Windows, []string{"30m", "6h"})
			forDur := nonEmptyStr(item.Alerting.TicketAlert.For, DefaultTicketFor)
			rules = append(rules, monitoringv1.Rule{
				Alert: item.Alerting.Name + "Ticket",
				Expr:  intstr.FromString(buildMultiWindowBurnExpr(svc, item, ws, thr)),
				For:   dur(forDur),
				Labels: mergeLabels(slo.Spec.Labels, item.Alerting.Labels,
					item.Alerting.TicketAlert.Labels, map[string]string{"service": svc, "slo": item.Name}),
				Annotations: defaultAnnotations(item, "ticket", svc),
			})
		}
		// Budget：1d 燃烧率 >1 持续 30m，仅 events 型 availability SLO 生成，统一名 Exhausting。
		if item.SLI.Events != nil {
			rules = append(rules, monitoringv1.Rule{
				Alert: AlertNameBudget,
				Expr:  intstr.FromString(fmt.Sprintf("%s:burn_rate1d > %s", svc, strconv.FormatFloat(BudgetBurnThreshold, 'g', -1, 64))),
				For:   dur(BudgetFor),
				Labels: mergeLabels(slo.Spec.Labels, item.Alerting.Labels,
					map[string]string{"severity": "ticket"}, map[string]string{"service": svc, "slo": item.Name}),
				Annotations: defaultAnnotations(item, "budget", svc),
			})
		}
	}
	return rules
}

// buildMultiWindowBurnExpr 构造多窗口燃烧率表达式：burn_rate5m > thr and burn_rate1h > thr。
// events 型用 ordersvc:burn_rate{w}，raw 型用 ordersvc:burn_rate_{name}{w}。
func buildMultiWindowBurnExpr(svc string, item v1.SLOItem, windows []string, thr float64) string {
	thrStr := strconv.FormatFloat(thr, 'g', -1, 64)
	var parts []string
	for _, w := range windows {
		var metric string
		if item.SLI.Events != nil {
			metric = fmt.Sprintf("%s:burn_rate%s", svc, w)
		} else {
			metric = fmt.Sprintf("%s:burn_rate_%s%s", svc, sanitize(item.Name), w)
		}
		parts = append(parts, fmt.Sprintf("%s > %s", metric, thrStr))
	}
	return strings.Join(parts, "\nand ")
}

// mergeLabels 合并多源 labels（后者覆盖前者）。
func mergeLabels(sources ...map[string]string) map[string]string {
	out := map[string]string{}
	for _, src := range sources {
		for k, v := range src {
			out[k] = v
		}
	}
	return out
}

// defaultAnnotations 生成 runbook_url / dashboard_url / summary。
// runbook_url 指向 GitHub 仓库，dashboard_url 指向 Grafana NodePort。
func defaultAnnotations(item v1.SLOItem, tier, svc string) map[string]string {
	summary := fmt.Sprintf("%s %s SLO 异常，错误预算燃烧", svc, item.Name)
	if tier == "page" {
		summary = fmt.Sprintf("%s 可用性急剧恶化，错误预算高速燃烧", svc)
	} else if tier == "budget" {
		summary = fmt.Sprintf("%s 错误预算 1d 燃烧率 >1，按当前速率将耗尽月预算", svc)
	}
	return map[string]string{
		"runbook_url":   "https://raw.githubusercontent.com/gyt-golang/sre-platform-ksce/main/docs/runbook/ordersvc-high-error-rate.md",
		"dashboard_url": "http://10.0.0.182:30300/d/ordersvc-sre-slo/ordersvc-sre-slo",
		"summary":       summary,
	}
}

func sanitize(s string) string {
	r := strings.NewReplacer(".", "_", "-", "_", " ", "_")
	return r.Replace(s)
}

func defaultFloat(p *float64, def float64) float64 {
	if p != nil {
		return *p
	}
	return def
}

func nonEmptyWindows(w []string, def []string) []string {
	if len(w) > 0 {
		return w
	}
	return def
}

func nonEmptyStr(s, def string) string {
	if s != "" {
		return s
	}
	return def
}

// collectRuleNames 收集所有 rule 名（record + alert），用于 status。
func (g *ruleGenerator) collectRuleNames(rec, alert []monitoringv1.Rule) []string {
	var names []string
	for _, r := range rec {
		if r.Record != "" {
			names = append(names, r.Record)
		}
	}
	for _, r := range alert {
		if r.Alert != "" {
			names = append(names, r.Alert)
		}
	}
	return names
}
