# Agent 单服务架构调整设计

- 状态：按用户确认实施
- 日期：2026-09-08
- 上位设计：[0013 创作编排与多媒体画布架构调整](0013-创作编排与多媒体画布架构调整设计.md)、[3004 Agent Harness 专业能力与创作流程](3004-AgentHarness专业能力与创作流程设计.md)

## 1. 结论

Lanverse 的 Agent 在部署上收敛为一个服务、一个镜像、一个容器和一个内部 HTTP 入口。剧本解析的命令接收、Temporal 编排、运行记录、失败恢复和 Harness 推理属于同一个 Agent 产品能力，由同一个 Python 模块化单体承载。

这不是把业务事实写入 Agent。Go Backend 仍然拥有用户权限、正式源版本、正式业务 Writer、审批采纳、媒体供应商和账本；Agent 只拥有创作运行库、草案、尝试记录和编排状态。单服务只减少部署和认知成本，不改变模块边界、数据所有权或签名合同。

## 2. 当前问题

当前实现把一个 Agent 能力拆成 `harness`、`creation-api` 和 `creation-worker` 三个 Compose 服务。它们虽然职责不同，但均属于同一 Agent 运行时，导致本地启动、Docker 展示、健康检查和环境变量出现重复概念。该拆分没有带来本轮需要的独立伸缩能力，反而让用户把 Agent 误解为多个产品服务。

## 3. 目标与非目标

目标：

1. 根 Compose 只声明 `agent` 一个 Agent 服务；Frontend 和 Go Backend 继续独立。
2. Agent 一个镜像同时提供命令 API、Harness 路由和 Temporal Worker。
3. Agent 内部按 `creation`、`candidate_runtime`、`modules`、`text_contract` 分模块，代码边界保留，部署边界合并。
4. Backend 只配置一个 Agent origin；Agent 内部调用通过服务自身的受控 HTTP 路由完成。
5. 保留独立 Creation 数据库、独立 Creation 签名、Execution 签名和 Codex 登录挂载；模型子进程仍只继承明确白名单。
6. 保留原命令、Workflow ID、Attempt、冻结 Skill Release 和恢复合同，历史运行不迁移、不重写。

非目标：

- 不把 Agent 写入 Go 平台数据库或授予正式业务采纳权。
- 不删除 `creation`、`candidate_runtime` 等内部模块目录。
- 不引入进程管理器、第二个调度器或新的消息系统。
- 不为未来供应商、媒体能力或弹性伸缩预建微服务。

## 4. 目标运行形态

```text
Frontend ──> Go Backend ── signed HTTP ──> Agent :8787
                                         ├─ creation API
                                         ├─ Temporal dispatcher/worker
                                         ├─ Harness invocation routes
                                         └─ Agent PostgreSQL + Codex subprocess
```

Agent 的公开内部入口保持单一 origin。已有 `/internal/creation/*`、`/internal/storygraph/*` 和 `/internal/text-storyboard/*` 路由继续由同一应用提供；它们是模块路由，不是额外服务。Creation Worker 调用本服务的 Harness 路由时使用 `http://agent:8787`，不再通过第二个容器名称回环。

## 5. 生命周期与失败路径

Agent 应用启动时验证 Creation schema、Skill Release 和授权配置，启动 Creation dispatcher、Temporal Worker 和 HTTP server。任一后台任务退出都触发应用关闭；停止时先停止领取新命令，再等待在途 Activity 按既有宽限时间收敛。HTTP `/readyz` 只表示服务和持久化入口可用，不冒充整条模型解析链已完成。

模型执行失败、网络未知、预算耗尽和审核阻塞仍沿用 0020 的失败合同。合并进程不允许通过内存状态跳过数据库 fence、Attempt 或人工门。

## 6. 实施 checklist

- [x] 将 candidate runtime 路由组合进 Creation 应用，保留原合同路径。
- [x] 将 Temporal Worker 作为 Agent 应用后台任务启动，并绑定统一关闭策略。
- [x] 合并镜像依赖和 Dockerfile，根 Compose 删除三个旧服务，只保留 `agent`。
- [x] Backend、Worker 配置改为单一 Agent origin，更新 Docker 网络白名单与测试。
- [x] 更新 README、0013/0020 运行说明和架构验收，明确单服务与内部模块的区别。
- [x] 通过 Python/Go 合同测试、Compose 渲染、镜像构建和真实 Agent 就绪/路由 smoke；真实剧本语义验收仍按现有专项记录执行。
- [x] 提交前确认数据库、Temporal 历史、冻结发布和工作区外部凭据未被修改。
