# Alembic 历史旧库迁移与恢复验收

- 状态：accepted（PT-DAT-004、DEV-MVPA-01、G-MVPA-003；仅当前本机 MVP 数据集）
- 日期：2026-08-13
- Red 提交：`abf5bfd`（`test(database): 固定历史旧库迁移守恒契约`）
- Green 提交：`f246e4b`（`impl(database): 接管真实历史旧库迁移链`）
- 数据库：本机 PostgreSQL 18.4，源库脱敏标识 `lanverse-local-pre-alembic`
- 对应需求：[REQ-015](../../requirement/015-AI短剧MVP核心制作能力需求.md)
- 对应设计：[DES-002](../../design/002-人工智能短剧平台架构与技术选型.md)、[DES-MOD-011](../../design/模块设计/011-数据库表与数据生命周期详细设计.md)、[DES-012](../../design/012-AI短剧MVP核心模块拆分与实施范围.md)
- 对应产品任务：[PRD-012 PT-DAT-004](../../prd/012-AI短剧MVP核心制作产品任务.md)
- 对应计划：[PLAN-012 DEV-MVPA-01](../../plan/012-AI短剧MVP核心制作执行计划.md)

## 1. 验收结论

1. 本机真实 `lanverse` 不是空库，也不是 42 表当前 Metadata：迁移前有 38 张业务表、19 行数据且没有 `alembic_version`。使用固定提交 `ce360d25^` 的历史代码加载 Metadata，只读 autogenerate 比较得到 `legacy_tables=38`、`schema_differences=0`，证明它是 Provider 数据底座引入前的已知完整结构。
2. 首版 Alembic baseline 错把 Provider 提交的 4 张新表和 `uq_prod_capability_id_version` 放入一次性 42 表基线，因而严格 adoption 会拒绝真实旧库。当前迁移链已校正为：`95c0d24572c5` 表示 38 表历史基线；`8d9f2a6c4b71` 增加 Provider 四表和 Capability 复合唯一约束。
3. 兼容路径明确且 fail closed：完整 38 表历史库先 stamp 历史 baseline、再在同一事务升级 head；完整当前 42 表无 revision 库可直接采用 head；已经应用首版 42 表 baseline 的库升级时安全跳过重复创建；部分 Provider、未知表、缺索引、缺外键或其他结构漂移均拒绝且不 stamp。
4. 源库先形成仓库外可恢复 custom archive，再用这一确切文件恢复到 `template0` 创建的隔离 `_test` 数据库。恢复前源库与副本的 38 表、19 行、逐表内容聚合 hash 完全相同；副本升级后旧表 hash 仍相同，才允许正式源库执行原子接管。
5. 正式源库结果为 42 张业务表、head `8d9f2a6c4b71`，4 张新增 Provider 表均为 0 行；原 38 张表的 19 行数据聚合 hash 从迁移前到迁移后保持 `48fa8bc48566347ec58384928374ab4ebc5949e82f9e263f96ee3852883678f2`。
6. `alembic current --check-heads` 返回 `8d9f2a6c4b71 (head)`，`alembic check` 返回 `No new upgrade operations detected.`。API、Scheduler、I/O Worker 和 Media Worker 继续只检查 head，不在运行角色中自动升级。

## 2. 备份与恢复证据

| 项目 | 真实结果 |
| --- | --- |
| 备份引用 | `lanverse-pre-alembic-20260813T101950-0800`；仓库外保存，未提交文件或绝对路径 |
| 命令 | `pg_dump -Fc --no-owner --no-privileges` |
| 文件属性 | 153,045 bytes，权限 `0600` |
| SHA-256 | `6ff3f562205eb0785aa6a0c18877f6ea99a9c71e5bf6f81b57cc4efad7346d69` |
| 恢复目标 | `lanverse_mvpa_legacy_20260813_test`，由 `template0` 创建 |
| 恢复命令 | `pg_restore --single-transaction --exit-on-error --no-owner --no-privileges` |
| 恢复前核对 | 源库与副本均为 38 表、19 行、hash `48fa…67f2` |
| 副本升级后 | head `8d9f2a6c4b71`；38 张旧表 hash 不变；4 张新增表均 0 行 |
| 正式源库升级后 | 42 张业务表；38 张旧表/19 行 hash 不变；head 与 Metadata 无漂移 |
| 清理 | 隔离 `_test` 数据库、`/tmp` dump 和历史源码临时目录均已删除；仓库外 0600 恢复备份保留 |

备份摘要和引用可证明当前文件身份，但不代替运维保管、异地副本或定期恢复演练。恢复时应先验证 SHA-256，并恢复到新建隔离库核对后再决定任何回切动作。

## 3. Red → Green 与失败路径

| 阶段 | 命令与结果 |
| --- | --- |
| Red | 新增历史接管、baseline 分段和 downgrade 测试后，定向迁移测试为 `3 failed, 6 passed`：历史库被当前 Metadata 拒绝、baseline 错含 Provider 表、head→baseline 未移除 Provider 表。 |
| Green 定向 | `.venv/bin/python -m pytest tests/unit/test_database_revision_gate.py tests/integration/migrations/test_database_migrations.py -q` 为 `16 passed`。 |
| 静态 | `.venv/bin/ruff check app tests alembic` 通过；`.venv/bin/pyright` 为 `0 errors, 0 warnings`。 |
| 全量后端 | `.venv/bin/python -m pytest` 为 `335 passed, 24 skipped`；skip 均是需显式打开的既有外部契约/性能开关。 |
| 依赖 | `.venv/bin/python -m pip check` 为 `No broken requirements found.` |
| 真实旧库签名 | 固定 `ce360d25^` 历史 Metadata 与源库比较：`legacy_tables=38`、`schema_differences=0`。 |
| 真实恢复 | 同一 0600 archive 恢复副本，恢复前源/副本 hash 相同；副本 adoption 到 head 后 hash 仍相同。 |
| 真实正式接管 | 源库 adoption 到 `8d9f2a6c4b71`，`current --check-heads`、`alembic check` 与旧表 hash 守恒均通过。 |

自动失败契约覆盖：空库必须 upgrade 而不能 adopt；无备份引用拒绝；历史部分 Provider 结构拒绝且 heads 仍为空；未知表、缺索引、缺外键拒绝；旧版完整 42 表 baseline 升级不重复建表；head 降到历史 baseline 后旧业务行保留、Provider 表和复合唯一约束移除；baseline 再降到 base 后业务表为空。

## 4. 范围、风险和不能外推事项

- 当前真实数据量只有 38 表/19 行，本次命令执行很快，但没有记录可用于 SLA 的锁等待和执行时长，不能外推到大表、长事务、并发写入或生产零停机。
- 本次证明备份可恢复且迁移后内容守恒，不定义生产 RPO/RTO、异地备份、加密介质、备份轮换或灾备值班流程。
- 已知历史签名严格固定到 `ce360d25^`。其他环境即使也叫“旧库”，只要结构不完全匹配就会拒绝；不得通过手工 stamp 绕过。
- downgrade 契约用于隔离测试验证 revision 对称性；正式库已有 Provider 数据后，不应把自动 downgrade 当常规回滚。失败优先恢复已验证备份或前滚修复。
- 迁移没有读取或记录账号邮箱、显示名、密码 hash 或其他业务正文；Acceptance 只保存表数、行数、revision 和不可逆聚合 hash。

在以上边界内，PT-DAT-004、DEV-MVPA-01 和 G-MVPA-003 接受。后续每个 MVP-A 新模型仍必须在自己的 DEV 中新增 revision，并重复空库、当前 head、含样本库、失败回滚和 Metadata drift 验证。
