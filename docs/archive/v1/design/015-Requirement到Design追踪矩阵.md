# DES-015 Requirement → Design 追踪矩阵

- 状态：proposed
- 版本：v1.2
- 日期：2026-08-19
- 上游：[Requirement 索引](../requirement/README.md)、DES-000～015、DES-017
- 用途：证明每个当前 Requirement 有明确 Design 落点和可执行验证；不复制需求正文

## 1. 使用规则

1. Requirement 是“要什么”的事实源；Design 是“如何满足”的事实源。本文仅存追踪关系，不用摘要替代原 FR/NFR/AC。
2. **主设计**拥有该 Requirement 的业务模型/主流程；**支撑设计**只提供身份、任务、媒体、治理、NFR 或跨模块契约。
3. 映射为连续区间时，包含起止之间全部 ID；区间中的任何新增 ID 必须再评审，不当然自动覆盖。
4. 所有 NFR 均有业务模块 Design + DES-014 的双落点；所有高风险 AC 还必须落 DES-003/012/013 之一。
5. 本矩阵不表示已实现或验收通过。只有真实 Test/PoC/Review 证据关联 Acceptance 后才能改 accepted。

## 2. Requirement 文档与主设计

| Requirement | 主设计 | 跨切支撑 | 核心事实所有者 |
| --- | --- | --- | --- |
| CUR-00 核心总览 | DES-001、DES-002、[DES-017](./017-Agent%20Harness与MVP业务闭环设计.md) | DES-000、DES-014 | 各领域模块；Production Control/Production Harness 统一阶段与门禁 |
| CUR-IAM | [DES-003](./003-身份Workspace与权限设计.md) | DES-013、DES-014 | Identity |
| CUR-PRJ | [DES-004](./004-项目与剧集工作台设计.md) | DES-003、DES-011、DES-014、DES-017 | Projects；只读消费 Production Control 快照 |
| CUR-SCR | [DES-005](./005-剧本与叙事版本设计.md) | DES-011～014、DES-017 | Scripts；输出剧集理解候选 |
| CUR-AST | [DES-006](./006-资产候选与选择设计.md) | DES-005、DES-011～014 | Assets |
| CUR-SBD | [DES-007](./007-分镜基线与叙事覆盖设计.md) | DES-005/006、DES-011～014 | Storyboards |
| CUR-KFR | [DES-008](./008-关键帧与视觉参考设计.md) | DES-006/007、DES-011～014 | Keyframes |
| CUR-VID | [DES-009](./009-视频生成候选与主选设计.md) | DES-008、DES-011～014 | Videos |
| CUR-EXP | [DES-010](./010-镜头检查快照与素材包导出设计.md) | DES-009、DES-011～014 | Exports |
| CUR-PLT | [DES-012](./012-平台Capability与Provider耐久任务及媒体设计.md) | DES-001/002、DES-013/014 | Platform |
| CUR-CAN（deferred/post-MVP） | [DES-011](./011-可视化制作画布与工作流投影设计.md) | DES-002/003、DES-012～014 | Canvas（布局事实 + 可重建领域投影；不进入当前 Gate） |
| CUR-SEC | [DES-013](./013-安全隐私权利与内容治理设计.md) | DES-003、DES-012、DES-014 | Governance |
| CUR-PROD | [DES-017](<./017-Agent Harness与MVP业务闭环设计.md>) | DES-001/002、DES-003、DES-012～014 | Production Control；Project 级运行与分作用域阶段 |

## 3. 功能需求追踪

### 3.1 CUR-IAM / CUR-PRJ / CUR-SCR

| Requirement ID | 主设计落点 | 支撑/验证落点 |
| --- | --- | --- |
| CUR-IAM-FR-001～003 | DES-003 §4～6：身份、会话、Workspace 建立/切换/撤销 | DES-013 §3/7；DES-014 §3/11 |
| CUR-IAM-FR-004～005 | DES-003 §2/6/8：Invitation/Membership/owner 原子转移 | DES-013 §10；DES-014 §3/10 |
| CUR-IAM-FR-006 | DES-003 §3～7：统一 action policy、ActorContext、ServiceGrant | DES-002 §10；DES-012 §10；DES-013 §4 |
| CUR-IAM-FR-007～009 | DES-003 §6～10：Workspace 生命周期、安全历史、账号交接 | DES-013 §9/10；DES-014 §8/9 |
| CUR-PRJ-FR-001～005 | DES-004 §2～5：最小 Project 元数据、EpisodePlan 物化、稳定 Episode 与强事务顺序 | DES-003 授权；DES-005 分集交接；DES-014 §3/4 |
| CUR-PRJ-FR-006～007 | DES-004 §2/5/8：ProductionSnapshot、Production Control 摘要、blocker、next action | DES-002 投影契约；DES-011 §4/6；DES-017 §3 |
| CUR-PRJ-FR-008～009 | DES-004 §4/6/7：归档/恢复/受控删除/工作台授权 | DES-003 §5～9；DES-013 §9/10 |
| CUR-SCR-FR-001～004 | DES-005 §2～6：DocumentRevision、确定性/AI 分集、整批物化 | DES-012 §4～6；DES-013 §3/4 |
| CUR-SCR-FR-005～007 | DES-005 §2～8：ScriptVersion current、AI 改写候选、NarrativeUnit | DES-002 版本契约；DES-012 长任务 |
| CUR-SCR-FR-008～011 | DES-005 §2/5～10：资产/镜头提示、人工决议、连续性、stale、生产处置语义 | DES-007 覆盖；DES-011 状态图层；DES-012 血缘；DES-017 §4.2/4.3 |
| CUR-SCR-FR-012 | DES-005 §2/4～10：EpisodeUnderstandingCandidate 与显式类型候选；DES-017 §4.2～4.4：长稿 fan-out/reduce 和人工 Gate | DES-012 WorkTask/Provider；DES-014 容量；CUR-PROD 作用域/恢复 |

### 3.2 CUR-AST / CUR-SBD / CUR-KFR

| Requirement ID | 主设计落点 | 支撑/验证落点 |
| --- | --- | --- |
| CUR-AST-FR-001～004 | DES-006 §2～5：Asset/State/Version、剧本候选决议 | DES-005 交接；DES-002 版本契约 |
| CUR-AST-FR-005～009 | DES-006 §2～8：上传/生成候选、唯一选择、readiness、影响/显式升级 | DES-012 任务/媒体/血缘；DES-013 权利 |
| CUR-AST-FR-010～011 | DES-006 §4/8/9：问题视图、批量浏览、归档/受控删除 | DES-011 投影；DES-013 保留/审计 |
| CUR-SBD-FR-001～003 | DES-007 §2～5：来源反查、AI DraftBatch、人工决议/原子应用 | DES-005/006 固定快照；DES-012 Task |
| CUR-SBD-FR-004～006 | DES-007 §2～6：ShotSpecVersion、拆合守恒、OrderRevision | DES-002 强事务/冲突契约 |
| CUR-SBD-FR-007～009 | DES-007 §2/7/8：Coverage、资产固定与影响、StoryboardBaseline | DES-011 coverage 边/readiness；DES-013 门禁 |
| CUR-KFR-FR-001～004 | DES-008 §2～5：Slot/Brief、Preflight、生成/上传候选 | DES-006/007 固定输入；DES-012 Capability/Task/Media |
| CUR-KFR-FR-005～007 | DES-008 §4～7：播放/比较/下载、共享标记、每 Slot 唯一主选 | DES-003 权限；DES-013 下载/权利 |
| CUR-KFR-FR-008～009 | DES-008 §2/7/8：VisualReferenceRevision、stale/重新选择、视频交接 | DES-009 输入快照；DES-011 图层 |

### 3.3 CUR-VID / CUR-EXP

| Requirement ID | 主设计落点 | 支撑/验证落点 |
| --- | --- | --- |
| CUR-VID-FR-001～003 | DES-009 §3～6：模式、固定预检、单镜/批量独立提交 | DES-012 §3～5；DES-013 §4 |
| CUR-VID-FR-004～006 | DES-009 §5～7：Task 投影、取消/重试/unknown、生成/上传 Candidate | DES-012 §5～7；DES-013 媒体门禁 |
| CUR-VID-FR-007～009 | DES-009 §4/5/8：播放比较下载、标记独立、每镜唯一主选 | DES-003 权限；DES-013 下载/主选门禁 |
| CUR-VID-FR-010 | DES-009 §8/9：影响、needs_reconfirmation/重选、Export readiness | DES-010 检查；DES-011 freshness/selection 图层 |
| CUR-EXP-FR-001～003 | DES-010 §2～5：目标 Baseline 检查、主选展示、Preflight | DES-009 Export readiness；DES-011 导出图层 |
| CUR-EXP-FR-004～005 | DES-010 §3～5：complete/partial 范围、原子 ExportSnapshot | DES-002 事务契约；DES-013 Freeze 再检 |
| CUR-EXP-FR-006～007 | DES-010 §6～7：ZIP64 原始视频集、JSON/CSV/README | DES-012 媒体；DES-013 AIGC/文件安全 |
| CUR-EXP-FR-008～009 | DES-010 §8/9：历史重下、同 Snapshot 恢复/重建 | DES-012 Package Task；DES-013 保留/下载；DES-014 DR |

### 3.4 CUR-PLT / CUR-CAN / CUR-SEC

| Requirement ID | 主设计落点 | 支撑/验证落点 |
| --- | --- | --- |
| CUR-PLT-FR-001 | DES-012 §3/4：Capability Catalog/Verification/Preflight | DES-013 DataProcessingProfile/Gate |
| CUR-PLT-FR-002～003 | DES-012 §4～6：幂等 Task、Attempt、取消/重试/对账/unknown | DES-001 工作流选型；DES-014 故障 Gate |
| CUR-PLT-FR-004 | DES-012 §2/7：MediaArtifact/Version、私有存储、hash/probe/访问 | DES-013 §7；DES-014 备份 |
| CUR-PLT-FR-005～006 | DES-012 §2/8：Lineage/Impact/内部 CostRecord；不形成项目预算 UI | DES-002 版本契约；DES-010 Snapshot |
| CUR-PLT-FR-007～008 | DES-012 §4/10/11：消费当前授权/治理决定、关联执行上下文并提交最小审计 | DES-013 §2～5/10 拥有决定与 AuditRecord |
| CUR-PLT-FR-009～010 | DES-012 §9/11：容量降级与诊断关联 | DES-014 §4/6/7；DES-013 最小诊断 |
| CUR-CAN-FR-001～003 | DES-011 §2～4：同域 Projection、类型节点/边、独立图层 | DES-002 Canvas 契约 |
| CUR-CAN-FR-004～005 | DES-011 §3/5/7/9：LayoutRevision、Viewport/Group、定位与大规模 | DES-014 容量/前端 SLI |
| CUR-CAN-FR-006～007 | DES-011 §5：详情导航、原模块等价 Command | DES-003 授权；DES-013 门禁 |
| CUR-CAN-FR-008～009 | DES-011 §6/8/9：Proposal/CommandPlan、真实 Task/partial failure/SSE 恢复 | DES-012 耐久 Task；DES-013 确认门禁 |
| CUR-CAN-FR-010～012 | DES-011 §7/9/11：列表等价、容量降级、View 归档/受控删除 | DES-014 无障碍/容量；DES-013 审计 |
| CUR-SEC-FR-001～003 | DES-013 §3/4/6：分类最小化、权利/内容门禁 | DES-012 Provider/Media 执行点 |
| CUR-SEC-FR-004～005 | DES-013 §5/6：ReviewCase/补证/申诉、DataProcessingProfile | DES-012 Capability active 条件 |
| CUR-SEC-FR-006～008 | DES-013 §7～9：AIGC 交接、保留/删除/hold、媒体/下载安全 | DES-010 Manifest；DES-012 Media；DES-014 DR |
| CUR-SEC-FR-009～010 | DES-013 §10/11：Audit/脱敏诊断/SecurityCase | DES-003 身份高风险事件；DES-014 观测/恢复 |

### 3.5 CUR-PROD

| Requirement ID | 主设计落点 | 支撑/验证落点 |
| --- | --- | --- |
| CUR-PROD-FR-001～003 | DES-017 §1/3/4：Project 级 ProductionRun、WorkflowDefinition、显式 StageRun scope 与固定输入 | DES-001 §4～7；DES-002 Production Control 契约 |
| CUR-PROD-FR-004～006 | DES-017 §3～5：领域 readiness、人工 Gate、暂停恢复、unknown 与分集/镜头 fan-out | DES-003 授权；DES-012 Task/Attempt；DES-013 治理 |
| CUR-PROD-FR-007～008 | DES-017 §3/5：影响、stale/superseded、Project/Episode 聚合快照和下一动作 | DES-004 ProductionSnapshot；DES-002 事件/投影 |
| CUR-PROD-FR-009～011 | DES-017 §4/7：受控 Skill、候选边界、交付完成、Workspace 隔离 | DES-005 类型化候选；DES-010 交付；DES-012～014 |

## 4. 非功能需求与 Acceptance 追踪

| 需求范围 | 模块设计 | 统一 NFR 落点 | Acceptance 范围 | 必需证据类型 |
| --- | --- | --- | --- | --- |
| CUR-IAM-NFR-001～010 | DES-003 §5/9～12 | DES-013 §3/7/10；DES-014 §3/6/11 | AC-CUR-IAM-001-A～009-B | 集成、并发、会话失效、IDOR、无障碍、审计 |
| CUR-PRJ-NFR-001～009 | DES-004 §3/6～9 | DES-014 §3/4/6 | AC-CUR-PRJ-001～012 | 容量压测、部分依赖、顺序冲突、无障碍 |
| CUR-SCR-NFR-001～012 | DES-005 §3/7～12 | DES-012 §5/8；DES-013 §3；DES-014 §3/4 | AC-CUR-SCR-001～019 | 字符守恒、确定性、深度语义完整、类型化候选、血缘、可恢复编辑 |
| CUR-AST-NFR-001～011 | DES-006 §3/8～11 | DES-011 投影恢复；DES-012 §7/8；DES-013 §4/7；DES-014 §3/4 | AC-CUR-AST-001～016 | 版本不漂移、候选媒体、影响预览、IDOR、审计、≤5s 状态可见 |
| CUR-SBD-NFR-001～010 | DES-007 §3/7～11 | DES-011 同源投影/列表等价；DES-014 §3/4 | AC-CUR-SBD-001-A～009-B | 120 Shot 压测、拆合守恒、冲突零部分写、无障碍、视图等价 |
| CUR-KFR-NFR-001～010 | DES-008 §3/7～12 | DES-012 任务/媒体；DES-013 权利；DES-014 §3/4 | AC-CUR-KFR-001-A～009-B | 候选容量、比较性能、唯一主选、刷新恢复、无障碍 |
| CUR-VID-NFR-001～011 | DES-009 §3～12 | DES-012 §5～11；DES-013 §4/7；DES-014 §3/4 | AC-CUR-VID-001-A～010-B | 批量独立、unknown 故障、媒体完整、唯一主选、列表等价 |
| CUR-EXP-NFR-001～011 | DES-010 §4/6～12 | DES-012 媒体/Task；DES-013 保留/安全；DES-014 §3/4/8～10 | AC-CUR-EXP-001-A～009-B | 120 Shot/20 GiB ZIP64、hash/Manifest、中断恢复、中文 CSV、下载安全 |
| CUR-PLT-NFR-001～010 | DES-012 §3～12 | DES-013 §3/4/7/10；DES-014 §3/5～10 | AC-CUR-PLT-001-A～010-C | 真 Provider、重复/崩溃/回调故障、媒体、OTel、第二 Adapter 架构测试 |
| CUR-CAN-NFR-001～012（post-MVP） | DES-011 §3～13 | DES-012 SSE/Task；DES-013 授权；DES-014 §3/4/6 | AC-CUR-CAN-001～022 | 同源 diff=0、600/900 渲染、SSE gap、并发布局、Agent 门禁、WCAG/列表 |
| CUR-SEC-NFR-001～012 | DES-013 §3～13 | DES-003 隔离；DES-012 Provider/Media；DES-014 §6/8～11 | AC-CUR-SEC-001-A～010-B | 门禁绕过=0、IDOR=0、敏感值=0、Provider Profile、媒体沙箱、删除/备份/密钥轮换 |
| CUR-PROD-NFR-001～009 | DES-017 §3～7 | DES-003 授权；DES-012 Task/Attempt；DES-013 治理；DES-014 容量/恢复 | AC-CUR-PROD-001～012 | Project current 唯一、分作用域并行、Gate、unknown、恢复、跨 Workspace、局部 stale |

## 5. 跨模块不变式追踪

| 不变式 | Requirement | Design 落点 | 最小可执行证据 |
| --- | --- | --- | --- |
| Workspace 强隔离 | CUR-00、CUR-IAM/SEC 隔离 NFR | DES-002 §10、DES-003 §5，DES-013 §3/7 | API/Worker/SSE/Media/Canvas/Export/Backup 跨空间成功数 0 |
| Workspace 是授权/隔离顶层，Project 是运行边界 | CUR-00、CUR-PRJ、CUR-SCR、CUR-PLT、CUR-PROD | DES-001 §1/4/5/6/8，DES-002 §1/3，DES-004 §2/4，DES-017 §1/3/4 | Workspace → Project → ProductionRun → StageRun(scope target) → Skill/WorkTask 归属完整；Episode/Shot/Package 仅作为 scope；跨 Workspace 成功数 0 |
| 稳定 ID + 不可变版本 | CUR-00 §6，各模块版本 NFR | DES-002 §8，DES-004～010 | 切 current/重排/重选后历史 hash/引用不变 |
| AI 只提候选 | CUR-SCR/AST/SBD/KFR/VID/CAN | DES-005～009、DES-011 §8 | AI 输出本身不设置 current/Selection/Baseline；Agent 提议未确认时 WorkTask/外发/预占为 0 |
| 流程控制唯一且局部隔离 | CUR-00、CUR-PRJ、CUR-SCR、CUR-PLT、CUR-PROD | DES-017 §3/4/5，DES-002 Production Control 契约，DES-004 §2/5 | 任一页面/Skill/WorkTask 都不能绕过 StageRun/Gate；单 Episode/Shot 失败不回滚其他范围；checkpoint 不成为业务状态 |
| 一镜唯一主选 | CUR-VID/EXP | DES-009 §4/8，DES-010 §2～4 | 并发唯一约束；无主选不兜底 |
| Canvas 非第二事实源 | CUR-CAN 全部 | DES-002 Canvas 契约，DES-011 §2～4 | 列表/画布同 ID/版本/状态 diff=0；布局不改业务 |
| Task ≠ Job，Attempt ≠ poll | CUR-PLT/VID/KFR | DES-001 §5/8，DES-012 §2/5 | 队列/工作流重启不产生第二 Task/Attempt |
| unknown 不盲重试 | CUR-PLT/VID/SCR/CAN | DES-001 §8，DES-012 §5/6 | submit timeout kill point 下外部双发数 0 |
| 媒体持久且完整 | CUR-PLT/KFR/VID/EXP/SEC | DES-012 §7，DES-013 §7，DES-010 §6 | URL 过期仍可读；ready 必有 size/type/probe/hash；包 hash 相等 |
| ExportSnapshot 不漂移 | CUR-EXP/PLT/SEC | DES-010 §2/4/8，DES-013 §4/8 | Freeze 后重排/重选/撤权不改 Snapshot/旧 Build |
| 无剪辑/成片/商业运营 | CUR-00 范围 | DES-000/001/004～014 非目标 | 不存在 Timeline/Render/Payment 模块/表/API/Worker；包只含逐镜原始视频 |

## 6. Design → Requirement 反向索引

| Design | 必须保持覆盖 |
| --- | --- |
| DES-000 | CUR-00 范围/工程交付、全模块依赖与验证结构 |
| DES-001 | CUR-00 不变式、CUR-PLT 技术约束、全模块长任务/媒体/部署需求 |
| DES-017 | CUR-PROD 全部；CUR-00 全流程控制、CUR-PRJ ProductionSnapshot、CUR-SCR 剧本理解候选和全模块阶段 Gate |
| DES-002 | CUR-00 跨模块交接、CUR-PRJ～SEC 版本/事件/权限/失败契约 |
| DES-003 | CUR-IAM 全部，所有模块 Workspace/action 要求 |
| DES-004～010 | 各自 CUR-PRJ/SCR/AST/SBD/KFR/VID/EXP 的全部 FR/NFR/AC |
| DES-011 | CUR-CAN 全部；CUR-PRJ～EXP 的列表/画布可见性与交接 |
| DES-012 | CUR-PLT 全部；CUR-SCR/AST/SBD/KFR/VID/EXP/CAN 的 Capability/Task/Media/Lineage/Cost |
| DES-013 | CUR-SEC 全部；CUR-IAM/PLT 高风险授权/权利/审计交接 |
| DES-014 | 全部模块 NFR；性能/容量/可观测/可用性/备份/灾备 Acceptance |

## 7. 追踪完整性 Gate

### 7.1 文档级检查

- Requirement README 中每个 current 模块恰好一个主 Design；
- 每个 Design 头部列出 FR/NFR/AC 范围且链接真实存在；
- 本矩阵的每个 FR 区间起止 ID 在 Requirement 中存在，无缺口/重叠主所有者；
- 每个 NFR 区间同时映射模块 Design 和 DES-014；
- 每个 AC 有验证类型，涉及外部依赖的标明 PoC/人工评审而不伪造自动通过；
- Design 未出现 Timeline/Render/FinalFilm/Billing/Payment/CommercialOps 业务实体或不受范围授权的预建模块。

### 7.2 实现期证据链

```text
Requirement ID
  → Design 章节/不变式
  → Plan 切片/Gate
  → Test/PoC/Review ID
  → 真实命令、数据集、环境、时间与结果
  → Acceptance 结论/Residual risk
```

实现后 CI 应扫描 Requirement/Design/Test 标识的不存在引用、重复测试事实源和无验证的 accepted。当前只是 Design 阶段，没有 Test/Acceptance 证据的 Requirement 仍为 proposed。

## 8. 当前开放追踪项

| 项 | 原因 | 关闭位置 |
| --- | --- | --- |
| API 框架、Workflow、对象存储 | 技术基线仍需同故障 PoC | DES-001 ADR-002～004，DES-012 §12 |
| Canvas 渲染库/真实容量 | post-MVP 能力，600/900 仅保留为未来样本 | DES-011 ADR-CAN-001，CUR-CAN-OQ |
| 首个图片/视频 Provider | 公开文档不证明账号/地域/计量/故障契约 | DES-012 真实 Provider Gate |
| 首发地区/权利/内容/AIGC 政策 | 必须产品/合规人工决策 | DES-013 §14 |
| 媒体/包/审计保留 | 需按引用、对账、申诉、删除和 hold 分类 | DES-013 §9，DES-014 ADR-DR-001 |
| 可用性 SLO 与 RPO/RTO | 当前为工程提案，无试运行/恢复演练证据 | DES-014 §12 |

## 9. 变更记录

| 版本 | 日期 | 变更 |
| --- | --- | --- |
| v1.0 | 2026-08-14 | 建立 CUR-00/IAM/PRJ/SCR/AST/SBD/KFR/VID/EXP/PLT/CAN/SEC 到 DES-000～014 的功能、NFR、AC、不变式和开放 Gate 双向追踪 |
| v1.1 | 2026-08-19 | 增加 DES-017 对 Production Control、全流程 Gate 和剧本理解候选的追踪，并将 Canvas 标为 post-MVP |
| v1.2 | 2026-08-19 | 增加 CUR-PROD 完整追踪，修正 Project 级 ProductionRun/StageRun scope，并纳入深度理解显式候选验收 |
