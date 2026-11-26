# 织光画布协作核心

提供 Yjs 文档、Tiptap 绑定、增量接受、协作会话、个人撤销、journal、重连查证和版本恢复原语。Tiptap 固定为与前端一致的 3.32.0，Yjs 为13.6.33。

生产运行时由 `pnpm start` 独立启动。Compose 服务名是 `canvas`，Go 调用 `grpc://canvas:4402`，`canvas:4400` 提供健康检查及内部 HTTP 兼容接口。前端消费 `@glowloom/canvas` 的共享子路径。目录职责见 [Canvas 工程规范](AGENTS.md) 和[设计 17](../workspace/content/design/17-画布Yjs协作编辑设计.md)。

在本目录执行 `pnpm install --frozen-lockfile` 和 `pnpm build`，再安装前端本地依赖或运行 Go 协作集成测试。`pnpm test` 会先编译，再对产物运行测试。

本包不直连业务数据库。`AdmissionGate` 必须获得业务持久存储的确认后才能广播；测试内存存储不作为运行时后备。Go 接口与协作服务接线的实施状态以根目录 BACKLOG 为准，不能将核心测试通过等同于多人服务已经启用。

最初导入的 JavaScript 基线及逐文件摘要见 `source-manifest.json`；第三方许可见前端公开许可声明。

## 浏览器编辑器与应用宿主

`frontend/src/components/canvas-editor/` 维护编辑器源码。`frontend/src/components/canvas/` 负责账号与空间、业务适配及协作会话装配。正式 `/canvas` 通过 `YjsCanvasWorkspace` 使用编辑器与技能工作流。源码归属见[设计 10](../workspace/content/design/10-画布交互设计.md)。

React、Tiptap、ProseMirror、Yjs 的实例一致性由前端包边界测试检查。服务 Dockerfile 使用协作运行时代码与私有 RPC 合同；前端 UI 由 Next.js 编译。来源清单记录最初导入的历史路径，许可见前端声明。

验证命令：

- `canvas/` 运行 `pnpm typecheck`、`pnpm check`、`pnpm test`。
- `frontend/` 运行 `pnpm exec eslint src/components/canvas-editor src/components/canvas`、`pnpm exec vitest run` 和 `GLOWLOOM_BACKEND_ORIGIN=<内部后端地址> pnpm exec next build --webpack`，构建后运行 `node --test tests/production/standalone-live-proxy.mjs`。
- 独立交互夹具：在 frontend 执行 `pnpm canvas:browser`，打开 `/tests/browser/canvas.html`。夹具使用合成数据，不代表真实身份、供应商或生产接受。

存储评估见[SQLite 调研](../workspace/content/research/58-2026-11-16-画布存储压力与SQLite适配评估.md)，实际验证见[计划13](../workspace/content/plan/13-画布编辑与素材实施计划.md)。
