# 剧本解析与批准物化真实闭环

- Evidence ID：`EV-A-20260822-script-analysis-gorm-e2e`
- 结论：`superseded`（Schema、身份会话和前端合同已变化，由 [`EV-A-20260823-script-analysis-canonical-browser`](./EV-A-20260823-script-analysis-canonical-browser.md) 替代）
- 执行时间与时区：2026-08-22，Asia/Shanghai
- 执行人 / 复核人：Codex / 待复核
- Git commit / 制品 digest：当前工作区，尚未提交
- 环境：本机 PostgreSQL、Kafka、Redis、临时 MinIO、Python Agent、Go API/operation-worker、Next.js dev server
- 关联：REQ-M03、DES-000/004/005/006、PRD-A、PLAN-A；`frontend/tests/unit/script-analysis-workspace.test.tsx`、`backend/src/scripts/model_test.go`

## 执行命令

```text
DATABASE_URL='postgres://postgres@127.0.0.1:5432/lanverse' LANVERSE_INTEGRATION=1 go test ./src/scripts -run TestApproveAnalysisMaterializesCanonicalWithGORM -count=1
DATABASE_URL='postgres://postgres@127.0.0.1:5432/lanverse' LANVERSE_ROLE=schema-init go run ./cmd
cd frontend && OPENAPI_SCHEMA_URL=../backend/docs/swagger.json npm run openapi2ts
cd agent && uv run pytest
agent-browser open http://127.0.0.1:8123
```

## 断言

- 浏览器完成“导入整本剧本 → Kafka operation task → Worker 解析 → 草稿 → 人工批准”。
- 页面展示 3 个剧集、3 个场景、2 个人物和 11 个生产资产；页面显示 `事实已批准`。
- PostgreSQL 核验 approved 项目存在 3 个 `prj_content_units`、3 个 `nar_scenes`、11 个 `pk_entities` 和 11 个 `pk_production_requirement_items`。
- 第二次批准调用不新增 canonical scene，GORM 集成测试通过重复批准断言。
- 错误响应包含标准 `error.code`、`next_action`、`request_id`；Malformed JSON 返回 `invalid_json`。
- `agent-browser a11y --tags wcag2a,wcag2aa`：violations=0，incomplete=0，passes=23。

## 范围与残余风险

本证据不覆盖完整 M01—M15、Elastic 投影、真实 Provider、完整 Redis 业务接入、RLS、完整导入格式、审阅交付或 ToC 搜索 Gate；这些仍保持 `not_run`/`未实现`，并由 `docs/traceability/full-scope-matrix.md` 追踪。
