#!/usr/bin/env bash
# SRE 可靠性工程平台 —— 一键部署脚本（kind 本地集群版，历史保留）
# 注意：本脚本是 kind 单机版。金山云 KEC 真实多节点集群部署请用 bootstrap-ksce.sh。
# 用途：拉起 kind 集群 + 构建/加载 ordersvc 镜像 + 部署应用 + 可观测性栈 + Chaos Mesh
# 前置：Docker Desktop 已启动（WSL2 已启用，首次需重启电脑让虚拟机平台功能生效）。
set -euo pipefail

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$PROJECT_ROOT"

# winget 安装的工具可能不在默认 PATH，补齐常见路径。
export PATH="$PATH:/c/Program Files/Docker/Docker/resources/bin:/c/Program Files/Go/bin"
export PATH="$PATH:$HOME/AppData/Local/Microsoft/WinGet/Links"
export PATH="$PATH:$HOME/AppData/Local/Microsoft/WinGet/Packages/Kubernetes.kind_Microsoft.Winget.Source_8wekyb3d8bbwe"
export PATH="$PATH:$HOME/AppData/Local/Microsoft/WinGet/Packages/Helm.Helm_Microsoft.Winget.Source_8wekyb3d8bbwe/windows-amd64"
export PATH="$PATH:$HOME/AppData/Local/Microsoft/WinGet/Packages/Kubernetes.kubectl_Microsoft.Winget.Source_8wekyb3d8bbwe"

log(){ echo -e "\033[1;34m[bootstrap]\033[0m $*"; }
err(){ echo -e "\033[1;31m[error]\033[0m $*" >&2; }

# 1. 工具与 Docker 就绪检查
for t in docker kind kubectl helm; do
  command -v $t >/dev/null || { err "缺少 $t，请确认已安装"; exit 1; }
done
log "等待 Docker engine 就绪..."
for i in $(seq 1 60); do
  docker info >/dev/null 2>&1 && break
  sleep 3
done
docker info >/dev/null 2>&1 || { err "Docker 未就绪，请先启动 Docker Desktop"; exit 1; }
log "Docker 就绪 ✓"

# 2. 创建 kind 集群
if kind get clusters | grep -q sre-demo; then
  log "kind 集群 sre-demo 已存在，跳过创建"
else
  log "创建 kind 集群..."
  kind create cluster --name sre-demo --config deploy/kind-config.yaml
fi
kubectl cluster-info --context kind-sre-demo >/dev/null
log "K8s 集群就绪 ✓"

# 3. 构建并加载 ordersvc 镜像
log "构建 ordersvc 镜像..."
docker build -t sre-demo/ordersvc:latest --build-arg VERSION=sre-demo-v1 app/
log "加载镜像到 kind 节点..."
kind load docker-image sre-demo/ordersvc:latest --name sre-demo

# 4. 部署应用
log "部署 ordersvc..."
kubectl apply -f deploy/manifests/ordersvc.yaml

# 5. 部署可观测性三支柱
log "部署可观测性栈（Prometheus/Grafana/Loki+Promtail/Jaeger+OTel）..."
kubectl apply -f observability/prometheus/prometheus.yaml
kubectl apply -f observability/grafana/grafana.yaml
kubectl apply -f observability/loki/loki.yaml
kubectl apply -f observability/jaeger/jaeger.yaml
# 加载 SLO 规则（PrometheusRule CRD 形式，供 Prometheus Operator；裸 Prometheus 用 ConfigMap 版）
kubectl apply -f observability/prometheus/prometheus-rule-slo.yaml 2>/dev/null || true

# 6. 部署 Chaos Mesh
log "部署 Chaos Mesh..."
helm repo add chaos-mesh https://charts.chaos-mesh.org 2>/dev/null || true
helm repo update
helm upgrade --install chaos-mesh chaos-mesh/chaos-mesh \
  --namespace chaos-mesh --create-namespace \
  --set chaosDaemon.runtime=containerd \
  --set controllerManager.replicaCount=1 \
  --version 2.7.0
log "部署混沌实验..."
kubectl apply -f chaos/experiments.yaml

# 7. 等待核心组件就绪
log "等待 Pod 就绪（最长 5 分钟）..."
kubectl -n sre-demo rollout status deployment/ordersvc --timeout=300s || true
kubectl -n observability rollout status deployment/prometheus --timeout=300s || true
kubectl -n observability rollout status deployment/grafana --timeout=300s || true

cat <<EOF

\033[1;32m✓ 部署完成\033[0m

访问地址（kind 已映射到宿主机）：
  ordersvc API :  http://localhost:8080/healthz
  Prometheus   :  http://localhost:9090   （查 SLO 规则：/rules，告警：/alerts）
  Grafana      :  http://localhost:3000   （SRE/SLO 大盘，admin/admin）
  Jaeger UI    :  http://localhost:16686  （查 ordersvc 链路）

下一步：
  # 1. 打流量，让 SLO 指标有数据
  bash deploy/scripts/load-test.sh
  # 2. 注入 50% 故障，观察错误预算燃烧速率告警
  bash deploy/scripts/chaos-inject-fault.sh 0.5 100
  # 3. 触发 Chaos Mesh 实验（Pod Kill / 网络延迟 / CPU 压力）
  kubectl -n sre-demo get podchaos,networkchaos,stresschaos
EOF
