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
| P0 | 全能参考可用性与成本验证 | — | 6 | 1 | 进行中 |
| M1 | 工程底座、命令层、Operation 骨架 | 11 | 66 | 14 | 进行中 |
| M2 | 剧本导入、解析、设定集 | 4 | 23 | 0 | 未开始 |
| M3 | 资产定稿、分镜、生成全链路 | 11 | 60 | 0 | 未开始 |
| M4 | 视频、配音、生成增强 | 3 | 18 | 0 | 未开始 |
| M5 | 画布、Agent、运营辅助 = MVP | 4 | 20 | 0 | 未开始 |
| 合计 | | 33 | 193 | 15 | |

## 2. 当前焦点与下一步

1. **评审文档**：PRD-01、REQ、DES 均为“草案（待评审）”；评审通过后把功能 Epic 的状态从 `待办` 改为 `就绪`。
2. **启动 P0**（P0-01～P0-06）：评测结论决定主供应商、内容审核、Agent LLM 与画布方案，并回填设计中的待确认问题。
3. **M1 准备**：M1-01～M1-03、M1-05、M1-09、M1-10 已完成；M1-04 继续补齐公共契约门禁，M1-06 等任务按设计依赖推进。M1-05 的平台追踪已验证 Go → Agent Worker 的本机跨进程传递，完整用户生成链路仍由后续功能任务验收。

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
| P0-01 | 全能参考评测 | 候选 Seedance（方舟）、海螺 H3、OpenRouter 海外版；5 种风格 × 单角色 / 双角色 / 动作 / 运镜 / 音频参考五类镜头；记录一致性、失败率、耗时、单镜成本 | `agent/evals/`、`docs/test/03-AI评测方案.md` | 进行中（公开接口与条件预算已预检；账号计费、预算确认与真实评测待实施） | `5832c77a`、`2709dd55` |
| P0-02 | 其他能力评测 | 带参考生图（Seedream、GPT Image）、TTS（MiniMax、豆包）、剧本解析 LLM（原文位置准确率） | `agent/evals/` | 待办 | — |
| P0-03 | 样片与成本模型 | 5 种风格各 1 条 30–60 秒样片；单镜成本、单分钟成本、平均重拍次数 | `docs/acceptance/P0-验收记录.md` | 待办 | — |
| P0-04 | 注册表初始配置 | 选定模型的 ModelProfile（模式、输入上限、参数 schema、价格）写入种子文件 | `backend/db/seed/catalog.yaml` | 待办 | — |
| P0-05 | 画布 PoC | React Flow + 移植 infinite-canvas；核实源码许可，并按 REQ-02 PERF-06 验证 500 节点 / 800 连线 ≥ 50 fps、打开 ≤ 3 秒、2,000 节点 ≥ 30 fps，记录 DES-08 §7 的内存目标（DES-38-Q1/Q2） | `frontend/`、`docs/acceptance/` | 完成（本机固定样本达标；DES-38-Q1 正式选型待产品确认，真实媒体复验归 E-36） | `5832c77a`、`7004567e`、`36f08449` |
| P0-06 | P0 决策收口 | 确定主供应商、内容审核服务、Agent LLM；回填 DES-11-Q1、DES-31-Q1、DES-33-Q1、DES-39-Q2、DES-41-Q3 等 | `docs/design/`、`docs/prd/01-产品需求文档.md` | 待办 | — |

**P0-05 验证记录（2026-09-27）**：固定上游提交 `dab19adc0847e32e39b7fc8ff90cb392561fb826` 的 LICENSE 为 MIT。PoC 用 React Flow 重新实现其卡片与工具栏界面，保留许可全文及公示入口；未复制上游业务代码。500/600、500/800、2,000/3,200 三组场景的本机重复帧率、首屏时间、进程 RSS 代理指标及局限见 [P0-05 画布 PoC 记录](docs/acceptance/P0-画布PoC记录.md)。依用户要求直接提交至 `main`，未创建 PoC 分支。此结果不替代 DES-38-Q1 的产品选型确认或 E-36 的真实媒体、跨设备与业务验收。

**P0-01 接口预检（2026-09-27）**：TST-03 §4.2.1 核对了三家公开参考输入能力、方舟真人素材限制与 MiniMax 输出费用基数。尚无账号授权、同一素材的三家实时计费核验、预算确认或真实生成与盲评证据；不能据此做模型选型或计为评测完成。

### M1 工程底座与顶层抽象（估算 4 周）

**里程碑目标**：登录 → 建项目 → 上传看到缩略图；假供应商跑通报价 → 确认 → 完成；杀 worker 后恢复且账本只记一次；`unknown` 对账成功；CI 全绿

**Epic 实施顺序**：E-09 → E-06 → E-30 → E-07 → E-08 → E-10 → E-11 → E-21 → E-25 → E-24 → E-33（“依赖”为必须先完成的前置 Epic；“联调”为后实施的 Epic，本 Epic 先按契约与模拟实现推进）

| 任务 | 内容 | 怎么做 | 涉及文件 | 状态 | 提交 |
| --- | --- | --- | --- | --- | --- |
| M1-01 | 旧代码处置 | 标签 `legacy-2026-09` 已推送；旧实现与旧工程配置已删除（2026-09-26，PLN-01 §5 执行记录） | `backend/`、`agent/`、`frontend/` | 完成 | `chore(repo)` 删除旧实现（2026-09-26） |
| M1-02 | 仓库骨架与工具链 | 三端目录、锁文件；gofmt/goimports/golangci-lint、ruff/mypy、ESLint/Prettier/tsc；初版 Makefile 与 Compose 环境样例已在 M1-03 按新要求移除 | `backend/`、`agent/`、`frontend/` | 完成 | `bdc167a5`、`c214607b` |
| M1-03 | 本地环境 | 根目录 `.env` 配置三端本机进程，指向已启动的 PostgreSQL、Redis、Kafka、MinIO、Temporal；逐项健康检查与隔离的备份恢复演练；后续部署的 `docker-compose-env.yml` 独立定义依赖环境，本地不通过 Docker 启动；不使用项目脚本或 Makefile | `.env.example`、`docker-compose-env.yml`、`docs/operation/01-环境与部署.md` | 完成（环境底座；业务客户端在 M1-05 接入） | `e4038453`、`a2c11fa6`、`30769030` |
| M1-04 | CI 流水线 | lint、format、typecheck、test、race、govulncheck、契约一致性、镜像构建（OPS-02）；首批三端静态检查、测试与镜像构建已写入工作流，`provider.*` Activity 共享示例随 Go/Python 测试运行，公共 API 生成物契约门禁待 M1-06 | `.github/workflows/` | 进行中（公共契约门禁与最终远端运行待验证） | `2690c95d` |
| M1-05 | 平台层 | config（Viper）、log（Zap）、db（GORM/pgx）、redis、kafka（franz-go）、objectstorage（MinIO / S3 兼容协议）、temporal、otel、Wire 组合根与 `--role=api|worker|relay|all` 均已接入；API、`flow` / `media` Worker、Relay 的进程探针与本机服务集成已验证；无 Collector 时保留 Go → Temporal → Agent Worker 的 TraceID，配置 OTLP/HTTP 时可导出。其他业务事件消费者归 M1-08，完整用户生成链路按 OBS-01 由后续功能任务验收 | `backend/internal/platform/`、`backend/internal/app/`、`backend/cmd/lanverse/`、`agent/app/main_worker.py` | 完成（平台底座与跨进程追踪；不代表完整 OBS-01 产品验收） | `7f4d27ff`、`c09607ce`、`39aecd56`、`84fab235`、`4acd6529`、`ca9f78db`、`ee19a1fc`、`583ef6ed`、`fb0b6715`、`87a8b157`、`a2307b2d`、`f911c33a`、`a9f42249`、`e2541616`、`d1c69906`、本次提交 |
| M1-06 | 契约链与统一错误响应 | Gin Handler 注解与 DTO → swag 自动生成并在线提供 `/swagger/doc.json`（禁止手改）→ `@umijs/openapi` 从在线文档生成 `frontend/src/gen/api`；公共 `/api` 的 Go 全局错误映射与 panic 恢复按 DES-03 返回 RFC 9457 错误，成功响应使用端点 DTO；Axios `frontend/src/lib/request.ts` 为请求唯一入口并统一解析错误；契约测试覆盖校验、鉴权、冲突、404、依赖和未预期错误及脱敏，CI 校验在线文档、公开路由覆盖和生成物一致。Activity 输入输出类型由 Go 与 Python 各自手写，以同一组示例校验一致；`provider.submit/query/cancel` 的内部预备见下文，公共契约链待 DES-03 评审 | `backend/internal/app/`、`backend/docs/`、`frontend/src/gen/api/`、`frontend/src/lib/request.ts` | 待办（内部 `provider.*` 预备已验证；公开部分待 IF4～IF6 决策） | — |
| M1-07 | 命令层 | 鉴权（Redis 会话）、幂等键、`expected_revision`、审计、Outbox 统一中间层 | `backend/internal/command/` | 待办 | — |
| M1-08 | Outbox 与实时链路 | Outbox relay → Kafka → realtime 消费者 → Redis Pub/Sub → SSE（`/api/projects/{pid}/events`、`/api/me/events`，Last-Event-ID）；已落 `infra.outbox`、`infra.processed_event` 迁移、常驻 Outbox 投递循环、月分区创建、默认分区搬迁及空历史分区删除方法、数据库副作用去重边界、30 天消费标记与已投递 7 天 Outbox 的分批清理方法、两项清理的 Temporal Workflow/Activity 与 Schedule 安装器、手动提交 Kafka offset 的消费者、`operation.status_changed.v1`、`workspace.project_changed.v1`、`billing.budget_changed.v1` 与 `billing.settled.v1` 实时投影、Redis 补读和项目 SSE 处理器 | `backend/db/migrations/`、`backend/internal/infra/`、`backend/internal/app/` | 进行中（本机 PostgreSQL、Kafka、Redis、Temporal 与项目 SSE 处理器的测试授权链路已验证；进程角色中的 Outbox/realtime 链路及清理 Schedule 安装命令已接入；公开项目 SSE 路由待身份与项目授权后挂载，个人通知 SSE、其余事件投影和部署环境 Schedule 周期触发待完成） | `cbfa526d`、`7632f630`、`abf70b5a`、`e9cd3b45`、`c2d83288`、`22e3b235`、`891a00f6`、`c8b63843`、`7939d661`、`a8aa6c20`、`6793c419`、`ba5e96da`、`c6877115`、`850e8ce7`、`74aab306` |
| M1-09 | Temporal Worker 与 OperationWorkflow 骨架 | flow / media 队列 Worker；Operation 状态机 quote → confirm → submit → poll → ingest → moderate → settle，`unknown` → 对账（DES-04） | `backend/internal/operation/` | 完成（骨架：已确认单项的 mock 图片生成、对账、接管、审核和结算；公开确认命令、批量、取消和真实供应商由对应 Epic 实现） | `9ec58b2e`、`a2e039db` |
| M1-10 | Agent 服务骨架 | FastAPI + Temporal Activity Worker（agent 队列）；Harness 骨架（Skill Registry、执行循环、校验、预算、Trace）与假供应商适配器；增加内部路由时统一映射异常并测试内部错误契约 | `agent/app/` | 完成（模拟供应商、双队列 Activity、Harness 与内部错误契约；真实模型和业务 Skill 由后续 Epic 接入） | `c3886325`、`1aeb91f4`、`cdee0eb2`、`a8763d61`、`ca7d7bd6`、`64b6a9b7`、`106fec93`、`bb4809e9`、`a398fc88`、`1839d0d0`、`ae1d01db`、`2d297630`、`754174bb`、`ba7d4594` |
| M1-11 | 前端骨架与共享状态 | App Router 布局、shadcn/ui、应用级 TanStack Query Provider 与查询失效、next-themes 明暗主题、按功能划分的 Zustand + Immer 编辑状态、SSE 订阅、页面错误恢复、`param_schema` 表单组件、报价确认组件框架；验证请求错误解析、共享查询与局部状态；已建立 `/projects` 工作台外壳、共享查询缓存、明暗主题、项目页异常恢复、模型参数表单和报价确认组件框架，其余待实施 | `frontend/src/` | 进行中 | `86601dc6`、`58a7ae66`、`15e1a9a7`、`a9467f55`、`55a94493`、`ab0d8a37`、`46bf86fd` |

**M1-03 技术验证（2026-09-27）**：本机 `pg_isready`、`redis-cli ping`、Kafka `kafka-broker-api-versions`、MinIO live 探针和 Temporal cluster health 均通过。使用 `.env.example`（不读取现有 `.env`）直接启动三端，三个健康接口均返回 `{"status":"ok"}`。在全新临时 PostgreSQL 库写入一行，`pg_dump -Fc` → `pg_restore` 后查得原值，随后清理两个临时库及转储文件。两份 Compose YAML 分别通过配置校验，未启动容器。此证据证明本机环境和进程启动，不代表 M1-05 的业务客户端连接或 M1 总体验收。

**M1-04 当前证据（2026-09-27）**：`.github/workflows/ci.yml` 的 GitHub Actions 语法经 `actionlint v1.7.12` 检查通过；相同三端工具命令已在本机通过。`docker build` 分别构建 backend、agent、frontend 镜像通过，未启动容器。PostgreSQL、Redis 与 Kafka 连接集成测试已在本机通过并加入 CI；Wire 生成与格式化后文件哈希一致且已加入 CI。Temporal 客户端仅在本机集成验证；GitHub Actions 远端运行、契约生成物、其余跨边界测试及端到端冒烟尚无通过证据，不能计为通过。

**M1-05 数据库切片（2026-09-27）**：`LV_DB_DSN` 缺失时 API 启动前失败；使用本机全新临时 PostgreSQL 库，GORM/pgx 执行 `SELECT 1`、API `/healthz` 返回成功，库中未生成业务表，随后清理临时库。Go 格式、静态检查、Race 测试及 `govulncheck` 通过；CI 已加入 PostgreSQL 服务，但远端运行未验证。

**M1-05 Redis 切片（2026-09-27）**：`LV_REDIS_URL` 缺失或格式错误时 API 启动前失败；客户端对本机 Redis 执行 `PING` 通过。隔离临时 PostgreSQL 库下运行 API，PostgreSQL 与 Redis 可达时 `/healthz`、`/readyz` 均为 200；把 Redis 指向本机关闭端口后，`/healthz` 仍为 200、`/readyz` 为 503，临时库已清理。Go 格式、静态检查、Race 测试及 `govulncheck` 通过；CI 已加入 Redis 服务，但远端运行未验证。

**M1-05 Kafka 客户端切片（2026-09-27）**：`LV_KAFKA_BROKERS` 从根目录环境配置读取，拒绝缺失或格式错误的 broker 地址；franz-go 客户端对本机 Kafka 执行只读元数据 `Ping` 通过，未创建主题或写入事件。Go 格式、静态检查、Race 测试及 `govulncheck` 通过；CI 已加入 Kafka 服务，但远端运行未验证。SASL/TLS、Outbox 投递和消费者尚未接入；MinIO、OTel、Wire、worker/relay 角色仍未实现，M1-05 不计完成。

**M1-05 Temporal 客户端切片（2026-09-27）**：本机 Temporal 集群健康检查通过，创建项目专用 `lanverse-local` 命名空间；Go SDK 对服务与命名空间检查通过。API 使用延迟连接，Temporal 可达时 `/healthz`、`/readyz` 为 200；指向关闭端口时 API 仍运行，`/healthz` 为 200、`/readyz` 为 503。使用隔离临时 PostgreSQL 库验证后已清理；Go 格式、静态检查、Race 测试及 `govulncheck` 通过。CI 尚未提供 Temporal 测试服务，远端集成未验证；Worker 与工作流仍待 M1-09，M1-05 不计完成。

**M1-05 Wire 组合根切片（2026-09-27）**：API 的 PostgreSQL、Redis、Temporal 与 HTTP Server 由 Wire v0.7.0 生成代码装配；生成代码在后续构造失败时释放已建立的客户端。本机重生成、格式化后文件哈希一致；Go 全部门禁和三依赖可用时的 `/healthz`、`/readyz` 均通过，隔离临时库已清理。官方 Wire 仓库已归档，见 DES-08 维护风险；MinIO、OTel、worker/relay 角色及远端 CI 仍待实现，M1-05 不计完成。

**M1-05 对象存储客户端切片（2026-09-27）**：`LV_OBJECT_STORAGE_*` 由根目录环境配置加载，对象存储适配器使用 S3 兼容协议，由 Wire 注入 API；`/readyz` 对配置的桶执行只读 `HEAD` 检查，不创建桶或写对象。单元测试用本地 HTTP 服务验证配置错误、桶存在和缺失路径；使用根目录 `.env` 中的本机 MinIO 凭据完成真实私有桶认证测试。根目录 `.env` 已补齐本机进程配置，权限为 0600 且被 Git 忽略；另建独立 `lanverse` PostgreSQL 数据库。API 在本机依赖可达时 `/healthz`、`/readyz` 均为 200，对象存储端点不可达时分别为 200、503。Compose 配置校验通过，未启动容器。`gofmt`、`goimports`、`go vet`、`go test -race ./...`、`golangci-lint` 通过；`govulncheck` 无可达漏洞，发现 1 个未调用模块的告警。远端 CI 待验证，OTel 与 worker/relay 角色待实施，M1-05 不计完成。

**M1-05 API 追踪切片（2026-09-27）**：`LV_OTEL_ENDPOINT` 为可选 OTLP/HTTP 根地址；本机未运行 Collector 时留空禁用导出，API 就绪状态不依赖追踪服务。Wire 为 API 显式注入 TracerProvider，Gin 接续入站 `traceparent`，GORM 查询跨度保留父链路且追踪属性不记录 SQL 参数；停机时刷新导出缓冲。使用本地 OTLP 接收端验证实际 protobuf 请求、服务名与跨度，并在本机 PostgreSQL、Redis、Temporal、MinIO 可达时验证整个 API 组合根导出 `/readyz` 请求；未启动 Docker。`govulncheck` 曾发现 GORM 官方插件间接引入的 ClickHouse 可达漏洞，改用不引入该模块的 otelgorm 插件后复测为 0 个可达漏洞。真实 Collector、Temporal 拦截器、Agent 跨进程传递、远端 CI 仍待验证，M1-05 不计完成。

**M1-05 Temporal 追踪切片（2026-09-27）**：Temporal 客户端使用显式注入的 TracerProvider 安装 OpenTelemetry 拦截器；本机 `lanverse-local` 命名空间中，对不存在的工作流执行只读 Query，确认客户端跨度继承 API 父跨度的 Trace ID 与 Parent Span ID，未创建工作流。连接失败仍由延迟客户端按原有就绪检查处理。Worker/Activity 侧、Agent 跨进程传播、真实 Collector 与远端 CI 尚未验证，M1-05 不计完成。

**M1-05 真实 Collector 验证（2026-09-27）**：从 OpenTelemetry 官方发布下载 `otelcol` v0.161.0 macOS ARM64 二进制及对应 SHA-256，校验一致后仅在临时目录运行；使用本机回环地址上的 OTLP/HTTP 接收器和 debug 导出器。以 `LV_OTEL_ENDPOINT=http://127.0.0.1:4318` 启动 API，带 `traceparent` 请求 `/readyz` 返回 200，Collector 记录 `resource spans: 1, spans: 1`。两个进程已停止，未启动 Docker。此证据证明 API 能将一条追踪送达真实 Collector；未验证 Tempo 持久化、前端到 Agent 的全链路、远端 CI 或生产采样与告警，M1-05 仍在进行中。

**M1-05 进程角色切片（2026-09-27）**：`--role=worker --queues=flow` 注册现有基础设施维护 Workflow/Activity，`--role=relay` 并发运行 Outbox 投递与 `operation.status_changed.v1` 消费；角色取消会等待循环退出，任一循环失败会停止另一循环并报告错误。先以角色测试失败确认 Red；本机隔离 PostgreSQL 库加 Temporal 的 Race 测试证明正式 Worker 执行清理 Workflow 并删除过期标记；另一隔离库加 Kafka、Redis 的 Race 测试证明正式 Relay 将 Outbox 行投递、写入消费标记并发布到项目 Redis Stream。临时库已删除，测试 Kafka 主题已请求删除；本机未启动 Docker。尚未接入审计消费者、其他 realtime 事件、`media` 队列、Schedule 安装命令、Worker/Relay 健康接口和远端 CI；三个进程角色现均由 Wire 装配，M1-05、M1-08、M1-09 不计完成。

**M1-05 Wire 角色装配复核（2026-09-27）**：按 DES-08 将 Worker/Relay 改为 Wire 组合根；生成代码在构造失败和进程退出时逆序释放 Trace、数据库、Temporal/Kafka/Redis 与消费者。先核对原角色接线和 CI 的测试目录约束，移除 `backend/internal/app` 中的测试并在 `backend/tests/app` 增加错误信封导致 Relay 停止的进程级用例。本机隔离 PostgreSQL 加 Kafka/Redis/Temporal 的 `go test -race ./tests/app -run 'TestWorkerRole|TestRelayRole' -count=1 -v` 三例通过；Wire 重新生成并经 goimports 格式化后 SHA-256 与原生成物一致。临时库已删除，测试 Kafka 主题已请求删除；未启动 Docker。审计消费、其余 realtime 事件、维护 Schedule 自动运行验收、`media` 队列与远端 CI 仍待完成。

**M1-05 合并角色切片（2026-09-27）**：`--role=all` 同时运行已实现的 API、`flow` Worker 和 Relay；任一角色退出后取消并等待其余角色，保留首个明确的失败原因。`TestParseRole/all` 先失败再通过，依赖初始化失败用例验证合并角色能及时退出；`go test -race ./...`、`go vet ./...`、`golangci-lint run ./...`、gofmt/goimports 检查通过，`govulncheck ./...` 无可达漏洞（仍有 1 个未调用依赖模块告警）。现有单角色的本机依赖集成证据不等同于 `all` 同时连接所有依赖的端到端验收；远端 CI 也未验证。M1-05 继续进行中。

**M1-05 Worker/Relay 存活探针（2026-09-27）**：两个进程在依赖初始化后分别监听 `LV_WORKER_HEALTH_ADDR`（默认 `:8081`）与 `LV_RELAY_HEALTH_ADDR`（默认 `:8082`），`GET /healthz` 返回进程存活状态；角色退出、监听失败或收到取消信号时关闭探针并等待循环结束。根目录 `.env` 与 `.env.example` 已补齐地址，应用 Compose 已加入两个角色，依赖 Compose 仍独立。无需外部服务的角色探针取消与错误传播 Race 测试通过；本机临时 PostgreSQL 库、Temporal、Kafka、Redis 的 `go test -race ./tests/app -run 'TestWorkerRoleRunsMaintenanceOnLocalTemporal|TestRelayRolePublishesAndProjectsOnLocalServices' -count=1 -v` 两例通过，包含探针 200 和停止后端口关闭断言；临时库、测试主题及消费组已删除。`go test -race ./...`、`go vet ./...`、`golangci-lint run ./...`、格式检查通过；`govulncheck ./...` 无可达漏洞，另有 1 个未调用依赖模块告警。两份 Compose 文件仅执行配置校验，未启动 Docker；真实部署的长期心跳、积压告警和远端 CI 尚未验证。M1-05 继续进行中。

**M1-05 合并角色本机集成验证（2026-09-27）**：使用根目录未跟踪的 `.env`，在隔离 PostgreSQL 库和临时 Kafka 主题上执行 `go test -race ./tests/app -run TestAllRoleRunsAndStopsOnLocalServices -count=1 -v`；测试直接调用 `app.RunAll`，同时启动 API、Worker、Relay，三个健康接口通过，Outbox 事件经 Kafka 投影到 Redis Stream，取消后三个监听端口关闭。首轮 Race 测试发现 Redis 全局日志配置由并发角色重复写入，已移至 CLI 启动时只配置一次；修复后测试通过。临时库、主题和消费组已删除；此证据不包含 CLI 整进程启动、生产长期运行或远端 CI，M1-05 继续进行中。

**M1-05 平台层收口（2026-09-28）**：Go `otelconn.Open` 在无 Collector 时原先使用 no-op provider，新增测试先复现无有效 TraceID，再改为只禁用导出、保留 SDK 追踪上下文；Agent Worker 接入 Temporal OpenTelemetry 拦截器与可选 OTLP/HTTP 导出，退出时刷新，应用 Compose 向 Agent Worker 传递 `LV_OTEL_ENDPOINT`。本机 Temporal 的 Go 工作流 / Activity 与 Python 工作流 / Activity 各自继承父 TraceID；隔离命名空间中的 `go test -race ./tests/temporalconn -run '^TestGoWorkflowToPythonActivityKeepsTraceID$' -count=1 -v` 由真实 Go 工作流调用真实 Python Agent Worker 的 `moderation.check`，本地 OTLP 接收端解码其跨度并核对与 Go 父 TraceID 相同；默认全量测试不设置本机服务变量时跳过此用例。`--role=worker --queues=media` 原生进程启动后，健康探针和 Temporal Activity poller 可用；隔离 PostgreSQL 库、Kafka、Redis、Temporal、MinIO 上的 `TestAllRoleRunsAndStopsOnLocalServices` Race 测试通过，退出后探针关闭。临时数据库和命名空间均已删除，本机未通过 Docker 启动服务。Go 全量 Race、vet、golangci-lint、goimports、govulncheck 与 Agent 全量 pytest、Ruff、mypy、锁文件检查通过；两份 Compose 只做配置解析。完整浏览器请求、公开生成命令、真实供应商调用与集中检索仍按 OBS-01 / 后续 Epic 验收，本项只认定平台底座完成；`--role=all` 的整进程 CLI 启动与生产长期运行未验证，现有证据是应用入口 `RunAll` 的真实服务集成。

**M1-08 项目 SSE 链路复核（2026-09-27）**：在上述 `app.RunAll` 的本机隔离集成测试中，用测试专用项目授权检查先建立 SSE 订阅，再写入 Outbox；Kafka 消费者写入 Redis Stream 后，HTTP 流实际收到同一事件 ID 的 `operation.updated` 帧，`operation_id` 与状态正确。Race 测试通过，临时 PostgreSQL 库、Kafka 主题和消费组已删除。核对 `NewRouter` 发现公开 API 尚未挂载项目 SSE：项目成员鉴权未实现，不能把测试授权处理器当作公开端点验收；其他事件投影和 `/api/me/events` 也未实现，M1-08 继续进行中。

**M1-08 项目事件与补读修正（2026-09-27）**：正式 Relay 新增 `workspace.project_changed.v1` 消费与 `project.updated` 投影；隔离本机 PostgreSQL、Kafka、Redis 的 Race 用例从真实创建项目命令验证 Outbox、审计、Redis Stream 和测试授权 SSE。旧补读实现以重复 ID 的最后一次出现为游标，若其间已有新事件会漏传；现改为首次匹配，再对后续 ID 去重。上述“Redis 补读切片”记录描述的是当时实现，本段修正其现行行为。公开项目 SSE 路由与 `/api/me/events` 仍待实现，M1-08 继续进行中。

**M1-08 清理 Schedule 安装命令（2026-09-27）**：按 DES-04 §11 与 OPS-01 接入 `lanverse temporal setup`，仅对已有命名空间安装 `outbox-cleanup` 和 `processed-event-cleanup`，并复用 Wire 装配的 Temporal 客户端；不安装尚缺审计和调用明细边界的 `partition-maintain`。先以缺失应用入口的编译失败确认 Red；隔离前缀的 `go test -race ./tests/app -run TestTemporalSetupInstallsCleanupSchedules -count=1 -v` 在本机 Temporal 验证安装、重复安装和 `flow` 队列，随后删除测试 Schedule。CLI `go run ./cmd/lanverse temporal setup --prefix=…` 也实际安装并通过 CLI 描述检查，两项测试 ID 已删除。项目正式 Schedule 尚未在部署环境安装或周期触发；数据库迁移、Worker 可用性、远端 CI 仍须验收，M1-08 不计完成。

**M1-08 预算与结算实时切片（2026-09-28）**：正式 Relay 新增 `billing.budget_changed.v1` 和 `billing.settled.v1` 消费，将预算调整及结算后的余额分别投影为只含 `available_micros` 的项目 `budget.updated`；旧结算事件缺少余额字段时投影 `resync`，避免新消费组从历史 offset 读取时停在首条旧事件。结算事务写入结算后余额，并在首次跌破 20% 时写入 `billing.budget_low.v1` Outbox；通知投递仍由 E-33 负责。预算调整与结算分别在隔离本机 PostgreSQL、独立 Kafka Broker、Redis 与测试授权 SSE 中通过正式 Relay 跨边界 Race 验证，包含 Outbox、消费标记和重复处理；结算低额场景证明结算与低额两条 Outbox 事实均进入各自 Kafka Topic，项目 SSE 只收到一条余额更新。结算生产侧另在隔离 PostgreSQL Race 测试中验证阈值、回滚和重放，旧事件 `resync` 经本机 Redis 发布与补读测试通过。Compose 增加需显式调用的一次性 Schedule 安装服务，仅执行配置解析；本机未启动 Docker。公开 SSE、个人通知 SSE、正式部署的 Schedule 周期触发与远端 CI 尚未验收，M1-08 不计完成。

**M1-08 Temporal 周期触发复核（2026-09-28）**：将维护集成测试从手动 `Trigger` 改为等待隔离 Schedule 的自动周期触发；本机 Temporal `lanverse-local` 命名空间与独立 Go Worker 实际执行两项清理 Workflow，隔离 PostgreSQL 中已投递的过期 Outbox 和过期消费标记被删除，未投递 Outbox 保留。`go test -race ./tests/maintenance -run '^TestScheduledMaintenanceRunsOnLocalTemporal$' -count=2` 连续两次通过；测试 Schedule 在清理回调中删除，本机未启动 Docker。此证据不等于正式部署的每日 Schedule 已安装和触发，M1-08 继续进行中。

**M1-08 项目 SSE 撤权复核（2026-09-28）**：项目 SSE 处理器在 Redis 补读后、发送实时事件前及心跳前重新调用注入的项目授权检查；补读期间撤权时返回 403，已建立的连接在下一次事件或心跳时关闭。本机 Redis 上三个 HTTP 集成场景连续两轮 Race 测试通过。授权回调目前仅在测试中注入，公开 `/api/projects/{pid}/events` 尚未挂载；此证据不构成真实浏览器会话与项目权限验收，M1-08 继续进行中。

**M1-11 工作台外壳切片（2026-09-27）**：根路径跳转 `/projects`；项目页有语义化主导航、跳转主内容入口、项目功能未接入的真实空状态与创作流程概览，内容分组遵循无边框设计。本机直接启动 Next 开发服务，在浏览器检查根路径跳转、键盘跳转入口及 1024px / 390px 视口无横向溢出；`eslint`、`prettier`、`next typegen`、`tsc`、`vitest`、`next build` 通过。登录、真实项目列表、TanStack Query、SSE、参数表单和报价组件仍待接入；本切片不构成 M1-11 完成或产品验收。

**M1-11 共享查询与错误恢复切片（2026-09-27）**：根布局挂载单一 TanStack Query Provider；组件测试证明同一查询由两个子组件共享、失效后共同更新。项目路由增加 Next 错误边界，提供重试操作且不显示内部错误文本；组件测试验证重试回调。前端类型依赖与 CI 的 Node 24 对齐，`pnpm peers check`、锁文件冻结安装均通过。本机 Next 16 开发服务下浏览器确认 `/projects` 正常渲染，`vitest`、ESLint、Prettier、`next typegen`、TypeScript 与生产构建已通过；错误恢复的浏览器验证见下述记录，M1-11 仍进行中。

**M1-11 错误边界回调修复（2026-09-27）**：[Next.js `error.tsx` 约定](https://nextjs.org/docs/app/api-reference/file-conventions/error)向页面错误组件注入 `reset`。原组件与测试误用 `retry`，导致真实错误页的重试按钮无法调用框架回调；先按 `reset` 修改测试并确认失败，再修复组件，目标测试、ESLint、Prettier 与 Next 生产构建通过。本机 Next 16 开发服务中临时加入 `/projects/recovery-probe` 子路由，点击后抛出客户端渲染异常，浏览器显示项目错误页；点击“重试加载”后原页面重新出现。临时路由与开发服务均已移除、停止；清理该路由留下的 `.next/dev/types` 缓存后，最终生产构建通过且路由表不含探针。该验证覆盖路由错误边界与重试回调，不代表真实项目数据加载故障的端到端恢复。

**M1-11 明暗主题切片（2026-09-27）**：按已接受的 DES-08 §2 接入 `next-themes`，在应用 Provider 管理 `html.dark`，项目页导航提供带可访问名称的切换按钮；默认跟随系统，切换选择由浏览器保存。按 Vercel 界面规范将共享 Button 的 `transition-all` 改为实际变化属性，并保留触控与键盘焦点反馈。本机浏览器验证浅色 → 深色、刷新后仍为深色、再切回浅色；`vitest`、ESLint、Prettier、依赖 peer 检查、冻结锁文件安装与 Next 生产构建通过。登录、真实项目列表、SSE、参数表单、报价组件与局部编辑状态尚未完成，M1-11 仍进行中。

**M1-11 模型参数表单切片（2026-09-27）**：按 DES-11 §3、REQ-08 R2/R3 的字段定义实现 `ModelParamsForm`，用 React Hook Form、Zod 与 shadcn Field 渲染输入、文本、下拉选项、滑块、开关、分段和音色选择；按模式过滤，模型版本或模式切换时清除旧值，整数范围和必填错误就地呈现。组件测试覆盖模式 / 模型切换、可选参数省略、无效输入阻止提交与首个错误聚焦。`pnpm install --frozen-lockfile --ignore-scripts`、`pnpm peers check`、全量 Vitest（6/6）、ESLint 零警告、Prettier、`next typegen`、TypeScript、`next build` 均通过。真实模型注册表与报价接口尚未接入，组件未在生产路由挂载，不能计作生成面板验收；SSE、局部编辑状态和报价确认仍待实施。

**M1-11 报价确认组件框架切片（2026-09-27）**：按 DES-24 §7、REQ-05 §4.1 实现共享 `QuoteConfirmDialog` 与 `useQuote`：逐项费用和可展开明细、批量剔除、合计与剩余预算、有效期、境外区域提示、零费复用及强制重做入口；金额与服务端合计不一致、余额不足、过期、错误项和服务端不可确认时阻止提交。浏览器在 1280px、390px 视口检查布局，390px 无横向溢出；剔除后按钮金额更新，聚焦按钮按 Enter 未确认；弹窗范围 axe 检查 0 项违规，临时预览路由已移除。`pnpm peers check`、全量 Vitest（11/11）、ESLint 零警告、Prettier、`next typegen`、TypeScript 与 `next build` 通过。组件尚未在生产路由挂载，真实报价/确认接口和 Playwright 端到端验收待 E-21 依赖完成；M1-11 与 E-21-04 均未计完成。

**M1-09 状态机切片（2026-09-27）**：按 DES-04 §4.1、§8.4–8.5 增加 Operation 状态枚举和迁移校验；测试覆盖异步供应商成功、`unknown` 对账、人工处理、零费复用、报价过期、取消成功和取消未生效，禁止跳过确认、结果未知后再次提交和终态改写。同状态重放由领域规则接受；设计文档明确供应商请求已发出或结果不明时不得按零费用取消。`gofmt`、`goimports`、`go vet ./...`、`golangci-lint run ./...`（0 项）、`go test -race ./...` 均通过；`govulncheck ./...` 无可达漏洞，另有 1 个未调用模块告警。当前仅为领域规则，尚未接入 PostgreSQL 条件更新、同事务结算与 Outbox、Temporal Worker 或真实 Operation 端到端验证，M1-09 不计完成。

**M1-09 工作流骨架完成（2026-09-28）**：正式 `--role=worker --queues=flow,media` 已同时注册两条队列；`OperationWorkflow` 从已确认单项开始，以持久 Activity 记录供应商调用、条件迁移和同事务预算结算，`unknown` 先对账，未解时等待人工决定，媒体结果经 HTTPS 来源校验、MinIO 接管与审核后入库。不安全及过期结果 URL 进入明确的失败结算路径，跨来源重定向不传递签名 URL 的 `Referer`。本机全新 PostgreSQL 库应用全部迁移，新增迁移的 down/up 再执行成功；`go test -race ./...`、真实 MinIO 的 media/objectstorage Race 测试、Python 全量 pytest/Ruff/mypy、Go vet/golangci-lint/govulncheck 均通过。真实本机 Temporal + PostgreSQL + MinIO + Agent 联调验证 flow Worker 重启后的完成、`unknown` 对账以及终态结算重放。此处是单项 mock 图片的工作流骨架；报价/确认公开命令、批量、真实供应商计费、取消与完整人工核对分别留在 E-21、E-23、E-24 等任务，不能据此认定其产品验收通过。

**M1-08 Outbox 建表切片（2026-09-27）**：按 DES-02 §5.11 建立 `infra.outbox` 及待发布索引，迁移时预建当前 UTC 月和未来三个月的分区；默认分区接纳超出窗口的写入，后续定时维护须在挂载新分区前搬迁其中已有数据。使用 `golang-migrate` v4.19.1 对本机临时 PostgreSQL 库执行 up/down，验证 5 个分区、当前月与默认分区分别落行及回滚；临时库已清理，现有 `lanverse` 业务库未执行该迁移。relay、消费者、SSE、分区定时维护及远端 CI 待实施，M1-08 不计完成。

**M1-08 Relay 投递切片（2026-09-27）**：应用层按单条事件协调领取与发布；PostgreSQL 适配器在事务中按 `create_time, id` 使用 `FOR UPDATE SKIP LOCKED` 领取待投递行，Kafka 同步确认后才更新 `published_at`；失败保留待投递状态，信封 `event_id`、`event_type` 与行不符时拒绝发布。使用本机临时 PostgreSQL 库和独立 Kafka 测试主题验证失败重试、真实投递、项目分区键、事件载荷和投递确认；临时库已删除，测试主题已请求删除。`gofmt`、`goimports`、`go vet ./...`、`golangci-lint run ./...`（0 项）、`go test -race ./...` 均通过；`govulncheck ./...` 无可达漏洞，另有 1 个未调用模块告警。应用进程尚未接入 relay 循环，业务事务写入、消费者去重、SSE 与远端 CI 仍待完成，本项不计完成。

**M1-08 消费去重切片（2026-09-27）**：按 DES-02 §5.11 建立 `infra.processed_event` 唯一 `(consumer, event_id)` 记录；PostgreSQL 适配器先插入去重记录，再在同一事务中调用处理回调。临时 PostgreSQL 库验证回调失败时记录与数据库副作用一同回滚，重试成功后重复事件不再调用回调；从仅有 Outbox 的前一迁移版本升级、回滚去重迁移后原 Outbox 数据保留，显式本机 Race 测试通过。两条迁移的 up/down 均通过，临时库已清理。Kafka 消费循环、offset 提交、Redis 实时扇出与 SSE 尚未接入；外部副作用需容忍数据库提交失败后的重放，M1-08 继续进行中。

**M1-08 实时消费切片（2026-09-27）**：新增 Kafka 单条消费循环，关闭自动提交并在处理成功后同步提交 offset；失败返回错误、不提交，rebalance 在处理完成后释放。`operation.status_changed.v1` 校验主题、项目分区键、事件与聚合 UUID、合法状态，将信封投影为 `operation.updated`；复用 `infra.processed_event`，Redis 写入项目 Stream（10 分钟、5,000 条）后发布到 `project:{id}`。本机独立 Kafka 主题验证失败后同组重投、成功后不重投；临时 PostgreSQL 库加本机 Redis 验证发布、缓冲和重复事件跳过，Race 测试通过；临时库已删除、主题已请求删除。尚未接入 `backend-relay` 进程角色、SSE Hub 与路由，其他 realtime 事件也未映射，M1-08 不计完成。Redis 副作用若已成功而 PostgreSQL 提交失败，可能以相同事件 ID 重放，后续 SSE Hub 应按 ID 去重。

**M1-08 Redis 补读切片（2026-09-27）**：按 `Last-Event-ID` 在项目 Stream 中读取后续事件；游标不存在时要求 `resync`，重复事件 ID 按最后一次出现的位置定位并在补读中去重。本机 Redis Race 用例先复现重复事件位置造成的旧事件多补发，再修复并通过；全量 Go Race、vet、golangci-lint、goimports 通过。SSE Hub 应先订阅 Pub/Sub 再补读，并在实时消息与补读之间继续按 ID 去重；鉴权路由尚未接入。

**M1-08 项目 SSE 处理器切片（2026-09-27）**：处理器构造时强制注入项目授权检查，订阅项目 Pub/Sub 后补读 Redis Stream，再推送实时消息；按事件 ID 去重，支持 `resync`、20 秒心跳和 30 分钟连接上限。使用本机 Redis、HTTP 流和 Race Detector 验证补读后实时事件、重复消息、拒绝访问、缺失游标、心跳与连接到期；全量 Go Race、vet、golangci-lint、goimports、govulncheck 通过。当前 API 尚无项目成员鉴权，故处理器没有挂载到公开路由；`/api/me/events` 和前端 SSE 订阅也尚未接入，本项不计完成。

**M1-08 常驻 Relay 循环切片（2026-09-27）**：Outbox Relay 有待投递行时连续处理，空队列时等待 250 毫秒，进程取消时及时退出；投递失败返回错误，供进程管理层报告和重启。单元测试按 Red→Green 验证积压清空、取消和错误传播；此前本机 PostgreSQL→Kafka 单条投递集成证据仍有效，本切片未重新执行该集成用例。全量 Go Race、vet、golangci-lint、goimports、govulncheck 通过。`backend-relay` 角色尚未接入运行循环，M1-08 不计完成。

**M1-08 Outbox 分区维护切片（2026-09-27）**：维护事务创建当前 UTC 月与未来三个月的缺失分区；挂载前将目标月份已有事件从默认分区搬入新分区，失败时整笔事务回滚。为避免并发维护和写入竞态，事务期间使用 Outbox 表级独占锁，期间会阻塞 Outbox 写入。临时 PostgreSQL 库应用迁移后，Race 集成用例验证超出初始窗口的待投递事件先落默认分区、并发执行维护后搬迁且载荷不变、再次执行仍成功；临时库已删除。全量 Go Race、vet、golangci-lint、goimports、govulncheck 通过。定时任务、已投递 7 天保留期清理的调度和实际写入阻塞时长评估尚未完成，M1-08 不计完成。

**M1-08 消费去重记录保留期切片（2026-09-27）**：`processed_event` 按 DES-02 §9 保留 30 天，新增 `(create_time, id)` 索引迁移与每次最多清理指定批量的 PostgreSQL 方法；锁定中的行跳过，截止时间当刻的记录保留。先以缺失方法的编译失败确认 Red，再在两次隔离的本机临时 PostgreSQL 库应用迁移，验证旧记录分批删除、临界与新记录保留、再次执行无副作用、原有消费去重用例仍通过；第二次使用 Race Detector，并验证索引回滚，临时库均已删除。全量 Go Race、vet、golangci-lint、goimports 通过；`govulncheck` 无可达漏洞，仍有 1 个未调用模块告警。定时调度尚未接入，因此保留期清理尚未实际运行；Outbox 保留期切片见下文，远端 CI 也待验证，M1-08 不计完成。

**M1-08 Outbox 保留期切片（2026-09-27）**：DES-02 §9 修正“按月分区直接删除”与“已投递 7 天”的冲突，明确按 `published_at` 分批删除，未投递事件保留。PostgreSQL 增加已投递时间索引与限量清理方法；编译失败确认 Red 后，在隔离临时库验证跨月旧记录按批次删除、锁定行跳过且解锁后可清理、未投递及最近投递记录保留、截止时间边界、再次执行无副作用；索引 up/down 与新分区挂载用例均通过，临时库已删除。全量 Go Race、vet、golangci-lint、goimports 通过；`govulncheck` 无可达漏洞，另有 1 个未调用模块告警。清空历史分区删除方法见下文；定时调度与远端 CI 未验证，因此实际数据保留期尚未生效，M1-08 不计完成。

**M1-08 空历史分区删除切片（2026-09-27）**：按 DES-02 §9 的保留期与 DES-04 §11 的 Temporal Schedule 契约，分区维护方法在独占锁保护下仅删除已结束且无行的 Outbox 月分区；当前、未来、默认分区和仍含未投递或保留期内事件的分区不删除。先以缺失方法编译失败确认 Red，再在两次隔离临时 PostgreSQL 库验证保留有行分区、清理到期行后删除空分区、重复执行无副作用，以及原分区预建与搬迁用例；显式 Race 测试通过，临时库均已删除。全量 Go Race、vet、goimports、golangci-lint 通过；`govulncheck` 无可达漏洞，另有 1 个未调用模块告警。DES-04 §11 已同步纠正旧的整月删除描述，并列出 `processed-event-cleanup`。Temporal Schedules 与 worker/relay 角色尚未接线，故尚未实现自动运行，M1-08 不计完成。

**M1-08 Temporal 维护切片（2026-09-27）**：`c6877115` 增加 Outbox 分区维护及两项保留期清理的 `flow` Workflow/Activity，清理按批次执行并发送 Activity 心跳；`outbox-cleanup` 每日 04:00 UTC、`processed-event-cleanup` 每日 04:30 UTC 的 Schedule 安装器支持重复安装、保留暂停状态和定义漂移拒绝。先以缺失包编译失败确认 Red，再在本机 `lanverse-local` 命名空间用独立任务队列和临时 Schedule 实际触发两个清理 Workflow；隔离 PostgreSQL 临时库中的过期 Outbox 行及消费标记被删除，未投递 Outbox 行保留，测试后删除 Schedule 与临时库。独立 Schedule 测试验证重复安装、暂停状态和任务队列漂移；全量 `go test -race ./...`、`go vet ./...`、`golangci-lint run ./...`、格式检查通过；`govulncheck ./...` 为 0 个可达漏洞，另有 1 个未调用模块告警。正式 Worker/角色与安装命令尚未接线；审计和调用明细未实现前不安装完整 `partition-maintain` Schedule。当前没有部署中的自动维护，M1-08 不计完成。

**M1-10 模拟供应商切片（2026-09-27）**：Agent 增加 Redis 持久的 `MockProvider`，以稳定任务 ID 和 `SET NX` 保证并发重复提交幂等；模拟 `accepted`、`rejected`、`not_submitted`、`unknown`，支持按请求键对账、延迟、失败、取消和结果过期。新建适配器及 Redis 客户端后仍可查询既有任务。`uv sync --locked`、Ruff 检查与格式、mypy 通过；当时默认全量 pytest 为 11 通过、1 条真实 Redis 用例因未配置 `LV_TEST_REDIS_URL` 跳过，另以本机 Redis 数据库 15 显式运行该用例通过，键使用隔离前缀和 60 秒 TTL。Agent 镜像构建通过，未启动容器。该切片只验证适配器；Activity 接入见下文，可接管媒体结果与重复回调仍待实施。

**M1-10 模拟 Activity Worker 切片（2026-09-27）**：`agent.mock` 队列注册 `provider.submit/query/cancel`，按 DES-03 §7.1 映射 `provider_request_key`，Redis 写入响应不确定时返回 `unknown`。本机根目录 `.env` 启动 Worker 后，Temporal 队列看到 Activity poller，Ctrl+C 正常退出；隔离测试工作流经本机 Temporal 调用 Agent Activity，Redis 数据库 15 中用 60 秒 TTL 的独立键完成提交丢失对账。`uv sync --locked`、Ruff、mypy、当时全量 pytest（14 通过、2 条本机依赖用例默认跳过）、显式本机 Temporal + Redis 用例（2 通过）、Compose 配置检查和 Agent 镜像构建通过，未启动容器。该切片只接入模拟供应商队列；后续 Harness 接入见下文，真实供应商、可接管媒体结果与 Go Operation 端到端链路仍待实施。

**M1-10 内部 Activity 契约预备切片（2026-09-27）**：按 DES-03 §7.1 手写 Go Operation 工作流与 Python Worker 的 `provider.submit/query/cancel` 输入输出类型，双方读取同三份 JSON 示例，校验字段往返与未知字段拒绝；Python 模型限制查询引用恰有一种，并在模拟 Activity 的输入和输出边界实际使用。先以 Go 缺少工作流包、Python 缺少输出模型确认 Red，补实现后全量 Go Race、vet、golangci-lint（0 项）、gofmt/goimports 通过；`govulncheck` 无可达漏洞，另有 1 个未调用模块告警。Python 全量 pytest 为 28 通过、2 条本机依赖用例默认跳过，Ruff 与 mypy 通过；显式接入本机 Temporal 和 Redis 的模拟 Activity 用例通过。CI 工作流的 Go/Python 测试命令会运行这些示例测试，远端运行结果待本次推送确认。此处仅覆盖三种内部供应商 Activity；`llm.run_skill` 等其他 Activity、Go OperationWorkflow 与公共 API 契约仍待实施，M1-06 保持待办。

**M1-10 Harness 骨架切片（2026-09-27）**：Skill Registry 启动时校验多版本索引、包 SHA-256 与输入输出 JSON Schema，已发布的 `mock.echo@1.0.0` 可由 `agent` 队列的 `llm.run_skill` 执行。执行循环限制修复次数，模型 Router 在每次调用前检查预算、返回后按调用 ID 记账一次；Trace 只记录摘要、校验路径、token 和费用，不含原文。当前只接零费用 `mock.structured`；含工具或非 schema 校验器的 Skill 明确失败，真实模型 token 计数与工具尚未接入。`uv sync --locked`、Ruff、mypy、默认全量 pytest（23 通过、2 条本机依赖用例跳过）均通过；显式本机 Temporal + Redis 测试 2 通过，两个 Activity 队列均显示 poller，本机 Worker 正常退出；Compose 配置检查和 Agent 镜像构建通过，未启动容器。模拟 Skill 与测试工作流证据不代表真实模型、业务 Skill 或 Go Operation 验收。

**M1-10 骨架完成（2026-09-28）**：`agent-api` 对现有健康路由及后续内部路由统一返回带稳定 `code`、`request_id` 的 `application/problem+json` 错误，404、405、参数错误及未预期异常均有脱敏测试；不可信请求 ID 会重新生成。先以默认 404/500 响应确认 Red，再补映射通过 Green。Agent 全量 pytest 通过（4 条需额外配置的用例默认跳过），Ruff 检查与格式、mypy 通过；显式本机 Temporal + Redis 两条集成用例通过，覆盖模拟供应商与 Skill 队列。M1-09 已记录真实本机 Go → Agent 模拟链路；本项只关闭 Agent 骨架，不代表真实模型、业务 Skill 或 M1 总体验收。

**功能 Epic**

#### E-06 账号与会话

- **需求**：[REQ-06](docs/requirement/06-账号与会话.md)（ACC-01 管理员创建账号、密码登录；ACC-03 会话管理与登出；ACC-04 禁用账号、重置密码；ACC-05 修改本人密码）　**设计**：[DES-09](docs/design/09-账号与会话.md)　**依赖**：E-09
- **验收**：TC-06-01～09（9 条）
- **待确认**：DES-09-Q1、DES-09-Q2、DES-09-Q3、DES-09-Q4（默认方案见设计文档，确认前按默认实施）

| 任务 | 内容 | 怎么做 | 涉及文件 | 状态 | 提交 |
| --- | --- | --- | --- | --- | --- |
| E-06-01 | 数据与领域模型 | 迁移建表 / 加列：`identity.user`；实现领域对象、状态机与仓储（组织 / 平台级，按管理员权限访问），Redis 会话和登录 IP 限流。详见 [DES-09 §2](docs/design/09-账号与会话.md#2-数据) | `backend/db/migrations/`、`backend/internal/identity/domain/`、`backend/internal/identity/adapter/postgres/`、`backend/internal/identity/adapter/redis/`、`backend/internal/platform/config/` | 完成（迁移、领域与仓储、Redis 会话和 IP 限流、首位管理员 bootstrap；隔离 PostgreSQL/Redis 集成测试已纳入 CI） | 本次提交 |
| E-06-02 | 用例与接口 | 写操作经命令层（鉴权、幂等键、`expected_revision`、审计、Outbox）实现：POST /api/auth/login、POST /api/admin/users；swag 注解生成 OpenAPI。详见 [DES-09 §3](docs/design/09-账号与会话.md#3-接口) | `backend/internal/identity/application/`、`backend/internal/identity/adapter/http/`、`backend/docs/` | 进行中（会话鉴权、管理员账号创建/修改/列表/禁用/启用/重置、密码登录、登出和本人改密的内部用例已验证；公开接口和幂等待实施，公共契约依赖 DES-03 评审） | — |
| E-06-03 | 异步、工作流与事件 | 事件 `identity.user_changed.v1`（Outbox → Kafka，消费者按事件 ID 去重）。要点：无工作流。账号变更事务内写 Outbox 事件 `identity.user_changed.v1` 与 `audit.recorded.v1`（审计只经后者写入，DES-12）。 详见 [DES-09 §4](docs/design/09-账号与会话.md#4-异步与工作流) | `backend/internal/identity/adapter/workflow/`、`backend/internal/identity/adapter/event/` | 进行中（创建、修改、禁用、启用、重置及本人改密的双 Outbox 事务已验证，创建审计已由正式 Relay 消费；identity 事件消费者待实施） | — |
| E-06-04 | 前端 | - 登录页：登录名、密码、错误与锁定提示（剩余时间）。 - 强制改密页：当前密码、新密码、确认新密码，实时显示强度规则。 - 管理 · 账号：列表（登录名、显示名、角色、状态、最后登录）、创建对话框、操作菜单（修改、禁用 / 启用、重置密码）。 - 应用外壳用户菜单：修改密码、登出。 详见 [DES-09 §6](docs/design/09-账号与会话.md#6-界面) | `frontend/src/features/auth/` | 待办 | — |
| E-06-05 | 测试与验收 | 单元：密码规则、锁定计数与解锁时间、会话纪元比较、最后一个管理员保护。 集成（testcontainers PostgreSQL + Redis）：登录 → 会话 → 禁用 → 会话失效；并发修改同一账号的版本冲突。 安全：未认证访问全部接口返回 401（遍历…；验收用例 TC-06-01～09（[TST-02](docs/test/02-需求追踪矩阵.md)）。 | `backend/tests/`、`frontend/tests/` | 待办 | — |

**E-06-01 账号数据与领域切片（2026-09-27）**：按 DES-02 §5.1 建立 `identity."user"`，`citext` 加组织范围部分唯一索引；领域实现 bcrypt cost 12、密码长度与字母数字规则、五次失败锁定 15 分钟、会话纪元比较和最后管理员禁用保护。仓储创建时拒绝明文密码，按组织查询并将重复登录名映射为可判定错误；登录状态更新使用 `revision` 乐观锁，禁用管理员使用组织级事务锁以保留至少一名启用管理员。核心规则先以缺失包和方法的编译失败确认 Red；本机隔离 PostgreSQL 库应用迁移后，`go test -race ./tests/identity -count=1 -v` 验证大小写重复、跨组织隔离、过期修订和并发禁用，随后回滚迁移并删除临时库。

**E-06-01 首位管理员初始化（2026-09-27）**：按 REQ-06 §2.5 接入 `lanverse admin bootstrap --login-name <name>`；组织迁移与 DES-02 §5.2 对齐。应用命令对初始密码执行既有 bcrypt 规则，PostgreSQL 全局事务锁串行化首次初始化；若无组织则同事务创建，若已有唯一启用组织则复用，若存在任何未删除账号、多个组织或停用组织则拒绝。首位管理员强制首次改密，并同事务写身份变更及 system actor 的 `user.created` 审计 Outbox。测试先以缺失命令编译失败固定 Red；隔离 PostgreSQL 库上的 Race 用例验证首次、重复、已有组织、审计写入失败全回滚与并发仅一次成功；CLI 使用伪终端交互输入一次性测试密码，库内得到一组织、一管理员、两条 Outbox。非终端输入被拒，组织迁移回滚通过，临时库已删除；本机未启动 Docker。公开登录和首登改密页面、正式部署迁移及 TC-06 验收仍待完成。

**E-06-01 Redis 会话与限流切片（2026-09-27）**：会话使用 256 位随机令牌，Redis 键只存令牌 SHA-256 摘要，记录组织、用户、纪元及创建/最后活动时间；空闲和绝对期限由根目录 `.env` 的 `LV_SESSION_IDLE_TTL`、`LV_SESSION_ABSOLUTE_TTL` 控制，默认 12 小时/7 天。续期只修改仍存在的键，登出后并发续期不能复活会话；Redis 不可用时返回存储错误，调用方须拒绝认证。登录 IP 先规范化再取摘要，以原子计数实现每分钟 20 次固定窗口限流。测试先确认缺少实现的编译失败，再以已运行的本机 Redis DB 15 执行 `LV_TEST_IDENTITY_REDIS_URL=redis://127.0.0.1:6379/15 go test -race ./tests/identity -run 'TestSessionStore|TestLoginLimiter' -count=1 -v`，仅清理测试创建的键；此证明存储规则，不代表登录 API、跨 PostgreSQL/Redis 的禁用即时失效或 TC-06 验收。命令层审计、Outbox、API 与真实登录流程仍待 E-06-02/03，E-06 尚未完成。

**E-06-01 真实依赖 CI 门禁（2026-09-28）**：在独立 PostgreSQL 测试库应用 `identity.user` 迁移，配合 Redis DB 14 运行账号范围仓储、会话、IP 限流及禁用后鉴权拒绝的 Race 用例；首位管理员 bootstrap 在另一全新 PostgreSQL 库只应用 Outbox、账号、组织迁移，验证原子性与仅初始化一次。两组本机真实依赖测试通过，并新增 CI 专用库及显式测试步骤，避免默认全量 Race 因缺少环境变量而跳过这些集成用例。E-06-01 数据与领域范围据此收口；公开 HTTP 接口、身份事件消费、前端和 TC-06 产品验收仍分别属于 E-06-02～05。

**E-06-02 会话鉴权应用切片（2026-09-27）**：`Authenticate` 读取 Redis 会话，再按会话中的组织和用户 ID 查询 PostgreSQL 当前账号，比较启用状态及纪元后才续期；返回不含密码哈希的 Principal。会话缺失、跨组织、禁用和旧纪元均拒绝；Redis/数据库异常保留可判定的不可用错误，不退化为授权成功。先以缺少应用包确认 Red，单元测试通过后，在全新临时 PostgreSQL 库应用身份迁移，配合本机 Redis DB 15 执行 `go test -race ./tests/identity -run TestAuthenticateRejectsDisabledAccountWithRealPostgresAndRedis -count=1 -v`，验证禁用后 Redis 键仍存在但下一次鉴权失败；迁移已回滚，临时库已删除，Redis 测试键由用例清理。全量 `go test -race ./...`、`go vet ./...`、`golangci-lint run ./...`、goimports/gofmt 检查通过；`govulncheck ./...` 无可达漏洞、另有 1 个未调用的依赖模块告警。此结果是应用层和真实双存储的鉴权证据；公开 HTTP 登录、Cookie/CSRF、完整审计与 Outbox、TC-06 验收仍未完成。

**E-06-02/03 管理员创建账号命令（2026-09-27）**：先用 `TestCreateUserCommand` 的缺失类型编译失败固定 Red，应用层随后实现管理员权限、初始密码哈希、安全结果模型，以及 `identity.user_changed.v1` 与 `audit.recorded.v1` 两条事件载荷。PostgreSQL 适配器在同一事务内重新核对管理员当前角色和改密状态，再插入账号及两个 Outbox 行；事件插入失败会回滚账号。隔离 PostgreSQL 临时库执行 `go test -race ./tests/identity -run '^TestCreateUserCommitsAccountAndTwoOutboxEventsAtomically$' -count=1 -v`，验证成功写入、审计写入失败回滚、过期管理员身份拒绝；另以本机 Kafka/Redis 和正式 Relay 执行 `go test -race ./tests/app -run '^TestAccountCreationAuditReachesPostgresThroughRelay$' -count=1 -v`，验证 `user.created` 审计最终落库并写入去重标记。`go test -race ./...`、`go vet ./...`、`golangci-lint run ./...`、gofmt/goimports 检查均通过；`govulncheck ./...` 无可达漏洞，另有 1 个未调用模块告警。临时库与三条测试主题已清理，未启动 Docker。账号列表、公开创建接口、幂等键、其余账号变更与身份事件消费者仍待实施，E-06-02/03 尚未完成。

**E-06-02 内部密码登录命令（2026-09-27）**：先用缺失 `LoginCommand` 的编译失败固定 Red，再实现 Redis IP 限流、组织范围账号读取、错密计数与五次锁定、会话创建、账号 `revision` 重试和账号动作审计。成功登录在数据库提交前创建不对外返回的 Redis 会话，数据库状态或审计写入失败时销毁；失败计数与一至两条审计 Outbox 同事务提交。`go test -race ./tests/identity -run '^TestLoginCommand' -count=1` 覆盖成功、事务失败补偿、第五次锁定、未知账号、Redis 故障、IP 限流和连续修订冲突；隔离 PostgreSQL 临时库加本机 Redis 的 `go test -race ./tests/identity -run '^TestLoginUsesRealPostgresRedisAndAtomicAudit$' -count=1 -v` 验证真实会话、锁定、对外同样拒绝与内部原因区分、审计载荷白名单及审计写入失败回滚。`go test -race ./...`、`go vet ./...`、`golangci-lint run ./...`、gofmt/goimports 均通过；`govulncheck ./...` 无可达漏洞，另有 1 个未调用模块告警。临时库已删除，测试 Redis 键由用例清理，未启动 Docker。公开登录路由、Cookie/CSRF、组织 bootstrap、P95 性能和 TC-06 验收仍待实施。

**E-06-02 内部登出命令（2026-09-27）**：先以缺失 `LogoutCommand` 的编译失败确认 Red；实现当前会话校验、Redis 令牌撤销和 `auth.logout` 审计 Outbox。撤销失败不写已登出审计；撤销成功而审计失败时结果仍标记已撤销，供后续 HTTP 层清除 Cookie。请求在撤销时取消的测试先失败，改为有界独立审计上下文后通过。`go test -race ./tests/identity -run '^TestLogoutCommand|^TestLoginCommand' -count=1 -v` 覆盖成功、撤销和审计故障、取消及旧会话；隔离 PostgreSQL 临时库与本机 Redis DB 15 的 `go test -race ./tests/identity -run '^TestLoginUsesRealPostgresRedisAndAtomicAudit$' -count=1 -v` 验证真实令牌不可再鉴权、审计载荷符合消费策略、审计插入失败时仍撤销。临时库已删除，测试 Redis 键由用例清理；未启动 Docker。全量 Race、vet、golangci-lint、gofmt/goimports 通过，govulncheck 无可达漏洞（另有 1 个未调用模块告警）。Redis 与 PostgreSQL 无共同事务，撤销成功后审计故障可能留下缺失记录；公开登出路由、Cookie 清除和浏览器验收仍待实施。

**E-06-02/03 管理员禁用账号命令（2026-09-27）**：先以缺失 `DisableUserCommand` 的编译失败确认 Red；命令层校验管理员、目标和预期修订，生成 `identity.user_changed.v1` 与 `user.disabled` 审计事件。PostgreSQL 事务持有组织级管理员变更锁，重查操作者当前权限和目标修订，禁用账号并递增 `session_epoch`、`revision`，随后写两条 Outbox；任一事件写入失败回滚状态与纪元。`go test -race ./tests/identity -run '^TestDisableUserCommand' -count=1 -v` 验证权限、事件载荷与可判定错误；隔离 PostgreSQL 临时库加本机 Redis DB 15 的 `go test -race ./tests/identity -run '^TestDisableUserCommitsStateAndEventsWithRealPostgresRedis$|^TestIdentityStoreProtectsLastActiveAdmin$' -count=1 -v` 验证旧 Redis 键仍存在但下一次鉴权被拒、审计故障事务回滚、最后管理员保护及过期管理员身份拒绝。临时库已删除，测试会话由用例撤销；未启动 Docker。公开禁用接口、身份事件消费者及 TC-06 验收仍待实施。

**E-06-02/03 本人改密命令（2026-09-27）**：先以缺失 `ChangePasswordCommand` 的编译失败确认 Red；命令校验当前会话与旧密码、新密码规则，在 Redis 预备新纪元会话，随后以 PostgreSQL 事务更新哈希、清除强制改密和锁定计数、递增纪元与修订号，并写 `identity.user_changed.v1` 与 `user.password_changed` 审计 Outbox。事务失败撤销预备会话，提交成功才返回新令牌。`go test ./tests/identity -run '^TestChangePassword' -count=1` 的单元测试通过；隔离 PostgreSQL 临时库及本机 Redis DB 15 的 `go test -race ./tests/identity -run '^TestChangePasswordInvalidatesOtherSessionsWithRealPostgresRedis$' -count=1 -v` 验证当前客户端新令牌可用、两个旧令牌下一次鉴权被拒、审计写入失败时数据库回滚且新令牌撤销。全量 `go test -race ./... -count=1`、`go vet ./...`、`golangci-lint run ./...`、gofmt/goimports 均通过；`govulncheck ./...` 无可达漏洞，另有 1 个未调用依赖模块告警。临时库已删除，测试令牌由用例清理；未启动 Docker。公开改密接口、Cookie 替换、浏览器双会话和 TC-06 验收仍待实施。

**E-06-02/03 管理员启用与重置密码（2026-09-27）**：按 DES-09 补齐账号启用和密码重置的状态、修订与审计规则。先以缺失领域方法和命令的编译失败确认 Red；启用只允许禁用账号，额外递增会话纪元并清除旧锁定，防止禁用前会话复活；重置密码拒绝复用旧密码，强制下次改密并使旧会话失效。PostgreSQL 事务内重新核对管理员当前权限、目标修订与原哈希，账号修改和 `identity.user_changed.v1`、`audit.recorded.v1` 同时提交。隔离 PostgreSQL 库配合本机 Redis DB 15 的 Race 用例验证禁用→启用后旧会话仍失效、重置后当前旧会话失效、两种审计插入失败都整体回滚，以及管理员权限撤销后的命令被拒绝；迁移回滚通过，临时库已删除。公共接口、幂等、账号修改/列表与 TC-06 验收仍待完成。

**E-06-02/03 管理员账号修改与列表（2026-09-27）**：补充 DES-09 的修改、分页与权限细则及账号目录索引迁移。先以缺失领域方法、命令和查询类型的编译失败确认 Red；内部修改只接受显示名或角色，事务中锁定组织、复核管理员及目标修订，保护最后一名启用管理员并将修改与 `identity.user_changed.v1`、`audit.recorded.v1` 原子提交。列表在同一事务重新核权，按 `(create_time DESC,id DESC)` 进行类型化游标分页，包含禁用账号，排除逻辑删除和其他组织账号。隔离本机 PostgreSQL 的 `go test -race ./tests/identity -run '^TestAdminDirectoryWithRealPostgres$' -count=1 -v` 验证并发降级只成功一次、审计故障回滚、过期修订、已降级管理员拒绝读取、分页与安全字段；索引迁移回退成功且测试库已删除。全量 `go test -race ./... -count=1`、`go vet ./...`、`golangci-lint run ./...`、gofmt/goimports 检查通过；`govulncheck ./...` 无可达漏洞，另有一个未调用的依赖模块告警。公共接口、公开游标编码和 TC-06 验收仍待 DES-03 契约评审后完成。

#### E-07 供应商凭据

- **需求**：[REQ-07](docs/requirement/07-供应商凭据.md)（ACC-02 管理员配置模型供应商凭据）　**设计**：[DES-10](docs/design/10-供应商凭据.md)　**依赖**：E-06、E-09　**联调**：E-08（后实施，先按契约与模拟实现联调，对方完成后重跑本 Epic 验收）
- **验收**：TC-07-01～04（4 条）
- **待确认**：DES-10-Q1、DES-10-Q2、DES-10-Q3、DES-10-Q4（默认方案见设计文档，确认前按默认实施）

| 任务 | 内容 | 怎么做 | 涉及文件 | 状态 | 提交 |
| --- | --- | --- | --- | --- | --- |
| E-07-01 | 数据与领域模型 | 迁移建表 / 加列：`catalog.provider`、`catalog.provider_credential`；实现领域对象、状态机与仓储（组织 / 平台级，按管理员权限访问）。详见 [DES-10 §2](docs/design/10-供应商凭据.md#2-数据) | `backend/db/migrations/`、`backend/internal/catalog/domain/`、`backend/internal/catalog/adapter/postgres/` | 完成（迁移、领域与单活仓储；管理业务入口的当前管理员复核及真实 PostgreSQL 测试已纳入 CI） | `40a9e1fc`、`43afa186`、本次提交 |
| E-07-02 | 用例与接口 | 写操作经命令层（鉴权、幂等键、`expected_revision`、审计、Outbox）实现：POST /api/admin/providers/{id}/credentials、GET /api/admin/providers/{id}；swag 注解生成 OpenAPI。详见 [DES-10 §3](docs/design/10-供应商凭据.md#3-接口) | `backend/internal/catalog/application/`、`backend/internal/catalog/adapter/http/`、`backend/docs/` | 进行中（内部供应商登记、修改、凭据保存、停用及安全详情和列表查询已验证；公共鉴权、持久幂等与 HTTP 契约待实施） | `ac110dac`、`26d1bfb6`、`5e45289d` |
| E-07-03 | 异步、工作流与事件 | 工作流 / Activity / 定时任务 `CredentialTestWorkflow`；事件 `catalog.credential_changed.v1`（Outbox → Kafka，消费者按事件 ID 去重）。要点：- 测试：`backend-api` 直接执行一个短工作流 `CredentialTestWorkflow` → Activity `provider.test_credential`（`agent` 队列，超时 15… 详见 [DES-10 §4](docs/design/10-供应商凭据.md#4-异步与工作流) | `backend/internal/catalog/adapter/workflow/`、`backend/internal/catalog/adapter/event/` | 进行中（Go 工作流、双阶段鉴权及测试结果与事件的事务写入已验证；公共启动接口、缓存消费者与真实供应商联调待实施） | `ac110dac`、`26d1bfb6`、`5e45289d` |
| E-07-04 | Agent 服务 | 供应商凭据解封与连通性测试 Activity `provider.test_credential`；输入输出类型由 Go 与 Python 各自定义，契约测试（同一组示例输入输出）校验一致，离线评测纳入 `agent/evals/`。 | `agent/app/activities/`、`agent/app/providers/` | 进行中（私钥文件配置、Activity 与 OpenRouter 免费鉴权探测已在模拟 HTTP 和本机 Temporal 验证；Go/Python 共享示例契约已接入，其他适配器、真实凭据与评测待实施） | `29aee2e6`、`614231dc`、`b65ef335`、`f2a7c4bb`、`5a40fe73` |
| E-07-05 | 前端 | 供应商列表：名称、区域、凭据状态（末 4 位、最后测试结果与时间）、模型数量；凭据对话框为密码输入框，保存后不可查看。 详见 [DES-10 §6](docs/design/10-供应商凭据.md#6-界面) | `frontend/src/features/admin/` | 待办 | — |
| E-07-06 | 测试与验收 | 单元：封装 / 解封；`secret` 按适配器 schema 校验。 集成：停用后新报价失败、进行中任务继续查询；日志与响应敏感词扫描；验收用例 TC-07-01～04（[TST-02](docs/test/02-需求追踪矩阵.md)）。 | `backend/tests/`、`agent/tests/`、`frontend/tests/` | 进行中（封装/解封、Go→Agent 互通、Agent 免费测试模拟 HTTP/Temporal、Go 工作流与本机 PostgreSQL 测试已通过；TC-07 与真实供应商验收未实施） | `2630afc0`、`28c77036`、`f2a7c4bb` |

**E-07-01 数据底座切片（2026-09-27）**：按 DES-02 §5.3 创建 `catalog.provider` 与 `catalog.provider_credential`，数据库部分唯一索引限制每供应商最多一个启用凭据；领域校验供应商状态、限额和凭据元数据，密文字段不参与 JSON 序列化。仓储用供应商行锁串行化替换，在同一事务中停用旧凭据并插入新凭据，支持修订号条件更新、启用凭据查询与停用。先以缺少 `catalog` 包确认 Red；隔离本机 PostgreSQL 中验证重复启用被拒绝、替换失败回滚、两次并发替换后仍只有一个启用凭据、停用供应商不再提供新提交凭据；迁移 up/down 均通过并删除测试库。全量 `go test -race ./... -count=1`、`go vet ./...`、`golangci-lint run ./...`、gofmt/goimports 通过；`govulncheck ./...` 无可达漏洞，另有一个未调用模块告警。此切片尚未实现公钥封装、管理员实时鉴权、审计与 Outbox；仓储仅接收已封装密文，不能将其视为完整凭据保存命令或 TC-07 验收。

**E-07-01 真实依赖 CI 门禁（2026-09-28）**：在独立 PostgreSQL 库只应用 Outbox、账号与供应商凭据迁移；本机 Race 用例验证领域校验、密文不参与 JSON、数据库单活约束、并发替换，以及登记、修改、保存、停用、详情读取在管理业务事务入口对当前管理员的复核和事件回滚。九项用例通过，并新增 CI 显式运行步骤，避免默认全量 Race 因缺少 `LV_TEST_CATALOG_DB_DSN` 而跳过。原始仓储方法仍供适配器内部组合与测试使用；管理业务应继续只接入带权限复核的事务方法。此处仅完成 E-07-01 数据与领域范围，公开接口、真实供应商联调、缓存失效和 TC-07 验收仍归后续任务。

**E-07-04 / E-07-06 凭据封装切片（2026-09-27）**：按 DES-07 §5.2 固定 `wrapped_dek || nonce || ct` 格式与两个 UUID 原始字节 AAD。Go 端仅用 Agent RSA 公钥封装 JSON secret；Python Agent 用对应私钥解封，拒绝错密钥标识、错凭据 ID、篡改密文、弱密钥及无效 JSON。先确认缺少 Go 模块与 Python 依赖的 Red；本机 `LV_TEST_CREDENTIAL_INTEROP=1 go test -race ./tests/catalog -run 'TestCredentialSeal|TestGoCredentialEnvelopeOpensInAgent' -count=1` 通过，并在 Agent CI 中加入跨语言互通步骤。`go test -race ./...`、`go vet ./...`、`golangci-lint run ./...`、Agent 的 `ruff`、`mypy`、`pytest` 通过；`govulncheck ./...` 无可达漏洞，另有一个未调用模块告警。密钥仅在测试中临时生成，未接入管理员保存命令、Agent 配置与 Activity；适配器 schema、真实供应商连通性、日志/响应扫描和 TC-07-01～04 均未验收。

**E-07-02 / E-07-03 内部凭据保存切片（2026-09-27）**：按 DES-10 §3 与 DES-12 §5 为方舟、MiniMax、OpenRouter 的凭据字段建立严格校验；保存命令只返回 ID、标签、末 4 位、状态与时间，公钥封装后才交给仓储。供应商读取与写入事务均在 PostgreSQL 复核当前管理员；替换旧凭据、新密文、`catalog.credential_changed.v1` 和 `audit.recorded.v1` 同事务提交。先以缺少校验适配器确认 Red；隔离本机 PostgreSQL 中执行 Race 测试，验证单活与旧记录保留、Outbox 无明文、审计只含 `last4`、审计写入失败整体回滚，以及并发撤权先提交后命令拒绝；测试库已删除并确认不存在。全量 Go Race、vet、golangci-lint、格式检查通过；`govulncheck` 无可达漏洞，另有一个未调用依赖模块告警。尚未接入运行时公钥配置、公共 HTTP 鉴权/幂等/OpenAPI、供应商其他管理命令、CredentialTestWorkflow、缓存消费者和真实 Agent 测试；E-07-02 / 03 仍在进行中。

**E-07-04 Agent 免费凭据测试切片（2026-09-27）**：补足 Activity 输入中的 `provider_id`、`adapter_key` 和 Base64 密文契约，保证 Agent 可用 AAD 验证供应商身份且不靠业务 `provider_key` 猜适配器。OpenRouter 经官方 `GET /api/v1/key` 免费接口测试，使用固定 HTTPS 地址、禁止跳转与环境代理、5 秒网络超时，只返回 `ok`、`auth_failed`、`unreachable`、`timeout`；火山方舟与 MiniMax 在兼容的免费鉴权接口核实前返回 `unsupported`。先以缺少 Activity 模块确认 Red；模拟 HTTP 验证只发起 GET、无请求体、无敏感响应透传，错误输入和错 AAD 不出站；本机 Temporal 中验证 `agent` 队列实际路由，并复跑原模拟供应商流程，修复了新增 HTTP 客户端导入影响 Temporal 沙箱的回归。Worker 从根目录 `.env` 指向的绝对 PEM 文件读取私钥，未配置时 Activity 返回不可用。`uv run --locked ruff check .`、`ruff format --check .`、`mypy app tests` 与 `pytest` 通过（43 通过、4 条依赖环境的用例默认跳过）；两条本机 Temporal 用例单独通过。未读取真实供应商凭据或执行真实外网验收。Go 工作流契约、真实服务测试、跨端状态落库、离线评测与 TC-07 仍待实施。

**E-07-03 Go 凭据测试工作流切片（2026-09-27）**：`flow` Worker 注册 `CredentialTestWorkflow`，先由 Go 数据库 Activity 复核当前管理员、启用供应商和指定启用凭据，再将含 Base64 密文的输入送到 `agent` 队列；Agent Activity 限时 15 秒且不重试。安全结果回到 Go 后，再次复核权限与凭据，并在一个 PostgreSQL 事务内更新结果和写入变更、审计两条 Outbox 事件。测试 ID 派生稳定事件 ID，旧结果不覆盖较新结果，旧凭据和撤权账号不能回写。Go/Python 共用 Activity JSON 示例；工作流 TestSuite 验证调用顺序与 Agent 失败后无写入；隔离本机 PostgreSQL 验证重放幂等、旧测试与凭据替换拒绝、审计仅含 `last4`、事件写入失败回滚、撤权后读取与写入拒绝。公共接口启动工作流、缓存失效消费者、真实供应商联调和 TC-07 验收仍待实施，因此 E-07-03 保持进行中。

**E-07-02 内部凭据停用切片（2026-09-27）**：停用命令要求规范 UUID 请求 ID 和精确的启用凭据 ID，前后两次复核管理员当前权限；即使供应商已停用，也允许管理员撤销其仍启用的凭据。PostgreSQL 对供应商、凭据按固定顺序加锁，将凭据状态、`catalog.credential_changed.v1` 与只含 `last4` 的 `credential.disabled` 审计 Outbox 在同一事务提交。先以缺失命令类型确认 Red；单元测试检查安全摘要、事件白名单与非法调用，隔离本机 PostgreSQL 的 Race 测试检查停用后无启用凭据、重复停用拒绝、审计写入失败整体回滚，以及读后撤权时写入阶段拒绝。公共接口、持久幂等回执、缓存失效消费者、停用后的报价与在途查询验收仍待实施。

**E-07-02 内部供应商登记切片（2026-09-27）**：应用命令限制供应商 key、名称、初始状态、限额及已声明的适配器；PostgreSQL 在同一事务中重新核对管理员当前权限，插入供应商并写 `provider.created` 审计 Outbox。审计只含稳定 key、适配器、区域、状态与两个限额。先以缺少命令类型确认 Red；单元测试覆盖未知适配器、非法调用与审计字段白名单；隔离本机 PostgreSQL Race 测试验证唯一 key、审计写入失败回滚供应商、撤权后拒绝及事件解析。DES-03 的 IF4～IF6 就绪评审仍未完成，本切片仅为内部命令；公共路由、持久幂等、供应商修改及查询待实施。

**E-07-02 内部供应商修改切片（2026-09-27）**：先按 DES-10 §3 在 DES-03 事件目录中补充 `catalog.provider_changed.v1`，避免把供应商配置变化误记为凭据变化。命令仅允许状态、并发与速率变更，要求规范 UUID 请求 ID、有效修订号和实际变化；读取与写入事务分别复核当前管理员，写入锁定供应商并复核修订号。状态和限额、只含供应商 ID 与新修订号的变更事件、只含前后状态和限额的 `provider.updated` 审计事件同事务提交。先以缺少命令类型确认 Red；单元测试检查安全事件与非法请求，隔离本机 PostgreSQL Race 测试覆盖审计写入失败回滚、读取后并发修订冲突、读取后撤权，以及停用供应商后启用凭据不可再读取。公共路由、持久幂等回执、缓存消费者、供应商安全查询与 TC-07 验收仍待实施。

**E-07-02 内部供应商详情切片（2026-09-27）**：按 DES-10 §3 提供仅管理员可读的详情查询；PostgreSQL 在同一事务复核当前角色并锁定供应商，读取当前启用凭据时只选择 ID、标签、末 4 位、状态、最后测试类别与时间及时间戳，不选择密文和密钥 ID。结果包含方舟、MiniMax、OpenRouter 的凭据输入字段声明，未知适配器拒绝推断。先以缺少查询类型确认 Red；单元测试覆盖字段声明、无密文字段响应及非法调用；隔离本机 PostgreSQL Race 测试覆盖无凭据、启用凭据与读前撤权。公共 GET 路由和响应契约、供应商列表与模型数量、TC-07 验收仍待实施。

**E-07-02 内部供应商列表切片（2026-09-27）**：基于 E-08-01 的模型表，按 `(create_time DESC,id DESC)` 有界游标分页；PostgreSQL 在同一事务复核当前管理员并一次查询供应商配置、启用凭据安全摘要及未删除模型数量。停用供应商仍列出，停用模型仍计数；凭据查询不选择密文与密钥 ID。先以缺少列表查询类型确认 Red；单元测试覆盖分页边界与非法调用，隔离本机 PostgreSQL Race 测试覆盖跨页顺序、真实模型数量、停用状态、安全响应和撤权。公共 GET 路由与正式游标编码、持久幂等及 TC-07 验收仍待实施。

#### E-08 模型注册表与价格

- **需求**：[REQ-08](docs/requirement/08-模型注册表与价格.md)（ADM-01 模型注册表管理；CST-04 价格表；GEN-09 模型能力驱动的参数界面）　**设计**：[DES-11](docs/design/11-模型注册表与价格.md)　**依赖**：E-07
- **验收**：TC-08-01～05（5 条）
- **待确认**：DES-11-Q1、DES-11-Q2、DES-11-Q3、DES-11-Q4（默认方案见设计文档，确认前按默认实施）

| 任务 | 内容 | 怎么做 | 涉及文件 | 状态 | 提交 |
| --- | --- | --- | --- | --- | --- |
| E-08-01 | 数据与领域模型 | 迁移建表 / 加列：`catalog.capability`、`catalog.model_profile`、`catalog.model_profile_version`、`catalog.price_rule_version`；实现领域对象、状态机与仓储（组织 / 平台级，按管理员权限访问）。详见 [DES-11 §3](docs/design/11-模型注册表与价格.md#3-数据) | `backend/db/migrations/`、`backend/internal/catalog/domain/`、`backend/internal/catalog/adapter/postgres/` | 完成（迁移、领域校验与状态机、管理员复核和只追加写入仓储已验证；管理用例与审计属 E-08-02） | — |
| E-08-02 | 用例与接口 | 实现查询接口：GET /api/models；swag 注解生成 OpenAPI。详见 [DES-11 §4](docs/design/11-模型注册表与价格.md#4-接口) | `backend/internal/catalog/application/`、`backend/internal/catalog/adapter/http/`、`backend/docs/` | 进行中（管理员登记、版本 / 价格发布、启停命令及项目模型查询读侧已验证；公共 HTTP 契约、接口与 OpenAPI 待实施） | — |
| E-08-03 | 异步、工作流与事件 | 工作流 / Activity / 定时任务 `OperationWorkflow`；事件 `catalog.model_changed.v1`、`catalog.price_changed.v1`（Outbox → Kafka，消费者按事件 ID 去重）。要点：无。模型版本中的 `queue` 与 `supports_*` 被 OperationWorkflow 读取（DES-04）。 详见 [DES-11 §5](docs/design/11-模型注册表与价格.md#5-异步与工作流) | `backend/internal/catalog/adapter/workflow/`、`backend/internal/catalog/adapter/event/` | 待办 | — |
| E-08-04 | 前端 | - 管理 · 模型注册表：列表（模型、供应商、能力、区域、状态、当前版本、价格）；详情编辑器（JSON 编辑 + 表单预览）；版本差异对比。 - 生成面板：`ModelParamsForm` 组件按 `param_schema` 渲染；`ReferenceLimitBar` 显示各用途用量 / 上限… 详见 [DES-11 §7](docs/design/11-模型注册表与价格.md#7-界面) | `frontend/src/features/admin/` | 待办 | — |
| E-08-05 | 测试与验收 | 单元：`param_schema` 与 `limits` 校验器（前后端共用同一套 JSON Schema 规则）；价格计算（按张、按秒、按 token、按字符、分辨率系数）。 集成：发布版本 → 缓存失效 → 报价使用新版本。 前端：各组件类型渲染与校验的快…；验收用例 TC-08-01～05（[TST-02](docs/test/02-需求追踪矩阵.md)）。 | `backend/tests/`、`frontend/tests/` | 待办 | — |

**E-08-01 模型注册表迁移切片（2026-09-27）**：按 DES-02 §5.3 与 DES-11 §3 建立能力、模型、模型版本和价格版本四表。约束当前版本只能归属同一模型、版本号唯一且为正、价格单位与 JSON 形状有效、非人民币价格必须有正汇率；为供应商模型计数建立索引。先在缺表的隔离 PostgreSQL 中确认 Red，再验证模型版本归属、重复版本和缺汇率拒绝，迁移 up/down 往返成功。

**E-08-01 领域与写入仓储切片（2026-09-27）**：能力、模型头、模型版本与价格版本具备领域校验；模型头按修订号附加当前版本并启停，启用要求当前版本和已生效价格。PostgreSQL 仓储在每次平台级读写时复核当前管理员；追加版本在模型行锁内检查预期修订、连续版本号和能力模式，仅插入历史版本并原子推进模型头。隔离本机 PostgreSQL 验证了无价启用拒绝、跨能力模式拒绝、旧版本与旧价格保留、重复版本和撤权拒绝。发布前的完整参数 / 价格规则校验、版本与价格发布审计及公共接口属于 E-08-02，尚未验收。

**E-08-02 管理员登记模型切片（2026-09-27）**：登记用例校验管理员、模型键、供应商与能力、展示名和请求 ID；新模型默认停用并以修订号 1 创建。仓储在同一 PostgreSQL 事务复核当前管理员、拒绝已逻辑删除的供应商或能力，并写入安全摘要的 `model.created` 审计 Outbox；审计失败回滚模型。隔离本机 PostgreSQL 验证成功落库、重复键、来源删除、审计失败回滚及撤权拒绝。公共 HTTP 契约依赖 DES-03 IF4–IF6 就绪评审；版本 / 价格发布、完整发布校验、启停和查询接口仍待实施。

**E-08-02 模型版本配置校验切片（2026-09-27）**：新增 Draft 2020-12 结构 Schema、Go 校验器和共享 JSON 向量，覆盖七种组件、模式、输入用途限制、字段唯一、数值精度、默认值范围及滑块步长；拒绝重复 JSON 属性与前端对象保留字段。校验器已在后续发布切片接入；前端尚未用 Ajv 消费同一 Schema 和测试向量，不能报告前后端共用校验完成。

**E-08-02 模型版本发布切片（2026-09-27）**：发布命令先校验管理员和版本配置；PostgreSQL 同一事务复核当前权限、锁定模型头，校验预期修订、连续版本号、能力模式及输入用途，只追加版本并推进当前指针，写入仅含安全标量的 `model.version_published` 审计 Outbox。移除无审计的旧导出版本追加方法。隔离本机 PostgreSQL Race 测试验证两版历史保留、真实校验拒绝、修订和能力冲突、审计失败整体回滚及撤权。价格发布、启停与公共查询接口仍待实施；DES-03 IF4–IF6 未确认前不注册公共路由，TC-08-01～05 未计完成。

**E-08-02 价格规则发布切片（2026-09-27）**：先明确 DES-11 中各计价单位的完整规则、系数精度、版本化汇率、生效时刻与未来价格缓存边界；价格校验器拒绝缺字段、未知或重复 JSON 属性、无效系数与不安全金额。发布命令与 PostgreSQL 单事务复核当前管理员、模型当前版本、修订、连续价格版本号及系数对应的模式 / 分辨率，只追加价格并写入安全摘要的 `price.published` 审计 Outbox；移除无审计的导出追加方法。隔离本机 PostgreSQL Race 测试覆盖未来价不能提前启用、两版历史、非法规则零写入、审计失败回滚及撤权。公共查询与参数系数仍待实施；DES-03 IF4–IF6 未确认前不注册公共路由，TC-08-01～05 未计完成。

**E-08-02 模型启停审计切片（2026-09-27）**：管理员命令按预期修订号执行 `disabled ⇄ active`；PostgreSQL 同一事务复核当前管理员、锁定模型头，启用时要求当前未删除版本与已生效价格，状态及修订更新与安全摘要的 `model.enabled` / `model.disabled` 审计 Outbox 原子提交。移除无审计的导出启停方法。隔离本机 PostgreSQL Race 测试覆盖无版本、无价格、仅未来价格、修订冲突、重复启用、审计失败回滚及撤权。公共查询、HTTP 接口、报价读侧校验、在途任务与缓存失效仍待各自切片验证；DES-03 IF4–IF6 未确认前不注册公共路由，TC-08-01～05 未计完成。

**E-08-02 项目模型查询读侧切片（2026-09-27）**：应用层限制登录角色、项目 ID、游标和页长；PostgreSQL 在同一事务复核当前用户、组织与未归档项目，从项目读取境外开关，按能力、当前版本模式和区域过滤。停用模型、停用供应商、无凭据及尚无已生效价格的模型保留可见；只投影当前版本、已生效最新价格和凭据安全状态，按模型键与 ID 稳定分页。隔离本机 PostgreSQL Race 测试验证跨组织与撤权拒绝、区域 / 模式过滤、同生效时刻价格版本优先、未来价排除、分页和凭据脱敏。公开 `available` / `unavailable_reason`、HTTP / OpenAPI 及 TC-08-01～05 仍待 DES-03 IF4～IF6 就绪评审和 M1-06 接口底座，不计完成。

#### E-09 审计日志

- **需求**：[REQ-09](docs/requirement/09-审计日志.md)（ADM-02 审计日志查询；REQ-01 G6 审计约定）　**设计**：[DES-12](docs/design/12-审计日志.md)　**依赖**：—
- **验收**：TC-09-01～03（3 条）
- **待确认**：DES-12-Q1、DES-12-Q2、DES-12-Q3、DES-12-Q4（默认方案见设计文档，确认前按默认实施）

| 任务 | 内容 | 怎么做 | 涉及文件 | 状态 | 提交 |
| --- | --- | --- | --- | --- | --- |
| E-09-01 | 数据与领域模型 | 迁移建表 / 加列：`audit.audit_log`、`infra.processed_event`；实现领域对象、状态机与仓储（`audit.audit_log`：`project_id` 可空，按用户与组织授权，带项目时再按项目过滤；`infra.processed_event`：组织 / 平台级，按管理员权限访问）。详见 [DES-12 §3](docs/design/12-审计日志.md#3-数据) | `backend/db/migrations/`、`backend/internal/audit/domain/`、`backend/internal/audit/adapter/postgres/` | 完成（数据层与真实受限登录契约；本机既有配置及部署账号切换仍待运维验证） | `eb1f0289`、`f639d9c8`、`091ae31f`、`8c6d9a38`、`9214b55b`、本次提交 |
| E-09-02 | 用例与接口 | 实现查询接口：GET /api/admin/audit-logs、GET /api/admin/audit-logs:export；swag 注解生成 OpenAPI。详见 [DES-12 §4](docs/design/12-审计日志.md#4-接口) | `backend/internal/audit/application/`、`backend/internal/audit/adapter/http/`、`backend/docs/` | 进行中（管理员权限实时复核的内部查询已实现；公开接口、导出及查询审计待实施） | `95f152b0`、`f67d2cd0` |
| E-09-03 | 异步、工作流与事件 | 事件 `audit.recorded.v1`（Outbox → Kafka，消费者按事件 ID 去重）。要点：audit 消费者（`backend-relay`）只消费 `audit.recorded.v1`：命令层对 REQ-09 R1 列出的每个动作（登录与账号变更、凭据、注册表与价格、预算、报价确认、取消、人工核对处理、所… 详见 [DES-12 §5](docs/design/12-审计日志.md#5-异步与工作流) | `backend/internal/audit/adapter/workflow/`、`backend/internal/audit/adapter/event/` | 进行中（正式 Relay 审计消费已联调；`user.created` 已经真实 Kafka 落审计，内部登录与登出的审计 Outbox 已在 PostgreSQL/Redis 验证；月分区维护及独立维护账号的手工双表预备命令已验证；定时调度、三年归档、调用明细维护和其余动作覆盖待实施） | `091ae31f`、`bfd63e5c`、`ac6e6733`、`e5d2a27d`、`d88890b6`、`83613bd2`、`1c516bad`、`1c5eeb5a` |
| E-09-04 | 前端 | 筛选栏（时间范围、操作人、项目、对象类型、动作）+ 虚拟滚动表格 + 详情抽屉（前后值差异）。 详见 [DES-12 §7](docs/design/12-审计日志.md#7-界面) | `frontend/src/features/admin/` | 待办 | — |
| E-09-05 | 测试与验收 | 命令覆盖测试：遍历 R1 中每个命令，断言产生对应审计记录。 消费者幂等：重复投递同一事件只产生一条记录；验收用例 TC-09-01～03（[TST-02](docs/test/02-需求追踪矩阵.md)）。 | `backend/tests/`、`frontend/tests/` | 待办 | — |

**E-09-01 审计建表切片（2026-09-27）**：先以缺失表运行集成测试确认 Red；`eb1f0289` 建立 `audit.audit_log` 当前 UTC 月及未来三个月分区和默认分区，触发器拒绝 `UPDATE` / `DELETE`，约束拒绝以 `is_delete=true` 写入。隔离 PostgreSQL 临时库中，Race 测试验证分区路由、拒绝修改的 SQLSTATE `42501`、逻辑删除约束错误 `23514`，迁移 down 后 schema 消失；临时库已删除。全量 Go Race、vet、golangci-lint、格式检查通过；`govulncheck` 0 个可达漏洞，另有 1 个未调用模块告警。`infra.processed_event` 已在 M1-08 建立；尚未配置非表所有者的应用数据库账号与仅 `INSERT` / `SELECT` 的 ACL，领域仓储、消费者、查询接口、审计动作覆盖与 3 年归档清理均未实现，E-09-01 和 E-09 均不计完成。

**E-09-01 组织隔离查询仓储切片（2026-09-27）**：`f639d9c8` 增加审计领域记录、组织必填的详情与列表仓储，以及 `(org_id, create_time DESC, id DESC)` 索引。先以缺失包编译失败确认 Red；在本机隔离 PostgreSQL 临时库中执行两次迁移，Race 集成用例验证跨组织不可见、项目/操作人/动作/时间筛选、游标分页和相同时间戳的分页无遗漏；数据在测试事务回滚，迁移按逆序回滚，临时库已删除。全量 Go Race、vet、golangci-lint、格式检查通过；`govulncheck` 为 0 个可达漏洞，另有 1 个未调用模块告警。管理员鉴权、应用数据库 ACL、事件写入与完整接口仍待实施；30 天范围 P95 指标尚未压测，E-09-01 不计完成。

**E-09-01 应用数据库角色权限切片（2026-09-27）**：数据库管理员须在迁移前预建非登录、非超级用户的 `lanverse_app` 角色；权限迁移向其授予审计 schema `USAGE` 与父表 `INSERT, SELECT`。先运行新增测试确认 ACL 迁移缺失的 Red；随后在本机隔离 PostgreSQL 临时库执行 Race 集成测试，受限角色经父表插入并读取当前月及新建未来分区记录，`UPDATE`、`DELETE`、`TRUNCATE`、`ALTER` 与取得表所有权均被拒。测试事务回滚后，审计 schema 和测试创建的角色均不存在，临时库已删除。全量 Go Race、vet、golangci-lint 与格式检查通过；`govulncheck` 无可达漏洞，另有 1 个未调用模块告警。当前仓库定义了角色授权和部署顺序，但本机与部署环境的实际应用登录凭据尚未切换并做进程级验证，E-09-01 不计完成。

**E-09-01 真实受限登录验收（2026-09-28）**：补充逐表运行时权限迁移，审计父表继续仅有 `INSERT, SELECT`。在本机独立已迁移 PostgreSQL 库，以真实非所有者 LOGIN 继承 `lanverse_app`，并使用该 DSN 调用审计仓储、管理员授权查询和正式 Relay；审计写入及 `infra.processed_event` 标记同事务提交，重复 Kafka 事件只留下各一行。测试验证 `session_user=current_user`、非超级用户且不继承表所有者、跨组织拒绝及审计 `UPDATE`、`DELETE`、`TRUNCATE`、`ALTER` 均被 SQLSTATE `42501` 拒绝。回滚新授权后，管理员读取测试因 `identity` schema 权限不足而按预期失败；重新应用迁移后 Race 测试通过。CI 增加独立数据库和受限登录专用门禁。E-09-01 数据层工程契约完成；本机既有 `.env` 与生产 `LV_DB_DSN` 切换、完整 E-09 功能和 TC-09 验收仍需分别验证。

**E-09-03 审计消费切片（2026-09-27）**：`091ae31f` 增加 `audit.recorded.v1` 的事件解析与字段策略，Kafka Record Handler 将审计行和 `infra.processed_event` 标记放在同一 PostgreSQL 事务；未登记动作或摘要字段、凭据字段、预签名 URL 与超长文本被拒绝。先以缺失包编译失败确认 Red；隔离本机 PostgreSQL 临时库中，Race 集成用例验证重复事件只写一条、审计写入冲突时去重标记回滚、无项目动作按组织 ID 路由；单元用例覆盖敏感字段与错误路由，临时库已清理。全量 Go Race、vet、golangci-lint、格式检查通过，`govulncheck` 为 0 个可达漏洞，另有 1 个未调用模块告警。当前字段策略仅在测试中登记示例 `budget.changed`；真实业务动作的字段清单、命令层 Outbox 写入、Kafka 进程挂载、真实 Kafka 联调及 100% 动作覆盖均未完成，E-09-03 不计完成。

**E-09-03 本机 Kafka 联调（2026-09-27）**：`bfd63e5c` 添加显式启用的本机 Kafka 集成用例。确认 `lanverse.audit.recorded.v1` 主题原先不存在后创建，向本机 Broker 发布一条审计事件；同一消费组首次处理故障不提交 offset，重启后由真实 audit Handler 写入一条审计行和一条去重标记，再次启动未重放已提交记录。`go test -race ./tests/audit -run TestAuditKafkaRetriesThenCommitsAfterDatabaseWrite -count=1 -v` 通过（约 9 秒测试执行）；测试主题和隔离 PostgreSQL 临时库随后删除并确认不存在。全量 Go Race、vet、golangci-lint、格式检查通过，`govulncheck` 无可达漏洞、另有 1 个未调用模块告警。此证据覆盖本机 Kafka 客户端与 Handler，不代表正式 `backend-relay` 进程已挂载消费者，也不代表业务命令已生产审计事件。

**E-09-03 审计月分区维护切片（2026-09-27）**：`ac6e6733` 固定默认分区记录搬迁、失败回滚、受限应用角色拒绝 DDL、重复维护及追加保护契约；`e5d2a27d` 增加由审计表所有者调用的维护方法。方法在一个事务中锁定父表，创建当前 UTC 月和未来三个月的缺失分区，先将目标月份记录从默认分区迁入再挂载；失败时回滚表结构、记录和触发器状态。隔离 PostgreSQL 临时库执行 Race 集成用例和全量 `go test -race ./...`、`go vet ./...`、`golangci-lint run ./...`，均通过；`govulncheck ./...` 无可达漏洞，另有 1 项未调用模块告警。测试事务回滚后临时库无审计 schema 或测试角色。此方法尚未接入独立高权限连接和 Temporal `partition-maintain` Schedule；三年归档、归档后删除、调用明细分区维护、跨连接并发及实际阻塞时长尚未验证，E-09-03 不计完成。

**E-09-03 独立账号分区预备命令（2026-09-27）**：`d88890b6` 增加独立维护连接、角色隔离、双表事务回滚及审计跨连接并发契约；`83613bd2` 接入 `lanverse partitions ensure`，校验应用与维护账号连到同库不同角色，应用账号不能拥有表或超级用户权限，维护账号须拥有 Outbox 和审计父表。命令在同一事务中预备两表当前 UTC 月及未来三个月分区。隔离本机 PostgreSQL 中，审计并发维护 Race 用例通过；双表用例连续运行两次并清理新增记录与分区，验证任一方失败整体回滚、默认分区记录完整搬迁与重复执行；CLI 实际调用通过。全量 `go test -race ./...`、`go vet ./...`、`golangci-lint run ./...`、格式检查通过，`govulncheck ./...` 无可达漏洞，另有 1 项未调用模块告警。该入口须由运维按月手动执行；三年冷归档、归档后删除、调用明细分区维护、完整 `partition-maintain` Schedule、生产账号隔离及写入阻塞时长评估尚未完成，E-09-03 和 M1-08 均不计完成。

**E-09-03 分区维护登录身份复核（2026-09-27）**：`1c516bad` 将集成用例改为真实受限登录账号，并加入超级用户登录后切换应用角色的拒绝断言；旧校验下该断言失败。`1c5eeb5a` 同时核验 `session_user` 和 `current_user`，修正后同一隔离 PostgreSQL 库的 Race 用例连续两次通过，实际 CLI 调用通过。全量 Go Race、vet、golangci-lint、格式检查通过；`govulncheck` 无可达漏洞，另有 1 项未调用模块告警。分区任务未因该权限修正变为完成。

**E-09-03 正式 Relay 审计消费接线（2026-09-27）**：DES-12 登记 M1 首批账号动作的摘要字段白名单；正式 `backend-relay` 在 Outbox 与实时投影循环之外，使用独立 `lanverse-audit` 消费组处理 `audit.recorded.v1`。`must_change_password` 仅允许布尔值，登录失败原因只接受声明过的内部原因码，其余密码字段继续拒绝。先运行新增进程级用例确认 Red：Outbox 已发布、审计行和消费标记未落库；接线后在隔离 PostgreSQL 临时库及本机 Kafka、Redis 上执行 `go test -race ./tests/app -run '^TestRelayRole' -count=1 -v`，三例通过，覆盖 Outbox → Kafka → 审计写入与标记、原实时投影、无效信封失败退出。`go test -race ./tests/audit -run 'TestAuditParserRejectsUnsafeSummaries|TestIdentityAuditActionPolicy' -count=1 -v` 通过；合并角色在同样隔离依赖加本机 Temporal、MinIO 的 `TestAllRoleRunsAndStopsOnLocalServices` 通过。全量 `go test -race ./...`、`go vet ./...`、`golangci-lint run ./...`、gofmt/goimports 与 Wire 重生成一致性检查通过；`govulncheck ./...` 无可达漏洞，另有 1 个未调用模块告警。测试后临时库及两条测试主题已删除并确认不存在。该切片当时尚无业务命令生产审计 Outbox；后续管理员创建账号切片已补上 `user.created`，其他动作仍待实施。

**E-09-02 管理员审计读取切片（2026-09-27）**：新增内部查询用例，对组织与操作人 ID、时间范围、游标和单页上限进行校验；PostgreSQL 在同一读取事务中复核操作人属于该组织、仍为启用且已完成首次登录的管理员，并以 `FOR SHARE` 锁定账号行。先以缺失查询用例确认编译失败的 Red，再在本机隔离 PostgreSQL 临时库执行 Race 集成测试，验证项目筛选、游标分页、跨组织详情不可见、降权 / 禁用 / 删除后的拒绝，以及另一连接撤权与读取事务互斥；临时库已删除。全量 Go Race、vet、golangci-lint 与格式检查通过；`govulncheck` 无可达漏洞，另有 1 个未调用模块告警。此切片只提供内部应用查询能力，公开查询 / 导出接口、查询行为审计、OpenAPI 与 30 天范围 P95 验收尚未完成，E-09-02 不计完成。

#### E-10 项目管理

- **需求**：[REQ-10](docs/requirement/10-项目管理.md)（PRJ-01 创建项目；PRJ-02 项目列表与概览；PRJ-06 归档与删除；PRJ-07 生成策略设置）　**设计**：[DES-13](docs/design/13-项目管理.md)　**依赖**：E-06、E-08　**联调**：E-11（后实施，先按契约与模拟实现联调，对方完成后重跑本 Epic 验收）
- **验收**：TC-10-01～06（6 条）
- **待确认**：DES-13-Q1、DES-13-Q2、DES-13-Q3（默认方案见设计文档，确认前按默认实施）

| 任务 | 内容 | 怎么做 | 涉及文件 | 状态 | 提交 |
| --- | --- | --- | --- | --- | --- |
| E-10-01 | 数据与领域模型 | 迁移建表 / 加列：`billing.budget`、`workspace.project`、`workspace.style_preset`；实现领域对象、状态机与仓储（`billing.budget`：仓储查询强制带 `project_id`；`workspace.project`：按当前用户组织授权并以项目 ID 过滤，MVP 无项目成员；`workspace.style_preset`：`project_id` 可空，按用户与组织授权，带项目时再按项目过滤）。详见 [DES-13 §2](docs/design/13-项目管理.md#2-数据) | `backend/db/migrations/`、`backend/internal/workspace/domain/`、`backend/internal/workspace/adapter/postgres/` | 完成（三表迁移、领域状态机、项目与零预算原子创建、组织授权仓储已验证；接口由 E-10-02 实现） | `2705c9f9`、`afb9b55c` |
| E-10-02 | 用例与接口 | 写操作经命令层（鉴权、幂等键、`expected_revision`、审计、Outbox）实现：POST /api/projects、GET /api/projects/{pid}/overview、DELETE /api/projects/{pid}；swag 注解生成 OpenAPI。详见 [DES-13 §3](docs/design/13-项目管理.md#3-接口) | `backend/internal/workspace/application/`、`backend/internal/workspace/adapter/http/`、`backend/docs/` | 进行中（内部创建、组织授权列表查询及四项设置修改命令已验证；公共接口、持久幂等、概览、其余设置和生命周期命令待实施） | `80e54fbf`、`620c66e8` |
| E-10-03 | 异步、工作流与事件 | 工作流 / Activity / 定时任务 `project-purge`；事件 `workspace.project_changed.v1`（Outbox → Kafka，消费者按事件 ID 去重）。要点：- `project-purge`（每日）：对 `purge_after < now()` 的项目执行分批清理工作流（删除对象存储前缀、删除各 schema 数据），可断点续跑。 - 概览物化：`realtime` 之外… 详见 [DES-13 §4](docs/design/13-项目管理.md#4-异步与工作流) | `backend/internal/workspace/adapter/workflow/`、`backend/internal/workspace/adapter/event/` | 进行中（创建和修改事件的 Relay → Redis → 项目 SSE 投影已验证；清理工作流、概览消费者和生命周期事件生产方待实施） | `850e8ce7`、`74aab306`、`133f1941` |
| E-10-04 | 前端 | 项目列表（卡片 / 表格切换、状态筛选、回收站视图）；新建项目对话框（风格类型切换后显示子风格与预设缩略图）；项目概览矩阵（单元格点击跳转）；设置页（画幅与风格只读并说明原因）。 详见 [DES-13 §6](docs/design/13-项目管理.md#6-界面) | `frontend/src/features/project/` | 进行中（已验证仅接收外部项目数据的卡片 / 表格展示组件；状态筛选、回收站查询、页面与接口接入、创建和设置交互待实现） | `59e56649`、`758d0985` |
| E-10-05 | 测试与验收 | 单元：概览阶段计算；生命周期状态机。 集成：清理工作流中断后续跑；触发器拒绝修改画幅；验收用例 TC-10-01～06（[TST-02](docs/test/02-需求追踪矩阵.md)）。 | `backend/tests/`、`frontend/tests/` | 待办 | — |

**E-10-01 项目数据底座切片（2026-09-27）**：依 DES-02/REQ-10 建立组织内风格预设、项目和每项目唯一预算三表，数据库固定 `1080p`、默认禁止境外模型、限制风格组合与预算余额，并以触发器禁止修改项目画幅和风格类型。领域校验覆盖中文名称长度、画幅、风格、分辨率及状态。隔离本机 PostgreSQL Race 测试验证约束与可修改字段，迁移 down/up 往返验证；项目与默认 0 预算的同事务创建、跨组织预设授权、项目读取及公共接口尚未实施。此数据底座为 E-08-02 按项目过滤模型的前置条件，TC-10-01～06 未计完成。

**E-10-01 仓储与状态机验收（2026-09-27）**：项目与默认 0 预算在同一 PostgreSQL 事务提交，预算唯一键冲突时项目回滚；预设选择核对组织、项目范围与风格，读取预设时返回提示词、负面提示词与参考素材 ID。项目、预算和预设仓储在同一事务重查账号及组织可用状态；预算查询必须带项目 ID 并联查项目归属。领域状态机覆盖归档只读、进行中任务阻止归档/删除、软删除保留原状态及 30 天内恢复。先以缺失状态方法、仓储和预设内容字段取得 Red；隔离本机 PostgreSQL 工作区 Race 集成测试、全仓库 Go Race、`go vet`、`golangci-lint` 与格式检查通过，`govulncheck` 无可达漏洞。公开项目接口、审计与 Outbox、在途任务查询及状态机持久化由 E-10-02 实现；TC-10-01～06 尚未计完成。

**E-10-02 内部创建项目命令切片（2026-09-27）**：命令校验制作者或管理员、项目规格及请求 ID，生成固定 1080p、默认关闭境外模型的新项目；仓储在单事务复核当前账号、组织和预设，提交项目、默认 0 预算、`project.created` 审计与 `workspace.project_changed.v1` Outbox。审计只含已声明的安全规格，两个事件按项目 ID 分区；旧的无审计创建入口已移除。隔离本机 PostgreSQL Race 测试覆盖成功、跨组织 / 撤权 / 无效预设拒绝、审计写入失败整体回滚和事件篡改拒绝。公共 `POST /api/projects`、持久幂等、概览、生命周期及 TC-10-01～06 尚未验收；创建事件的实时消费者见 E-10-03，DES-03 IF4～IF6 未确认前不注册公开路由。

**E-10-02 内部项目列表查询切片（2026-09-27）**：查询按当前账号与组织授权，默认排除已删除项目；状态与回收站筛选、名称字面搜索及 `update_time DESC, id DESC` keyset 分页均由隔离本机 PostgreSQL Race 测试验证，包含跨组织不可见、撤权拒绝和同时间戳翻页。公开列表路由、游标编码及浏览器验收仍待 DES-03 IF4～IF6 / M1-06，生命周期命令的进行中任务持久化检查仍待任务数据层落地。

**E-10-02 内部项目设置修改切片（2026-09-27）**：命令以 `expected_revision` 修改名称、描述、风格预设和境外模型开关；同一 PostgreSQL 事务重新核对当前账号、组织、项目状态与修订号、预设范围，提交项目修订及 `project.updated` 审计、`workspace.project_changed.v1`（`change=updated`）双 Outbox。审计只保留修订号、变化的预设和境外开关，以及名称 / 描述是否变化的布尔标记；无实际变化不写事件。Red 由缺失命令、修订冲突错误取得；隔离本机 PostgreSQL Race 测试覆盖成功、并发修订冲突、跨组织 / 撤权 / 归档 / 回收拒绝、预设校验、篡改事件拒绝及审计插入失败整体回滚。默认模型、AIGC 标识样式、公开 PATCH 持久幂等和 TC-10 验收仍待后续任务。

**E-10-03 项目变更实时投影切片（2026-09-27）**：正式 Relay 订阅 `workspace.project_changed.v1`，核对项目分区键、信封与修订号后按事件 ID 去重，向项目 Redis Stream / Pub/Sub 发布仅含 `project_id`、`revision`、`change` 的 `project.updated`。先以缺少投影处理器的失败用例确认 Red；隔离本机 PostgreSQL、Kafka、Redis 上的 Race 集成用例从真实创建项目命令验证两条 Outbox 投递、审计落库、实时消费标记、Redis Stream 与测试授权的 SSE 帧。重复外部副作用可使同一事件 ID 在 Stream 中再次出现；断线补读改从首次匹配的 ID 续传，避免漏掉中间的新事件，并由 Redis 用例验证。公开项目 SSE 路由仍待身份与项目授权接入，项目列表创建成功后须主动失效查询；`project-purge`、概览消费者与生命周期事件生产方仍待实施，E-10-03 不计完成。

**E-10-03 修改事件真实链路验证（2026-09-27）**：在原创建事件集成用例中接续执行项目设置修改命令，核对第二组双 Outbox 不含新名称和描述，Relay 发布及审计 / 实时消费者标记、`project.updated` 审计落库、Redis Stream 补读与测试授权 SSE 第二帧均对应修订号 2、`change=updated`，投影 data 精确只有三个安全字段。隔离本机 PostgreSQL、Kafka、Redis 的定向 Go Race 用例通过；临时数据库已删除并确认不存在。公开 SSE 路由鉴权、项目生命周期事件、概览消费者与清理工作流仍待完成。

**E-10-04 项目列表展示组件切片（2026-09-27）**：基于 DES-13 §6 和项目无边框视觉规范，实现仅接收外部数据的卡片 / 语义表格组件，受控切换视图，展示进行中、已归档、回收中状态和真实空列表；名称长文本可换行，按钮支持键盘焦点。先以缺少组件确认 Red，再以定向 Vitest 4 条用例、ESLint、Prettier 与 TypeScript 检查确认 Green。组件尚未连接页面、公开项目查询或真实会话，状态筛选、回收站数据、新建对话框、概览和设置页仍待实现；本切片不计 E-10-04 或 TC-10 完成。

#### E-11 项目预算

- **需求**：[REQ-11](docs/requirement/11-项目预算.md)（PRJ-03 项目预算）　**设计**：[DES-14](docs/design/14-项目预算.md)　**依赖**：E-10　**联调**：E-21、E-32（后实施，先按契约与模拟实现联调，对方完成后重跑本 Epic 验收）
- **验收**：TC-11-01～04（4 条）
- **待确认**：DES-14-Q1、DES-14-Q2、DES-14-Q3（默认方案见设计文档，确认前按默认实施）

| 任务 | 内容 | 怎么做 | 涉及文件 | 状态 | 提交 |
| --- | --- | --- | --- | --- | --- |
| E-11-01 | 数据与领域模型 | 迁移建表 / 加列：`billing.budget`、`billing.ledger_entry`（其中 `billing.budget` 由 E-10 建表，本 Epic 只加列或复用）；实现领域对象、状态机与仓储（仓储查询强制带 `project_id`）。详见 [DES-14 §2](docs/design/14-项目预算.md#2-数据) | `backend/db/migrations/`、`backend/internal/billing/domain/`、`backend/internal/billing/adapter/postgres/` | 完成（隔离 PostgreSQL 迁移、只追加权限、项目范围与原子仓储已验证） | `7e3ba601`、`26e129aa`、`825829f5`、`ec634a77`、`0ca81f16` |
| E-11-02 | 用例与接口 | 写操作经命令层（鉴权、幂等键、`expected_revision`、审计、Outbox）实现：GET /api/projects/{pid}/budget、PUT /api/projects/{pid}/budget；swag 注解生成 OpenAPI。详见 [DES-14 §3](docs/design/14-项目预算.md#3-接口) | `backend/internal/billing/application/`、`backend/internal/billing/adapter/http/`、`backend/docs/` | 进行中（内部预算调整命令与原子事务已验证；公开路由、持久幂等和 OpenAPI 待 DES-03 IF4～IF6 / M1-06） | `ec634a77`、`0ca81f16` |
| E-11-03 | 异步、工作流与事件 | 事件 `billing.budget_changed.v1`、`billing.budget_low.v1`、`billing.budget_overrun.v1`（Outbox → Kafka，消费者按事件 ID 去重）。要点：无；预留与结算在 `flow` 队列 Activity 的数据库事务中执行（DES-04）。 详见 [DES-14 §4](docs/design/14-项目预算.md#4-异步与工作流) | `backend/internal/billing/adapter/workflow/`、`backend/internal/billing/adapter/event/` | 进行中（预算调整与结算的余额事实、低余额和超支 Outbox 已接入；预算实时投影有本机跨边界证据；完整工作流与 E-33 通知消费仍待验收） | `5213b3e3`、`52bcfb9e` |
| E-11-04 | 前端 | 设置页预算卡片（上限、已结算、已预留、可用、使用率进度条）；报价对话框显示剩余预算与差额；顶部低余额横幅。 详见 [DES-14 §6](docs/design/14-项目预算.md#6-界面) | `frontend/src/features/project/` | 进行中（纯属性预算卡片与状态测试已完成；设置页挂载、真实预算 API 与全局横幅待 E-11-02 公开接口） | `35c485b7`、`dffe11cf` |
| E-11-05 | 测试与验收 | 并发确认测试（race）；结算超支路径；阈值通知去重；验收用例 TC-11-01～04（[TST-02](docs/test/02-需求追踪矩阵.md)）。 | `backend/tests/`、`frontend/tests/` | 待办 | — |

**E-11-01 预算与账本数据底座切片（2026-09-27）**：复用 E-10 的项目默认 0 预算，新增 DES-02 §5.10 的 `billing.ledger_entry` 迁移、只追加触发器和应用角色列级权限；预算领域校验最低可设金额、整数溢出与超支恢复，账本和预算仓储读取强制核对当前账号、组织及项目范围。补明 `budget_change` 的正负号口径（新上限减旧上限）。先以缺失领域包确认 Red，再以隔离本机 PostgreSQL 迁移、应用角色权限、跨组织读取和 Go Race 测试确认数据底座；预算调整事务在下一切片补齐，TC-11 尚未计完成。

**E-11-02 内部预算调整事务切片（2026-09-27）**：命令校验用户与组织、项目、规范请求 ID、预算修订号和最低可设额度；无变化不写，变更时在项目与预算锁下原子更新额度、写入正负分录及 `billing.budget_changed.v1` / `audit.recorded.v1` 双 Outbox。审计摘要只包含额度、修订号和超支标记。先以缺失应用包确认 Red；隔离本机 PostgreSQL Race 用例验证升降额、并发仅一方成功、归档项目拒绝及 Outbox 插入失败时三表一起回滚；全量 Go 测试、vet、lint 与漏洞扫描通过。公开 GET/PUT 路由、持久幂等响应、实时消费、低余额及结算超支事件仍待实施，TC-11 不计完成。

**E-11-03 预算调整低余额事件切片（2026-09-27）**：严格比较可用额与上限 20%，对 0 上限不告警，使用整数计算避免大金额乘法溢出；仅由不低于阈值跨入低余额时发 `billing.budget_low.v1`。事件与额度修订、账本、变更事件和审计在同一 PostgreSQL 事务写入；低额 Outbox 插入失败则整体回滚。隔离本机 PostgreSQL Race 用例、真实 Outbox Relay → Kafka 消费验证通过。此处只生产预算调整事件事实；结算触发低额与超支事件的后续实现见 M1-08 预算与结算实时切片。同日通知去重与实际投递仍待 E-33，TC-11 不计完成。

**E-11-04 预算卡片组件切片（2026-09-27）**：新增无边框预算卡片，展示额度、已结算、已预留、可用额和占用率；默认 0 额度显示未设置，低于 20% 显示余额提示，超支优先展示并保留负可用额。复用报价金额格式化，组件只接收预算快照，不预设尚未确认的公开接口；组件测试及前端类型、Lint、格式和构建门禁通过。设置页尚未挂载，报价对话框已有的差额提示与未来预算快照仍需真实联调，全局低额横幅未完成；TC-11 不计完成。

#### E-21 报价与二次确认

- **需求**：[REQ-21](docs/requirement/21-报价与二次确认.md)（GEN-01 生成前报价；GEN-11 付费生成二次确认；CST-03 预留与结算）　**设计**：[DES-24](docs/design/24-报价与二次确认.md)　**依赖**：E-08、E-10、E-11　**联调**：E-25、E-32（后实施，先按契约与模拟实现联调，对方完成后重跑本 Epic 验收）
- **验收**：TC-21-01～06（6 条）
- **待确认**：DES-24-Q1、DES-24-Q2、DES-24-Q3、DES-24-Q4（默认方案见设计文档，确认前按默认实施）

| 任务 | 内容 | 怎么做 | 涉及文件 | 状态 | 提交 |
| --- | --- | --- | --- | --- | --- |
| E-21-01 | 数据与领域模型 | 迁移建表 / 加列：`billing.budget`、`billing.ledger_entry`、`billing.reservation`、`operation.batch`、`operation.operation`、`operation.operation_input`（其中 `billing.budget` 由 E-10 建表，本 Epic 只加列或复用、`billing.ledger_entry` 由 E-11 建表，本 Epic 只加列或复用）；实现领域对象、状态机与仓储（`billing.budget`、`billing.ledger_entry`、`billing.reservation`、`operation.batch`、`operation.operation`、`billing.budget`、`billing.ledger_entry`：仓储查询强制带 `project_id`；`operation.operation_input`：无 `project_id`，经父对象外键继承项目范围校验）。详见 [DES-24 §3](docs/design/24-报价与二次确认.md#3-数据) | `backend/db/migrations/`、`backend/internal/operation/domain/`、`backend/internal/operation/adapter/postgres/` | 完成（迁移、领域状态、项目隔离读取及报价快照原子创建；确认和终态写入归 E-21-02/03） | `920b0431`、`51007ed8` |
| E-21-02 | 用例与接口 | 写操作经命令层（鉴权、幂等键、审计、Outbox；`expected_revision` 按 DES-03 的 `Rev` 标记）实现：POST /api/projects/{pid}/quotes、POST /api/batches/{id}:confirm、POST /api/operations/{id}:confirm；swag 注解生成 OpenAPI。详见 [DES-24 §4](docs/design/24-报价与二次确认.md#4-接口) | `backend/internal/operation/application/`、`backend/internal/operation/adapter/http/`、`backend/docs/` | 进行中（内部 `free` 单项及批量报价已覆盖提示词、媒体输入与项目默认模型；单项与批量确认、复用源复核和持久幂等已实现；其余目标、公开接口与 OpenAPI 待实施） | — |
| E-21-03 | 异步、工作流与事件 | 工作流 / Activity / 定时任务 `OperationWorkflow`、`BatchWorkflow`、`flow.SettleOperation`、`quote-expiry`；事件 `operation.confirmed.v1`（单项）、`batch.confirmed.v1`（批量）（Outbox → Kafka，消费者按事件 ID 去重）。要点：确认事务提交后启动 `OperationWorkflow`（单项）或 `BatchWorkflow`（批量；`agent_session` 例外，不启动工作流，见 DES-04 §8.7）；工作流型操作的结算由工作流最后… 详见 [DES-24 §5](docs/design/24-报价与二次确认.md#5-异步与工作流) | `backend/internal/operation/adapter/workflow/`、`backend/internal/operation/adapter/event/` | 进行中（单项与真实批次确认事件的 Kafka→Temporal 启动、批次父工作流本机运行与 Worker 重启后取消恢复、复用零费结算、报价过期 Schedule 及非 mock 用量计价已验证；真实供应商适配、人工正费用判定和全链路验收待实施） | — |
| E-21-04 | 前端 | `QuoteConfirmDialog` 组件（REQ-05 §4.1）：逐项费用（可展开）、合计、剩余预算、倒计时、区域提示、错误项列表与“剔除”操作；确认按钮不响应回车；按钮文案“生成（约 ¥X）”由 `useQuote` 钩子统一计算。 详见 [DES-24 §7](docs/design/24-报价与二次确认.md#7-界面) | `frontend/src/features/operation/` | 进行中（共享组件框架；待真实接口、入口接入与 Playwright） | `46bf86fd` |
| E-21-05 | 测试与验收 | 单元：计价（各计价单位、系数）、`input_hash` 规范化、报价失效判断。 集成：确认事务（预算约束、并发）；工作流启动失败恢复；复用路径。 故障注入：确认后立即重启 API 与 Worker，任务仍被执行一次；验收用例 TC-21-01～06（[TST-02](docs/test/02-需求追踪矩阵.md)）。 | `backend/tests/`、`frontend/tests/` | 进行中（计价、指纹、单项确认并发及复用零费路径已有单元 / 本机 PostgreSQL Race 证据；启动恢复与 TC-21 全链路待实施） | — |

**E-21-01 数据底座切片（2026-09-27）**：新增批次、操作、冻结输入和预留表，外键约束同项目批次、复用与预留身份；实现批次状态、报价有效性、预留结算计算，以及按当前账号、组织、项目读取的仓储。隔离本机 PostgreSQL 执行迁移 up/down/up；集成测试覆盖跨项目关联拒绝、跨组织与已删除项目不可见、撤销账号拒绝读取。`go test -race ./...`、`go vet ./...`、`golangci-lint run ./...` 通过；`govulncheck ./...` 无可达漏洞，另有 1 个未调用模块告警。条件写入、同事务预留与结算、真实报价确认及 TC-21 尚未完成；DES-03 IF4–IF6 未决前不注册公开接口。

**E-21-01 完成补充（2026-09-27）**：报价快照仓储在同一事务创建单项或批次的 `quoted` Operation 与全部冻结输入；真实 PostgreSQL 验证批次金额、项目写权限、输入冲突时整笔回滚，未确认时预留和账本仍为空。预算领域补齐安全的 Reserve/Settle 金额与超支状态计算；纯计价规则覆盖五种已发布计价单位、模式与分辨率系数、汇率和单次向上取整。CI 新增迁移后的 Operation PostgreSQL 契约测试。跨表确认事务、Outbox 与终态结算仍归 E-21-02/03，尚无 TC-21 全链路验收。

**E-21-02 确认写权限切片（2026-09-28）**：新增最小运行角色迁移，允许 `lanverse_app` 写预留、确认字段与批次状态；隔离本机 PostgreSQL 按全量迁移重现权限拒绝，应用迁移后测试通过，回滚后再次拒绝，重新应用后 Race 测试通过。测试同时验证未授予修改报价金额和删除预留的权限。CI 的 Operation 契约库会应用全部迁移并运行此测试；公开确认 API、同事务预算预留 / 审计 / Outbox、幂等与 OpenAPI 尚未实现。PLN-18 要求 `expected_revision`，DES-24 请求体和当前表无该字段，契约决策待确认；本任务不计完成。

**E-21 报价输入指纹切片（2026-09-28）**：修正 REQ-25 / PRD-22 / DES-02、04、28 的指纹定义，加入输出数量、冻结文本与遮罩媒体摘要，避免不同实际请求错误复用。应用层按已发布参数字段规范化默认值、键序与精确数值，按输入顺序计算带版本标记的 SHA-256；省略 seed 的请求标为不可复用，并用随机标记与显式默认 seed 区分。先以缺失实现确认 Red，再以 `go test -race ./tests/operation -run '^TestInputHash' -count=1 -v` 验证等价、不等价、随机和非法输入。指纹尚未接入公开报价命令或真实复用查询，TC-21 / TC-25 均不计完成。

**E-21 输入素材身份修正（2026-09-28）**：冻结输入可直接使用媒体或遮罩素材 ID；此前哈希只包含其内容摘要，两个不同素材 ID 在摘要相同时会错误碰撞。REQ-25 / PRD-22 / DES-02、28 已明确直接素材 ID 也属于输入身份，哈希序列化同步纳入 `media_asset_id`、`mask_asset_id`；两种碰撞先以失败测试复现，再由 `go test -race ./tests/operation -run '^TestInputHash' -count=1` 验证通过。尚未接入报价命令，E-21-02/05 仍进行中。

**E-21 复用候选读侧切片（2026-09-28）**：报价仓储新增按当前账号、活跃项目与 `input_hash` 查询同项目已完成操作的能力；要求至少有一个未删除输出，全部输出及其媒体仍可用，排除后续审核拒绝或媒体未就绪的候选。先以缺失方法确认 Red，再在应用全部迁移的隔离本机 PostgreSQL 库运行 `go test -race ./tests/operation -count=1`，覆盖跨组织不可见、输出与媒体审核、运行角色 `lanverse_app` 权限和账号撤权；`go vet ./...`、`golangci-lint run ./...`、格式检查通过。读侧尚未接入报价命令，确认时的二次可用性检查与复用输出写入仍待实现，E-21-02/05 未完成。

**E-21 确认失效规则切片（2026-09-28）**：应用层按报价截止时刻、目标版本、当前模型与价格版本、授权撤销和复用源可用性生成稳定有序的失效原因；`agent_session` 无单模型 / 价格快照的例外也已覆盖。先以缺失类型和方法确认 Red，`go test -race ./tests/operation -run '^TestQuoteFreshness' -count=1 -v` 通过。该规则只处理已读取的事实，尚未在确认事务 A/B 中持锁重读目标、模型、价格与授权；预留、审计、Outbox 和公开 API 均未接入，E-21-02/05 不计完成。

**E-21 报价目录现状读取切片（2026-09-28）**：Operation 仓储按当前账号、组织和活跃项目读取报价所引用模型的当前版本及生效价格，校验模型、供应商、唯一活跃凭据、模式和项目境外策略；停用或不可用时返回空 ID 供确认失效规则判定。读取时对项目、模型、供应商及凭据加共享锁，并与价格发布使用的模型行锁协调；不可变版本表保持只读，兼容最小权限的 `lanverse_app` 运行角色。先以缺失方法确认 Red，再在全量迁移的隔离本机 PostgreSQL 库通过 `go test -race ./tests/operation -count=1`，覆盖版本与生效时间变化、停用、境外策略、跨组织与账号撤权；只向测试进程传入该隔离库 DSN 后，`go test -race ./... -count=1` 全部通过。`go vet ./...`、`golangci-lint run ./...` 通过，`govulncheck ./...` 无可达漏洞（1 个未调用模块告警），测试库已清理。当前方法仅供预检，确认事务 B 必须持有自己的写事务并重读同一事实；目标版本、授权和复用源的锁内复核及预算预留等写入仍待实现，E-21-02/05 不计完成。

**E-21-02 原子预留写入切片（2026-09-28）**：Billing 新增供确认协调器在调用方事务中使用的预留方法：锁项目预算、按领域规则拒绝余额不足或超支、创建单项 held 预留、更新预算修订并追加 reserve 账本分录；零费用也保留预留与账本事实。先以缺失类型和方法确认 Red，隔离本机 PostgreSQL 的 `go test -race ./tests/billing -count=1` 覆盖顺序与并发争用、重复操作不双重占用、调用方回滚、零费用及最小权限 `lanverse_app`；仅向测试进程传入该隔离库 DSN 后，`go test -race ./... -count=1` 全部通过。`go vet ./...`、`golangci-lint run ./...` 通过，`govulncheck ./...` 无可达漏洞（1 个未调用模块告警）。该方法不单独提交事务，Operation 确认协调器仍须持锁复核报价、在同一事务关联预留并写审计及 Outbox；批量确认、公开接口、幂等与 OpenAPI 未完成，E-21-02 不计完成。

**E-21 复用源确认复核切片（2026-09-28）**：对报价冻结的 `reused_from_id`、同项目 `input_hash`、已完成状态及全部输出进行锁内复核；媒体输出还要求当前素材仍就绪且审核通过，结构化输出要求审核通过。复核在调用方确认事务中锁源 Operation、输出和素材，避免检查后审核状态变化或新输出插入。先以缺失方法确认 Red；隔离本机 PostgreSQL 的 Race 测试覆盖源状态、输入指纹、输出与媒体失效、跨组织和账号撤权、`lanverse_app` 运行角色，并用并发锁超时证实外层事务提交前无法修改素材或追加输出。`go test -race ./tests/operation -count=1` 和仅传入该隔离库 DSN 的 `go test -race ./... -count=1` 通过；`go vet ./...`、`golangci-lint run ./...` 通过，`govulncheck ./...` 无可达漏洞（1 个未调用模块告警）。该方法独立调用只提供预检；确认事务仍须持锁读取目标版本与授权，并在同事务完成预算预留、状态、审计及 Outbox，E-21-02/05 不计完成。

**E-21-02 内部单项确认事务切片（2026-09-28）**：在同一 PostgreSQL 事务锁定当前用户、活跃项目及 `quoted` Operation，复核模型、价格、复用源与冻结素材，锁预算并预留、记账，然后写 `confirmed` 状态、工作流 ID、`operation.confirmed.v1` 与经过字段白名单校验的审计 Outbox。预算等待后再次按当前时间核对过期与生效价格；失效报价在预留前置为 `expired`，预留等待期间才失效的报价回滚并留待后续失效处理。先以缺少命令与仓储方法确认 Red；全量迁移的隔离本机 PostgreSQL `go test -race ./tests/operation -count=1` 覆盖成功、重复确认、余额不足、跨组织 / 撤权、模型失效、运行角色、Outbox 写入失败整体回滚及并发预算争用。仅向测试进程传入该隔离库 DSN 的 `go test -race ./... -count=1`、`go vet ./...`、`golangci-lint run ./...` 通过；`govulncheck ./...` 无可达漏洞（1 个未调用模块告警）。当前内部路径只接受无目标版本的 `free` 单项；目标版本表与真人授权未就绪时拒绝确认。批量确认、持久幂等、`expected_revision` 决策、公开 API / OpenAPI 与工作流启动补偿仍未实现，E-21-02 不计完成。

**E-21-02/03 复用零费完成切片（2026-09-28）**：确认后的复用项由 `OperationWorkflow` 调用 `flow.CompleteFromReuse`，事务内再次锁定同项目、相同输入指纹且已完成的来源，要求输出数量与序号完整、输出及媒体仍可用；真人素材在授权表未就绪前拒绝复用。新 `operation_output` 行保留原媒体素材 ID，不复制对象；零费预留、账本结算与 `completed` 状态及事件原子提交，重放不重复写入。来源在确认后失效时不复制输出，工作流按零费用失败结算，不调用供应商。先以缺失 Activity 和完成方法确认 Red；再以真实本机 PostgreSQL 的 Operation Race 测试覆盖媒体与结构化输出复制、来源事后失效、账本 / Outbox、`lanverse_app` 运行角色与重放，Temporal 工作流测试覆盖正常及失效分支，候选查询以失败测试修正了输出不完整及真人素材未核验仍可入选的问题。全量迁移的隔离库仅传入 Operation 测试 DSN 后 `go test -race ./... -count=1` 通过；`go vet ./...`、`goimports -l`、`golangci-lint run ./...` 通过，`govulncheck ./...` 无可达漏洞（1 个未调用模块告警），测试库已清理。真实 Temporal 的确认事件启动与补偿、批量确认、公开接口和 TC-21 全链路尚未验收，E-21-02/03/05 均不计完成。

**E-21-03 单项启动补偿切片（2026-09-28）**：`relay` 在配置 Temporal 时消费 `operation.confirmed.v1`，验证事件与已提交的单项 Operation、组织 / 项目和预留身份一致后，以固定 `operation/{id}` 启动 `OperationWorkflow`；Temporal 拒绝重复 ID 时视为已启动，消费标记只在启动成功后提交。每分钟扫描超过两分钟仍为 `confirmed` 的单项，分页重试遗漏事件，明确排除批内项和 `agent_session`。缺失适配器时的测试先确认 Red；单元 Race 测试、全量迁移隔离本机 PostgreSQL 的仓储测试、本机 Temporal 对运行中及已终止工作流 ID 的重复启动测试均通过。隔离库仅传入 Operation 测试 DSN 的 `go test -race ./... -count=1` 全部通过；`go vet ./...`、`goimports -l`、`golangci-lint run ./...` 通过，`govulncheck ./...` 无可达漏洞（1 个未调用模块告警）。真实 Kafka→Temporal 端到端启动、Temporal 故障后的进程级恢复、批次启动与报价过期清理尚未验证；E-21-03 仍进行中，未提交。

**E-21-03 真实启动与报价过期切片（2026-09-28）**：独立测试库中的单项确认事务产生 Outbox，真实本机 Kafka 消费后在本机 Temporal 创建固定 `operation/{id}`，PostgreSQL 消费标记持久化且重放不新建执行；本机 Kafka 禁止自动建主题，测试预建了 `lanverse.operation.confirmed.v1`，运维文档同步列入预建清单。`quote-expiry` 每五分钟由 Temporal Schedule 触发 `flow.ExpireQuotes`，按固定截止时刻分批将过期报价置为 `expired`，全部子项过期后关闭批次；确认项与预留不变。先以缺少仓储方法确认 Red；全量迁移的隔离 PostgreSQL 对批量分页、运行角色与 up/down/up 索引通过，本机 Temporal 的 Schedule 安装、重装与真实定时执行通过。整包 Operation Race 测试第一次暴露共享测试库污染，已将 Kafka 链路测试改用独立测试库、过期样本固定在历史截止时刻；修正后以两个全量迁移隔离库运行 `go test -race ./... -count=1`，所有 Go 包通过。`go vet ./...`、`goimports -l`、`golangci-lint run ./...` 通过，`govulncheck ./...` 无可达漏洞（1 个未调用模块告警），临时测试库已清理。批次确认事件 / BatchWorkflow、Temporal 故障重启补偿与 TC-21 尚未完成，E-21-03 不计完成。

**E-21-03 批次启动边界切片（2026-09-28）**：`batch.confirmed.v1` 的事件结构、项目与组织校验、确认批次及已预留子项资格查询、`batch/{id}` 固定 ID 启动器和独立批次补偿扫描已实现；失效或用户剔除的 `expired` 子项不进入确认数量。`OperationWorkflow` 校验父工作流身份，拒绝把批内项作为独立工作流启动。单元测试先 Red，隔离全量迁移 PostgreSQL、Operation Race 测试包、全量 Go Race 测试、`go vet`、`goimports` 和 `golangci-lint` 通过，批次索引迁移 up/down/up 通过。批次事件订阅与补偿尚未接入进程：必须先实现并注册 `BatchWorkflow`，避免把事件投递给未注册的工作流；批量确认事务、批次执行与故障恢复仍待完成，本项未提交。

**E-21-03 批次工作流快照切片（2026-09-28）**：父工作流用 `batch_id` 从数据库重载确认集合，而不是信任事件中的子项列表；快照排除保留批次关联的 `expired` 项，校验确认数量、预留、固定子项工作流 ID 与状态。`confirmed → running` 写入具备幂等重试语义。先以缺失仓储方法确认 Red；全量迁移隔离 PostgreSQL 对剔除项、未预留项、运行角色权限及状态重试通过，Operation Race 测试、`go vet`、`goimports`、`golangci-lint` 通过。并发名额、父子工作流、取消、熔断、批次完成事件及重启恢复仍待实施，E-21-03 未完成且未提交。

**E-21-03 批次并发名额切片（2026-09-28）**：新增 `operation.batch_launch` 持久名额，按项目行锁与供应商事务锁序列化批内申请；同一 Operation 重试只占一格，项目批内活动名额上限 20，并以供应商 `concurrency_limit` 限制批内申请。子项终态结算与名额释放在同一事务，非终态不得释放；供应商成本证据缺失导致的结算回滚不释放名额。隔离 PostgreSQL 的跨项目并发 Race 测试重复 5 次仅一个子项取得限额为 1 的供应商名额；运行角色申请与结算、迁移 up/down/up、全量 Operation 真实数据库 Race 测试已验证。当前名额只覆盖批内启动，单项工作流的在途额度尚未统一纳入供应商并发，因此本切片不能证明全局供应商限额；父工作流与端到端故障恢复仍待实现，本项未提交。

**E-21-03 父子工作流与完成事件切片（2026-09-28）**：`BatchWorkflow` 已以数据库确认集合为输入，先申请持久名额，再用固定 `operation/{id}` 与 `ParentClosePolicy=ABANDON` 启动子工作流；子工作流 Future 回调与数据库快照共同推进，已存在子工作流按固定 ID 重载，名额不足时等待，每启动 100 项 ContinueAsNew。`flow.FinishBatch` 只在确认集合全部终态且名额释放后，原子更新批次成功 / 失败计数并写一次 `batch.finished.v1`，重放不重复投递。Temporal 测试环境验证真实子工作流的父身份、容量等待、重复子工作流、100 项续跑；隔离全量迁移 PostgreSQL 验证完成事件载荷、运行角色和上 / 下 / 上迁移，Operation 全包 Race、`go vet`、`goimports`、`golangci-lint` 通过，`govulncheck` 无可达漏洞（1 个未调用模块告警）。暂停 / 恢复、取消、连续供应商错误熔断、未知结果汇总及进程重启验收尚未完成，父工作流尚未注册到生产 Worker，批次事件订阅也未启用；E-21-03 未完成且未提交。

**E-21-03 批次熔断切片（2026-09-28）**：供应商失败码规范化并与平台错误码区分；父工作流按已结算子项终态顺序检测连续 5 个相同供应商码，`flow.PauseBatch` 将原因持久化并阻止后续名额申请，收到 `resume` 信号后清除暂停。已确认过的终态子项 ID 保存在工作流状态并随 ContinueAsNew 传递，防止恢复后被旧失败再次熔断。先以缺少判定与仓储方法确认 Red；Temporal 测试验证暂停期间不启动子项、恢复后继续，隔离全量迁移 PostgreSQL 的 Operation Race 包验证运行角色暂停 / 恢复与名额约束。全仓 `go test -race ./... -count=1` 首次因错误导出全部 `.env` 变量干扰配置测试而失败，以只传测试库地址的干净环境重跑后全部通过（身份测试耗时约 109 秒）；`go vet ./...`、`goimports -l`、`golangci-lint run ./...` 通过，`govulncheck ./...` 无可达漏洞（1 个未调用模块告警），临时测试库已清理。暂停通知消费者、取消、未知结果汇总、进程重启和真实批次链路仍待完成；父工作流尚未接入生产 Worker，E-21-03 未完成且未提交。

**E-21-03 批次取消意图与零费排队项切片（2026-09-28）**：新增 `cancel_requested_at`，父工作流接收 `cancel` 信号后先持久写入，再由同一批次行锁阻止新名额；`confirmed` / `submitting` 且没有供应商调用记录的子项可以零费用取消，预留与活动名额在终态结算事务释放。已经取得名额但尚未调用供应商的子项同样可安全取消；存在调用记录时保留预留和名额，不据此推断未执行。批次在确认集合全部终态后置为 `cancelled`，汇总 Outbox 只写一次。先以缺少方法确认 Red；隔离 PostgreSQL 的运行角色、重放、三种调用边界和全 Operation Race 测试通过，迁移 `070` 上 / 下 / 上及运行角色列权限通过；Temporal 测试覆盖取消信号先落库和排队项继续推进。`go vet ./...`、`goimports -l`、`golangci-lint run ./...` 通过，`govulncheck ./...` 无可达漏洞（1 个未调用模块告警）。已提交供应商的子工作流取消、取消通知 / 审计、未知结果汇总、真实批次链路及进程故障恢复仍待实现；父工作流尚未接入生产 Worker，E-21-03 未完成且未提交。

**E-21-03 已提交子项取消证据切片（2026-09-28）**：批次父工作流向仍在执行的子项按固定工作流 ID 发送一次 `cancel` 信号，并随 ContinueAsNew 保存已发送 ID；子工作流在已知任务 ID 且供应商声明支持取消时，先进入 `cancelling` 并写持久取消调用记录，再发 `provider.cancel`。供应商明确取消时按已核定成本结算；未生效时回到 `submitted` 继续查询；结果未知时进入对账并保留预留。仓储限制取消调用必须处于 `cancelling`、绑定已记录的任务 ID 且供应商支持取消。Temporal 测试先 Red，覆盖已确认取消、未生效后继续查询和父子信号去重；隔离全量迁移 PostgreSQL 验证未确认取消不能零费结算，确认后预留与名额一起释放；Operation 全包 Race、`go vet` 通过。真实供应商的用量定价、取消通知 / 审计、未知结果汇总、Temporal Update 契约、真实批次链路及进程故障恢复仍待实施；父工作流尚未接入生产 Worker，E-21-03 未完成且未提交。

**E-21-03 批次生产接线与本机恢复切片（2026-09-28）**：正式 `flow` Worker 注册 `BatchWorkflow`、持久批次 Activity 与排队子项取消结算；Relay 同时订阅 `operation.confirmed.v1`、`batch.confirmed.v1`，每分钟对两类已确认对象分别补偿扫描。独立 PostgreSQL 测试库、本机 Temporal、Kafka 和 Redis 验证：正式 Worker 对取消批次写一次终态结算并释放预留；正式 Relay 将合成批次确认 Outbox 经 Kafka 启动固定 `batch/{id}`，重复启动保持同一运行记录；遗漏事件的旧确认批次由扫描启动；父工作流暂停后停止 Worker，停机期间持久写入取消意图，重启 Worker 后完成取消且无供应商调用。缺少注册的本机 Temporal 测试先 Red。完整 Operation Race 测试首轮暴露共享测试库残留 Outbox 导致未知主题，已将 Relay 用例移入专用隔离测试库，复跑通过；批量确认事务尚未产出真实 `batch.confirmed.v1`，因此本轮仅证明下游合成事件链路。真实批次含已发出供应商调用的完整执行、未知结果汇总、通知与审计、实际费用核定和端到端故障注入仍待完成；E-21-03 未完成且未提交。

**E-21-02 内部批量确认事务切片（2026-09-28）**：新增 `ConfirmBatchQuote` 内部命令与 PostgreSQL 两阶段确认：独立事务持久化用户剔除和失效项，第二事务锁批次及有效子项、复核报价、逐项预留和记账，原子更新批次确认集合及唯一的 `batch.confirmed.v1` 与审计 Outbox。预算不足时第二事务回滚；第二事务新发现的失效项由独立事务重新复核并持久化。运行角色新增批次 `workflow_id` 列写权限。隔离本机 PostgreSQL 的 Race 测试覆盖选中确认、剔除、部分及全部过期、预算不足整批回滚、已有过期项和运行角色；批内项不生成单项确认事件。当前仅支持与内部单项确认一致的 `free` 目标；目标版本与真人授权读取、持久幂等、`expected_revision`、公开 HTTP / OpenAPI 和真实批次上游到下游全链路验收仍待实施，E-21-02 不计完成。

**E-21-02/03 真实批次事件联调切片（2026-09-28）**：将原先手工合成批次事件的 Relay 测试改为先运行真实 `ConfirmBatchQuote`，从已提交的 Outbox 读取 `batch.confirmed.v1`，再由正式 Relay 通过本机 Kafka 启动 Temporal 固定 `batch/{id}` 工作流；再次启动不生成新运行记录。全量迁移的独立 PostgreSQL 测试库配合本机 Redis、Kafka、Temporal，通过 `go test -race ./tests/operation -run '^TestProductionRelayStartsConfirmedBatchThroughLocalKafka$' -count=1 -v`。用例证明确认事件到工作流启动，尚未证明批内供应商调用、完整终态结算或前端验收，E-21-02/03 仍未完成。

**E-21-03 真实确认批次取消联调切片（2026-09-28）**：正式 Worker 取消用例改由真实批量确认创建预留、子项与批次事件；测试先将确认批次领取为运行中并持久记录取消意图，再在本机 Temporal 启动正式 `BatchWorkflow`，验证子项零费用取消结算、预留释放和批次 `cancelled` 终态。独立 PostgreSQL 全量迁移库通过 `go test -race ./tests/operation -run '^TestProductionFlowWorkerCancelsBatchOnLocalTemporal$' -count=1 -v`。随后用两个独立的全量迁移 PostgreSQL 库和本机 Kafka、Redis、Temporal 运行完整 `go test -race ./tests/operation -count=1` 通过；需要 Agent Worker 与 MinIO 的真实供应商用例未提供对应测试开关，因此本轮未执行。本取消用例明确从取消意图进入流程，不覆盖正常供应商提交后的费用核定，E-21-03 仍未完成。

**E-21-02 确认幂等切片（2026-09-28）**：新增只追加的 `operation.confirmation_request`，以当前组织、用户和 UUID 请求键保存指纹与首次结果；成功结果与预留、状态、审计及确认事件同事务写入。批量确认跨独立的失效事务和预留事务时，在同一 PostgreSQL 会话上持有请求键咨询锁；可重放的报价过期、预算不足、状态冲突和目标不可用结果在响应前保存，依赖故障不作为稳定业务结果缓存。相同键更换确认对象或剔除集合返回 `idempotency_key_reused`。隔离 PostgreSQL Race 测试覆盖单项和批量重放、并发同键只预留一次、更换请求体拒绝、预算变化后仍重放首次拒绝；两个隔离全量迁移库及本机 Kafka、Redis、Temporal 下 `go test -race ./... -count=1` 全部 Go 包通过。迁移 up/down/up 与 `lanverse_app` 只读/追加权限通过，`go vet`、`golangci-lint` 和格式检查通过；`govulncheck ./...` 无代码可达漏洞，另有 1 个未调用模块告警。PLN-18 与 DES-24 已按 DES-03 的逐端点 `Rev` 标记明确这三个报价/确认端点不使用 `expected_revision`；该切片结束时，报价命令的幂等、公开 HTTP / OpenAPI、其他目标与完整 TC-21 仍待完成，E-21-02 未提交。

**E-21-02 自由生成报价命令切片（2026-09-28）**：新增内部画布自由生成报价命令，按当前项目、模型版本、启用凭据和生效价格读取事实；服务器校验提示词、模式、发布参数、输出数量、分辨率及比例限制，计算输入指纹和价格，查询同项目可用的已完成结果后原子写入冻结输入与 `quoted` 快照。新增 `operation.quote_request`，以组织、用户和 UUID 请求键保存指纹与首次成功结果，咨询事务锁保证并发同键只创建一次；同键改请求返回 `quote idempotency key reused`，重放前仍检查当前项目访问权限。隔离本机 PostgreSQL 全量迁移及 100 号迁移 down/up 后，`go test -race ./tests/operation -count=1` 通过；测试覆盖报价接现有确认、复用与强制重做、并发重放、权限拒绝和运行角色。`go vet ./...`、`golangci-lint run ./...`、`goimports`、`git diff --check` 通过；`govulncheck ./...` 无可达漏洞（1 个未调用模块告警）。当前仅覆盖显式指定模型、纯文本提示词的 `free` 目标；其他目标解析、完整参数与输入类型、公开 HTTP / OpenAPI 及 TC-21 仍待实现，E-21-02 不计完成，未提交。

**E-21-02 批量自由生成报价切片（2026-09-28）**：在同一内部报价契约下新增 2～150 项的 `free` 纯文本批量报价。逐项保留请求顺序并返回 `model_unavailable` / `input_not_ready` / `price_unavailable`；仅有效项进入同一个 `quoted` 批次，全部无效时不创建批次或 Operation。有效项的冻结输入、批次合计和幂等结果同事务提交，确认前不预留；重放先复核项目访问权限，同键改请求返回冲突，并发同键只创建一个批次。隔离本机 PostgreSQL 全量迁移的 `go test -race ./tests/operation -count=1` 通过，专项覆盖部分有效项接真实 `ConfirmBatchQuote`、全无效结果重放、重复参数请求变更、运行角色、并发及 150 项边界。单次本机 150 项报价耗时约 0.4 秒，仅是该次环境观察，不代表 PERF-10 的 P95 验收；公开 API、其他目标类型、错误展示与 TC-21 仍未完成，E-21-02 不计完成，未提交。

**E-21-02 自由生成媒体输入报价切片（2026-09-28）**：内部单项与批量 `free` 报价现接受有序 `media_asset` 输入，事务中按项目锁定素材并校验就绪、审核、SHA-256 摘要及模型发布的用途、类型、数量、总量、时长和大小限制；真人或附有授权记录的素材在授权表尚不可查询时拒绝报价。素材身份与摘要进入输入指纹，摘要同时冻结于 `operation_input.ref_version`，确认和提交前复核内容未变化。同键更换素材返回冲突，批内异项目素材按 `input_not_ready` 返回且不创建该项。独立全量迁移 PostgreSQL 的 `go test -race ./tests/operation -count=1` 通过；未注入数据库测试变量的全仓 `go test -race ./... -count=1` 也通过，后者不代替跨数据库集成证据。另通过 `go vet ./...`、`golangci-lint run ./...`、`goimports` 与 `govulncheck ./...`（无可达漏洞，1 个未调用模块告警）。`min_resolution` 的供应商格式尚未约定，配置该限制的媒体报价保守拒绝；遮罩、其他目标解析、公开 HTTP / OpenAPI 与 TC-21 仍待实现，E-21-02 不计完成，未提交。

**E-21-02 项目默认模型报价切片（2026-09-28）**：内部 `free` 报价允许不传 `model_key`，在同一事务持锁读取项目 `default_models[capability]`，显式模型优先；批量有效项返回实际选中的 `model_key`。报价请求幂等指纹保留原始请求，因此默认模型变化后同键仍重放原结果，新键使用当前默认模型；缺失或不可用的默认模型返回 `model_unavailable`，显式模型仍可独立报价。先用失败的输入验证测试确认 Red，再在独立全量迁移 PostgreSQL 运行 `go test -race ./tests/operation -count=1` 通过，覆盖单项、批量、默认模型变更后的重放、缺失默认模型和运行角色。`go vet ./...`、`golangci-lint run ./...`、`goimports` 与 `govulncheck ./...` 通过，漏洞扫描无可达漏洞（1 个未调用模块告警）。目标版本模型优先级、其余目标展开、公开接口与 TC-21 仍待完成，E-21-02 不计完成，未提交。

**E-21-02/04 报价费用明细切片（2026-09-28）**：计价函数在同一次精确计算中返回可展示的数量、原币单价、输出数、命中的模式 / 分辨率系数、币种与汇率；按 token 计价返回输入估算、输出上限和各自的千 token 单价。`free` 单项与批量报价将明细写入 Operation 快照并带回命令结果；确认对话框按币种显示六位小数内的微元单价，按次计费不把输出数重复乘入金额。先用失败测试确认原明细缺字段和前端误显，再通过独立全量迁移 PostgreSQL 的 `go test -race ./tests/operation -count=1`，单项与批量集成用例核对结果与持久化明细；前端 `pnpm exec vitest run` 为 8 文件 / 24 用例通过，`tsc`、ESLint、Prettier 和 Go 静态检查通过，`govulncheck ./...` 无可达漏洞（1 个未调用模块告警）。公开报价接口、真实浏览器交互和 TC-21 仍待实施，E-21-02/04 不计完成，未提交。

**E-21-05 工作流历史回放切片（2026-09-28）**：本机 Temporal 的正式 Flow Worker 完成真实确认批次取消，以及暂停后停止 Worker、持久化取消意图并重启恢复；两条用例在完成后读取实际 Temporal 执行历史，由 SDK `WorkflowReplayer` 回放当前 `BatchWorkflow`。单项 `OperationWorkflow` 的本机 Agent 模拟供应商、MinIO、媒体接管联调同样在 Flow Worker 重启后完成，分别对正常完成和提交结果未知后对账的两条真实历史执行回放。三条集成用例均在全量迁移的独立 PostgreSQL 库以 Race Detector 通过，测试库已清理，Agent Worker 已停止。此证据覆盖这四条具体历史，不代表人工核对、批次熔断、真实供应商用量定价或 TC-21-01～06 全链路验收；E-21-05 不计完成，未提交。

**E-21-03 供应商用量结算切片（2026-09-28）**：明确标准化计费用量字段，非 mock 供应商在终态查询或确认取消时按 Operation 冻结的价格版本、模式、参数和汇率精确计价；明确拒绝记零费，缺失或非法用量阻止结算，同一任务的重复终态查询只计费一次且冲突用量被拒绝。取消 Activity 契约可传递已产生用量。计价单元测试先 Red；全量迁移的独立本机 PostgreSQL 中，非 mock 终态费用集成测试先 Red，随后 Operation 全包 `go test -race ./tests/operation -count=1` 通过，覆盖拒绝零费、新价格发布后仍使用冻结版本、运行角色计价、缺失 / 冲突证据、重复调用及有用量取消；后续新增用例以 `go test -race ./tests/operation -run '^TestRealProvider' -count=1 -v` 通过。Agent 契约 / Activity 测试、`go vet ./...`、`goimports`、`golangci-lint run ./...` 通过，`govulncheck ./...` 无可达漏洞（另有 1 个未调用模块告警）。尚无真实供应商适配器与供应商账单核对，人工用量判定、批次未知结果汇总和完整 TC-21 链路仍待实施；E-21-03 不计完成，未提交。

**E-21-03 真实供应商人工未执行核对切片（2026-09-28）**：DES-04 §8.4 的 `manual → not_executed` 零费处理不再限定 mock 适配器。活跃管理员的私有核对依据仍须先持久写入；写入时拒绝已有明确执行 / 取消回执或已知正费用，工作流信号门禁和终态结算再次核对，阻止把“已执行但用量为零”改判为“未执行”。若信号校验后供应商证据变化导致结算被拒，工作流继续等待下一次人工决定，不把任务错误结束。真实适配器标识、陈旧决定与信号竞态的失败用例先 Red，修复后在全量迁移的独立本机 PostgreSQL 库通过 `go test -race ./tests/operation -count=1`；`gofmt`、`goimports`、`go vet ./...`、`golangci-lint run ./...` 通过，`govulncheck ./...` 无可达漏洞（另有 1 个未调用模块告警）。人工判定“已执行且有用量”、24 小时告警、真实供应商账单及完整 TC-21 验收仍待实施；E-21-03 未完成且未提交。

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

- **需求**：[REQ-30](docs/requirement/30-媒体库.md)（MED-01 上传素材；MED-02 浏览、检索与预览；MED-03 删除素材；MED-04 来源与合规信息）　**设计**：[DES-33](docs/design/33-媒体库.md)　**依赖**：E-30-01 建表前需 E-21-01 的 `operation.operation` 迁移（已完成）　**联调**：E-17（后实施，先按契约与模拟实现联调，对方完成后重跑本 Epic 验收）
- **验收**：TC-30-01～05（5 条）
- **待确认**：DES-33-Q1、DES-33-Q2（默认方案见设计文档，确认前按默认实施）；DES-33-Q3 已由 PRD-27 §8 与 REQ-30 R6 明确

| 任务 | 内容 | 怎么做 | 涉及文件 | 状态 | 提交 |
| --- | --- | --- | --- | --- | --- |
| E-30-01 | 数据与领域模型 | 迁移建表 / 加列：`media.media_asset`、`media.rendition`；实现领域对象、状态机与仓储（`media.media_asset`：仓储查询强制带 `project_id`；`media.rendition`：无 `project_id`，经父对象外键继承项目范围校验）。详见 [DES-33 §3](docs/design/33-媒体库.md#3-数据) | `backend/db/migrations/`、`backend/internal/media/domain/`、`backend/internal/media/adapter/postgres/` | 完成（迁移、状态规则、对象键归属、项目隔离仓储及真实 PostgreSQL 验证；公开接口与导入工作流属后续任务） | — |
| E-30-02 | 用例与接口 | 写操作经命令层（鉴权、幂等键、`expected_revision`、审计、Outbox）实现：POST /api/projects/{pid}/uploads、POST /api/uploads/{media_asset_id}:complete、GET /api/projects/{pid}/media、GET /api/media/{id}、DELETE /api/media/{id}；swag 注解生成 OpenAPI。详见 [DES-33 §4](docs/design/33-媒体库.md#4-接口) | `backend/internal/media/application/`、`backend/internal/media/adapter/http/`、`backend/docs/` | 待办 | — |
| E-30-03 | 异步、工作流与事件 | 工作流 / Activity / 定时任务 `MediaIngestWorkflow`、`media.DetectAndProbe`、`media.Hash`、`media.MakeRenditions`、`flow.MarkMediaReady`、`media-purge`；事件 `media.asset_status_changed.v1`（Outbox → Kafka，消费者按事件 ID 去重）。要点：`MediaIngestWorkflow`（`media-ingest/{id}`）：`media.DetectAndProbe` → `media.Hash` → `media.MakeRenditions`（并行）→… 详见 [DES-33 §5](docs/design/33-媒体库.md#5-异步与工作流) | `backend/internal/media/adapter/workflow/`、`backend/internal/media/adapter/event/` | 待办 | — |
| E-30-04 | Agent 服务 | 内容审核适配器 `moderation.check`；输入输出类型由 Go 与 Python 各自定义，契约测试（同一组示例输入输出）校验一致，离线评测纳入 `agent/evals/`。 | `agent/app/moderation/` | 待办 | — |
| E-30-05 | 前端 | 媒体库网格 / 列表切换（虚拟滚动）、筛选栏、搜索；拖拽上传区（多文件进度、失败原因）；预览（图片灯箱、视频播放器、音频波形）；详情抽屉（来源、合规、引用位置）。 详见 [DES-33 §7](docs/design/33-媒体库.md#7-界面) | `frontend/src/features/media/` | 待办 | — |
| E-30-06 | 测试与验收 | 内容类型识别；分片上传续传；引用检查覆盖全部引用来源（表驱动）；清理任务；验收用例 TC-30-01～05（[TST-02](docs/test/02-需求追踪矩阵.md)）。 | `backend/tests/`、`agent/tests/`、`frontend/tests/` | 待办 | — |

**E-30-01 数据与领域模型（2026-09-28）**：新增素材与派生版本迁移，绑定项目及同项目生成操作；领域规则限制对象键身份、状态迁移、审核通过后引用和删除 30 天后清理。PostgreSQL 仓储对当前账号、组织、项目与父素材复核后创建或读取记录，派生版本对象键必须位于父素材前缀。隔离 PostgreSQL 验证迁移 up/down/up、跨项目外键、撤权后不可读、错误对象键不可写与生成来源完整保留；`go test -race ./... -count=1`、`go vet ./...`、`golangci-lint run ./...`、`gofmt` / `goimports` 和 `govulncheck ./...` 已通过（漏洞扫描另报告 1 个未调用模块告警）。CI 增加媒体真实 PostgreSQL 契约门禁，远端结果随本项提交核验。上传/浏览接口、真实对象存储处理、引用检查及 TC-30-01～05 尚未完成，不计为 Epic 验收。

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
