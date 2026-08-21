# Postmortem —— ordersvc 错误预算燃烧告警（混沌演练复盘示例）

> 本示例基于项目混沌实验「HTTP 50% 5xx 注入」触发的 OrdersvcHighErrorRatePage 告警，演示完整复盘流程。

## 事故概要
- **事故标题**：混沌演练注入 50% 故障率触发 Page 级燃烧率告警
- **严重级别**：Page
- **影响 SLO**：可用性 99.9%
- **开始时间**：2026-08-19 15:00 (UTC+8)
- **恢复时间**：2026-08-19 15:08 (UTC+8)
- **MTTD**：~30s（告警 for:2m 后 Firing，Prometheus 自动检测）
- **MTTR**：~8 分钟（注入停止后 burn_rate 自然回落）
- **错误预算消耗**：演练期间消耗约 1.2% 月预算
- **事故指挥官**：oncall SRE

## 影响范围
- **受影响服务**：ordersvc /order 接口
- **受影响业务**：演练流量，无真实用户影响（已提前公告）
- **外部表现**：50% 请求返回 500，P99 延迟因注入 100ms 而上升

## 时间线
| 时间 | 事件 | 操作人 |
|---|---|---|
| 15:00:00 | 执行 `chaos-inject-fault.sh 0.5 100` 注入 50% 故障 + 100ms 延迟 | SRE |
| 15:00:30 | burn_rate5m 飙升至 500（0.5/0.001），远超 14.4 阈值 | 自动 |
| 15:02:30 | OrdersvcHighErrorRatePage Firing（for:2m 满足） | Alertmanager |
| 15:03:00 | oncall 确认 OrdersvcChaosInjected 告警同时存在 → 标注演练中 | oncall |
| 15:05:00 | 验证 Grafana SLO 大盘错误率曲线与燃烧率一致 | oncall |
| 15:08:00 | 执行 `chaos-inject-fault.sh 0 0` 恢复，告警 Resolved | SRE |

## 根因分析（5 Whys）
1. 为什么触发 Page 告警？→ 5m/1h 燃烧率均 > 14.4。
2. 为什么燃烧率高？→ 实际错误率 50% 远超允许的 0.1%。
3. 为什么错误率 50%？→ /admin/fault 主动注入（演练）。
4. 为什么要演练？→ 验证 SLO 告警链路在真实故障下能否及时触发。
5. **根本原因**：非生产事故，属计划内演练。**验证结论：SLO 告警链路（指标→录制规则→燃烧率→多窗口告警）工作正常，MTTD ~30s 满足设计预期。**

## 哪些做得好 / 哪些待改进
- ✓ 做得好：告警在 30s 内触发，多窗口策略有效区分真实劣化与瞬时抖动；混沌注入告警（OrdersvcChaosInjected）帮助快速识别演练场景，避免误当真实故障处置。
- ✗ 待改进：演练期间未在告警注解里自动标注「演练中」标签导致 oncall 需二次确认；恢复后 burn_rate1h 因长窗口记忆仍维持高值，告警 Resolved 延迟约 6 分钟。

## 行动项
| 行动项 | 类型 | Owner | 截止 | 状态 |
|---|---|---|---|---|
| 给演练故障打 chaos label，Page 告警抑制演练期（✅ 已落地：Alertmanager `inhibit_rules` chaos=true 抑制 Page，见 `observability/alertmanager/alertmanager.yaml`） | prevent | SRE | 2026-08-21 | DONE |
| 长窗口告警 Resolved 延迟评估，考虑增加恢复确认规则 | detect | SRE | 下周 | TODO |
| 补充 NetworkChaos 场景的延迟 SLO 告警演练 | prevent | SRE | 两周 | TODO |
