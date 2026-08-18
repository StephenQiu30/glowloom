# Agent Harness MVP 实施计划

- 状态：in_progress
- 目标：跑通“剧本导入 → 预览 → 剧集创建 → 深度 Skill 解析 → 候选审核 → 场景/资产/任务生产”的最小业务闭环
- 当前执行容量：1 名开发代理，串行修改和验证
- 外部依赖：PostgreSQL、RabbitMQ、MinIO；本地结构解析默认依赖本机 Codex 登录态，DeepSeek 仅用于显式备用 provider，不影响单元/集成测试

## 1. 工作包

| 工作包 | 内容 | 三点估算 | 关键路径 | 当前状态 |
| --- | --- | --- | --- | --- |
| P0 | LangGraph StateGraph Harness、输入边界、结构化输出和错误映射 | 0.5/1/2 天 | 是 | completed |
| P1 | 将剧本结构解析接入 Harness，保持现有候选审核契约 | 0.5/1/1.5 天 | 是 | completed |
| P2 | DOCX/MD/文本统一导入与预览校验 | 0.5/1/2 天 | 是 | completed |
| P3 | 重复消息、非法输出、超时和未知状态测试 | 0.5/1/2 天 | 是 | completed |
| P4 | 长剧本解析 Skill：分集/场景切块、LangGraph fan-out/reduce、全局范围映射、候选引用和资产去重 | 1/2/4 天 | 是 | completed |
| P5 | 深度结构契约：剧集摘要、人物档案、世界观规则、资产档案和场景语义落库 | 1/2/4 天 | 是 | in_progress |
| P6 | 用真实 60 集 DOCX 完成输入边界、场景边界、Chunk 重组和深度候选契约验收 | 0.5/1/2 天 | 是 | pending |
| P7 | 其余 Agent 能力迁移到 Harness 的设计，不进入本次代码范围 | 1/2/4 天 | 否 | deferred |

三点估算依次为乐观/最可能/悲观，不构成发布承诺。P0～P3 的关键路径依赖现有任务和消息测试基础；若真实 DeepSeek 服务不可用，只记录外部等待，不把本地测试标记为外部通过。

## 2.1 当前执行结果

- P0～P4 已完成本地实现和相关验收：Harness、剧本解析适配、DOCX 读取、重复消息、非法输出、超时、未知状态、`trace_id` 传递，以及长剧本的分块解析边界均有覆盖。
- 剧本结构 Skill 已从单次整稿调用升级为 LangGraph map-reduce：先按分集和场景边界切块，再并行抽取场景、对白、资产、镜头、连续性和场景级生产任务建议，最后映射回全文字符范围并合并重复资产。
- 深度结构契约正在收敛：`continuity(scope=episode)` 表达剧集摘要，`continuity(scope=world)` 表达世界观事实/规则，角色资产表达目标、关系、外观、声音和弧光；场景确认时写入 `semantic_context`。
- 真实验收样本 `He Left Our Kids to Drown—He Didn’t Know I Was the Empress.docx` 提取为 139,723 字符、60 集、131 个场景头、3,981 个确定性块，生成 121 个 Chunk，最大 Chunk 3,837 字符，Chunk 重组与原文一致。
- LangGraph Checkpointer 当前只作为 Harness 的可注入能力；MVP 的任务事实仍由既有 `Task + Outbox/Inbox + RabbitMQ + PostgreSQL` 持有，避免引入第二套任务持久化事实。
- 本地 Codex 适配器已接入 `codex app-server` stdio：只读沙箱、拒绝工具审批、临时线程、严格 JSON Schema 和并发保护均在 provider 边界完成；整稿不再使用固定字符数或 Chunk 数上限，只有单 Chunk 上限和基础设施安全保护。
- 本机 Codex 小样本真实调用已通过：返回场景、对白、资产和镜头候选，且 source range 合法；完整 DOCX 真实调用曾启动但因 121 个 Chunk 的长时间外部等待被停止，尚未作为通过证据，需在深度结构契约完成后重新验收。
- 已清理历史文档追踪残留：删除引用已删除 PRD、旧需求、旧设计和旧验收文件的架构测试及空 PRD 索引，改由当前文档树与 Markdown 本地链接检查保护现行文档边界。

## 3. 实施顺序

1. 先写 LangGraph Harness 单元测试，再实现最小状态图运行时；
2. 将现有剧本解析 Provider 接入，确保旧的 `ScriptStructureExtractor` 测试契约保持；
3. 增加 DOCX 安全文本提取，并让媒体探测和文档导入使用同一规则；
4. 运行模块单元、剧本文档集成、脚本提取集成和架构测试；
5. 查看 diff、排除缓存/本地数据，确认当前没有画布代码或数据库迁移产物被引入。

## 4. 停止条件

- LangGraph Harness 无法在现有 Task/Inbox/Outbox 边界内保持幂等时，停止扩展 Skill，先修复平台任务契约；
- DOCX 无法在字节上限、压缩包条目和 XML 解析约束下安全读取时，停止文档扩展，保留 MD/文本路径；
- 真实 Provider 结果未知时不能区分失败和未知，停止自动重试设计，不引入隐式重试；
- 任何测试需要读取 `.env`、本地媒体或真实用户数据才能通过时，停止并改为隔离夹具。

## 5. 交付验收

```bash
cd backend
pytest -q tests/unit/test_agent_harness.py tests/unit/test_deepseek_extractor.py
pytest -q tests/unit/test_script_structure_workflow.py tests/unit/scripts/documents/test_document_analysis.py
pytest -q tests/integration/scripts/documents tests/integration/test_script_extractions_api.py
pytest -q tests/architecture
ruff check app tests
```

若基础设施未启动，集成测试必须报告为未运行/阻塞，不得报告为通过。
