// Package handler 实现 remediator 的 HTTP 接口。
//
// 路由：
//   POST /webhook  —— Alertmanager webhook 接收点，同步分诊立即返回 200，异步处理事件
//   GET  /events   —— 查询分诊事件列表（JSON）
//   GET  /healthz  —— 健康检查
//   GET  /readyz   —— 就绪检查
package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/sre-demo/remediator/internal/k8s"
	"github.com/sre-demo/remediator/internal/llm"
	"github.com/sre-demo/remediator/internal/loki"
	"github.com/sre-demo/remediator/internal/metrics"
	"github.com/sre-demo/remediator/internal/prom"
	"github.com/sre-demo/remediator/internal/remediate"
	"github.com/sre-demo/remediator/internal/triage"
)

// Handler 持有所有依赖。
type Handler struct {
	store      *triage.Store
	promClient *prom.Client
	lokiClient *loki.Client
	llmClient  *llm.Client
	k8sClient  *k8s.Client
	engine     *remediate.Engine
	service    string // 目标服务名（ordersvc）
	deployName string // 目标 deployment 名
}

// New 创建 handler。k8sClient 在集群内才可用，非集群内传 nil（修复动作降级跳过）。
func New(service, deployName string, pc *prom.Client, lc *loki.Client, llmC *llm.Client, kc *k8s.Client, dryRun bool) *Handler {
	h := &Handler{
		store: triage.NewStore(), promClient: pc, lokiClient: lc,
		llmClient: llmC, k8sClient: kc, service: service, deployName: deployName,
	}
	exec := &kubeExecutor{client: kc, deployName: deployName}
	h.engine = &remediate.Engine{
		Cooldown: remediate.NewCooldown(),
		Executor: exec,
		DryRun:   dryRun,
	}
	return h
}

// Routes 注册路由。
func (h *Handler) Routes(mux *http.ServeMux) {
	mux.HandleFunc("/webhook", h.handleWebhook)
	mux.HandleFunc("/events", h.handleEvents)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200); _, _ = w.Write([]byte("ok")) })
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200); _, _ = w.Write([]byte("ok")) })
	mux.Handle("/metrics", promhttp.Handler())
}

// handleWebhook 接收 Alertmanager webhook。
func (h *Handler) handleWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var payload triage.WebhookPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "bad payload: "+err.Error(), http.StatusBadRequest)
		return
	}
	// 统计收到的告警。
	for _, a := range payload.Alerts {
		metrics.AlertsReceived.WithLabelValues(a.Labels["severity"]).Inc()
	}

	// 同步分诊（查 burn_rate），立即返回 200，避免 Alertmanager 重试。
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	burn5m, burn1h, _ := h.promClient.BurnRates(ctx, h.service)
	event := triage.Triage(&payload, burn5m, burn1h, h.store)
	if event != nil {
		metrics.EventsCreated.WithLabelValues(string(event.Tier)).Inc()
		// 异步处理：LLM 推断 + 规则引擎修复。
		go h.processEvent(event)
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"accepted"}`))
}

// processEvent 异步处理事件：查日志 → LLM 根因推断 → 规则引擎修复 → 持久化。
func (h *Handler) processEvent(event *triage.Event) {
	if event.Status == "resolved" {
		// 阶段四进化功能：事件 resolved 后，喂 LLM 生成 postmortem 草稿，落 ConfigMap 供 validate 校验。
		h.generatePostmortem(event)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// 查 Loki 日志摘要喂 LLM。
	logSummary := ""
	if h.lokiClient != nil {
		logSummary, _ = h.lokiClient.Tail(ctx,
			`{namespace="sre-demo",container="ordersvc"} |= "error" | json`, 50)
	}

	// LLM 根因推断（失败不阻断）。
	if h.llmClient != nil {
		event.LLMResult = h.llmClient.Infer(ctx, event, logSummary)
		h.store.Upsert(event)
	}

	// 规则引擎评估并执行修复。
	log := h.engine.Evaluate(event)
	event.Remediations = append(event.Remediations, log)
	h.store.Upsert(event)
	if log.Result == "executed" || log.Result == "failed" {
		metrics.RemediationActions.WithLabelValues(log.Action, log.Result).Inc()
	} else {
		metrics.RemediationActions.WithLabelValues(log.Action, log.Result).Inc()
	}
}

// handleEvents 返回事件列表。
func (h *Handler) handleEvents(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(h.store.List())
}

// generatePostmortem 阶段四：事件 resolved 后调 LLM 生成 postmortem 草稿，落 ConfigMap。
// 草稿按 postmortem/template.md 结构，可被 validate-postmortem.py 校验。体现"自动闭环运营"。
func (h *Handler) generatePostmortem(event *triage.Event) {
	if h.llmClient == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	draft, err := h.llmClient.GenerateRCA(ctx, event, event.Remediations)
	if err != nil {
		return
	}
	// 落 ConfigMap（key = auto-<eventID>.md），供 validate-postmortem.py 校验。
	if h.k8sClient != nil {
		_ = h.k8sClient.WritePostmortemDraft(event.ID, draft)
	}
}

// kubeExecutor 实现 remediate.ActionExecutor，调用 k8s client + ordersvc /admin/fault。
type kubeExecutor struct {
	client    *k8s.Client
	deployName string
}

func (e *kubeExecutor) FaultReset() (string, error) {
	// 调 ordersvc Service /admin/fault 复位故障注入。
	// ClusterIP service ordersvc:8080/admin/fault?fail=0&latency=0
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get("http://ordersvc.sre-demo:8080/admin/fault?fail=0&latency=0")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fault reset status %d", resp.StatusCode)
	}
	return "fault_reset 调用成功，故障注入已复位", nil
}

func (e *kubeExecutor) ScaleUp(replicas int) (string, error) {
	if e.client == nil {
		return "", fmt.Errorf("k8s client 不可用（非集群内运行）")
	}
	return e.client.ScaleUp(e.deployName, int32(replicas))
}

func (e *kubeExecutor) RolloutUndo() (string, error) {
	if e.client == nil {
		return "", fmt.Errorf("k8s client 不可用（非集群内运行）")
	}
	return e.client.RolloutUndo(e.deployName)
}
