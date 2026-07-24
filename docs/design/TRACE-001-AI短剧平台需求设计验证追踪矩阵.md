---
layer: Design
doc_type: Requirements Design Verification Traceability Matrix
doc_no: TRACE-001
title: AI短剧平台需求设计验证追踪矩阵
status: review
version: 0.2.1
owner: Lanverse
audience: [Product, Architecture, Frontend, Backend, QA, Security, Operations, Governance]
feature_area: 需求、设计与验证追踪
purpose: 为每份需求的全部规范性条款建立设计位置、覆盖状态和分阶段验证入口
canonical_path: docs/design/TRACE-001-AI短剧平台需求设计验证追踪矩阵.md
inputs: [SRS-001, FR-001至FR-021, NFR-001, TCR-001至TCR-003, ADG-001, ARCH-001至ARCH-007, ADR-001至ADR-002]
outputs: [逐需求覆盖状态, 设计位置, Design验证入口, 实施验收入口]
triggers: [需求条款变化, 设计位置变化, 覆盖状态变化, PRD或Plan或Acceptance建立]
updated: 2026-07-24
downstream: [PRD, Plan, Test, Acceptance]
---

# TRACE-001 AI 短剧平台需求设计验证追踪矩阵

## 1. 使用规则

本矩阵是 [ARCH-001](ARCH-001-AI短剧制作平台总体架构设计.md) 总体追踪的逐需求明细。`covered` 表示指定范围内的全部规范性条款已有设计落点，不表示需求或设计已接受；`partial` 必须列出未覆盖条款，`missing` 阻断 Design 准入。A1～A7 依次代表 [ARCH-001](ARCH-001-AI短剧制作平台总体架构设计.md)、[ARCH-002](ARCH-002-短剧生产流程与工作台设计.md)、[ARCH-003](ARCH-003-AI策划与生成任务架构设计.md)、[ARCH-004](ARCH-004-API事件文件与数据契约设计.md)、[ARCH-005](ARCH-005-媒体安全隐私与数据生命周期设计.md)、[ARCH-006](ARCH-006-部署观测灾备容量成本与测试设计.md)、[ARCH-007](ARCH-007-业务模块边界与服务协作设计.md)。A7 §4～7 的事实所有权、公开契约、依赖和失败规则适用于每个 FR 行。

实施前“验证入口”仅表示 Plan 中必须建立的测试或评审标识；表中的 `A7 AC` 明确指 AC-ARCH-007-001～006，并由对应风险选择具体编号。没有已接受 Plan 时不得把计划当成已执行证据；后续 PRD、Plan、Test 与 Acceptance 必须回填实际文档编号、用例编号和结果链接。

## 2. 逐需求追踪

| 需求条款范围 | 覆盖 | 设计位置 | Design / 实施验证入口 |
| --- | --- | --- | --- |
| SRS-001 全部总体规则 | covered | A1 §1～10；A2 §2～11；A7 §1～9 | A1 §13；A7 AC；端到端单集 Acceptance |
| FR-001-001～013 | covered | A1 §5、7～9；A2 §3～4、10；A4 §3～6、12；A5 安全边界；A7 `identity-access` | A4 `SEC`；A7 AC；授权矩阵/租户否定测试 |
| FR-002-001～012 | covered | A1 §7；A2 §2～4；A4 §3～4、12；A7 `project-catalog` | A2/A7 AC；项目/系列/分集 API 与 E2E |
| FR-003-001～017 | covered | A2 §2、5；A3 §2～4；A4 §11～12；A7 `story-development` | A3 AC-001；A7 AC；来源解析/证据保留测试 |
| FR-004-001～020 | covered | A2 §2、5、7；A3 §2～4；A4 §4、7；A7 `story-development` | A3 AC-001；A7 AC；版本/基线授权与并发测试 |
| FR-005-001～012 | covered | A1 §7；A2 §5～6；A3 §2、4～6；A7 `continuity` | A3 AC-001/005；A7 AC；连续性规则与过期传播测试 |
| FR-006-001～017 | covered | A1 §7；A2 §3、6；A3 §2；A4 §11～12；A5 媒体生命周期；A7 `creative-assets` | A2 AC-003；A7 AC；资产版本/绑定/权限测试 |
| FR-007-001～012 | covered | A3 §2、10；A4 §11～13；A5 媒体对象与谱系；A7 `media-library` | A4 `FILE/LIFECYCLE`；A7 AC；媒体谱系与删除测试 |
| FR-008-001～016 | covered | A2 §3、5～8；A3 §2、5～6；A7 `storyboard` | A2 AC-002/003；A7 AC；拆镜候选/快照/准备度测试 |
| FR-009-001～016 | covered | A3 §5～6、10；A4 §4、13；A6 §8～9；A7 `model-catalog` | A3 AC-002；A7 AC；能力 Manifest/路由解释测试 |
| FR-010-001～019 | covered | A2 §7～9、11；A3 §8～9；A4 §7～10；A6 §5～10；A7 `production-jobs` | A3 AC-003/004；A7 AC；幂等/恢复/故障注入测试 |
| FR-011-001～014 | covered | A2 §8；A3 §5～10；A5 生成媒体处理；A7 `generation` | A3 AC-002/005；A7 AC；候选、一致性与谱系测试 |
| FR-012-001～014 | covered | A1 §7；A2 §2～3；A5 音频、口型与时间轴；A7 `creative-assets/media-library/postproduction/review-approval` | A5/A7 验证入口；音频/口型/授权/回退样片测试 |
| FR-013-001～011 | covered | A1 §7；A2 §2～3；A5 音乐、音效与混音；A7 `media-library/postproduction/compliance-governance/review-approval` | A5/A7 验证入口；授权/混音/响度测试 |
| FR-014-001～014 | covered | A1 §7；A2 §2～3；A5 字幕与本地化；A7 `story-development/postproduction/delivery` | A5/A7 验证入口；timebase/字幕/字体测试 |
| FR-015-001～012 | covered | A1 §7；A2 §2～3、11；A5 时间线与渲染；A7 `media-library/postproduction/review-approval` | A5/A7 验证入口；非破坏编辑/快照/导出测试 |
| FR-016-001～013 | covered | A2 §2～3、7、10～11；A3 §2～4、9；A4 §4、12；A7 `review-approval` | A2 AC-002；A7 AC；审核/采用分离及越权测试 |
| FR-017-001～012 | covered | A3 §2、5～10；A4 §4、7、10；A6 §5、9～10；A7 `cost-billing` | A3 AC-003；A7 AC；预算/账本/P1 支付契约测试 |
| FR-018-001～016 | covered | A1 §9；A3 §5～6、10；A4 §6、11～12；A5 威胁/隐私/权利；A7 `compliance-governance` | A4 `THREAT`；A5/A7 安全与删除验证 |
| FR-019-001～017 | covered | A2 §2～3、7、11；A4 §11～13；A5 质检/交付；A6 §10～11；A7 `media-library/postproduction/review-approval/compliance-governance/delivery` | A5/A6/A7 样片、校验和、导出及回滚测试 |
| FR-020-001～015 | covered | A1 §7、9；A2 §3、9、11；A4 §8、10；A6 §5～6、11；A7 `operations-support/identity-access/compliance-governance` | A2 AC-004；A7 AC；通知/临时授权/DSR/分析测试 |
| FR-021-001～022 | covered | A2 §3、5、10；A3 §2～4、9～10；A4 §4、12；A7 `agent-runtime/review-approval` | A2 AC-006；A3 AC-001；A7 AC；Agent 权限/注入测试 |
| NFR-001-001～046 | covered | A1 §5～9、12；A4 §3～13；A5 全文；A6 §2～11；A7 §6～10 | A4/A5/A7 Design 验证；A6 §10 实施证据 |
| TCR-001-001～027 | covered | A1 §1～12；A4 §2；A6 §2～4、8；A7 §8～9 | 架构评审；构建/部署/独立扩缩证据 |
| TCR-002-001～040 | covered | A1 §5、7、9；A3 §2～11；A4 §2～13；A6 §2～11；A7 §4～10 | A3/A4/A7 AC；边界/集成/工作流/迁移/恢复测试 |
| TCR-003-001～044 | covered | A1 §6、8～9；A2 §3～11；A4 §2～8、13；A6 §3、5～6、10；A7 §9 | A2/A4/A7 AC；类型、浏览器、性能与 E2E |
| ADG-001-001～047 | covered | A1～A7；本矩阵；ADR-001～ADR-002 | §3 交付物检查；分 scope 门禁记录 |

## 3. ADG 设计交付物映射

| ADG 范围 | 主设计文档 | 覆盖结论 |
| --- | --- | --- |
| 013～015 总体、领域、前端 | A1、A2、A3、A7 | covered |
| 016～018 API、数据、工作流 | A3、A4、A7 | covered |
| 019 AI/Agent | A2、A3、A4、A7 | covered |
| 020～021 媒体、安全、隐私 | A4、A5、A7 | covered |
| 022～024 部署、容量、成本、测试 | A6、A7 | covered |
| 025 双向追踪 | A1、A7、本矩阵、各 ARCH 追踪节 | covered |
| 026 架构决策 | [ADR-001](ADR-001-首发平台架构与仓库边界.md)、[ADR-002](ADR-002-业务模块拆分与依赖方向.md)及 Design README 索引 | covered |

## 4. 评审与变更规则

- 当前全部映射处于 `review`；仅在适用 SRS、FR、NFR、TCR 与 ADG 均为 `accepted`，且 A1～A7、ADR-001～ADR-002 和未决 P0 决策完成评审后，才能形成 `design_entry` 结论。
- 任一需求新增、删除、优先级或语义变化时，先把相关行改为 `partial`，列出缺口，再更新设计；禁止仅修改总范围文字。
- 每次门禁随机抽取至少一条身份权限、一条长任务、一条媒体生命周期、一条费用和一条交付需求，从原需求逐跳核对至设计与拟定验证。
- PRD、Plan 及实现后的 Acceptance 建立时在本矩阵增补版本链接；测试失败或残余风险不能以 `covered` 掩盖，须记录到对应 Acceptance。
