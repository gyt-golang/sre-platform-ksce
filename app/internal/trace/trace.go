package trace

import (
	"context"
	"fmt"
	"os"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// Init 初始化 OpenTelemetry TracerProvider，通过 OTLP/HTTP 上报到 Jaeger Collector。
// service name 取自环境变量 OTEL_SERVICE_NAME（默认 ordersvc）。
// 返回 shutdown 函数，进程退出前调用以 flush 残留 span。
func Init(ctx context.Context) (func(context.Context) error, error) {
	svc := os.Getenv("OTEL_SERVICE_NAME")
	if svc == "" {
		svc = "ordersvc"
	}

	// OTLP HTTP exporter，默认指向 http://otel-collector:4318（集群内）。
	// 本地调试可通过 OTEL_EXPORTER_OTLP_ENDPOINT 覆盖为 http://localhost:4318。
	exporter, err := otlptracehttp.New(ctx,
		otlptracehttp.WithTimeout(10*time.Second),
	)
	if err != nil {
		return nil, fmt.Errorf("create otel exporter: %w", err)
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(semconv.ServiceNameKey.String(svc)),
	)
	if err != nil {
		return nil, fmt.Errorf("create otel resource: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		// 采样率：SRE 可观测性场景下默认全量采样，便于故障演练时抓到完整链路。
		// 生产可替换为 ParentBased(TraceIDRatioBased(0.1)) 降低开销。
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})

	return tp.Shutdown, nil
}
