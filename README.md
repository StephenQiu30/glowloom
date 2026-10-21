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
docker compose up -d --build          # 启动或更新前后端
docker compose up -d --build backend  # 仅启动或更新后端
```

现有依赖服务、Temporal namespace、身份密钥及 `.env` 须提前准备；Compose 复用这些资源，不执行数据库迁移。启动后使用 `docker compose ps` 查看状态，使用 `docker compose logs --tail=100 backend` 排查编译或启动错误。

身份与会话密钥由 `.env` 直接注入，配置名及格式见 `.env.example`：`GLOWLOOM_IDENTITY_KEY`、`GLOWLOOM_IDENTITY_JWT_PRIVATE_KEY`、`GLOWLOOM_IDENTITY_JWT_VERIFY_KEYS`、`GLOWLOOM_IDENTITY_JWT_ACTIVE_KID`、`GLOWLOOM_IDENTITY_REFRESH_RECEIPT_KEY`。JWT 私钥使用 Base64 PEM，验证映射为 KID 到 Base64 公钥 PEM 的 JSON，回执密钥为独立32字节 Base64值；运行时不加载独立密钥文件或 Compose secret 挂载。已有密钥不得随启动重新生成，缺失密钥按恢复授权处理，身份主密钥必须保留。

Temporal返回命名空间不存在时，先用`temporal operator namespace list --address 127.0.0.1:7233`核对现有服务，再按`.env`中的实际命名空间显式注册；不改用其他项目的命名空间。本地默认值可使用`temporal operator namespace create --namespace glowloom-local --address 127.0.0.1:7233 --retention 7d`，随后重启后端。

后端每次启动都编译只读挂载的当前源码，成功后才启动服务；编译失败直接停止，不运行旧程序。独立 Go 构建缓存卷减少重复编译耗时。首次更新开发镜像后，纯 Go 源码更新也可执行 `docker compose restart backend`；修改依赖、Dockerfile、启动入口或 Compose 配置时重新执行上述带 `--build` 的命令。裸 `docker compose up -d` 不会强制更新已运行容器，开发后端运行中也不会自动热重载。

启动会使用工作区当前源码，包括未提交修改；存在并行任务时，先确认这些修改可以一起运行，或在隔离的源码快照中验证。前端继续使用现有 Next.js 开发服务热更新，依赖卷的升级仍按现有依赖管理流程处理；生产镜像仍在构建阶段编译。

前端开发可在 `frontend/` 执行 `pnpm exec next dev --hostname 127.0.0.1 --port 3141`。服务访问地址以实际启动配置为准；启动成功不代表业务或邮件已验收。

工程与门禁见 [PROJECT](PROJECT.md)，协作规则见 [AGENTS](AGENTS.md)，共享视觉规范见 [DESIGN](DESIGN.md)，当前事项见 [BACKLOG](BACKLOG.md)。进程命令为 `glowloom`，入口是 `backend/cmd/glowloom`。配置使用 `GLOWLOOM_*` 并在过渡期兼容旧 `LV_*`；新键优先且不改动凭据值。数据库与存储标识以实际部署为准。

文档直接维护 Markdown，只保留现行正文。第三方版权与许可见 [THIRD_PARTY_NOTICES](frontend/public/THIRD_PARTY_NOTICES.md)。
