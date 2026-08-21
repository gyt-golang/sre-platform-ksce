# 基于金山云平台的 SRE 可靠性工程平台

> 在**金山云 KEC 真实多节点 K8s 集群（3 master + 2 node，K8s v1.31 / Calico / containerd）**上，用 SRE 方法论端到端构建的微服务可靠性工程平台：从 SLO 定义、可观测性三支柱、错误预算告警 + Alertmanager 链路闭环，到混沌工程验证与故障自愈。接入金山云 **KECR（容器镜像仓库）** 与 **KS3（对象存储）**，覆盖大厂 SRE 岗位全部核心能力点。

## 一、项目背景

传统运维关注"怎么把服务部署起来、出了问题怎么修"；SRE 关注"**如何用软件工程方法让系统天然更稳定、可量化、可自治**"。本项目用 Go 写一个电商订单微服务，在金山云 KEC K8s 集群上完整落地 SRE 可靠性工程方法论，回答四个问题：

1. 系统有多稳？（**SLO / 错误预算**）
2. 不稳了怎么第一时间知道？（**可观测性三支柱 + 燃烧率告警 + Alertmanager 触达 oncall**）
3. 故障来了系统能不能自己扛？（**探针自愈 + HPA 弹性 + 熔断**）
4. 怎么主动验证它真的扛得住？（**混沌工程**）

## 二、架构

```
        ┌──────────────── 金山云 KEC 公网入口（EIP + 安全组放行 NodePort）────────────────┐
        │   10.0.0.182:30088(ordersvc)  30090(Prom)  30300(Grafana)  30686(Jaeger)    │
        └────────┬───────────┬────────┬──────┬───────────────────────────────────────────┘
                 │           │        │      │
   ┌──── 金山云 KEC 5 节点 K8s 集群 (v1.31 / Calico / containerd) ───────────────────────────┐
   │   master01/02/03 (control-plane)        node01/02                                        │
   │                                                                                          │
   │  namespace: sre-demo                  namespace: observability                           │
   │  ┌─────────────────────┐              ┌──────────────────────┐                          │
   │  │  ordersvc (Go) x3   │──/metrics──▶ │ Prometheus           │                          │
   │  │  + /metrics + OTLP  │              │  (SLO 录制规则+告警)  │                          │
   │  │  + liveness/ready   │              └──────────┬───────────┘                          │
   │  │  + HPA (CPU/inflt)  │                         │                                      │
   │  └──────┬──────┬───────┘              ┌──────────▼───────────┐                          │
   │     OTLP│  logs│                       │  Grafana (SLO 大盘)   │                          │
   │         │      │                       └──────────┬───────────┘                          │
   │         ▼      ▼                       ┌──────────▼───────────┐                          │
   │  ┌──────────┐ ┌──────────┐             │  Loki ◀── Promtail    │ ── chunks ──▶ 金山云 KS3 │
   │  │OTel Col. │ │ Promtail │             │  (日志聚合+对象存储)   │   (存储分离)             │
   │  └────┬─────┘ └──────────┘             └──────────────────────┘                          │
   │       ▼                                                                    │
   │  ┌──────────┐                ┌──────────────────────┐                                   │
   │  │  Jaeger  │                │  Chaos Mesh          │                                   │
   │  │ (链路)   │                │  PodKill/Net/CPU     │──▶ 故障注入                       │
   │  └──────────┘                └──────────────────────┘                                   │
   │                                                                                          │
   │  镜像来源：自研 ordersvc → 金山云 KECR 私有仓库（imagePullSecrets）；第三方组件 →       │
   │            containerd 镜像加速（docker.io + ghcr.io）                                    │
   └──────────────────────────────────────────────────────────────────────────────────────────┘
```

## 三、技术栈与对应 SRE 能力

| 模块 | 技术选型 | 体现的 SRE 能力（命中 JD） |
|---|---|---|
| **云基础设施** | **金山云 KEC（5 节点）+ VPC + EIP + 安全组** | 公有云资源编排、网络与安全组规划 |
| **镜像仓库** | **金山云 KECR**（私有仓库 + imagePullSecrets） | 镜像分发、私有仓库治理、凭证管理 |
| **对象存储** | **金山云 KS3**（Loki chunks 后端，S3 兼容） | 存算分离、日志长期归档、云原生存储 |
| 业务服务 | **Go** + 多阶段 Dockerfile + 非 root | Go 语言、容器化、镜像安全 |
| 编排 | K8s Deployment/Service/HPA/Probe | 滚动发布零宕机、探针自愈、容量弹性 |
| Metrics | Prometheus + 自定义指标 | 指标体系设计、SLI 计算 |
| Logs | Loki + Promtail（DaemonSet）+ KS3 | 日志聚合、结构化日志、存储分离 |
| Traces | OpenTelemetry + OTel Collector + Jaeger | 分布式链路追踪、采样治理 |
| 可视化 | Grafana + provisioning | SLO 大盘、错误预算可视化 |
| **SLO 引擎** | Recording Rules + 多窗口多燃烧率告警 | **SLO/错误预算、燃烧率告警（核心考点）** |
| 混沌工程 | Chaos Mesh（PodChaos/NetworkChaos/StressChaos） | 故障演练、可靠性验证 |
| 故障自愈 | liveness/readiness/startup probe + HPA + /admin/fault | 自愈、流量摘除、降级 |
| IaC | 全部声明式 YAML + 一键 bootstrap 脚本 | 基础设施即代码、可复现 |

## 四、SLO 设计（项目核心）

### SLI 与目标
| SLI | 定义 | 目标（30 天窗口） | 错误预算 |
|---|---|---|---|
| 可用性 | 非 5xx 请求占比 | 99.9% | 0.1% ≈ 43 分钟/月 |
| 延迟 | P99 请求延迟 | < 500ms | — |

### 错误预算燃烧率告警（Google SRE 多窗口多燃烧率法）
燃烧率 = 实际错误率 / 允许错误率（0.001）。三档告警（见 `observability/prometheus/prometheus-rule-slo.yaml`）：

| 告警 | 短窗口 & 长窗口 | 阈值 | 含义 | 动作 |
|---|---|---|---|---|
| Page | 5m & 1h | > 14.4 | 1h 消耗 ≥2% 月预算 | 立即介入 |
| Ticket | 30m & 6h | > 6 | 6h 消耗 ~5% 月预算 | 建工单 |
| Budget | 1d | > 1 | 按此速率将耗尽预算 | 排期优化 |

> 多窗口双条件（短窗口负责快速发现 + 长窗口防抖降噪）避免单窗口告警抖动，是 SRE 告警设计的工程精髓。

## 五、混沌工程实验

| 实验 | 类型 | 验证的可靠性假设 |
|---|---|---|
| Pod Kill | PodChaos | 随机杀 Pod，K8s 30s 内自愈，SLO 不击穿 |
| Network Delay 200ms | NetworkChaos | P99 延迟告警触发，服务仍可用 |
| CPU Stress 80% | StressChaos | HPA 触发扩容 3→N，CPU 回落 |
| HTTP 50% 5xx | /admin/fault | 错误预算燃烧率告警触发 |

## 六、一键部署（金山云 KEC 集群）

```bash
# 前置：deploy/scripts/kubeconf-ksce.conf（公网化 kubeconfig）已就绪；
#       安全组放行 30088/30090/30300/30686。
export KSCE_PWD=<master root 密码>
export KECR_PWD=<KECR 登录密码>
bash deploy/scripts/bootstrap-ksce.sh

# 打流量（公网 NodePort）
SRE_HOST=10.0.0.182 bash deploy/scripts/load-test.sh 20 600

# 注入 50% 故障，观察错误预算燃烧率告警
SRE_HOST=10.0.0.182 bash deploy/scripts/chaos-inject-fault.sh 0.5 100
```

访问地址（金山云 KEC 公网 IP）：ordersvc `:30088` / Prometheus `:30090` / Grafana `:30300` / Jaeger `:30686`

## 七、目录结构

```
sre-project/
├── app/                      # Go 微服务（main + handler + metrics + trace + Dockerfile）
├── deploy/
│   ├── manifests/ordersvc.yaml  # Deployment/Service/HPA/Probe/ConfigMap(SLO)
│   └── scripts/                # bootstrap-ksce.sh / load-test / chaos-inject / kubeconf-ksce / 远程与镜像搬运工具
├── observability/
│   ├── prometheus/           # Prometheus + SLO 规则（ConfigMap 版 + PrometheusRule CRD）
│   ├── grafana/              # Grafana + datasource/dashboard provisioning
│   ├── loki/                 # Loki + Promtail DaemonSet（chunks 落金山云 KS3）
│   └── jaeger/               # OTel Collector + Jaeger
├── chaos/experiments.yaml    # Chaos Mesh 三类实验
├── postmortem/               # 事故复盘模板与示例
└── docs/                     # 项目技术文档、runbook
```

## 八、本项目体现的 SRE 思维

- **数据驱动稳定性**：所有决策基于 SLO 指标与错误预算，而非主观感觉。
- **消除琐事**：故障注入/自愈/扩缩容全部自动化，减少人工介入；toil-log/toil-report 量化手动劳动成本驱动自动化优先级。
- **拥抱错误预算**：允许小故障消耗预算，告警只在预算高速燃烧时触发，避免告警疲劳。
- **可观测性先行**：Metrics/Logs/Traces 三支柱齐全，故障可定位、可复盘。
- **主动验证**：混沌工程主动制造故障验证假设，而非等真实故障才发现短板。
- **云原生落地**：在金山云真实多节点集群上跑通，接入 KECR/KS3 云产品，非本地 demo 玩具。

## SRE 运营闭环（持续运营，非一次性定义）

- **SLO 单一事实源**：`observability/slo-spec.yaml` 声明 SLO 目标/SLI/告警阈值，recording/alert rules 由 spec 派生，避免手写 26 条规则易错。
- **SLO 周期报告**：`deploy/scripts/slo-report.py` 查 Prometheus 生成月度 SLO 达成报告（SLI/错误预算剩余/燃烧率趋势），主动回顾而非被动等告警。
- **告警链路闭环**：Alertmanager 分组去重 + inhibit 抑制（Page 抑制 Ticket、混沌期抑制 Page）+ 飞书 webhook 触达 oncall；告警自带 `runbook_url`/`dashboard_url` enrichment。
- **Toil 量化**：`toil-log.py`/`toil-report.py` 量化手动劳动成本，排自动化优先级。实测回填 84min toil、可自动化 76%、可回收 160 元。
- **Postmortem 闭环**：`postmortem/schema.json` + `validate-postmortem.py` 校验必填章节 + 行动项 ≥1 DONE，禁止全 TODO 堆积。
