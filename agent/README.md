# Lanverse AI Worker（Python）

> **当前状态（2026-09-25）：** 本目录下的代码是旧实现（FastAPI 服务 + 自有执行存储 + Codex Harness），不符合新设计，处置方式见 [0401 第 5 节](../docs/design/0401-实施路线与交付计划.md#5-现有代码的处置待确认)。旧实现说明可通过 `git show c99a5528:agent/README.md` 查看。以下为新设计下本单元的目标职责。

## 职责

AI Worker 是 Temporal 的 Activity Worker，监听 `ai` 任务队列，按 Go 工作流给出的**冻结输入**执行 AI 步骤并返回结构化结果。

| 负责 | 不负责 |
| --- | --- |
| LLM 结构化输出：剧本解析、资产抽取、分镜生成，含校验与有限次修复 | 连接业务数据库、决定业务状态 |
| 生图 / 生视频 / TTS 供应商调用：提交、查询、取消、结果地址、用量 | 自动重新提交结果未知的付费请求 |
| 内容审核调用 | 暴露公共 HTTP 接口 |
| 模型网关：按能力路由到主 / 备供应商 | 保存媒体文件（由 Go 媒体 Worker 接管） |

设计依据：[0201 系统架构 §3、§5](../docs/design/0201-系统架构设计.md)、[0301 技术选型 §3、§7](../docs/design/0301-技术选型决策.md)。

## 目标目录

```text
agent/
  app/
    main.py                  # 连接 Temporal，注册 ai 队列 Activity
    config.py                # 集中配置校验
    activities/<能力>/       # script_parse、asset_extract、storyboard、image、video、tts、moderation
    gateway/providers/       # 各供应商适配
    llm/                     # 结构化输出、修复循环、原文位置校验
    prompts/<能力>/<版本>/   # 版本化提示词
  evals/                     # 评测集与离线回归
  tests/
```

## 关键约定

1. Activity 输入输出以 `contracts/activities/` 中的 JSON Schema 为唯一来源，本端对应 Pydantic 模型并用共享样例测试。
2. 付费请求使用 Go 预先持久化的 `provider_request_key`；超时或响应丢失返回 `unknown`，由 Go 工作流对账。
3. LLM 引用原文的字段必须校验原文存在；修复次数有上限，超限明确失败并保留原始输出。
4. 提示词修改须更新版本目录并通过 `evals/` 回归。
5. 供应商密钥只存在于本进程环境，不写日志、不回传。

完整工程约定见 [PROJECT.md 第 5 节](../PROJECT.md#5-python-ai-worker)。
