# Lanverse 软件工程文档导航

- 状态：active
- 最近审查：2026-08-14
- 当前产品需求状态：proposed

## 1. 当前产品边界

Lanverse 当前从零定义一款 AI 短剧核心制作平台。产品从身份与 Workspace 开始，依次完成项目与剧集组织、剧本结构化、角色/场景/道具资产、分镜与镜头设计、关键帧、逐镜视频候选、每镜唯一主选，以及固定、有序、可追溯的镜头视频素材包导出。列表与可视化制作画布读取同一领域事实，用于理解依赖、阻塞、任务进度和局部返工。

当前阶段不建设时间线、多轨剪辑、镜头拼接、转场、调色、字幕与音频后期、整集渲染、单一成片 MP4、发布分发、支付计费或商业运营。

## 2. 正式文档链路

Research → Requirement（含 AC）→ Design → PRD → Plan → 实现与测试 → Acceptance Evidence → Release。

范围或业务事实变化从 Requirement 开始更新；技术设计不能反向扩大产品范围。当前 Requirement 尚未完成评审，因此新的 PRD、Plan、开发周期和 Acceptance 尚未建立。

## 3. 文档入口

| 目录 | 作用 | 当前入口 |
| --- | --- | --- |
| research | 固定证据、产品观察、开源工作流模式与明确拒绝点 | [AI 短剧研究索引](./research/ai-short-drama/README.md) |
| requirement | 用户、场景、功能、规则、边界与验收条件 | [当前需求索引](./requirement/README.md) |
| design | 产品模块协作、状态、交接、失败与技术约束 | [Design 索引](./design/README.md) |
| prd | 评审通过后的产品任务与优先级 | [PRD 状态](./prd/README.md) |
| plan | 评审通过后的实施工作包与验证门禁 | [Plan 状态](./plan/README.md) |
| acceptance | 实现完成后的真实验收证据 | [Acceptance 状态](./acceptance/README.md) |
| archive | 已归档但仍需追溯的历史设计与交付文档 | [归档索引](./archive/README.md) |

## 4. 推荐阅读顺序

1. [AI 短剧研究与工作流证据](./research/ai-short-drama/README.md)，重点阅读[跨项目产品决策](./research/ai-short-drama/021-跨项目工作流模式与Lanverse产品决策.md)；
2. [001 当前核心产品需求总览](./requirement/001-当前核心产品需求总览.md)；
3. [002 身份、Workspace 与成员协作](./requirement/002-身份Workspace与成员协作需求.md)；
4. [003 项目与剧集工作台](./requirement/003-项目与剧集工作台需求.md)；
5. [004 剧本分析与叙事结构](./requirement/004-剧本分析与叙事结构需求.md)；
6. [005 角色、场景与道具资产](./requirement/005-角色场景与道具资产需求.md)；
7. [006 分镜与镜头设计](./requirement/006-分镜与镜头设计需求.md)；
8. [007 镜头关键帧与视觉参考](./requirement/007-镜头关键帧与视觉参考需求.md)；
9. [008 镜头视频生成与候选选择](./requirement/008-镜头视频生成与候选选择需求.md)；
10. [009 镜头检查与素材包导出](./requirement/009-镜头检查与素材包导出需求.md)；
11. [010 平台支撑与跨模块质量](./requirement/010-平台支撑与跨模块质量需求.md)；
12. [011 可视化制作画布与工作流信息](./requirement/011-可视化制作画布与工作流信息需求.md)；
13. [012 安全、隐私、权利与内容治理](./requirement/012-安全隐私权利与内容治理需求.md)；
14. [目标技术架构与选型](./design/001-目标技术架构与选型.md)及[Design 索引](./design/README.md)；
15. [Requirement → Design 追踪矩阵](./design/015-Requirement到Design追踪矩阵.md)。

## 5. 维护规则

- Requirement 只描述业务事实和用户可观察结果，不复制代码目录、数据库表或供应商 SDK；
- Research 必须固定证据时间和 GitHub commit，并区分公开事实、合理推断与待验证；开源项目只用于工作流和模块模式研究，不授权复制代码；
- 每条需求使用稳定全局 ID，并追踪到 Design、AC、测试和发布证据；
- accepted 只表示需求已通过评审，verified 只表示实现已有真实证据；
- Mock、字段占位、路由名称和历史代码行为均不能证明需求完成；
- 新模块分析产生的 Design、PRD、Plan、周期表和验收设计属于当前成果，必须保留并回链 CUR ID；
- 开发周期只能在模块范围、开放问题、验收条件和关键技术风险完成评审后估算。
