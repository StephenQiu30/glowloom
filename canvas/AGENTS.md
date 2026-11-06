# Canvas 服务工程规范

本文件继承[仓库规范](../AGENTS.md#nodejs-服务工程规范)。服务边界、数据合同和部署拓扑见[设计 17](../docs/design/17-画布Yjs协作编辑设计.md)，依赖版本以包清单和锁文件为准。

1. 手写 TypeScript 遵循 [Google TypeScript Style Guide](https://google.github.io/styleguide/tsguide.html)，使用 ESM、具名导出与显式职责。纯类型用 `import type`，Node 内置模块用 `node:`，NodeNext 相对导入使用运行时 `.js` 扩展名。新增边界接收外部值时先用 `unknown`，校验后交给用例。
2. `server.ts` 只启动进程并管理退出。Nest Controller、服务身份与请求限制集中于 `http/`。运行时、文档、会话和编解码使用普通 TypeScript，按 [设计 17](../docs/design/17-画布Yjs协作编辑设计.md) 划分。避免无职责的目录和仅转发的包装。
3. `package.json` 的子路径导出面向前端共享能力。共享模块的依赖闭包不包含 Nest、Fastify 或 Node 专属入口。`types/` 随导出一起维护。Go 通过私有 HTTP 调用候选计算并持有接受和持久化权威。
4. 使用冻结锁文件安装。在本目录运行 `pnpm typecheck`、`pnpm check` 和 `pnpm test`。构建和目录变更还须验证 Go 真实运行时 fixture、前端本地包解析以及 Compose 接线。
