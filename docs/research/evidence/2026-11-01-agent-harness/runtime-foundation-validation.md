# 创作 Agent 运行底座验证

日期：2026-11-03。范围：[Plan28](../../../plan/28-完整创作Agent与Skill多Agent实施计划.md) T-AH-01，依据需求 SRC-AGH-07 使用 PostgreSQL。实现入口：[运行时说明](../../../../agent-runtime/README.md)。当前调度与后续工作只在 [BACKLOG](../../../../BACKLOG.md) 维护。

## 环境与数据边界

- 实际 Node `24.19.0`、pnpm `12.4.2`、耐久 Harness SDK `1.3.0`，依赖固定在运行时锁文件。
- PostgreSQL 实际为 `18.6`，使用本轮单独创建的临时容器和隔离测试库；没有读取业务环境凭据或迁移业务库，没有重启项目应用。
- 全部会话、输入、任务、memos、检查点与压缩回执保存到 PostgreSQL。SDK 公共内存实现仅作为校验与重建索引；不使用 SQLite/本地文件数据库。
- 模型出口使用受控 HTTP 网关，不访问真实供应商、不产生真实账户费用。子会话测试工具只安装在测试中，产品 Host 没有通用 shell/文件工具或测试工具。

## 执行与结果

| 检查                                                      | 实际结果                                                            |
| --------------------------------------------------------- | ------------------------------------------------------------------- |
| `pnpm --dir agent-runtime install --frozen-lockfile`      | 通过；未运行依赖生命周期脚本                                        |
| `pnpm --dir agent-runtime typecheck`                      | 生产代码与测试类型检查通过                                          |
| `pnpm --dir agent-runtime test`                           | 构建成功；7 个测试文件、50 项全部通过，无跳过                       |
| 上游 Storage conformance                                  | 完整 24 项通过，使用 `glowloom_agent_runtime` 受限角色              |
| 隔离库执行完整 `backend/db/schema.sql`                    | PostgreSQL 18.6 空库初始化通过                                      |
| 隔离库重复执行 `backend/db/agent_harness_incremental.sql` | 通过，不重放全库结构                                                |
| `go test -race -count=1 ./tests/db`                       | 设置隔离空库 `GLOWLOOM_TEST_SCHEMA_DB_DSN` 后通过；不是 Go 全仓测试 |
| 格式、文档相对链接与 `git diff --check`                   | 在交付前按当前文件核对                                              |

覆盖的实际故障与合同：

1. 第二写者被 PostgreSQL advisory lock 拒绝；锁释放后递增 epoch；连接丢失后拒绝旧内存索引读写。
2. ID 分配与提交串行，未提交的已分配 ID 在重启后不复用；SDK 支持的 NUL/特殊字符串无损保存。
3. 真实 PostgreSQL COMMIT 完成后注入确认丢失：旧写者关闭，重新打开读取数据库已提交结果，不重建第二条提交。
4. 重投同命令恢复同一 submission；不同输入同 request_id 冲突；每次重放重新检查 Go 授权。
5. 子进程真实 SIGKILL 后恢复原 generation taskId/memo/exchange；网关已保存的结果重放，物理请求计数不增加。压缩交换确认中断后也恢复原 taskId/memo。
6. generation Hook 拒绝、PostgreSQL 在模型授权等待期间断开，都不能继续发起模型交换；压缩不会绕过同一网关。
7. 压缩拒绝 length、toolUse 和空白摘要，保留原始历史；终态、关闭并重新打开后仍可读取压缩 usage 回执。
8. 两个真实 SDK task-owned 子 conversation 使用独立上下文、工具与交换身份；父 abort 收敛两个子会话。此项是 SDK 接入合同，不代表专业 Agent 业务服务已交付。
9. Host 校验签名过期、method/path/body、载荷摘要和身份；并发投递只产生一个提交；关闭期间迟到的 body 不再开新会话；旧 owner drain 无法关闭新 owner；失效缓存清除后可重投原命令恢复。
10. 编译入口可导入；缺少显式内部配置时拒绝启动，环境中的供应商 Key 不会被自动发现或输出。

在完整空库中核对独立角色的实际权限：项目 SELECT/UPDATE 均为 false，SDK 提交 INSERT 为 true、UPDATE 为 false。应用启动不创建角色、不执行 DDL；正式登录只继承 SDK 存储权限，不使用 Go 业务账号或管理员账号。

## 失败与修正记录

首轮上游合同暴露 jsonb 无法保存全部 SDK 字符串、ID 水位不完整；改为 PostgreSQL bytea 保存 SDK 提交写集，并原子分配 ID。并发测试随后复现 ID 分配插入尚未提交事务的问题，已通过连接操作串行化修正。

独立审查提出数据库故障到模型出口的取消传播、不完整摘要采用、关闭时迟到请求和旧 drain 重放四项问题。均先补失败断言，再修复并通过对应回归；另补两子 conversation 合同、持久压缩回执和失效缓存恢复。审查者对最终增量只读复核，无未关闭问题；测试执行结果来自实施方，不冒称独立重复运行。

## 接受限制

本报告只证明 T-AH-01 的 SDK/存储/宿主与身份合同。Go 生产控制账本、规划/执行批准转换、模型物理交换与根预算、业务工具、Skill 生命周期/沙箱、专业多 Agent、剧本/分镜/成片与 UI 均继续按后续任务完成。当前没有开启产品准入或接入 Compose 运行服务。

恢复采用按会话分批回放提交日志和 SDK 内存索引，未接受容量 SLO、PITR/跨主机自动接管。Go dispatcher/owner 恢复、真实供应商计费、专用 Linux 沙箱与人工创作接受尚未验证；本地测试不能替代这些结果。本次未提交、推送或触发远端 CI。
