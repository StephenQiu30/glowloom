# Agent FastAPI 标准化目录与 Skill 运行时需求规格

- 状态：本轮实施
- 依据：[0023-Agent FastAPI 标准化目录与 Skill 运行时设计](../design/0023-Agent%20FastAPI%20标准化目录与%20Skill%20运行时设计.md)

## 可测试合同

| ID | 必须满足的行为 | 验证 |
|---|---|---|
| FAPI-STD-001 | 生产只创建一个 FastAPI 应用；入口为 `app.main:create_app --factory` | 源码边界测试、Docker smoke |
| FAPI-STD-002 | `app/api/router.py` 只组合 `app/api/routes/` 的 APIRouter | 架构测试 |
| FAPI-STD-003 | 业务模块不包含 FastAPI 应用、`add_api_route` 或 `lifespan_context` | 静态架构测试 |
| FAPI-STD-004 | 路由通过 `Depends` 获取运行时、应用服务和 SkillCatalog | HTTP 集成测试、依赖覆盖 |
| FAPI-STD-005 | 单一 `lifespan` 拥有数据库、Dispatcher 和 Temporal Worker 的启动、就绪、停止 | 生命周期测试、代码审查 |
| SKILL-STD-001 | SkillCatalog 在应用容器中创建一次并注入 Harness/readiness | 服务单元测试 |
| SKILL-STD-002 | Skill 只能从镜像内静态 `skills/` 资源按固定注册项加载 | 注册表测试、Docker 文件检查 |
| SKILL-STD-003 | 三个已发布 Skill 的 hash 漂移、未知 key、缺失文件都返回阻塞状态 | 负向测试 |
| SKILL-STD-004 | 不允许网络、用户目录、任意类路径或请求内容动态发现 Skill | 负向测试、依赖扫描 |
| BOUND-STD-001 | Creation/Harness/Reasoning/Skill 的业务合同保持现有版本和错误语义 | 现有合同与单元测试 |
| BOUND-STD-002 | 本轮不新增 Go API、数据库 schema、Temporal Workflow 或前端功能 | 差异审查 |

## 完成定义

所有条款均有自动化或静态证据；Ruff、Pyright、Pytest、Docker 构建和镜像冒烟通过；跳过的真实 PostgreSQL、Temporal、Codex 条件单独记录。
