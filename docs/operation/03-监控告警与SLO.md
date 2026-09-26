# OPS-03 监控告警与 SLO

| 项 | 内容 |
| --- | --- |
| 文档状态 | 草案，待评审（2026-09-25） |
| 上游 | [REQ-02 §1、§2、§3、§8](../requirement/02-非功能需求规格.md)、[DES-04 工作流](../design/04-工作流与生成操作.md)、[OPS-01 环境与部署](01-环境与部署.md)；功能 [REQ-34 系统健康视图](../requirement/34-V2与待定需求池.md) |
| 范围 | SLI / SLO 与错误预算、指标目录、日志与链路、看板、告警规则与通知、值守安排 |

## 1. 可观测栈

```text
各服务（OTel SDK：traces、metrics；Zap / Python 结构化日志 → stdout）
   → OTel Collector（ops-1）
        ├─ 指标 → Prometheus（保留 30 天）→ Grafana
        ├─ 链路 → Tempo（保留 7 天，采样：错误与慢请求 100%，其余 20%）
        └─ 日志 → Loki（保留 30 天；审计另存数据库 3 年）
Temporal Server 指标、PostgreSQL / Redis / Kafka 托管服务监控（云监控导出或 exporter）→ Prometheus
Alertmanager → 通知渠道（§6）
外部拨测（云拨测或 GitHub Actions 定时）→ /healthz、登录页
```

所有日志、指标、链路共享 `trace_id`、`project_id`、`operation_id` 维度（REQ-02 OBS-01、OBS-02）；日志脱敏见 DES-07 §14。

## 2. SLI 与 SLO

服务时段：每日 9:00–24:00（REQ-02 AVL-01）；窗口：自然月。

| SLO | SLI 定义 | 目标 | 来源 |
| --- | --- | --- | --- |
| 可用性 | 外部拨测成功次数 / 总次数（每分钟一次，服务时段） | ≥ 99.5% | AVL-01 |
| API 成功率 | 非 5xx 响应 / 全部响应（排除 `dependency_unavailable` 中供应商原因） | ≥ 99.5% | AVL-01 |
| 读接口延迟 | P95 | ≤ 300 ms | PERF-02 |
| 写命令延迟 | P95 | ≤ 500 ms | PERF-02 |
| 状态推送延迟 | 落库到 SSE 发出的 P95（服务端埋点） | ≤ 2 s | PERF-03 |
| 调度延迟 | 确认到向供应商提交的 P95 | ≤ 5 s | PERF-09 |
| 接管延迟 | 供应商成功到候选可见的 P95 | ≤ 30 s | PERF-09 |
| 事件传播 | Outbox 写入到消费者处理完成的 P95 | ≤ 5 s | REL-07 |
| 重复计费 | 同一 `provider_request_key` 产生 > 1 次计费 | 0 | REL-02 |
| 账本平衡 | 每日对账差异 | 0 | REL-08 |

**错误预算策略**：月度可用性错误预算约 0.5% × 465 小时 ≈ 2.3 小时。当月消耗 > 50% 时暂停非紧急功能发布，优先修复稳定性问题；> 100% 时只允许修复类发布，并做复盘。重复计费与账本差异没有预算，出现即 S1（PLN-02 §7）。

## 3. 指标目录

### 3.1 业务与生成链路（自定义指标）

| 指标 | 类型 | 标签 | 说明 |
| --- | --- | --- | --- |
| `lv_operation_total` | counter | `capability`、`provider`、`model`、`outcome`（completed / failed / cancelled / reused） | 生成结果 |
| `lv_operation_status` | gauge | `status` | 各状态数量（由定时查询更新） |
| `lv_operation_duration_seconds` | histogram | `capability`、`provider`、`phase`（schedule / provider / ingest / total） | 各阶段耗时 |
| `lv_operation_unknown_total` | counter | `provider` | 进入 unknown |
| `lv_operation_manual_open` | gauge | — | 待人工核对数量；附最久等待时长 `lv_operation_manual_oldest_seconds` |
| `lv_provider_call_total` | counter | `provider`、`action`、`outcome`、`http_status` | 供应商调用 |
| `lv_provider_call_seconds` | histogram | `provider`、`action` | 调用延迟 |
| `lv_billing_reserved_micros` / `lv_billing_settled_micros` | counter | `provider`、`capability` | 预留 / 结算金额 |
| `lv_billing_reconcile_diff` | gauge | — | 每日对账差异条数 |
| `lv_budget_used_ratio` | gauge | `project_id` | 预算使用率（只对活跃项目） |
| `lv_batch_paused_total` | counter | `provider`、`reason` | 批量熔断 |
| `lv_skill_runs_total` | counter | `skill`、`version`、`outcome` | Harness 运行 |
| `lv_skill_repair_rounds` | histogram | `skill` | 修复轮次 |
| `lv_moderation_total` | counter | `kind`、`result` | 审核结果 |
| `lv_render_duration_seconds` | histogram | — | 渲染耗时 |
| `lv_realtime_push_lag_seconds` | histogram | — | 推送延迟 |
| `lv_outbox_backlog` | gauge | — | 未投递 Outbox 条数与最老时长 |
| `lv_consumer_lag` | gauge | `consumer` | Kafka 消费延迟 |
| `lv_lineage_propagation_seconds` | histogram | — | 过期传播耗时 |

### 3.2 平台指标

HTTP RED（otelgin：请求数、错误数、延迟，按路由模板）；Go runtime（goroutine、GC、内存）；Python 进程；Temporal（工作流 / Activity 失败、任务队列积压 `temporal_task_queue_backlog`、调度到开始延迟）；PostgreSQL（连接、慢查询、复制延迟、磁盘）；Redis（内存、连接、命中率）；Kafka（broker、分区、消费组延迟）；主机（CPU、内存、磁盘、网络）；前端 RUM（Web Vitals：LCP、INP、CLS，按页面）。

## 4. 日志与链路

- 日志级别：生产 `info`；单个请求可通过 `X-Debug` 头（仅管理员）临时提升到 `debug`。
- 必带字段：`ts`、`level`、`service`、`env`、`trace_id`、`span_id`、`request_id`、`user_id`、`project_id`、`operation_id`（有则带）、`msg`、`error`（含错误链）。
- 链路：API → 工作流（Temporal 拦截器传播上下文）→ Activity → Agent 服务 → 供应商 HTTP 调用；供应商调用 span 带 `provider`、`model`、`request_key`、`outcome`。

## 5. 看板（Grafana）

| 看板 | 内容 |
| --- | --- |
| 总览 | SLO 达成与错误预算、可用性、API 错误率与延迟、在线用户、今日生成数与费用 |
| 生成链路 | 各状态数量、各阶段耗时、按供应商成功率与延迟、unknown / manual、批量熔断、队列积压 |
| 成本 | 按供应商 / 能力的预留与结算、预算使用率 Top 10 项目、对账差异 |
| 事件与实时 | Outbox 积压、消费延迟、推送延迟、过期传播耗时 |
| Agent 服务 | Skill 成功率、修复轮次、token 与费用、审核结果 |
| 媒体 | 接管耗时、渲染耗时、media 队列积压、FFmpeg 失败 |
| 基础设施 | 主机、PostgreSQL、Redis、Kafka、Temporal |
| 前端 | Web Vitals、前端错误 |

产品内的“系统健康视图”（REQ-34）读取其中关键指标，阈值与本文告警一致。

## 6. 告警规则

| 级别 | 通知方式 | 响应 |
| --- | --- | --- |
| P1 | 电话 / 短信 + IM 群 | 立即（服务时段 15 分钟内确认） |
| P2 | IM 群 | 1 小时内 |
| P3 | IM 群（汇总） | 下一个工作日 |

| 告警 | 条件 | 级别 | 处置入口 |
| --- | --- | --- | --- |
| 服务不可用 | 外部拨测连续 3 次失败 | P1 | OPS-04 §5 RB-01 |
| API 错误率高 | 5xx 比例 5 分钟 > 5% | P1 | RB-01 |
| 重复计费嫌疑 | 同一 `provider_request_key` 出现第二次成功 submit 或第二笔结算 | P1 | RB-06 |
| 账本差异 | 每日对账差异 > 0 | P1 | RB-06 |
| 凭据疑似泄露 | 供应商侧异常用量 / gitleaks 命中 / 日志扫描命中 | P1 | RB-07 |
| 备份失败 | 最近一次全量或 WAL 归档失败 | P1 | OPS-04 §2 |
| 延迟超标 | API P95 15 分钟 > 1 s | P2 | RB-01 |
| 供应商失败率高 | 某供应商 15 分钟失败率 > 20%（且样本 ≥ 10） | P2 | RB-02 |
| unknown 堆积 | `unknown` + `reconciling` > 10，或单条 > 30 分钟 | P2 | RB-03 |
| 待人工核对超时 | `manual` 单条 > 24 小时 | P2 | RB-03 |
| 队列积压 | 任一任务队列积压 > 100 持续 10 分钟 | P2 | RB-04 |
| Outbox 积压 | 未投递 > 1,000 或最老 > 5 分钟 | P2 | RB-05 |
| 消费延迟 | 任一消费者延迟 > 5 分钟 | P2 | RB-05 |
| 数据库 | 连接 > 80%、复制延迟 > 30 s、磁盘 > 80% | P2 | RB-08 |
| 渲染失败（V2） | 1 小时内失败 ≥ 3 | P2 | RB-09 |
| 预算超支 | 任一项目 `is_overrun = true` | P2 | 产品负责人 |
| 证书即将过期 | < 14 天 | P3 | — |
| 磁盘 / 内存 | 主机 > 85% | P3 | — |
| 依赖漏洞 | 每日扫描出现新的高危 | P3 | OPS-02 §9 |

告警规则以代码维护（根目录 `observability/alerts/*.yml`），变更走 PR。每条告警附处置入口链接（Runbook）。

## 7. 值守

- 团队 1–2 人：服务时段内由研发轮值；夜间（0:00–9:00）只对 P1 电话通知，非 P1 次日处理。
- 每周复盘告警：无效告警调整阈值或删除；同类告警一周内 ≥ 3 次必须建改进任务。

## 8. 待确认

| # | 问题 | 默认处理 |
| --- | --- | --- |
| MO-Q1 | 告警通知渠道 | 默认飞书群机器人 + 云短信（P1） |
| MO-Q2 | 夜间 P1 值守人 | 产品负责人与研发轮换 |
