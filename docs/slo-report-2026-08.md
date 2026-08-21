# SLO 周期达成报告 — 2026-08

> 由 `deploy/scripts/slo-report.py` 查询 Prometheus recording rules 自动生成。
> 数据源：`http://10.0.0.182:30090`（金山云 KEC 集群 Prometheus，NodePort 30090）
> 生成时间：2026-08-21

## 1. SLO 达成概览

| SLO | 目标 | 当前值 | 状态 |
|---|---|---|---|
| 可用性 availability-99.9 | ≥99.9% | 92.8091% | ❌ 未达标 |
| 延迟 latency-p99-500ms | P99<500ms | 5ms | ✅ 达标 |

## 2. 关键指标（recording rules 实时值）

- 错误率 `ordersvc:error_ratio1d`：7.1909%
- 燃烧率 `ordersvc:burn_rate1d`：71.91（>1 表示按当前速率 30 天耗尽预算）
- 请求速率 `ordersvc:request_rate1d`：0.07 req/s
- P99 延迟 `ordersvc:latency_p99_5m`：5ms

## 3. 错误预算分析

- 月度预算（99.9% of 43200min）：43.2 分钟
- 错误预算剩余：0.0%
- 按当前燃烧率，预计 0.4 天耗尽

## 4. SLO 单一事实源

SLO 目标与 SLI 查询定义在 `observability/slo-spec.yaml`（声明式 spec，单一事实源）。
recording/alert rules 由 spec 派生，本报告数据即来自派生的 recording rules。
改 SLO 目标只改 spec，规则与报告随之对齐。

## 5. 趋势与建议

- ⚠️ 燃烧率 71.91 > 1，错误预算按当前速率将在 30 天内耗尽，建议排期优化根因。

---
*本报告是 SLO 闭环的「回顾」环节：SLO 定义 → 监控 → 告警 → **周期回顾** → 调整目标。*