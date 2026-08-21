#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""SLO 周期达成报告生成器。

查询 Prometheus recording rules，计算本月 SLI / 错误预算消耗，生成 markdown 报告。
补 SLO 闭环的「回顾」环节：SLO 定义 → 监控 → 告警 → 周期回顾 → 调整目标。

用法：
  SRE_PROM_URL=http://10.0.0.182:30090 python deploy/scripts/slo-report.py
  # 本地验证金山云集群用公网：export SRE_PROM_URL=http://<集群公网IP>:30090

数据源：金山云 KEC 集群 Prometheus（NodePort 30090），指标来自 slo-spec.yaml 派生的 recording rules。
"""
import os, sys, json, urllib.request, urllib.parse, datetime

PROM = os.environ.get("SRE_PROM_URL", "http://10.0.0.182:30090").rstrip("/")
OUT = "docs/slo-report-2026-08.md"
MONTH = "2026-08"
SLO_TARGET = 99.9          # availability-99.9
LATENCY_TARGET_MS = 500    # latency-p99-500ms


def query(promql):
    """查 Prometheus instant query，返回首个 scalar 值。"""
    url = PROM + "/api/v1/query?" + urllib.parse.urlencode({"query": promql})
    try:
        with urllib.request.urlopen(url, timeout=10) as r:
            d = json.load(r)
        res = d.get("data", {}).get("result", [])
        return float(res[0]["value"][1]) if res else None
    except Exception as e:
        print(f"[warn] query failed: {promql[:50]} -> {e}", file=sys.stderr)
        return None


def main():
    err1d = query("ordersvc:error_ratio1d")
    burn1d = query("ordersvc:burn_rate1d")
    p99 = query("ordersvc:latency_p99_5m")
    req1d = query("ordersvc:request_rate1d")

    sli = (1 - err1d) * 100 if err1d is not None else None
    # burn_rate1d=1 表示按当前速率 30 天耗尽预算；剩余 = 100% - 当前消耗占比
    budget_remaining = max(0, 100 - (burn1d or 0) * 100 / 30) if burn1d is not None else None
    days_to_exhaust = (30 / burn1d) if (burn1d and burn1d > 0) else None
    avail_ok = sli is not None and sli >= SLO_TARGET
    lat_ok = p99 is not None and p99 * 1000 <= LATENCY_TARGET_MS

    def fmt(v, f):
        return f.format(v) if v is not None else "N/A"

    lines = [
        f"# SLO 周期达成报告 — {MONTH}",
        "",
        f"> 由 `deploy/scripts/slo-report.py` 查询 Prometheus recording rules 自动生成。",
        f"> 数据源：`{PROM}`（金山云 KEC 集群 Prometheus，NodePort 30090）",
        f"> 生成时间：{datetime.date.today()}",
        "",
        "## 1. SLO 达成概览",
        "",
        "| SLO | 目标 | 当前值 | 状态 |",
        "|---|---|---|---|",
        f"| 可用性 availability-99.9 | ≥{SLO_TARGET}% | {fmt(sli, '{:.4f}%')} | {'✅ 达标' if avail_ok else '❌ 未达标' if sli is not None else '⚠️ 无数据'} |",
        f"| 延迟 latency-p99-500ms | P99<{LATENCY_TARGET_MS}ms | {fmt(p99*1000 if p99 else None, '{:.0f}ms')} | {'✅ 达标' if lat_ok else '❌ 未达标' if p99 is not None else '⚠️ 无数据'} |",
        "",
        "## 2. 关键指标（recording rules 实时值）",
        "",
        f"- 错误率 `ordersvc:error_ratio1d`：{fmt(err1d*100, '{:.4f}%') if err1d is not None else 'N/A'}",
        f"- 燃烧率 `ordersvc:burn_rate1d`：{fmt(burn1d, '{:.2f}')}（>1 表示按当前速率 30 天耗尽预算）",
        f"- 请求速率 `ordersvc:request_rate1d`：{fmt(req1d, '{:.2f}')} req/s",
        f"- P99 延迟 `ordersvc:latency_p99_5m`：{fmt(p99*1000 if p99 else None, '{:.0f}')}ms",
        "",
        "## 3. 错误预算分析",
        "",
        f"- 月度预算（{SLO_TARGET}% of 43200min）：{43200*(1-SLO_TARGET/100):.1f} 分钟",
        f"- 错误预算剩余：{fmt(budget_remaining, '{:.1f}%')}",
        f"- 按当前燃烧率，预计 {fmt(days_to_exhaust, '{:.1f}')} 天耗尽" if days_to_exhaust else "- 燃烧率 ≤0，预算无消耗风险",
        "",
        "## 4. SLO 单一事实源",
        "",
        "SLO 目标与 SLI 查询定义在 `observability/slo-spec.yaml`（声明式 spec，单一事实源）。",
        "recording/alert rules 由 spec 派生，本报告数据即来自派生的 recording rules。",
        "改 SLO 目标只改 spec，规则与报告随之对齐。",
        "",
        "## 5. 趋势与建议",
        "",
    ]
    if burn1d and burn1d > 1:
        lines.append(f"- ⚠️ 燃烧率 {burn1d:.2f} > 1，错误预算按当前速率将在 30 天内耗尽，建议排期优化根因。")
    elif burn1d and burn1d > 0.5:
        lines.append(f"- 燃烧率 {burn1d:.2f} 偏高，关注错误率趋势。")
    else:
        lines.append("- 燃烧率正常，错误预算健康。")
    if p99 is not None and not lat_ok:
        lines.append(f"- ⚠️ P99 延迟 {p99*1000:.0f}ms 超 {LATENCY_TARGET_MS}ms SLO，排查慢查询/资源瓶颈。")
    lines += ["", "---", "*本报告是 SLO 闭环的「回顾」环节：SLO 定义 → 监控 → 告警 → **周期回顾** → 调整目标。*"]

    with open(OUT, "w", encoding="utf-8") as f:
        f.write("\n".join(lines))
    print(f"OK SLO 报告已生成: {OUT}")


if __name__ == "__main__":
    main()
