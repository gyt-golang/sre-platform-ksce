// Package controller 实现 SLO CRD 的 reconciliation。
//
// Reconcile 流程：SLO CR → 派生 recording/alert rules → 双写 PrometheusRule CRD + ConfigMap → 更新 status。
// 双写原因：本项目用裸 Prometheus（非 Prometheus Operator），PrometheusRule CRD 可能未安装，
// ConfigMap 作为降级路径保证规则一定被 Prometheus 加载（prometheus.yaml rules volume 挂 ordersvc-slo-rules）。
package controller

import (
	"context"
	"fmt"
	"time"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	yaml "sigs.k8s.io/yaml"

	v1 "github.com/sre-demo/slo-operator/api/v1"
)

// SLOReconciler reconcile SLO CR。
type SLOReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	Generator *ruleGenerator
}

// +kubebuilder:rbac:groups=slo.sre-demo.io,resources=slos,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=slo.sre-demo.io,resources=slos/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=slo.sre-demo.io,resources=slos/finalizers,verbs=update
// +kubebuilder:rbac:groups=monitoring.coreos.com,resources=prometheusrules,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;patch;delete

func init() {
	// 注册 monitoringv1 到 scheme（PrometheusRule CRD 类型）。
	utilruntime.Must(monitoringv1.AddToScheme(runtime.NewScheme()))
}

// prometheusRuleKind 用于 monitoringv1 scheme 注册。
var prometheusRuleKind = &monitoringv1.PrometheusRule{}

// Reconcile 处理 SLO CR 变更。
func (r *SLOReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var slo v1.SLO
	if err := r.Get(ctx, req.NamespacedName, &slo); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	logger.Info("reconciling SLO", "service", slo.Spec.Service, "slos", len(slo.Spec.SLOs))

	if r.Generator == nil {
		r.Generator = newRuleGenerator()
	}

	// 1. 派生规则
	recRules := r.Generator.buildRecordingRules(&slo)
	alertRules := r.Generator.buildAlertRules(&slo)
	logger.Info("rules generated", "recording", len(recRules), "alert", len(alertRules))

	// 2. 写 PrometheusRule CRD（主路径，需 CRD 已安装；失败不阻断，记 condition）
	prName := slo.Spec.Service + "-slo"
	prErr := r.upsertPrometheusRule(ctx, &slo, prName, recRules, alertRules)

	// 3. 写 ConfigMap 到 observability ns，名 prometheus-rules（裸 Prometheus 的 rules volume 挂载点）。
	// 跨 ns：SLO CR 在 sre-demo，但 Prometheus 在 observability。Operator 需 ClusterRole 写该 ns ConfigMap。
	// 写入即替换原手写规则，Prometheus 重启/reload 后加载 Operator 派生的规则。
	cmNs := "observability"
	cmName := "prometheus-rules"
	cmErr := r.upsertConfigMap(ctx, &slo, cmNs, cmName, recRules, alertRules)

	// 4. 更新 status
	patch := client.MergeFrom(slo.DeepCopy())
	slo.Status.PrometheusRuleName = prName
	slo.Status.ConfigMapName = cmNs + "/" + cmName
	slo.Status.GeneratedRules = r.Generator.collectRuleNames(recRules, alertRules)
	now := metav1.NewTime(time.Now())
	slo.Status.LastReconcileAt = &now

	if prErr != nil {
		meta.SetStatusCondition(&slo.Status.Conditions, metav1.Condition{
			Type: "PrometheusRuleSyncFailed", Status: metav1.ConditionTrue,
			Reason: "CRDMissing", Message: fmt.Sprintf("PrometheusRule CRD not installed: %v", prErr),
		})
		logger.Info("PrometheusRule CRD sync skipped (likely not installed)", "err", prErr)
	} else {
		meta.SetStatusCondition(&slo.Status.Conditions, metav1.Condition{
			Type: "RulesGenerated", Status: metav1.ConditionTrue,
			Reason: "Reconciled", Message: fmt.Sprintf("%d recording + %d alert rules", len(recRules), len(alertRules)),
		})
	}
	if cmErr != nil {
		meta.SetStatusCondition(&slo.Status.Conditions, metav1.Condition{
			Type: "ConfigMapSyncFailed", Status: metav1.ConditionTrue,
			Reason: "ConfigMapError", Message: cmErr.Error(),
		})
	}
	_ = r.Status().Patch(ctx, &slo, patch)
	return ctrl.Result{}, nil
}

// upsertPrometheusRule 创建或更新 PrometheusRule CRD。
func (r *SLOReconciler) upsertPrometheusRule(ctx context.Context, slo *v1.SLO, name string, rec, alert []monitoringv1.Rule) error {
	pr := &monitoringv1.PrometheusRule{
		ObjectMeta: metav1.ObjectMeta{
			Name: name, Namespace: slo.Namespace, Labels: slo.Spec.Labels,
		},
		Spec: monitoringv1.PrometheusRuleSpec{
			Groups: []monitoringv1.RuleGroup{
				{Name: slo.Spec.Service + ".sli.recording", Interval: dur("30s"), Rules: rec},
				{Name: slo.Spec.Service + ".slo.alerts", Interval: dur("30s"), Rules: alert},
			},
		},
	}
	if err := controllerutil.SetControllerReference(slo, pr, r.Scheme); err != nil {
		return err
	}
	existing := &monitoringv1.PrometheusRule{}
	err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: slo.Namespace}, existing)
	if apierrors.IsNotFound(err) {
		return r.Create(ctx, pr)
	}
	if err != nil {
		return err
	}
	existing.Spec = pr.Spec
	existing.Labels = pr.Labels
	return r.Update(ctx, existing)
}

// upsertConfigMap 创建或更新 ConfigMap（目标 ns 与 name 由调用方指定），data.slo.yml 含规则。
// 裸 Prometheus 的 rule volume 挂 prometheus-rules ConfigMap，Operator 写入即替换原手写规则。
func (r *SLOReconciler) upsertConfigMap(ctx context.Context, slo *v1.SLO, ns, name string, rec, alert []monitoringv1.Rule) error {
	ruleYAML, err := buildRulesYAML(slo, rec, alert)
	if err != nil {
		return err
	}
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: name, Namespace: ns, Labels: slo.Spec.Labels,
		},
		Data: map[string]string{"slo.yml": ruleYAML},
	}
	if err := controllerutil.SetControllerReference(slo, cm, r.Scheme); err != nil {
		// 跨 ns 的 owner reference 不被 K8s 支持（owner 与 owned 须同 ns），跨 ns 时忽略 owner ref，
		// 改由 SLO CR 删除时 finalizer 或人工清理。记录但不阻断。
		_ = err
	}
	existing := &corev1.ConfigMap{}
	err = r.Get(ctx, types.NamespacedName{Name: name, Namespace: ns}, existing)
	if apierrors.IsNotFound(err) {
		return r.Create(ctx, cm)
	}
	if err != nil {
		return err
	}
	existing.Data = cm.Data
	existing.Labels = cm.Labels
	return r.Update(ctx, existing)
}

// buildRulesYAML 把规则序列化成 Prometheus rule_files YAML（groups: - name/rules）。
func buildRulesYAML(slo *v1.SLO, rec, alert []monitoringv1.Rule) (string, error) {
	groups := []map[string]any{
		{"name": slo.Spec.Service + ".sli.recording", "interval": "30s", "rules": ruleList(rec)},
		{"name": slo.Spec.Service + ".slo.alerts", "interval": "30s", "rules": ruleList(alert)},
	}
	doc := map[string]any{"groups": groups}
	out, err := yaml.Marshal(doc)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// ruleList 把 monitoringv1.Rule 转成可序列化的 map 列表（record/expr 或 alert/expr/for/labels/annotations）。
func ruleList(rules []monitoringv1.Rule) []map[string]any {
	var out []map[string]any
	for _, r := range rules {
		m := map[string]any{"expr": r.Expr.String()}
		if r.Record != "" {
			m["record"] = r.Record
		}
		if r.Alert != "" {
			m["alert"] = r.Alert
		}
		if r.For != nil {
			m["for"] = string(*r.For)
		}
		if len(r.Labels) > 0 {
			m["labels"] = r.Labels
		}
		if len(r.Annotations) > 0 {
			m["annotations"] = r.Annotations
		}
		out = append(out, m)
	}
	return out
}

// SetupWithManager 注册 controller。
func (r *SLOReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1.SLO{}).
		Owns(&monitoringv1.PrometheusRule{}).
		Owns(&corev1.ConfigMap{}).
		Complete(r)
}
