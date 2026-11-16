# 织光画布协作核心

提供 Yjs 文档、Tiptap 绑定、增量接受、协作会话、个人撤销、journal、重连查证和版本恢复原语。Tiptap 固定为与前端一致的 3.32.0，Yjs 为13.6.33。

生产运行时使用 TypeScript、NestJS 和 Fastify，由 `pnpm start` 启动。Compose 服务名是 `canvas`，Go 调用 `http://canvas:4400`。前端消费 `@glowloom/canvas` 的共享子路径。目录职责见 [Canvas 工程规范](AGENTS.md) 和[设计 17](../workspace/content/docs/design/17-画布Yjs协作编辑设计.md)。

在本目录执行 `pnpm install --frozen-lockfile` 和 `pnpm build`，再安装前端本地依赖或运行 Go 协作集成测试。`pnpm test` 会先编译，再对产物运行测试。

本包不直连业务数据库。`AdmissionGate` 必须获得业务持久存储的确认后才能广播；测试内存存储不作为运行时后备。Go 接口与协作服务接线的实施状态以根目录 BACKLOG 为准，不能将核心测试通过等同于多人服务已经启用。

最初导入的 JavaScript 基线及逐文件摘要见 `source-manifest.json`；第三方许可见前端公开许可声明。

## 浏览器编辑器与应用宿主

`editor/` 唯一维护浏览器编辑器源码，包括节点、连线、手势、工具栏、富文本、导演台、时间线及样式。前端从 `@glowloom/canvas/editor` 导入主编辑器，从 `@glowloom/canvas/editor/*` 导入具体文件；源码由 frontend 的 Next.js 编译，浏览器源码不进入服务端 dist 或运行镜像。

frontend 保留页面、账号/空间、生成 API 客户端、业务适配与协作会话装配。`frontend/canvas-host.ts` 为构建提供 `@glowloom/canvas-host/*` 业务/公共 UI 接入和 `@glowloom/canvas-host-public/*` 公共元数据；这是应用内部的源码组合，不是宿主无关的发布 SDK。共享业务类型继续引用应用合同，不复制 DTO，也不注册全局可变 host。

React、Tiptap、ProseMirror、Yjs 的实例一致性由前端包边界测试检查。服务端安装配置关闭自动安装浏览器 peers，服务 Dockerfile 仍只复制 src/dist。来源清单记录最初导入的历史路径，迁移不改写来源或许可。

验证命令：

- `canvas/`：`pnpm typecheck`、`pnpm check`、`pnpm check:editor`、`pnpm test`。
- `frontend/`：`pnpm canvas:typecheck`、`pnpm canvas:lint`、`pnpm exec vitest run`、`GLOWLOOM_BACKEND_ORIGIN=<内部后端地址> pnpm exec next build --webpack`；构建后运行 `node --test tests/production/standalone-live-proxy.mjs`。
- 独立交互夹具：在 frontend 执行 `pnpm canvas:browser`，打开 `/tests/browser/canvas.html`。夹具使用合成数据，不代表真实身份、供应商或生产接受。

存储评估见[SQLite 调研](../workspace/content/docs/research/58-2026-11-16-画布存储压力与SQLite适配评估.md)，实际验证见[计划13](../workspace/content/docs/plan/13-画布编辑与素材实施计划.md)。
