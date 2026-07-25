---
layer: PRD
doc_type: Product Overview Requirements Document
doc_no: PRODUCT-01
title: MVP产品总览与质量目标
status: accepted
version: 1.0.0
owner: Lanverse
audience: [Product, Architecture, Frontend, Backend, QA, Operations]
feature_area: AI 短剧端到端制作 MVP
purpose: 固化单操作者从中文故事到可播放 AI 短剧的产品价值、全局范围和闭环质量
canonical_path: docs/prd/01-MVP产品总览与质量目标.md
inputs: [REQ-01至REQ-08, DESIGN-01至DESIGN-06, DESIGN-12, DESIGN-13]
outputs: [MVP 产品地图, 全局业务规则, AC-PRD-009至AC-PRD-010]
triggers: [MVP 目标变化, 全局范围变化, E2E 或真实 Provider 门禁变化]
updated: 2026-07-25
downstream: [PRODUCT-02至PRODUCT-07, PLAN-01, PLAN-02, PLAN-09, ACCEPTANCE-01]
---

# MVP产品总览与质量目标

## 1. 输入与状态

本文是模块 PRD 的总览，不复制模块细节。产品范围来自 REQ-01～08，技术与纵向闭环受 DESIGN-01～06 约束，前端体验受 DESIGN-12 约束，完整追踪以 [DESIGN-13](../design/13-需求实现追踪.md) 为准。

本文与 PRODUCT-02～07 均已根据 accepted Design 完成评审。PRD 接受只放行 Plan 评审；应用代码仍需全部 Plan 接受并通过 `database_design_ready` 和 `implementation_start`。

## 2. 产品问题与用户价值

内部创作者需要把一份已获权中文故事转成可播放 AI 短剧，但多模态生成具有失败、长耗时和不确定性。MVP 的价值不是一次生成某个素材，而是让用户能清楚地完成“输入→确认创作基线→按位置生成和采用→字幕/渲染→下载”，并在刷新、重试和 Worker 重启后仍知道发生了什么。

在受控本地环境中，单个创作者应在 10 分钟内（不含真实供应等待）将固定故事通过确定性 Mock 制作为一部 30～60 秒、6～10 镜头、720×1280、24fps、含逐句语音和源语言字幕的 MP4；流程连续执行 3 次均成功，并能反查全部输入与生成尝试。

## 3. 产品模块地图

| 产品文档 | 用户价值 | 核心出口 |
| --- | --- | --- |
| [PRODUCT-02 项目与来源](02-项目与来源产品需求.md) | 安全建立单集项目和可追溯来源 | confirmed SourceRevision |
| [PRODUCT-03 剧本分镜与创作资产](03-剧本分镜与创作资产产品需求.md) | 把故事转为人工可控的生产计划 | confirmed Script/Assets/Storyboard |
| [PRODUCT-04 生产任务与恢复](04-生产任务与恢复产品需求.md) | 长任务可见、可恢复、可局部重试 | 每个 Task 可判断 |
| [PRODUCT-05 AI媒体与候选采用](05-AI媒体与候选采用产品需求.md) | 多 AI 生成可比较且按位置明确采用 | 必需媒体 Adoption 齐全 |
| [PRODUCT-06 字幕渲染与交付](06-字幕渲染与交付产品需求.md) | 获得可播放且可追溯成片 | ready MP4/SRT/Manifest |
| [PRODUCT-07 前端工作区与交互](07-前端工作区与交互产品需求.md) | 只通过五个工作区完成闭环 | 页面状态与服务端事实收敛 |

以上六份模块 PRD 与本文共同穷尽 MVP。未出现在活动 Requirement、Design 和本组 PRD 的能力不是实现输入。

## 4. 主用户故事

| ID | 用户与触发 | 前置条件 | 主流程 | 失败/恢复 | 完成定义 |
| --- | --- | --- | --- | --- | --- |
| US-MVP-001 | 内部创作者获得一份可制作故事 | 本地应用、PostgreSQL、MinIO 可用且已有批准模型配置 | 建项目→输入来源→确认剧本分镜→生成采用→字幕渲染→下载 | 每阶段显示稳定错误与下一动作；成功事实不因局部失败丢失 | 浏览器播放 MP4，SRT/Manifest 可下载且谱系完整 |
| US-MVP-002 | QA 要证明闭环可重复 | 固定六镜头 fixture、确定性 Mock、隔离测试数据 | 连续执行完整用户流 3 次 | 任一次失败保留 trace/日志/数据库差异 | 三次均在 10 分钟内完成且无需改库/对象 |
| US-MVP-003 | 产品需确认真实 AI 可交付 | 已批准文本/图片/视频/TTS 配置、凭据、额度和获权样本 | 运行一次全部模态真实生成与成片质检 | 供应失败必须如实记录，不能以 Mock 替代 | 至少一部真实完整样片满足成片规格 |

## 5. 全局产品规则

- 主体只有部署边界内的 `internal_operator`，用途固定 `internal_review`。
- AI 输出始终是提案或候选；Task succeeded、Adoption active、Delivery ready 是三个独立事实。
- 产品只支持一个 Project 对应一个 Episode、9:16、6～10 镜头、30～60 秒，不提供用户上传媒体、多集、团队、发布或商业化能力。
- 每个高成本命令固定不可变输入和模型 profile；失败后不得静默换模型或 Provider。
- PostgreSQL 是正式状态和恢复事实源，MinIO 只保存私有媒体字节；浏览器状态与 URL 不构成业务事实。
- 部分失败只修复失败位置；历史版本、Task、候选、采用、媒体和交付不得被覆盖。

## 6. 非目标

不包含多租户/登录权限、小说抓取、模板市场、协同编辑、自动采用、自动发布、模型训练、工作流编排平台、WebSocket、移动端、生产云部署或运营后台。未来需求必须先进入 Requirement，不能以隐藏入口、空目录或 Feature Flag 预埋。

## 7. SMART 验收标准

| ID | 可观察行为与阈值 | 事实源、样本/环境与门禁 |
| --- | --- | --- |
| AC-PRD-009 | 使用既有本地 PostgreSQL/MinIO，从创建项目到浏览器播放 Mock 成片的 E2E 连续 3 次成功；每次≤10分钟且无需人工改库、改对象或跳过页面 | 三个独立 run_id 的 Playwright trace、Task/Delivery、结构化日志和样片；PLAN-09/CI 门禁 |
| AC-PRD-010 | 正式 Acceptance 结论前，使用批准凭据至少完成 1 个 6～10 镜头、30～60 秒的真实 Provider 完整样片；文本、图片、视频、TTS 全部真实执行，成片满足 AC-PRD-008 且日志无密钥 | 脱敏 Provider 请求 ID、Task/Attempt、ffprobe、样片 URI 与费用摘要；供应失败不得排除；PLAN-09/Acceptance 门禁 |

## 8. 追踪与接受条件

| 输入 | 本文落点 | 下游 |
| --- | --- | --- |
| SRS-001-001～006 | 第 2～7 节、AC-PRD-009/010 | PLAN-01、02、09 |
| NFR-001-001～014 | 全局时限、恢复、安全与证据 | PLAN-01～03、05～09 |
| TCR-001-001～016 | 技术约束只作为体验和验收边界 | PLAN-02、03 |

接受评审已确认六份模块 PRD 范围无缺口、AC-PRD-001～010 全部可测、真实 Provider 前置条件具有 Owner 与失败结果。实际运行证据只在实现后的 ACCEPTANCE-01 记录。
