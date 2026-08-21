#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""Toil 月度报告生成器。

读 docs/toil-log.csv，量化手动劳动成本，按可回收成本排自动化优先级。
SRE 差异化能力：把救火换算成 $$$，数据驱动自动化排期。

用法：
  python deploy/scripts/toil-report.py            # 当月
  python deploy/scripts/toil-report.py 2026-08    # 指定月份

输出：docs/toil-report-YYYY-MM.md
"""
import csv, sys, datetime, os
from collections import defaultdict

CSV = "docs/toil-log.csv"
MONTH = sys.argv[1] if len(sys.argv) > 1 else datetime.date.today().strftime("%Y-%m")
# SRE 成本估算：北京应届 ~25k/月 ÷ (21 工作日 × 8h) = ~148 元/小时 ≈ 2.5 元/分钟
COST_PER_MIN = 2.5


def main():
    if not os.path.exists(CSV):
        print(f"[err] {CSV} 不存在，先跑 toil-log.py 记录事件", file=sys.stderr)
        sys.exit(1)
    with open(CSV, encoding="utf-8") as f:
        rows = [r for r in csv.DictReader(f) if r["date"].startswith(MONTH)]

    total_min = sum(int(r["duration_min"]) for r in rows)
    automatable_min = sum(int(r["duration_min"]) for r in rows if r["automatable"] == "yes")
    cost = total_min * COST_PER_MIN
    auto_cost = automatable_min * COST_PER_MIN
    auto_pct = f"{automatable_min / total_min * 100:.0f}%" if total_min else "0%"

    # 按任务聚合：task -> [总耗时, 可自动化耗时]
    by_task = defaultdict(lambda: [0, 0])
    for r in rows:
        t = r["task"]
        by_task[t][0] += int(r["duration_min"])
        by_task[t][1] += int(r["duration_min"]) if r["automatable"] == "yes" else 0
    candidates = sorted(by_task.items(), key=lambda x: -x[1][1])  # 按可自动化耗时降序

    lines = [
        f"# Toil 月度报告 — {MONTH}",
        "",
        f"> 由 `deploy/scripts/toil-report.py` 读 `docs/toil-log.csv` 生成。",
        f"> SRE 核心能力：量化手动劳动（toil）成本，驱动自动化优先级。",
        f"> 成本估算：{COST_PER_MIN} 元/分钟（北京 SRE 应届 ≈25k/月 ÷ 168h）。",
        "",
        "## 1. 总览",
        "",
        f"- toil 事件数：{len(rows)}",
        f"- 总耗时：{total_min} 分钟（{total_min / 60:.1f} 小时）",
        f"- 可自动化耗时：{automatable_min} 分钟（{automatable_min / 60:.1f} 小时，{auto_pct}）",
        f"- 估算人力成本：{cost:.0f} 元",
        f"- 可回收成本（自动化后）：{auto_cost:.0f} 元",
        "",
        "## 2. 自动化候选优先级（按可回收成本降序）",
        "",
        "| 任务 | 总耗时(min) | 可自动化(min) | 可回收成本(元) | 优先级 |",
        "|---|---|---|---|---|",
    ]
    for task, (tot, auto) in candidates:
        prio = "P0" if auto >= 10 else "P1" if auto >= 5 else "P2"
        lines.append(f"| {task} | {tot} | {auto} | {auto * COST_PER_MIN:.0f} | {prio} |")

    lines += ["", "## 3. 建议落地的自动化", ""]
    for task, (tot, auto) in candidates:
        if auto >= 5:
            lines.append(f"- **{task}**（{auto}min 可自动化）：建议实现自动化（脚本/runbook/自愈），预计月省 {auto * COST_PER_MIN:.0f} 元。")
    if not any(auto >= 5 for _, (_, auto) in candidates):
        lines.append("- 暂无高优先级自动化候选。")

    lines += [
        "",
        "---",
        "*Toil 是 SRE 三大支柱之一（可靠性工程、运维运营、消除劳动）。SRE 目标：toil 占比 < 50% 工作时间，超出则需自动化。*",
    ]
    out = f"docs/toil-report-{MONTH}.md"
    with open(out, "w", encoding="utf-8") as f:
        f.write("\n".join(lines))
    print(f"OK Toil 报告已生成: {out}（总 {total_min}min，可自动化 {automatable_min}min，可回收 {auto_cost:.0f} 元）")


if __name__ == "__main__":
    main()
