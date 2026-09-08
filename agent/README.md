# Lanverse Agent 服务

实现依据：[创作编排架构](../docs/design/0013-创作编排与多媒体画布架构调整设计.md)、[Harness 专业能力](../docs/design/3004-AgentHarness专业能力与创作流程设计.md)。

## 模块边界

| 目录 | 职责与依赖 |
| --- | --- |
| `app/api/` | FastAPI 应用工厂、生命周期和内部能力路由组合；不拥有业务事实 |
| `app/skills/` | 当前镜像内置 Skill 的显式注册和发布摘要校验；不下载或修改 Skill |
| `app/protocol/` | 跨进程 canonical JSON/摘要，纯编码合同，不依赖运行层 |
| `app/text_contract/` | 文本任务/结果、来源与专业检查、短期调用签名；可信编排与受限执行共享 |
| `app/reasoning/` | Codex 进程适配器：隔离、Schema、时间/字节预算、取消及退出等待；不依赖业务模块 |
| `app/modules/` | StoryGraph、文本分镜等专业上下文与领域规则；文本模块不依赖旧 StoryGraph 执行器 |
| `app/candidate_runtime/` | 受限 HTTP 边界与能力就绪检查，调用专业 Harness |
| `app/creation/` | 可信命令、执行存储、Temporal 编排、失败恢复和 Agent 服务组装 |

这些模块部署在一个 Agent 镜像和一个容器中；模块边界仍由导入、凭据白名单和 HTTP 合同维护，不把模块误拆成多个产品服务。跨语言编码和专业发布摘要保持原合同。实施与检查见 [Agent 单服务设计](../docs/design/0021-Agent单服务架构调整设计.md)。

受限服务的 `GET /readyz`：在统一 `app/skills/catalog.py` 注册表上逐项校验 StoryGraph、SceneAnalysis、文本分镜的安装发布摘要，并检查本地 Codex 可执行文件和内部签名配置。Agent 正式入口会先检查 Creation 存储，再检查 Skill 注册表；任一项缺失/漂移返回 503 与明确错误码，不返回路径或密钥；检查不会运行模型。该接口只证明本地候选执行前置条件，不证明模型认证、真实推理、Temporal 或正式采纳可用。

文本调用签名的唯一实现位于 `app.text_contract.authorization`，受限 HTTP 使用它，可信调用方从此处导入。Codex 同时发送提示词并读取 stdout/stderr，输出超限不会被堵塞的 stdin 拖到总超时；异常会终止进程组并等待退出。结构化结果按字节预算读取普通文件，拒绝符号链接、重复 JSON 键和非有限数字，不将不明确的输出作为有效候选。

| 模块 | 入口 | 当前职责 |
| --- | --- | --- |
| Agent 服务 | `app.main:create_agent_app --factory` | 统一 HTTP、命令鉴权、持久回执、Temporal 编排、运行与草案读取、Harness 调用 |
| Candidate Runtime | `app.candidate_runtime` | 同一 Agent 内部的 StoryGraph/SceneAnalysis/Text Harness 路由和 Codex 执行 |
| Creation Workflow | `app.creation.worker` | 同一 Agent 进程内的 Temporal Worker、冻结源读取、草案保存、人工门等待与恢复 |

`accepted` 表示命令数据库已提交，`started` 表示已核验 Temporal 执行身份；两者都不表示已生成或正式采纳文本。Agent 实际注册四阶段文本 Workflow，正式采纳由 Go 人工门与业务 Owner 决定。`CREATION_AGENT_URL` 和 `AGENT_URL` 在 Docker 中都指向同一个 `agent:8787` origin。

## 文本与分镜 Harness

`POST /internal/text-storyboard/invocations` 是统一 Agent 内部的受限入口，不创建额外业务服务。四类 stage 为 `map_manuscript`、`analyze_episode`、`build_world`、`direct_scene`。专业包为 `agent/skills/text-storyboard`，与旧包独立冻结；发布摘要可从 `app.modules.text_storyboard.harness.RELEASE_HASH` 读取。包文件、输入/输出 Schema 和执行上限参与摘要。

调用者使用 `TextTask` 固定 invocation_id、SourceEdition（源版本、完整原文及 UTF-8 SHA-256）、release_hash、当前 scope 和必要上游草案。`analyze_episode` 只读取选定集，`build_world` 拒绝未完成全稿解析的输入，`direct_scene` 只装载当前场与经过披露过滤的提及。原文不再规范化；含重复短语的 Evidence 必须显式定位 occurrence，代码补出 Unicode code point 偏移和片段 hash。人物/地点/道具提及含 presence，台词中仅被提及的人物不能直接入画。

请求头 `X-Lanverse-Text-Authorization` 由可信调用者通过 `sign_task(task, AGENT_EXECUTION_SECRET, expires_at)` 生成，最多有效 60 秒，绑定完整任务、release 与 invocation；它与旧接口和 Creation 命令使用不同 audience。此短期授权不替代可信应用层的持久预算、租约和项目权限。Harness 没有幂等数据库；超时或断线后不能盲目重投并假定没有消耗推理额度。

结果为 `TextResult`：候选及其 hash、来源证据、实际 ContextManifest、待审 Issue。所有成功结果都是 `needs_review`，model_calls=1，用量暂标 unknown；持久引用和正式采纳分别由可信 Worker、Go 保存。每次上下文最多 240,000 UTF-8 bytes（含规则），结果和各诊断流最多 2,000,000 bytes，deadline 不超过 900 秒。超限明确失败，不截断全稿。受限接口不拥有持久状态；其调用预算、草案保存与审批恢复由可信执行层控制。当前不具备超长稿分块归并；跨场状态使用仍需要平台已审阅的披露映射。

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
| `CREATION_PLATFORM_URL` | Worker 必填，Go 平台 origin；HTTPS，或 loopback HTTP；不接受任意路径、userinfo、query |
| `CREATION_HARNESS_URL` | Worker 必填，统一 Agent 自身的受限 Harness origin；Docker 中为 `http://agent:8787` |
| `CREATION_HARNESS_SECRET` | Worker 必填，与 Harness 的 `AGENT_EXECUTION_SECRET` 对应，必须独立于平台交接密钥 |
| `CREATION_TEXT_RELEASE_HASH` | Worker 必填，显式固定已发布文本 Skill 摘要；不能从可信镜像导入专业 Harness |
| `CREATION_CALL_LIMIT` | Worker 调用总上限，默认 1000，范围 1–1000；运行首次使用时冻结，恢复不能重置 |
| `CREATION_INVOCATION_TIMEOUT_SECONDS` | Worker 单次推理时限，默认 600 秒，范围 1–900；写入固定 TextTask，恢复时修改将发生输入冲突 |

在 `agent/` 中安装已锁定的可信服务依赖：`uv sync --locked --extra dev --extra creation`。将上述变量注入可信服务进程，使用单独数据库及角色。迁移使用专用 schema owner；运行角色对 creation_commands 仅授予 SELECT/INSERT，对 creation_start_outbox 授予 SELECT/INSERT/UPDATE，对 creation_schema 仅授予 SELECT，并授予 schema USAGE；新增执行库表按职责授予权限：creation_executions、creation_steps 为 SELECT/INSERT/UPDATE，creation_drafts、creation_output_bindings、creation_result_outbox 为 SELECT/INSERT；不授予平台库业务写入权限。不得把可信进程环境传给候选生成进程。

```sh
.venv/bin/python -m app.creation.migrate
.venv/bin/uvicorn app.main:create_agent_app --factory --host 127.0.0.1 --port 8787
```

初次迁移要求空的独立数据库，重复迁移核对 checksum。应用启动只检查迁移，不执行 DDL。`/healthz` 是进程存活检查；`/readyz` 证明 Creation 存储和候选执行前置条件就绪，但不证明 Worker 已完成业务任务、模型推理成功或正式业务主链就绪。

服务的 POST/GET 内部合同见设计第 11–12 节。POST 正文最多 16 KiB；拒绝重复 JSON 键、未知字段、非规范身份和签名不匹配。启动未知时以原身份退避；类型、队列或 memo 冲突持久阻塞。接受 12 小时后仍查不到 Temporal 历史的命令停止自动启动，需要依据原记录排障，不能通过换 ID 绕过。

## 验证与镜像

```sh
.venv/bin/ruff check app tests
.venv/bin/ruff format --check app tests
.venv/bin/pyright app tests
.venv/bin/pytest -q
```

真实服务测试使用 `LANVERSE_TEST_CREATION_DATABASE_URL` 指向现有 PostgreSQL 实例中的独立临时库，并设置 `LANVERSE_TEST_TEMPORAL_ADDRESS`。`tests/creation` 会清空该测试库的 Creation 表，不能指向业务库。缺少这些条件时集成测试明确 skip。

本机集成测试运行短生命周期 Agent HTTP 进程，使用真实 Go HTTP 客户端和现有 Temporal，退出后关闭测试进程并终止精确的合成 Workflow。`tests/creation/test_workflow.py` 注册真实生产 Workflow 与 Activity，使用合成专业结果验证四道人工门、Worker 重启和草案复用；这些测试不调用真实模型。Temporal 测试历史由既有保留策略清理，不改其他 Workflow 或 Namespace。

Agent 服务使用一个 `Dockerfile` 和一个 `requirements-creation.txt`，同一镜像包含可信编排、Temporal Worker、受限 Harness 和 Codex CLI。可信依赖从唯一锁文件导出：

```sh
uv export --locked --extra creation --no-dev --no-hashes --no-emit-project --output-file requirements-creation.txt
```

## 文本生产 Workflow 与持久恢复

`app.creation.execution.ExecutionStore` 冻结运行调用额度和 Skill release，持久保存步骤输入、尝试 fence、unknown 用量、草案、OutputBinding 与 result_ready Outbox。相同输入读取已保存结果；过期尝试不会自动重新调用模型。保存结果时在可信层重新检查来源、候选摘要及覆盖，草案和输出引用在同一事务提交。`app/text_contract` 是同一 Agent 镜像内共享的纯合同，不包含 Skill 或推理执行能力。

`text-execution`、`text-workflow` 与 `text-resume` 是追加迁移，不改写先前校验和。升级后须先显式迁移再启动可信服务；现有业务库不自动迁移。统一 Agent 镜像已经包含 Worker，应用启动时在同一进程内运行 API、Dispatcher、Temporal Worker 和 Harness 路由；不添加第二个 Worker 容器。模型子进程仍只继承显式白名单。

生产流程为冻结源 → 分集 → 平台人工门 → 全部分集解析 → 结构人工门 → 全稿设定 → 设定人工门 → 已选场文本分镜 → 分镜人工门。每次模型调用先从 Go 固定源桥重查权限、源版本与摘要，并检查前置正式采纳回执。结构门确定场范围，Worker 验证范围属于已采纳结构且剩余额度足够。Workflow History 仅携带范围标识与草案引用，完整源、任务和结果保存在 ExecutionStore。

等待人工门时不占用模型进程。`review_changed` 信号只唤醒查询，只有 Go 中匹配运行、候选摘要、审阅决定和 Owner Effect 的回执才能通过。长时间等待会通过 Continue-as-New 收敛 History，仍复用原运行、草案和额度。重启 Worker 不改变步骤输入；已保存结果直接读回，未完成的未知调用不会自动重投。

平台使用下列受签名保护的 GET 路由拉取执行状态与候选，签名继续绑定空正文、精确路径、method 和 `lanverse.creation.command` audience：

- `/internal/creation/commands/{command_id}/execution`：`creation-execution-production`，含运行身份、冻结源、额度、步骤、输出引用、当前阶段、`can_resume` 与 `running/waiting_review/blocked/rejected/completed` 状态。
- `/internal/creation/commands/{command_id}/drafts/{draft_id}`：`creation-draft-production`，固定 revision=1，含完整 TextTask/TextResult、步骤身份与结果/候选摘要；读取时重新验证存储摘要和专业合同。

Go 的门查询可同步拉取候选并建立审阅任务；这不是 HTTPS 业务事件广播。草案事务仍写入 `creation_result_outbox`，Kafka 发布与消费尚未接通，不能作为已完成事件投递验收。`completed` 只表示四道文本正式采纳门完成，不代表媒体生产、成片或外部最终验收。

源桥/平台的网络故障与临时 5xx、429、权限暂时失效会保存 `blocked`、具体安全 `last_error` 和 `can_resume=true`，原 Workflow 等待显式恢复。Go 重新核验原运行权限后，可签名 POST `/internal/creation/commands/{command_id}/resume`，正文严格为 `{ "payload_hash": "原命令摘要" }`。202 `creation-resume-production` 回执仅表示 `resume_requested`；Workflow 通过同业务身份 Continue-as-New 重新验证源、复用原草案与额度，不承诺立即成功。同正文已恢复至 running/waiting_review/completed 时是 202 幂等空操作，不再次发信号；rejected 或不可恢复 blocked 返回 409。

`harness_response_unknown`、`harness_result_invalid` 或租约过期导致 `blocked`，调用额度保持已占用。排障先读快照、原步骤 task/input_hash、已存 draft 和 Temporal history；已提交草案可据原身份恢复执行，尚无可确认结果时保持 unknown。本轮没有把未知消费自动判为失败的按钮，也不通过新建运行重置预算绕过不确定结果；后续人工恢复必须先有明确的调用对账证据。平台撤权或回执不一致同样阻止继续推理。

真实合成评测将每次调用前的占用写入输出目录 `evaluation-budget.json`，总上限 8。复用草案时重新执行完整合同校验；中断后的已知或未知调用均继续占用原额度，显式恢复时可用 `LANVERSE_TEXT_EVAL_PRIOR_CALLS` 补记中断前记录。该评测文件不替代生产 ExecutionStore 或未知调用对账。

## 执行尝试记录

可信执行层新增 `text-attempts` 追加迁移，发布前需显式执行 `python -m app.creation.migrate`，启动只检查迁移而不执行 DDL。运行角色对 `creation_attempts` 需要 SELECT/INSERT/UPDATE，不授予 DELETE；迁移由专用 schema owner 执行。已有迁移文本与 checksum 不变。

每次新领取都会在同一事务保存尝试身份、原始 fence、输入摘要、开始/执行截止/租约截止时间、步骤指针和调用额度。成功时，尝试终态、草案、输出绑定和结果 Outbox 一并提交；数据库约束保证事件引用属于同一步骤和尝试的草案。失联/取消/过期记录为 unknown，保留已占用额度，不自动再次调用模型。终态和尝试身份不可重写。

签名 GET `/internal/creation/commands/{command_id}/steps/{step_id}/attempts` 只接受空正文和无查询串的精确路径请求，返回 `creation-attempt-history-production`。响应含 `history_origin`、`current_attempt_id` 和按 attempt_no 排序的 `attempts`：每项包括 attempt_id、fence、input_hash、状态、时间、结果摘要、错误码及 unknown 用量。运行与步骤必须匹配；不会返回原稿、提示词或候选正文。它尚未接入 Go 公共 API 或画布。

旧步骤返回 `history_origin=unavailable` 和空尝试列表；不会伪造曾经的执行或消费记录。新步骤为 recorded。`running` 且 `lease_expired=true` 表示已过租约但仍待对账，不能按可重试理解；下一次原身份恢复会将过期步骤和原尝试原子转为 unknown。本轮未开放人工重做或自动新增 Attempt 的接口，也未改变已发布的 execution/draft 响应字段。

## 初始执行清单

`ExecutionStore.freeze` 在首次模型调用前，将执行策略与初始 Manifest 同事务保存。清单包含四个文本阶段、必要审阅门、尚未展开的逐集/逐场集合，以及固定输入/配置/模板摘要。重复启动读取原清单，不用部署后的模板覆盖历史。

签名 GET `/internal/creation/commands/{command_id}/manifest` 返回 `recorded`、`not_frozen` 或 `unavailable`；后两者的清单为空。接口不执行推理，不返回原稿或候选正文。新增独立 `text-manifest` 迁移，须通过既有显式迁移入口应用；旧执行不回填虚构清单。动态实例、多产物和 Go/画布消费不在此切片内。规范及验收见 [Spec](../docs/requirement/0015-Agent执行清单与尝试追踪需求规格.md) 与 [验收记录](../docs/acceptance/0015-Agent执行可追踪性验收记录.md)。
