# Agent FastAPI 结构与 Skill 运行时整合需求规格

日期：2026-09-08。范围：Agent 应用入口、FastAPI 组装和当前内置 Skill 运行时整合。依据：[0022 设计](../design/0022-Agent%20FastAPI%20结构与%20Skill%20运行时整合设计.md)。

本规格只覆盖当前可运行的 `build-storygraph`、Scene Analysis 和 `text-storyboard` 能力，不把 3003 中未实现的 production Stage、Skill Release Control 或用户 Skill 市场纳入本次完成定义。

## 可测试合同

| ID | 必须满足的行为 | 最低验证 |
|---|---|---|
| FAPI-001 | `app.main` 导出 `app` 和 `create_agent_app`；规范工厂不再从 `app.creation.api` 直接启动 | Python import、架构测试 |
| FAPI-002 | Docker 使用 `app.main:create_agent_app --factory`；镜像包含 `app/api` 和 `app/skills` | Dockerfile 静态检查、镜像 smoke |
| FAPI-003 | `app/api/application.py` 统一创建 FastAPI、挂载内部能力路由并安装 Agent 生命周期 | 应用工厂测试、源码边界测试 |
| FAPI-004 | Creation、Candidate Runtime 和文本合同的原路径、请求鉴权和响应 schema 不变 | 现有契约/集成测试 |
| FAPI-005 | `app.creation.api:create_agent_app` 只作为兼容转发，规范文档和启动命令不再引用它 | 静态引用扫描 |
| SKILL-001 | 注册表只声明 `storygraph`、`scene_analysis`、`text_storyboard` 三个能力键，资源包名称和校验器固定 | 注册表单元测试 |
| SKILL-002 | 注册表校验结果与现有 Bundle/TextSkill 的发布摘要逐项相等；任一漂移返回 Skill 不可用 | 正常校验、篡改测试 |
| SKILL-003 | 资源只从仓库内 `agent/skills` 加载；不读取 `.agents`、用户目录、网络或环境变量路径 | 路径负向测试、Dockerfile检查 |
| SKILL-004 | 启动校验和就绪检查复用同一注册表，不存在第二套能力列表 | readiness 测试、架构扫描 |
| SKILL-005 | 每个 Harness 继续按 stage 的显式 Reference 白名单装载，不能递归读取整个 Skill 目录 | 现有 Bundle/TextStoryboard 测试 |
| SKILL-006 | Skill 校验错误不泄露绝对路径、原稿、Prompt、凭据或异常详情 | HTTP readiness 负向测试 |
| BOUND-001 | Candidate Runtime 通过 Skill 注册层时仍不导入 Creation、Temporal、数据库或平台客户端 | 模块依赖测试 |
| BOUND-002 | 本切片不新增数据库表、迁移、Temporal Workflow、Go API 或前端 Skill 市场入口 | 差异和架构审查 |

## 数据与目录约束

- `agent/skills/` 是静态资源目录；`app/skills/` 是 Python 运行时注册目录，不能互换。
- 注册表不复制 Skill 正文，不生成新的 release hash，不允许同一能力键多重注册。
- Skill 校验只证明安装的资源和固定摘要可用，不证明真实 Codex 登录、语义质量、Owner 采纳或媒体生成可用。

## 完成定义

所有 FAPI、SKILL 和 BOUND 条款都有测试或静态证据；Agent 全量 Ruff、格式、Pyright、Pytest 通过；Dockerfile 入口和 Compose 渲染通过；工作区只包含本切片文件。真实模型和真实剧本验收单独记录，不以本规格抵扣。
