# Agent FastAPI 标准化目录与 Skill 运行时设计

- 状态：本轮实现基线（2026-09-08）
- 依据：FastAPI 官方多文件应用、依赖注入、生命周期与测试指南
- 关联：`docs/design/3004-AgentHarness专业能力与创作流程设计.md`、`docs/design/0022-Agent FastAPI 结构与 Skill 运行时整合设计.md`

## 1. 需要纠正的问题

上一轮只新增了 `app/api/application.py`，没有真正把 HTTP 层从业务模块中分离出来。当前结构仍存在四个问题：

1. `app/creation/api.py` 同时负责 FastAPI 应用创建、路由注册、鉴权、数据库访问、Temporal 生命周期和 Worker 生命周期。
2. `app/harness/api.py` 自己创建第二个 `FastAPI` 应用，生产应用通过导入它来复用路由。
3. `app/creation/attempt_api.py`、`manifest_api.py` 通过 `add_api_route` 安装闭包路由，依赖运行时捕获对象，而不是 FastAPI 的 `Depends`。
4. `app/api/application.py` 通过修改 `app.router.lifespan_context` 叠加生命周期，无法形成一个可测试、可组合的应用生命周期。

因此，目录改名或增加一个应用壳不能视为 FastAPI 标准化。本设计以 FastAPI 的 HTTP 层约定重新划分边界，现有业务合同和持久化语义只作为迁移对象，不作为目录标准的来源。

## 2. 标准基线

FastAPI 官方的多文件应用结构以 `app/main.py` 作为应用入口，以 `APIRouter` 按资源拆分路由，并在入口统一 `include_router`。依赖由 `Depends` 声明，资源初始化与释放使用应用的 `lifespan` 参数，测试通过应用工厂和 HTTPX/TestClient 驱动：

- [Bigger Applications - Multiple Files](https://fastapi.tiangolo.com/tutorial/bigger-applications/)
- [Dependencies](https://fastapi.tiangolo.com/tutorial/dependencies/)
- [Lifespan Events](https://fastapi.tiangolo.com/advanced/events/)
- [Testing](https://fastapi.tiangolo.com/tutorial/testing/)

FastAPI 没有规定必须使用某个 `services/` 或 `repositories/` 模板。Lanverse 采用按业务能力组织的模块化单体：API 层遵循上述标准，业务域保留自己的合同、服务和基础设施；不为了形式创建空的 Controller、Service、DAO 三层。

## 3. 目标目录

```text
agent/
├── app/
│   ├── __init__.py
│   ├── main.py                         # 唯一 FastAPI 应用工厂
│   ├── core/
│   │   ├── __init__.py
│   │   ├── config.py                   # 进程配置与环境校验
│   │   ├── container.py                # 显式运行时依赖容器
│   │   └── lifespan.py                 # 唯一 asynccontextmanager 生命周期
│   ├── api/
│   │   ├── __init__.py
│   │   ├── dependencies.py             # Depends 提供者与签名请求体
│   │   ├── router.py                   # 只负责 include_router
│   │   └── routes/
│   │       ├── __init__.py
│   │       ├── health.py               # healthz/readyz
│   │       ├── creation.py             # Creation 内部 HTTP 合同
│   │       ├── storygraph.py           # StoryGraph Harness HTTP 合同
│   │       ├── scene_analysis.py       # Scene Analysis Harness HTTP 合同
│   │       └── text_storyboard.py      # Text Storyboard Harness HTTP 合同
│   ├── creation/                       # Creation 领域、应用服务和持久化实现
│   ├── harness/                        # Harness 输入/输出合同和执行服务
│   ├── modules/                        # StoryGraph 等专业领域实现
│   ├── reasoning/                      # Codex 进程适配器
│   ├── protocol/                       # 跨进程 canonical 合同
│   ├── text_contract/                  # 文本任务和结果合同
│   └── skills/
│       ├── catalog.py                 # 只读 Skill Release 注册表
│       └── runtime.py                 # 单次应用生命周期内的 Skill 解析器
└── skills/                             # 镜像内静态 SKILL.md 发布包
    ├── build-storygraph/
    └── text-storyboard/
```

`app/main.py` 不实现业务；`api/routes` 不直接创建 Repository、Temporal Client、Worker 或 Skill；`core` 不承载业务规则；`skills/` 根目录是不可变资源，不能从网络、用户目录或请求内容动态发现 Skill。

## 4. 应用组装和生命周期

`create_app()` 创建一个 `AgentContainer`，将 `Settings`、Repository、SkillCatalog、Temporal 客户端工厂和 Worker 运行器显式注入。应用只构造一个 `FastAPI` 实例，并在构造函数中传入一个 `lifespan`：

```python
@asynccontextmanager
async def lifespan(app: FastAPI) -> AsyncIterator[None]:
    runtime = app.state.runtime
    await runtime.start()
    try:
        yield
    finally:
        await runtime.stop()

app = create_app()
```

不得在运行时修改 `app.router.lifespan_context`，不得由业务模块创建 FastAPI 子应用，也不得在路由安装函数中捕获可变的数据库或 Temporal 对象。

Worker 的所有权由 `AgentContainer` 持有，启动、就绪等待、取消、停止和超时收敛都在同一个 lifespan 中完成。测试可以传入假的 Repository、Temporal Client、SkillCatalog 和 Reasoner，不需要导入或启动真实基础设施。

## 5. HTTP 路由和依赖

每个 HTTP 资源拥有独立的 `APIRouter`，路由函数只做四件事：解析 Pydantic 输入、取得 `Depends` 注入的服务、调用一个应用用例、将领域结果映射为 HTTP 响应。

- `api/dependencies.py` 提供 `get_runtime`、`get_skill_runtime`、`get_creation_service`、`get_harness_service`、`get_skill_catalog` 和签名请求体读取器。
- Creation 的鉴权、UUID 校验、数据库异常和恢复语义由 Creation 应用服务提供；路由不再嵌套定义这些函数。
- Harness 路由只负责 Header 鉴权和错误映射；StoryGraph、Scene Analysis、Text Storyboard 的执行由同一个 Harness 服务调用。
- `health.py` 只返回存活状态；`readyz` 通过注入的 SkillCatalog、Codex 可执行文件和授权配置做确定性前置检查，不调用模型。

## 6. Skill 运行时

Skill 是 Agent 的受控运行时能力，不是 FastAPI 路由，也不是可执行的用户上传内容。`app/skills/catalog.py` 维护固定的 `SkillRegistration`，每个注册项声明：

- 稳定能力键、发布包目录、版本/内容摘要；
- 输入/输出合同和允许的 Harness 工厂；
- 发布包校验器与撤回后的不可用状态。

`SkillRuntime` 在应用启动时接收一个 `SkillCatalog`，校验所有已声明发布包，并向 Harness 服务提供只读的已验证 Skill 实例。路由和 Harness 不得直接拼接 `skills/` 路径、导入任意类路径或重新创建全局 Catalog。新增 Skill 必须同时新增注册项、合同、校验、正反例测试和发布记录。

本轮保留三个已发布能力：`storygraph`、`scene_analysis`、`text_storyboard`。不引入 Skill 市场、网络下载、用户目录扫描、动态插件执行或额外 Agent 服务。

## 7. 迁移边界

本轮必须完成：

- 删除业务模块中的 FastAPI 应用创建和 `add_api_route` 注册；
- 将所有生产路由迁移到 `app/api/routes/`；
- 用 `Depends` 和 `AgentContainer` 替代闭包捕获和模块级运行时对象；
- 用单一 `lifespan` 替代生命周期叠加；
- 让 SkillCatalog/SkillRuntime 由应用组装并注入 Harness 与 readiness；
- 更新 Docker、测试、架构检查和文档入口为明确的 `--factory app.main:create_app`；测试工厂可以通过同样的 `--factory` 方式注入替身资源。

本轮不改变 StoryGraph/Text Storyboard 的输入输出合同、Creation 数据库 schema、Temporal Workflow 身份、Go 正式采纳边界和静态 Skill 的内容摘要。业务合同变更必须另立设计，不借目录迁移顺便修改。

## 8. 完成条件

1. `rg` 检查确认 `FastAPI(` 只出现在 `app/main.py` 和测试工厂，`add_api_route` 不再出现在生产代码。
2. `app/api/router.py` 只包含路由组合；每个生产 HTTP 路径都来自 `app/api/routes/`。
3. 应用只创建一个 FastAPI 实例，应用生命周期只由一个 `lifespan` 管理。
4. 通过依赖覆盖可以在不启动 PostgreSQL、Temporal、Codex 的情况下测试每个路由。
5. 启动和 `/readyz` 使用同一个注入的 SkillCatalog，未知 Skill、发布摘要漂移和静态资源缺失都返回明确阻塞状态。
6. Ruff、Pyright、Pytest、Docker 构建和非 root 镜像冒烟通过，并对跳过的外部条件单独记录。
