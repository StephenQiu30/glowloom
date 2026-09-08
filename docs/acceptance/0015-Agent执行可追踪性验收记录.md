# Agent 执行可追踪性验收记录

日期：2026-09-08。关联：[Spec](../requirement/0015-Agent执行清单与尝试追踪需求规格.md)、[Plan](../plan/0015-Agent服务实施计划.md)。

## 验收 Checklist

全部初始未通过。历史测试只作线索；在待发布或本轮实现快照上重新执行后才能勾选。

- [x] AT-01 并发领取和预算原子性。
- [x] AT-02 成功/草案/事件同事务及重复结果。
- [x] AT-03 unknown、过期、迟到、终态与 fence。
- [x] AT-04 签名作用域及旧数据。
- [x] MF-01 策略与清单原子冻结及故障回滚。
- [x] MF-02 输入/配置/模板与摘要固定。
- [x] MF-03 四阶段、必要门和未展开集合。
- [x] MF-04 并发、重放与部署后历史稳定。
- [x] MF-05 新库/旧库迁移及 checksum。
- [x] MF-06 HTTP 身份和签名边界。
- [x] MF-07 清单可用性与重连读取。
- [x] MF-08 不可变、漂移失败与正文隔离。

## 完成后审查 Checklist

- [x] RV-01 逐条核对 Spec → 测试 → 结果，无未声明范围变化。
- [x] RV-02 对事务中途失败、并发竞争、旧记录和迟到结果作反例审查。
- [x] RV-03 审查路由签名复用、查询范围、输出字段与错误脱敏。
- [x] RV-04 审查新增目录职责、单向依赖、测试隔离；无兼容转发或空抽象。
- [x] RV-05 核对已发布迁移文本未改写、临时测试库归属和清理结果。
- [x] RV-06 运行 Ruff、格式、Pyright、完整 Agent 测试；列明跳过条件。
- [x] RV-07 审查最终 diff、文件白名单、凭据/缓存/生成物；保留并逐项记录其他任务修改。
- [x] RV-08 记录提交与远端引用、远端 CI、部署及浏览器状态，禁止把本地通过写成上线验收。

## 证据登记

| 范围 | 命令/输入/快照 | 结果 | 限制 |
|---|---|---|---|
| 发布上一轮 Attempt | eedd57fc；独立 PostgreSQL；pytest -q | 152 passed、8 skipped | 2 项 Temporal、6 项真实 Codex 未启用；Ruff/格式/Pyright 通过 |
| 本轮 Manifest | Red 7 failed；最终工作区 pytest -q | 172 passed、9 skipped | 未调用真实模型、未部署 |

审查结论：本轮 Agent 初始清单与尝试追踪的本地合同通过，未发现当前切片内未解决的阻断项。以下证据只覆盖列明快照；完整 Harness、跨服务与浏览器验收仍未完成。

## 需求与证据映射

测试位于 `agent/tests/creation/`，输入为合成命令/稿件，数据库为本任务新建的专用临时库，未读取业务数据或执行生产迁移。

| Spec | 测试证据 |
|---|---|
| AT-01～04 | `test_attempt_history.py` 的 14 项测试；此前发布快照与本轮独立快照均重跑 |
| MF-01、MF-02、MF-03 | `test_freeze_records_plan_before_any_attempt`、`test_manifest_failure_rolls_back_policy`；验证零步骤/零额度、清单和策略同事务 |
| MF-04 | `test_parallel_freeze_and_replay_ignore_new_template`；8 路并发冻结、重连、模拟模板升级、策略冲突 |
| MF-05、MF-07 | `test_upgrade_does_not_invent_legacy_manifests`、`test_manifest_checksum_drift_is_not_repaired`；升级前后其他 checksum 不变、缺失/漂移失败 |
| MF-06、MF-07 | `test_signed_manifest_availability_and_scope`；签名、查询串、请求体、非法/跨命令路径、not_frozen/recorded |
| MF-08 | `test_manifest_is_immutable_and_drift_fails_closed`、`test_rehashed_manifest_cannot_change_bound_inputs`（3 种输入）；数据库不可改/删、重算 hash 仍不能换输入、HTTP 409 脱敏 |

新增 Manifest 测试共 10 项。首次 7 项失败确认缺少模块/表/路由；后续审查补充 3 项重新计算摘要的反例。测试日志保存在本地交付证据目录 `Documents/Codex/2026-09-08/lanverse-agent-manifest/`，未提交日志或测试数据。

## 最终验证快照与审查

| 快照/检查 | 真实结果 |
|---|---|
| 已推送 Attempt `eedd57fc` 隔离快照 | pytest 152 passed、8 skipped；Ruff、84 文件格式、Pyright 0 errors/0 warnings |
| 本轮 Manifest 加当前并发工作区 | pytest 172 passed、9 skipped；Ruff、94 文件格式、Pyright 0 errors/0 warnings |
| `eedd57fc` + 仅本轮 Manifest 代码的独立快照 | pytest 162 passed、8 skipped；Ruff、88 文件格式、Pyright 0 errors/0 warnings；新建空库迁移通过 |
| 发布 SQL 静态比对 | command、execution、attempt 三份已发布 SQL 与 HEAD 完全相同 |
| 测试库清理 | 两个本任务新建测试库均正常 dropdb，pg_database 查询均为 0；未 force 或操作业务库 |

RV-01/02：逐项反例复核覆盖写入失败回滚、并发重放、历史缺失和输入漂移。RV-03：复用现有 method/raw path/body 授权闭包；内部接口只返回固定字段和摘要，错误不带输入正文。RV-04：新模块有明确消费者，Repository 只依赖迁移定义，execution/API 单向调用 manifest；测试仍在 tests，无新通用目录。RV-05/06：上述真实数据库与完整质量门禁证据成立。RV-07：审查本轮文件与最终 diff，凭据模式扫描未发现匹配；其他任务改动保留在原位，逐项清单随本地交付报告保存。RV-08：已验证 origin/main=eedd57fc67119307d77be7ea2e2a61c476c75187；本轮 Manifest 与 Spec 文档尚未提交/推送。

收尾审查由当前任务执行，未声称独立第二审阅者。新增切片没有改 Go，未运行 Go 门禁；2～3 项 native Temporal、6 项真实 Codex 条件测试跳过。没有镜像构建、业务迁移、Go 公共接口/浏览器全流程或上线验收。[上一轮远端 CI](https://github.com/StephenQiu30/lanverse/actions/runs/34181395027) 在最后查询时仍为 in_progress，不计作通过。后续先完成 CI/跨服务验收，再领取动态展开、多输出或画布消费任务。
