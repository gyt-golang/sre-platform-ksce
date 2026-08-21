#!/usr/bin/env bash
# 应用层故障注入脚本：通过 ordersvc /admin/fault 接口热更新失败率与延迟。
# 用法：bash chaos-inject-fault.sh [fail_rate 0-1] [latency_ms]
#   bash chaos-inject-fault.sh 0.5 100   # 50% 5xx + 100ms 延迟
#   bash chaos-inject-fault.sh 0 0       # 恢复
# 用途：触发错误预算燃烧速率告警（OrdersvcHighErrorRatePage），验证 SLO 告警链路。
set -uo pipefail
FAIL="${1:-0.3}"
LATENCY="${2:-100}"
SRE_HOST="${SRE_HOST:-10.0.0.182}"
URL="http://${SRE_HOST}:30088/admin/fault"
echo "[chaos] 注入故障: fail_rate=$FAIL latency_ms=${LATENCY}"
curl -s "${URL}?fail=${FAIL}&latency=${LATENCY}"
echo
echo "[chaos] 观察告警："
echo "  Prometheus /alerts -> OrdersvcHighErrorRatePage (burn_rate5m>14.4 & burn_rate1h>14.4)"
echo "  Grafana SLO 大盘 -> 错误预算燃烧率曲线"
