# Agent Harness MVP 实施计划

- 状态：in_progress
- 目标：在一个已选定且已授权的 Workspace 内，跑通“Project → 剧本导入/预览 → 分集与深度理解 → 候选审核/正式资产 → 分镜 → 逐镜视觉与视频候选 → 主选 → 固定素材包”的最小业务闭环
- 顶层作用域：`Workspace → Project → ProductionRun → StageRun(scope=project/episode/shot/package) → Skill/WorkTask`；Episode/Shot/Package 是 StageRun 目标，不是 ProductionRun 父链。所有命令、异步任务、候选、媒体和导出都必须校验 Workspace/Project/scope 归属，禁止跨 Workspace 复用或查询制作事实
- 当前执行容量：1 名开发代理，串行修改和验证
- 外部依赖：PostgreSQL、RabbitMQ、MinIO；本地结构解析默认依赖本机 Codex 登录态，DeepSeek 仅用于显式备用 provider，不影响单元/集成测试

## 1. 工作包

| 工作包 | 内容 | 三点估算 | 关键路径 | 当前状态 |
| --- | --- | --- | --- | --- |
| P0 | Skill Harness：LangGraph StateGraph、输入边界、结构化输出和错误映射 | 0.5/1/2 天 | 是 | completed |
| P1 | 将剧本结构解析接入 Harness，保持现有候选审核契约 | 0.5/1/1.5 天 | 是 | completed |
| P2 | DOCX/MD 统一导入、预览确认与格式校验 | 0.5/1/2 天 | 是 | completed |
| P3 | 重复消息、非法输出、超时和未知状态测试 | 0.5/1/2 天 | 是 | completed |
| P4 | 长剧本解析 Skill：分集/场景切块、LangGraph fan-out/reduce、全局范围映射、候选引用和资产去重 | 1/2/4 天 | 是 | completed |
| P5 | 深度结构契约：剧集摘要、人物档案、世界观规则、资产档案和场景语义落库 | 1/2/4 天 | 是 | in_progress |
| P6 | 用真实 60 集 DOCX 完成输入边界、场景边界、Chunk 重组和深度候选契约验收 | 0.5/1/2 天 | 是 | pending |
| P7 | 其余 Agent 能力迁移到 Harness 的设计，不进入本次代码范围 | 1/2/4 天 | 否 | deferred |
| P8 | Production Control：Project 级 ProductionRun、WorkflowDefinition、分作用域 StageRun、Gate、暂停/恢复/重入 | 2/4/7 天 | 是 | pending |
| P9 | Workbench Projection：Project/Episode 阶段聚合、blocker、next action、partial/unavailable 和局部 stale | 1/2/4 天 | 是 | pending |
| P10 | 真实端到端闭环：Workspace → Project → DOCX/MD → 深度理解 → 候选审核 → 资产/分镜 → 逐镜候选 → 素材包 | 2/4/8 天 | 是 | pending |

三点估算依次为乐观/最可能/悲观，不构成发布承诺。P0～P3 的关键路径依赖现有任务和消息测试基础；若真实 DeepSeek 服务不可用，只记录外部等待，不把本地测试标记为外部通过。

## 2. 当前执行结果

- P0～P4 已完成本地实现和相关自动化验证：Harness、剧本解析适配、DOCX 读取、重复消息、非法输出、超时、未知状态、`trace_id` 传递，以及长剧本的分块解析边界均有覆盖；这不是端到端 Acceptance Evidence。
- 剧本结构 Skill 已从单次整稿调用升级为 LangGraph map-reduce：先按分集和场景边界切块，再并行抽取场景、对白、资产、镜头、连续性和场景级生产任务建议，最后映射回全文字符范围并合并重复资产。
- 深度结构契约正在收敛：使用 `EpisodeUnderstandingCandidate`、`SceneCandidate`、`DialogueCandidate`、`CharacterCandidate`、`WorldFactCandidate`、`AssetCandidate`、`ShotCueCandidate`、`ContinuityIssueCandidate` 和 `ProductionTaskSuggestion` 等显式类型；不再借用 continuity/asset 通用枚举混装语义。场景确认时将受审语义写入 `semantic_context`。
- 真实验收样本 `He Left Our Kids to Drown—He Didn’t Know I Was the Empress.docx` 提取为 139,723 字符、60 集、131 个场景头、3,981 个确定性块，生成 121 个 Chunk，最大 Chunk 3,837 字符，Chunk 重组与原文一致。
- LangGraph Checkpointer 当前只作为 Harness 的可注入能力；MVP 的任务事实仍由既有 `Task + Outbox/Inbox + RabbitMQ + PostgreSQL` 持有，避免引入第二套任务持久化事实。
- 本地 Codex 适配器已接入 `codex app-server` stdio：只读沙箱、拒绝工具审批、临时线程、严格 JSON Schema 和并发保护均在 provider 边界完成；整稿不再使用固定字符数或 Chunk 数上限，只有单 Chunk 上限和基础设施安全保护。
- 本机 Codex 小样本真实调用已通过：返回场景、对白、资产和镜头候选，且 source range 合法；完整 DOCX 真实调用曾启动但因 121 个 Chunk 的长时间外部等待被停止，尚未作为通过证据，需在深度结构契约完成后重新验收。
- 已清理历史文档追踪残留和纯静态文件门禁：文档目录、链接、文件名、目录是否存在和源码文本片段不再作为 pytest 业务通过条件；测试资源集中验证可观察功能、领域不变式、跨 Workspace 隔离、幂等和故障恢复。
- 当前实现尚无 CUR-PROD 定义的 `ProductionRun`、带 `scope_kind/scope_id` 的 `StageRun` 和 `ProductionGate` 事实；现有项目快照仍从局部模块摘要推断阶段。因此 P0～P4 只证明 Skill/文档解析能力，不代表端到端 Production Harness 或工作台闭环已完成，P8～P10 是新的关键路径。

## 3. 实施顺序

1. 先固定当前 Workspace、Project、Project 级 ProductionRun、StageRun scope 归属和 ActorContext，再写 LangGraph Harness 单元测试并实现最小状态图运行时；
2. 将现有剧本解析 Provider 接入，确保 `ScriptStructureExtractor` 的结果始终挂在同一 Project ProductionRun 及正确 project/episode StageRun 下；
3. 增加 DOCX 安全文本提取，并让媒体探测和文档导入使用同一规则；
4. 实现 CUR-PROD Project 级 ProductionRun、分作用域 StageRun/Gate 与恢复契约，迁移项目快照停止硬编码推断阶段；
5. 将深度理解显式类型候选接入人工审核和领域物化，按 Episode/Shot fan-out 并验证局部失败隔离；
6. 运行模块单元、剧本文档集成、脚本提取集成、跨 Workspace 隔离、架构测试和端到端真实素材包验收；
7. 查看 diff、排除缓存/本地数据，确认当前没有画布代码或数据库迁移产物被引入。

## 4. 停止条件

- LangGraph Harness 无法在现有 Task/Inbox/Outbox 边界内保持幂等时，停止扩展 Skill，先修复平台任务契约；
- DOCX 无法在字节上限、压缩包条目和 XML 解析约束下安全读取时，停止文档扩展，保留 Markdown 路径；
- 真实 Provider 结果未知时不能区分失败和未知，停止自动重试设计，不引入隐式重试；
- 任何测试需要读取 `.env`、本地媒体或真实用户数据才能通过时，停止并改为隔离夹具。

## 5. 交付验收

```bash
cd backend
pytest -q tests/unit/test_agent_harness.py tests/unit/test_deepseek_extractor.py
pytest -q tests/unit/test_script_structure_workflow.py tests/unit/scripts/documents/test_document_analysis.py
pytest -q tests/integration/scripts/documents tests/integration/test_script_extractions_api.py
pytest -q tests/integration/production tests/integration/projects
pytest -q tests/e2e/test_workspace_project_production_flow.py
pytest -q tests/architecture
ruff check app tests
```

上述 production/project/E2E 测试文件是 P8～P10 的目标验收入口；实现前不存在时必须报告 `not_started`，不得把命令缺失或局部 Skill 测试通过描述为端到端通过。若基础设施未启动，集成测试必须报告为未运行/阻塞，不得报告为通过。
