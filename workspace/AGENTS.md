# Workspace 工程约束

继承[根协作规则](../AGENTS.md)、[工程事实](../PROJECT.md)与[视觉规范](../DESIGN.md)。本文件只补充服务边界。

1. 项目知识正文统一在 `content/`；Skill 和流程在 `skills/`、`workflows/`。根规范、业务源码和数据库契约不复制。原始证据放受控产物，不写入知识正文。
2. `src/app/`、共用阅读组件与内容模型必须可静态构建；不能导入 `src/server/`、在线路由、PG 或执行器。在线源码只从 `src/online/` 入口组装。
3. `scripts/build.ts` 分别生成 `.build/static`、`.build/online` 和不同模式的不可变产物；这些目录不可手改或提交。两种构建均不连接生产数据库、主机或 TTS。
4. 界面原语复用 frontend 的 shadcn 源码，由 prepare-ui／构建脚本复制到忽略目录。修改共享原语应在原所属工程评审，不能在派生副本修复。
5. 数据库结构只修改根 `backend/db/schema.sql`。研发运行使用 `dev_workspace` 和受限 `glowloom_workspace` 角色；不得混用产品 `workspace` 或 `agent` 数据，不在启动时建表。
6. 在线服务不挂可写仓库。文档应用和主机执行通过受控桥；固定路径、修订、授权、幂等键与原句柄。未知回执先查询，不重新派发；暂停或取消不回滚已有副作用。
7. 修改代码后运行适用 test、typecheck、lint、content、双构建及真实浏览器验证。数据库测试只允许明确命名的隔离测试库；模拟、真实主机和产品接受分别报告。
8. 真实 TTS 暂时禁用。音频导入需有使用权，校验源版本、摘要、实测时长与 cue；不按字数伪造时序，不调用供应商、不上传声音。
9. 新增知识文件后同步 catalog 稳定身份和导航；保留现有业务ID。当前进度仍只在根 BACKLOG，Git提交、推送和发布按当前授权执行。
