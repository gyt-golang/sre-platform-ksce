// Package main 是 remediator 入口。
//
// 启动 HTTP 服务（8080 webhook/events/healthz + 9090 metrics），注册 Alertmanager webhook receiver。
// 依赖环境变量：LLM_API_URL / LLM_API_KEY / LLM_MODEL / PROM_URL / LOKI_URL / SERVICE / DEPLOY_NAME / DRY_RUN。
package main

import (
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/sre-demo/remediator/internal/handler"
	"github.com/sre-demo/remediator/internal/k8s"
	"github.com/sre-demo/remediator/internal/llm"
	"github.com/sre-demo/remediator/internal/loki"
	"github.com/sre-demo/remediator/internal/prom"
)

var buildVersion = "dev"

func main() {
	service := getenv("SERVICE", "ordersvc")
	deployName := getenv("DEPLOY_NAME", "ordersvc")
	dryRun := getenv("DRY_RUN", "false") == "true"
	namespace := getenv("NAMESPACE", "sre-demo")

	log.Printf("[info] remediator %s starting, service=%s deploy=%s dryRun=%v", buildVersion, service, deployName, dryRun)

	// 初始化各 client。k8sClient 仅集群内可用，非集群内为 nil（修复动作降级）。
	pc := prom.New(getenv("PROM_URL", "http://prometheus.observability.svc.cluster.local:9090"))
	lc := loki.New(getenv("LOKI_URL", "http://loki.observability.svc.cluster.local:3100"))
	llmC := llm.New()
	var kc *k8s.Client
	if realKC, err := k8s.New(namespace); err != nil {
		log.Printf("[warn] k8s client 不可用（修复动作降级）: %v", err)
	} else {
		kc = realKC
	}

	h := handler.New(service, deployName, pc, lc, llmC, kc, dryRun)

	// 8080：webhook/events/healthz；9090：metrics（与 ordersvc 双端口规范一致）。
	appMux := http.NewServeMux()
	h.Routes(appMux)
	// metrics 路由单独挂到 9090（避免与业务端口混用）。但为简洁，本 demo 都挂 8080，9090 仅 metrics。
	metricsMux := http.NewServeMux()
	metricsMux.Handle("/metrics", appMux)

	go func() {
		log.Println("[info] serving app on :8080")
		if err := http.ListenAndServe(":8080", appMux); err != nil {
			log.Fatalf("app server: %v", err)
		}
	}()

	// 优雅关闭。
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	log.Println("[info] stopping")
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
