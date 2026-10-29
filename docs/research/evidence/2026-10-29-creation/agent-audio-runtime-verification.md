# Agent 与全能配音运行验证（2026-10-29）

## 事务故障与修复证据

真实 PostgreSQL 18 隔离实例，端口55489、数据库glowloom_creation_verify。旧事务边界在3秒 deadline 内复现 `lock authentication account: timeout: context deadline exceeded`。原因是会话事务先持有账号/项目锁，再通过池中的另一个连接调用 operation；第二个事务等待前一个事务释放锁。

会话 Store 现向转换回调提供绑定当前事务的 Operations，生产 wiring 通过 `BindTransaction` 保留原有依赖。报价、确认、预留与会话状态共同提交。补修所有权判断中的收据字段 `OperationID`（内部 JSON 使用该字段，不能误读 HTTP 的 operation_id）。

最终 `GLOWLOOM_TEST_OPERATION_STORE_DB_DSN=... go test -race ./tests/operation -run TestAgent -count=1 -v` 通过3例：真实会话存储/私有读取；报价→确认→重复确认只有一笔 reservation；在真实确认写入后注入错误，reservation和会话一起回滚；运行阶段读取；项目内非所有者读取/确认/取消拒绝。权限测试用宽松项目port隔离检验操作所有者检查，主流程使用真实账号/项目权限锁。

## Figma 与代码

Figma file uLqmeRWuQuzfrp9xOYg6FT。全能配音四态3211:63326、3211:63408、3211:63490、3211:63572；Agent五态3217:60567、3217:60772、3217:60925、3217:61078、3217:61231。设计写后回读，截图已查看。首个Agent为受限JSON规划的单图链路，每一阶段显式确认，并非任意工具平台。

全能配音接入 Seed-Audio 1.0 精确协议，独立模式、参考媒体服务端校验、@音频物理编号、120秒最大预留、original_duration小数秒结算、审核后字幕与SRT导出。HTTP fixture协议验证不等于真实供应商生成。

Agent按需加载，未发送目标接回原有账号/空间/类型草稿机制。该机制为页面内存；切换类型能保留，整页刷新不恢复未发送文本。已经提交的会话存储于数据库，刷新后重新选择项目可读取。

## 业务数据库与运行进程

先保存catalog能力/预设/版本的无凭据数据快照，再以单事务、5秒锁超时、事务咨询锁应用 `omni_audio_incremental.sql` 和 `agent_session_incremental.sql`。before/after：用户38→38，项目26→26，资产72→72，预设版本8→9，Agent表不存在→存在。数据快照有循环外键提示，不能称为独立完整恢复包。

本轮后端于05:14:50 UTC重启；健康端点返回ok。共享环境随后发生另一次并行重启（05:21:02 UTC），恢复healthy后继续验收。最终业务库只读检查：验收项目1条Agent会话、0条Agent操作、语音预设2个版本。未启用模型、未调用真实供应商或扣费。

## 实际页面证据

Chrome真实登录localhost1122，经正式BFF→Go→业务PostgreSQL，无API mock。创建Agent会话成功，刷新、重新选择同一项目后重新打开；Agent文字→图片空草稿→Agent原文保持。全能配音进入项目编辑器、填写台词与海浪描述、打开参考菜单，切换数字人/动作/音乐/视频后返回，文字与全能配音模式保持。数字人为角色图片/讲话音频，动作模仿为角色图片/动作视频且无正文，音乐显示歌曲，图片和视频为正式多参考编辑器。

真实模型菜单显示暂无匹配的可用模型。不能由此验收任何真实生成、最终供应商账单或字幕落库。未添加用户人物素材。

截图：父工作区 output/jimeng-creation-2026-10-29/agent-runtime.png、omni-audio-runtime.png、agent-design.png、omni-audio-design.png。

## 工程检查

- Agent、providerhttp、managed、catalog、prompt专项Race通过；四类候选真实PG持久化/成本/保持禁用回归通过。
- 目标Go vet通过；最终golangci-lint目标目录0 issues。全仓lint另有并行artifacts/media/canvas错误，不冒充全仓通过。
- govulncheck可调用漏洞0，模块中另有1个未调用漏洞。
- Agent/字幕/类型合同3文件8测试通过。首页13项：重跑12项通过、六类型循环曾20秒超时；该循环单独重跑通过（12项筛除），未修改测试断言，单独运行时允许60秒，实际测试部分约6秒。此前并行5秒超时照实保留。
- 目标ESLint退出0、范围diff check通过。全量tsc仍失败：canvas/yjs-live-session.test.ts的mode空值类型、subjects/unified-library.test.tsx和unified-upload-return.test.tsx的Testing Library exact参数，均未在本轮改动。没有声称最终生产构建通过。

本轮未新提交或推送，工作区逐文件保留清单在父工作区output/jimeng-creation-2026-10-29/agent-audio-worktree-manifest.md。后续范围与优先级仅见BACKLOG。
