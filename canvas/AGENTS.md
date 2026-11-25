# Canvas 服务工程规范

本文件继承[仓库规范](../AGENTS.md#nodejs-服务工程规范)。服务边界、数据合同和部署拓扑见[设计 17](../workspace/content/design/17-画布Yjs协作编辑设计.md)，依赖版本以包清单和锁文件为准。

1. 手写 TypeScript 遵循 [Google TypeScript Style Guide](https://google.github.io/styleguide/tsguide.html)，使用 ESM、具名导出与显式职责。`src/` 采用扁平两层包组织（`src/<package>/<module>.ts`），文件名统一 `kebab-case`，禁止手写 `index.ts` barrel、`export default` 及 `../../` 深层相对路径攀爬。纯类型用 `import type`，Node 内置模块用 `node:`，NodeNext 相对导入使用运行时 `.js` 扩展名。新增边界接收外部值时先用 `unknown`，校验后交给用例。
2. `server.ts` 只负责启动配置校验、进程监听与有界退出；`config.ts` 集中解析并校验启动环境。Nest HTTP 适配器、gRPC Server、共享 Bearer 鉴权与统一并发门禁集中于 `transport/`。运行时（`runtime/`）、文档（`document/`）、协作会话（`collaboration/`）和编解码（`protocol/`）保持普通 TypeScript，按 [设计 17](../workspace/content/design/17-画布Yjs协作编辑设计.md) 划分，避免无职责目录和仅转发包装。
3. `package.json` 的子路径导出仅面向同构纯协作内核（`recovery/session/journal/wire/model/rich-text/service-document`）。共享内核的依赖闭包不包含 Nest、Fastify、gRPC、React/Next/Three.js 或 Node 专属入口；全部浏览器画布 UI 归 `frontend/src/components/canvas-editor/` 维护。`types/` 随导出一起维护。Go 通过私有 gRPC/HTTP 调用候选计算并持有接受、WebSocket 广播和持久化权威。
4. 使用冻结锁文件安装。构建前清理 `dist/` 产物，防止历史文件残留。在本目录运行 `pnpm typecheck`、`pnpm check` 和 `pnpm test`。构建和目录变更还须验证 Go 真实运行时 fixture、前端本地包解析以及 Compose 接线。
