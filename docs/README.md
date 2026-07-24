# docs

本目录用于存放 Codex 侧项目文档。`requirement/` 作为前后端共同的需求分析入口；需求确认后，所有正式功能遵循 `Design → PRD → Plan → Acceptance`，文档按阶段进入对应子目录。

## 需求分析入口

`requirement/` 用于正式设计前的需求发现、平台范围、功能与质量要求、首发技术基线、前后端职责边界和设计准入门禁。需求分析以业务能力为主并共享一套事实源；适用 Requirement 达到 `accepted` 且通过 [`ADG-001`](requirement/ADG-001-前后端集成契约与设计完整性评审门禁.md) 的 `design_entry` 后，Design 才能转为 `accepted` 并进入 PRD。Requirement 与该门禁不单独授权实现。

## 核心流程

1. `Design` 可在 Requirement 评审期形成草案；转为 `accepted` 前，适用 Requirement 必须已接受且通过 `ADG-001` 的 `design_entry`。
2. `PRD` 基于已接受的 Design 固化用户价值、范围、非目标和验收标准，并在实现前转为 `accepted`。
3. `Plan` 基于 Design 与 PRD 拆分实现、测试、依赖和交付顺序；仅当 Design、PRD 和可执行 Plan 均为 `accepted` 后才能实现。
4. `Acceptance` 是实现后的验证产物，对照前三阶段逐项记录证据并给出通过、风险接受或不通过结论。

平台级 Design 负责共同架构基线，切片级 Design 负责单次可交付闭环；二者均被接受后才能建立对应 PRD。下游文档编号可在 Design 中规划，但不得以空 PRD、Plan 或 Acceptance 文件提前占位。纵向切片与编号见 [`ARCH-008`](design/ARCH-008-交付切片文档链与工程结构设计.md)。

`operations/` 只承载验收后的发布、部署、回滚和运行手册，不是第五个核心阶段。

## 子目录

1. `requirement/`：统一需求分析、研究事实、产品判断和待验证假设。
2. `design/`：技术方案、架构决策、接口设计、实现设计。
3. `prd/`：产品需求、范围定义、用户故事、MVP 边界。
4. `plans/`：执行计划、阶段拆解、任务清单、排期安排。
5. `acceptance/`：验收标准、测试记录、验证报告、回归证据。
6. `operations/`：发布流程、Git/PR 规范、部署说明、运行手册。

## 编写规范

1. 新文档先判断类型，再放入对应子目录。
2. 文档标题和文件名应能表达主题，避免使用 `new.md`、`temp.md` 等模糊命名。
3. 文档内容应保持 MVP 风格，只写当前阶段需要的信息。

## 收录边界

1. `docs/` 只收录会对项目产生长期真实影响的文档。
2. 可收录内容包括需求边界、产品决策、技术方案、验收标准、发布流程、Git/PR 规范和长期维护说明。
3. 不收录执行 todo、临时任务清单、过程性进展记录、一次性排查记录、会议流水账或仅服务当前一次执行的中间材料。
4. 过程性任务应保留在 PR 或其他协作记录中，不额外沉淀为 docs 文档。
5. 如果文档不能影响后续开发、验收、发布或维护决策，默认不应放入 `docs/`。

## 必要信息

每一份正式文档都应以 YAML frontmatter 写入必要信息：

1. `layer`：文档层级，例如 `PRD`、`Plan`、`Design`、`Acceptance`、`Operations`。
2. `doc_no`：文档编号，便于排序和引用。
3. `audience`：目标读者，例如 PM、Dev、QA、Ops。
4. `feature_area`：文档所属功能域。
5. `purpose`：一句话说明文档目的。
6. `canonical_path`：本文档在仓库中的标准路径。
7. `status`：使用 `draft`、`review`、`accepted`、`archived` 之一。
8. `version`：文档版本号。
9. `owner`：文档负责人或维护人。
10. `inputs`：输入或前置文档。
11. `outputs`：本文档产出的长期决策或交付物。
12. `triggers`：什么情况下需要阅读或更新本文档。
13. `downstream`：下游计划、设计、验收或运维文档。

## 推荐文档结构

以下结构仅为通用提示，不是固定模板。Requirement 文档应优先遵循 [`requirement/README.md`](requirement/README.md) 的模块化与可演进规则，根据模块特性选择功能目录、生命周期、关系图或规则矩阵。

正式文档建议按以下结构编写：

1. `背景`：为什么需要这份文档。
2. `目标`：本文件要解决什么问题，可用 BDD 场景描述关键目标。
3. `非目标`：明确不做事项，避免范围膨胀。
4. `核心内容`：按文档类型展开主体内容。
5. `关联文档`：列出输入、输出和下游文档。
6. `验收门禁`：说明如何判断文档完整、可执行、可验证。
7. `风险与边界`：列出风险、约束和延期事项。
8. `待确认问题`：列出需要用户或团队确认的问题。
9. `变更记录`：记录重要修改历史。

## 关联规则

1. Requirement 是 Design 的需求输入；未确认的假设不能直接成为设计结论。
2. Design 是 PRD 的上游，必须关联后续 PRD、Plan 和实现后 Acceptance。
3. PRD 必须引用已接受的 Design，并关联后续 Plan 和 Acceptance。
4. Plan 必须引用 Design 与 PRD，并关联验证它的 Acceptance。
5. Acceptance 必须引用并逐项验收 Design、PRD 和 Plan。
6. operations 应关联对应的发布、提交、PR、部署或回滚流程。
7. 双向追踪统一使用 `Requirement → Design AC → PRD AC → Plan Task/Test/Evidence → Implementation → Acceptance Result`；任何 P0 断链都阻止进入下一阶段。
