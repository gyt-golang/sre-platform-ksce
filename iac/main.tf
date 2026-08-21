# 金山云基础设施 IaC（Terraform）
#
# 把项目用到的金山云资源声明成代码，体现"基础设施即代码"。
# 资源已由控制台手动创建，用 `terraform import` 导入现有资源到 state（不重建），
# 之后 `terraform plan` 应无 diff，证明代码与现网一致。
#
# 金山云 Terraform Provider：ksyun（registry.terraform.io/providers/ksyun/ksyun）
# 前置：export KSYUN_ACCESS_KEY=<AK> KSYUN_SECRET_KEY=<SK>（与 KS3 AK/SK 同一组）
# 用法：
#   terraform init
#   terraform import ksyun_kec_instance.master01 i-xxxxxxxx
#   terraform import ksyun_ks3_bucket.sre_loki <bucket-name>
#   terraform plan   # 期望 no changes
terraform {
  required_providers {
    ksyun = {
      source  = "ksyun/ksyun"
      version = "~> 1.5"
    }
  }
}

provider "ksyun" {
  region = var.region
}

variable "region" {
  description = "金山云 region"
  type        = string
  default     = "cn-beijing-6"
}

variable "availability_zone" {
  description = "可用区"
  type        = string
  default     = "cn-beijing-6a"
}

variable "instance_type_master" {
  description = "master 节点规格（3 master，control-plane）"
  type        = string
  default     = "N3.2B"   # 2C8G，按实际替换
}

variable "instance_type_node" {
  description = "node 节点规格（2 node，业务负载）"
  type        = string
  default     = "N4.4B"   # 4C16G，按实际替换
}

variable "image_id" {
  description = "基础镜像 ID（Rocky Linux 9 / Ubuntu 等）"
  type        = string
  default     = "IMG-..."   # 实际镜像 ID，import 后回填
}

variable "vpc_id" {
  description = "VPC ID"
  type        = string
  default     = "..."
}

variable "subnet_id" {
  description = "子网 ID"
  type        = string
  default     = "..."
}

variable "security_group_id" {
  description = "安全组 ID（放行 30088/30090/30300/30686/6443）"
  type        = string
  default     = "..."
}

variable "ks3_bucket_name" {
  description = "KS3 桶名（Loki chunks + Velero 备份共用，不同 prefix）"
  type        = string
  default     = "sre-platform-ksce"
}

variable "kecr_repo" {
  description = "KECR 仓库名"
  type        = string
  default     = "czmtest/gyt_test"
}

# ============================================================
# KEC 实例：3 master + 2 node
# 资源已存在，用 terraform import ksyun_kec_instance.master01 <instance-id> 导入
# ============================================================
locals {
  master_instances = ["master01", "master02", "master03"]
  node_instances   = ["node01", "node02"]
}

resource "ksyun_kec_instance" "master" {
  for_each              = toset(local.master_instances)
  instance_name         = each.value
  instance_type         = var.instance_type_master
  image_id              = var.image_id
  subnet_id             = var.subnet_id
  security_group_id     = [var.security_group_id]
  system_disk_size      = 50
  purchase_time         = 1
  charge_type           = "Monthly"
}

resource "ksyun_kec_instance" "node" {
  for_each              = toset(local.node_instances)
  instance_name         = each.value
  instance_type         = var.instance_type_node
  image_id              = var.image_id
  subnet_id             = var.subnet_id
  security_group_id     = [var.security_group_id]
  system_disk_size      = 100
  data_disk_gb          = 100   # node 节点挂载数据盘（Loki/容器镜像）
  purchase_time         = 1
  charge_type           = "Monthly"
}

# ============================================================
# KS3 桶：Loki chunks（loki/）+ Velero 备份（velero/）
# terraform import ksyun_ks3_bucket.sre_loki <bucket-name>
# ============================================================
resource "ksyun_ks3_bucket" "sre_loki" {
  bucket = var.ks3_bucket_name
  acl    = "private"
}

# ============================================================
# KECR 镜像仓库（项目镜像 ordersvc/slo-operator/remediator 推送至此）
# ============================================================
# KECR 仓库通常通过控制台创建，Terraform 支持有限，此处声明为 data source 引用。
data "ksyun_kcr_repo" "project" {
  name = var.kecr_repo
}

# ============================================================
# 输出：master01 公网 IP（kubectl kubeconfig 用）、KS3 桶名、KECR 仓库
# ============================================================
output "master01_public_ip" {
  description = "master01 公网 IP（kubectl 访问入口）"
  value       = ksyun_kec_instance.master["master01"].public_ip
}

output "ks3_bucket" {
  value = ksyun_ks3_bucket.sre_loki.bucket
}

output "kecr_repo" {
  value = data.ksyun_kcr_repo.project.name
}
