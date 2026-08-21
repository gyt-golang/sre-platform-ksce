# Toil 月度报告 — 2026-08

> 由 `deploy/scripts/toil-report.py` 读 `docs/toil-log.csv` 生成。
> SRE 核心能力：量化手动劳动（toil）成本，驱动自动化优先级。
> 成本估算：2.5 元/分钟（北京 SRE 应届 ≈25k/月 ÷ 168h）。

## 1. 总览

- toil 事件数：5
- 总耗时：84 分钟（1.4 小时）
- 可自动化耗时：64 分钟（1.1 小时，76%）
- 估算人力成本：210 元
- 可回收成本（自动化后）：160 元

## 2. 自动化候选优先级（按可回收成本降序）

| 任务 | 总耗时(min) | 可自动化(min) | 可回收成本(元) | 优先级 |
|---|---|---|---|---|
| ks3-integration-debug | 45 | 45 | 112 | P0 |
| chaos-drill-manual-inject | 8 | 8 | 20 | P1 |
| manual-recovery-watch | 6 | 6 | 15 | P1 |
| log-grep | 5 | 5 | 12 | P1 |
| alertmanager-deploy | 20 | 0 | 0 | P2 |

## 3. 建议落地的自动化

- **ks3-integration-debug**（45min 可自动化）：建议实现自动化（脚本/runbook/自愈），预计月省 112 元。
- **chaos-drill-manual-inject**（8min 可自动化）：建议实现自动化（脚本/runbook/自愈），预计月省 20 元。
- **manual-recovery-watch**（6min 可自动化）：建议实现自动化（脚本/runbook/自愈），预计月省 15 元。
- **log-grep**（5min 可自动化）：建议实现自动化（脚本/runbook/自愈），预计月省 12 元。

---
*Toil 是 SRE 三大支柱之一（可靠性工程、运维运营、消除劳动）。SRE 目标：toil 占比 < 50% 工作时间，超出则需自动化。*