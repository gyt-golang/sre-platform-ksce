#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""生成《基于金山云平台的 SRE 可靠性工程平台》项目报告 Word 版（v3）。

结构：项目背景 / 技术栈 / 项目痛点 / 核心职责 / 最终成果。
用法：python deploy/scripts/gen-report.py
依赖：python-docx（pip install python-docx）
"""
from docx import Document
from docx.shared import Pt, RGBColor
from docx.enum.text import WD_ALIGN_PARAGRAPH
from docx.oxml.ns import qn
from docx.oxml import OxmlElement

doc = Document()

# 全局字体（中文微软雅黑，西文 Calibri）
style = doc.styles['Normal']
style.font.name = 'Calibri'
style.font.size = Pt(10.5)
style.element.rPr.rFonts.set(qn('w:eastAsia'), '微软雅黑')


def set_cn(run, name='微软雅黑'):
    run.font.name = 'Calibri'
    run._element.rPr.rFonts.set(qn('w:eastAsia'), name)


def TITLE(text):
    p = doc.add_paragraph()
    p.alignment = WD_ALIGN_PARAGRAPH.CENTER
    r = p.add_run(text); set_cn(r); r.font.size = Pt(22); r.bold = True
    r.font.color.rgb = RGBColor(0x1F, 0x4E, 0x79)


def SUB(text):
    p = doc.add_paragraph()
    p.alignment = WD_ALIGN_PARAGRAPH.CENTER
    r = p.add_run(text); set_cn(r); r.font.size = Pt(11); r.italic = True
    r.font.color.rgb = RGBColor(0x59, 0x59, 0x59)


def H1(text):
    p = doc.add_heading(level=1)
    r = p.add_run(text); set_cn(r); r.font.size = Pt(16); r.font.color.rgb = RGBColor(0x1F, 0x4E, 0x79)


def H2(text):
    p = doc.add_heading(level=2)
    r = p.add_run(text); set_cn(r); r.font.size = Pt(13); r.font.color.rgb = RGBColor(0x2E, 0x74, 0xB5)


def H3(text):
    p = doc.add_heading(level=3)
    r = p.add_run(text); set_cn(r); r.font.size = Pt(11.5); r.font.color.rgb = RGBColor(0x2E, 0x74, 0xB5)


def P(text, bold=False):
    p = doc.add_paragraph()
    r = p.add_run(text); set_cn(r); r.font.size = Pt(10.5); r.bold = bold
    return p


def BULLET(text, bold_head=None):
    """列表项；bold_head 为前导加粗短语。"""
    p = doc.add_paragraph(style='List Bullet')
    if bold_head:
        r = p.add_run(bold_head); set_cn(r); r.bold = True; r.font.size = Pt(10.5)
        r2 = p.add_run(text); set_cn(r2); r2.font.size = Pt(10.5)
    else:
        r = p.add_run(text); set_cn(r); r.font.size = Pt(10.5)


def A(text):
    """强调说明段（斜体灰）。"""
    p = doc.add_paragraph()
    r = p.add_run(text); set_cn(r); r.font.size = Pt(10); r.italic = True
    r.font.color.rgb = RGBColor(0x59, 0x59, 0x59)


def CODE(text):
    """等宽代码/架构图块。"""
    p = doc.add_paragraph()
    pf = p.paragraph_format
    pf.left_indent = Pt(6); pf.space_before = Pt(4); pf.space_after = Pt(4)
    r = p.add_run(text); r.font.name = 'Consolas'
    r._element.rPr.rFonts.set(qn('w:eastAsia'), 'Consolas')
    r.font.size = Pt(9.5); r.font.color.rgb = RGBColor(0x33, 0x33, 0x33)
    # 浅灰底纹
    shd = OxmlElement('w:shd'); shd.set(qn('w:fill'), 'F2F2F2')
    p.paragraph_format.element.get_or_add_pPr().append(shd)


def table(headers, rows):
    t = doc.add_table(rows=1, cols=len(headers))
    t.style = 'Light Grid Accent 1'
    t.alignment = WD_ALIGN_PARAGRAPH.CENTER
    for i, h in enumerate(headers):
        c = t.rows[0].cells[i]
        c.paragraphs[0].clear()
        r = c.paragraphs[0].add_run(str(h)); set_cn(r); r.bold = True; r.font.size = Pt(9.5)
    for row in rows:
        cells = t.add_row().cells
        for i, v in enumerate(row):
            cells[i].paragraphs[0].clear()
            r = cells[i].paragraphs[0].add_run(str(v)); set_cn(r); r.font.size = Pt(9.5)
    return t


# ============ 封面 ============
TITLE('基于金山云平台的 SRE 可靠性工程平台')
SUB('项目技术报告 · 秋招 SRE 岗位作品')
SUB('金山云 KEC 真实多节点 K8s 集群端到端落地 · 2026-08')
doc.add_paragraph()

# ============ 一、项目背景 ============
H1('一、项目背景')
P('传统运维关注"怎么把服务部署起来、出了问题怎么修"，SRE 关注"如何用软件工程方法让系统天然更稳定、可量化、可自治"。'
  '本项目用 Go 编写一个电商订单微服务 ordersvc，在金山云 KEC 真实 5 节点 Kubernetes 集群上，端到端落地 SRE 可靠性工程方法论，'
  '回答四个核心问题：')
BULLET('系统有多稳？——SLO 定义与错误预算量化可靠性。', '① ')
BULLET('不稳了怎么第一时间知道？——可观测性三支柱 + 多窗口燃烧率告警 + Alertmanager 触达 oncall。', '② ')
BULLET('故障来了系统能不能自己扛？——探针自愈 + HPA 弹性扩缩容。', '③ ')
BULLET('怎么持续改进、不让稳定性退化？——SLO 周期回顾 + Toil 量化 + Postmortem 闭环。', '④ ')
P('它不是"把服务部署起来"，而是用软件工程方法让系统可量化、可自治、可运营。项目已开源：'
  'github.com/gyt-golang/sre-platform-ksce。')

# ============ 二、技术栈 ============
H1('二、技术栈')
P('围绕 SRE 可靠性工程的五大领域选型，全部在金山云真实云环境落地：')
table(
    ['领域', '技术选型', '用途'],
    [
        ['业务服务', 'Go 1.22 + chi router + prometheus client', 'ordersvc 订单微服务，数据面/控制面端口分离，OTel 链路埋点，专用 metrics registry'],
        ['计算平台', '金山云 KEC（5 节点：3 master + 2 node）', 'K8s v1.31.14 + Calico CNI + containerd 2.2.6 + Rocky 9.8，kubeadm 高可用集群'],
        ['SLO 引擎', 'Prometheus recording rules + 声明式 SLO spec', 'slo-spec.yaml 单一事实源，派生 5 窗口×5 指标 recording rules + 多窗口燃烧率告警'],
        ['Metrics', 'Prometheus + Grafana', 'Pod 注解自动发现，16 面板 SLO 大盘（错误预算/燃烧率/4 黄金信号）'],
        ['Logs', 'Loki + Promtail + 金山云 KS3', 'Promtail DaemonSet 采集，Loki chunks + tsdb 索引落 KS3 对象存储，存算分离'],
        ['Traces', 'Jaeger + OpenTelemetry SDK', 'ordersvc 全链路 trace，故障定位到具体 span'],
        ['告警链路', 'Alertmanager + 飞书 webhook', '分组去重 + inhibit 抑制 + 通知触达，告警自带 runbook/dashboard enrichment'],
        ['混沌工程', 'Chaos Mesh 2.7', 'PodKill / NetworkDelay / CPUStress 主动验证系统韧性'],
        ['自愈', 'K8s HPA + liveness/readiness 探针', '流量激增自动扩容，进程挂掉自动重启'],
        ['镜像分发', '金山云 KECR + imagePullSecrets', 'ordersvc 自研镜像推私有仓库，集群经 Secret 拉取'],
    ]
)

# ============ 三、项目痛点 ============
H1('三、项目痛点')
P('传统运维体系在云原生场景下的典型痛点，本项目逐一用 SRE 方法论解决：')
table(
    ['痛点', '传统运维做法', '本项目 SRE 解法'],
    [
        ['可靠性靠感觉', '"我觉得挺稳"，无量化指标', 'SLO（99.9% 可用性 / P99<500ms）+ 错误预算量化，所有决策基于数据'],
        ['告警疲劳', '错误率>5% 等拍脑袋阈值，告警泛滥', '多窗口多燃烧率告警（14.4/6/1 三级），只在预算高速燃烧时触发，Alertmanager 分组抑制'],
        ['告警不触达', '规则 firing 无人知晓', 'Alertmanager → 飞书 webhook，分组去重 + inhibit 抑制，告警带 runbook_url 一键排障'],
        ['日志存不起', '本地盘存日志，容量受限、Pod 重启即丢', 'Loki 接金山云 KS3 对象存储，chunks + 索引落 KS3，存算分离、近无限容量、便宜归档'],
        ['故障靠人救', '手动 kubectl 排查、手动扩容', 'HPA 自动扩缩容 + 探针自愈 + Toil 量化驱动自动化，可回收成本数据驱动排期'],
        ['复盘走过场', 'Postmortem 写完即束之高阁', 'schema 校验必填章节 + 行动项强制 ≥1 DONE 闭环，禁止全 TODO 堆积'],
        ['SLO 定完即忘', '定义一次后再不回顾', 'slo-spec 单一事实源 + slo-report.py 周期生成达成报告，主动回顾调整'],
        ['本地 demo 玩具', 'kind 单机，无法体现真实网络/多节点', '金山云 KEC 5 节点真实集群 + KECR/KS3 云产品，真实云环境落地'],
    ]
)

# ============ 四、核心职责 ============
H1('四、核心职责')

H2('4.1 SLO 定义与错误预算')
P('用 slo-spec.yaml 作为单一事实源声明 SLO（声明式 spec），recording 与 alert rules 由 spec 派生，'
  '避免手写 26 条规则易错。改 SLO 目标只改 spec，规则随之对齐。')
BULLET('可用性 SLO 99.9%：5xx 视为错误，允许错误预算 0.1%/月（≈43.2 分钟）。', '• ')
BULLET('延迟 SLO P99<500ms：99% 请求延迟达标，允许 1% 违规。', '• ')
BULLET('5 窗口 recording rules：error_ratio / burn_rate 各 5m/30m/1h/6h/1d，覆盖短中长期趋势。', '• ')
BULLET('多窗口多燃烧率告警：Page（5m&1h, >14.4，1h 耗 ≥2% 月预算）/ Ticket（30m&6h, >6）/ Budget（1d, >1）。', '• ')
A('阈值有理论依据：14.4 = 1 小时消耗 2% 月预算，从 SLO 推导而非拍脑袋。多窗口天然分级，避免告警疲劳。')

H2('4.2 可观测性三支柱')
H3('Metrics —— Prometheus + Grafana')
P('通过 kubernetes_sd_configs + Pod 注解自动发现 ordersvc:9090/metrics，加载 SLO recording/alert rules。'
  'Grafana 16 面板 SLO 大盘覆盖错误预算、燃烧率、USE/RED 四黄金信号。')
H3('Logs —— Loki + Promtail + 金山云 KS3（存算分离）')
P('Promtail 以 DaemonSet 采集每个节点容器日志，解析 ordersvc 结构化 JSON 日志打 level 标签。'
  'Loki chunks + tsdb 索引落金山云 KS3 对象存储（S3 兼容 API），实现存算分离：')
BULLET('容量近乎无限、按需扩容，不受单节点磁盘限制。', '• ')
BULLET('Pod 重启/节点故障日志不丢，对象存储独立于集群。', '• ')
BULLET('Loki 无状态化可水平扩展，本地仅存 wal/cache。', '• ')
BULLET('对象存储便宜，适合海量日志长期归档（保留 168h）。', '• ')
A('踩坑：Loki 3.x S3 字段名是 bucketnames/s3forcepathstyle（非老式 bucket_name）；'
  'Promtail 需配 ServiceAccount RBAC + __path__ relabel，否则静默不采集日志。实测 KS3 出现 100+ chunks 对象。')

H3('Traces —— Jaeger + OpenTelemetry')
P('ordersvc 用 OTel SDK 埋点，全链路 trace 上报 Jaeger，故障可定位到具体 span（如 /order 处理慢在哪一段）。')

H2('4.3 告警链路闭环（Alertmanager）')
P('Prometheus 告警规则 firing 后推送 Alertmanager，完成分组去重、抑制、路由通知，闭环"告警如何触达 oncall"：')
BULLET('group_by [alertname, slo, severity] 聚合同源告警，5 窗口×多指标同时 firing 只通知一次。', '• ')
BULLET('inhibit_rules：同 SLO 下 Page 抑制 Ticket；混沌演练期 chaos=true 抑制 Page，演练故障不打扰真实 oncall。', '• ')
BULLET('飞书 webhook 通知，send_resolved=true 恢复时也通知。', '• ')
BULLET('Enrichment：4 个业务告警带 runbook_url（GitHub raw）+ dashboard_url（Grafana deep link），一键跳转排障。', '• ')
A('实测：注入 fail=1.0 故障，burn_rate5m 达 69.5，OrdersvcHighErrorRatePage firing，'
  'Alertmanager 收到带 runbook_url 的告警，链路端到端验证通过。')

H2('4.4 混沌工程验证（Chaos Mesh）')
P('主动制造故障验证系统韧性，而非等真实故障才发现短板：')
BULLET('PodKill：验证 K8s 自愈（Pod 被杀后自动重建、流量自动转移）。', '• ')
BULLET('NetworkDelay：注入网络延迟，验证延迟 SLO 告警与 P99 监控。', '• ')
BULLET('CPUStress：压测 CPU，验证 HPA 弹性扩容是否及时触发。', '• ')
BULLET('应用层故障注入：ordersvc /admin/fault 热更新失败率与延迟，触发燃烧率告警验证 SLO 链路。', '• ')

H2('4.5 K8s 自愈')
P('ordersvc Deployment 配置 liveness/readiness 双探针 + HPA：')
BULLET('livenessProbe：进程死锁/崩溃自动重启容器。', '• ')
BULLET('readinessProbe：未就绪自动从 Service Endpoints 摘除，流量不打进不健康实例。', '• ')
BULLET('HPA：基于 CPU/自定义指标自动扩缩容，应对流量激增。', '• ')

H2('4.6 变更安全：金丝雀发布与自动回滚（Argo Rollouts）')
P('变更是 80% 故障的来源。用 Argo Rollouts 把 ordersvc 从原生 Deployment 升级为 Rollout，'
  '实现渐进式交付 + SLO 驱动自动回滚，把"发布是否健康"从人肉看大盘变成指标驱动的自动决策：')
BULLET('金丝雀策略：20% → 40% → 60% → 100% 流量逐步切到新版本，每步 pause 跑 AnalysisTemplate，故障被限制在 20% 流量内。', '• ')
BULLET('AnalysisTemplate 指标驱动：每步查 Prometheus 错误率（≤5%）与 P99 延迟（≤800ms），超阈值自动 abort 回滚到上一个 stable ReplicaSet——判断依据是 SLO 指标而非 Pod Ready。', '• ')
BULLET('版本区分：Dockerfile 加 BUGGY ARG + ldflags 注入，handler.go 在 buggy=true 时 50% 返回 500，构建 v3-bad 坏版本验证回滚；正式版本不编译此 flag。', '• ')
BULLET('canary-deploy.sh 脚本化：远程构建推 KECR + patch image 触发金丝雀 + 观察进度。', '• ')

H2('4.7 SRE 运营闭环（持续改进）')
P('让稳定性"可运营"而非一次性定义，覆盖持续改进的全链路：')
H3('SLO 周期报告')
P('slo-report.py 查询 Prometheus recording rules，生成月度 SLO 达成报告（SLI / 错误预算剩余 / 燃烧率趋势 / 调整建议），'
  '主动回顾而非被动等告警。')
H3('Toil 量化驱动自动化')
P('toil-log.py 记录每次手动干预（任务/耗时/可自动化），toil-report.py 聚合算成本（元/分钟）排自动化优先级。'
  '实测回填 5 条 toil：总 84 分钟、可自动化 76%、可回收成本 160 元，ks3-integration-debug 列 P0 优先自动化。')
H3('Postmortem 闭环')
P('schema.json 定义必填章节（概要/影响/根因/行动项），validate-postmortem.py 校验，'
  '行动项强制 ≥1 个 DONE/IN-PROGRESS，禁止全 TODO 堆积。本次混沌演练的"打 chaos label 抑制告警"行动项已 DONE，'
  '落地为 Alertmanager inhibit_rules，形成"复盘 → 行动 → 落地 → 闭环"。')

# ============ 五、最终成果 ============
H1('五、最终成果')

H2('5.1 集群与部署')
CODE('金山云 KEC 5 节点 K8s 集群（3 master + 2 node）\n'
     '  K8s v1.31.14 / Calico / containerd 2.2.6 / Rocky 9.8\n'
     '  接入金山云 KECR（私有镜像仓库）+ KS3（Loki 对象存储）\n\n'
     '访问入口（NodePort）：\n'
     '  ordersvc     :30088      Prometheus :30090    Grafana :30300\n'
     '  Jaeger       :30686      Alertmanager :30093')

H2('5.2 实测验证数据')
table(
    ['验证项', '方法', '结果'],
    [
        ['SLO 告警链路', '注入 fail=1.0 + 持续打流量', 'burn_rate5m=69.5，OrdersvcHighErrorRatePage firing，Alertmanager 收到带 runbook_url 告警'],
        ['KS3 存算分离', 'Loki 接 KS3 + flush + list bucket', 'KS3 出现 100+ fake/ chunks 对象，Loki series 返回 ordersvc 日志流'],
        ['告警分级抑制', '混沌演练 chaos=true', 'info 告警路由 null-receiver，演练期 Page 被抑制不打扰 oncall'],
        ['Toil 量化', '回填 5 条手动干预', '总 84min，可自动化 76%，可回收成本 160 元'],
        ['Postmortem 闭环', 'validate-postmortem.py 校验', '章节齐全 + 行动项 ≥1 DONE，闭环通过'],
        ['SLO 达成报告', 'slo-report.py 查 Prom', '生成月度报告：SLI/错误预算剩余/燃烧率趋势/调整建议'],
        ['金丝雀自动回滚', '发 v3-bad(50% 500) + loadgen 打 canary', '错误率升至 0.33，AnalysisRun Failed，v3-bad RS 缩 0，自动回滚 v2 恢复 Healthy'],
    ]
)

H2('5.3 落地的 SRE 核心能力')
BULLET('SLO 与错误预算：单一事实源 spec + 多窗口燃烧率告警。', '✓ ')
BULLET('可观测性三支柱：Prometheus/Loki(+KS3)/Jaeger 全覆盖。', '✓ ')
BULLET('告警链路闭环：Alertmanager 分组/抑制/触达 + runbook enrichment。', '✓ ')
BULLET('混沌工程：Chaos Mesh 三类实验主动验证韧性。', '✓ ')
BULLET('故障自愈：HPA + 双探针，被动自愈。', '✓ ')
BULLET('变更安全：Argo Rollouts 金丝雀 + AnalysisTemplate SLO 驱动自动回滚。', '✓ ')
BULLET('运营闭环：SLO 周期报告 + Toil 量化 + Postmortem 闭环，持续改进。', '✓ ')
BULLET('云原生落地：金山云 KEC + KECR + KS3 三层云产品真实接入。', '✓ ')

H2('5.4 交付物')
BULLET('项目代码与文档：github.com/gyt-golang/sre-platform-ksce（开源）', '• ')
BULLET('一键部署脚本：bootstrap-ksce.sh（凭证经环境变量注入，不入库）', '• ')
BULLET('项目技术文档：docs/项目技术文档.md（含实测数据与面试讲解稿）', '• ')
BULLET('SLO 报告：docs/slo-report-2026-08.md', '• ')
BULLET('Toil 报告：docs/toil-report-2026-08.md', '• ')
BULLET('面经脚本：gen-interview.py 生成面试问答 .docx', '• ')

doc.save(r'C:\Users\KC\Desktop\秋招\简历\简历\设计师修改\基于金山云平台的SRE可靠性工程平台-项目报告-v3.docx')
print('OK 报告已生成（v3，新结构：项目背景/技术栈/项目痛点/核心职责/最终成果）')
