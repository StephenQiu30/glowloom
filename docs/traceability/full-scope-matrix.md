# 全量交付追溯矩阵

本矩阵是 Requirement → Design → PRD → Plan → Acceptance 的执行入口。它不把“已写入文档”误报为“已验收”：验收状态仍以 `docs/acceptance/README.md` 中的证据包为准，未产生真实证据前保持 `not_run`。

| 范围 | 需求与设计 | PRD | Plan | 当前实现状态 |
| --- | --- | --- | --- | --- |
| 平台基线 | REQ-000、DES-000—005 | PRD-000 | PLAN-000 | 进行中：Go/Agent/frontend 固定，GORM、统一 HTTP envelope/状态码、基础 Schema、单 Kafka、MinIO、OpenAPI 已落地；RLS、Redis 业务接入、Elastic 投影、OTel、严格 Swagger 仍待实现 |
| A 手工事实主线 | M01—M05、M07—M09、M11、M14 | PRD-A | PLAN-A | 进行中：剧本解析草稿/批准、canonical GORM 物化、人物×集数与资产投影已可用并完成真实浏览器闭环；租户深度策略、导入格式扩展、检索、完整镜头、交付预演待实现 |
| B Agent 提案 | M03、M04、M06 | PRD-B | PLAN-B | 未实现：当前 Python 仅提供私有确定性 Harness 骨架 |
| C 真实可恢复生成 | M07—M09、M11、M14 | PRD-C | PLAN-C | 未实现：真实 Provider、Usage 与完整 Candidate/Selection 状态机尚未启用；当前仅保留 Fixture Candidate/Selection 试点闭环 |
| D 质量与修复 | M04、M09、M10、M14 | PRD-D | PLAN-D | 未实现 |
| E 审阅与交付 | M10、M12、M13、M14 | PRD-E | PLAN-E | 未实现 |
| F 团队与扩展 | M01、M06、M15 | PRD-F | PLAN-F | 未实现 |

## 执行约束

- 所有公共接口继续使用当前 `/api` 契约；不新增路径版本或兼容入口。
- PostgreSQL 是业务事实，MinIO 是对象存储，Go backend 是唯一 Kafka 接入方；Python Agent 不接入 Kafka、Redis、Elastic 或业务数据库。
- 本矩阵只记录事实，不替代代码、测试或验收证据。每完成一个工作包，必须同时更新对应 Acceptance evidence、自动化测试和状态。
