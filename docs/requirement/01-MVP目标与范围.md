---
layer: Requirement
doc_type: Software Requirements Specification
doc_no: REQ-01
title: MVP目标与范围
status: review
version: 1.0.0
owner: Lanverse
audience: [Product, Architecture, Dev, QA]
feature_area: AI短剧端到端制作MVP
purpose: 定义单操作者本地MVP的边界、成功条件和全局业务规则
canonical_path: docs/requirement/01-MVP目标与范围.md
inputs: [AI短剧端到端任务跑通目标]
outputs: [MVP范围, 功能需求目录, SMART验收基线]
updated: 2026-07-25
downstream: [DESIGN-01至DESIGN-06, PRODUCT-01, PLAN-01, ACCEPTANCE-01]
---

# MVP目标与范围

## 1. 产品目标与边界

Lanverse MVP 是供一名受信操作者在本地环境完成一集 AI 短剧的制作工作台，验证可执行、可恢复的生产闭环。

固定验收基线：输入规范化后 300～3000 个 Unicode 代码点且至少含一个 Han 字符的原创或获授权中文故事，形成 6～10 个镜头、30～60 秒、9:16 的单集，输出 720×1280、24fps、H.264 视频与 AAC 语音音轨，并包含源语言字幕。

## 2. 唯一用户与主流程

唯一用户是本地操作者，其主流程为：

1. 创建单集项目并保存原始内容快照。
2. 使用文本 Provider 生成结构化剧本，经人工编辑和确认。
3. 生成角色、场景、风格资产及 6～10 个 ShotSpec，经人工确认。
4. 使用图片 Provider 为角色/场景资产参考和镜头生成图片候选，经人工采用后固定视频生成输入；整体视觉风格只以已确认文字版本进入 Prompt 与兼容性哈希，不建立图片使用位置。
5. 使用视频 Provider 按镜头生成视频，并使用 TTS Provider 按确认对白或旁白逐句生成音频；分别采用视频和音频候选，失败项可单独重试。
6. 生成源语言字幕，按镜头顺序合成视频、语音和字幕。
7. 质检并导出可下载、可追溯的 MP4 成片。

## 3. 规范性需求

| 编号 | P0 需求 | 验证 |
| --- | --- | --- |
| SRS-001-001 | 系统应在单操作者、本地单应用实例中完成第 2 节全部步骤；外部 AI Provider 只按服务端批准配置访问。 | AC-SRS-001 |
| SRS-001-002 | 系统应对规范化后超出 300～3000 个 Unicode 代码点、没有 Han 字符或含禁用代码点的输入，以及非 6～10 个镜头或非 30～60 秒的计划给出明确校验错误。 | AC-SRS-002 |
| SRS-001-003 | 系统应通过已批准 Provider 为每个角色/场景资产和每个镜头生成图片、为每个镜头生成视频，并为每条对白或旁白语音条目生成逐句 TTS 结果；只接收相应 Provider 返回的媒体，不提供用户上传或导入媒体入口。操作者可为每个使用位置选择唯一当前候选，整体视觉风格不形成媒体使用位置。 | AC-SRS-003 |
| SRS-001-004 | 系统应使长任务在进程重启、确定性供应失败和重复命令后收敛到可判断状态，且不产生重复供应副作用。 | AC-SRS-004 |
| SRS-001-005 | 系统应导出满足固定规格且包含可听对白和源语言字幕的可播放 MP4，并能反查全部采用镜头及生成尝试。 | AC-SRS-005 |
| SRS-001-006 | 系统应使用 Mock Provider 连续自动验证完整闭环，并使用已批准的真实 Provider 完成至少一部覆盖文本、图片、视频和 TTS 的完整样片。 | AC-SRS-006 |

## 4. 全局业务规则

- BR-SRS-001：原始内容、确认剧本、确认 ShotSpec、候选、Adoption 和渲染输入均以不可变版本引用；修改形成新版本。
- BR-SRS-002：AI 输出在人工确认或选择前均为提案或候选，不得自动成为当前生产输入。
- BR-SRS-003：任务执行状态、候选可用状态和 active Adoption 是三类独立事实。
- BR-SRS-004：系统自动重试在原 Task 下创建新 ProductionAttempt；用户对终态失败的 Task 发起重试时创建新 ProductionTask 并引用原 Task；两者均不覆盖历史结果，同一传输命令重放不创建新副作用。
- BR-SRS-005：运行中任务固定输入快照；上游变化不得静默替换输入。
- BR-SRS-006：系统不得在未配置的情况下静默切换 Provider、能力或模型版本。
- BR-SRS-007：任何成片必须由 active 的镜头视频/语音 Adoption 和已确认 SubtitleVersion 合成。
- BR-SRS-008：验收只使用原创或获授权素材、合成角色和服务端批准的非克隆合成音色。

## 5. 功能需求目录

- REQ-02 项目与故事输入：单集项目、原始文本及不可变来源快照。
- REQ-03 剧本分镜与创作资产：结构化剧本、创作资产和 ShotSpec。
- REQ-04 AI 媒体任务与候选采用：图片、视频、TTS 任务、候选、Adoption 和恢复。
- REQ-05 字幕合成与成片交付：源语言字幕、固定合成、质检和 MP4 导出。
- REQ-06 MVP 质量标准：性能、可靠性、安全、追踪和自动化质量。
- REQ-07 技术与工程约束：技术栈、数据边界、Temporal/OutboxEvent、Provider 和工程结构。

## 6. 实现范围判定

当前实现只交付第 3～5 节明确编号的 P0 与 BR，以及 REQ-02～REQ-07 中明确编号的 P0、BR、DR 与 AC。未被这些编号明确要求的能力不属于实现契约，不得预建实现或测试占位。

## 7. 验收标准

- AC-SRS-001：在干净本地环境中，操作者不直接修改数据库、对象存储或任务状态即可完成一部基准单集。
- AC-SRS-002：分别提交规范化后 299、300、3000、3001 个代码点的含 Han 正文，以及 300 个代码点但无 Han 或含禁用代码点的正文和越界镜头/时长计划；只有同时满足字符规则与边界的输入可确认。
- AC-SRS-003：基准单集至少含一条语音条目；每个角色/场景资产和每个镜头图片位置均有 active 图片 Adoption，每个镜头均有 active 视频 Adoption，每条对白或旁白语音条目均有 active TTS Adoption；整体视觉风格没有 `asset_image` Candidate 或 Adoption。
- AC-SRS-004：重复提交、注入供应失败和中途终止 Worker 后，任务可恢复或重试；同一幂等命令重放不额外产生 Task/Attempt 或供应请求，每个合法 Attempt 至多产生一次供应副作用。
- AC-SRS-005：`ffprobe` 证明成片为 720×1280、24fps、H.264/AAC、30～60 秒；播放可听见对白并看到源语言字幕，且全部片段可追溯。
- AC-SRS-006：完整 Mock E2E 连续运行 3 次通过；至少 1 部真实 Provider 完整样片覆盖文本、图片、视频和 TTS，满足成片规格并保存脱敏证据。

## 8. 下游与状态

本文件输出给 DESIGN-01～DESIGN-06、PRODUCT-01 和 PLAN-01。上述 Requirement、Design、PRD 与 Plan 全部为 `accepted` 且通过 REQ-08 前，不得开始实现；ACCEPTANCE-01 只在实现完成后建立。
