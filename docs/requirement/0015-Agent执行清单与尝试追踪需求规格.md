# Agent 执行清单与尝试追踪需求规格（Spec）

日期：2026-09-08。范围：0015 的 Agent 执行可追踪性增量。本文在下一切片编码前建立，不将上一轮代码反称为按本 Spec 先行开发。

## 依据与事实

[Design §8](../design/0015-Harness能力缺口评审与创作画布闭环详细设计.md)、[PRD](../prd/0015-Agent执行可追踪性产品需求.md) 是范围依据。代码基线为 main `53321b95`；工作区有上一轮 Attempt 增量及其他任务修改，发布须分离。Go 保持正式资源与采纳 Owner，Python 只持有可信执行事实与候选；测试独立位于 `agent/tests/`。

## 可测试契约

| ID | 必须满足的行为 | 验证方式 |
|---|---|---|
| AT-01 | 同一步骤并发领取只增加一次 Attempt 和一次额度；Attempt 与步骤当前指针原子写入 | 真实 PostgreSQL 并发及插入失败 |
| AT-02 | 成功尝试、草案、结果事件绑定同一身份并一起提交；重复完成不改历史 | 事务回滚、重复返回、跨 Attempt 引用拒绝 |
| AT-03 | unknown/过期不退还额度、不自动重试；原始 fence 与终态不可改，迟到/伪造尝试不能提交 | 状态竞争、过期、数据库约束 |
| AT-04 | 签名查询只读且限定 command/step；历史步骤无账时明确不可用 | HTTP、重连、旧库迁移 |
| MF-01 | 首次 freeze 将执行策略和初始 Manifest 原子保存；任何一方失败均回滚；清单存在先于首次 reserve | 事务故障、首次调用前查询 |
| MF-02 | Manifest 固定 command/run、payload_hash、source revision/content_hash、release_hash、call_limit、模板版本及模板/清单摘要 | 确定性摘要、输入漂移检查 |
| MF-03 | 模板含 map_manuscript、analyze_episode、build_world、direct_scene 四阶段及每阶段必要审阅门；集/场为未展开集合，不伪造数量、StepInstance 或正式资源 ID | 模板合同测试 |
| MF-04 | 并发/重复 freeze 只产生一份清单；已有运行重放读取已保存模板，不用当前模板覆盖历史；策略不一致报 execution_policy_conflict | 并发、重连、模拟模板升级 |
| MF-05 | 独立追加迁移，已发布 SQL/checksum 不变；旧执行不回填虚构清单；缺失/漂移迁移导致 ready 失败 | 新库、升级、checksum 故障 |
| MF-06 | 签名 GET /internal/creation/commands/{command_id}/manifest，空 body、无 query；不存在 command 为 404，非法签名为 401，非法 UUID/body 为 422 | 精确 method/path/body 签名 HTTP |
| MF-07 | 返回 schema=creation-manifest-production、command_id、run_id、availability 和 manifest；存在为 recorded；未冻结为 not_frozen；旧执行缺失为 unavailable；后两种 manifest=null | HTTP、历史升级、重连 |
| MF-08 | 清单只读且不可覆盖，读取核验摘要及运行输入/策略；漂移报 creation_manifest_conflict，HTTP 409；不得返回内容正文或调用模型 | 数据篡改、不可变约束、信息边界 |

## 数据与目录边界

Manifest 存储在 Agent 专有库，独立表以 command_id 为主键并引用 creation_executions，保存 JSON、manifest_hash、created_at。Manifest JSON 包括模板及 template_hash；manifest_hash 对 JSON 使用现有规范化摘要函数计算，不自包含。静态描述不冒充实时 execution/review/adoption 状态。

职责文件限于 `app/creation/manifest.py`（模板、摘要和持久读写）、`manifest_schema.py`（追加迁移）、`manifest_api.py`（签名只读路由），接入已有 repository、execution.freeze 和 API 组合根。不得让 repository 导入 execution 形成循环；不要新建通用框架或空目录。具体字段以测试与严格校验实现固定。

## 失败与演进

清单不能代替调用授权、审批回执、实际状态或预算检查。旧执行重放可保持原策略，但不得补造历史清单。记录不一致必须报错，不能静默重编译。以后新增动态 PlanExpansion 时须追加版本并绑定正式采纳回执，本轮禁止将模型候选直接展开为正式执行范围。

## 完成定义

每个 ID 都要有真实测试/证据映射；Plan 完成不等于验收通过。完成后逐项审查合同、事务、权限、历史数据、依赖方向、部署边界和 Git 差异。未执行的真实模型、Temporal、Go、浏览器和远端 CI 单列，不抵扣本轮本地证据。详见 [执行 Checklist](../plan/0015-Agent服务实施计划.md) 和 [验收审查 Checklist](../acceptance/0015-Agent执行可追踪性验收记录.md)。
