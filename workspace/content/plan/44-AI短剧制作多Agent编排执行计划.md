# AI 短剧制作多 Agent 编排执行计划

唯一来源 PRD：[PRD35：AI短剧制作多Agent编排](../prd/35-AI短剧制作多Agent编排.md)。技术合同沿 [设计49](../design/49-AI短剧制作三层Agent与六阶段Workflow设计.md) 展开；实施进度与调度见 [BACKLOG](../../../BACKLOG.md)。

## 1. 任务与退出条件

| ID        | 阶段 | 对应接受   | 文件责任与核心动作                                                                                                       | 退出条件                                                                    |
| --------- | ---- | ---------- | ------------------------------------------------------------------------------------------------------------------------ | --------------------------------------------------------------------------- |
| T-SDMA-01 | P0   | AC-SDMA-01 | 新增需求34、设计49、PRD35、Plan44 并同步 `index.mdx` 与各目录导航                                                        | Workspace `types:check`、`lint`、`prettier --check` 通过                    |
| T-SDMA-02 | P1   | AC-SDMA-02 | 在 `agent/` 落地三层 5 角色（`runtime.ts` + `prompts/`）、六阶段短剧 Workflow（`short-drama-workflow.ts`）与 6 套 `SKILL.md` | `pnpm typecheck`、`pnpm lint`、`pnpm test`（含三处 HITL `suspend/resume`）全绿 |
| T-SDMA-03 | P2   | AC-SDMA-03 | 在 `agent/evals/short-drama.eval.test.ts` 落地短剧专项 Mastra Scorers（主体绑定率、零未审批扣费、监督层 ≤2 轮收敛）      | `pnpm eval` 全部评测通过                                                    |
| T-SDMA-04 | P3   | AC-SDMA-04 | 在 Go `backend/internal/agent/adapter/http/short_drama_mcp.go` 注册 5 组短剧原子 MCP 工具与安全校验及单元测试            | `go test -race -count=1 ./internal/agent/... ./tests/db/...` 全绿           |

## 2. 验证方法

- `workspace/`：`pnpm types:check && pnpm lint`
- `agent/`：`pnpm typecheck && pnpm lint && pnpm test && pnpm eval && pnpm build`
- `backend/`：`go test -race -count=1 ./internal/agent/... ./tests/db/...`

[Plan导航](00-Plan导航.md) · [唯一来源PRD35](../prd/35-AI短剧制作多Agent编排.md)
