# Runbook —— OrdersvcHighErrorRatePage 告警处置

> 触发条件：5m 与 1h 双窗口错误预算燃烧率均 > 14.4（1 小时内消耗 ≥2% 月错误预算）。
> 严重级别：**Page**（立即介入）。

## 一、告警含义
错误预算高速燃烧，意味着当前错误率远超 SLO 允许的 0.1%。若不介入，将在数小时内耗尽月预算。

## 二、诊断决策树

```
告警触发
  │
  ├─ Prometheus /alerts 查看 OrdersvcHighErrorRatePage 是否仍 Firing
  │    └─ 查询：ordersvc:burn_rate5m, ordersvc:burn_rate1h
  │
  ├─ 确认是否混沌演练（误报）
  │    └─ 查询：ordersvc_failure_rate_injected > 0 ？→ 若是，标注演练中，降级处理
  │
  ├─ 查看 Grafana SLO 大盘 → 错误率/延迟/P99 曲线定位劣化起点
  │
  ├─ Jaeger 查 ordersvc 链路 → 定位慢/失败 span（downstream.payment？）
  │
  ├─ Loki 查 ordersvc 日志：{namespace="sre-demo",container="ordersvc"} | json
  │
  └─ kubectl -n sre-demo describe pod / events → 排查 Pod 重启、OOM、调度失败
```

## 三、处置动作

### 场景 A：下游（支付/库存）超时导致 5xx
1. `curl /admin/fault?fail=0&latency=0` 确认非注入故障。
2. 在 Jaeger 链路确认 downstream.payment span 耗时异常。
3. 临时降级：调大 ordersvc 客户端超时 + 熔断下游，返回降级响应而非 500。
4. 联系下游团队，跟踪恢复。

### 场景 B：Pod 大量重启（OOM/Crash）
1. `kubectl -n sre-demo get pod -w` 观察 RestartCount。
2. `kubectl logs <pod> --previous` 查崩溃前日志。
3. 若 OOMKilled：临时扩 limits.memory 或 `kubectl scale deploy/ordersvc --replicas=6`。
4. 排查内存泄漏，定位到 commit 后回滚。

### 场景 C：流量突增打满容量
1. `kubectl -n sre-demo get hpa` 查看是否已达 maxReplicas。
2. 若已达上限：手动 `kubectl scale` 或扩 HPA maxReplicas。
3. 评估是否需要限流降级保护核心链路。

## 四、恢复标准
- `ordersvc:burn_rate5m < 1` 持续 10 分钟
- 可用性 SLI 回到 99.9% 以上
- 告警状态 Resolved

## 五、复盘
故障恢复后 24h 内发起 Postmortem（见 `postmortem/template.md`），无责复盘，产出行动项跟进至闭环。
