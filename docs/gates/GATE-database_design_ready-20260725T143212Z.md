---
gate_scope: database_design_ready
result: passed
input_versions: [DESIGN-06/1.0.0@2ffb359227088ccf6ee0d970749aa2aa333498cb, PLAN-03/1.0.0@9e27f2ac60bece99ff7c3fde9e26981932289309]
owner: Lanverse Architecture
reviewers: [Product Owner (current task approval), Codex (Database Architecture and QA executable audit)]
decided_at: 2026-07-25T14:32:12Z
supersedes: GATE-database_design_ready-20260725T113947Z
gaps: []
evidence: [git:93fa483, sql-bundle-sha256:5322bbc0189358f71300b37bd99be9f5e396e7682f2fb4bc4ac3f1e90cebcf3f, test:T-DATABASE-DESIGN-GATE, gate:GATE-design_entry-20260725T143211Z]
next_stage: implementation_start_review
---

# Database design ready 可执行门禁记录

## 1. 结论

DESIGN-06、PLAN-03 与根 SQL bundle 没有发生内容变化。本次用已提交的 P03-T001 可执行测试取代一次性统计描述，并纠正上一记录把 `272 physical columns` 误写为 `471 columns` 的证据标签；Schema bundle hash、20 表、51 FK、22 JSONB 和 57 indexes 均未变化，结论为 `passed`。

## 2. Schema exact-set

| 检查项 | 结果 | 可执行证据 |
| --- | --- | --- |
| 文件/表/列 | passed | 20 个编号文件、20 张 `public` 表、272 个物理列逐名 exact-set |
| 关系与索引 | passed | 51 个内联 FK、57 个所属表索引、13 个 partial unique indexes |
| JSONB | passed | 22 个 JSONB 物理列与 DESIGN-06 具名映射一致 |
| SQL allowlist | passed | 每文件只含一张表及所属索引；无 DML、ALTER、DROP、数据库/schema 创建 |
| Bundle | passed | `filename + NUL + bytes + NUL` 顺序 SHA-256 与既有记录一致 |
| 门禁输入 | passed | DESIGN-06 与 PLAN-03 的 accepted 版本及 Git 输入未变化 |

## 3. 复现方法

```bash
make test-database-design
```

本门禁仍不把 PostgreSQL 语法、迁移或 catalog 结果伪装成静态证据；这些结果由 P03-T002 在隔离数据库中关闭。

## 4. 失效条件

DESIGN-06、PLAN-03 或任一 `sql/*.sql` 改变时，本记录立即失效。数据库运行验证只允许使用经过名称核对的隔离测试数据库。
