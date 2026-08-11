# ACC-008 D-006 全栈 Compose 部署验收

- 日期：2026-07-31；原生 MinIO 入口复核：2026-08-11
- 状态：工程验收通过
- 对应决策：PRD-003 D-006
- 范围：共享全栈 Compose、本地非 Docker 开发、development 快速启动、production 镜像化部署

## 1. 验收结论

根 `docker-compose.yml` 是六服务共享事实源，包含 PostgreSQL、Redis、RabbitMQ、MinIO、server 和 web。`docker-compose.prod.yml` 仅表达生产差异，不复制服务：隐藏四项基础设施宿主机端口、移除 server/web 本地构建并强制 `ENVIRONMENT=production`。基础镜像使用 `latest`，不固定 tag、digest 或二进制发行版。六个服务均配置健康检查与失败重启；server 等待四项基础设施健康并先幂等初始化数据库，web 等待 server 健康。四项有状态服务使用 Compose project 隔离的命名 volume，不设置固定 `container_name`。

本地开发直接运行 venv/npm 命令，并复用 Homebrew 安装的 PostgreSQL、Redis、RabbitMQ 和已在 `127.0.0.1:9000` 运行的 MinIO；项目不提供本机 MinIO 启停或管理封装。完整开发环境直接以 `docker compose up -d --build --wait` 一键启动共享 Compose 的六服务；容器内 server 使用 `minio:9000`，宿主机发布 `9100/9101` 以避开本机 MinIO。生产配置由 `.env.production.example` 描述，真实 `.env.production` 不提交，必需 secret 为空时快速失败；生产使用 Compose 薄覆盖并固定执行 `--no-build --pull always --wait`，不维护第二套完整服务定义或 `CONTAINER_*` 地址。

## 2. 验收证据

| 验证 | 结果 |
| --- | --- |
| Red：运行新的全栈 Compose 架构测试 | 预期失败；旧实现缺少 PostgreSQL、Redis、RabbitMQ、server、web |
| Red：运行 development/production 分层架构与配置测试 | 预期失败；3 项失败，旧实现缺少生产 env 模板、生产薄覆盖层和显式环境命令 |
| development `docker compose --env-file .env.example config` | 通过；服务为 minio/postgres/rabbitmq/redis/server/web，基础镜像均为 `latest` |
| production 模板不注入 secret 直接解析 | 预期拒绝；数据库、RabbitMQ、MinIO、JWT 必需 secret 不会静默使用示例值 |
| development/production env 字段一致性 | 通过；变量集合完全一致，production 已包含 `DATABASE_URL`、`TEST_DATABASE_URL`、PostgreSQL 初始化与全部基础设施连接字段，含凭据值保持为空 |
| production 合并配置解析 | 通过；六服务保留，server 环境为 production，四项基础设施 ports 与 server/web build 均为空 |
| development 隔离全栈 `up -d --build --wait --wait-timeout 180` | 通过；四项依赖先健康，server 完成数据库初始化后健康，web 最后健康 |
| production 隔离全栈 `up -d --no-build --pull never --wait --wait-timeout 180` | 通过；复用本地已构建 server/web 镜像，六服务全部健康；用于验证合并后的生产运行语义，不替代真实镜像仓库拉取 |
| production API `/readyz` 与 Web `/` | 均返回 HTTP 200；server 容器中 `ENVIRONMENT=production` |
| production 基础设施端口 | PostgreSQL、Redis、RabbitMQ、MinIO 均无 published host port；只有 API/Web 分别使用隔离高位端口 48000/43000 |
| 本机 MinIO 复用（2026-08-11） | 已确认 Homebrew 安装的 `/opt/homebrew/opt/minio/bin/minio` 使用 `/opt/homebrew/var/minio`，API/Console 分别监听 `127.0.0.1:9000` 与 `9001`；Lanverse 只配置 endpoint 和凭据，不启动第二个本机实例 |
| 本机 MinIO 真实契约（2026-08-11） | 在 `backend/` 显式设置 `LANVERSE_RUN_MINIO_CONTRACT=1` 后直接运行相关 Pytest，结果为 7 passed；私有桶、八项存储端口、预签名上传下载、匿名拒绝、媒体 API 直传和 hash mismatch 清理全部通过。首次运行因缺少硬编码安全测试库 `lanverse_test` 在夹具阶段为 5 passed/2 errors；补建空测试库后复核 7/7，通过过程未触碰开发库 |
| Docker 完整环境保留（2026-08-11） | development Compose 解析为 minio/postgres/rabbitmq/redis/server/web 六服务，MinIO 发布 `9100/9101` 且 server 内部 endpoint 保持 `minio:9000`；production 薄覆盖仍保留六服务、隐藏全部 MinIO 宿主机端口并移除 server 构建。当前 Docker daemon 未运行，因此本轮只复核合并配置；2026-07-31 的 development/production 六服务真实启动证据继续有效 |
| 隔离 project 清理 | development/production 验收容器、网络和四个数据卷均已删除，无残留 |
| `make check` | 通过；Ruff、Pyright、后端 149 passed/7 skipped、前端 15 文件/43 测试、pip check、Next.js build、development/production Compose config 均通过 |
| `make check`（2026-08-03 复用增量） | 通过；Ruff、Pyright、后端 175 passed/7 skipped、前端 16 文件/49 测试、pip check、Next.js 16.2.12 build、development/production Compose config 均通过 |
| Docker 工具链 | Docker 29.6.2；Compose 5.3.1 |

隔离验收使用宿主机高位端口，不占用或停止现有本地 PostgreSQL、Redis、RabbitMQ、MinIO、API 和 Web；development project 为 `lanverse-compose-acceptance`，production project 为 `lanverse-prod-acceptance`，结束时都通过 `down --volumes --remove-orphans` 清理。

## 3. 未验证与残余风险

- `latest` 会随未来拉取变化；这是产品负责人明确选择的便利性取舍，每次部署仍须以健康启动和业务 smoke test 判断是否可用。
- 当前 Compose 提供单机部署启动健壮性，不等同于多节点高可用、滚动发布、TLS 终止或云 secret manager；这些能力应在真实发布拓扑确定后单独设计。
- 尚未配置真实镜像仓库，所以未执行生产命令中的远端 `--pull always`；本次以 `--pull never` 验证相同生产合并配置、无现场 build 和运行健康，真实发布仍需补镜像推送/拉取证据。
- 默认外部 Provider 密钥为空；本次验收不包含 DeepSeek、Seedream 或 Seedance 真实调用。
- 本地 `.env` 必须使用该 Homebrew MinIO 的实际凭据；凭据不进入 README、日志或 Git。Compose 使用自己的 volume 和 `9100/9101`，不读写本机 `/opt/homebrew/var/minio`；该本地边界不改变 production 必须显式配置存储凭据与网络边界的要求。
- Docker 构建期间 npm 对现有 lockfile 报告高危依赖审计项，未在本次 Compose 范围内升级依赖。
