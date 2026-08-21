#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""Postmortem 闭环校验器。

校验 postmortem/*.md 包含必填章节（事故概要/影响范围/根因/行动项），
且行动项表至少 1 个 DONE/IN-PROGRESS（闭环证据，禁止全 TODO 堆积）。
保证 postmortem 不只是写文档，要 schema 校验 + 行动项跟踪到闭环。

用法：python deploy/scripts/validate-postmortem.py
退出码：0 全部通过，1 存在缺失/未闭环。可挂 CI 强制 postmortem 质量。
"""
import glob, re, sys

REQUIRED_SECTIONS = ["事故概要", "影响范围", "根因", "行动项"]
CLOSURE_STATES = ["DONE", "IN-PROGRESS"]  # 闭环状态（非 TODO）


def validate(path):
    errs = []
    with open(path, encoding="utf-8") as f:
        content = f.read()
    for sec in REQUIRED_SECTIONS:
        if sec not in content:
            errs.append(f"缺必填章节: {sec}")
    # 行动项表状态列：匹配 | TODO | / | DONE | / | IN-PROGRESS |
    statuses = re.findall(r"\|\s*(TODO|DONE|IN-PROGRESS)\s*\|", content)
    if not statuses:
        errs.append("行动项表无状态标记（应为 TODO/DONE/IN-PROGRESS）")
    elif not any(s in CLOSURE_STATES for s in statuses):
        errs.append(f"行动项全为 TODO（{len(statuses)} 项），无闭环证据，需 ≥1 个 DONE/IN-PROGRESS")
    return errs


def main():
    pms = [p for p in glob.glob("postmortem/*.md") if "template" not in p]
    if not pms:
        print("[warn] 无 postmortem 实例可校验")
        return
    all_ok = True
    for p in sorted(pms):
        errs = validate(p)
        if errs:
            all_ok = False
            print(f"[FAIL] {p}")
            for e in errs:
                print(f"  - {e}")
        else:
            print(f"[OK]   {p}  (章节齐全 + 行动项有闭环)")
    sys.exit(0 if all_ok else 1)


if __name__ == "__main__":
    main()
