# Lanverse 文档

Lanverse 当前只交付一个 AI 短剧制作 MVP：单个创作者在受控本地环境中，把获权文本转换为结构化剧本和分镜，执行图片、视频与基础 TTS 任务，选择候选并导出带对白和字幕的可播放 MP4。

## 唯一交付链

正式工作严格按 `Requirement → Design → PRD → Plan → Implementation → Acceptance` 推进：

1. [`requirement/`](requirement/README.md) 固定 MVP 能力、质量边界和技术约束。
2. [`design/`](design/README.md) 定义 FastAPI/前端架构、任务恢复、契约、20 张应用表物理数据模型和工程结构。
3. [`prd/`](prd/README.md) 固化用户价值、范围和 SMART 验收。
4. [`plans/`](plans/README.md) 给出 test-first 的实现任务、命令和证据位置。
5. [`acceptance/`](acceptance/README.md) 只在实现完成后记录实际结果。

只有适用 Requirement、Design、PRD 和 Plan 均为 `accepted`，且 `database_design_ready` 与 `implementation_start` 依次 `passed` 才授权实现。当前文档可在上游评审稳定后提前形成下游草案，但不得以草案状态运行后端或前端脚手架。

`docs/gates/` 只在首次产生有效门禁 Markdown 时创建，用于保存阶段之间的不可变放行记录；它不构成新的交付阶段，也不替代任何正式文档或 Acceptance。

## 唯一 MVP 实现清单

- 单操作者、单应用实例、一个活动单集，部署在本机且只通过 loopback 访问。
- 粘贴获权中文文本，形成可追溯的来源版本。
- 文本→剧本→资产与 6–10 镜头分镜→确认后的生产输入。
- 图片、视频和逐句 TTS 独立任务，以及任务恢复、候选预览与逐位置采用。
- 源语言字幕、服务端合成与 30–60 秒纵向 MP4/SRT/Manifest 交付。
- 项目、故事、制作室、任务与交付五个前端工作区。
- 根 `sql/` 中面向 `public` schema 的逐表标准 SQL，以及 FastAPI/asyncpg/Alembic 后端、PostgreSQL 20 张应用表（19 张业务事实表 + 1 张幂等技术表）、TaskJob 租约 Worker、MinIO 私有对象存储和 FFmpeg。
- create-next-app 前端、显式 shadcn/Radix 基座、Redux Toolkit/RTK Query 状态边界，以及 Swagger→`@umijs/openapi` 生成请求链。
- Mock Provider 自动 E2E，以及至少一部覆盖文本、图片、视频和 TTS 的真实 Provider 完整样片。

以上清单穷尽本次产品实现范围。未被活跃 Requirement、Design、PRD 和 Plan 明确定义并追踪到验收标准的能力，不是实现输入，也不得产生目录、接口、数据表、Feature Flag 或占位代码。

## 文档规则

- `docs/` 只允许 `.md` 正式文档、索引和阶段记录；JSON、YAML、Python、生成物与可执行测试放在所属实现目录，禁止混入文档目录。
- 活跃正式文档状态只使用 `draft/review/accepted`；验证结果使用 `passed/failed/insufficient/not_applicable`。
- 除目录索引 `README.md` 外，正式 Markdown 文件统一使用 `NN-语义名称.md`：目录内按阅读和交付顺序使用两位数字，名称直接说明文档职责，不在文件名中重复内部文档编号。
- `doc_no` 使用简短的层级语义编号（如 `REQ-01`、`DESIGN-01`、`PRODUCT-01`、`PLAN-01`、`ACCEPTANCE-01`）；条款级 P0、AC、Test 和 Evidence ID 保持稳定，用于机器追踪，不写入文件名。
- 每份正式文档包含 `layer/doc_no/status/version/owner/inputs/outputs/downstream`。
- 双向追踪使用 `Requirement → Design AC → PRD AC → Plan Task/Test/Evidence → Acceptance Result`。
- `operations/` 只承载实现验收后的发布与运行说明，不增加流程阶段。
- `docs/` 不保存临时 todo、会议流水或一次性调查记录。
