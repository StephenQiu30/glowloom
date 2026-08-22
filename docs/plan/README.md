# Lanverse Plan 索引

> 状态：`proposed`
> 最近审查：2026-08-22
> 文档职责：Plan 把已接受的 PRD 和 Design 转换为可排序、可验证、可停止的实施工作包；Plan 不扩大产品范围，也不把未来能力预建为目录或空服务。

## 1. 计划索引

| 序号 | Plan | 交付对象 | 入口条件 | 状态 |
| --- | --- | --- | --- | --- |
| 000 | [交付路线与依赖总计划](./000-交付路线与依赖总计划.md) | 共通工程基线、切片依赖和发布治理 | Requirement、Design、PRD 可追溯 | proposed |
| A | [手工事实主线实施计划](./001-切片A手工事实主线实施计划.md) | 可手工完成的最小生产闭环 | PRD-000/A accepted | proposed |
| B | [受限 Agent 提案实施计划](./002-切片B受限Agent提案实施计划.md) | 单一脚本分析技能与人工接管 | A verified | proposed |
| C | [真实可恢复生成实施计划](./003-切片C真实可恢复生成实施计划.md) | 一项真实图片和一项真实视频能力 | A verified，Provider Gate 通过 | proposed |
| D | [质量与局部修复实施计划](./004-切片D质量与局部修复实施计划.md) | 可定位问题和最小范围重做 | C verified | proposed |
| E | [审阅与不可变交付实施计划](./005-切片E审阅与不可变交付实施计划.md) | 内部审阅、装配和快照交付 | D verified | proposed |
| F | [团队、画布与组织扩展实施计划](./006-切片F团队画布与组织扩展实施计划.md) | 分别过 Gate 的组织级能力 | E verified + 单项 Gate | proposed |

模块编号表示事实所有权，不表示独立服务数量或平行研发线。

## 2. 工作包标准

每个工作包必须具备以下字段：

- 稳定 WP ID、交付结果和明确依赖；
- 所覆盖的 PRD 功能或验收条件；
- 至少一个可执行验证，以及应归档的预期证据包 ID；
- 失败、超时、重复、并发、权限或降级路径中的适用项；
- 完成条件和触发停止/回到 Design 的条件。

工作包以纵向可验证结果拆分，不按 Controller、Service、Repository、页面等技术层分别计为完成。POJO 风格实体、Go Service、Python Agent、前端 ViewModel 等结构必须服从 Design 中已确定的边界。

## 3. 状态与完成定义

| 状态 | 含义 |
| --- | --- |
| `proposed` | 工作包和验证方式可审阅，尚未开始 |
| `ready` | 上游 PRD/Design accepted，人员、环境和数据 Gate 齐备 |
| `in_progress` | 已先建立失败测试或等价可执行验收，正在实现 |
| `verified` | 工作包证据包通过，且没有未说明的跳过项 |
| `blocked` | 缺少明确外部条件；记录原因、责任方和恢复条件 |

只有一个工作包真正实现并通过对应验证，才能标为 `verified`。代码合并、页面可打开或测试被跳过都不等于完成。

## 4. 证据包命名

Plan 使用 `EPK-<切片>-<主题>` 指代一组预期证据，例如 `EPK-C-RECOVERY`。证据包只是验收清单，不是通过结论；实际执行记录按 [Acceptance 规范](../acceptance/README.md) 创建 `EV-<切片>-<日期>-<场景>`，并引用提交、环境和数据集。

## 5. 顺序与变更控制

1. 先满足 Plan-000 的共通工程 Gate，再进入切片工作包。
2. A 是 B/C 的共同前置；B 与 C 可独立推进，C 不依赖 Agent。
3. D→E 是真实媒体的顺序链；F 的每项能力独立准入、独立验收。
4. PRD 或 Design 改变时，先把受影响工作包恢复为 `proposed` 并废止尚未重跑的旧证据。
5. 不通过兼容层、第二套消息队列、额外微服务、历史 migration 或空抽象来规避当前设计冲突。

## 6. 统一验证层次

- 领域/状态机测试：不变量、合法迁移、重复与并发；
- 真实依赖集成：PostgreSQL、Kafka、Redis、MinIO、Elastic 以及真实 Provider 沙箱；
- 契约测试：唯一 OpenAPI、Go 生成边界、Swagger 展示、`@umijs/openapi` 生成 API 和 Axios `request.ts`；
- 权限与故障注入：跨租户、撤销、依赖中断、超时、进程重启和 unknown；
- 切片 E2E：由用户入口走到稳定业务结果，并保存可复现证据。
