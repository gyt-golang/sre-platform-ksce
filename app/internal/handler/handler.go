package handler

import (
	"encoding/json"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/sre-demo/ordersvc/internal/metrics"
)

// buggy 由 ldflags 注入（-X handler.buggy=true），用于构建会 50% 500 的 v3 版本验证金丝雀自动回滚。
var buggy = "false"

// Handler 聚合订单服务的全部 HTTP 路由与业务逻辑。
type Handler struct {
	metrics *metrics.OrderServiceMetrics

	// 故障注入参数，运行时可热更新（/admin/fault），用于混沌演练动态调参。
	failRate   float64
	latencyMs  int
	buildVer   string
}

func New(m *metrics.OrderServiceMetrics, buildVer string) *Handler {
	h := &Handler{metrics: m, buildVer: buildVer}
	// 初始故障注入参数从环境变量读取，默认无故障。
	h.failRate, _ = strconv.ParseFloat(getenv("FAILURE_RATE", "0"), 64)
	h.latencyMs, _ = strconv.Atoi(getenv("LATENCY_MS", "0"))
	h.metrics.FailureRateInjected.Set(h.failRate)
	h.metrics.LatencyInjectedMs.Set(float64(h.latencyMs))
	return h
}

// Routes 注册全部路由。使用 otelhttp 在外层包装以自动注入 trace。
func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", h.index)
	mux.HandleFunc("/healthz", h.healthz)
	mux.HandleFunc("/readyz", h.readyz)
	mux.HandleFunc("/order", h.createOrder)
	mux.HandleFunc("/admin/fault", h.setFault)
	return h.instrument(mux)
}

// instrument 中间件：统一记录指标 + 链路 span，是 SLO 计算（成功率/延迟）的数据来源。
func (h *Handler) instrument(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := &statusRecorder{ResponseWriter: w, status: 200}
		next.ServeHTTP(ww, r)
		elapsed := time.Since(start).Seconds()

		// 记录延迟与请求计数，供 Prometheus 计算 SLI。
		h.metrics.HTTPRequestDurationSeconds.WithLabelValues(r.Method, r.URL.Path).Observe(elapsed)
		h.metrics.HTTPRequestsTotal.WithLabelValues(r.Method, r.URL.Path, strconv.Itoa(ww.status)).Inc()

		// 为每条 HTTP 请求补一条 server span（含状态码），串联业务 span。
		span := trace.SpanFromContext(r.Context())
		span.SetAttributes(attribute.Int("http.status_code", ww.status))
	})
}

func (h *Handler) index(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(map[string]any{
		"service": "ordersvc",
		"version": h.buildVer,
		"endpoints": []string{
			"GET  /healthz", "GET  /readyz", "POST /order", "GET  /admin/fault?fail=0.5&latency=200",
		},
	})
}

func (h *Handler) healthz(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

// readyz：就绪探针。模拟依赖检查——当注入失败率过高时标记不可读，
// 让 K8s 摘除流量，演示「探针自愈 + 流量摘除」机制。
func (h *Handler) readyz(w http.ResponseWriter, r *http.Request) {
	if h.failRate > 0.9 {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte("not ready: injected failure too high"))
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ready"))
}

// createOrder 核心业务：模拟下单流程，按注入参数产生延迟与失败。
func (h *Handler) createOrder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// BUGGY 版本：编译期注入 50% 500 错误，用于演示金丝雀自动回滚（Analysis 检测错误率超阈值）。
	// 正式版本不编译此 flag。
	if buggy == "true" && rand.Float64() < 0.5 {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "buggy-version-500"})
		return
	}

	ctx := r.Context()
	tracer := otel.Tracer("ordersvc")
	ctx, span := tracer.Start(ctx, "createOrder")
	defer span.End()

	h.metrics.OrdersInFlight.Inc()
	defer h.metrics.OrdersInFlight.Dec()

	// 注入处理延迟，模拟下游支付/库存调用耗时（混沌演练压测长尾延迟）。
	if h.latencyMs > 0 {
		_, sleepSpan := tracer.Start(ctx, "downstream.payment", trace.WithAttributes(
			attribute.Int("injected_latency_ms", h.latencyMs),
		))
		time.Sleep(time.Duration(h.latencyMs) * time.Millisecond)
		sleepSpan.End()
	}

	// 注入失败：按概率返回 500，模拟下游故障，用于验证 SLO 错误预算与告警。
	if h.failRate > 0 && rand.Float64() < h.failRate {
		h.metrics.OrdersFailedTotal.WithLabelValues("injected_500").Inc()
		span.SetAttributes(attribute.String("order.result", "failed"))
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "injected failure", "order_id": ""})
		return
	}

	// 成功下单
	orderID := strconv.FormatInt(time.Now().UnixNano(), 36)
	h.metrics.OrdersCreatedTotal.Inc()
	span.SetAttributes(attribute.String("order.result", "ok"), attribute.String("order.id", orderID))
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"order_id": orderID, "status": "created"})
}

// setFault 运行时热更新故障注入参数，供混沌实验动态控制失败率/延迟。
// 例：/admin/fault?fail=0.5&latency=300
func (h *Handler) setFault(w http.ResponseWriter, r *http.Request) {
	if v := r.URL.Query().Get("fail"); v != "" {
		if rate, err := strconv.ParseFloat(v, 64); err == nil && rate >= 0 && rate <= 1 {
			h.failRate = rate
			h.metrics.FailureRateInjected.Set(rate)
		}
	}
	if v := r.URL.Query().Get("latency"); v != "" {
		if ms, err := strconv.Atoi(v); err == nil && ms >= 0 {
			h.latencyMs = ms
			h.metrics.LatencyInjectedMs.Set(float64(ms))
		}
	}
	json.NewEncoder(w).Encode(map[string]any{"fail_rate": h.failRate, "latency_ms": h.latencyMs})
}

// statusRecorder 捕获响应状态码用于指标统计。
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

// MetricsHandler 暴露专用 registry 的 /metrics，供 Prometheus 采集。
func (h *Handler) MetricsHandler() http.Handler {
	return promhttp.HandlerFor(h.metrics.Registry(), promhttp.HandlerOpts{})
}

func getenv(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
