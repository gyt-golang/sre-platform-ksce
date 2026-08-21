#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""成本指标生成器：把金山云资源成本/用量数据写成 Prometheus 文本指标，供 Grafana 成本大盘展示。

体现 FinOps：把"云成本"变成可观测的指标，纳入监控体系。
数据来源（demo 用估算 + KS3 API 查询）：
  - KS3 存储量：通过 KS3 ListObjects 统计桶内对象大小（Loki chunks + Velero 备份）
  - KEC 实例成本：节点数 × 月单价（静态估算）
  - Loki 日志量：kubectl 统计 ConfigMap/日志大小，或用 Prom 已有指标 loki_distributor_lines_received_total

输出为 Prometheus textfile 格式（node_exporter textfile collector 采集）或直接 pushgateway。
demo 简化：输出到 stdout，可由 cron 定时跑并写入 /tmp/cost.prom 供 node_exporter 采集。

依赖：boto3（KS3 兼容 S3 协议），环境变量 KS3_AK/KS3_SK/KS3_ENDPOINT/CLUSTER_NODES。
"""
import os
import sys
import time
import subprocess
import json

# 静态成本估算（月单价，元）——按金山云公开定价粗估，实际以账单为准。
KEC_MONTHLY_PRICE = {
    "master": 200,   # 2C8G 控制面
    "node":   500,   # 4C16G 业务负载
}
MASTER_COUNT = 3
NODE_COUNT = 2

def get_ks3_bucket_size(ak, sk, endpoint, bucket):
    """通过 KS3（S3 兼容）ListObjects 统计桶内对象总大小（字节）。"""
    try:
        import boto3
        s3 = boto3.client("s3", aws_access_key_id=ak, aws_secret_access_key=sk,
                          endpoint_url=endpoint)
        total = 0
        prefix_sizes = {}  # 按 prefix（loki/ velero/）分组
        token = None
        while True:
            kwargs = {"Bucket": bucket}
            if token:
                kwargs["ContinuationToken"] = token
            resp = s3.list_objects_v2(**kwargs)
            for obj in resp.get("Contents", []):
                size = obj["Size"]
                total += size
                key = obj["Key"]
                prefix = key.split("/")[0] + "/" if "/" in key else "(root)"
                prefix_sizes[prefix] = prefix_sizes.get(prefix, 0) + size
            if not resp.get("IsTruncated"):
                break
            token = resp.get("NextContinuationToken")
        return total, prefix_sizes
    except Exception as e:
        print(f"# KS3 查询失败: {e}", file=sys.stderr)
        return 0, {}

def get_loki_ingest_rate():
    """查 Prometheus Loki 摄入速率（lines/s），从 Grafana 大盘可看日志量趋势。"""
    try:
        import urllib.request, urllib.parse, json as j
        prom = os.environ.get("PROM_URL", "http://localhost:30090")
        q = urllib.parse.quote("sum(rate(loki_distributor_lines_received_total[5m]))")
        with urllib.request.urlopen(f"{prom}/api/v1/query?query={q}", timeout=5) as r:
            data = j.loads(r.read())
            if data["data"]["result"]:
                return float(data["data"]["result"][0]["value"][1])
    except Exception:
        pass
    return 0.0

def emit_metric(name, value, labels=None, help_text=""):
    """输出 Prometheus textfile 格式指标。"""
    label_str = ""
    if labels:
        parts = [f'{k}="{v}"' for k, v in labels.items()]
        label_str = "{" + ",".join(parts) + "}"
    if help_text:
        print(f"# HELP {name} {help_text}")
    print(f"# TYPE {name} gauge")
    print(f"{name}{label_str} {value}")

def main():
    ts = int(time.time())
    ak = os.environ.get("KS3_AK", "")
    sk = os.environ.get("KS3_SK", "")
    endpoint = os.environ.get("KS3_ENDPOINT", "https://ks3-cn-beijing-internal.ksyun.com")
    bucket = os.environ.get("KS3_BUCKET", "sre-platform-ksce")

    # 1. KEC 实例成本（月）
    master_cost = MASTER_COUNT * KEC_MONTHLY_PRICE["master"]
    node_cost = NODE_COUNT * KEC_MONTHLY_PRICE["node"]
    total_kec = master_cost + node_cost
    emit_metric("sre_cost_kec_monthly_yuan", total_kec,
                {"role": "master", "count": MASTER_COUNT}, "KEC master 月成本估算（元）")
    emit_metric("sre_cost_kec_monthly_yuan", node_cost,
                {"role": "node", "count": NODE_COUNT}, "KEC node 月成本估算（元）")
    emit_metric("sre_cost_kec_monthly_total_yuan", total_kec, {}, "KEC 总月成本估算（元）")

    # 2. KS3 存储量
    if ak and sk:
        total_size, prefix_sizes = get_ks3_bucket_size(ak, sk, endpoint, bucket)
        emit_metric("sre_cost_ks3_storage_bytes", total_size, {"bucket": bucket}, "KS3 桶存储量（字节）")
        for prefix, size in prefix_sizes.items():
            emit_metric("sre_cost_ks3_storage_bytes", size,
                        {"bucket": bucket, "prefix": prefix}, "KS3 按 prefix 存储量（字节）")
        # KS3 存储成本估算：0.12 元/GB/月
        storage_cost = total_size / (1024**3) * 0.12
        emit_metric("sre_cost_ks3_monthly_yuan", storage_cost,
                    {"bucket": bucket}, "KS3 存储月成本估算（元）")

    # 3. Loki 日志摄入速率（成本驱动：存越多越贵）
    ingest = get_loki_ingest_rate()
    emit_metric("sre_cost_loki_ingest_lines_per_sec", ingest, {}, "Loki 日志摄入速率（lines/s）")

    # 4. 总成本
    total = total_kec + (total_size / (1024**3) * 0.12 if ak else 0)
    emit_metric("sre_cost_total_monthly_yuan", total, {}, "SRE 平台月总成本估算（元）")
    print(f"# generated at {ts}")

if __name__ == "__main__":
    main()
