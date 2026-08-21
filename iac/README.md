# 金山云基础设施即代码（IaC / Terraform）

把项目用到的金山云资源声明成 Terraform 代码，体现"基础设施即代码"——资源定义可版本化、可 review、可重建。

## 现状

资源已由金山云控制台手动创建并在运行（KEC 5 节点集群 + KS3 桶 + KECR 仓库）。IaC 的价值不是重建，而是**把现网资源反向导入 Terraform state**，让代码与现网一致，后续变更走 `terraform plan/apply` 而非控制台手点。

## 资源清单（main.tf）

| 资源 | Terraform 类型 | 数量 | 说明 |
|---|---|---|---|
| KEC 实例 | `ksyun_kec_instance.master` | 3 | master 节点（control-plane） |
| KEC 实例 | `ksyun_kec_instance.node` | 2 | node 节点（业务负载，挂载数据盘） |
| KS3 桶 | `ksyun_ks3_bucket.sre_loki` | 1 | Loki chunks（loki/）+ Velero 备份（velero/）共用 |
| KECR 仓库 | `data.ksyun_kcr_repo.project` | 1 | ordersvc/slo-operator/remediator 镜像仓库 |

## 用法

```bash
# 1. 配置凭证（与 KS3 AK/SK 同一组）
export KSYUN_ACCESS_KEY=<AK>
export KSYUN_SECRET_KEY=<SK>

# 2. 初始化
cd iac
terraform init

# 3. 导入现有资源到 state（不重建，需填实际 instance-id / bucket-name）
terraform import ksyun_kec_instance.master["master01"] i-xxxxxxxx
terraform import ksyun_kec_instance.master["master02"] i-yyyyyyyy
terraform import ksyun_kec_instance.master["master03"] i-zzzzzzzz
terraform import ksyun_kec_instance.node["node01"]   i-aaaaaaaa
terraform import ksyun_kec_instance.node["node02"]   i-bbbbbbbb
terraform import ksyun_ks3_bucket.sre_loki sre-platform-ksce

# 4. 校验：plan 应无 diff（代码与现网一致）
terraform plan
# 期望: No changes. Your infrastructure matches the configuration.
```

## 注意

- `image_id`/`vpc_id`/`subnet_id`/`security_group_id` 等需按实际回填到 `variables` 或 `terraform.tfvars`（不入库，`.gitignore` 排除 tfvars/state）。
- 实例规格 `N3.2B`/`N4.4B` 为示例，按实际控制台规格替换。
- 金山云 Terraform Provider 文档：https://registry.terraform.io/providers/ksyun/ksyun
- `terraform.tfstate*` 含资源 ID，已加入 `.gitignore`，不入库。

## 价值（面试讲点）

- 基础设施即代码：资源声明版本化，变更可 review、可回滚，告别控制台手点不可追溯。
- 现网导入：`terraform import` 把已存在资源纳入管理，证明 IaC 可平滑接管存量基础设施。
- 成本可控：规格/数量参数化，`terraform plan` 可预演变更对成本的影响。
