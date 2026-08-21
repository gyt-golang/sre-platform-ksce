#!/usr/bin/env bash
# 灾备：Velero 备份集群资源到 KS3 对象存储，支持按 namespace 定时备份与恢复。
#
# 体现 SRE 可靠性思维：灾难恢复（RPO/RTO）纳入体系，不只在故障时才想起备份。
# Velero 用 KS3 作后端（与 Loki 同桶，velero/ prefix），KS3 凭证走 Secret。
#
# 前置：
#   1. export KS3_AK=<AK> KS3_SK=<SK>（与 Loki 同一组）
#   2. helm 已装（master01）
#   3. KS3 桶 sre-platform-ksce 已存在（bootstrap 已建）
# 用法：bash deploy/scripts/velero-bootstrap.sh
set -euo pipefail

KS3_AK="${KS3_AK:?需要 KS3_AK}"
KS3_SK="${KS3_SK:?需要 KS3_SK}"
KS3_BUCKET="${KS3_BUCKET:-sre-platform-ksce}"
KS3_REGION="${KS3_REGION:-cn-beijing-6}"
KS3_ENDPOINT="${KS3_ENDPOINT:-https://ks3-cn-beijing-internal.ksyun.com}"
VELERO_NS="velero"

echo "==> 安装 Velero（KS3 后端，velero/ prefix）..."
# 创建凭证 Secret（KS3 兼容 S3，Velero 用 aws plugin）
cat > /tmp/velero-credentials <<EOF
[default]
aws_access_key_id=${KS3_AK}
aws_secret_access_key=${KS3_SK}
EOF
kubectl create namespace "${VELERO_NS}" 2>/dev/null || true
kubectl -n "${VELERO_NS}" create secret generic cloud-credentials \
  --from-file=cloud=/tmp/velero-credentials --dry-run=client -o yaml | kubectl apply -f -
rm -f /tmp/velero-credentials

# helm 安装 Velero，配置 KS3 作 backupStorageLocation
helm repo add vmware-tanzu https://vmware-tanzu.github.io/helm-charts 2>/dev/null || true
helm repo update
helm upgrade --install velero vmware-tanzu/velero \
  --namespace "${VELERO_NS}" \
  --set configuration.provider=aws \
  --set configuration.backupStorageLocation.name=ks3 \
  --set configuration.backupStorageLocation.bucket="${KS3_BUCKET}" \
  --set configuration.backupStorageLocation.prefix=velero \
  --set configuration.backupStorageLocation.config.region="${KS3_REGION}" \
  --set configuration.backupStorageLocation.config.endpoint="${KS3_ENDPOINT}" \
  --set configuration.backupStorageLocation.config.s3ForcePathStyle=true \
  --set credentials.useSecret=true \
  --set credentials.secretContents.cloud=credentials \
  --set image.repository=velero/velero \
  --set image.tag=v1.14.0 \
  --set initContainers[0].name=velero-plugin-for-aws \
  --set initContainers[0].image=velero/velero-plugin-for-aws:v1.10.0 \
  --set initContainers[0].volumeMounts[0].mountPath=/target \
  --set initContainers[0].volumeMounts[0].name=plugins \
  --wait --timeout 300s

echo "==> 配置定时备份（每日备份 sre-demo + observability namespace）..."
# BackupStorageLocation 就绪后创建 schedule
sleep 10
cat <<EOF | kubectl apply -f -
apiVersion: velero.io/v1
kind: Schedule
metadata:
  name: sre-platform-daily
  namespace: ${VELERO_NS}
spec:
  schedule: "0 2 * * *"   # 每日凌晨 2 点
  template:
    includedNamespaces:
      - sre-demo
      - observability
      - slo-system
    storageLocation: ks3
    ttl: 720h   # 保留 30 天
EOF

echo "==> 验证 backupStorageLocation 就绪..."
kubectl -n "${VELERO_NS}" get backupstoragelocation -w --timeout=60s | head -5

echo ""
echo "✓ Velero 灾备已就绪"
echo "  手动备份：velero backup create sre-demo-test --include-namespaces sre-demo"
echo "  恢复演练：velero restore create --from-backup sre-demo-test"
echo "  定时备份：kubectl -n ${VELERO_NS} get schedule"
