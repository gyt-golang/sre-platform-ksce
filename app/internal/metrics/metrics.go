package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// OrderServiceMetrics 聚合订单服务所有 Prometheus 指标。
// 设计原则：只暴露对 SLO 计算与告警有用的指标，避免指标爆炸。
type OrderServiceMetrics struct {
	// HTTPRequestsTotal 按 method/path/code 维度统计请求总量，用于计算可用性 SLI（成功率）。
	HTTPRequestsTotal *prometheus.CounterVec

	// HTTPRequestDurationSeconds 延迟直方图，用于计算延迟 SLI（如 P99 < 500ms）。
	// Bucket 覆盖 5ms~10s，兼顾快速 API 与故障态长尾。
	HTTPRequestDurationSeconds *prometheus.HistogramVec

	// OrdersInFlight 当前在途订单数，用于 HPA 扩缩容决策与容量观察。
	OrdersInFlight prometheus.Gauge

	// OrdersCreatedTotal 业务侧下单成功总量。
	OrdersCreatedTotal prometheus.Counter

	// OrdersFailedTotal 业务侧下单失败总量（含库存不足、支付超时等）。
	OrdersFailedTotal *prometheus.CounterVec

	// FailureRateInjected 注入的错误率（混沌演练时由环境变量动态调整），便于大盘直接观察故障注入强度。
	FailureRateInjected prometheus.Gauge

	// LatencyInjectedMs 注入的处理延迟，便于大盘观察故障注入强度。
	LatencyInjectedMs prometheus.Gauge

	// BuildInfo 构建信息，方便定位上线版本与故障关联。
	BuildInfo *prometheus.GaugeVec
}

var registry = prometheus.NewRegistry()

// Registry 返回服务专用 Collector Registry，隔离默认 Go/进程指标噪声。
func Registry() *prometheus.Registry { return registry }

// Registry 方法：供 handler 通过指标对象获取同一 registry（指标注册与暴露共用一份）。
func (m *OrderServiceMetrics) Registry() *prometheus.Registry { return registry }

// New 注册并返回全部指标。
func New(buildVersion string) *OrderServiceMetrics {
	m := &OrderServiceMetrics{
		HTTPRequestsTotal: promauto.With(registry).NewCounterVec(
			prometheus.CounterOpts{
				Name: "ordersvc_http_requests_total",
				Help: "Total HTTP requests served, partitioned by method/path/code.",
			},
			[]string{"method", "path", "code"},
		),
		HTTPRequestDurationSeconds: promauto.With(registry).NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "ordersvc_http_request_duration_seconds",
				Help:    "HTTP request latency in seconds.",
				Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
			},
			[]string{"method", "path"},
		),
		OrdersInFlight: promauto.With(registry).NewGauge(prometheus.GaugeOpts{
			Name: "ordersvc_orders_in_flight",
			Help: "Current in-flight orders being processed.",
		}),
		OrdersCreatedTotal: promauto.With(registry).NewCounter(prometheus.CounterOpts{
			Name: "ordersvc_orders_created_total",
			Help: "Total orders successfully created.",
		}),
		OrdersFailedTotal: promauto.With(registry).NewCounterVec(
			prometheus.CounterOpts{
				Name: "ordersvc_orders_failed_total",
				Help: "Total orders failed, partitioned by reason.",
			},
			[]string{"reason"},
		),
		FailureRateInjected: promauto.With(registry).NewGauge(prometheus.GaugeOpts{
			Name: "ordersvc_failure_rate_injected",
			Help: "Current injected failure rate (0..1) for chaos drill.",
		}),
		LatencyInjectedMs: promauto.With(registry).NewGauge(prometheus.GaugeOpts{
			Name: "ordersvc_latency_injected_ms",
			Help: "Current injected processing latency in ms for chaos drill.",
		}),
		BuildInfo: promauto.With(registry).NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "ordersvc_build_info",
				Help: "Build info labelled by version.",
			},
			[]string{"version"},
		),
	}
	m.BuildInfo.WithLabelValues(buildVersion).Set(1)
	return m
}
