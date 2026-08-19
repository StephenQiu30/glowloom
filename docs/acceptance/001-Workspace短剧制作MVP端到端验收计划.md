# Workspace 短剧制作 MVP 端到端验收计划

- 文档类型：Acceptance Plan（不是通过证据）
- 状态：planned
- 日期：2026-08-19
- 需求范围：CUR-00、CUR-IAM、CUR-PRJ、CUR-SCR、CUR-AST、CUR-SBD、CUR-KFR、CUR-VID、CUR-EXP、CUR-PLT、CUR-SEC、CUR-PROD
- 当前结论：尚未执行，不得据此宣称功能可用或 verified

## 1. 验收目的

本计划证明的是：一个有权用户能在同一 Workspace 和 Project 中，从 DOCX/Markdown 整剧原稿开始，经过可恢复的 Production Harness，得到经人工确认的深度剧本理解、正式资产、分镜、逐镜视频主选和固定素材包；所有状态、阻塞、版本、任务和失败均可解释且不会跨 Workspace 泄露。

文档链接、Mock、单元测试源码、页面静态展示和 LangGraph 单次成功都不能替代本验收。

## 2. 被测环境与固定证据

执行时必须登记：

- commit SHA、分支、执行时间、执行人；
- API/Worker/Web 版本与 workflow/skill/schema version；
- PostgreSQL、RabbitMQ、对象存储、Codex/备用 Provider 的实际状态；
- 测试 Workspace A/B、Project、ProductionRun、DocumentRevision、Episode、StageRun、WorkTask、Media 和 ExportSnapshot 的稳定 ID；
- 每条用例的命令、响应/页面截图、数据库或只读 Query 证据、日志/trace ID 和清理结果；
- 外部 Provider 未运行时明确标记 blocked，不以 fixture 成功替代真实能力。

黄金剧本：`He Left Our Kids to Drown—He Didn’t Know I Was the Empress.docx`，预期确定性事实为 139,723 字符、60 个显式分集、131 个场景头和完整 source range；数值变化必须先解释解析规则/fixture 变化，不能静默更新期望。

## 3. 主成功流程

| 步骤 | 操作 | 必须观察到的结果 | 主要追踪 |
| --- | --- | --- | --- |
| E2E-01 | 用户登录并选择 Workspace A | 会话恢复后当前 Workspace 可见；Workspace B 资源不出现在列表/搜索 | CUR-IAM、AC-CUR-PROD-011 |
| E2E-02 | 只填写名称/简介创建 Project | 创建一个 Project 和一个 current Project ProductionRun；不创建占位 Episode；下一动作是导入 DOCX/MD | AC-CUR-PRJ-001、AC-CUR-PROD-001 |
| E2E-03 | 上传 DOCX | 先显示安全检查、文件名和内容预览；不要求勾选非必要权利复选框或填写语言/预算/画幅；未确认前不创建 Episode | CUR-SCR-FR-001/002 |
| E2E-04 | 确认 DocumentRevision 与分集计划 | 60 集边界 gap=0、overlap=0；原子建立 60 个 Episode/ScriptVersion；任一冲突时零部分成功 | AC-CUR-SCR-003/006/007、AC-CUR-PRJ-005 |
| E2E-05 | 启动深度剧本理解 | 使用固定输入、Skill/schema version 和 project/episode StageRun；刷新/重登恢复同一 WorkTask/StageRun | CUR-SCR-FR-012、AC-CUR-PROD-012 |
| E2E-06 | 查看深度理解结果 | 60 集都有非占位语义标题、logline、摘要、hook、关键节拍和合法 source range；人物、世界观、场景、对白、资产、镜头、连续性、任务建议均为显式类型候选 | AC-CUR-SCR-017/018 |
| E2E-07 | 审核候选 | 可从候选跳回原文；接受/修改/关联/忽略均记录 actor、版本与来源；AI succeeded 与人工 Gate passed 分开显示 | AC-CUR-PROD-004、CUR-SCR 候选 AC |
| E2E-08 | 物化共享资产 | 重复人物/地点/道具归一；正式资产由 CUR-AST 命令创建，跨集引用共享稳定身份；未决候选不冒充 ready | CUR-AST、CUR-PROD-FR-004 |
| E2E-09 | 推进两个 Episode | Episode 1 满足依赖后进入分镜；人为阻塞 Episode 2；Episode 1 继续而 Episode 2 保持 blocker，Project 快照分别汇总 | AC-CUR-PROD-002 |
| E2E-10 | 生成并确认分镜 | NarrativeUnit 覆盖守恒、Shot 顺序和 ShotSpec 固定；AI 提示不直接成为正式 Shot | CUR-SBD、AC-CUR-SCR-016 |
| E2E-11 | 准备视觉参考并逐镜生成 | 每个 Shot 固定输入；候选媒体已持久化、探测和 hash；单镜失败不删除同批其他成功镜头 | CUR-KFR、CUR-VID、CUR-PLT |
| E2E-12 | 审核并选择视频 | 星标与主选分离；每镜最多一个当前主选；拒绝当前主选进入 needs_reselection，不自动补位 | CUR-VID、CUR-00 不变式 |
| E2E-13 | 预检和冻结素材包 | `complete` 范围无缺镜/兜底/stale；ExportSnapshot 固定顺序、主选和媒体版本；PackageBuild 产生 Manifest/hash | CUR-EXP、AC-CUR-PROD-010 |
| E2E-14 | 修改上游并重下历史包 | 只影响相关 Episode/Shot StageRun；历史 Snapshot/Build 不漂移；同 Build 重下 hash 一致，重建形成新 Build | AC-CUR-PROD-007、CUR-EXP |

## 4. 必须通过的失败与恢复流程

| 用例 | 故障注入 | 通过标准 |
| --- | --- | --- |
| FAIL-01 | DOCX 为空、伪扩展名、损坏 ZIP/XML 或压缩炸弹 | 不创建 DocumentRevision/Episode；显示可定位错误；服务不崩溃 |
| FAIL-02 | 深度 Skill 返回非法 JSON、越界 source range、悬空 candidate key | Stage/Gate 不 passed，不产生正式对象，显示失败范围与安全重试 |
| FAIL-03 | 121 Chunk 中一个失败 | 整体不冒充完整成功；其他已提交项目事实不回滚；重入只针对固定输入 |
| FAIL-04 | Provider submit 超时且可能已受理 | WorkTask 进入 unknown/对账；Worker/API 重启后不产生第二 submit |
| FAIL-05 | 重复 Outbox、回调和用户提交各 100 次 | 不产生第二 Task/Attempt/Candidate/Decision/StageRun 或双重外部消耗 |
| FAIL-06 | 页面刷新、断网、会话续期和 Worker/Broker 重启 | 回到同一 ProductionRun/StageRun/Task；已确认事实不丢失，未确认状态明确 |
| FAIL-07 | 上游 ScriptVersion 在页面打开后变化 | 旧 Gate 决定零写入或 expired；受影响范围 stale，未受影响 Episode 不变化 |
| FAIL-08 | 某摘要服务不可用 | 该分区显示 unavailable/最后成功时间，其他分区继续可用，数字不伪造为 0 |
| FAIL-09 | 当前主选被 rejected | Selection 进入 needs_reselection，导出 blocked，不自动选择星标/最新候选 |
| FAIL-10 | 打包中断或对象存储短暂不可用 | 固定 ExportSnapshot 保留；恢复不重新生成视频、不更换主选、不覆盖旧 Build |

## 5. 安全、隔离与治理流程

1. Workspace A actor 对 Workspace B 的 Project、Run、Stage、Document、Candidate、Task、Media、Export 的列表、直接 ID、事件、恢复和下载成功数均为 0。
2. Membership 在页面打开后撤销，下一次写入、Provider submit、主选和下载均重新校验并拒绝。
3. 权利、内容安全或 Provider 数据处理决定 unavailable/expired 时，新的外部副作用 fail-closed；历史只读按策略处理。
4. 普通日志、指标、事件和错误响应中的完整剧本、Prompt、媒体签名 URL、Provider 原响应和凭据出现数为 0。
5. 删除、归档、Gate 拒绝、候选决议、主选和交付冻结可关联 actor、固定输入、时间、理由和 AuditRecord。

## 6. 页面和体验验收

- 全局壳层始终显示当前 Workspace，支持切换；Project 页面显示当前 Project，不把“资产”放在全局顶栏中跨项目混用。
- 项目首页以项目卡片进入；进入 Project 后才显示完整制作阶段、Episode 和项目内资源。
- 工作台不显示项目预算、币种、语言、目标时长、画幅默认值、已导入剧本侧栏、非必要勾选项或单集先建入口。
- 主流程首阶段为“剧本导入与解析”；每个阶段只显示真实状态、blocker、下一动作和来源新鲜度。
- 不出现“成片、声音时间线、字幕编排、音频后期、预算管理”等当前范围外承诺。
- AI 完成、候选待审、正式就绪、流程完成、任务 unknown 和数据 unavailable 必须以文字区分，不只依赖颜色或百分比。
- 1280px 及以上无固定头部遮挡；小于 1024px 至少保持当前 Workspace/Project、阶段、blocker 和主动作可用。
- 创建项目、上传、预览、候选审核、继续制作、处理 blocker、主选和导出可仅用键盘完成，满足 WCAG 2.2 AA。

## 7. 通过规则

- 主成功流程 E2E-01～14 全部通过；任何一步 skipped/blocked 都不能判定端到端通过。
- FAIL-01～10、安全隔离 1～5 和页面体验要求全部有真实证据；隔离、重复副作用、历史漂移、敏感泄露容忍数均为 0。
- 真实图片 Provider、真实视频 Provider 和真实持久对象存储至少各完成一次成功、失败/unknown 和恢复路径；否则对应 Gate 保持 blocked。
- 60 集黄金样本必须完成深度语义完整性验收；仅证明确定性分段/Chunk 重组不等于深度解析通过。
- 通过后另建带 commit、环境、命令、证据和残余风险的 Acceptance Evidence；不得把本计划状态改成 evidence。

## 8. 当前已知阻塞

1. 当前代码尚无 CUR-PROD 定义的 Project ProductionRun、显式 scope StageRun 和 ProductionGate。
2. 当前项目快照仍从局部摘要硬编码推断阶段，不能作为 Production Control 唯一事实。
3. 当前深度解析实现与计划仍需迁移到显式类型候选并完成 60 集真实全量调用。
4. 当前公开入口仍承诺“成片、费用、声音时间线/字幕编排”，与当前 Requirement 非范围冲突。
5. 当前 Acceptance 目录没有任何已执行的真实通过证据。
6. Workspace 管理页面和数据隔离已存在，但当前 Project 壳层没有把“当前 Workspace + 可切换入口”作为持续可见的一等上下文，尚不能通过 E2E-01 与页面体验验收。
