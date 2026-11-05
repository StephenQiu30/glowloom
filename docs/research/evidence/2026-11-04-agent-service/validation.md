# 独立 Agent 服务工程验证

终验日期：2026-11-05。当前合同：[设计32](../../../design/32-完整创作AgentHarness与Skill多Agent架构设计.md) → [PRD19](../../../prd/19-完整创作Agent与Skill多Agent.md) → [Plan28](../../../plan/28-完整创作Agent与Skill多Agent实施计划.md)。会话输入与历史合同补充见[设计35](../../../design/35-对话工作台与参考输入一致性设计.md)。实施及产品接受状态仅见 [BACKLOG](../../../../BACKLOG.md#pi-durable-migration)。

## 验证结果

| 检查                                                   | 实际结果                                                                                 | 边界                                                                                    |
| ------------------------------------------------------ | ---------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------- |
| `go test -race -count=1 -timeout=20m -json ./...`      | exit 0；176 包完成编译，其中37包执行测试；4478个测试/子测试通过，0失败                   | 1919个顶层测试通过；139包无测试文件；57项已有条件测试跳过，逐项见下文                   |
| 前端 `vitest run --maxWorkers=2`                       | 295文件、2099项通过，0失败                                                               | 包含官方API重生成核对、会话输入/消息/恢复和当前页面夹具                                 |
| 前端 ESLint、Prettier、next typegen、tsc、Next生产构建 | 全部通过                                                                                 | 最终格式修复只改排版，后续单独完整格式检查通过                                          |
| Agent strict、ESLint、Prettier、api:check、build、test | 全部通过；24文件、134项，无跳过                                                          | 真实隔离PostgreSQL、官方Storage/Harness、实际子进程与SIGKILL；供应商响应为确定性fixture |
| 私有协作运行时                                         | 106项通过，0失败、0跳过，格式检查通过                                                    | 本地Node测试，不代替多人完整浏览器接受                                                  |
| Go质量门禁                                             | 全仓build/vet/lint通过，gofmt/goimports差异为空                                          | golangci-lint v2.15.0，未关闭规则或新增nolint                                           |
| govulncheck                                            | 0可达、0导入包漏洞                                                                       | 1个仅required module中的未调用公告；本轮未升级无关依赖                                  |
| Compose和Agent镜像                                     | 唯一Compose配置校验、镜像构建通过                                                        | 未启动现有backend/frontend执行应用切换                                                  |
| 当前镜像实际运行                                       | 非root node、read_only、1GiB/1CPU；health/ready 200；官方Harness完成1步模型、5事件含文本 | 独立验证容器使用真实隔离PG及本地HTTP回执fixture；无收费供应商调用                       |
| 镜像SIGTERM                                            | exit 0，活动运行租约0；验证容器已移除                                                    | 未操作现有应用容器配置                                                                  |
| 当前SQL                                                | 新空库初始化成功；8运行表/8业务表角色隔离；6份冻结片段正文/hash一致                      | 只在专属测试实例执行DDL，未迁移现有业务库                                               |

机器可读结果见 [checks.json](checks.json)。完整命令、失败反例、终验JSONL及分域审查位于 `/Users/stephenqiu/.codex/validations/glowloom-agent-20261105/`；此路径不包含业务环境文件或业务凭据。

## 已复现并修复的问题

- 测试改用当前独立Agent服务、rich draft输入、分页会话/消息和正式物理存储准入；没有恢复旧构造器、阶段接口或兼容分支。
- 转存采用追加的物理output attempt，逻辑素材/条目身份与原路由保持；成功项不重写，旧租约不接受，关闭的写入意图不重开。实体排队取消进入可靠清理，逐项取得真实缺失/删除证明后结束；未知债务保留待核对。
- 主体来源SQL显式约束bigint参数，个人上传与当前空间本人发起的生成来源按各自权威范围查询；未把团队来源缩成个人空间以掩盖失败。
- profile fixture通过正式RegisterProfiles计算不可变resource identity；删除伪造hash。Script配置版本使用严格正十进制单调序列；单体计划超限在数据库准入处验证23514及事务无残留，聚合/数量guard仍由非owner角色验证。
- App/身份夹具使用独立数据库，分区维护使用三个独立登录角色及受限role-switch连接；不靠拉长锁超时或模拟权限通过。
- TypeBox生成命名Go对象，Swagger与前端保留optional及显式nullable。无提案运行允许省略proposal，pending_proposal允许声明的null；不手改生成物或启用全局可空来绕过类型检查。
- 音频取消等待数据库实际渲染阶段；Prune验证全局有界清理。所有原权限、回执、旧租约、不可变绑定、重试和未发布断言保留。

## 原始失败与复验说明

2026-11-04记录的旧接口编译、上传object_location_id、搜索历史策略及初始全量失败不再代表当前结果。原始需求和历史证据保留；本文以2026-11-05完整终验替代旧的“全仓未通过”描述。

初轮真实Race暴露了数据库角色配置错误、UTC时区字面假设、媒体metadata遗漏、旧版本号、错误物理身份和scope参数，均先保留RED再修复。一次同时运行Go、前端和镜像构建时，Agent的1200ms故障夹具租约到期；停止并发重型检查后按原租约、原超时独立重跑完整Agent套件，134项全部通过，未修改Agent恢复场景的租约长度、断言截止时间或生产超时。

原始检查中前端格式曾失败；仅排版修复后完整Prettier复验通过。全仓Go和前端终验后，仅删除了测试清理中的重复DELETE并补格式，另对当前Go/Node边界做定向Race复验。

## 条件型跳过与接受边界

57项原有条件测试的名称及原因见 [go-skips.json](go-skips.json)。包括Kafka/relay/audit受限部署、现有本机完整依赖/OTLP、真实COS、显式Codex CLI/收费推理与交互浏览器、12项原生VDA包、5项真实Whisper服务/官方样本、1项官方FCPXML DTD，以及未配置的专项连接。没有新增skip、宽松断言或替代真实接受的假成功。

真实PostgreSQL、受限角色、Redis Lua、MinIO与本地TOS协议fixture的原生SDK、FFmpeg、Go/Node生产HTTP边界与官方Pi故障恢复已经执行。真实供应商效果、账单清结、长剧本到可编辑成片、完整登录浏览器接受、线上CI和既有库离线切换不由这些fixture结果代替。

## 环境与交付

使用Node 24.22.2、Go1.26.10、PostgreSQL18和锁定依赖。每次全仓重跑使用新建专属数据库实例，保留旧失败夹具事实，不篡改不可变数据或删除未知债务。没有读取业务.env或向其他聊天发送消息。

恢复环境时启动Docker Desktop，已有restart策略自动恢复了部分现有容器；这是环境副作用，未更改其配置，也未用本轮实现执行现有业务库DDL。之后验证使用专属原生PG/Redis/MinIO和独立Agent验证容器。

本轮专属PG、Redis、MinIO和HTTP fixture进程均已停止，失败数据和日志保留；验证容器已移除。

工程终验对应提交前快照；后续提交、远端同步与CI结果以实际Git交付为准。共享工作区逐路径状态与归类见 [working-tree-paths.tsv](working-tree-paths.tsv)。

## 提交范围冻结复验

本次Git交付仅包含独立Agent迁移、当前合同及全仓测试修复的必要依赖。独立探索搜索功能、导演台静态资源排版和冻结后出现的并行修改留在工作区。

提交候选快照重新通过前端293文件/2095项测试、类型检查和生产构建；相较上述共享工作区全量记录，少的4项属于未纳入的独立探索搜索测试。独立Agent24文件/134项再次全部通过；Go Agent HTTP、真实业务/Yjs与跨语言边界定向Race通过。子进程启动失败时的测试诊断读取先停止并等待进程，消除stderr读取竞态；该测试夹具另通过格式、vet和lint。初次快照缺少私有协作运行时依赖而暴露此失败路径，补齐锁定依赖后按原断言复验。

完整原始输出保留在前述验证目录的 `frozen-*` 日志。Git提交、远端同步和线上CI状态以交付时的实际结果为准。
