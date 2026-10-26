# Glowloom · 织光

织光围绕图片、视频、声音、主体、素材和画布组织视觉创作。仓库为 `StephenQiu30/glowloom-server`，包含 `frontend/`、`backend/` 和 `docs/`；命令从对应目录执行。

## 当前需求

当前范围涵盖用户体系、项目与主画布、统一创作入口、多参考编辑器、AI接入、上传存储及画布数据/协作。完整候选目录不等于全部已排期；当前实施进度、剩余工作与阻塞统一见 [BACKLOG](BACKLOG.md)，规则见原始需求及设计，实施与历史证据见对应 Plan。

- [文档入口](docs/00-文档索引.md)
- [产品总览设计](docs/design/24-产品总览与交付边界设计.md) · [全部主题设计](docs/design/00-设计导航.md)
- [统一工作台与创作入口设计](docs/design/09-统一顶栏与阶段入口设计.md) · [AI接入设计](docs/design/07-AI平台接入市场调研与架构设计.md)
- [详细需求与验收](docs/requirement/02-用户系统需求.md)
- [系统设计](docs/design/01-用户系统总体设计.md) · [数据库](docs/design/02-用户系统数据库设计.md) · [接口](docs/design/03-用户系统接口设计.md) · [测试与运行](docs/design/04-用户系统测试与运行设计.md)
- [用户系统产品定义](docs/prd/01-用户系统.md)
- [落地任务与验证](docs/design/01-用户系统总体设计.md)
- [全产品能力范围](docs/requirement/01-产品范围.md)

任务按[设计、用户故事、测试场景模板](docs/templates/00-模板导航.md)组织，当前实施状态统一维护在 [BACKLOG](BACKLOG.md)。原始需求、技术设计、PRD、Plan 和长期调研独立维护；按 design → PRD → Plan 显式链接，原文、确认和证据保留；多参考编辑器仍沿已确认的 Tiptap 方案，规则和分层证据见[编辑器设计](docs/design/06-多参考提示词编辑器调研与设计.md)。

## 开发与协作

Docker 开发环境直接在仓库根目录执行标准 Compose 命令：

```sh
docker compose up -d --build          # 启动或更新项目
docker compose up -d --build backend  # 更新后端，自动处理必要依赖
```

`docker-compose.yml` 是唯一启动入口，包含 `frontend`、`backend` 和现有画布必需的私有 `canvas-collaboration`。后端在同一容器运行 API、worker、relay，并等待协作运行时健康后启动，无需指定额外文件或手动排序。访问地址固定如下，旧 `GLOWLOOM_FRONTEND_PORT`、`GLOWLOOM_BACKEND_PORT` 及 `LV_*` 端口变量不覆盖开发栈：

应用服务统一通过上述 Docker Compose 命令启动和管理，不另开宿主机 Next 或 Go 服务。PostgreSQL、Redis、Kafka、Temporal 和对象存储继续复用现有基础依赖。

| 服务 | 宿主访问地址            | 容器内部端口             |
| ---- | ----------------------- | ------------------------ |
| 前端 | <http://localhost:1122> | 3000                     |
| 后端 | <http://127.0.0.1:1123> | 18080（代理至 API 8080） |

两端的浏览器 Origin 同步固定为 `http://localhost:1122`，忽略 `.env` 中旧的 Origin，避免端口切换后登录或写入被 Origin 校验拒绝。worker/relay 健康检查在容器内执行，不额外开放宿主端口。

现有依赖服务、Temporal namespace、身份密钥、前端开发缓存卷及 `.env` 须提前准备；Compose 复用这些资源，不执行数据库迁移。仓库不再提供额外的依赖栈、生产拆分或协作 overlay Compose 文件。

旧画布库须先按[画布增量升级记录](docs/plan/17-画布数据库持久化与DAG实施计划.md#75-本地存量画布增量升级2026-10-23)核对、备份并演练，再由结构所有者执行 `backend/db/canvas_protocol_incremental.sql`。不得对存量库重放 `schema.sql`。容器健康不代表业务表已升级，仍需验证真实画布读取。

启动前确认1122和1123未被占用：`lsof -nP -iTCP:1122 -iTCP:1123 -sTCP:LISTEN`。若被其他进程占用，应先协调端口冲突，再启动应用。启动后使用 `docker compose ps` 查看状态，使用 `docker compose logs --tail=100 backend` 排查编译或启动错误；可分别请求前端、后端的 `/healthz` 检查访问链路。

身份与会话密钥由 `.env` 直接注入，配置名及格式见 `.env.example`：`GLOWLOOM_IDENTITY_KEY`、`GLOWLOOM_IDENTITY_JWT_PRIVATE_KEY`、`GLOWLOOM_IDENTITY_JWT_VERIFY_KEYS`、`GLOWLOOM_IDENTITY_JWT_ACTIVE_KID`、`GLOWLOOM_IDENTITY_REFRESH_RECEIPT_KEY`。JWT 私钥使用 Base64 PEM，验证映射为 KID 到 Base64 公钥 PEM 的 JSON，回执密钥为独立32字节 Base64值；运行时不加载独立密钥文件或 Compose secret 挂载。已有密钥不得随启动重新生成，缺失密钥按恢复授权处理，身份主密钥必须保留。

模型预设接入还需要独立的凭据加密 RSA 密钥对与请求 MAC 密钥，配置四个 `GLOWLOOM_CREDENTIAL_*` 项（见 `.env.example`）。Compose 将本机 `.local/credentials/` 只读挂载到 `/run/glowloom-credentials/`，文件路径填写容器路径。首次配置使用至少 2048 位 RSA 的 PKCS8 私钥及 PKIX 公钥；请求 MAC 密钥为随机 32 字节的十六进制编码。密钥文件和 `.env` 不进入 Git，已有加密凭据对应的密钥必须保留，不在启动时自动重新生成。配置私钥后 `--role=all` 会同时消费 `provider` 队列；独立部署仍通过 `--role=worker --queues=provider` 运行。旧库使用结构所有者执行 `backend/db/provider_onboarding_incremental.sql` 增量升级，不重放全库 schema。

Temporal返回命名空间不存在时，先用`temporal operator namespace list --address 127.0.0.1:7233`核对现有服务，再按`.env`中的实际命名空间显式注册；不改用其他项目的命名空间。本地默认值可使用`temporal operator namespace create --namespace glowloom-local --address 127.0.0.1:7233 --retention 7d`，随后重启后端。

后端每次启动都编译只读挂载的当前源码，成功后才启动服务；编译失败直接停止，不运行旧程序。独立 Go 构建缓存卷减少重复编译耗时。首次更新开发镜像后，纯 Go 源码更新也可执行 `docker compose restart backend`；修改依赖、Dockerfile、启动入口或 Compose 配置时重新执行上述带 `--build` 的命令。裸 `docker compose up -d` 不会强制更新已运行容器，开发后端运行中也不会自动热重载。

启动会使用工作区当前源码，包括未提交修改；存在并行任务时，先确认这些修改可以一起运行，或在隔离的源码快照中验证。前端继续使用现有 Next.js 开发服务热更新，依赖卷的升级仍按现有依赖管理流程处理；生产镜像仍在构建阶段编译。

默认 CI 只运行两条业务门禁：后端保留全量 Race、隔离 PostgreSQL 集成与协作运行时测试；前端保留全量 Vitest 和含 TypeScript 检查的生产构建。依赖按锁文件安装并缓存；静态风格、漏洞扫描、生成一致性等检查按变更风险在本地执行，详见 [PROJECT](PROJECT.md#4-质量门禁)。启动或 CI 成功不代表邮件、真实供应商或生产环境已经验收。

工程与门禁见 [PROJECT](PROJECT.md)，协作规则见 [AGENTS](AGENTS.md)，共享视觉规范见 [DESIGN](DESIGN.md)，当前事项见 [BACKLOG](BACKLOG.md)。进程命令为 `glowloom`，入口是 `backend/cmd/glowloom`。配置使用 `GLOWLOOM_*` 并在过渡期兼容旧 `LV_*`；新键优先且不改动凭据值。数据库与存储标识以实际部署为准。

文档直接维护 Markdown，只保留现行正文。第三方版权与许可见 [THIRD_PARTY_NOTICES](frontend/public/THIRD_PARTY_NOTICES.md)。
