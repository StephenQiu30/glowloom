# Lanverse Agent 服务（Python）

> **当前状态（2026-09-25）：** 本目录下的代码是旧实现（自有执行存储 + Python 侧编排 + Codex Harness），不符合新设计，处置方式见 [0401 第 5 节](../docs/plan/0401-实施路线与交付计划.md#5-现有代码的处置已确认方案-a)。旧实现说明可通过 `git show c99a5528:agent/README.md` 查看。以下为新设计下本单元的目标职责。

## 职责

Agent 服务是受控的 AI 执行单元：**FastAPI + Temporal Activity Worker + Agent Harness**。它执行 Go 工作流下发的 AI 步骤并返回结构化结果；业务流程、状态与落库都由 Go 负责。

| 负责 | 不负责 |
| --- | --- |
| `agent` 队列 Activity：分集、逐集解析、设定集抽取、分镜、提示词编写等 LLM 任务（经 Harness） | 连接业务数据库 |
| 供应商适配器：生图、视频（图生视频、全能参考）、TTS 的提交 / 查询 / 取消，输入角色映射，用量解析 | 编排业务流程（归 Go 工作流） |
| 内容审核调用 | 持有对象存储管理凭据（只用预签名 URL） |
| `agent-api`：健康检查、Harness 调试与评测；V1 对话式 Agent（AG-UI） | 自动重提结果未知的付费请求 |

设计依据：[0301 §6–7](../docs/design/0301-系统架构设计.md#7-agent-服务与-agent-harness)、[0308 §4](../docs/design/0308-技术选型决策.md#4-agent-服务fastapi--agent-harness)。

## 技术栈

Python 3.12+、uv、FastAPI、Uvicorn、Pydantic v2、pydantic-settings、Temporal Python SDK、httpx、redis-py、OpenAI 兼容 SDK；V1 对话式 Agent 使用 ag-ui-protocol（与 LibTV 同为 AG-UI 协议）；Ruff、mypy、pytest。

## Agent Harness

| 组件 | 职责 |
| --- | --- |
| Skill Registry | 版本化专业能力包（`SKILL.md` + 参考资料 + schema） |
| Context Builder | 按冻结输入组装上下文，控制 token 预算 |
| Model Router | 按模型注册表选择模型与参数 |
| Tool Registry | 只读工具，按任务白名单授予 |
| Execution Loop | 调用 → 校验 → 修复，有上限，响应取消 |
| Validators | schema、原文位置、业务规则 |
| Budget / Trace | 预算控制与逐步追踪 |

## 关键约定

1. 任务只经 Temporal Activity 进入；Activity 输入输出以 `contracts/activities/` 的 JSON Schema 为唯一来源。
2. 付费请求使用 Go 生成的 `provider_request_key`；超时或响应丢失返回“结果未知”，不自动重提。
3. 长任务定期 heartbeat 并响应取消。
4. Skill 修改须更新版本并通过 `evals/` 回归。
5. 供应商凭据只在本服务环境中，不写日志、不回传。

目录与完整工程约定见 [PROJECT.md 第 5 节](../PROJECT.md#5-agent-服务)。
