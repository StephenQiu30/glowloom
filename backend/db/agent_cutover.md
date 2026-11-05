# 独立 Agent 与数据库任务的一次性离线切换

来源：[设计32 §7.3–7.9](../../docs/design/32-完整创作AgentHarness与Skill多Agent架构设计.md)、[Plan28 Task13/14](../../docs/plan/28-完整创作Agent与Skill多Agent实施计划.md)。本目录脚本供部署负责人审阅后离线执行；编写脚本、隔离演练和提供证据参数都不等于获得现有业务库执行授权。

## 范围与前置条件

`agent_cutover_inventory.sql` 在 `REPEATABLE READ READ ONLY` 事务中盘点旧表、FK、角色/ACL、运行、供应商原请求和费用，只输出ID/状态/金额，不输出会话正文、请求体或密钥。它与迁移共用 `agent_cutover_blockers.sql` 的27项PG阻断规则；缺少预期表/列视为部署结构不符，不能把查询失败解释为“没有在途”。使用 `psql -X -v ON_ERROR_STOP=1`，不要通过应用启动流程调用。

`agent_cutover.sql` 只接受已确认的HEAD旧结构：8张旧表 `agent.session`、`runtime_session`、`runtime_commit`、`harness_run`、`harness_candidate`、`harness_command`、`harness_approval`、`harness_admission`；单独旧增量可能另建第9张 `agent.graph_checkpoint`，盘点按实际存在对象计数。未知Agent表/视图/序列/函数、外部入站FK或半完成的新结构都会阻止执行。

执行前必须停写并关闭本库所有其他客户端、完成可恢复的PG备份、取得删除旧Agent聊天/运行记忆的独立明确授权，并分别清结旧Temporal与供应商工作/费用。脚本不停止服务、不取消远程任务、不删除或改写账号/项目/素材/operation/账本、不迁移旧执行状态、不安装兼容历史读取器。DDL在单个事务中精确按依赖逆序 `DROP TABLE ... RESTRICT`；失败全部回滚。第二次执行明确拒绝。

旧 `operation.target_type='agent_session'` 即使已终态也阻止切换：当前正式目标约束已移除该类型，旧记录需要另行审阅并确定现行正式资源归属。不能删除operation/账本、修改为随意目标、使用 `NOT VALID` 或增加旧枚举来绕开。`ck_operation_agent_session` 的豁免只在零旧目标记录后移除；`ck_operation_upload` 与目标名单改为当前 `schema.sql` 的有效约束。画布 `source_context` 约束本身与旧HEAD一致，继续保留，不引入另一套来源格式。

部署管理员预建 `glowloom_agent LOGIN` 与 `glowloom_app NOLOGIN` 并离线配置凭据；脚本不设置密码。两个角色及旧 `glowloom_agent_runtime NOLOGIN` 必须无管理/复制/绕过RLS权限；新Agent无角色成员关系、数据库CREATE或Agent schema所有权。安装后Node只有Agent运行表权限，Go只有业务表权限。旧runtime失去本库Agent schema/运行对象授权；如它在其他业务表上还有权限则拒绝，要求单独审阅。它是集群级角色，本脚本不执行 `DROP ROLE`、`DROP OWNED` 或跨库撤权。管理员须另行盘点其他库依赖后决定是否移除这个空的NOLOGIN角色。

当前目标包含8张 `agent_business` 表；其中 `source_authorization` 以原调用回执/执行grant和source hash保存业务授权，没有Pi执行状态。最终门禁逐表拒绝Node业务权限，并核验Go对该新授权表的SELECT/INSERT/UPDATE权限。

## 先盘点和取得独立证据

以下变量由操作员明确提供，不读取项目 `.env`。使用受保护的连接配置/密码通道；不要把实际DSN、密码或会话内容贴入审批记录。

```sh
psql "$GLOWLOOM_CUTOVER_DB_DSN" -X -v ON_ERROR_STOP=1 \
  -f backend/db/agent_cutover_inventory.sql > "$GLOWLOOM_CUTOVER_INVENTORY_FILE"
```

PG可以拒绝本库已知非终态/未知记录，却不能证明外部世界全部结束。须逐个核验旧部署的所有Temporal namespace、task queue、schedule、workflow/run（包括管理员trial与 `agent.codex` 的外部历史）；与盘点中的 `workflow_id`、供应商 `provider_task_id/request_key`、原operation及回执逐一对应。还要核验本库无记录或本地失败后仍在远程运行的请求、对象写入、供应商最终费用、预留/结算与账本。任何运行中/待派发/未知接单/费用未知都不能签发清结证明。PG中的未知值必须先经正式业务对账清结，外部证明不能覆盖PG阻断。

需要五份不可变证据的SHA-256：完整PG备份及恢复验证、最终只读盘点、独立外部执行/费用清结、旧聊天/记忆删除的明确授权、停写和其他连接已关闭的记录。外部清结文件应写明目标库、部署/版本、Temporal各namespace与队列、原请求与最终账单/对象回执、核验人和核验时间。没有实际外部核验时不填写 `all-old-executions-and-charges-closed`。哈希只绑定人工审阅文件；脚本不会自动访问或认证这些文件，更不会替操作员确认外部事实。

## 审阅后执行

以下命令是交付物，不表示已授权在业务库运行。五个hash变量必须为真实证据的64位小写十六进制值；`GLOWLOOM_CUTOVER_DATABASE` 必须精确匹配目标库名，`GLOWLOOM_CUTOVER_CLEARANCE_AS_OF` 是30分钟内重新核验的ISO8601时间。不要用占位证明、旧盘点或合成fixture证明执行生产切换。

```sh
psql "$GLOWLOOM_CUTOVER_DB_DSN" -X -v ON_ERROR_STOP=1 \
  -v cutover_database="$GLOWLOOM_CUTOVER_DATABASE" \
  -v clearance_as_of="$GLOWLOOM_CUTOVER_CLEARANCE_AS_OF" \
  -v backup_sha256="$GLOWLOOM_CUTOVER_BACKUP_SHA256" \
  -v inventory_sha256="$GLOWLOOM_CUTOVER_INVENTORY_SHA256" \
  -v external_clearance_sha256="$GLOWLOOM_CUTOVER_EXTERNAL_CLEARANCE_SHA256" \
  -v history_drop_authorization_sha256="$GLOWLOOM_CUTOVER_HISTORY_AUTHORIZATION_SHA256" \
  -v stopped_writes_sha256="$GLOWLOOM_CUTOVER_STOPPED_WRITES_SHA256" \
  -v external_clearance_state=all-old-executions-and-charges-closed \
  -f backend/db/agent_cutover.sql
```

不要给该命令增加 `--single-transaction`：脚本已有完整事务。不要把带 `BEGIN/COMMIT` 的六份新表片段直接 `\ir` 到迁移中，否则片段的COMMIT会提前提交删除前缀。迁移已按源哈希冻结这六份片段的正文并移除独立事务包装；源片段变化后必须重新审阅、同步正文和hash、再演练。四个Agent片段（含 `agent_conversation.sql`）与 `task_runtime.sql`、`operation_task.sql` 之外的对象存储/头像等迁移不在本脚本范围，须按各自前置条件独立审阅，不能认为此脚本覆盖整份当前空库schema。

成功后保留命令输出中的库名、切换ID与五份证据hash，并独立核验旧表均不存在、Agent运行表与业务表互相拒绝跨角色访问、保护业务记录不变，再启动当前四服务拓扑。新任务表只创建当前任务合同；不会从旧Temporal历史填充旧任务。内容删除周期保留 `next_run_at='infinity'`，仍需显式启用。

## 隔离演练边界

用新建的回环 `*_test` 库导入 `git show HEAD:backend/db/schema.sql` 的旧完整结构，再按需要导入HEAD的 `agent_checkpoint.sql`，不要对任何已有业务库重放旧/新完整schema。可创建随机账号/项目/素材/账本sentinel和终态旧Agent记录，逐项注入非终态turn/run、未知admission/operation/provider、held预留、旧业务目标、外来FK/未知对象/其他连接，确认非零退出及旧结构/业务数据保持。还须注入新Agent对业务表的异常授权，让最终权限校验失败，以验证已执行删除/创建前缀也会整体回滚。

只有所有fixture清结后允许一次成功；比较账号/项目/素材/operation/账本的完整行指纹，实测新旧角色权限，再运行一次必须因已安装新对象而拒绝。演练只能证明SQL门禁和原子性，不能证明真实Temporal、供应商、费用、备份恢复或现有部署接受。

转存的物理尝试模型由 [media_transfer_attempt.sql](media_transfer_attempt.sql) 单独切换：旧转存对象事实、活动任务及未结束/未知债务必须先按正式流程全部收敛；脚本拒绝回填或改写旧绑定。本 Agent 切换脚本不执行该存储迁移。
