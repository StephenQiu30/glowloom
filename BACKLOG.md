# 当前待办

这里只记录尚未完成的工作、阻塞和验收入口。涉及多个领域时，只保留一条待办并链接到对应计划。设计决定和验收证据维护在各自的 Workspace 文档，提交与构建结果见 Git 和 GitHub Actions。

| 待办 | 完成条件 | 来源 |
| --- | --- | --- |
| 首条真实云模型接入：方舟 Seedance | 校准运行镜像与数据库目录，验证一个获授权的 720p 文生视频型号。交付可选择的模型、原任务回执、可播放视频及按实际 Token 核对的流水。 | [Plan29 首条真实线路](workspace/content/plan/29-模型与供应商接入扩展实施计划.md#first-live-seedance) |
| 内置模型预设覆盖 | 继续补齐 Seedance 720p 文生视频之外的可售型号和收费规格。每个规格具备审核后的计量、采购、销售及验证计划，并能从快捷入口录入 Key。 | [PRD03 接入验收](workspace/content/prd/03-AI平台接入与预设管理.md#key-only-model-onboarding) |
| 积分钱包发放与充值 | T-CREDIT-15 补齐来源批次、模型适用范围、有效期和冻结分摊。T-CREDIT-17 验收单一支付渠道、幂等到账和原单退款。 | [Plan32 统一钱包](workspace/content/plan/32-模型商业计费与积分系统实施计划.md#credit-service-foundation) |
| 资产工作台两级导航与游标续载 | 完成来源和筛选行的桌面与窄屏验收，生成历史按日合组，资产列表按游标滚动续载并取消每页控件。 | [Plan22](workspace/content/plan/22-资产对齐即梦实施计划.md#两级导航按日历史与游标续载) |
| 短剧 Skill 生产链路 | 接入隔离脚本执行、结构化项目成果写回、媒体审批与真实生成渲染，并验证中断后的续跑。 | [设计49](workspace/content/design/49-AI短剧制作三层Agent与六阶段Workflow设计.md) · [研究02](workspace/content/research/02-短剧开源Skill套件接入调研.md) |
| 普通画布单镜端到端验收 | 完成 T-CONV-23 的双账号浏览器验收，核对 Agent 文本授权段账单和逐次媒体审批。真实 T-CONV-24 依赖首条 Seedance 线路、已验证文本模型和项目消费授权。 | [Plan31](workspace/content/plan/31-对话工作台与参考输入一致性实施计划.md) |
| Agent 项目上下文与检索评测 | 在获准语料上验证上下文、项目记忆与 RAG 的联合质量、延迟、费用和撤权行为。 | [设计32](workspace/content/design/32-完整创作AgentHarness与Skill多Agent架构设计.md) · [Plan43](workspace/content/plan/43-Agent服务Mastra架构重构执行计划.md) |
| Go 域边界实施 | 分阶段验证跨域准入、结果归档和账务回执，通过对应 owner 的独立验收后再切换。 | [Plan42](workspace/content/plan/42-Go模块化单体架构改造执行计划.md) |
| 模型销售定价与结算验收 | 按模型规格对照官方估算、审核售价、原线路采购实账和未知结果恢复。完成 Agent 文本授权段的精确合计与一次核销。 | [Plan36](workspace/content/plan/36-模型销售定价与结算执行计划.md) |
