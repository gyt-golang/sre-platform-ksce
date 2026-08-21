#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""Toil（人工劳动）事件记录器。

SRE 三大支柱之一「消除 toil」：记录每次手动干预的耗时与可自动化程度，
追加到 docs/toil-log.csv，供 toil-report.py 聚合分析。

用法：
  python deploy/scripts/toil-log.py --service ordersvc --task manual-rollback \
      --duration 8 --automatable --notes "混沌演练手动注入故障并观察告警恢复"
  python deploy/scripts/toil-log.py --service ordersvc --task log-grep \
      --duration 5 --automatable --notes "kubectl logs 手动排查错误日志"
"""
import argparse, csv, datetime, os

CSV = "docs/toil-log.csv"
HEADERS = ["date", "service", "task", "duration_min", "automatable", "notes"]


def main():
    p = argparse.ArgumentParser(description="记录一次 toil 事件到 docs/toil-log.csv")
    p.add_argument("--service", required=True, help="服务名，如 ordersvc")
    p.add_argument("--task", required=True, help="任务名，如 manual-rollback / manual-scale / log-grep")
    p.add_argument("--duration", type=int, required=True, help="耗时（分钟）")
    p.add_argument("--automatable", action="store_true", help="标记为可自动化（驱动自动化优先级）")
    p.add_argument("--notes", default="", help="备注")
    p.add_argument("--date", default=datetime.date.today().isoformat(), help="日期，默认今天")
    a = p.parse_args()

    os.makedirs(os.path.dirname(CSV), exist_ok=True)
    exists = os.path.exists(CSV)
    with open(CSV, "a", newline="", encoding="utf-8") as f:
        w = csv.writer(f)
        if not exists:
            w.writerow(HEADERS)
        w.writerow([a.date, a.service, a.task, a.duration, "yes" if a.automatable else "no", a.notes])
    print(f"OK toil 已记录: {a.service}/{a.task} {a.duration}min automatable={a.automatable}")


if __name__ == "__main__":
    main()
