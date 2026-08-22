# Lanverse 验收标准与证据规范

> 状态：`waiting_for_implementation`
> 最近审查：2026-08-22
> 适用范围：PRD-000、切片 A—F 及其引用的 Requirement 验收条件

## 1. 当前结论

当前仓库没有目标方案的真实执行证据，因此所有 PRD 验收条件和 Requirement 中的 `AC-SYS-*`、`AC-IAM-*`、`AC-PRJ-*`、`AC-NAR-*`、`AC-KNO-*`、`AC-SHT-*`、`AC-AIC-*`、`AC-PLN-*`、`AC-EXE-*`、`AC-MED-*`、`AC-QAR-*`、`AC-USG-*`、`AC-REV-*`、`AC-DLV-*`、`AC-GOV-*`、`AC-INT-*` 均为待执行标准，不代表已经通过。

本文件定义判定规则、固定数据集、全局门禁和证据格式。实际实现并执行后，才为具体场景创建 Evidence 文件；当前不预建空证据文档。

## 2. 判定状态

| 状态 | 定义 | 是否计入通过 |
| --- | --- | --- |
| `not_run` | 尚未在指定环境和数据集上执行 | 否 |
| `passed` | 实际结果满足全部预期，证据完整且可复现 | 是 |
| `failed` | 至少一个关键断言不满足，或出现未允许的副作用 | 否 |
| `blocked` | 缺少真实外部条件而不能执行；已记录原因、责任方和恢复条件 | 否 |

不存在 `partial_pass`。一个场景可记录部分成功信息，但只要关键断言未完成、测试被跳过或环境不等价，最终结论必须是 `failed`、`blocked` 或 `not_run`。

## 3. 切片通过规则

一个切片只有同时满足以下条件才能从 `in_progress` 变为 `verified`：

1. 对应 PRD 的全部 P0 `PRD-*-AC-*` 均为 `passed`；
2. PRD 映射的 Requirement 验收条件在本切片范围内均为 `passed`；
3. 本文件中适用的全局门禁均为 `passed`；
4. Plan 规定的正常、失败、重复、并发、权限和恢复场景均已执行；
5. 没有未接受的 S0/S1 缺陷，没有用 mock 替代要求的真实依赖；
6. 证据绑定当前提交、当前合同、当前数据集和可识别环境；上游合同变化后旧证据已重跑；
7. 残余风险有责任人和明确处理决定，且不违反 P0 业务规则。

后续切片通过不能反向证明前序切片通过；发布成功也不能替代功能、权限或恢复验收。

## 4. 全局验收门禁

| Gate ID | 标准 | 最低证据 |
| --- | --- | --- |
| ACC-GATE-001 追溯 | Requirement→Design→PRD→Plan→Test/Evidence 链路完整，无孤立 P0 合同 | ID/链接检查结果、变更提交、覆盖矩阵 |
| ACC-GATE-002 当前接口 | 唯一当前 OpenAPI、无 `/v1`/兼容入口；Swagger、Go 边界、前端生成 API 同源 | Schema hash、生成命令、生成后无 diff、手写 URL/DTO 扫描 |
| ACC-GATE-003 租户与权限 | UI/API/Worker/媒体/搜索/异步任务均执行同一 Workspace 和最小授权判断 | 跨租户已知 ID、撤销、停用、RLS 与对象授权负向结果 |
| ACC-GATE-004 幂等与并发 | 重复命令、Kafka 重投、并发决定和回调重放不产生第二个不可区分事实或副作用 | 业务行/Job/Provider 调用/Candidate/Usage 前后计数 |
| ACC-GATE-005 失败与恢复 | `failed`、`partial`、`unknown` 和依赖中断有权威状态、安全下一动作和可恢复路径 | 故障注入步骤、Operation/Attempt 状态、恢复后断言 |
| ACC-GATE-006 观测与审计 | 用户动作、Command、Operation、Job、外调和治理决定可由 ID/Trace 关联 | 日志/Metric/Trace/Audit 样本及 Collector 中断结果 |
| ACC-GATE-007 权利与治理 | 外发、生成、下载、模板和私有能力在权利/安全/资源门禁前不产生副作用 | 拒绝路径、Provider 调用计数为 0、决定与理由 |
| ACC-GATE-008 数据与搜索 | current/approved/published 范围明确；投影/索引可重建且不成为权威事实 | 重建前后业务键一致、跨租户/未批准零命中、stale/unavailable |
| ACC-GATE-009 可用性与无障碍 | P0 旅程有加载、空、失败、重试和键盘路径，不依赖只有颜色/画布的操作 | 浏览器 E2E、键盘操作、错误恢复录像或结构化记录 |
| ACC-GATE-010 发布与回退 | 发布制品、配置、Schema、备份恢复和停用能力路径已演练 | 制品 digest、空库初始化、备份恢复、回退/停用记录 |

并非每个工作包都要重复十个 Gate，但切片退出前必须说明每个 Gate 是 `passed`、不适用及理由，或未通过。`blocked` 不能当作不适用。

## 5. 固定验收数据集

| 数据集 ID | 适用切片 | 必须覆盖的样本 |
| --- | --- | --- |
| DS-A-SCRIPT | A，且 B 复用 | 同一份有授权整本剧本，不少于 3 集；明确/缺失/冲突标题，跨集别名、同名反证、地点复用、道具、服装、单集部分失败和另一个 Workspace 的相似对象 |
| DS-A-KNOWLEDGE | A/D | UnresolvedSubjectRevision 未决人物、缺失 current MentionResolution、active 未决重叠、Occurrence failed/unassigned/overlap、零 Requirement 引用 coverage、schema 换版 stale、参考/媒体变化只影响 readiness |
| DS-A-MEDIA | A | 一集 3—5 分钟目标、10—40 个 Shot 的有授权 Fixture 图片/视频、可确定顺序和主选的 Animatic 样本 |
| DS-B-ADVERSARIAL | B | 合法/非法 Proposal、不同 request hash、真实 read-set 冲突与不相交 item、Prompt injection、恶意来源指令、Tool 越权和 checkpoint 故障 |
| DS-C-PROVIDER | C | 图片/视频各一个真实 sandbox capability；成功、失败、cancelled、unknown、响应丢失、重复/乱序/伪造回调、限流、下载损坏和迟到计量 |
| DS-D-QUALITY | D | 正确/错误身份或服装/关键道具、黑帧、静音、损坏媒体、相邻/兄弟镜头、中途事实变化、误报/不适用和规则升级 |
| DS-E-DELIVERY | E | 10 个以上主选 Shot，23.976/25/29.97 fps、VFR、不同采样率、字幕边界、代理/原件差异、缺轨和必需附属输出失败 |
| DS-F-ORG | F | 依能力选择真实多人 Workspace、外部审片人、API client/webhook endpoint、重复项目模板、企业 IdP 或私有 capability；不得用虚构消费者证明采用 |

所有数据集都必须记录来源、权利依据、内容 hash、标注/期望版本和敏感级别。数据内容变化后必须生成新的数据集 revision，并重跑依赖证据。

## 6. A—F 验收矩阵

| 切片 | PRD 验收范围 | 必需证据包 | 当前状态 |
| --- | --- | --- | --- |
| 000 | PRD-000-AC-001—006 | `EPK-000-TRACE/BOUNDARY/DATA/ASYNC/CONTRACT/OBSERVE/RELEASE` | not_run |
| A | PRD-A-AC-001—010 | `EPK-A-SCRIPT/NARRATIVE/KNOWLEDGE/SEARCH/SHOTS/FIXTURE/TENANCY/MANUAL-E2E/RECOVERY` | not_run |
| B | PRD-B-AC-001—009 | `EPK-B-CONTRACT/ORCHESTRATION/RUNTIME/TOOLS/TAKEOVER/GOVERNANCE/DEGRADE/SECURITY-RECOVERY` | not_run |
| C | PRD-C-AC-001—010 | `EPK-C-PLAN/PREFLIGHT/EXECUTION/KAFKA-LIMIT/IMAGE/VIDEO/CALLBACK/MEDIA/SELECTION/USAGE/RECOVERY` | not_run |
| D | PRD-D-AC-001—008 | `EPK-D-TECHNICAL/SEMANTIC/TRIAGE/IMPACT/REPAIR-PLAN/LOCAL-REPAIR/RISK/REPAIR-E2E` | not_run |
| E | PRD-E-AC-001—010 | `EPK-E-PACKAGE/ACCESS/ANNOTATION/DECISION/REPAIR-LOOP/ASSEMBLY/PREFLIGHT/RENDER/IMMUTABILITY/DOWNLOAD/DELIVERY-E2E` | not_run |
| F | PRD-F-AC-001—009，按已准入能力选取，不适用项需有产品决定 | `EPK-F-IAM/EXTERNAL-REVIEW/CANVAS/TEMPLATE/API/WEBHOOK/PRIVATE-CAPABILITY/SSO-SCIM` | not_run |

F 的能力独立验收不表示全部 F 已完成。索引页和 PRD 只有在明确说明哪些能力已准入、哪些不适用后，才能给出聚合状态。

## 7. Evidence 文件标准

实际执行时，文件名使用 `EV-<切片>-<YYYYMMDD>-<scenario>.md`。每个文件至少包含：

```markdown
# <场景名称>

- Evidence ID：EV-A-20260822-script-manifest
- 结论：passed | failed | blocked | not_run
- 执行时间与时区：
- 执行人 / 复核人：
- Git commit / 制品 digest：
- 环境与关键配置 hash：
- 数据集 ID / revision / 内容 hash / 权利依据：
- 关联 Requirement / Design / PRD AC / Plan WP / 自动化测试：

## 前置条件
## 执行命令或人工步骤
## 预期结果
## 实际结果与逐项断言
## 故障注入与恢复结果
## 产物链接、日志、Trace、对象或响应摘要
## 指标与样本统计
## 偏差、残余风险和后续动作
```

证据引用产物时应记录稳定路径、对象 key/hash 或 CI artifact ID；不要把大体积媒体、日志、凭据或本地数据复制进 Git。敏感字段需脱敏，但脱敏不能移除判断租户、状态、次数或时间顺序所需的信息。

## 8. 不构成通过的材料

以下内容可以辅助定位，但不能单独证明验收通过：

- 只有代码、测试源码、接口定义、数据库表或文档存在；
- 只有 Swagger 页面、UI 截图、视频演示或“人工看起来正常”；
- 需要真实 Provider/PostgreSQL/Kafka/Redis/MinIO/Elastic 时只运行 mock；
- 测试被 skip、命令中断、日志缺失，或只保留成功路径；
- 只报告 HTTP 200，不断言业务状态、持久化结果和外部调用次数；
- 旧提交/旧合同的历史通过记录，或另一环境无法关联当前制品的结果；
- 用 Kibana 日志出现证明 Outbox/Kafka 业务事件已投递；
- 用一次 Demo、聊天满意度或单一质量总分代替金标/对抗/故障矩阵。

## 9. 关键专项标准

### 9.1 OpenAPI 与前端生成链

`AC-SYS-012` 和 `ACC-GATE-002` 的证据必须包含：

- 已提交的 `backend/api/openapi.json` hash，以及 Swagger UI 实际加载的 schema hash；
- Go strict server 和 `@umijs/openapi` 的真实生成命令；
- 生成后 `git diff --exit-code` 或等价无漂移结果；
- 前端 ViewModel 只调用生成 API，生成 API 只调用唯一 Axios `request.ts` 的架构检查；
- 无 `/v1`、`ApiV1`、手写 URL、重复 DTO 或第二 endpoint registry 的扫描；
- 公共合同中不存在 Python Agent 私有路由的负向结果。

只有 Swagger 页面截图或生成文件存在不能证明通过。

### 9.2 剧本、人物与资产首轮标准

切片 A 必须使用 DS-A-SCRIPT、DS-A-KNOWLEDGE 和 DS-A-MEDIA 完成整本输入到 Animatic 的同一项目链路，并验证：来源唯一覆盖/具名忽略、稳定 ContentUnit、Narrative Anchor、人物未决行、人物对应集数、地点/道具/服装清单、`unassessed=0`、approved 私有检索、10 个以上 Shot、Fixture Selection 和 Agent/Provider 全关闭。不得把分散的接口测试拼接成未实际执行的用户闭环。

### 9.3 Elastic 与 ToC 搜索标准

首轮 A 只验收 approved 私有内容的 BM25/结构过滤、高亮、Anchor、stale/unavailable、索引删除重建、跨租户和未批准内容零命中，不把未来 ToC 公共检索、向量/混合检索或推荐报告为已通过。

进入 ToC 生产 Gate 时必须另行执行完整 `AC-SYS-011`：使用签认规模的私有/公共语料、查询集、人工 relevance judgment 和并发模型，记录召回/排序、p95/p99、热点租户、索引重建和观测洪峰隔离。业务搜索与观测后端未处于独立故障域时，不得通过 ToC 生产验收。

### 9.4 真实 Provider 标准

切片 C 的通过必须包含图片/视频各一次真实成功外调，以及响应丢失、unknown 对账、限流、回调伪造/重放、下载失败和迟到计量。若 Provider 不提供某能力，证据应验证明确的降级/停止行为，不得在 Adapter 中伪造支持。

## 10. 复核与失效规则

- Evidence 至少由执行人之外的一名责任人复核关键断言；高风险权限、治理和正式交付需产品/安全/业务责任人按 Design 指定职责复核。
- PRD AC、Requirement 语义、OpenAPI、Schema、Provider 能力、关键配置或数据集 revision 变化时，标记相关 Evidence `superseded` 并重跑。
- 仅修改说明文字且不改变合同，可保留证据，但需在变更评审中说明不受影响的依据。
- 被替代的 Evidence 从当前验收矩阵移除；Git 历史只保存审计轨迹，不证明当前方案通过。
