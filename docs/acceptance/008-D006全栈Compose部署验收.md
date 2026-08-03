# ACC-008 D-006 全栈 Compose 部署验收

- 日期：2026-07-31
- 状态：工程验收通过
- 对应决策：PRD-003 D-006
- 范围：共享全栈 Compose、本地非 Docker 开发、development 快速启动、production 镜像化部署

## 1. 验收结论

根 `docker-compose.yml` 是六服务共享事实源，包含 PostgreSQL、Redis、RabbitMQ、MinIO、server 和 web。`docker-compose.prod.yml` 仅表达生产差异，不复制服务：隐藏四项基础设施宿主机端口、移除 server/web 本地构建并强制 `ENVIRONMENT=production`。基础镜像使用 `latest`，不固定 tag、digest 或二进制发行版。六个服务均配置健康检查与失败重启；server 等待四项基础设施健康并先幂等初始化数据库，web 等待 server 健康。四项有状态服务使用 Compose project 隔离的命名 volume，不设置固定 `container_name`。

本地开发仍可直接运行 venv/npm 命令，只在需要 MinIO 时执行 `make minio-up`；该命令同时验证 Lanverse Compose 容器归属与端口健康，其他 project 的健康 MinIO 占用同端口时快速失败，不复用未知凭据或启动第二实例。完整开发环境通过 `make docker-dev-up` 启动。生产配置由 `.env.production.example` 描述，真实 `.env.production` 不提交，必需 secret 为空时快速失败；`make docker-prod-up` 固定执行 `--no-build --pull always --wait`。生产文件是基础 Compose 的薄覆盖层，不维护第二套完整服务定义或 `CONTAINER_*` 地址。

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
| MinIO Compose 归属门禁 | Red 架构测试先证明 `minio-up` 只看通用 health；Green 后以 `docker compose ps --status running -q minio` 区分当前 project。实测 9000 被外部 `baozi-minio` 占用时明确拒绝，未停止外部容器或启动第二实例 |
| 隔离 project 清理 | development/production 验收容器、网络和四个数据卷均已删除，无残留 |
| `make check` | 通过；Ruff、Pyright、后端 149 passed/7 skipped、前端 15 文件/43 测试、pip check、Next.js build、development/production Compose config 均通过 |
| Docker 工具链 | Docker 29.6.2；Compose 5.3.1 |

隔离验收使用宿主机高位端口，不占用或停止现有本地 PostgreSQL、Redis、RabbitMQ、MinIO、API 和 Web；development project 为 `lanverse-compose-acceptance`，production project 为 `lanverse-prod-acceptance`，结束时都通过 `down --volumes --remove-orphans` 清理。

## 3. 未验证与残余风险

- `latest` 会随未来拉取变化；这是产品负责人明确选择的便利性取舍，每次部署仍须以健康启动和业务 smoke test 判断是否可用。
- 当前 Compose 提供单机部署启动健壮性，不等同于多节点高可用、滚动发布、TLS 终止或云 secret manager；这些能力应在真实发布拓扑确定后单独设计。
- 尚未配置真实镜像仓库，所以未执行生产命令中的远端 `--pull always`；本次以 `--pull never` 验证相同生产合并配置、无现场 build 和运行健康，真实发布仍需补镜像推送/拉取证据。
- 默认外部 Provider 密钥为空；本次验收不包含 DeepSeek、Seedream 或 Seedance 真实调用。
- 当前 9000 端口由外部 Compose project 占用，Lanverse MinIO 未启动；归属门禁已正确失败，但依赖当前 MinIO 的新浏览器增量仍须在端口恢复后重跑。
- Docker 构建期间 npm 对现有 lockfile 报告高危依赖审计项，未在本次 Compose 范围内升级依赖。
