// Package v1 的 validating webhook：SLO CR 准入校验。
//
// 校验点：
//   - service 非空
//   - objective ∈ [0,100]
//   - SLI 必填 events 或 raw
//   - error_query 禁 status= 强制 code=（修复 metrics.go label 名 bug）
//   - query 必含 {{.window}} 占位符
//   - burnThreshold > 0
package v1

import (
	"strings"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// SetupWebhookWithManager 注册 validating webhook。ENABLE_WEBHOOK 环境变量为 false 时跳过（demo 免证书）。
func (r *SLO) SetupWebhookWithManager(mgr ctrl.Manager) error {
	if v := getEnv("ENABLE_WEBHOOK", "false"); v != "true" {
		return nil
	}
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

// +kubebuilder:webhook:path=/validate-slo-sre-demo-io-v1-slo,mutating=false,failurePolicy=fail,sideEffects=None,groups=slo.sre-demo.io,resources=slos,verbs=create;update,versions=v1,name=vslo.kb.io,admissionReviewVersions=v1

var _ webhook.Validator = &SLO{}

// ValidateCreate 创建时校验。
func (r *SLO) ValidateCreate() (admission.Warnings, error) {
	return r.validateSLO()
}

// ValidateUpdate 更新时校验。
func (r *SLO) ValidateUpdate(old runtime.Object) (admission.Warnings, error) {
	return r.validateSLO()
}

// ValidateDelete 删除不校验。
func (r *SLO) ValidateDelete() (admission.Warnings, error) {
	return nil, nil
}

func (r *SLO) validateSLO() (admission.Warnings, error) {
	var allErrs field.ErrorList
	if r.Spec.Service == "" {
		allErrs = append(allErrs, field.Invalid(field.NewPath("spec.service"), "", "service must not be empty"))
	}
	for i, item := range r.Spec.SLOs {
		fp := field.NewPath("spec.slos").Index(i)
		if item.Name == "" {
			allErrs = append(allErrs, field.Required(fp.Child("name"), "slo name required"))
		}
		if item.Objective < 0 || item.Objective > 100 {
			allErrs = append(allErrs, field.Invalid(fp.Child("objective"), item.Objective, "objective must be in [0,100]"))
		}
		if item.SLI.Events == nil && item.SLI.Raw == nil {
			allErrs = append(allErrs, field.Required(fp.Child("sli"), "must define events or raw SLI"))
		}
		if item.SLI.Events != nil {
			if item.SLI.Events.ErrorQuery == "" || item.SLI.Events.TotalQuery == "" {
				allErrs = append(allErrs, field.Required(fp.Child("sli.events"), "error_query and total_query required"))
			}
			// 修复 status→code bug：metrics.go HTTPRequestsTotal label 是 code，禁 status=。
			if hasStatusLabel(item.SLI.Events.ErrorQuery) || hasStatusLabel(item.SLI.Events.TotalQuery) {
				allErrs = append(allErrs, field.Invalid(fp.Child("sli.events"),
					"query contains status=", "must use code= not status= (metric label is code, see metrics.go HTTPRequestsTotal)"))
			}
			if !strings.Contains(item.SLI.Events.ErrorQuery, "{{.window}}") {
				allErrs = append(allErrs, field.Invalid(fp.Child("sli.events.error_query"),
					item.SLI.Events.ErrorQuery, "query must contain {{.window}} placeholder"))
			}
		}
		if item.SLI.Raw != nil {
			if !strings.Contains(item.SLI.Raw.ErrorRatioQuery, "{{.window}}") {
				allErrs = append(allErrs, field.Invalid(fp.Child("sli.raw.error_ratio_query"),
					item.SLI.Raw.ErrorRatioQuery, "query must contain {{.window}} placeholder"))
			}
		}
		if item.Alerting.PageAlert != nil && item.Alerting.PageAlert.BurnThreshold != nil &&
			*item.Alerting.PageAlert.BurnThreshold <= 0 {
			allErrs = append(allErrs, field.Invalid(fp.Child("alerting.page_alert.burnThreshold"),
				*item.Alerting.PageAlert.BurnThreshold, "must be > 0"))
		}
	}
	if len(allErrs) == 0 {
		return nil, nil
	}
	return nil, allErrs.ToAggregate()
}

// hasStatusLabel 检测 query 是否误用 status= label。
func hasStatusLabel(q string) bool {
	return strings.Contains(q, "status=") || strings.Contains(q, "status=~")
}

// getEnv 读环境变量，缺省返回 def。（避免在 api 包直接 import os，集中在此）
func getEnv(key, def string) string {
	return envLookup(key, def)
}
