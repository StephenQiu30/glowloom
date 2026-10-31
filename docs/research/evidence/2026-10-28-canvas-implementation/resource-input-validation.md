# 同节点生成输入：独立审计与真实数据库验证

日期：2026-10-28。依据设计27 §7.4.1，审查 text/image/video/audio 的 `config.draft.composer`、权威 `input_revision`、`node_input`。这是后端合同验证，不代表真实模型生成、结果采用或前端产品验收完成。

## 修正

1. `IsDraft` 扩展为四类资源输入所有者后，删除误走旧生成意图节点的专用分支，跳过普通资源的时间线/导演引用清理及保护消费者检查。新增回归测试先确认删除报 `invalid canvas command`，再以私有 `isGenerationDraft` 只区分旧 image_generation/video_generation 的删除规则；资源保留既有删除语义。时间线删除后稳定 asset_id 保留、node_id 清除。
2. 正式 `prompt/postgres.ReadInput` 原先只检查选中 composer，真实 PostgreSQL 负例证明未知配置、未知其他节点与未来 schema 仍能进入生成。增加文档锁内的 schema 与整图 `ValidateGraph` 门禁；quote/confirm 均拒绝只读图。项目授权继续由正式 `promptwire.DraftAccess` 处理，不使用允许所有调用者的测试替身。

## 已运行证据

使用新建一次性 `postgres:18-alpine` 容器 `glowloom-canvas-input-qa-20261028`，绑定本机随机端口，独立数据库 `glowloom_reference`，从仓库 schema.sql 初始化。写入角色为 `canvas_test_runtime` 登录后切换的 `glowloom_app`；测试明确断言 current_user。未读取任何 .env、未清空或修改已有服务数据库。测试编辑经过真实 Node/Yjs 二进制更新与正式 PostgreSQL 接受事务；不是内存仓储测试。

- `go test -race ./tests/canvas -run '^TestResourceInput' -count=1 -v`：通过，12.004s。
- `go test -race ./tests/canvas -count=1 -json`：通过，173.134s；354 个测试/子测试均 pass，0 fail、0 skip。四个既有 canvas/http/copy/runtime DSN 全指向上述新建隔离库。
- `go vet ./internal/canvas/... ./internal/prompt/... ./tests/canvas`：通过。
- `golangci-lint run ./internal/canvas/... ./internal/prompt/... ./tests/canvas`：初次报测试变量覆盖 builtin copy，改名 duplicate 后 0 issues。
- goimports（含项目 local prefix）与 gofmt：作用于本任务 Go 文件。
- `govulncheck ./internal/canvas/... ./internal/prompt/...`：0 可达漏洞；工具另外报告依赖模块中 1 个不在当前调用路径的漏洞，不把该模块提示写为“全部依赖无漏洞”。

原始终端日志保留于本机 `/tmp/resource-input-race.log`、`/tmp/resource-input-canvas-full-race.json`、`/tmp/resource-input-admission-red.log`。这些是本轮运行证据，未将临时数据、凭据或生成二进制纳入 Git。

## 具体覆盖

| 边界 | 实际断言 |
| --- | --- |
| 四类同节点 | ID/type/action 不变；先有结果再添输入；文本规范化后的正文/Tiptap保持；首次输入为1 |
| 保存/重开 | 新 service/store 读回完整 composer；正式 ReadInput 返回该保存来源 |
| 不可变快照 | 首次一条；layout不增；参数修改仅增一次；原输入行不可变由既有全套测试覆盖 |
| 重放/CAS | 持久receipt重放相同结果、不重复快照；旧输入CAS拒绝；input_revision不从客户端伪造 |
| quote/confirm | quote要求精确canvas_revision；只布局变化confirm允许；输入变化confirm拒绝 |
| 删除/恢复/本地复制 | 删除不增快照；恢复同ID并推进输入版本；新ID复制从revision1建立独立快照 |
| 固定标签 | 不活跃tombstone不推进input；ordinal回退拒绝；正文/名称/背景变化不推进input |
| 只读/未知 | 未知类型、旧字符串draft混填、受保护执行节点编辑拒绝；未来schema/未知配置/未知图节点不准生成 |
| 正式权限 | 异账号、异项目读取拒绝；归档项目quote与confirm拒绝 |
| 事务回滚 | 项目特定审计触发器注入失败后，权威Yjs字节、文档revision、资源composer均保持；无新node_input或成功receipt泄漏 |

## 仍需验证与真实服务条件

- `CopyProjectDocument` 对带版本化输入的资源节点仍显式 fail-closed；它没有输入来源跨项目映射合同。不能偷偷丢弃 composer，也不能把本地节点复制测试当作跨项目复制已支持。
- `LastOperationID` 受保护业务投影仍不可直接编辑。本轮没有放宽它，也没有证明所有生成结果采用路径都回写同一资源节点。
- 真实模型的报价、确认、供应商运行、失败恢复、最终媒体采用需要有效模型/项目服务配置及实际任务回执。本轮测试只能证明保存输入可被正式 ReadInput 读取，不证明供应商输出。
- UI 编辑/刷新/两客户端用户操作验收由根任务继续执行；本地测试替身生成的Yjs更新不等于已经检查浏览器输入法与鼠标手势。

2026-10-29收尾：只读整图拒绝同时保留 `ErrGraphReadOnly` 与既有 `ErrInputStale` 错误链，因此正式quote/confirm映射HTTP409源冲突，不落入503依赖不可用。该负例定向 `-race` 再次通过（3.965s）。隔离PostgreSQL容器已停止并自动删除。
