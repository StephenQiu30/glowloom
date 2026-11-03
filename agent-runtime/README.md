# 创作 Agent 运行时

独立内部 Host 使用固定版本的耐久 Harness SDK。会话、输入、任务、工具意图、memos 和检查点由 PostgreSQL 保存；进程内索引只服务读取与重建，不是持久化源。不使用本地文件数据库。Host 没有供应商密钥，也不加载通用 shell、文件读写或第三方 JS 扩展。

## 已实现的底座

- PostgreSQL 提交日志与连续 hash 校验、SDK Storage 公共合同、会话 advisory lock、存储 epoch、原子 ID 分配；数据库提交确认丢失时关闭写者，恢复读取实际结果。
- 真实 SDK 会话、持久命令准入/同键冲突、同 submission 恢复、快照、压缩、进程关闭。
- 每会话独立模型 Provider，generation/compaction 通过 SDK taskId + memo 固定交换身份；失败的 hook 不能绕过网关；SDK 重试关闭。
- 内部命令、快照、drain API，过期时间及 method/path/body 绑定签名，当前 Go 授权检查。

这部分不等于完整产品。当前命令只接纳冻结身份、模型及批准上下文下的输入；规划到执行的批准变更、完整 Run 动作族由后续 Go 控制协议接线。Go 生产控制/模型交换/工具接口、Skill、多 Agent、业务交付与界面仍按 Plan28 接入；当前 Host 不应开启产品准入。测试中运行真实 SDK 和 PostgreSQL，模型网关是受控 HTTP 服务，不能据此声称已验证真实供应商费用或创作质量。

## 开发与验证

Node 固定为 `.node-version`，依赖由 `pnpm-lock.yaml` 固定。安装运行 `pnpm install --frozen-lockfile`，检查运行 `pnpm typecheck`。

在隔离 PostgreSQL 18 数据库中预置 `glowloom_app NOLOGIN NOSUPERUSER` 与 `glowloom_agent_runtime NOLOGIN NOSUPERUSER`，由结构所有者显式执行 `backend/db/agent_harness_incremental.sql`。Host 的数据库登录仅继承 `glowloom_agent_runtime` 的 SDK 表权限，不使用 Go 业务账号或管理员账号。应用启动不会自动建表或迁移；完整空库来源仍是 `backend/db/schema.sql`。业务库不得用作测试库。

设置 `GLOWLOOM_TEST_AGENT_DB_DSN` 后运行 `pnpm test`。测试会创建随机会话并仅清理自己的会话；包含上游全套 Storage 合同、受限数据库角色、锁竞争、断连、COMMIT 确认丢失以及子进程 SIGKILL 恢复。缺少隔离 DSN 直接失败，不跳过。

## 内部运行配置

`pnpm build` 生成忽略的 `dist/`。`pnpm start` 需要显式提供 `GLOWLOOM_AGENT_DATABASE_DSN`、`GLOWLOOM_AGENT_GATEWAY_ORIGIN` 和至少 32 字符的 `GLOWLOOM_AGENT_SIGNING_KEY`；可通过 `GLOWLOOM_AGENT_PORT` 设置内部端口，默认 4401。生产仅从唯一 Compose 入口接线，不能向宿主/浏览器开放该端口。Go 网关必须同时实现正式授权与模型交换合同，不能连接测试夹具服务。

`GET /healthz` 只表示 Host 进程；`GET /readyz` 检查进程与 PostgreSQL 连接，不能代替 Go 出口、具体会话锁或模型可用性验收。所有 `/internal/v1` 路由需要短期签名。签名为 HMAC-SHA256，内容为 method、原始 path、到期秒数和原始 body SHA256，按换行连接；到期不得超过当前 120 秒，签名分别放在 `x-glowloom-runtime-expires` 与 `x-glowloom-runtime-signature`。内部密钥不属于供应商凭据。

关闭或失败的会话在资源释放后从 Host 缓存移除；可信 dispatcher 重投原命令时重新打开同一 PostgreSQL 会话。Host 启动不会自行扫描并接管所有会话，Go 的所有权恢复与投递链路另按 Plan28 完成。

初版恢复采用按会话分批读取提交日志和 SDK 内存索引，尚无日志压缩/容量 SLO。需要 PostgreSQL 一致备份与 Go 业务回执对账；数据库备份恢复不是已发生外部副作用的回滚。
