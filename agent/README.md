# Lanverse Agent 服务

实现依据：[创作编排架构](../docs/design/0013-创作编排与多媒体画布架构调整设计.md)、[Harness 专业能力](../docs/design/3004-AgentHarness专业能力与创作流程设计.md)。

| 进程 | 入口 | 当前职责 |
| --- | --- | --- |
| 受限候选生成 | `app.candidate_runtime.api:app` | StoryGraph/SceneAnalysis Stage，以及分集→分场解析→设定→文字分镜四类有来源的 Harness 任务 |
| 可信命令接受 | `app.creation.api:create_configured_app --factory` | Go 命令鉴权、持久回执、启动 Outbox、Temporal 原身份对账 |

新服务目前完成可靠接受和启动交接。`accepted` 表示数据库已提交，`started` 表示已核验 Temporal 执行身份；两者都不表示已生成或正式采纳文本。真实文本 Workflow/Worker、Python 草案及 Go 采纳桥接仍按实施计划推进。部署匹配的 Worker 前，保持 Go 的 `CREATION_AGENT_URL` 为空。

## 文本与分镜 Harness

`POST /internal/text-storyboard/invocations` 是新增的受限内部入口，挂载在现有候选进程；不会创建额外业务服务。四类 stage 为 `map_manuscript`、`analyze_episode`、`build_world`、`direct_scene`。专业包为 `agent/skills/text-storyboard`，与旧包独立冻结；发布摘要可从 `app.modules.text_storyboard.harness.RELEASE_HASH` 读取。包文件、输入/输出 Schema 和执行上限参与摘要。

调用者使用 `TextTask` 固定 invocation_id、SourceEdition（源版本、完整原文及 UTF-8 SHA-256）、release_hash、当前 scope 和必要上游草案。`analyze_episode` 只读取选定集，`build_world` 拒绝未完成全稿解析的输入，`direct_scene` 只装载当前场与经过披露过滤的提及。原文不再规范化；含重复短语的 Evidence 必须显式定位 occurrence，代码补出 Unicode code point 偏移和片段 hash。人物/地点/道具提及含 presence，台词中仅被提及的人物不能直接入画。

请求头 `X-Lanverse-Text-Authorization` 由可信调用者通过 `sign_task(task, AGENT_EXECUTION_SECRET, expires_at)` 生成，最多有效 60 秒，绑定完整任务、release 与 invocation；它与旧接口和 Creation 命令使用不同 audience。此短期授权不替代可信应用层的持久预算、租约和项目权限。Harness 没有幂等数据库；超时或断线后不能盲目重投并假定没有消耗推理额度。

结果为 `TextResult`：候选及其 hash、来源证据、实际 ContextManifest、待审 Issue。所有成功结果都是 `needs_review`，model_calls=1，用量暂标 unknown；不会伪造正式采纳或可读取的持久资源引用。每次上下文最多 240,000 UTF-8 bytes（含规则），结果和各诊断流最多 2,000,000 bytes，deadline 不超过 900 秒。超限明确失败，不截断全稿。此受限接口尚未接通可信持久执行存储，不具备超长稿分块归并和审批恢复；跨场状态使用需要平台已审阅的披露映射。

本机完整候选链评测使用设计中的合成三集剧本，不读取业务库或真实用户原稿：

```sh
LANVERSE_TEST_REAL_CODEX=1 LANVERSE_TEXT_EVAL_OUTPUT=/tmp/lanverse-text-storyboard-eval \
  .venv/bin/python -m pytest -q -s tests/integration/test_text_storyboard_real_codex.py
```

它调用本机 Codex，依次检查全稿分集、逐集解析、WorldBook 和第一集文字分镜。目录保存已校验草案、模型原始候选及汇总；相同输入/发布摘要的已完成步骤可在评测中复用。此缓存只属于测试，不是生产恢复机制；评测串联草案不代表跳过生产审阅门，也不证明 Go 正式采纳已经接通。公开框架核验、合同与下一阶段生产接线见 [3004 第 11 节](../docs/design/3004-AgentHarness专业能力与创作流程设计.md#11-文本链实施合同2026-09-08)。

## 本机运行配置

复用已有 `agent/.venv`、PostgreSQL 和 Temporal。服务不会自动读取根目录 `.env`，也不会创建、启动或重启基础设施。

| 变量 | 含义 |
| --- | --- |
| `CREATION_DATABASE_URL` | 必填，专属 Agent 数据库的 PostgreSQL URL；不回退到平台 DATABASE_URL |
| `CREATION_AGENT_SECRET` | 必填，与 Go 相同的独立交接密钥，至少 32 字节，不能复用 AGENT_EXECUTION_SECRET |
| `CREATION_TEMPORAL_ADDRESS` | 默认 `127.0.0.1:7233` |
| `CREATION_TEMPORAL_NAMESPACE` | 默认 `default`；自动启动要求历史保留期至少 24 小时 |
| `CREATION_TEMPORAL_TLS` | 默认 `false`；非 loopback 地址必须为 `true` |
| `CREATION_TASK_QUEUE` | 默认 `lanverse-creation-text`，首次接受后固定保存 |

在 `agent/` 中安装已锁定的可信服务依赖：`uv sync --locked --extra dev --extra creation`。将上述变量注入可信服务进程，使用单独数据库及角色。迁移使用专用 schema owner；运行角色对 creation_commands 仅授予 SELECT/INSERT，对 creation_start_outbox 授予 SELECT/INSERT/UPDATE，对 creation_schema 仅授予 SELECT，并授予 schema USAGE；新增执行库表按职责授予权限：creation_executions、creation_steps 为 SELECT/INSERT/UPDATE，creation_drafts、creation_output_bindings、creation_result_outbox 为 SELECT/INSERT；不授予平台库业务写入权限。不得把可信进程环境传给候选生成进程。

```sh
.venv/bin/python -m app.creation.migrate
.venv/bin/uvicorn app.creation.api:create_configured_app --factory --host 127.0.0.1 --port 8788
```

初次迁移要求空的独立数据库，重复迁移核对 checksum。应用启动只检查迁移，不执行 DDL。`/healthz` 是进程存活检查；`/readyz` 只证明持久接受能力就绪，不证明 Worker、模型或业务主链就绪。

服务的 POST/GET 内部合同见设计第 11–12 节。POST 正文最多 16 KiB；拒绝重复 JSON 键、未知字段、非规范身份和签名不匹配。启动未知时以原身份退避；类型、队列或 memo 冲突持久阻塞。接受 12 小时后仍查不到 Temporal 历史的命令停止自动启动，需要依据原记录排障，不能通过换 ID 绕过。

## 验证与镜像

```sh
.venv/bin/ruff check app tests
.venv/bin/ruff format --check app tests
.venv/bin/pyright app tests
.venv/bin/pytest -q
```

真实服务测试使用 `LANVERSE_TEST_CREATION_DATABASE_URL` 指向现有 PostgreSQL 实例中的独立临时库，并设置 `LANVERSE_TEST_TEMPORAL_ADDRESS`。`tests/creation` 会清空该测试库的 Creation 表，不能指向业务库。缺少这些条件时集成测试明确 skip。

本机集成测试运行短生命周期 Agent HTTP 进程，使用真实 Go HTTP 客户端和现有 Temporal，退出后关闭测试进程并终止精确的合成 Workflow。Temporal 测试历史由既有保留策略清理；不改其他 Workflow 或 Namespace。没有注册占位创作 Worker，也不调用真实模型。

受限镜像继续使用 `Dockerfile` 和 `requirements.txt`；可信镜像使用 `Dockerfile.creation` 和 `requirements-creation.txt`，不含 Codex CLI、Skill 或 Harness 模块。可信依赖从唯一锁文件导出：

```sh
uv export --locked --extra creation --no-dev --no-hashes --no-emit-project --output-file requirements-creation.txt
```

## 持久执行存储（尚未接入生产 Workflow）

`app.creation.execution.ExecutionStore` 冻结运行调用额度和 Skill release，持久保存步骤输入、尝试 fence、unknown 用量、草案、OutputBinding 与 result_ready Outbox。相同输入读取已保存结果；过期尝试不会自动重新调用模型。保存结果时在可信层重新检查来源、候选摘要及覆盖，草案和输出引用在同一事务提交。`app/text_contract` 为两种镜像共享的纯合同，不包含 Skill 或推理执行能力。

`text-execution` 是追加迁移，不改写原 command-acceptance 的校验和。升级后须先显式执行迁移再启动可信服务；现有业务库未自动迁移。当前存储仅完成组件级接线与本机数据库测试，尚未暴露执行入口、投递结果事件或注册生产 Workflow，也不能替代 Go 的当前权限与四道审批门。
