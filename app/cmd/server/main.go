package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/sre-demo/ordersvc/internal/handler"
	"github.com/sre-demo/ordersvc/internal/metrics"
	"github.com/sre-demo/ordersvc/internal/trace"
)

// buildVersion 由 ldflags 注入（-X main.buildVersion=...），便于通过 ordersvc_build_info 追踪上线版本。
var buildVersion = "dev"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// 初始化链路追踪（OTLP -> Jaeger）。失败不阻断启动：可观测性退化为指标+日志，仍满足 SLO 基础观测。
	shutdown, err := trace.Init(ctx)
	if err != nil {
		log.Printf("[warn] otel init failed, traces disabled: %v", err)
	} else {
		defer func() { _ = shutdown(context.Background()) }()
	}

	m := metrics.New(buildVersion)
	h := handler.New(m, buildVersion)

	// 数据面（业务）与控制面（指标采集）端口分离，避免互相影响——大厂常见做法。
	businessAddr := getenv("BUSINESS_ADDR", ":8080")
	metricsAddr := getenv("METRICS_ADDR", ":9090")

	// 业务端口：/ /healthz /readyz /order /admin/fault
	srv := &http.Server{Addr: businessAddr, Handler: h.Routes()}

	// 指标端口：仅 /metrics，供 Prometheus 采集。
	metricsMux := http.NewServeMux()
	metricsMux.Handle("/metrics", promhttp.HandlerFor(m.Registry(), promhttp.HandlerOpts{}))
	metricsSrv := &http.Server{Addr: metricsAddr, Handler: metricsMux}

	go func() {
		log.Printf("[info] ordersvc %s business on %s", buildVersion, businessAddr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[fatal] business server: %v", err)
		}
	}()
	go func() {
		log.Printf("[info] metrics on %s", metricsAddr)
		if err := metricsSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[fatal] metrics server: %v", err)
		}
	}()

	<-ctx.Done()
	log.Printf("[info] shutting down...")

	shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutCtx)
	_ = metricsSrv.Shutdown(shutCtx)
	log.Printf("[info] stopped")
}

func getenv(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
