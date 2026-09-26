# Lanverse BACKLOG

项目进度与待执行任务的唯一清单。需求与设计以 `docs/` 为准，本文件只记录**要做什么、怎么做（要点与设计链接）、改哪些文件、做到哪一步**。

| 项 | 内容 |
| --- | --- |
| 范围 | 首个版本（MVP）：P0 技术验证 + M1～M5（[PLN-01](docs/plan/01-实施路线与交付计划.md)）；V2 见 [REQ-34](docs/requirement/34-V2与待定需求池.md)，MVP 验收后再排入 |
| 生成 | 2026-09-26 由需求文件 REQ-06～REQ-39、功能设计 DES-09～DES-41、TST-02 拆解生成；之后人工维护 |
| 状态词 | `待办` → `就绪`（依赖与设计已确认）→ `进行中` → `评审中` → `完成`；`阻塞`（写明原因） |
| 更新规则 | 开始任务改为 `进行中`；提交后在“提交”列填提交哈希；验收通过改 `完成`。新增任务追加编号，不重排；需求或设计变化先改 `docs/`，再同步本文件 |
| 编号 | 基础任务 `P0-nn`、`M1-nn`；功能 Epic `E-<REQ 号>`（如 `E-21` 对应 REQ-21），任务 `E-<REQ 号>-<序号>` |

## 1. 进度总览

| 阶段 | 目标 | 功能 Epic | 任务 | 完成 | 状态 |
| --- | --- | --- | --- | --- | --- |
| 文档（需求 → 设计 → 计划 → 测试 → 运维） | 全生命周期文档与需求设计拆解 | — | — | — | 完成（`bbd42ca7`），待评审 |
| P0 | 全能参考可用性与成本验证 | — | 6 | 0 | 未开始 |
| M1 | 工程底座、命令层、Operation 骨架 | 11 | 66 | 3 | 进行中 |
| M2 | 剧本导入、解析、设定集 | 4 | 23 | 0 | 未开始 |
| M3 | 资产定稿、分镜、生成全链路 | 11 | 60 | 0 | 未开始 |
| M4 | 视频、配音、生成增强 | 3 | 18 | 0 | 未开始 |
| M5 | 画布、Agent、运营辅助 = MVP | 4 | 20 | 0 | 未开始 |
| 合计 | | 33 | 193 | 3 | |

## 2. 当前焦点与下一步

1. **评审文档**：PRD-01、REQ、DES 均为“草案（待评审）”；评审通过后把功能 Epic 的状态从 `待办` 改为 `就绪`。
2. **启动 P0**（P0-01～P0-06）：评测结论决定主供应商、内容审核、Agent LLM 与画布方案，并回填设计中的待确认问题。
3. **M1 准备**：M1-01～M1-03 已完成（旧实现保留在标签 `legacy-2026-09`，三端骨架、工具链和本机环境已验证）；M1-04 和 M1-05 进行中，继续补齐契约、其余平台客户端和远端流水线验证。

## 3. 待确认事项

设计层面的待确认问题（共 100 条）按确认时机汇总；每条的默认方案见对应功能设计的“待确认”一节，确认前按默认实施，确认后在设计文档中更新并在此勾掉。

| 确认时机 | 数量 | 编号 |
| --- | --- | --- |
| P0 | 11 | DES-10-Q4、DES-11-Q1、DES-18-Q1、DES-29-Q2、DES-31-Q1、DES-31-Q2、DES-33-Q1、DES-38-Q1、DES-38-Q2、DES-39-Q2、DES-41-Q3 |
| M1（开始前） | 20 | DES-09-Q1、DES-09-Q2、DES-09-Q4、DES-10-Q1、DES-11-Q2、DES-11-Q3、DES-12-Q1、DES-12-Q3、DES-12-Q4、DES-13-Q2、DES-14-Q1、DES-14-Q3、DES-24-Q2、DES-24-Q3、DES-27-Q1、DES-28-Q1、DES-28-Q2、DES-35-Q3、DES-36-Q1、DES-36-Q2 |
| M1（实施中） | 14 | DES-09-Q3、DES-10-Q2、DES-10-Q3、DES-11-Q4、DES-12-Q2、DES-13-Q3、DES-14-Q2、DES-24-Q1、DES-24-Q4、DES-27-Q2、DES-28-Q3、DES-33-Q2、DES-33-Q3、DES-36-Q3 |
| M2（开始前） | 6 | DES-15-Q1、DES-15-Q3、DES-16-Q2、DES-17-Q2、DES-34-Q1、DES-34-Q3 |
| M2（实施中） | 6 | DES-15-Q2、DES-16-Q1、DES-16-Q3、DES-17-Q1、DES-17-Q3、DES-34-Q2 |
| M3（开始前） | 16 | DES-18-Q2、DES-19-Q1、DES-19-Q3、DES-20-Q1、DES-20-Q2、DES-21-Q1、DES-21-Q2、DES-21-Q4、DES-22-Q1、DES-22-Q2、DES-25-Q1、DES-25-Q2、DES-26-Q1、DES-29-Q1、DES-31-Q3、DES-35-Q2 |
| M3（实施中） | 10 | DES-13-Q1、DES-18-Q3、DES-19-Q2、DES-20-Q3、DES-21-Q3、DES-23-Q1、DES-23-Q2、DES-26-Q2、DES-26-Q3、DES-35-Q1 |
| M4（开始前） | 8 | DES-30-Q1、DES-30-Q2、DES-30-Q3、DES-32-Q1、DES-32-Q2、DES-32-Q3、DES-41-Q1、DES-41-Q2 |
| M5（开始前） | 6 | DES-37-Q1、DES-37-Q2、DES-39-Q1、DES-39-Q3、DES-40-Q1、DES-40-Q3 |
| M5（实施中） | 3 | DES-38-Q3、DES-39-Q4、DES-40-Q2 |

需求与产品层面的待确认问题见 [PRD-01 §15](docs/prd/01-产品需求文档.md#15-待确认) 与各需求文件的“待确认”。

## 4. 任务清单

### P0 技术验证（估算 3 周）

**里程碑目标**：回答“全能参考能不能用、要花多少钱”；未达标则暂停 M3 之后的工作

| 任务 | 内容 | 怎么做 | 涉及文件 | 状态 | 提交 |
| --- | --- | --- | --- | --- | --- |
| P0-01 | 全能参考评测 | 候选 Seedance（方舟）、海螺 H3、OpenRouter 海外版；5 种风格 × 单角色 / 双角色 / 动作 / 运镜 / 音频参考五类镜头；记录一致性、失败率、耗时、单镜成本 | `agent/evals/`、`docs/test/03-AI评测方案.md` | 待办 | — |
| P0-02 | 其他能力评测 | 带参考生图（Seedream、GPT Image）、TTS（MiniMax、豆包）、剧本解析 LLM（原文位置准确率） | `agent/evals/` | 待办 | — |
| P0-03 | 样片与成本模型 | 5 种风格各 1 条 30–60 秒样片；单镜成本、单分钟成本、平均重拍次数 | `docs/acceptance/P0-验收记录.md` | 待办 | — |
| P0-04 | 注册表初始配置 | 选定模型的 ModelProfile（模式、输入上限、参数 schema、价格）写入种子文件 | `backend/db/seed/catalog.yaml` | 待办 | — |
| P0-05 | 画布 PoC | React Flow + 移植 infinite-canvas，500 节点 ≥ 50 fps；核实 infinite-canvas 许可（DES-38-Q1/Q2） | `frontend/`（PoC 分支内） | 待办 | — |
| P0-06 | P0 决策收口 | 确定主供应商、内容审核服务、Agent LLM；回填 DES-11-Q1、DES-31-Q1、DES-33-Q1、DES-39-Q2、DES-41-Q3 等 | `docs/design/`、`docs/prd/01-产品需求文档.md` | 待办 | — |

### M1 工程底座与顶层抽象（估算 4 周）

**里程碑目标**：登录 → 建项目 → 上传看到缩略图；假供应商跑通报价 → 确认 → 完成；杀 worker 后恢复且账本只记一次；`unknown` 对账成功；CI 全绿

**Epic 实施顺序**：E-09 → E-06 → E-30 → E-07 → E-08 → E-10 → E-11 → E-21 → E-25 → E-24 → E-33（“依赖”为必须先完成的前置 Epic；“联调”为后实施的 Epic，本 Epic 先按契约与模拟实现推进）

| 任务 | 内容 | 怎么做 | 涉及文件 | 状态 | 提交 |
| --- | --- | --- | --- | --- | --- |
| M1-01 | 旧代码处置 | 标签 `legacy-2026-09` 已推送；旧实现与旧工程配置已删除（2026-09-26，PLN-01 §5 执行记录） | `backend/`、`agent/`、`frontend/` | 完成 | `chore(repo)` 删除旧实现（2026-09-26） |
| M1-02 | 仓库骨架与工具链 | 三端目录、锁文件；gofmt/goimports/golangci-lint、ruff/mypy、ESLint/Prettier/tsc；初版 Makefile 与 Compose 环境样例已在 M1-03 按新要求移除 | `backend/`、`agent/`、`frontend/` | 完成 | `bdc167a5`、`c214607b` |
| M1-03 | 本地环境 | 根目录 `.env` 配置三端本机进程，指向已启动的 PostgreSQL、Redis、Kafka、MinIO、Temporal；逐项健康检查与隔离的备份恢复演练；后续部署的 `docker-compose-env.yml` 独立定义依赖环境，本地不通过 Docker 启动；不使用项目脚本或 Makefile | `.env.example`、`docker-compose-env.yml`、`docs/operation/01-环境与部署.md` | 完成（环境底座；业务客户端在 M1-05 接入） | `e4038453`、`a2c11fa6`、`30769030` |
| M1-04 | CI 流水线 | lint、format、typecheck、test、race、govulncheck、契约一致性、镜像构建（OPS-02）；首批三端静态检查、测试与镜像构建已写入工作流，契约门禁待 M1-06 | `.github/workflows/` | 进行中（远端运行未验证） | `2690c95d` |
| M1-05 | 平台层 | config（Viper）、log（Zap）、db（GORM/pgx）、redis、kafka（franz-go）、minio、temporal、otel、Wire 组合根，`--role=api|worker|relay`；数据库、Redis、Temporal 客户端与 API 就绪探针已接入，Kafka 客户端只读连通性已验证，API 依赖由 Wire 生成装配；其余客户端和角色待实施 | `backend/internal/platform/`、`backend/internal/app/`、`backend/cmd/lanverse/` | 进行中 | `7f4d27ff`、`c09607ce`、`39aecd56`、`84fab235`、`4acd6529`、`ca9f78db`、`ee19a1fc`、`583ef6ed`、`fb0b6715`、`87a8b157`、`a2307b2d`、`f911c33a`、`a9f42249`、`e2541616`、`a47c0835`、`d1c69906` |
| M1-06 | 契约链 | swag → OpenAPI → `frontend/src/gen/api`；Activity 输入输出类型由 Go 与 Python 各自手写，契约测试（同一组示例输入输出）校验一致 | `backend/docs/`、`frontend/src/gen/api/` | 待办 | — |
| M1-07 | 命令层 | 鉴权（Redis 会话）、幂等键、`expected_revision`、审计、Outbox 统一中间层 | `backend/internal/command/` | 待办 | — |
| M1-08 | Outbox 与实时链路 | Outbox relay → Kafka → realtime 消费者 → Redis Pub/Sub → SSE（`/api/projects/{pid}/events`、`/api/me/events`，Last-Event-ID） | `backend/internal/infra/`、`backend/internal/app/` | 待办 | — |
| M1-09 | Temporal Worker 与 OperationWorkflow 骨架 | flow / media 队列 Worker；Operation 状态机 quote → confirm → submit → poll → ingest → moderate → settle，`unknown` → 对账（DES-04） | `backend/internal/operation/adapter/workflow/` | 待办 | — |
| M1-10 | Agent 服务骨架 | FastAPI + Temporal Activity Worker（agent 队列）；Harness 骨架（Skill Registry、执行循环、校验、预算、Trace）与假供应商适配器 | `agent/app/` | 待办 | — |
| M1-11 | 前端骨架 | App Router 布局、shadcn/ui、TanStack Query、SSE 订阅、`param_schema` 表单组件、报价确认组件框架 | `frontend/src/` | 待办 | — |

**M1-03 技术验证（2026-09-27）**：本机 `pg_isready`、`redis-cli ping`、Kafka `kafka-broker-api-versions`、MinIO live 探针和 Temporal cluster health 均通过。使用 `.env.example`（不读取现有 `.env`）直接启动三端，三个健康接口均返回 `{"status":"ok"}`。在全新临时 PostgreSQL 库写入一行，`pg_dump -Fc` → `pg_restore` 后查得原值，随后清理两个临时库及转储文件。两份 Compose YAML 分别通过配置校验，未启动容器。此证据证明本机环境和进程启动，不代表 M1-05 的业务客户端连接或 M1 总体验收。

**M1-04 当前证据（2026-09-27）**：`.github/workflows/ci.yml` 的 GitHub Actions 语法经 `actionlint v1.7.12` 检查通过；相同三端工具命令已在本机通过。`docker build` 分别构建 backend、agent、frontend 镜像通过，未启动容器。PostgreSQL、Redis 与 Kafka 连接集成测试已在本机通过并加入 CI；Wire 生成与格式化后文件哈希一致且已加入 CI。Temporal 客户端仅在本机集成验证；GitHub Actions 远端运行、契约生成物、其余跨边界测试及端到端冒烟尚无通过证据，不能计为通过。

**M1-05 数据库切片（2026-09-27）**：`LV_DB_DSN` 缺失时 API 启动前失败；使用本机全新临时 PostgreSQL 库，GORM/pgx 执行 `SELECT 1`、API `/healthz` 返回成功，库中未生成业务表，随后清理临时库。Go 格式、静态检查、Race 测试及 `govulncheck` 通过；CI 已加入 PostgreSQL 服务，但远端运行未验证。

**M1-05 Redis 切片（2026-09-27）**：`LV_REDIS_URL` 缺失或格式错误时 API 启动前失败；客户端对本机 Redis 执行 `PING` 通过。隔离临时 PostgreSQL 库下运行 API，PostgreSQL 与 Redis 可达时 `/healthz`、`/readyz` 均为 200；把 Redis 指向本机关闭端口后，`/healthz` 仍为 200、`/readyz` 为 503，临时库已清理。Go 格式、静态检查、Race 测试及 `govulncheck` 通过；CI 已加入 Redis 服务，但远端运行未验证。

**M1-05 Kafka 客户端切片（2026-09-27）**：`LV_KAFKA_BROKERS` 从根目录环境配置读取，拒绝缺失或格式错误的 broker 地址；franz-go 客户端对本机 Kafka 执行只读元数据 `Ping` 通过，未创建主题或写入事件。Go 格式、静态检查、Race 测试及 `govulncheck` 通过；CI 已加入 Kafka 服务，但远端运行未验证。SASL/TLS、Outbox 投递和消费者尚未接入；MinIO、OTel、Wire、worker/relay 角色仍未实现，M1-05 不计完成。

**M1-05 Temporal 客户端切片（2026-09-27）**：本机 Temporal 集群健康检查通过，创建项目专用 `lanverse-local` 命名空间；Go SDK 对服务与命名空间检查通过。API 使用延迟连接，Temporal 可达时 `/healthz`、`/readyz` 为 200；指向关闭端口时 API 仍运行，`/healthz` 为 200、`/readyz` 为 503。使用隔离临时 PostgreSQL 库验证后已清理；Go 格式、静态检查、Race 测试及 `govulncheck` 通过。CI 尚未提供 Temporal 测试服务，远端集成未验证；Worker 与工作流仍待 M1-09，M1-05 不计完成。

**M1-05 Wire 组合根切片（2026-09-27）**：API 的 PostgreSQL、Redis、Temporal 与 HTTP Server 由 Wire v0.7.0 生成代码装配；生成代码在后续构造失败时释放已建立的客户端。本机重生成、格式化后文件哈希一致；Go 全部门禁和三依赖可用时的 `/healthz`、`/readyz` 均通过，隔离临时库已清理。官方 Wire 仓库已归档，见 DES-08 维护风险；MinIO、OTel、worker/relay 角色及远端 CI 仍待实现，M1-05 不计完成。

**功能 Epic**

#### E-06 账号与会话

- **需求**：[REQ-06](docs/requirement/06-账号与会话.md)（ACC-01 管理员创建账号、密码登录；ACC-03 会话管理与登出；ACC-04 禁用账号、重置密码；ACC-05 修改本人密码）　**设计**：[DES-09](docs/design/09-账号与会话.md)　**依赖**：E-09
- **验收**：TC-06-01～09（9 条）
- **待确认**：DES-09-Q1、DES-09-Q2、DES-09-Q3、DES-09-Q4（默认方案见设计文档，确认前按默认实施）

| 任务 | 内容 | 怎么做 | 涉及文件 | 状态 | 提交 |
| --- | --- | --- | --- | --- | --- |
| E-06-01 | 数据与领域模型 | 迁移建表 / 加列：`identity.user`；实现领域对象、状态机与仓储（组织 / 平台级，按管理员权限访问）。详见 [DES-09 §2](docs/design/09-账号与会话.md#2-数据) | `backend/db/migrations/`、`backend/internal/identity/domain/`、`backend/internal/identity/adapter/postgres/` | 待办 | — |
| E-06-02 | 用例与接口 | 写操作经命令层（鉴权、幂等键、`expected_revision`、审计、Outbox）实现：POST /api/auth/login、POST /api/admin/users；swag 注解生成 OpenAPI。详见 [DES-09 §3](docs/design/09-账号与会话.md#3-接口) | `backend/internal/identity/application/`、`backend/internal/identity/adapter/http/`、`backend/docs/` | 待办 | — |
| E-06-03 | 异步、工作流与事件 | 事件 `identity.user_changed.v1`（Outbox → Kafka，消费者按事件 ID 去重）。要点：无工作流。账号变更事务内写 Outbox 事件 `identity.user_changed.v1` 与 `audit.recorded.v1`（审计只经后者写入，DES-12）。 详见 [DES-09 §4](docs/design/09-账号与会话.md#4-异步与工作流) | `backend/internal/identity/adapter/workflow/`、`backend/internal/identity/adapter/event/` | 待办 | — |
| E-06-04 | 前端 | - 登录页：登录名、密码、错误与锁定提示（剩余时间）。 - 强制改密页：当前密码、新密码、确认新密码，实时显示强度规则。 - 管理 · 账号：列表（登录名、显示名、角色、状态、最后登录）、创建对话框、操作菜单（修改、禁用 / 启用、重置密码）。 - 应用外壳用户菜单：修改密码、登出。 详见 [DES-09 §6](docs/design/09-账号与会话.md#6-界面) | `frontend/src/features/auth/` | 待办 | — |
| E-06-05 | 测试与验收 | 单元：密码规则、锁定计数与解锁时间、会话纪元比较、最后一个管理员保护。 集成（testcontainers PostgreSQL + Redis）：登录 → 会话 → 禁用 → 会话失效；并发修改同一账号的版本冲突。 安全：未认证访问全部接口返回 401（遍历…；验收用例 TC-06-01～09（[TST-02](docs/test/02-需求追踪矩阵.md)）。 | `backend/tests/`、`frontend/tests/` | 待办 | — |

#### E-07 供应商凭据

- **需求**：[REQ-07](docs/requirement/07-供应商凭据.md)（ACC-02 管理员配置模型供应商凭据）　**设计**：[DES-10](docs/design/10-供应商凭据.md)　**依赖**：E-06、E-09　**联调**：E-08（后实施，先按契约与模拟实现联调，对方完成后重跑本 Epic 验收）
- **验收**：TC-07-01～04（4 条）
- **待确认**：DES-10-Q1、DES-10-Q2、DES-10-Q3、DES-10-Q4（默认方案见设计文档，确认前按默认实施）

| 任务 | 内容 | 怎么做 | 涉及文件 | 状态 | 提交 |
| --- | --- | --- | --- | --- | --- |
| E-07-01 | 数据与领域模型 | 迁移建表 / 加列：`catalog.provider`、`catalog.provider_credential`；实现领域对象、状态机与仓储（组织 / 平台级，按管理员权限访问）。详见 [DES-10 §2](docs/design/10-供应商凭据.md#2-数据) | `backend/db/migrations/`、`backend/internal/catalog/domain/`、`backend/internal/catalog/adapter/postgres/` | 待办 | — |
| E-07-02 | 用例与接口 | 写操作经命令层（鉴权、幂等键、`expected_revision`、审计、Outbox）实现：POST /api/admin/providers/{id}/credentials、GET /api/admin/providers/{id}；swag 注解生成 OpenAPI。详见 [DES-10 §3](docs/design/10-供应商凭据.md#3-接口) | `backend/internal/catalog/application/`、`backend/internal/catalog/adapter/http/`、`backend/docs/` | 待办 | — |
| E-07-03 | 异步、工作流与事件 | 工作流 / Activity / 定时任务 `CredentialTestWorkflow`；事件 `catalog.credential_changed.v1`（Outbox → Kafka，消费者按事件 ID 去重）。要点：- 测试：`backend-api` 直接执行一个短工作流 `CredentialTestWorkflow` → Activity `provider.test_credential`（`agent` 队列，超时 15… 详见 [DES-10 §4](docs/design/10-供应商凭据.md#4-异步与工作流) | `backend/internal/catalog/adapter/workflow/`、`backend/internal/catalog/adapter/event/` | 待办 | — |
| E-07-04 | Agent 服务 | 供应商凭据解封与连通性测试 Activity `provider.test_credential`；输入输出类型由 Go 与 Python 各自定义，契约测试（同一组示例输入输出）校验一致，离线评测纳入 `agent/evals/`。 | `agent/app/activities/`、`agent/app/providers/` | 待办 | — |
| E-07-05 | 前端 | 供应商列表：名称、区域、凭据状态（末 4 位、最后测试结果与时间）、模型数量；凭据对话框为密码输入框，保存后不可查看。 详见 [DES-10 §6](docs/design/10-供应商凭据.md#6-界面) | `frontend/src/features/admin/` | 待办 | — |
| E-07-06 | 测试与验收 | 单元：封装 / 解封；`secret` 按适配器 schema 校验。 集成：停用后新报价失败、进行中任务继续查询；日志与响应敏感词扫描；验收用例 TC-07-01～04（[TST-02](docs/test/02-需求追踪矩阵.md)）。 | `backend/tests/`、`agent/tests/`、`frontend/tests/` | 待办 | — |

#### E-08 模型注册表与价格

- **需求**：[REQ-08](docs/requirement/08-模型注册表与价格.md)（ADM-01 模型注册表管理；CST-04 价格表；GEN-09 模型能力驱动的参数界面）　**设计**：[DES-11](docs/design/11-模型注册表与价格.md)　**依赖**：E-07
- **验收**：TC-08-01～05（5 条）
- **待确认**：DES-11-Q1、DES-11-Q2、DES-11-Q3、DES-11-Q4（默认方案见设计文档，确认前按默认实施）

| 任务 | 内容 | 怎么做 | 涉及文件 | 状态 | 提交 |
| --- | --- | --- | --- | --- | --- |
| E-08-01 | 数据与领域模型 | 迁移建表 / 加列：`catalog.capability`、`catalog.model_profile`、`catalog.model_profile_version`、`catalog.price_rule_version`；实现领域对象、状态机与仓储（组织 / 平台级，按管理员权限访问）。详见 [DES-11 §3](docs/design/11-模型注册表与价格.md#3-数据) | `backend/db/migrations/`、`backend/internal/catalog/domain/`、`backend/internal/catalog/adapter/postgres/` | 待办 | — |
| E-08-02 | 用例与接口 | 实现查询接口：GET /api/models；swag 注解生成 OpenAPI。详见 [DES-11 §4](docs/design/11-模型注册表与价格.md#4-接口) | `backend/internal/catalog/application/`、`backend/internal/catalog/adapter/http/`、`backend/docs/` | 待办 | — |
| E-08-03 | 异步、工作流与事件 | 工作流 / Activity / 定时任务 `OperationWorkflow`；事件 `catalog.model_changed.v1`、`catalog.price_changed.v1`（Outbox → Kafka，消费者按事件 ID 去重）。要点：无。模型版本中的 `queue` 与 `supports_*` 被 OperationWorkflow 读取（DES-04）。 详见 [DES-11 §5](docs/design/11-模型注册表与价格.md#5-异步与工作流) | `backend/internal/catalog/adapter/workflow/`、`backend/internal/catalog/adapter/event/` | 待办 | — |
| E-08-04 | 前端 | - 管理 · 模型注册表：列表（模型、供应商、能力、区域、状态、当前版本、价格）；详情编辑器（JSON 编辑 + 表单预览）；版本差异对比。 - 生成面板：`ModelParamsForm` 组件按 `param_schema` 渲染；`ReferenceLimitBar` 显示各用途用量 / 上限… 详见 [DES-11 §7](docs/design/11-模型注册表与价格.md#7-界面) | `frontend/src/features/admin/` | 待办 | — |
| E-08-05 | 测试与验收 | 单元：`param_schema` 与 `limits` 校验器（前后端共用同一套 JSON Schema 规则）；价格计算（按张、按秒、按 token、按字符、分辨率系数）。 集成：发布版本 → 缓存失效 → 报价使用新版本。 前端：各组件类型渲染与校验的快…；验收用例 TC-08-01～05（[TST-02](docs/test/02-需求追踪矩阵.md)）。 | `backend/tests/`、`frontend/tests/` | 待办 | — |

#### E-09 审计日志

- **需求**：[REQ-09](docs/requirement/09-审计日志.md)（ADM-02 审计日志查询；REQ-01 G6 审计约定）　**设计**：[DES-12](docs/design/12-审计日志.md)　**依赖**：—
- **验收**：TC-09-01～03（3 条）
- **待确认**：DES-12-Q1、DES-12-Q2、DES-12-Q3、DES-12-Q4（默认方案见设计文档，确认前按默认实施）

| 任务 | 内容 | 怎么做 | 涉及文件 | 状态 | 提交 |
| --- | --- | --- | --- | --- | --- |
| E-09-01 | 数据与领域模型 | 迁移建表 / 加列：`audit.audit_log`、`infra.processed_event`；实现领域对象、状态机与仓储（`audit.audit_log`：`project_id` 可空，按用户与组织授权，带项目时再按项目过滤；`infra.processed_event`：组织 / 平台级，按管理员权限访问）。详见 [DES-12 §3](docs/design/12-审计日志.md#3-数据) | `backend/db/migrations/`、`backend/internal/audit/domain/`、`backend/internal/audit/adapter/postgres/` | 待办 | — |
| E-09-02 | 用例与接口 | 实现查询接口：GET /api/admin/audit-logs、GET /api/admin/audit-logs:export；swag 注解生成 OpenAPI。详见 [DES-12 §4](docs/design/12-审计日志.md#4-接口) | `backend/internal/audit/application/`、`backend/internal/audit/adapter/http/`、`backend/docs/` | 待办 | — |
| E-09-03 | 异步、工作流与事件 | 事件 `audit.recorded.v1`（Outbox → Kafka，消费者按事件 ID 去重）。要点：audit 消费者（`backend-relay`）只消费 `audit.recorded.v1`：命令层对 REQ-09 R1 列出的每个动作（登录与账号变更、凭据、注册表与价格、预算、报价确认、取消、人工核对处理、所… 详见 [DES-12 §5](docs/design/12-审计日志.md#5-异步与工作流) | `backend/internal/audit/adapter/workflow/`、`backend/internal/audit/adapter/event/` | 待办 | — |
| E-09-04 | 前端 | 筛选栏（时间范围、操作人、项目、对象类型、动作）+ 虚拟滚动表格 + 详情抽屉（前后值差异）。 详见 [DES-12 §7](docs/design/12-审计日志.md#7-界面) | `frontend/src/features/admin/` | 待办 | — |
| E-09-05 | 测试与验收 | 命令覆盖测试：遍历 R1 中每个命令，断言产生对应审计记录。 消费者幂等：重复投递同一事件只产生一条记录；验收用例 TC-09-01～03（[TST-02](docs/test/02-需求追踪矩阵.md)）。 | `backend/tests/`、`frontend/tests/` | 待办 | — |

#### E-10 项目管理

- **需求**：[REQ-10](docs/requirement/10-项目管理.md)（PRJ-01 创建项目；PRJ-02 项目列表与概览；PRJ-06 归档与删除；PRJ-07 生成策略设置）　**设计**：[DES-13](docs/design/13-项目管理.md)　**依赖**：E-06、E-08　**联调**：E-11（后实施，先按契约与模拟实现联调，对方完成后重跑本 Epic 验收）
- **验收**：TC-10-01～06（6 条）
- **待确认**：DES-13-Q1、DES-13-Q2、DES-13-Q3（默认方案见设计文档，确认前按默认实施）

| 任务 | 内容 | 怎么做 | 涉及文件 | 状态 | 提交 |
| --- | --- | --- | --- | --- | --- |
| E-10-01 | 数据与领域模型 | 迁移建表 / 加列：`billing.budget`、`workspace.project`、`workspace.style_preset`；实现领域对象、状态机与仓储（`billing.budget`：仓储查询强制带 `project_id`；`workspace.project`：项目本身，按项目成员授权；`workspace.style_preset`：`project_id` 可空，按用户与组织授权，带项目时再按项目过滤）。详见 [DES-13 §2](docs/design/13-项目管理.md#2-数据) | `backend/db/migrations/`、`backend/internal/workspace/domain/`、`backend/internal/workspace/adapter/postgres/` | 待办 | — |
| E-10-02 | 用例与接口 | 写操作经命令层（鉴权、幂等键、`expected_revision`、审计、Outbox）实现：POST /api/projects、GET /api/projects/{pid}/overview、DELETE /api/projects/{pid}；swag 注解生成 OpenAPI。详见 [DES-13 §3](docs/design/13-项目管理.md#3-接口) | `backend/internal/workspace/application/`、`backend/internal/workspace/adapter/http/`、`backend/docs/` | 待办 | — |
| E-10-03 | 异步、工作流与事件 | 工作流 / Activity / 定时任务 `project-purge`；事件 `workspace.project_changed.v1`（Outbox → Kafka，消费者按事件 ID 去重）。要点：- `project-purge`（每日）：对 `purge_after < now()` 的项目执行分批清理工作流（删除对象存储前缀、删除各 schema 数据），可断点续跑。 - 概览物化：`realtime` 之外… 详见 [DES-13 §4](docs/design/13-项目管理.md#4-异步与工作流) | `backend/internal/workspace/adapter/workflow/`、`backend/internal/workspace/adapter/event/` | 待办 | — |
| E-10-04 | 前端 | 项目列表（卡片 / 表格切换、状态筛选、回收站视图）；新建项目对话框（风格类型切换后显示子风格与预设缩略图）；项目概览矩阵（单元格点击跳转）；设置页（画幅与风格只读并说明原因）。 详见 [DES-13 §6](docs/design/13-项目管理.md#6-界面) | `frontend/src/features/project/` | 待办 | — |
| E-10-05 | 测试与验收 | 单元：概览阶段计算；生命周期状态机。 集成：清理工作流中断后续跑；触发器拒绝修改画幅；验收用例 TC-10-01～06（[TST-02](docs/test/02-需求追踪矩阵.md)）。 | `backend/tests/`、`frontend/tests/` | 待办 | — |

#### E-11 项目预算

- **需求**：[REQ-11](docs/requirement/11-项目预算.md)（PRJ-03 项目预算）　**设计**：[DES-14](docs/design/14-项目预算.md)　**依赖**：E-10　**联调**：E-21、E-32（后实施，先按契约与模拟实现联调，对方完成后重跑本 Epic 验收）
- **验收**：TC-11-01～04（4 条）
- **待确认**：DES-14-Q1、DES-14-Q2、DES-14-Q3（默认方案见设计文档，确认前按默认实施）

| 任务 | 内容 | 怎么做 | 涉及文件 | 状态 | 提交 |
| --- | --- | --- | --- | --- | --- |
| E-11-01 | 数据与领域模型 | 迁移建表 / 加列：`billing.budget`、`billing.ledger_entry`（其中 `billing.budget` 由 E-10 建表，本 Epic 只加列或复用）；实现领域对象、状态机与仓储（仓储查询强制带 `project_id`）。详见 [DES-14 §2](docs/design/14-项目预算.md#2-数据) | `backend/db/migrations/`、`backend/internal/billing/domain/`、`backend/internal/billing/adapter/postgres/` | 待办 | — |
| E-11-02 | 用例与接口 | 写操作经命令层（鉴权、幂等键、`expected_revision`、审计、Outbox）实现：GET /api/projects/{pid}/budget、PUT /api/projects/{pid}/budget；swag 注解生成 OpenAPI。详见 [DES-14 §3](docs/design/14-项目预算.md#3-接口) | `backend/internal/billing/application/`、`backend/internal/billing/adapter/http/`、`backend/docs/` | 待办 | — |
| E-11-03 | 异步、工作流与事件 | 事件 `billing.budget_changed.v1`、`billing.budget_low.v1`、`billing.budget_overrun.v1`（Outbox → Kafka，消费者按事件 ID 去重）。要点：无；预留与结算在 `flow` 队列 Activity 的数据库事务中执行（DES-04）。 详见 [DES-14 §4](docs/design/14-项目预算.md#4-异步与工作流) | `backend/internal/billing/adapter/workflow/`、`backend/internal/billing/adapter/event/` | 待办 | — |
| E-11-04 | 前端 | 设置页预算卡片（上限、已结算、已预留、可用、使用率进度条）；报价对话框显示剩余预算与差额；顶部低余额横幅。 详见 [DES-14 §6](docs/design/14-项目预算.md#6-界面) | `frontend/src/features/project/` | 待办 | — |
| E-11-05 | 测试与验收 | 并发确认测试（race）；结算超支路径；阈值通知去重；验收用例 TC-11-01～04（[TST-02](docs/test/02-需求追踪矩阵.md)）。 | `backend/tests/`、`frontend/tests/` | 待办 | — |

#### E-21 报价与二次确认

- **需求**：[REQ-21](docs/requirement/21-报价与二次确认.md)（GEN-01 生成前报价；GEN-11 付费生成二次确认；CST-03 预留与结算）　**设计**：[DES-24](docs/design/24-报价与二次确认.md)　**依赖**：E-08、E-10、E-11　**联调**：E-25、E-32（后实施，先按契约与模拟实现联调，对方完成后重跑本 Epic 验收）
- **验收**：TC-21-01～06（6 条）
- **待确认**：DES-24-Q1、DES-24-Q2、DES-24-Q3、DES-24-Q4（默认方案见设计文档，确认前按默认实施）

| 任务 | 内容 | 怎么做 | 涉及文件 | 状态 | 提交 |
| --- | --- | --- | --- | --- | --- |
| E-21-01 | 数据与领域模型 | 迁移建表 / 加列：`billing.budget`、`billing.ledger_entry`、`billing.reservation`、`operation.batch`、`operation.operation`、`operation.operation_input`（其中 `billing.budget` 由 E-10 建表，本 Epic 只加列或复用、`billing.ledger_entry` 由 E-11 建表，本 Epic 只加列或复用）；实现领域对象、状态机与仓储（`billing.budget`、`billing.ledger_entry`、`billing.reservation`、`operation.batch`、`operation.operation`、`billing.budget`、`billing.ledger_entry`：仓储查询强制带 `project_id`；`operation.operation_input`：无 `project_id`，经父对象外键继承项目范围校验）。详见 [DES-24 §3](docs/design/24-报价与二次确认.md#3-数据) | `backend/db/migrations/`、`backend/internal/operation/domain/`、`backend/internal/operation/adapter/postgres/` | 待办 | — |
| E-21-02 | 用例与接口 | 写操作经命令层（鉴权、幂等键、`expected_revision`、审计、Outbox）实现：POST /api/projects/{pid}/quotes、POST /api/batches/{id}:confirm、POST /api/operations/{id}:confirm；swag 注解生成 OpenAPI。详见 [DES-24 §4](docs/design/24-报价与二次确认.md#4-接口) | `backend/internal/operation/application/`、`backend/internal/operation/adapter/http/`、`backend/docs/` | 待办 | — |
| E-21-03 | 异步、工作流与事件 | 工作流 / Activity / 定时任务 `OperationWorkflow`、`BatchWorkflow`、`flow.SettleOperation`、`quote-expiry`；事件 `operation.confirmed.v1`（单项）、`batch.confirmed.v1`（批量）（Outbox → Kafka，消费者按事件 ID 去重）。要点：确认事务提交后启动 `OperationWorkflow`（单项）或 `BatchWorkflow`（批量；`agent_session` 例外，不启动工作流，见 DES-04 §8.7）；工作流型操作的结算由工作流最后… 详见 [DES-24 §5](docs/design/24-报价与二次确认.md#5-异步与工作流) | `backend/internal/operation/adapter/workflow/`、`backend/internal/operation/adapter/event/` | 待办 | — |
| E-21-04 | 前端 | `QuoteConfirmDialog` 组件（REQ-05 §4.1）：逐项费用（可展开）、合计、剩余预算、倒计时、区域提示、错误项列表与“剔除”操作；确认按钮不响应回车；按钮文案“生成（约 ¥X）”由 `useQuote` 钩子统一计算。 详见 [DES-24 §7](docs/design/24-报价与二次确认.md#7-界面) | `frontend/src/features/operation/` | 待办 | — |
| E-21-05 | 测试与验收 | 单元：计价（各计价单位、系数）、`input_hash` 规范化、报价失效判断。 集成：确认事务（预算约束、并发）；工作流启动失败恢复；复用路径。 故障注入：确认后立即重启 API 与 Worker，任务仍被执行一次；验收用例 TC-21-01～06（[TST-02](docs/test/02-需求追踪矩阵.md)）。 | `backend/tests/`、`frontend/tests/` | 待办 | — |

#### E-24 任务中心、取消与对账

- **需求**：[REQ-24](docs/requirement/24-任务中心与对账.md)（GEN-04 任务中心；GEN-06 结果未知先对账；GEN-07 取消任务）　**设计**：[DES-27](docs/design/27-任务中心与对账.md)　**依赖**：E-21　**联调**：E-23、E-33（后实施，先按契约与模拟实现联调，对方完成后重跑本 Epic 验收）
- **验收**：TC-24-01～04（4 条）
- **待确认**：DES-27-Q1、DES-27-Q2（默认方案见设计文档，确认前按默认实施）

| 任务 | 内容 | 怎么做 | 涉及文件 | 状态 | 提交 |
| --- | --- | --- | --- | --- | --- |
| E-24-01 | 数据与领域模型 | 迁移建表 / 加列：`operation.operation`、`operation.operation_event`、`operation.provider_call`（其中 `operation.operation` 由 E-21 建表，本 Epic 只加列或复用）；实现领域对象、状态机与仓储（`operation.operation`、`operation.provider_call`、`operation.operation`：仓储查询强制带 `project_id`；`operation.operation_event`：无 `project_id`，经父对象外键继承项目范围校验）。详见 [DES-27 §2](docs/design/27-任务中心与对账.md#2-数据) | `backend/db/migrations/`、`backend/internal/operation/domain/`、`backend/internal/operation/adapter/postgres/` | 待办 | — |
| E-24-02 | 用例与接口 | 写操作经命令层（鉴权、幂等键、`expected_revision`、审计、Outbox）实现：GET /api/projects/{pid}/operations、GET /api/operations/{id}、POST /api/operations/{id}:cancel、POST /api/operations/{id}:retry、POST /api/operations/{id}:resolve-manual；swag 注解生成 OpenAPI。详见 [DES-27 §3](docs/design/27-任务中心与对账.md#3-接口) | `backend/internal/operation/application/`、`backend/internal/operation/adapter/http/`、`backend/docs/` | 待办 | — |
| E-24-03 | 异步、工作流与事件 | 工作流 / Activity / 定时任务 `OperationWorkflow`、`inflight-watchdog`；事件 `operation.manual.v1`、`operation.status_changed.v1`（Outbox → Kafka，消费者按事件 ID 去重）。要点：OperationWorkflow 的提交、轮询、对账、取消、人工核对分支见 DES-04；`inflight-watchdog`（每 5 分钟）检测 R4 并向工作流发送 `reconcile` 信号。 详见 [DES-27 §4](docs/design/27-任务中心与对账.md#4-异步与工作流) | `backend/internal/operation/adapter/workflow/`、`backend/internal/operation/adapter/event/` | 待办 | — |
| E-24-04 | 前端 | 任务中心表格（筛选栏、状态徽标、进度、费用、耗时、操作：取消 / 重试 / 查看）；详情抽屉（时间线、输入缩略图、调用明细、费用）；管理员“待人工核对”视图与处理表单。 详见 [DES-27 §6](docs/design/27-任务中心与对账.md#6-界面) | `frontend/src/features/operation/` | 待办 | — |
| E-24-05 | 测试与验收 | 模拟供应商（REQ-02 DEP-06）注入：超时但已受理、超时且未受理、重复回调、结果链接过期、查询接口 5xx；每种场景断言状态与账本；验收用例 TC-24-01～04（[TST-02](docs/test/02-需求追踪矩阵.md)）。 | `backend/tests/`、`frontend/tests/` | 待办 | — |

#### E-25 结果复用与写入边界

- **需求**：[REQ-25](docs/requirement/25-结果复用与写入边界.md)（GEN-05 相同输入复用结果；GEN-10 任务不覆盖人工配置）　**设计**：[DES-28](docs/design/28-结果复用与写入边界.md)　**依赖**：E-21
- **验收**：TC-25-01～03（3 条）
- **待确认**：DES-28-Q1、DES-28-Q2、DES-28-Q3（默认方案见设计文档，确认前按默认实施）

| 任务 | 内容 | 怎么做 | 涉及文件 | 状态 | 提交 |
| --- | --- | --- | --- | --- | --- |
| E-25-01 | 异步、工作流与事件 | 工作流 / Activity / 定时任务 `OperationWorkflow`、`flow.CompleteFromReuse`。要点：复用项确认后不启动供应商流程：`OperationWorkflow` 直接执行 `flow.CompleteFromReuse`。 详见 [DES-28 §4](docs/design/28-结果复用与写入边界.md#4-异步与工作流) | `backend/internal/operation/adapter/workflow/`、`backend/internal/operation/adapter/event/` | 待办 | — |
| E-25-02 | 前端 | 报价对话框中复用项显示“复用已有结果 ¥0”与“强制重新生成”开关；候选卡片显示“基于 vN 生成”。 详见 [DES-28 §6](docs/design/28-结果复用与写入边界.md#6-界面) | `frontend/src/features/operation/` | 待办 | — |
| E-25-03 | 测试与验收 | `input_hash` 规范化的表驱动测试（等价与不等价用例）；架构依赖规则测试；验收用例 TC-25-01～03（[TST-02](docs/test/02-需求追踪矩阵.md)）。 | `backend/tests/`、`frontend/tests/` | 待办 | — |

#### E-30 媒体库

- **需求**：[REQ-30](docs/requirement/30-媒体库.md)（MED-01 上传素材；MED-02 浏览、检索与预览；MED-03 删除素材；MED-04 来源与合规信息）　**设计**：[DES-33](docs/design/33-媒体库.md)　**依赖**：—　**联调**：E-17（后实施，先按契约与模拟实现联调，对方完成后重跑本 Epic 验收）
- **验收**：TC-30-01～05（5 条）
- **待确认**：DES-33-Q1、DES-33-Q2、DES-33-Q3（默认方案见设计文档，确认前按默认实施）

| 任务 | 内容 | 怎么做 | 涉及文件 | 状态 | 提交 |
| --- | --- | --- | --- | --- | --- |
| E-30-01 | 数据与领域模型 | 迁移建表 / 加列：`media.media_asset`、`media.rendition`；实现领域对象、状态机与仓储（`media.media_asset`：仓储查询强制带 `project_id`；`media.rendition`：无 `project_id`，经父对象外键继承项目范围校验）。详见 [DES-33 §3](docs/design/33-媒体库.md#3-数据) | `backend/db/migrations/`、`backend/internal/media/domain/`、`backend/internal/media/adapter/postgres/` | 待办 | — |
| E-30-02 | 用例与接口 | 写操作经命令层（鉴权、幂等键、`expected_revision`、审计、Outbox）实现：POST /api/projects/{pid}/uploads、POST /api/uploads/{media_asset_id}:complete、GET /api/projects/{pid}/media、GET /api/media/{id}、DELETE /api/media/{id}；swag 注解生成 OpenAPI。详见 [DES-33 §4](docs/design/33-媒体库.md#4-接口) | `backend/internal/media/application/`、`backend/internal/media/adapter/http/`、`backend/docs/` | 待办 | — |
| E-30-03 | 异步、工作流与事件 | 工作流 / Activity / 定时任务 `MediaIngestWorkflow`、`media.DetectAndProbe`、`media.Hash`、`media.MakeRenditions`、`flow.MarkMediaReady`、`media-purge`；事件 `media.asset_status_changed.v1`（Outbox → Kafka，消费者按事件 ID 去重）。要点：`MediaIngestWorkflow`（`media-ingest/{id}`）：`media.DetectAndProbe` → `media.Hash` → `media.MakeRenditions`（并行）→… 详见 [DES-33 §5](docs/design/33-媒体库.md#5-异步与工作流) | `backend/internal/media/adapter/workflow/`、`backend/internal/media/adapter/event/` | 待办 | — |
| E-30-04 | Agent 服务 | 内容审核适配器 `moderation.check`；输入输出类型由 Go 与 Python 各自定义，契约测试（同一组示例输入输出）校验一致，离线评测纳入 `agent/evals/`。 | `agent/app/moderation/` | 待办 | — |
| E-30-05 | 前端 | 媒体库网格 / 列表切换（虚拟滚动）、筛选栏、搜索；拖拽上传区（多文件进度、失败原因）；预览（图片灯箱、视频播放器、音频波形）；详情抽屉（来源、合规、引用位置）。 详见 [DES-33 §7](docs/design/33-媒体库.md#7-界面) | `frontend/src/features/media/` | 待办 | — |
| E-30-06 | 测试与验收 | 内容类型识别；分片上传续传；引用检查覆盖全部引用来源（表驱动）；清理任务；验收用例 TC-30-01～05（[TST-02](docs/test/02-需求追踪矩阵.md)）。 | `backend/tests/`、`agent/tests/`、`frontend/tests/` | 待办 | — |

#### E-33 站内通知

- **需求**：[REQ-33](docs/requirement/33-站内通知.md)（NTF-01 站内通知）　**设计**：[DES-36](docs/design/36-站内通知.md)　**依赖**：E-11、E-24　**联调**：E-17、E-23、E-32（后实施，先按契约与模拟实现联调，对方完成后重跑本 Epic 验收）
- **验收**：TC-33-01～02（2 条）
- **待确认**：DES-36-Q1、DES-36-Q2、DES-36-Q3（默认方案见设计文档，确认前按默认实施）

| 任务 | 内容 | 怎么做 | 涉及文件 | 状态 | 提交 |
| --- | --- | --- | --- | --- | --- |
| E-33-01 | 数据与领域模型 | 迁移建表 / 加列：`notify.notification`；实现领域对象、状态机与仓储（`project_id` 可空，按用户与组织授权，带项目时再按项目过滤）。详见 [DES-36 §2](docs/design/36-站内通知.md#2-数据) | `backend/db/migrations/`、`backend/internal/notify/domain/`、`backend/internal/notify/adapter/postgres/` | 待办 | — |
| E-33-02 | 用例与接口 | 写操作经命令层（鉴权、幂等键、`expected_revision`、审计、Outbox）实现：GET /api/notifications、POST /api/notifications/{id}:read、POST /api/notifications:read-all、GET /api/me/events；swag 注解生成 OpenAPI。详见 [DES-36 §3](docs/design/36-站内通知.md#3-接口) | `backend/internal/notify/application/`、`backend/internal/notify/adapter/http/`、`backend/docs/` | 待办 | — |
| E-33-03 | 异步、工作流与事件 | 工作流 / Activity / 定时任务 消费者 `notify`（`backend-relay`，按 §4 把事件转换为通知并发布 SSE）；事件 消费 `batch.finished.v1`、`operation.failed.v1`、`operation.manual.v1`、`billing.budget_low.v1`、`billing.budget_overrun.v1`、`script.version_imported.v1`、`media.consent_revoked.v1`、`billing.reconciliation_diff.v1`（Outbox → Kafka，消费者按事件 ID 去重）。要点：`notify` 消费者（`backend-relay`）按下表把事件转换为通知，并发布 SSE。事件的生产方与消费者清单以 DES-03 §6.2 为准，本表只定义通知规则（类型、接收人、去重键）： / 通知类型（RE… 详见 [DES-36 §4](docs/design/36-站内通知.md#4-异步与工作流) | `backend/internal/notify/adapter/workflow/`、`backend/internal/notify/adapter/event/` | 待办 | — |
| E-33-04 | 前端 | 顶栏铃铛（未读数）→ 下拉列表（分组：今天 / 更早）→ 点击跳转；Sonner toast 即时提示。 详见 [DES-36 §6](docs/design/36-站内通知.md#6-界面) | `frontend/src/features/notify/` | 待办 | — |
| E-33-05 | 测试与验收 | 去重；接收人计算；验收用例 TC-33-01～02（[TST-02](docs/test/02-需求追踪矩阵.md)）。 | `backend/tests/`、`frontend/tests/` | 待办 | — |

### M2 剧本到设定集（估算 3 周）

**里程碑目标**：≥ 10 集真实剧本导入、分集、逐集解析可追溯原文；设定集抽取与别名合并；修改后下游过期

**Epic 实施顺序**：E-12 → E-13 → E-14 → E-31（“依赖”为必须先完成的前置 Epic；“联调”为后实施的 Epic，本 Epic 先按契约与模拟实现推进）

**功能 Epic**

#### E-12 剧本导入与分集

- **需求**：[REQ-12](docs/requirement/12-剧本导入与分集.md)（SCR-01 导入整部剧；SCR-02 分集识别与调整）　**设计**：[DES-15](docs/design/15-剧本导入与分集.md)　**依赖**：E-10、E-21、E-30
- **验收**：TC-12-01～05（5 条）
- **待确认**：DES-15-Q1、DES-15-Q2、DES-15-Q3（默认方案见设计文档，确认前按默认实施）

| 任务 | 内容 | 怎么做 | 涉及文件 | 状态 | 提交 |
| --- | --- | --- | --- | --- | --- |
| E-12-01 | 数据与领域模型 | 迁移建表 / 加列：`operation.operation`、`script.episode`、`script.script_source`、`script.script_version`（其中 `operation.operation` 由 E-21 建表，本 Epic 只加列或复用）；实现领域对象、状态机与仓储（仓储查询强制带 `project_id`）。详见 [DES-15 §3](docs/design/15-剧本导入与分集.md#3-数据) | `backend/db/migrations/`、`backend/internal/script/domain/`、`backend/internal/script/adapter/postgres/` | 待办 | — |
| E-12-02 | 用例与接口 | 写操作经命令层（鉴权、幂等键、`expected_revision`、审计、Outbox）实现：POST /api/projects/{pid}/script-imports、GET /api/projects/{pid}/script-imports/{import_id}、GET /api/projects/{pid}/episodes、POST /api/projects/{pid}/episodes/split:confirm；swag 注解生成 OpenAPI。详见 [DES-15 §4](docs/design/15-剧本导入与分集.md#4-接口) | `backend/internal/script/application/`、`backend/internal/script/adapter/http/`、`backend/docs/` | 待办 | — |
| E-12-03 | 异步、工作流与事件 | 工作流 / Activity / 定时任务 `ScriptImportWorkflow`；事件 `script.version_imported.v1`、`script.split_confirmed.v1`（Outbox → Kafka，消费者按事件 ID 去重）。要点：导入由 `ScriptImportWorkflow`（`script-import/{import_id}`）执行抽取、规范化、去重、保存版本与规则分集，写入规则分集候选；需要 AI 分集时生成 `episode_spl… 详见 [DES-15 §5](docs/design/15-剧本导入与分集.md#5-异步与工作流) | `backend/internal/script/adapter/workflow/`、`backend/internal/script/adapter/event/` | 待办 | — |
| E-12-04 | Agent 服务 | Skill `split_episodes`（规则分集失败时）；输入输出类型由 Go 与 Python 各自定义，契约测试（同一组示例输入输出）校验一致，离线评测纳入 `agent/evals/`。 | `agent/skills/split_episodes/`、`agent/evals/` | 待办 | — |
| E-12-05 | 前端 | - 剧本页：导入按钮 → 多文件拖拽区（显示每个文件的校验结果与顺序）→ 版权确认复选框。 - 分集编辑器：左侧集列表（集号、标题、字数、首尾预览），右侧原文；在原文中点击“在此处拆分”、在列表中“与下一集合并”、编辑标题；顶部“确认分集”。 详见 [DES-15 §7](docs/design/15-剧本导入与分集.md#7-界面) | `frontend/src/features/script/` | 待办 | — |
| E-12-06 | 测试与验收 | 单元：编码识别（UTF-8、GBK、GB18030、带 BOM）；集号识别正则；边界校验（重叠、遗漏）。 评测：10 部样例剧本的分集准确率（TST-03）。 集成：导入工作流中断续跑；重复导入；验收用例 TC-12-01～05（[TST-02](docs/test/02-需求追踪矩阵.md)）。 | `backend/tests/`、`agent/tests/`、`frontend/tests/` | 待办 | — |

#### E-13 逐集结构解析

- **需求**：[REQ-13](docs/requirement/13-逐集结构解析.md)（SCR-03 逐集结构解析；SCR-04 解析结果人工修改；SCR-05 解析按集独立执行）　**设计**：[DES-16](docs/design/16-逐集结构解析.md)　**依赖**：E-12、E-21　**联调**：E-31（后实施，先按契约与模拟实现联调，对方完成后重跑本 Epic 验收）
- **验收**：TC-13-01～04（4 条）
- **待确认**：DES-16-Q1、DES-16-Q2、DES-16-Q3（默认方案见设计文档，确认前按默认实施）

| 任务 | 内容 | 怎么做 | 涉及文件 | 状态 | 提交 |
| --- | --- | --- | --- | --- | --- |
| E-13-01 | 数据与领域模型 | 迁移建表 / 加列：`lineage.dependency`、`script.action_line`、`script.dialogue_line`、`script.episode`、`script.episode_structure`、`script.scene`（其中 `script.episode` 由 E-12 建表，本 Epic 只加列或复用）；实现领域对象、状态机与仓储（`lineage.dependency`、`script.episode`、`script.episode_structure`、`script.episode`：仓储查询强制带 `project_id`；`script.action_line`、`script.dialogue_line`、`script.scene`：无 `project_id`，经父对象外键继承项目范围校验）。详见 [DES-16 §3](docs/design/16-逐集结构解析.md#3-数据) | `backend/db/migrations/`、`backend/internal/script/domain/`、`backend/internal/script/adapter/postgres/` | 待办 | — |
| E-13-02 | 用例与接口 | 写操作经命令层（鉴权、幂等键、`expected_revision`、审计、Outbox）实现：POST /api/projects/{pid}/quotes、GET /api/episodes/{eid}/structure、POST /api/episodes/{eid}/structure、POST /api/episodes/{eid}/structure:confirm、GET /api/episodes/{eid}/source-text、POST /api/episodes/{eid}/parse:retry；swag 注解生成 OpenAPI。详见 [DES-16 §4](docs/design/16-逐集结构解析.md#4-接口) | `backend/internal/script/application/`、`backend/internal/script/adapter/http/`、`backend/docs/` | 待办 | — |
| E-13-03 | 异步、工作流与事件 | 工作流 / Activity / 定时任务 `OperationWorkflow`（LLM 同步分支，`target_type = episode_parse`）、`BatchWorkflow`；事件 `script.episode_structure_confirmed.v1`（Outbox → Kafka，消费者按事件 ID 去重）。要点：每集一个 `OperationWorkflow`（`operation/{operation_id}`，`target_type = episode_parse`）的 LLM 同步分支（DES-04 §3、§8.3）：`… 详见 [DES-16 §5](docs/design/16-逐集结构解析.md#5-异步与工作流) | `backend/internal/script/adapter/workflow/`、`backend/internal/script/adapter/event/` | 待办 | — |
| E-13-04 | Agent 服务 | Skill `parse_episode`（原文位置校验与修复）；输入输出类型由 Go 与 Python 各自定义，契约测试（同一组示例输入输出）校验一致，离线评测纳入 `agent/evals/`。 | `agent/skills/parse_episode/`、`agent/app/harness/validators/`、`agent/evals/` | 待办 | — |
| E-13-05 | 前端 | 双栏：原文（虚拟滚动，高亮当前选中条目）/ 结构树（场 → 动作与台词，行内编辑说话人、情绪、内容；拖拽调整归属；拆场 / 合场按钮）；“待处理”抽屉；版本下拉与差异对比；确认按钮（未处理完禁用并说明）。 详见 [DES-16 §7](docs/design/16-逐集结构解析.md#7-界面) | `frontend/src/features/script/` | 待办 | — |
| E-13-06 | 测试与验收 | 单元：稳定键保持算法（编辑、拆分、合并）；`content_hash`；偏移校验。 评测：样例剧本解析指标（TST-03）。 集成：确认 → 过期事件 → 镜头过期；验收用例 TC-13-01～04（[TST-02](docs/test/02-需求追踪矩阵.md)）。 | `backend/tests/`、`agent/tests/`、`frontend/tests/` | 待办 | — |

#### E-14 设定集抽取与造型

- **需求**：[REQ-14](docs/requirement/14-设定集抽取与造型.md)（BIB-01 全剧设定抽取与别名合并；BIB-02 角色造型）　**设计**：[DES-17](docs/design/17-设定集抽取与造型.md)　**依赖**：E-13、E-21
- **验收**：TC-14-01～04（4 条）
- **待确认**：DES-17-Q1、DES-17-Q2、DES-17-Q3（默认方案见设计文档，确认前按默认实施）

| 任务 | 内容 | 怎么做 | 涉及文件 | 状态 | 提交 |
| --- | --- | --- | --- | --- | --- |
| E-14-01 | 数据与领域模型 | 迁移建表 / 加列：`bible.character`、`bible.episode_asset_usage`、`bible.location`、`bible.look`、`bible.prop`；实现领域对象、状态机与仓储（`bible.character`、`bible.location`、`bible.look`、`bible.prop`：仓储查询强制带 `project_id`；`bible.episode_asset_usage`：无 `project_id`，经父对象外键继承项目范围校验）。详见 [DES-17 §3](docs/design/17-设定集抽取与造型.md#3-数据) | `backend/db/migrations/`、`backend/internal/bible/domain/`、`backend/internal/bible/adapter/postgres/` | 待办 | — |
| E-14-02 | 用例与接口 | 写操作经命令层（鉴权、幂等键、`expected_revision`、审计、Outbox）实现：POST /api/projects/{pid}/quotes、GET /api/projects/{pid}/characters、POST /api/characters/{id}:merge、POST /api/characters/{id}:split、POST /api/projects/{pid}/bible:confirm、GET /api/projects/{pid}/speaker-mapping、POST /api/projects/{pid}/speaker-mapping、POST /api/characters/{id}/looks；swag 注解生成 OpenAPI。详见 [DES-17 §4](docs/design/17-设定集抽取与造型.md#4-接口) | `backend/internal/bible/application/`、`backend/internal/bible/adapter/http/`、`backend/docs/` | 待办 | — |
| E-14-03 | 异步、工作流与事件 | 工作流 / Activity / 定时任务 `OperationWorkflow`（LLM 同步分支，`target_type = bible_extract`）；事件 `bible.entries_changed.v1`（Outbox → Kafka，消费者按事件 ID 去重）。要点：`OperationWorkflow`（`target_type = bible_extract`）的 LLM 同步分支（DES-04 §3）：`llm.run_skill(extract_bible)`（可分块，见 D… 详见 [DES-17 §5](docs/design/17-设定集抽取与造型.md#5-异步与工作流) | `backend/internal/bible/adapter/workflow/`、`backend/internal/bible/adapter/event/` | 待办 | — |
| E-14-04 | Agent 服务 | Skill `extract_bible`（分块抽取 + 别名合并）；输入输出类型由 Go 与 Python 各自定义，契约测试（同一组示例输入输出）校验一致，离线评测纳入 `agent/evals/`。 | `agent/skills/extract_bible/`、`agent/evals/` | 待办 | — |
| E-14-05 | 前端 | 三栏列表（角色 / 场景 / 道具）；多选合并；条目详情：别名标签、描述、出场集时间轴、造型卡片；说话人映射对话框。 详见 [DES-17 §7](docs/design/17-设定集抽取与造型.md#7-界面) | `frontend/src/features/bible/` | 待办 | — |
| E-14-06 | 测试与验收 | 合并 / 拆分对引用与台词回填的影响；分块抽取的合并正确性（评测集）；验收用例 TC-14-01～04（[TST-02](docs/test/02-需求追踪矩阵.md)）。 | `backend/tests/`、`agent/tests/`、`frontend/tests/` | 待办 | — |

#### E-31 依赖传播与影响清单

- **需求**：[REQ-31](docs/requirement/31-依赖传播与影响清单.md)（LIN-01 依赖传播；LIN-02 影响范围与批量重做）　**设计**：[DES-34](docs/design/34-依赖传播与影响清单.md)　**依赖**：E-13　**联调**：E-15、E-16、E-18、E-19、E-22、E-23、E-29（后实施，先按契约与模拟实现联调，对方完成后重跑本 Epic 验收）
- **验收**：TC-31-01～05（5 条）
- **待确认**：DES-34-Q1、DES-34-Q2、DES-34-Q3（默认方案见设计文档，确认前按默认实施）

| 任务 | 内容 | 怎么做 | 涉及文件 | 状态 | 提交 |
| --- | --- | --- | --- | --- | --- |
| E-31-01 | 数据与领域模型 | 迁移建表 / 加列：`lineage.dependency`、`lineage.stale_mark`（其中 `lineage.dependency` 由 E-13 建表，本 Epic 只加列或复用）；实现领域对象、状态机与仓储（仓储查询强制带 `project_id`）。详见 [DES-34 §2](docs/design/34-依赖传播与影响清单.md#2-数据) | `backend/db/migrations/`、`backend/internal/lineage/domain/`、`backend/internal/lineage/adapter/postgres/` | 待办 | — |
| E-31-02 | 用例与接口 | 写操作经命令层（鉴权、幂等键、`expected_revision`、审计、Outbox）实现：GET /api/projects/{pid}/impact、POST /api/projects/{pid}/impact:quote、POST /api/projects/{pid}/impact:dismiss；swag 注解生成 OpenAPI。详见 [DES-34 §3](docs/design/34-依赖传播与影响清单.md#3-接口) | `backend/internal/lineage/application/`、`backend/internal/lineage/adapter/http/`、`backend/docs/` | 待办 | — |
| E-31-03 | 异步、工作流与事件 | 事件 `lineage.stale_marked.v1`（Outbox → Kafka，消费者按事件 ID 去重）。要点：`lineage` 消费者（`backend-relay`）：接收上游事件 → 递归 CTE 查询下游 → 批量写 `stale_mark` → 发 `lineage.stale_marked.v1`；清除逻辑在下游对象… 详见 [DES-34 §4](docs/design/34-依赖传播与影响清单.md#4-异步与工作流) | `backend/internal/lineage/adapter/workflow/`、`backend/internal/lineage/adapter/event/` | 待办 | — |
| E-31-04 | 前端 | 各列表中的“过期”徽标（悬停显示原因）；影响清单页（分组、勾选、合计预估、“为选中项报价”“标记为无需重做”）；改锁 / 改选前的影响确认框。 详见 [DES-34 §6](docs/design/34-依赖传播与影响清单.md#6-界面) | `frontend/src/features/lineage/` | 待办 | — |
| E-31-05 | 测试与验收 | 依赖表（§2.1）逐行的集成测试；递归深度；大规模传播性能；清除条件；验收用例 TC-31-01～05（[TST-02](docs/test/02-需求追踪矩阵.md)）。 | `backend/tests/`、`frontend/tests/` | 待办 | — |

### M3 资产定稿、分镜与生成全链路（估算 4 周）

**里程碑目标**：一集完成到每镜已选关键帧或已配参考组合；≥ 3 个全能参考镜头；改造型只重做选中镜头；账本抽样一致

**Epic 实施顺序**：E-16 → E-22 → E-23 → E-32 → E-15 → E-17 → E-18 → E-19 → E-20 → E-26 → E-28（“依赖”为必须先完成的前置 Epic；“联调”为后实施的 Epic，本 Epic 先按契约与模拟实现推进）

**功能 Epic**

#### E-15 参考定稿与跨集复用

- **需求**：[REQ-15](docs/requirement/15-参考定稿与跨集复用.md)（BIB-03 参考图生成、上传与锁定；BIB-05 跨集复用；BIB-06 按集聚焦定稿）　**设计**：[DES-18](docs/design/18-参考定稿与跨集复用.md)　**依赖**：E-14、E-21、E-22、E-30、E-31　**联调**：E-17（后实施，先按契约与模拟实现联调，对方完成后重跑本 Epic 验收）
- **验收**：TC-15-01～04（4 条）
- **待确认**：DES-18-Q1、DES-18-Q2、DES-18-Q3（默认方案见设计文档，确认前按默认实施）

| 任务 | 内容 | 怎么做 | 涉及文件 | 状态 | 提交 |
| --- | --- | --- | --- | --- | --- |
| E-15-01 | 数据与领域模型 | 迁移建表 / 加列：`bible.episode_asset_usage`、`bible.reference_slot`、`bible.reference_version`、`lineage.dependency`、`operation.operation`（其中 `bible.episode_asset_usage` 由 E-14 建表，本 Epic 只加列或复用、`lineage.dependency` 由 E-13 建表，本 Epic 只加列或复用、`operation.operation` 由 E-21 建表，本 Epic 只加列或复用）；实现领域对象、状态机与仓储（`bible.episode_asset_usage`、`bible.episode_asset_usage`：无 `project_id`，经父对象外键继承项目范围校验；`bible.reference_slot`、`bible.reference_version`、`lineage.dependency`、`operation.operation`、`lineage.dependency`、`operation.operation`：仓储查询强制带 `project_id`）。详见 [DES-18 §2](docs/design/18-参考定稿与跨集复用.md#2-数据) | `backend/db/migrations/`、`backend/internal/bible/domain/`、`backend/internal/bible/adapter/postgres/` | 待办 | — |
| E-15-02 | 用例与接口 | 写操作经命令层（鉴权、幂等键、`expected_revision`、审计、Outbox）实现：GET /api/episodes/{eid}/asset-readiness、GET /api/reference-slots/{sid}、GET /api/reference-slots/{sid}/impact、POST /api/reference-slots/{sid}:lock、POST /api/projects/{pid}/quotes；swag 注解生成 OpenAPI。详见 [DES-18 §3](docs/design/18-参考定稿与跨集复用.md#3-接口) | `backend/internal/bible/application/`、`backend/internal/bible/adapter/http/`、`backend/docs/` | 待办 | — |
| E-15-03 | 异步、工作流与事件 | 工作流 / Activity / 定时任务 `OperationWorkflow`；事件 `bible.reference_locked.v1`（Outbox → Kafka，消费者按事件 ID 去重）。要点：候选生成使用通用 OperationWorkflow（DES-04）。锁定为同步命令；事务内写 `reference_version`、更新槽位、写 Outbox `bible.reference_locked.v1`。 详见 [DES-18 §4](docs/design/18-参考定稿与跨集复用.md#4-异步与工作流) | `backend/internal/bible/adapter/workflow/`、`backend/internal/bible/adapter/event/` | 待办 | — |
| E-15-04 | Agent 服务 | 生图适配器（参考图生成）；输入输出类型由 Go 与 Python 各自定义，契约测试（同一组示例输入输出）校验一致，离线评测纳入 `agent/evals/`。 | `agent/app/providers/` | 待办 | — |
| E-15-05 | 前端 | 资产定稿页：按角色 / 场景 / 道具分组的卡片，每卡显示槽位锁定状态；批量“为未锁定槽位生成”；槽位详情：候选网格（REQ-22 组件）、锁定按钮、版本历史、改锁影响确认框。 详见 [DES-18 §6](docs/design/18-参考定稿与跨集复用.md#6-界面) | `frontend/src/features/bible/` | 待办 | — |
| E-15-06 | 测试与验收 | 锁定并发冲突；改锁过期传播覆盖（参考组合、关键帧、视频）；本集清单计算；验收用例 TC-15-01～04（[TST-02](docs/test/02-需求追踪矩阵.md)）。 | `backend/tests/`、`agent/tests/`、`frontend/tests/` | 待办 | — |

#### E-16 角色音色

- **需求**：[REQ-16](docs/requirement/16-角色音色.md)（BIB-04 角色音色）　**设计**：[DES-19](docs/design/19-角色音色.md)　**依赖**：E-08、E-14、E-21　**联调**：E-29（后实施，先按契约与模拟实现联调，对方完成后重跑本 Epic 验收）
- **验收**：TC-16-01～03（3 条）
- **待确认**：DES-19-Q1、DES-19-Q2、DES-19-Q3（默认方案见设计文档，确认前按默认实施）

| 任务 | 内容 | 怎么做 | 涉及文件 | 状态 | 提交 |
| --- | --- | --- | --- | --- | --- |
| E-16-01 | 数据与领域模型 | 迁移建表 / 加列：`bible.voice_profile`、`lineage.dependency`、`operation.operation`（其中 `lineage.dependency` 由 E-13 建表，本 Epic 只加列或复用、`operation.operation` 由 E-21 建表，本 Epic 只加列或复用）；实现领域对象、状态机与仓储（仓储查询强制带 `project_id`）。详见 [DES-19 §2](docs/design/19-角色音色.md#2-数据) | `backend/db/migrations/`、`backend/internal/bible/domain/`、`backend/internal/bible/adapter/postgres/` | 待办 | — |
| E-16-02 | 用例与接口 | 写操作经命令层（鉴权、幂等键、`expected_revision`、审计、Outbox）实现：GET /api/models、GET /api/characters/{id}/voice、PUT /api/characters/{id}/voice、POST /api/projects/{pid}/quotes；swag 注解生成 OpenAPI。详见 [DES-19 §3](docs/design/19-角色音色.md#3-接口) | `backend/internal/bible/application/`、`backend/internal/bible/adapter/http/`、`backend/docs/` | 待办 | — |
| E-16-03 | 异步、工作流与事件 | 工作流 / Activity / 定时任务 `OperationWorkflow`；事件 `bible.voice_changed.v1`（Outbox → Kafka，消费者按事件 ID 去重）。要点：试听走 OperationWorkflow（同步型 TTS，秒级完成）。 详见 [DES-19 §4](docs/design/19-角色音色.md#4-异步与工作流) | `backend/internal/bible/adapter/workflow/`、`backend/internal/bible/adapter/event/` | 待办 | — |
| E-16-04 | Agent 服务 | TTS 适配器（音色枚举与试听）；输入输出类型由 Go 与 Python 各自定义，契约测试（同一组示例输入输出）校验一致，离线评测纳入 `agent/evals/`。 | `agent/app/providers/` | 待办 | — |
| E-16-05 | 前端 | 音色选择器（筛选标签、播放官方示例）；“用本角色台词试听”按钮（显示预估费用）；试听结果播放器。 详见 [DES-19 §6](docs/design/19-角色音色.md#6-界面) | `frontend/src/features/bible/` | 待办 | — |
| E-16-06 | 测试与验收 | 音色变更的过期范围；试听复用；验收用例 TC-16-01～03（[TST-02](docs/test/02-需求追踪矩阵.md)）。 | `backend/tests/`、`agent/tests/`、`frontend/tests/` | 待办 | — |

#### E-17 真人授权

- **需求**：[REQ-17](docs/requirement/17-真人授权.md)（BIB-07 写实人物授权；REQ-02 CMP-02）　**设计**：[DES-20](docs/design/20-真人授权.md)　**依赖**：E-15、E-30
- **验收**：TC-17-01～03（3 条）
- **待确认**：DES-20-Q1、DES-20-Q2、DES-20-Q3（默认方案见设计文档，确认前按默认实施）

| 任务 | 内容 | 怎么做 | 涉及文件 | 状态 | 提交 |
| --- | --- | --- | --- | --- | --- |
| E-17-01 | 数据与领域模型 | 迁移建表 / 加列：`media.consent_record`、`media.media_asset`（其中 `media.media_asset` 由 E-30 建表，本 Epic 只加列或复用）；实现领域对象、状态机与仓储（仓储查询强制带 `project_id`）。详见 [DES-20 §2](docs/design/20-真人授权.md#2-数据) | `backend/db/migrations/`、`backend/internal/media/domain/`、`backend/internal/media/adapter/postgres/` | 待办 | — |
| E-17-02 | 用例与接口 | 写操作经命令层（鉴权、幂等键、`expected_revision`、审计、Outbox）实现：POST /api/media/consents、POST /api/media/consents/{id}:revoke、POST /api/uploads/{media_asset_id}:complete、GET /api/media/consents/{id}；swag 注解生成 OpenAPI。详见 [DES-20 §3](docs/design/20-真人授权.md#3-接口) | `backend/internal/media/application/`、`backend/internal/media/adapter/http/`、`backend/docs/` | 待办 | — |
| E-17-03 | 异步、工作流与事件 | 事件 `media.consent_recorded.v1`、`media.consent_revoked.v1`（Outbox → Kafka，消费者按事件 ID 去重）。要点：无。 详见 [DES-20 §4](docs/design/20-真人授权.md#4-异步与工作流) | `backend/internal/media/adapter/workflow/`、`backend/internal/media/adapter/event/` | 待办 | — |
| E-17-04 | 前端 | 上传对话框中的“包含真实人物”开关与声明表单；媒体详情显示授权状态；参考选择器中未授权素材置灰并说明原因。 详见 [DES-20 §6](docs/design/20-真人授权.md#6-界面) | `frontend/src/features/bible/` | 待办 | — |
| E-17-05 | 测试与验收 | 引用校验在锁定、参考组合、报价三处均生效；验收用例 TC-17-01～03（[TST-02](docs/test/02-需求追踪矩阵.md)）。 | `backend/tests/`、`frontend/tests/` | 待办 | — |

#### E-18 分镜生成与镜头编辑

- **需求**：[REQ-18](docs/requirement/18-分镜生成与镜头编辑.md)（SB-01 AI 生成镜头表；SB-02 镜头语言参数编辑；SB-03 镜头增删改、排序、合并、拆分；SB-06 镜头语言模板）　**设计**：[DES-21](docs/design/21-分镜生成与镜头编辑.md)　**依赖**：E-13、E-14、E-15、E-21、E-31　**联调**：E-19（后实施，先按契约与模拟实现联调，对方完成后重跑本 Epic 验收）
- **验收**：TC-18-01～04（4 条）
- **待确认**：DES-21-Q1、DES-21-Q2、DES-21-Q3、DES-21-Q4（默认方案见设计文档，确认前按默认实施）

| 任务 | 内容 | 怎么做 | 涉及文件 | 状态 | 提交 |
| --- | --- | --- | --- | --- | --- |
| E-18-01 | 数据与领域模型 | 迁移建表 / 加列：`lineage.dependency`、`storyboard.reference_item`、`storyboard.scene_board`、`storyboard.shot`、`storyboard.shot_line`、`storyboard.shot_template`、`storyboard.shot_version`（其中 `lineage.dependency` 由 E-13 建表，本 Epic 只加列或复用、`storyboard.shot` 由 E-22 建表，本 Epic 只加列或复用）；实现领域对象、状态机与仓储（`lineage.dependency`、`storyboard.scene_board`、`storyboard.shot`、`storyboard.shot_version`、`lineage.dependency`、`storyboard.shot`：仓储查询强制带 `project_id`；`storyboard.reference_item`、`storyboard.shot_line`、`storyboard.shot_template`：无 `project_id`，经父对象外键继承项目范围校验）。详见 [DES-21 §3](docs/design/21-分镜生成与镜头编辑.md#3-数据) | `backend/db/migrations/`、`backend/internal/storyboard/domain/`、`backend/internal/storyboard/adapter/postgres/` | 待办 | — |
| E-18-02 | 用例与接口 | 写操作经命令层（鉴权、幂等键、`expected_revision`、审计、Outbox）实现：POST /api/projects/{pid}/quotes、GET /api/episodes/{eid}/shots、POST /api/shots/{id}/versions、POST /api/shots:bulk-update、POST /api/episodes/{eid}/shots、POST /api/shots/{id}:move、POST /api/shots:merge、POST /api/shots/{id}:split、DELETE /api/shots/{id}、GET /api/shots/{id}/versions、POST /api/scene-boards/{id}:confirm；swag 注解生成 OpenAPI。详见 [DES-21 §4](docs/design/21-分镜生成与镜头编辑.md#4-接口) | `backend/internal/storyboard/application/`、`backend/internal/storyboard/adapter/http/`、`backend/docs/` | 待办 | — |
| E-18-03 | 异步、工作流与事件 | 工作流 / Activity / 定时任务 `OperationWorkflow`（LLM 同步分支，`target_type = scene_storyboard`）、`BatchWorkflow`；事件 `storyboard.shot_version_created.v1`（Outbox → Kafka，消费者按事件 ID 去重）。要点：每场一个 `OperationWorkflow`（`target_type = scene_storyboard`）的 LLM 同步分支（DES-04 §3）：`llm.run_skill(storyboard)` →… 详见 [DES-21 §5](docs/design/21-分镜生成与镜头编辑.md#5-异步与工作流) | `backend/internal/storyboard/adapter/workflow/`、`backend/internal/storyboard/adapter/event/` | 待办 | — |
| E-18-04 | Agent 服务 | Skill `storyboard`（台词分配、引用、模式建议）；输入输出类型由 Go 与 Python 各自定义，契约测试（同一组示例输入输出）校验一致，离线评测纳入 `agent/evals/`。 | `agent/skills/storyboard/`、`agent/evals/` | 待办 | — |
| E-18-05 | 前端 | 镜头表（TanStack Table + Virtual）：按场分组、行内编辑（下拉、时长步进器）、多选工具栏（批量改参数、批量生成、删除）、拖拽排序（dnd-kit）；画面描述使用 Tiptap + Mention（@造型 / @场景 / @道具）；版本抽屉；场确认按钮与校验提示。 详见 [DES-21 §7](docs/design/21-分镜生成与镜头编辑.md#7-界面) | `frontend/src/features/storyboard/` | 待办 | — |
| E-18-06 | 测试与验收 | 台词分配校验；合并拆分对 `shot_line` 与引用的影响；批量部分成功；分镜 Skill 评测（TST-03）；验收用例 TC-18-01～04（[TST-02](docs/test/02-需求追踪矩阵.md)）。 | `backend/tests/`、`agent/tests/`、`frontend/tests/` | 待办 | — |
| E-18-07 | 镜头语言模板（SB-06，M5） | `storyboard.shot_template`；模板应用 = 批量修改生成新版本（DES-21-Q4） | `backend/internal/storyboard/`、`frontend/src/features/storyboard/` | 待办 | — |

#### E-19 生成模式与参考组合

- **需求**：[REQ-19](docs/requirement/19-生成模式与参考组合.md)（SB-04 生成模式与参考组合）　**设计**：[DES-22](docs/design/22-生成模式与参考组合.md)　**依赖**：E-08、E-15、E-18、E-30、E-17　**联调**：E-29（后实施，先按契约与模拟实现联调，对方完成后重跑本 Epic 验收）
- **验收**：TC-19-01～04（4 条）
- **待确认**：DES-22-Q1、DES-22-Q2（默认方案见设计文档，确认前按默认实施）

| 任务 | 内容 | 怎么做 | 涉及文件 | 状态 | 提交 |
| --- | --- | --- | --- | --- | --- |
| E-19-01 | 数据与领域模型 | 迁移建表 / 加列：`lineage.dependency`、`operation.operation_input`、`storyboard.reference_item`、`storyboard.shot_version`（其中 `lineage.dependency` 由 E-13 建表，本 Epic 只加列或复用、`operation.operation_input` 由 E-21 建表，本 Epic 只加列或复用、`storyboard.reference_item` 由 E-18 建表，本 Epic 只加列或复用、`storyboard.shot_version` 由 E-18 建表，本 Epic 只加列或复用）；实现领域对象、状态机与仓储（`lineage.dependency`、`storyboard.shot_version`、`lineage.dependency`、`storyboard.shot_version`：仓储查询强制带 `project_id`；`operation.operation_input`、`storyboard.reference_item`、`operation.operation_input`、`storyboard.reference_item`：无 `project_id`，经父对象外键继承项目范围校验）。详见 [DES-22 §2](docs/design/22-生成模式与参考组合.md#2-数据) | `backend/db/migrations/`、`backend/internal/storyboard/domain/`、`backend/internal/storyboard/adapter/postgres/` | 待办 | — |
| E-19-02 | 用例与接口 | 写操作经命令层（鉴权、幂等键、`expected_revision`、审计、Outbox）实现：POST /api/shots/{id}/versions、POST /api/shots/{id}:validate-references、GET /api/projects/{pid}/characters、GET /api/projects/{pid}/media、GET /api/episodes/{eid}/dialogue；swag 注解生成 OpenAPI。详见 [DES-22 §3](docs/design/22-生成模式与参考组合.md#3-接口) | `backend/internal/storyboard/application/`、`backend/internal/storyboard/adapter/http/`、`backend/docs/` | 待办 | — |
| E-19-03 | 前端 | 参考组合面板（REQ-05 §4.4）：顶部上限条；卡片列表（缩略图 / 波形、名称与版本、用途下拉、“有新版本”提示、移除）；添加菜单（设定集 / 媒体库 / 台词音频 / 上传）；拖拽排序。 详见 [DES-22 §6](docs/design/22-生成模式与参考组合.md#6-界面) | `frontend/src/features/storyboard/` | 待办 | — |
| E-19-04 | 测试与验收 | 校验器的所有规则组合（表驱动单元测试）；前后端校验结果一致性（共用测试向量）；验收用例 TC-19-01～04（[TST-02](docs/test/02-需求追踪矩阵.md)）。 | `backend/tests/`、`frontend/tests/` | 待办 | — |

#### E-20 故事板视图

- **需求**：[REQ-20](docs/requirement/20-故事板视图.md)（SB-05 故事板视图）　**设计**：[DES-23](docs/design/23-故事板视图.md)　**依赖**：E-18、E-22、E-23、E-31
- **验收**：TC-20-01～03（3 条）
- **待确认**：DES-23-Q1、DES-23-Q2（默认方案见设计文档，确认前按默认实施）

| 任务 | 内容 | 怎么做 | 涉及文件 | 状态 | 提交 |
| --- | --- | --- | --- | --- | --- |
| E-20-01 | 数据与领域模型 | 迁移建表 / 加列：`lineage.stale_mark`、`media.rendition`、`storyboard.shot`（其中 `lineage.stale_mark` 由 E-31 建表，本 Epic 只加列或复用、`media.rendition` 由 E-30 建表，本 Epic 只加列或复用、`storyboard.shot` 由 E-22 建表，本 Epic 只加列或复用）；实现领域对象、状态机与仓储（`lineage.stale_mark`、`storyboard.shot`、`lineage.stale_mark`、`storyboard.shot`：仓储查询强制带 `project_id`；`media.rendition`、`media.rendition`：无 `project_id`，经父对象外键继承项目范围校验）。详见 [DES-23 §2](docs/design/23-故事板视图.md#2-数据) | `backend/db/migrations/`、`backend/internal/storyboard/domain/`、`backend/internal/storyboard/adapter/postgres/` | 待办 | — |
| E-20-02 | 用例与接口 | 实现查询接口：GET /api/episodes/{eid}/shots；swag 注解生成 OpenAPI。详见 [DES-23 §3](docs/design/23-故事板视图.md#3-接口) | `backend/internal/storyboard/application/`、`backend/internal/storyboard/adapter/http/`、`backend/docs/` | 待办 | — |
| E-20-03 | 异步、工作流与事件 | 事件 `operation.status_changed.v1`（Outbox → Kafka，消费者按事件 ID 去重）。要点：无。 详见 [DES-23 §4](docs/design/23-故事板视图.md#4-异步与工作流) | `backend/internal/storyboard/adapter/workflow/`、`backend/internal/storyboard/adapter/event/` | 待办 | — |
| E-20-04 | 前端 | 网格（TanStack Virtual 行虚拟化）；按场分组标题；筛选栏；多选（Shift 连选）后底部操作栏：批量生成关键帧 / 视频、批量改参数（跳镜头表）。 详见 [DES-23 §6](docs/design/23-故事板视图.md#6-界面) | `frontend/src/features/storyboard/` | 待办 | — |
| E-20-05 | 测试与验收 | 进度状态机单元测试；前端性能录制（150 格）；验收用例 TC-20-01～03（[TST-02](docs/test/02-需求追踪矩阵.md)）。 | `backend/tests/`、`frontend/tests/` | 待办 | — |

#### E-22 多候选与选定

- **需求**：[REQ-22](docs/requirement/22-多候选与选定.md)（GEN-02 多候选与选定；VID-03 候选预览与选片）　**设计**：[DES-25](docs/design/25-多候选与选定.md)　**依赖**：E-21、E-31
- **验收**：TC-22-01～03（3 条）
- **待确认**：DES-25-Q1、DES-25-Q2（默认方案见设计文档，确认前按默认实施）

| 任务 | 内容 | 怎么做 | 涉及文件 | 状态 | 提交 |
| --- | --- | --- | --- | --- | --- |
| E-22-01 | 数据与领域模型 | 迁移建表 / 加列：`media.rendition`、`operation.operation_output`、`storyboard.shot`、`storyboard.shot_selection_log`（其中 `media.rendition` 由 E-30 建表，本 Epic 只加列或复用）；实现领域对象、状态机与仓储（`media.rendition`、`media.rendition`：无 `project_id`，经父对象外键继承项目范围校验；`operation.operation_output`、`storyboard.shot`、`storyboard.shot_selection_log`：仓储查询强制带 `project_id`）。详见 [DES-25 §2](docs/design/25-多候选与选定.md#2-数据) | `backend/db/migrations/`、`backend/internal/storyboard/domain/`、`backend/internal/storyboard/adapter/postgres/` | 待办 | — |
| E-22-02 | 用例与接口 | 写操作经命令层（鉴权、幂等键、`expected_revision`、审计、Outbox）实现：GET /api/shots/{id}/candidates、POST /api/shots/{id}:select-take；swag 注解生成 OpenAPI。详见 [DES-25 §3](docs/design/25-多候选与选定.md#3-接口) | `backend/internal/storyboard/application/`、`backend/internal/storyboard/adapter/http/`、`backend/docs/` | 待办 | — |
| E-22-03 | 异步、工作流与事件 | 事件 `storyboard.selection_changed.v1`（Outbox → Kafka，消费者按事件 ID 去重）。要点：无；选定为同步命令。 详见 [DES-25 §4](docs/design/25-多候选与选定.md#4-异步与工作流) | `backend/internal/storyboard/adapter/workflow/`、`backend/internal/storyboard/adapter/event/` | 待办 | — |
| E-22-04 | 前端 | `CandidateCompare` 组件：2–4 列网格；视频同步播放、逐帧（`← →`）、静音切换；数字键 1–9 聚焦，`S` 选定（REQ-05 §7）；“已选定”角标；改选确认框（影响数量）。 详见 [DES-25 §6](docs/design/25-多候选与选定.md#6-界面) | `frontend/src/features/review/` | 待办 | — |
| E-22-05 | 测试与验收 | 选定命令并发；改选下游过期；前端同步播放；验收用例 TC-22-01～03（[TST-02](docs/test/02-需求追踪矩阵.md)）。 | `backend/tests/`、`frontend/tests/` | 待办 | — |

#### E-23 批量生成

- **需求**：[REQ-23](docs/requirement/23-批量生成.md)（GEN-03 批量生成）　**设计**：[DES-26](docs/design/26-批量生成.md)　**依赖**：E-21、E-24
- **验收**：TC-23-01～03（3 条）
- **待确认**：DES-26-Q1、DES-26-Q2、DES-26-Q3（默认方案见设计文档，确认前按默认实施）

| 任务 | 内容 | 怎么做 | 涉及文件 | 状态 | 提交 |
| --- | --- | --- | --- | --- | --- |
| E-23-01 | 数据与领域模型 | 迁移建表 / 加列：`operation.batch`（其中 `operation.batch` 由 E-21 建表，本 Epic 只加列或复用）；实现领域对象、状态机与仓储（仓储查询强制带 `project_id`）。详见 [DES-26 §3](docs/design/26-批量生成.md#3-数据) | `backend/db/migrations/`、`backend/internal/operation/domain/`、`backend/internal/operation/adapter/postgres/` | 待办 | — |
| E-23-02 | 用例与接口 | 写操作经命令层（鉴权、幂等键、`expected_revision`、审计、Outbox）实现：POST /api/projects/{pid}/quotes、POST /api/batches/{id}:confirm、GET /api/batches/{id}、POST /api/batches/{id}:cancel、POST /api/batches/{id}:resume、POST /api/batches/{id}:retry-failed；swag 注解生成 OpenAPI。详见 [DES-26 §4](docs/design/26-批量生成.md#4-接口) | `backend/internal/operation/application/`、`backend/internal/operation/adapter/http/`、`backend/docs/` | 待办 | — |
| E-23-03 | 异步、工作流与事件 | 工作流 / Activity / 定时任务 `BatchWorkflow`、`OperationWorkflow`；事件 `batch.finished.v1`（Outbox → Kafka，消费者按事件 ID 去重）。要点：`BatchWorkflow`（`batch/{id}`）：子工作流 `operation/{op_id}`，`ParentClosePolicy = ABANDON`（父工作流异常不影响已提交的子项）；使用信号 `ca… 详见 [DES-26 §5](docs/design/26-批量生成.md#5-异步与工作流) | `backend/internal/operation/adapter/workflow/`、`backend/internal/operation/adapter/event/` | 待办 | — |
| E-23-04 | 前端 | 批量工具栏；报价对话框（逐项列表可剔除）；批次进度条（在任务中心与发起页面顶部）；完成后结果摘要与“重试失败项”。 详见 [DES-26 §7](docs/design/26-批量生成.md#7-界面) | `frontend/src/features/operation/` | 待办 | — |
| E-23-05 | 测试与验收 | 父子工作流回放测试；熔断暂停；取消与预留释放；验收用例 TC-23-01～03（[TST-02](docs/test/02-需求追踪矩阵.md)）。 | `backend/tests/`、`frontend/tests/` | 待办 | — |

#### E-26 关键帧

- **需求**：[REQ-26](docs/requirement/26-关键帧.md)（IMG-01 生成关键帧；IMG-02 上传关键帧）　**设计**：[DES-29](docs/design/29-关键帧.md)　**依赖**：E-15、E-18、E-21、E-22、E-30
- **验收**：TC-26-01～03（3 条）
- **待确认**：DES-29-Q1、DES-29-Q2（默认方案见设计文档，确认前按默认实施）

| 任务 | 内容 | 怎么做 | 涉及文件 | 状态 | 提交 |
| --- | --- | --- | --- | --- | --- |
| E-26-01 | 数据与领域模型 | 迁移建表 / 加列：`operation.operation`（其中 `operation.operation` 由 E-21 建表，本 Epic 只加列或复用）；实现领域对象、状态机与仓储（仓储查询强制带 `project_id`）。详见 [DES-29 §2](docs/design/29-关键帧.md#2-数据) | `backend/db/migrations/`、`backend/internal/storyboard/domain/`、`backend/internal/storyboard/adapter/postgres/` | 待办 | — |
| E-26-02 | 用例与接口 | 写操作经命令层（鉴权、幂等键、`expected_revision`、审计、Outbox）实现：POST /api/projects/{pid}/quotes、POST /api/shots/{id}/frames:upload、POST /api/shots/{id}:select-frame；swag 注解生成 OpenAPI。详见 [DES-29 §3](docs/design/29-关键帧.md#3-接口) | `backend/internal/storyboard/application/`、`backend/internal/storyboard/adapter/http/`、`backend/docs/` | 待办 | — |
| E-26-03 | 异步、工作流与事件 | 工作流 / Activity / 定时任务 `OperationWorkflow`。要点：生成：OperationWorkflow（图片同步或异步视供应商）；上传裁剪：`media` 队列 `CropImage` Activity。 详见 [DES-29 §4](docs/design/29-关键帧.md#4-异步与工作流) | `backend/internal/storyboard/adapter/workflow/`、`backend/internal/storyboard/adapter/event/` | 待办 | — |
| E-26-04 | Agent 服务 | 生图适配器（关键帧，含参考输入）；输入输出类型由 Go 与 Python 各自定义，契约测试（同一组示例输入输出）校验一致，离线评测纳入 `agent/evals/`。 | `agent/app/providers/` | 待办 | — |
| E-26-05 | 前端 | 镜头详情“关键帧”标签页：生成按钮（数量、模型、参数）、上传区（裁剪框）、候选对比、选定。 详见 [DES-29 §6](docs/design/29-关键帧.md#6-界面) | `frontend/src/features/storyboard/` | 待办 | — |
| E-26-06 | 测试与验收 | 输入展开正确性；上传裁剪；审核拒绝路径；验收用例 TC-26-01～03（[TST-02](docs/test/02-需求追踪矩阵.md)）。 | `backend/tests/`、`agent/tests/`、`frontend/tests/` | 待办 | — |

#### E-28 全能参考生视频

- **需求**：[REQ-28](docs/requirement/28-全能参考生视频.md)（VID-04 全能参考生视频（必须，不可降级））　**设计**：[DES-31](docs/design/31-全能参考生视频.md)　**依赖**：E-08、E-15、E-19、E-21、E-22　**联调**：E-29（后实施，先按契约与模拟实现联调，对方完成后重跑本 Epic 验收）
- **验收**：TC-28-01～05（5 条）
- **待确认**：DES-31-Q1、DES-31-Q2、DES-31-Q3（默认方案见设计文档，确认前按默认实施）

| 任务 | 内容 | 怎么做 | 涉及文件 | 状态 | 提交 |
| --- | --- | --- | --- | --- | --- |
| E-28-01 | 数据与领域模型 | 迁移建表 / 加列：`lineage.dependency`、`operation.operation`、`operation.operation_input`、`storyboard.reference_item`（其中 `lineage.dependency` 由 E-13 建表，本 Epic 只加列或复用、`operation.operation` 由 E-21 建表，本 Epic 只加列或复用、`operation.operation_input` 由 E-21 建表，本 Epic 只加列或复用、`storyboard.reference_item` 由 E-18 建表，本 Epic 只加列或复用）；实现领域对象、状态机与仓储（`lineage.dependency`、`operation.operation`、`lineage.dependency`、`operation.operation`：仓储查询强制带 `project_id`；`operation.operation_input`、`storyboard.reference_item`、`operation.operation_input`、`storyboard.reference_item`：无 `project_id`，经父对象外键继承项目范围校验）。详见 [DES-31 §2](docs/design/31-全能参考生视频.md#2-数据) | `backend/db/migrations/`、`backend/internal/operation/domain/`、`backend/internal/operation/adapter/postgres/` | 待办 | — |
| E-28-02 | 用例与接口 | 写操作经命令层（鉴权、幂等键、`expected_revision`、审计、Outbox）实现：POST /api/projects/{pid}/quotes；swag 注解生成 OpenAPI。详见 [DES-31 §3](docs/design/31-全能参考生视频.md#3-接口) | `backend/internal/operation/application/`、`backend/internal/operation/adapter/http/`、`backend/docs/` | 待办 | — |
| E-28-03 | Agent 服务 | 视频适配器 `omni_reference` 用途映射与提示词指代；输入输出类型由 Go 与 Python 各自定义，契约测试（同一组示例输入输出）校验一致，离线评测纳入 `agent/evals/`。 | `agent/app/providers/` | 待办 | — |
| E-28-04 | 前端 | 镜头详情：参考组合（REQ-19）+ 提示词预览（带指代编号高亮）+ 生成；候选卡片可展开“所用参考”列表。 详见 [DES-31 §6](docs/design/31-全能参考生视频.md#6-界面) | `frontend/src/features/storyboard/` | 待办 | — |
| E-28-05 | 测试与验收 | 适配器映射单元测试（每个供应商的参数形态）；提示词指代与输入顺序一致性。 人工评测：TST-03 全能参考评测集（写实 + 4 种风格化 × 单角色 / 双角色 / 动作 / 运镜 / 音频）；验收用例 TC-28-01～05（[TST-02](docs/test/02-需求追踪矩阵.md)）。 | `backend/tests/`、`agent/tests/`、`frontend/tests/` | 待办 | — |

#### E-32 成本账本与报表

- **需求**：[REQ-32](docs/requirement/32-成本账本与报表.md)（CST-01 调用记账；CST-02 成本汇总；CST-06 成本报表与预算告警）　**设计**：[DES-35](docs/design/35-成本账本与报表.md)　**依赖**：E-11、E-21、E-24
- **验收**：TC-32-01～04（4 条）
- **待确认**：DES-35-Q1、DES-35-Q2、DES-35-Q3（默认方案见设计文档，确认前按默认实施）

| 任务 | 内容 | 怎么做 | 涉及文件 | 状态 | 提交 |
| --- | --- | --- | --- | --- | --- |
| E-32-01 | 数据与领域模型 | 迁移建表 / 加列：`billing.cost_summary`、`billing.ledger_entry`、`billing.reconciliation_run`（其中 `billing.ledger_entry` 由 E-11 建表，本 Epic 只加列或复用）；实现领域对象、状态机与仓储（`billing.cost_summary`、`billing.ledger_entry`、`billing.ledger_entry`：仓储查询强制带 `project_id`；`billing.reconciliation_run`：组织 / 平台级，按管理员权限访问）。详见 [DES-35 §2](docs/design/35-成本账本与报表.md#2-数据) | `backend/db/migrations/`、`backend/internal/billing/domain/`、`backend/internal/billing/adapter/postgres/` | 待办 | — |
| E-32-02 | 用例与接口 | 写操作经命令层（鉴权、幂等键、`expected_revision`、审计、Outbox）实现：GET /api/projects/{pid}/costs、GET /api/projects/{pid}/ledger、POST /api/admin/ledger:adjust、GET /api/admin/costs、GET /api/projects/{pid}/metrics；swag 注解生成 OpenAPI。详见 [DES-35 §3](docs/design/35-成本账本与报表.md#3-接口) | `backend/internal/billing/application/`、`backend/internal/billing/adapter/http/`、`backend/docs/` | 待办 | — |
| E-32-03 | 异步、工作流与事件 | 工作流 / Activity / 定时任务 `ledger-reconcile`；消费者 `billing-report`（消费 `billing.settled.v1`、`billing.ledger_adjusted.v1`，维护 `billing.cost_summary`）；事件 发布 `billing.reconciliation_diff.v1`；消费 `billing.settled.v1`、`billing.ledger_adjusted.v1`（Outbox → Kafka，消费者按事件 ID 去重）。要点：`billing-report` 消费者；`ledger-reconcile`（每日 02:00）。 详见 [DES-35 §4](docs/design/35-成本账本与报表.md#4-异步与工作流) | `backend/internal/billing/adapter/workflow/`、`backend/internal/billing/adapter/event/` | 待办 | — |
| E-32-04 | 前端 | 成本页：项目总览卡（已结算、预留、预算）、按集表格（费用、选定片段总时长、单分钟成本、按能力拆分）、镜头明细下钻、账本明细。 详见 [DES-35 §6](docs/design/35-成本账本与报表.md#6-界面) | `frontend/src/features/billing/` | 待办 | — |
| E-32-05 | 测试与验收 | 汇总与全量重建一致性；对账差异检测；生产指标按固定数据集的计算结果（REQ-32 验收）；验收用例 TC-32-01～04（[TST-02](docs/test/02-需求追踪矩阵.md)）。 | `backend/tests/`、`frontend/tests/` | 待办 | — |
| E-32-06 | 成本报表（CST-06，M5） | `GET /api/admin/costs` 跨项目报表界面与 CSV 导出（复用 E-32-02 的查询接口）；报表中呈现各项目预算告警状态。低余额与超支的判断、事件与站内通知已在 M1 由 E-11、E-33 交付 | `backend/internal/billing/`、`frontend/src/features/billing/` | 待办 | — |

### M4 视频、配音与生成增强（估算 3–4 周）

**里程碑目标**：PRD-01 §8.2 场景 1–7 通过；REQ-39 验收通过

**Epic 实施顺序**：E-27 → E-29 → E-39（“依赖”为必须先完成的前置 Epic；“联调”为后实施的 Epic，本 Epic 先按契约与模拟实现推进）

**功能 Epic**

#### E-27 图生视频

- **需求**：[REQ-27](docs/requirement/27-图生视频.md)（VID-01 图生视频；VID-02 视频时长）　**设计**：[DES-30](docs/design/30-图生视频.md)　**依赖**：E-21、E-22、E-26
- **验收**：TC-27-01～03（3 条）
- **待确认**：DES-30-Q1、DES-30-Q2、DES-30-Q3（默认方案见设计文档，确认前按默认实施）

| 任务 | 内容 | 怎么做 | 涉及文件 | 状态 | 提交 |
| --- | --- | --- | --- | --- | --- |
| E-27-01 | 数据与领域模型 | 迁移建表 / 加列：`lineage.dependency`、`operation.operation`（其中 `lineage.dependency` 由 E-13 建表，本 Epic 只加列或复用、`operation.operation` 由 E-21 建表，本 Epic 只加列或复用）；实现领域对象、状态机与仓储（仓储查询强制带 `project_id`）。详见 [DES-30 §2](docs/design/30-图生视频.md#2-数据) | `backend/db/migrations/`、`backend/internal/operation/domain/`、`backend/internal/operation/adapter/postgres/` | 待办 | — |
| E-27-02 | 用例与接口 | 写操作经命令层（鉴权、幂等键、`expected_revision`、审计、Outbox）实现：POST /api/projects/{pid}/quotes、GET /api/shots/{id}/prompt-preview；swag 注解生成 OpenAPI。详见 [DES-30 §3](docs/design/30-图生视频.md#3-接口) | `backend/internal/operation/application/`、`backend/internal/operation/adapter/http/`、`backend/docs/` | 待办 | — |
| E-27-03 | 异步、工作流与事件 | 工作流 / Activity / 定时任务 `OperationWorkflow`。要点：OperationWorkflow（异步视频）：提交 → 轮询（退避 5 s → 30 s）→ 接管（下载、ffprobe、转码、缩略图、代理）→ 审核 → 完成。提示词由 Go 规则模板拼接（免费）；AI 优化提示词不… 详见 [DES-30 §4](docs/design/30-图生视频.md#4-异步与工作流) | `backend/internal/operation/adapter/workflow/`、`backend/internal/operation/adapter/event/` | 待办 | — |
| E-27-04 | Agent 服务 | 视频适配器 `image2video` 模式（提示词由 Go 规则模板生成，不需 Skill）；输入输出类型由 Go 与 Python 各自定义，契约测试（同一组示例输入输出）校验一致，离线评测纳入 `agent/evals/`。 | `agent/app/providers/` | 待办 | — |
| E-27-05 | 前端 | 镜头详情“视频”标签页：前置状态提示、提示词预览与改写、时长提示、生成、候选对比。 详见 [DES-30 §6](docs/design/30-图生视频.md#6-界面) | `frontend/src/features/storyboard/` | 待办 | — |
| E-27-06 | 测试与验收 | 时长交集计算；提示词组装快照测试；接管流程（转码、代理）；验收用例 TC-27-01～03（[TST-02](docs/test/02-需求追踪矩阵.md)）。 | `backend/tests/`、`agent/tests/`、`frontend/tests/` | 待办 | — |

#### E-29 台词配音

- **需求**：[REQ-29](docs/requirement/29-台词配音.md)（AUD-01 台词配音）　**设计**：[DES-32](docs/design/32-台词配音.md)　**依赖**：E-13、E-16、E-21、E-22、E-30
- **验收**：TC-29-01～02（2 条）
- **待确认**：DES-32-Q1、DES-32-Q2、DES-32-Q3（默认方案见设计文档，确认前按默认实施）

| 任务 | 内容 | 怎么做 | 涉及文件 | 状态 | 提交 |
| --- | --- | --- | --- | --- | --- |
| E-29-01 | 数据与领域模型 | 迁移建表 / 加列：`audio.dialogue_audio`、`audio.line_audio_selection`、`audio.line_tts_override`、`operation.operation`（其中 `operation.operation` 由 E-21 建表，本 Epic 只加列或复用）；实现领域对象、状态机与仓储（仓储查询强制带 `project_id`）。详见 [DES-32 §2](docs/design/32-台词配音.md#2-数据) | `backend/db/migrations/`、`backend/internal/audio/domain/`、`backend/internal/audio/adapter/postgres/` | 待办 | — |
| E-29-02 | 用例与接口 | 写操作经命令层（鉴权、幂等键、`expected_revision`、审计、Outbox）实现：GET /api/episodes/{eid}/dialogue、PUT /api/episodes/{eid}/dialogue/{line_key}/override、POST /api/projects/{pid}/quotes、POST /api/episodes/{eid}/dialogue/{line_key}:select-audio；swag 注解生成 OpenAPI。详见 [DES-32 §3](docs/design/32-台词配音.md#3-接口) | `backend/internal/audio/application/`、`backend/internal/audio/adapter/http/`、`backend/docs/` | 待办 | — |
| E-29-03 | 异步、工作流与事件 | 工作流 / Activity / 定时任务 `OperationWorkflow`；事件 `audio.selection_changed.v1`（Outbox → Kafka，消费者按事件 ID 去重）。要点：TTS：`OperationWorkflow`（同步型）。 详见 [DES-32 §4](docs/design/32-台词配音.md#4-异步与工作流) | `backend/internal/audio/adapter/workflow/`、`backend/internal/audio/adapter/event/` | 待办 | — |
| E-29-04 | Agent 服务 | TTS 适配器（逐句参数、发音修正）；输入输出类型由 Go 与 Python 各自定义，契约测试（同一组示例输入输出）校验一致，离线评测纳入 `agent/evals/`。 | `agent/app/providers/` | 待办 | — |
| E-29-05 | 前端 | 按场分组的台词表：说话人、文本、音色、语速 / 情绪、候选播放、选定；批量工具栏（批量报价 TTS、批量选定最新候选）。 详见 [DES-32 §6](docs/design/32-台词配音.md#6-界面) | `frontend/src/features/audio/` | 待办 | — |
| E-29-06 | 测试与验收 | 过期规则（文本、音色）；逐句覆盖参数进入 `input_hash`；组合键 `target_key` 解析；验收用例 TC-29-01～02（[TST-02](docs/test/02-需求追踪矩阵.md)）。 | `backend/tests/`、`agent/tests/`、`frontend/tests/` | 待办 | — |

#### E-39 生成增强：首尾帧、片段续接、改图、模型切换

- **需求**：[REQ-39](docs/requirement/39-生成增强.md)（VID-05 片段作为后续镜头参考；VID-06 首尾帧控制；IMG-03 局部重绘与改图；GEN-08 切换供应商与模型）　**设计**：[DES-41](docs/design/41-生成增强.md)　**依赖**：E-08、E-19、E-22、E-26、E-27、E-28
- **验收**：TC-39-01～04（4 条）
- **待确认**：DES-41-Q1、DES-41-Q2、DES-41-Q3（默认方案见设计文档，确认前按默认实施）

| 任务 | 内容 | 怎么做 | 涉及文件 | 状态 | 提交 |
| --- | --- | --- | --- | --- | --- |
| E-39-01 | 数据与领域模型 | 迁移建表 / 加列：`storyboard.reference_item`、`storyboard.shot_version`、`operation.operation_input`（其中 `storyboard.reference_item` 由 E-18 建表，本 Epic 只加列或复用、`storyboard.shot_version` 由 E-18 建表，本 Epic 只加列或复用、`operation.operation_input` 由 E-21 建表，本 Epic 只加列或复用）；实现领域对象、状态机与仓储（`storyboard.reference_item`、`operation.operation_input`、`storyboard.reference_item`、`operation.operation_input`：无 `project_id`，经父对象外键继承项目范围校验；`storyboard.shot_version`、`storyboard.shot_version`：仓储查询强制带 `project_id`）。详见 [DES-41 §2](docs/design/41-生成增强.md#2-数据) | `backend/db/migrations/`、`backend/internal/storyboard/domain/`、`backend/internal/storyboard/adapter/postgres/` | 待办 | — |
| E-39-02 | 用例与接口 | 写操作经命令层（鉴权、幂等键、`expected_revision`、审计、Outbox）实现：POST /api/shots/{id}/frames:extract-from-take、GET /api/shots/{id}/candidates、POST /api/projects/{pid}/quotes、POST /api/operations/{id}:confirm；swag 注解生成 OpenAPI。详见 [DES-41 §3](docs/design/41-生成增强.md#3-接口) | `backend/internal/storyboard/application/`、`backend/internal/storyboard/adapter/http/`、`backend/docs/` | 待办 | — |
| E-39-03 | 异步、工作流与事件 | 工作流 / Activity / 定时任务 `media.ExtractFrame`、`OperationWorkflow`。要点：抽帧 `media.ExtractFrame`；其余复用 OperationWorkflow。 详见 [DES-41 §4](docs/design/41-生成增强.md#4-异步与工作流) | `backend/internal/storyboard/adapter/workflow/`、`backend/internal/storyboard/adapter/event/` | 待办 | — |
| E-39-04 | Agent 服务 | 适配器支持 `frames2video`、`image.edit`（蒙版）；输入输出类型由 Go 与 Python 各自定义，契约测试（同一组示例输入输出）校验一致，离线评测纳入 `agent/evals/`。 | `agent/app/providers/` | 待办 | — |
| E-39-05 | 前端 | 参考选择器新增“前一镜片段”；首尾帧槽位；关键帧编辑器（画笔蒙版 + 描述）；候选对比按模型筛选。 详见 [DES-41 §5](docs/design/41-生成增强.md#5-界面) | `frontend/src/features/storyboard/` | 待办 | — |
| E-39-06 | 测试与验收 | 抽帧精度；新模式在注册表与适配器中的映射；过期传播扩展；验收用例 TC-39-01～04（[TST-02](docs/test/02-需求追踪矩阵.md)）。 | `backend/tests/`、`agent/tests/`、`frontend/tests/` | 待办 | — |

### M5 画布、对话式 Agent 与运营辅助（估算 5–6 周）= MVP

**里程碑目标**：PRD-01 §8.2 的 9 个 MVP 验收场景全部通过

**Epic 实施顺序**：E-35 → E-36 → E-37 → E-38（“依赖”为必须先完成的前置 Epic；“联调”为后实施的 Epic，本 Epic 先按契约与模拟实现推进）

**功能 Epic**

#### E-35 系统健康视图

- **需求**：[REQ-35](docs/requirement/35-系统健康视图.md)（ADM-03 系统健康视图）　**设计**：[DES-37](docs/design/37-系统健康视图.md)　**依赖**：E-24
- **验收**：TC-35-01～02（2 条）
- **待确认**：DES-37-Q1、DES-37-Q2（默认方案见设计文档，确认前按默认实施）

| 任务 | 内容 | 怎么做 | 涉及文件 | 状态 | 提交 |
| --- | --- | --- | --- | --- | --- |
| E-35-01 | 用例与接口 | 实现查询接口：GET /api/admin/health；swag 注解生成 OpenAPI。详见 [DES-37 §3](docs/design/37-系统健康视图.md#3-接口) | `backend/internal/operation/application/`、`backend/internal/operation/adapter/http/`、`backend/docs/` | 待办 | — |
| E-35-02 | 前端 | 卡片 + 表格，异常红色高亮，外链到 Grafana、Temporal UI。 详见 [DES-37 §4](docs/design/37-系统健康视图.md#4-界面) | `frontend/src/features/admin/` | 待办 | — |
| E-35-03 | 测试与验收 | 阈值判断单元测试；验收用例 TC-35-01～02（[TST-02](docs/test/02-需求追踪矩阵.md)）。 | `backend/tests/`、`frontend/tests/` | 待办 | — |

#### E-36 画布

- **需求**：[REQ-36](docs/requirement/36-画布.md)（CNV-01 项目画布；CNV-02 画布发起生成；CNV-03 画布参考连线；CNV-04 布局保存；CNV-05 撤销重做、复制粘贴、快捷键；CNV-06 画布性能；CNV-08 风格与镜头语言素材节点）　**设计**：[DES-38](docs/design/38-画布功能.md)　**依赖**：E-18、E-19、E-21、E-22、E-30
- **验收**：TC-36-01～05（5 条）
- **待确认**：DES-38-Q1、DES-38-Q2、DES-38-Q3（默认方案见设计文档，确认前按默认实施）

| 任务 | 内容 | 怎么做 | 涉及文件 | 状态 | 提交 |
| --- | --- | --- | --- | --- | --- |
| E-36-01 | 数据与领域模型 | 迁移建表 / 加列：`canvas.command_log`、`canvas.document`、`canvas.document_snapshot`、`canvas.edge`、`canvas.node`；实现领域对象、状态机与仓储（`canvas.command_log`、`canvas.document_snapshot`、`canvas.edge`、`canvas.node`：无 `project_id`，经父对象外键继承项目范围校验；`canvas.document`：仓储查询强制带 `project_id`）。详见 [DES-38 §2](docs/design/38-画布功能.md#2-数据) | `backend/db/migrations/`、`backend/internal/canvas/domain/`、`backend/internal/canvas/adapter/postgres/` | 待办 | — |
| E-36-02 | 用例与接口 | 写操作经命令层（鉴权、幂等键、`expected_revision`、审计、Outbox）实现：GET /api/projects/{pid}/canvases、POST /api/projects/{pid}/canvases、GET /api/canvases/{id}、GET /api/canvases/{id}/changes、POST /api/canvases/{id}/commands、POST /api/canvases/{id}/nodes/{nid}:run、POST /api/canvases/{id}/snapshots；swag 注解生成 OpenAPI。详见 [DES-38 §3](docs/design/38-画布功能.md#3-接口) | `backend/internal/canvas/application/`、`backend/internal/canvas/adapter/http/`、`backend/docs/` | 待办 | — |
| E-36-03 | 异步、工作流与事件 | 工作流 / Activity / 定时任务 `OperationWorkflow`；事件 `canvas.document_changed.v1`（Outbox → Kafka，消费者按事件 ID 去重）。要点：生成节点运行复用 OperationWorkflow；无新工作流。 详见 [DES-38 §4](docs/design/38-画布功能.md#4-异步与工作流) | `backend/internal/canvas/adapter/workflow/`、`backend/internal/canvas/adapter/event/` | 待办 | — |
| E-36-04 | Agent 服务 | AI 画布助手（经对话式 Agent 下发画布命令提案）；输入输出类型由 Go 与 Python 各自定义，契约测试（同一组示例输入输出）校验一致，离线评测纳入 `agent/evals/`。 | `agent/app/harness/` | 待办 | — |
| E-36-05 | 前端 | React Flow 引擎 + 移植 infinite-canvas 卡片与交互（DES-08 §7）；左侧节点库、右侧属性面板、顶部工具栏、小地图；快捷键见 REQ-05 §7。 详见 [DES-38 §6](docs/design/38-画布功能.md#6-界面) | `frontend/src/features/canvas/` | 待办 | — |
| E-36-06 | 测试与验收 | 命令冲突重放；reference 连线与参考组合双向一致；Playwright 性能录制；验收用例 TC-36-01～05（[TST-02](docs/test/02-需求追踪矩阵.md)）。 | `backend/tests/`、`agent/tests/`、`frontend/tests/` | 待办 | — |

#### E-37 对话式 Agent

- **需求**：[REQ-37](docs/requirement/37-对话式Agent.md)（AGT-01 对话式 Agent；AGT-02 会话管理；AGT-03 Agent 执行可见性；CNV-07 AI 画布助手）　**设计**：[DES-39](docs/design/39-对话式Agent.md)　**依赖**：E-18、E-19、E-21、E-36
- **验收**：TC-37-01～04（4 条）
- **待确认**：DES-39-Q1、DES-39-Q2、DES-39-Q3、DES-39-Q4（默认方案见设计文档，确认前按默认实施）

| 任务 | 内容 | 怎么做 | 涉及文件 | 状态 | 提交 |
| --- | --- | --- | --- | --- | --- |
| E-37-01 | 数据与领域模型 | 迁移建表 / 加列：`agent.message`、`agent.proposal`、`agent.session`、`agent.run`、`operation.operation`（额度）、`operation.provider_call`（其中 `operation.operation` 由 E-21 建表，本 Epic 只加列或复用、`operation.provider_call` 由 E-24 建表，本 Epic 只加列或复用）；实现领域对象、状态机与仓储（`agent.message`：无 `project_id`，经父对象外键继承项目范围校验；`agent.proposal`、`agent.session`、`agent.run`、`operation.operation`、`operation.provider_call`、`operation.operation`、`operation.provider_call`：仓储查询强制带 `project_id`）。详见 [DES-39 §2](docs/design/39-对话式Agent.md#2-数据) | `backend/db/migrations/`、`backend/internal/agent/domain/`、`backend/internal/agent/adapter/postgres/` | 待办 | — |
| E-37-02 | 用例与接口 | 写操作经命令层（鉴权、幂等键、`expected_revision`、审计、Outbox）实现：POST /api/projects/{pid}/agent/sessions、GET /api/projects/{pid}/agent/sessions、POST /api/projects/{pid}/agent/sessions/{sid}/runs、GET /api/projects/{pid}/agent/sessions/{sid}/messages、POST /api/agent/proposals/{id}:apply、POST /api/agent/proposals/{id}:reject、DELETE /api/projects/{pid}/agent/sessions/{sid}、GET /api/admin/agent/runs/{run_id}/debug、POST /api/projects/{pid}/agent/sessions/{sid}/budget-quotes、POST /api/projects/{pid}/agent/sessions/{sid}:close、POST /internal/agent/runs/{run_id}/calls、PUT /internal/agent/runs/{run_id}/calls/{call_seq}；swag 注解生成 OpenAPI。详见 [DES-39 §3](docs/design/39-对话式Agent.md#3-接口) | `backend/internal/agent/application/`、`backend/internal/agent/adapter/http/`、`backend/docs/` | 待办 | — |
| E-37-03 | 异步、工作流与事件 | 工作流 / Activity / 定时任务 `OperationWorkflow`、`agent-session-settle`。要点：对话运行不使用 Temporal（交互式、短时），`agent-api` 直接流式执行。每次运行前 Go 在当前额度内登记一条 `agent.run` 并占用 `run_cap`，作为 Harness Budget 传入… 详见 [DES-39 §4](docs/design/39-对话式Agent.md#4-异步与工作流) | `backend/internal/agent/adapter/workflow/`、`backend/internal/agent/adapter/event/` | 待办 | — |
| E-37-04 | Agent 服务 | agent-api 对话运行、只读工具、提案与生成草稿（AG-UI）；输入输出类型由 Go 与 Python 各自定义，契约测试（同一组示例输入输出）校验一致，离线评测纳入 `agent/evals/`。 | `agent/app/api/`、`agent/app/harness/tools.py`、`agent/evals/` | 待办 | — |
| E-37-05 | 前端 | CopilotKit 面板：流式消息（Streamdown）、步骤与工具调用折叠卡、提案差异卡（应用 / 拒绝）、生成草稿卡（报价确认组件）；会话列表；管理员调试抽屉（上下文、事件流）。 详见 [DES-39 §6](docs/design/39-对话式Agent.md#6-界面) | `frontend/src/features/agent/` | 待办 | — |
| E-37-06 | 测试与验收 | 提案失效与重放；工具范围越权测试；提示注入评测集（TST-03）；验收用例 TC-37-01～04（[TST-02](docs/test/02-需求追踪矩阵.md)）。 | `backend/tests/`、`agent/tests/`、`frontend/tests/` | 待办 | — |

#### E-38 剧本修订对比

- **需求**：[REQ-38](docs/requirement/38-剧本修订对比.md)（SCR-06 剧本修订对比）　**设计**：[DES-40](docs/design/40-剧本修订对比.md)　**依赖**：E-12、E-13、E-31
- **验收**：TC-38-01～02（2 条）
- **待确认**：DES-40-Q1、DES-40-Q2、DES-40-Q3（默认方案见设计文档，确认前按默认实施）

| 任务 | 内容 | 怎么做 | 涉及文件 | 状态 | 提交 |
| --- | --- | --- | --- | --- | --- |
| E-38-01 | 数据与领域模型 | 迁移建表 / 加列：`script.episode`、`script.script_version`（其中 `script.episode` 由 E-12 建表，本 Epic 只加列或复用、`script.script_version` 由 E-12 建表，本 Epic 只加列或复用）；实现领域对象、状态机与仓储（仓储查询强制带 `project_id`）。详见 [DES-40 §2](docs/design/40-剧本修订对比.md#2-数据) | `backend/db/migrations/`、`backend/internal/script/domain/`、`backend/internal/script/adapter/postgres/` | 待办 | — |
| E-38-02 | 用例与接口 | 写操作经命令层（鉴权、幂等键、`expected_revision`、审计、Outbox）实现：POST /api/projects/{pid}/script-imports、GET /api/projects/{pid}/script-versions/{a}/diff/{b}、POST /api/projects/{pid}/script-versions/{id}:adopt；swag 注解生成 OpenAPI。详见 [DES-40 §3](docs/design/40-剧本修订对比.md#3-接口) | `backend/internal/script/application/`、`backend/internal/script/adapter/http/`、`backend/docs/` | 待办 | — |
| E-38-03 | 异步、工作流与事件 | 工作流 / Activity / 定时任务 `ScriptRevisionWorkflow`。要点：`ScriptRevisionWorkflow`（对齐 → 重解析变化集 → 台词匹配 → 待确认）。 详见 [DES-40 §4](docs/design/40-剧本修订对比.md#4-异步与工作流) | `backend/internal/script/adapter/workflow/`、`backend/internal/script/adapter/event/` | 待办 | — |
| E-38-04 | 前端 | 版本对比页（集列表带变化标记、并排文本差异、受影响镜头数）。 详见 [DES-40 §5](docs/design/40-剧本修订对比.md#5-界面) | `frontend/src/features/script/` | 待办 | — |
| E-38-05 | 测试与验收 | 台词匹配准确率（评测集）；集对齐边界（插入新集导致集号偏移时按内容相似度对齐）；验收用例 TC-38-01～02（[TST-02](docs/test/02-需求追踪矩阵.md)）。 | `backend/tests/`、`frontend/tests/` | 待办 | — |

## 5. 变更记录

| 日期 | 变化 |
| --- | --- |
| 2026-09-26 | 首次生成：P0 6 项、M1 基础 11 项、功能 Epic 33 个，共 193 项任务 |
