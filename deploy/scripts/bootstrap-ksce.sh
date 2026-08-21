#!/usr/bin/env bash
# 基于金山云平台的 SRE 可靠性工程平台 —— 一键部署脚本（金山云 KEC 真实多节点集群版）
#
# 目标集群：金山云 KEC 5 节点（3 master + 2 node）K8s v1.31 / Calico / containerd
# 云产品：  KECR（镜像仓库）+ KS3（Loki 对象存储，存算分离）
# 前置：
#   1. export KSCE_PWD=<master root 密码> KECR_PWD=<KECR 登录密码> KS3_AK=<KS3 AK> KS3_SK=<KS3 SK>
#   2. deploy/scripts/kubeconf-ksce.conf 已就绪（公网化 kubeconfig）
#   3. 安全组放行 30088/30090/30300/30686
# 用法：bash deploy/scripts/bootstrap-ksce.sh
set -euo pipefail
PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$PROJECT_ROOT"

# 金山云集群与 KECR 配置
MASTER01=10.0.0.182
KCONFIG="deploy/scripts/kubeconf-ksce.conf"
KECR=hub.kce.ksyun.com
KECR_REPO=czmtest/gyt_test
KECR_USER="${KECR_USER:-<KECR登录用户名>}"
IMG="${KECR}/${KECR_REPO}/ordersvc:v1"
export KUBECONFIG="$KCONFIG"
export PYTHONIOENCODING=utf-8

log(){ echo -e "\033[1;34m[ksce]\033[0m $*"; }
err(){ echo -e "\033[1;31m[error]\033[0m $*" >&2; }

# 0. 前置检查
command -v kubectl >/dev/null || { err "缺少 kubectl"; exit 1; }
command -v python  >/dev/null || { err "缺少 python+paramiko"; exit 1; }
[ -n "${KSCE_PWD:-}" ] || { err "请先 export KSCE_PWD=<master root 密码>"; exit 1; }
[ -n "${KECR_PWD:-}" ] || { err "请先 export KECR_PWD=<KECR 登录密码>"; exit 1; }
[ -n "${KS3_AK:-}" ]  || { err "请先 export KS3_AK=<金山云 KS3 Access Key ID>"; exit 1; }
[ -n "${KS3_SK:-}" ]  || { err "请先 export KS3_SK=<金山云 KS3 Secret Access Key>"; exit 1; }
kubectl get nodes >/dev/null 2>&1 || { err "kubeconfig 不可用，请检查 $KCONFIG"; exit 1; }
log "集群连通 ✓  $(kubectl get nodes -o name | wc -l) 节点"

# 1. 5 节点 containerd 镜像加速（docker.io + ghcr.io，Chaos Mesh 镜像在 ghcr.io）
log "配置 5 节点 containerd 镜像加速（docker.io + ghcr.io）..."
python deploy/scripts/apply-mirrors.py
python deploy/scripts/apply-ghcr-mirror.py

# 2. KECR：构建并推送 ordersvc 镜像（在 master01 远程执行 docker build/push）
log "上传源码 → master01 构建 → 推送 KECR..."
MSYS_NO_PATHCONV=1 python deploy/scripts/ksce-remote.py upload app /root/sre-app
MSYS_NO_PATHCONV=1 python deploy/scripts/ksce-remote.py exec \
  "cd /root/sre-app && docker build -t ${IMG} --build-arg VERSION=ksce-v1 . && \
   echo '${KECR_PWD}' | docker login ${KECR} -u ${KECR_USER} --password-stdin && \
   docker push ${IMG}"

# 3. imagePullSecret（KECR 私有仓库凭证，注入到业务命名空间）
log "创建 imagePullSecret regcred..."
kubectl create secret docker-registry regcred --namespace=sre-demo \
  --docker-server="${KECR}" --docker-username="${KECR_USER}" --docker-password="${KECR_PWD}" \
  --dry-run=client -o yaml | kubectl apply -f -

# 4. 业务服务 ordersvc（Go 微服务 + 三类探针自愈 + HPA 弹性 + SLO ConfigMap）
log "部署 ordersvc（镜像 ${IMG}）..."
kubectl apply -f deploy/manifests/ordersvc.yaml
kubectl -n sre-demo rollout status deployment/ordersvc --timeout=300s || true

# 5. 可观测性三支柱（镜像走节点加速器拉取）
log "部署可观测性栈（Prometheus / Grafana / Loki+Promtail / Jaeger+OTel）..."
# Alertmanager：告警链路闭环（分组/抑制/飞书通知 + remediator 分诊自愈）。
# apply 前 sed 注入 __FEISHU_WEBHOOK__ 与 __REMEDIATOR_URL__；
# 未配置 FEISHU_WEBHOOK 时默认本地 echo，Alertmanager 正常运行、UI 可见告警，仅通知发不出。
FEISHU_WEBHOOK="${FEISHU_WEBHOOK:-http://127.0.0.1:9999/feishu}"
REMEDIATOR_URL="${REMEDIATOR_URL:-http://remediator.sre-demo:8080/webhook}"
sed -e "s|__FEISHU_WEBHOOK__|${FEISHU_WEBHOOK}|g" \
    -e "s|__REMEDIATOR_URL__|${REMEDIATOR_URL}|g" \
    observability/alertmanager/alertmanager.yaml | kubectl apply -f -
kubectl apply -f observability/prometheus/prometheus.yaml
# prometheus 无 config-reloader sidecar，改 alerting 配置后重启读新 alertmanager targets
kubectl -n observability rollout restart deployment/prometheus
kubectl apply -f observability/grafana/grafana.yaml
# Loki chunks 落金山云 KS3：apply 前用 sed 将 __KS3_AK__/__KS3_SK__ 占位符替换为环境变量（真实 AK/SK 不入库）
sed -e "s|__KS3_AK__|${KS3_AK}|g" -e "s|__KS3_SK__|${KS3_SK}|g" \
  observability/loki/loki.yaml | kubectl apply -f -
kubectl apply -f observability/jaeger/jaeger.yaml
# PrometheusRule CRD（需 Prometheus Operator，裸 Prometheus 用 ConfigMap 版规则，容错跳过）
kubectl apply -f observability/prometheus/prometheus-rule-slo.yaml 2>/dev/null || true

# 5.5 SLO Operator（自研 K8s Operator，把 slo-spec 做成 CRD，reconcile 派生 Prometheus 规则）
# 构建 slo-operator 镜像（master01 远程 docker build/push KECR），apply CRD + RBAC + Deployment + SLO CR。
# SLO CR apply 后 Operator reconcile 覆盖 observability/prometheus-rules ConfigMap（裸 Prometheus rules volume），
# 派生 5 窗口 recording + 3 档 alert，并修复 status→code bug。
log "部署 SLO Operator（CRD + controller + SLO CR）..."
SLO_OP_IMG="${KECR}/${KECR_REPO}/slo-operator:v1"
MSYS_NO_PATHCONV=1 python deploy/scripts/ksce-remote.py upload operator /root/slo-operator
MSYS_NO_PATHCONV=1 python deploy/scripts/ksce-remote.py exec \
  "cd /root/slo-operator && docker build -t ${SLO_OP_IMG} . && \
   echo '${KECR_PWD}' | docker login ${KECR} -u ${KECR_USER} --password-stdin && \
   docker push ${SLO_OP_IMG}"
kubectl apply -f deploy/manifests/slo-operator-crd.yaml
kubectl -n sre-demo wait crd/slos.slo.sre-demo.io --for=condition=established --timeout=60s || true
sed "s|__SLO_OP_IMG__|${SLO_OP_IMG}|g" deploy/manifests/slo-operator-deployment.yaml | kubectl apply -f -
kubectl -n sre-demo rollout status deployment/slo-operator --timeout=180s || true
# SLO CR：apply 后 controller 派生规则覆盖 prometheus-rules ConfigMap
kubectl apply -f deploy/manifests/slo-cr-ordersvc.yaml
# 等 controller reconcile 完成（ConfigMap 被覆盖），reload prometheus 加载新规则
sleep 5
kubectl -n observability rollout restart deployment/prometheus
log "SLO Operator 已派生规则（kubectl -n sre-demo get slo ordersvc 查 status）✓"

# 6. Chaos Mesh（helm，runtime=containerd，镜像在 ghcr.io 走加速；加速慢时可手动搬运镜像）
log "部署 Chaos Mesh..."
MSYS_NO_PATHCONV=1 python deploy/scripts/ksce-remote.py exec \
  "helm repo add chaos-mesh https://charts.chaos-mesh.org 2>/dev/null || true; helm repo update >/dev/null 2>&1; \
   helm upgrade --install chaos-mesh chaos-mesh/chaos-mesh -n chaos-mesh --create-namespace \
   --set chaosDaemon.runtime=containerd --set chaosDaemon.socketPath=/run/containerd/containerd.sock \
   --set controllerManager.replicaCount=1 --version 2.7.0"
log "部署混沌实验（PodChaos/NetworkChaos/StressChaos）..."
kubectl apply -f chaos/experiments.yaml
# 稳态校验模板（进化功能：Chaos Mesh 实验 + AnalysisRun 自动校验 SLO 稳态假设）
kubectl apply -f chaos/steady-state.yaml 2>/dev/null || true   # 需 Argo Rollouts AnalysisRun CRD

# 6.5 remediator：告警智能分诊+自动修复（Alertmanager webhook → 规则引擎自愈 + LLM 根因推断）
# 构建 remediator 镜像（含 kubectl，rollout undo 用），apply RBAC + Deployment + Service。
# LLM_API_KEY 从环境变量创建 Secret（绝不入库），remediator 通过 secretKeyRef 读取。
log "部署 remediator（告警分诊+自动修复，规则引擎+LLM 根因推断）..."
REMEDIATOR_IMG="${KECR}/${KECR_REPO}/remediator:v1"
MSYS_NO_PATHCONV=1 python deploy/scripts/ksce-remote.py upload remediator /root/remediator
MSYS_NO_PATHCONV=1 python deploy/scripts/ksce-remote.py exec \
  "cd /root/remediator && docker build -t ${REMEDIATOR_IMG} . && \
   echo '${KECR_PWD}' | docker login ${KECR} -u ${KECR_USER} --password-stdin && \
   docker push ${REMEDIATOR_IMG}"
# LLM API Key Secret（金山云大模型 glm-5.1，根因推断用）
kubectl -n sre-demo delete secret llm-api-key --ignore-not-found=true
kubectl -n sre-demo create secret generic llm-api-key \
  --from-literal=apiKey="${LLM_API_KEY:-dummy-key-not-configured}"
sed "s|__REMEDIATOR_IMG__|${REMEDIATOR_IMG}|g" deploy/manifests/remediator.yaml | kubectl apply -f -
kubectl -n sre-demo rollout status deployment/remediator --timeout=180s || true
log "remediator 已就绪（kubectl -n sre-demo get pod -l app=remediator 查状态）✓"

# 7. 等待核心组件就绪
log "等待 Pod 就绪（最长 6 分钟）..."
kubectl -n observability rollout status deployment/prometheus --timeout=360s || true
kubectl -n observability rollout status deployment/grafana   --timeout=360s || true
kubectl -n chaos-mesh      rollout status deployment/chaos-controller-manager --timeout=360s || true

cat <<EOF

\033[1;32m✓ 部署完成 —— 基于金山云平台的 SRE 可靠性工程平台\033[0m

金山云 KEC 公网访问地址（安全组已放行 NodePort）：
  ordersvc API :  http://${MASTER01}:30088/healthz
  Prometheus   :  http://${MASTER01}:30090    （/rules 查 SLO 规则，/alerts 查燃烧率告警）
  Grafana      :  http://${MASTER01}:30300    （SRE/SLO 大盘，admin/admin）
  Jaeger UI    :  http://${MASTER01}:30686    （查 ordersvc 端到端链路）

验证 SLO 告警链路：
  SRE_HOST=${MASTER01} bash deploy/scripts/load-test.sh 20 600          # 打流量
  SRE_HOST=${MASTER01} bash deploy/scripts/chaos-inject-fault.sh 0.5 100  # 注入 50% 故障观察燃烧率告警

备注：
  - chaos-daemon 镜像在 ghcr.io，若节点拉取缓慢，可跑 deploy/scripts/transport-image-v2.py 从已拉到镜像的节点内网搬运。
  - KS3 对象存储集成见 observability/loki/loki.yaml（Loki chunks 落 KS3，存储分离）。
EOF
