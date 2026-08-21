#!/usr/bin/env bash
# 流量压测脚本：向 ordersvc 打稳定流量，让 SLO 指标（成功率/延迟/P99）持续产生数据。
# 用法：bash load-test.sh [QPS] [持续时间秒]
#   默认 20 QPS，持续 600s。配合混沌注入观察 SLO 燃烧率变化。
set -uo pipefail
QPS="${1:-20}"
DURATION="${2:-600}"
# 金山云集群 ordersvc NodePort（30088），默认走 master01 公网 IP，可用 SRE_HOST 覆盖。
SRE_HOST="${SRE_HOST:-10.0.0.182}"
URL="http://${SRE_HOST}:30088/order"
INTERVAL=$(awk "BEGIN{print 1/$QPS}")

echo "[load-test] $QPS QPS -> $URL, 持续 ${DURATION}s"
END=$(( $(date +%s) + DURATION ))
ok=0; fail=0
while [ "$(date +%s)" -lt "$END" ]; do
  code=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$URL" 2>/dev/null || echo "000")
  if [ "$code" = "201" ]; then ok=$((ok+1)); else fail=$((fail+1)); fi
  sleep "$INTERVAL"
done
total=$((ok+fail))
if [ "$total" -gt 0 ]; then
  rate=$(awk "BEGIN{printf \"%.2f\", $ok*100/$total}")
  echo "[load-test] 完成: 成功 $ok / 总 $total (成功率 ${rate}%), 失败 $fail"
fi
