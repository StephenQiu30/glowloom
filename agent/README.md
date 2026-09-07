# Lanverse Agent 服务

实现依据：[创作编排架构](../docs/design/0013-创作编排与多媒体画布架构调整设计.md)、[Harness 专业能力](../docs/design/3004-AgentHarness专业能力与创作流程设计.md)。

| 进程 | 入口 | 当前职责 |
| --- | --- | --- |
| 受限候选生成 | `app.candidate_runtime.api:app` | 已有 StoryGraph/SceneAnalysis Stage、短时执行授权和 Codex Harness |
| 可信命令接受 | `app.creation.api:create_configured_app --factory` | Go 命令鉴权、持久回执、启动 Outbox、Temporal 原身份对账 |

新服务目前完成可靠接受和启动交接。`accepted` 表示数据库已提交，`started` 表示已核验 Temporal 执行身份；两者都不表示已生成或正式采纳文本。真实文本 Workflow/Worker、Python 草案及 Go 采纳桥接仍按实施计划推进。部署匹配的 Worker 前，保持 Go 的 `CREATION_AGENT_URL` 为空。

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

在 `agent/` 中安装已锁定的可信服务依赖：`uv sync --locked --extra dev --extra creation`。将上述变量注入可信服务进程，使用单独数据库及角色。迁移使用专用 schema owner；运行角色对 creation_commands 仅授予 SELECT/INSERT，对 creation_start_outbox 授予 SELECT/INSERT/UPDATE，对 creation_schema 仅授予 SELECT，并授予 schema USAGE；不授予平台库业务写入权限。不得把可信进程环境传给候选生成进程。

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
