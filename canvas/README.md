# 织光画布协作核心

提供 Yjs 文档、Tiptap 绑定、增量接受、协作会话、个人撤销、journal、重连查证和版本恢复原语。Tiptap 固定为与前端一致的 3.32.0，Yjs 为13.6.33。

生产运行时使用 TypeScript、NestJS 和 Fastify，由 `pnpm start` 启动。Compose 服务名是 `canvas`，Go 调用 `http://canvas:4400`。前端消费 `@glowloom/canvas` 的共享子路径。目录职责见 [Canvas 工程规范](AGENTS.md) 和[设计 17](../workspace/content/design/17-画布Yjs协作编辑设计.md)。

在本目录执行 `pnpm install --frozen-lockfile` 和 `pnpm build`，再安装前端本地依赖或运行 Go 协作集成测试。`pnpm test` 会先编译，再对产物运行测试。

本包不直连业务数据库。`AdmissionGate` 必须获得业务持久存储的确认后才能广播；测试内存存储不作为运行时后备。Go 接口与协作服务接线的实施状态以根目录 BACKLOG 为准，不能将核心测试通过等同于多人服务已经启用。

最初导入的 JavaScript 基线及逐文件摘要见 `source-manifest.json`；第三方许可见前端公开许可声明。
