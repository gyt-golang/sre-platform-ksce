#!/usr/bin/env bash
# 金丝雀发布 + 自动回滚 脚本（Argo Rollouts）
#
# 流程：
#   1. 在 master01 远程构建新版本镜像（--build-arg VERSION 注入版本号）
#   2. push 到金山云 KECR
#   3. kubectl set image 触发 Rollout 金丝雀（20%→40%→60%→100%）
#   4. 每步 AnalysisTemplate 查 Prometheus SLO 指标，超阈值自动回滚
#
# 用法：
#   bash deploy/scripts/canary-deploy.sh ksce-v2          # 正常发布 v2
#   bash deploy/scripts/canary-deploy.sh ksce-v3-bad      # 故意发坏版本，验证自动回滚
#
# 前置环境变量（与 bootstrap-ksce.sh 一致）：
#   export KSCE_PWD=<master root 密码> KECR_USER=<KECR账号> KECR_PWD=<KECR密码>
#   export KSCE_HOST=<master01 公网IP>  # 本地访问集群用公网
set -uo pipefail
PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$PROJECT_ROOT"

NEW_TAG="${1:?用法: bash canary-deploy.sh <新版本tag，如 ksce-v2>}"
MASTER01="${KSCE_HOST:-10.0.0.182}"
KECR=hub.kce.ksyun.com
KECR_REPO=czmtest/gyt_test/ordersvc
IMG="${KECR}/${KECR_REPO}:${NEW_TAG}"
export KUBECONFIG="${KUBECONFIG:-deploy/scripts/kubeconf-ksce.conf}"
export PYTHONIOENCODING=utf-8

log(){ echo -e "\033[1;34m[canary]\033[0m $*"; }
err(){ echo -e "\033[1;31m[error]\033[0m $*" >&2; }

[ -n "${KSCE_PWD:-}" ]  || { err "请先 export KSCE_PWD=<master root 密码>"; exit 1; }
[ -n "${KECR_USER:-}" ] || { err "请先 export KECR_USER=<KECR 账号>"; exit 1; }
[ -n "${KECR_PWD:-}" ]  || { err "请先 export KECR_PWD=<KECR 密码>"; exit 1; }

log "1/4 上传源码到 master01..."
MSYS_NO_PATHCONV=1 python deploy/scripts/ksce-remote.py upload app /root/sre-app

log "2/4 构建镜像 ${IMG} 并推送 KECR..."
MSYS_NO_PATHCONV=1 python deploy/scripts/ksce-remote.py exec \
  "cd /root/sre-app && DOCKER_BUILDKIT=0 docker build -t ${IMG} --build-arg VERSION=${NEW_TAG} . && \
   echo '${KECR_PWD}' | docker login ${KECR} -u ${KECR_USER} --password-stdin 2>/dev/null; \
   docker push ${IMG}" || { err "构建/推送失败"; exit 1; }

log "3/4 触发 Rollout 金丝雀（新版本 ${NEW_TAG}）..."
# kubectl rollouts 插件未必装，用 kubectl patch 改 image 字段触发
kubectl -n sre-demo patch rollout ordersvc --type='json' \
  -p="[{\"op\":\"replace\",\"path\":\"/spec/template/spec/containers/0/image\",\"value\":\"${IMG}\"}]" \
  || { err "patch 失败，检查 Rollout 是否存在"; exit 1; }

log "4/4 金丝雀进度观察（20%→40%→60%→100%，每步 Analysis 查 SLO 指标，超阈值自动回滚）..."
log "实时状态（Ctrl+C 退出观察，不影响后台金丝雀）："
kubectl -n sre-demo get rollout ordersvc -w
