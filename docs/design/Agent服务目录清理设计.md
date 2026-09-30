# Agent 服务目录清理

- 决策：2026-09-30 用户明确要求先清理当前 `agent/` 服务目录；此前提出必要时由 Go 统一服务实现。本次按该指令移除独立 Python 服务。
- 范围：删除 `agent/` 源码、测试、Skill 示例、依赖与锁文件，以及该目录下的虚拟环境和缓存；移除应用 Compose、CI 镜像构建和两项跨语言测试的直接依赖；同步当前启动与工程说明。
- 本次不实施：Go 供应商适配器、凭据解封、限流、审核、Harness、对话式 Agent；不删除数据库、迁移、业务历史、Go 工作流或前端 Agent 产品入口。

## 当前模块边界

运行单元保留 Next.js 前端与 Go 后端的 API、Worker、Relay 角色。业务事实、权限、预算、Operation、媒体处理和事件处理继续由现有 Go 模块承担；PostgreSQL、Redis、Kafka、对象存储、Temporal 的职责保持。

原 Python 服务承担的 `provider.submit/query/cancel`、`provider.test_credential`、`moderation.check`、Skill/Harness 等执行能力在删除后没有运行实现。Go 统一承接的详细设计和验收继续归 M1-12；目录清理不代表迁移完成。

## 数据、协议与失败路径

- 不改 Activity 名称、队列、输入输出、Operation ID、供应商请求键或账本去重协议。Go 中现有 `agent` / `agent.mock` 队列引用保留，供迁移核对；当前应用部署不提供这些队列的执行 Worker。
- 保留已有数据库迁移与历史任务，不通过 SQL 删除、终止工作流或释放预算。已有任务若等待被移除的 Activity，会继续等待或按现有超时规则失败，不能将它们计为完成。
- 凭据封装和 Go 单元测试保留；Go→Python 解封和跨语言追踪测试随执行端删除。后续 Go 实现需补真实解封、鉴权、追踪、取消与供应商验证。
- API 的健康/就绪检查和 Go 工作流单元测试不证明供应商执行可用。补齐 Go 执行端前，生成、凭据测试、审核和 Skill 路径不能用于概念验证的成功证据。

## 验证范围

确认目录不存在，Compose 与 CI 不再引用该目录，保留的服务和 CI 门禁与清理前一致；运行受影响 Go 模块的格式、静态检查与 Race 测试。依赖缺失导致的集成测试跳过单列记录，不计真实链路通过。

DES-01/08、OPS-01/02 及 BACKLOG 中旧 Python 服务的部署、职责和待办路径，以本记录为清理范围的最新依据；历史验收记录保留当时事实。完整 Go 承接方案仍须在 M1-12 更新对应设计后实施。

## 本地实施与验证记录（2026-09-30）

清理前 Git 根目录为 Lanverse，分支 `main`，HEAD `751492571906d63d57be92360f308f794f25dde2`，工作区干净。`agent/` 有 45 个跟踪文件，没有额外未跟踪源码；忽略文件均为虚拟环境、工具缓存或 Python 字节码。删除仅针对该目录和下列直接依赖。

| 文件 | 变化 |
| --- | --- |
| `agent/` | 删除全部 45 个跟踪文件、虚拟环境与缓存 |
| `.github/workflows/ci.yml` | 删除 Agent 作业、镜像构建与对应 job 依赖 |
| `docker-compose.yml` | 删除 agent-api、agent-worker |
| `backend/tests/catalog/credential_seal_test.go` | 删除 Go→Python 测试；保留 Go 封装、解密、篡改与错误输入测试 |
| `backend/tests/temporalconn/cross_language_tracing_test.go` | 删除仅依赖 Python Worker 的追踪测试文件；现有 Go 追踪测试保留 |
| `README.md` | 更新服务组成、架构图与启动说明 |
| `PROJECT.md` | 更新目录、运行职责、配置与门禁；AI 承接保持待办 |
| `BACKLOG.md` | 区分历史 Python 骨架交付、当前目录删除和未完成的 Go 承接 |
| `docs/design/01-系统架构设计.md` | 标明旧 Python 部署方案已撤销 |
| `docs/design/08-技术选型决策.md` | 标明旧 Python 服务选型已撤销 |
| `docs/design/BeefTV能力引入设计.md` | 同步用户最新清理决定 |
| `docs/operation/01-环境与部署.md` | 更新当前配置，标明旧 Agent 部署项失效 |
| `docs/operation/02-CI-CD与发布.md` | 去除 Python 作业、镜像与依赖更新门禁 |
| 本文件 | 记录授权范围、未承接能力、失败路径与验证证据 |

未新增业务逻辑，因此未新增 Red→Green 测试；以配置语义比较和现有 Go 回归作为可执行验收。

| 实际检查 | 结果 |
| --- | --- |
| `test ! -e agent`、直接依赖搜索、`git diff --check` | 通过；无剩余运行/CI 路径依赖 |
| `docker compose --env-file /dev/null -f docker-compose.yml config --no-interpolate --no-env-resolution --quiet` | 通过；未读取实际 `.env`，未启动容器 |
| Node + YAML 解析 CI、比较清理前后 Compose JSON / CI 对象 | 通过；仅移除指定 Agent 服务、job、镜像和依赖，其他服务/门禁相同 |
| `gofmt`、`goimports -local github.com/StephenQiu30/lanverse/backend` 检查变更的 Go 测试文件 | 通过 |
| `go vet ./...`、`golangci-lint run ./...`（backend） | 通过，lint 为 0 issues |
| `go test -race -count=1 -json ./tests/catalog ./tests/temporalconn ./tests/operation`（backend） | 106 个顶层测试通过、102 个跳过、0 失败；跳过项缺少 PostgreSQL/Redis/Kafka/Temporal 等集成条件，不计真实链路通过 |
| `govulncheck ./...`（backend） | 0 可达漏洞；1 个所需模块存在未调用漏洞提示 |
| 变更白名单、文档链接、生产目录与生成物 diff | 通过；前端、Go 生产代码、SQL、Swagger/API 生成物及依赖清单未改动 |

未运行前端构建、镜像构建、真实依赖集成、真实供应商或部署验收。本记录生成时尚未提交或推送，后续按用户指令提交到 main；交付状态以 Git 与远端 CI 为准。本记录仅证明目录和直接工程依赖已清理。
