#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""打包前清理：公网 IP → 内网 IP，KECR_USER 硬编码 → 环境变量占位。
遍历指定后缀文件，做字符串替换。"""
import os, re

# 公网 → 内网映射（探测到的真实内网 IP）
IP_MAP = {
    "10.0.0.182": "10.0.0.182",   # master01
    "10.0.0.41":  "10.0.0.41",    # master02
    "10.0.0.107":   "10.0.0.107",   # master03
    "10.0.0.136": "10.0.0.136",   # node01
    "10.0.0.242":  "10.0.0.242",   # node02
}
# KECR 用户名（金山云账号 ID）硬编码 → 环境变量
USER_REPLACE = ('KECR_USER="${KECR_USER:-<KECR登录用户名>}"', 'KECR_USER="${KECR_USER:-<KECR登录用户名>}"')

EXTS = ('.md', '.yaml', '.yml', '.sh', '.py', '.go', '.json')
ROOT = r'C:\Users\KC\Desktop\秋招\简历\简历\设计师修改\sre-project'
SKIP_DIRS = {'.git', '__pycache__'}

changed = []
for dirpath, dirnames, filenames in os.walk(ROOT):
    dirnames[:] = [d for d in dirnames if d not in SKIP_DIRS]
    for fn in filenames:
        if not fn.endswith(EXTS): continue
        fp = os.path.join(dirpath, fn)
        try:
            txt = open(fp, encoding='utf-8').read()
        except UnicodeDecodeError:
            continue
        orig = txt
        for pub, priv in IP_MAP.items():
            txt = txt.replace(pub, priv)
        txt = txt.replace(*USER_REPLACE)
        if txt != orig:
            open(fp, 'w', encoding='utf-8').write(txt)
            rel = os.path.relpath(fp, ROOT)
            changed.append(rel)

print(f'已修改 {len(changed)} 个文件：')
for f in changed: print(' ', f)
