# Agent FastAPI 结构与 Skill 运行时整合设计

- 状态：按用户要求实施当前切片
- 日期：2026-09-08
- 上位设计：[Agent 单服务架构调整](0021-Agent单服务架构调整设计.md)、[Harness 专业能力与创作流程](3004-AgentHarness专业能力与创作流程设计.md)、[StoryGraph Harness 与内置 Skill](3003-StoryGraph剧本解析Harness与内置Skill设计.md)
- 配套文档：[需求规格](../requirement/0022-AgentFastAPI结构与Skill运行时整合需求规格.md)、[实施计划](../plan/0022-AgentFastAPI结构与Skill运行时实施计划.md)、[验收记录](../acceptance/0022-AgentFastAPI结构与Skill运行时验收记录.md)

## 1. 结论

Agent 保持一个 FastAPI 服务、一个镜像和一个容器。本次只重整应用壳层和 Skill 运行时入口：`app.main:create_agent_app` 成为唯一正式 Uvicorn 入口；FastAPI 应用组装移到 `app/api/`；`creation` 继续拥有命令、持久执行、Temporal 和恢复业务；`candidate_runtime` 继续拥有受限候选 HTTP 合同；专业模块不被搬进无业务含义的 Controller/Service 层。

Skill 分成两层：

1. `agent/skills/` 是镜像内的不可变 Skill 资源包，保留 `build-storygraph` 和 `text-storyboard` 的文件、逐阶段 Reference 和既有发布摘要。
2. `app/skills/` 是 Agent 内的显式运行时注册层，统一声明 `storygraph`、`scene_analysis`、`text_storyboard` 三个当前能力的资源包、预期发布摘要和校验器。它不从用户目录、网络或工作区自动发现 Skill，也不负责下载、安装或修改 Skill。

这样既符合 FastAPI 的应用入口习惯，也把 Skill 变成服务启动、就绪检查和受限 Harness 都能使用的明确依赖，不改变 Agent/Go Backend 的数据所有权和人工采纳边界。

## 2. 当前问题与证据

当前 `app/main.py` 只转发到 `app.creation.api`，而 `app/creation/api.py` 同时负责：

- FastAPI 应用创建和路由注册；
- Creation 数据库就绪检查和命令鉴权；
- Dispatcher、Temporal Worker 的生命周期；
- Candidate Runtime 路由组合和 Skill 启动校验。

`agent/skills/` 已被 Dockerfile 复制进镜像，`StoryGraphBundle`、`SceneAnalysisBundle` 和 `TextSkill` 各自能校验资源，但验证和就绪报告分散在 Candidate Runtime 中，没有一个服务级 Skill 注册表。当前的功能合同是有效的，本次不把“目录标准化”误解为重写这些业务合同。

## 3. 目标与非目标

### 3.1 目标

1. 统一正式入口为 `app.main:create_agent_app --factory`。
2. 在 `app/api/application.py` 集中组装 Agent FastAPI 应用、内部能力路由和统一生命周期。
3. 将 Candidate Runtime 路由通过 `app/api/router.py` 组合，避免在业务模块中承担服务组装职责。
4. 在 `app/skills/catalog.py` 建立显式 Skill 注册表，覆盖当前三个能力键，并复用现有 Bundle/TextSkill 校验实现。
5. 启动校验和 `/readyz` 使用同一注册表；任一资源缺失、漂移或发布摘要不匹配都 fail closed。
6. Docker 镜像复制新的 `app/api`、`app/skills` 包，并使用新的正式入口。
7. 保留旧 Python 导入路径作为短期代码兼容别名，但不再作为 Docker、文档和测试的规范入口。

### 3.2 非目标

- 不迁移 `creation`、`candidate_runtime`、`modules`、`text_contract` 的业务文件，不建立空的 `controllers`、`services`、`repositories` 目录。
- 不实现 3003 中尚未落地的十三个 production Stage、Release Control、动态 Skill 市场或用户上传 Skill。
- 不允许运行时联网搜索、远程安装、执行用户目录或动态加载未注册的 Skill。
- 不改变数据库 schema、Temporal Workflow ID、HTTP Wire、Skill 内容、发布摘要或 Go Backend 的 Owner/采纳权限。
- 不把 Skill 资源暴露成前端自动发现市场，也不把 Skill 状态当作正式业务事实。

## 4. 目标目录与职责

```text
agent/
├── app/
│   ├── main.py                 # Uvicorn 规范入口，导出 app 和 create_agent_app
│   ├── api/
│   │   ├── application.py      # FastAPI 应用工厂与 Agent 生命周期
│   │   └── router.py           # 内部能力路由组合
│   ├── skills/
│   │   └── catalog.py          # 显式 Skill 注册、发布摘要校验
│   ├── creation/               # 可信命令、ExecutionStore、Temporal 与恢复
│   ├── candidate_runtime/      # 受限 HTTP、授权与候选结果边界
│   ├── modules/                # StoryGraph 与文本分镜专业 Harness/Schema
│   ├── text_contract/          # 可信编排与受限执行共享的纯文本合同
│   ├── protocol/               # canonical 编码合同
│   └── reasoning/              # Codex 子进程边界
└── skills/                     # 不可变 Skill 资源包，不是 Python 导入包
```

`app/api` 只负责 HTTP 组装和后台任务生命周期；业务模块仍由实际消费方定义接口。`app/skills` 只负责“哪些资源包可以被这个镜像使用、如何验证其固定摘要”，不复制 SKILL 正文或业务规则。

## 5. FastAPI 应用合同

### 5.1 入口和组装

- `app.main:create_agent_app` 返回已配置的 `FastAPI` 实例；`app.main:app` 供普通 Uvicorn 导入。
- Docker 使用 `uvicorn app.main:create_agent_app --factory ...`。
- `app.api.application` 先创建 Creation 应用，再挂载 Candidate Runtime 的内部路由，并安装同一生命周期：数据库就绪、Skill 校验、Dispatcher、Temporal Worker、优雅停止。
- `app.creation.api:create_agent_app` 仅保留惰性兼容转发，避免历史测试/脚本在迁移期间失效；它不再是规范入口。

### 5.2 路由边界

- Creation 的命令、执行、草案、Attempt、Manifest 路由保持原路径和鉴权合同。
- StoryGraph、SceneAnalysis、TextStoryboard 路由保持原路径和受限授权合同。
- `/healthz` 保持进程存活语义；`/readyz` 继续由 Agent 应用提供存储/运行入口就绪语义，技能能力列表使用统一 Skill 注册校验，不宣称真实模型、人工采纳或完整生产链已完成。

## 6. Skill 运行时合同

### 6.1 当前注册表

| 能力键 | 资源包 | 校验实现 | 用途 |
|---|---|---|---|
| `storygraph` | `build-storygraph` | `StoryGraphBundle.verify_installed_bundle` | 旧 StoryGraph stage 候选 |
| `scene_analysis` | `build-storygraph` | `SceneAnalysisBundle.verify_installed_bundle` | Scene Span/Fact 候选 |
| `text_storyboard` | `text-storyboard` | `TextSkill.release_hash` | 分集、场景、人物/地点/道具和文字分镜草案 |

注册表必须是代码中的固定集合，不接受环境变量提供路径或能力键。每项同时固定公开能力键、资源包名称、期望 release hash 和校验器；路径解析由既有 Bundle/TextSkill 实现完成并继续执行符号链接、文件集合、UTF-8、路径范围和内容摘要检查。

### 6.2 加载与失败

- 服务启动时验证全部注册项；缺少文件、非法资源、hash 漂移或未注册能力都返回明确的 Skill 不可用错误并阻止服务就绪。
- Readiness 只返回能力键、状态、发布摘要和安全错误码，不返回绝对路径、原稿、Prompt、凭据或异常堆栈。
- Harness 继续按 stage 显式选择 Reference；注册表不能把整个 `skills/` 目录递归拼入 Prompt。
- Skill 内容变化必须更新对应发布摘要和合同；不提供“找最接近版本”或旧路径 fallback。

### 6.3 安全和所有权

Skill 只读、随镜像发布；Agent 不在运行期写 Skill、不下载外部内容、不调用 Provider、不写 Go 正式业务库或 Elasticsearch。用户原稿和 Skill 文本都按不可信数据处理，现有 Codex 工具拒绝策略保持不变。

## 7. 实施与回滚

实施顺序为：先建立文档和失败测试，再增加 `app/api` 入口与路由组合，随后增加 `app/skills` 注册表并接入启动/就绪校验，最后更新 Dockerfile、README 和运行时测试。每一步保持旧导入路径可用，完成镜像和契约验证后再切换规范入口。

回滚只需恢复 Docker/启动入口和应用壳层文件；不触碰 Creation 数据、Temporal 历史、Skill 资源内容和已发布摘要。若 Skill 校验失败，保持 fail closed，不通过回退到另一资源包绕过错误。

## 8. 完成边界

本设计完成只代表 Agent 的 FastAPI 入口、应用组装和当前 Skill 注册/校验边界完成。它不代表长稿分块、真实 60 集语义质量、人物正式 Owner 映射、ES 投影、媒体 Provider 或 3003 的完整 production Stage 已完成；这些仍按各自 Design/Spec/Acceptance 追踪。
