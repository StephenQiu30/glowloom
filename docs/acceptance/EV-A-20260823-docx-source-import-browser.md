# DOCX 原件导入、解析报告与批准物化闭环

- Evidence ID：`EV-A-20260823-docx-source-import-browser`
- 结论：`passed`（只覆盖本文列出的来源导入目标场景，不代表切片 A 或 PRD-A-AC-001—010 全部通过）
- 执行时间与时区：2026-08-23，Asia/Shanghai
- 执行人 / 复核人：Codex / 待复核
- Git commit：测试 `2f3c2b8`，实现 `f6aee1f`，production 启动链 `4daa2ac`
- 环境：macOS 本机 PostgreSQL 18.4、Redis、Kafka、临时 MinIO、Go 1.26.5 API/operation-worker、Node.js 24.19.0 Next.js standalone、agent-browser 0.33.0；未使用 Docker
- 数据集：测试专用三集中文 DOCX，原件 SHA-256 `13f2eec16824dee6c359f94dc7ccddb6b55812c5cd964eb49db8497405ebd7ad`；由本次验收生成，不含第三方生产内容
- 当前 Swagger SHA-256：`0bd361e6060aa346e3e82cd09b4815cb65318736ab765f43366145f608135a98`
- 关联：`NAR-FR-001/002`、`NAR-NFR-001`、`PRD-A-FR-002`、`PLAN-A A3`、`EPK-A-SCRIPT` 的 DOCX 正常/损坏来源子场景
- 自动化测试：`backend/tests/scripts/source_document_test.go`、`backend/tests/architecture/formal_document_contract_test.go`、`frontend/tests/unit/script-analysis-workspace.test.tsx`

## 前置条件

- 复用本机 PostgreSQL、Redis 和 Kafka，不重建或修改用户已有服务。
- 创建隔离数据库 `lanverse_codex_import_20260823` 并加载 `backend/schema/current.sql`；使用本机 MinIO 二进制在独立临时目录和 `29000/29001` 端口运行验收实例。
- API、Worker、前端与浏览器只使用测试专用身份和配置；没有把 Cookie、JWT、MinIO 身份或 `.env` 内容写入日志、证据或 Git。
- 验收结束后已关闭浏览器与全部测试进程，删除隔离数据库和两个精确临时目录；没有删除共享 Kafka 主题或已有消息。

## 执行命令与步骤

```text
cd backend
go test ./... -count=1
go vet ./...
make swagger
DATABASE_URL=<隔离数据库> LANVERSE_INTEGRATION=1 \
  go test ./src/scripts -run TestApproveAnalysisMaterializesCanonicalWithGORM -count=1 -v

cd frontend
OPENAPI_SCHEMA_URL=../backend/docs/swagger.json npm run openapi2ts
npm run lint
npm run typecheck
npm test -- --run
npm run build
npm run start

agent-browser --session lanverse open http://127.0.0.1:8123
agent-browser --session lanverse snapshot -i
agent-browser --session lanverse upload <file-ref> <测试 DOCX>
agent-browser --session lanverse click <submit-ref>
agent-browser --session lanverse wait --text '待人工批准'
agent-browser --session lanverse click <approve-ref>
agent-browser --session lanverse wait --text '事实已批准'
agent-browser --session lanverse snapshot
agent-browser --session lanverse a11y
agent-browser --session lanverse errors
agent-browser --session lanverse console
agent-browser --session lanverse close
```

浏览器随后以同一入口上传一个扩展名为 `.docx`、内容不是 ZIP 包的损坏文件，等待稳定恢复文本并重新 snapshot。

## 实际结果与逐项断言

1. 当前接口只接受 multipart 文件；旧 JSON `name/content` 正文入口已删除。Swagger 生成的前端 API 使用 FormData，并只经过 `frontend/src/lib/request.ts` 的 Axios 边界。
2. DOCX 原始 1129 字节按原字节保存在 MinIO；数据库 SourceRevision 为 `source_type=docx`、`status=approved`、64 位 content hash。Worker 回读原件并通过 hash 校验后才解析。
3. ParseReport 为 `status=complete`、`format=docx`、`paragraph_count=19`、`character_count=147`、`episode_count=3`、`failed_scopes=0`；页面明确显示同一结果。
4. Operation `9660fe57-a597-430c-8db0-0242b81637e9` 经真实 PostgreSQL Outbox、Kafka 和 operation-worker 到达 `succeeded/100%`，不是页面本地状态。
5. 人工批准后，Project `4195ef3b-a507-4e66-8e90-facb1b2cee44` 中有 3 个 ContentUnit、1 个 approved Narrative、3 个 Scene、11 个 Entity 和 11 个 ProductionRequirement；页面显示 3 集、3 场、2 人物与 11 项生产资产。
6. 损坏 DOCX 在上传边界返回 `script_invalid`，页面显示“剧本文件格式无效或内容损坏”以及“确认文件是未加密、可正常打开的 DOCX、Markdown 或 UTF-8 TXT 后重试”，没有创建解析 Operation。
7. API/前端重启后，HttpOnly refresh 会话恢复到工作台；production `console` 与 `errors` 均为空。批准结果页 axe-core 4.12.1 为 `violations=0`、`incomplete=0`、`passes=35`；standalone 重启后的入口页为 `0/0/32`。
8. `npm run build` 自动装配 `.next/static` 和 `public`，`npm run start` 直接启动 standalone server；不再依赖人工复制产物或与 `output: standalone` 冲突的 `next start`。

## Red → Green 与缺陷修复

- Go 首个测试因 `ParseSourceDocument` 不存在失败；实现后 DOCX/Markdown/TXT、无效 UTF-8、伪 DOCX、空源和未支持 PDF 参数化测试通过。
- 前端首个测试因缺少“剧本文件”入口失败；实现后真实 File/FormData、ParseReport 和恢复动作测试通过。
- production 首次 snapshot 只有“正在恢复登录会话”，定位到 standalone 静态资源未装配；已固化 build/start 脚本并重启验证。
- 损坏 DOCX 首次显示底层 ZIP 英文错误且丢弃 `next_action`；已改为稳定中文业务原因，并在唯一请求适配层保留恢复动作。

## 偏差、残余风险和后续动作

- Markdown/TXT 已由独立自动化测试覆盖，但本证据的 production 浏览器成功样本只使用 DOCX。
- 本数据集不含 DS-A-SCRIPT 要求的缺失/冲突标题、跨集别名、同名反证和单集部分失败，未验证边界 split/merge/move/ignore、局部恢复或来源版本差异。
- 未验证跨 Workspace 已知 ID、Worker/媒体对象授权、Elastic approved 检索、10 个以上 Shot、Fixture Selection 和可播放 Animatic；PRD-A 因此继续保持 `not_run`。
- 当前 `.env` 仍含旧 Python 数据库 URL 形态且本机既有 MinIO 身份与代码默认值不一致；本次没有读取、修改或兼容该文件。正式本机启动前需要由环境责任人按 `.env.example` 修正当前 Go/MinIO 配置。
