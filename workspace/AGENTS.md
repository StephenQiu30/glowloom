# Workspace 工程约束

继承[根协作规则](../AGENTS.md)、[工程事实](../PROJECT.md)与[视觉规范](../DESIGN.md)。本文件只补充服务边界。

1. 项目知识正文统一在 `content/`；Skill 和流程在 `skills/`、`workflows/`。根规范、业务源码和数据库契约不复制。原始证据放受控产物，不写入知识正文。
2. `src/app/`、共用阅读组件与内容模型必须可静态构建；不能导入 `src/server/`、在线路由、PG 或执行器。在线源码只从 `src/online/` 入口组装。
3. `src/build/service.ts` 分别生成 `.build/static`、`.build/online` 和不同模式的不可变产物；这些目录不可手改或提交。两种构建均不连接生产数据库、主机或 TTS。
4. Workspace 独立拥有 shadcn 配置、组件、Hook、主题与资源；不得从 sibling frontend 复制或运行时导入代码。不建立 scripts 目录；构建、内容、朗读、运行分别由职责模块提供可调用能力，CLI 只负责参数与装配。业务规范仍以原工程为权威，引用快照只作为受控知识输入。
5. 数据库结构只修改根 `backend/db/schema.sql`。研发运行使用 `dev_workspace` 和受限 `glowloom_workspace` 角色；不得混用产品 `workspace` 或 `agent` 数据，不在启动时建表。
6. 在线服务不挂可写仓库。文档应用和主机执行通过受控桥；固定路径、修订、授权、幂等键与原句柄。未知回执先查询，不重新派发；暂停或取消不回滚已有副作用。
7. 修改代码后运行适用 test、typecheck、lint、content、双构建及真实浏览器验证。数据库测试只允许明确命名的隔离测试库；模拟、真实主机和产品接受分别报告。
8. 真实 TTS 暂时禁用。音频导入需有使用权，校验源版本、摘要、实测时长与 cue；不按字数伪造时序，不调用供应商、不上传声音。
9. 新增知识文件后同步 catalog 稳定身份和导航；保留现有业务ID。当前进度仍只在根 BACKLOG，Git提交、推送和发布按当前授权执行。

10. 自有 TS／TSX、工具入口与测试遵循 [Google TypeScript Style Guide](https://google.github.io/styleguide/tsguide.html)：命名函数、单变量声明、单引号、花括号、导入前置、对象形状优先 interface、unknown 边界校验；strict／lint／format 作为可执行门禁。Next.js 页面、布局及配置要求的默认导出和文件命名是框架例外，其他自有模块使用命名导出。第三方导入按其真实导出；本地 shadcn 原语经审阅后独立维护并纳入同样门禁。
11. UI 继承根 DESIGN，布局及控件复用现有 shadcn。修改界面须核验 Vercel Web Interface Guidelines 与 React Best Practices，并按适用 UI/UX Pro Max 规则检查键盘、焦点、移动端、双主题和减少动态效果。Markdown 正文与原生媒体保留标准语义。
12. 通用文档能力使用 Fumadocs Core／Headless；shadcn 负责界面。正文仍按普通 Markdown 处理，不引入可执行 MDX。catalog 的文档 ID、旧阅读／raw 地址和显式锚点保持稳定，先做隔离全量兼容再替换。身份／摘要模块不得依赖文档渲染器；不建设 TTS 服务。
