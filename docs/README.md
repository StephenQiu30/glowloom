# Lanverse 文档中心

`docs/` 是产品范围、技术决策、可验证需求、实施顺序和验收证据的长期事实入口。目录只保留 `prd/`、`design/`、`requirement/`、`plan/`、`acceptance/` 五类文档；架构与设计决策统一收口到 `design/`，不再建立平行目录。

本轮有效设计采用纯 0→1 口径：PRD、Design、Requirement 和 Plan 不读取或盘点既有项目代码，不以目录、依赖、Schema、接口、测试文件或历史运行结果作为方案输入或完成证据。所有实施与目标验收 Checklist 初始均为 `[ ]`。

## 目录职责

| 目录 | 回答的问题 | 必须包含 | 不应包含 |
|---|---|---|---|
| `prd/` | 为什么做、为谁做、做什么 | 产品目标、范围、非目标、发布门 | 技术实现细节、执行日志 |
| `design/` | 系统和模块如何设计 | 决策与取舍、边界、数据与接口、状态、失败路径 | 易变排期、未验证的完成声明 |
| `requirement/` | 什么契约必须被满足 | 可测试的功能/非功能需求、输入输出、约束 | 方案推演、模糊愿景 |
| `plan/` | 按什么顺序安全落地 | 依赖、阶段、交付门、回滚点、执行 Checklist | 把计划状态当作实现事实 |
| `acceptance/` | 如何判定完成、实际验证了什么 | 验收标准、命令、输入、结果、缺失条件与残余风险 | 没有证据的“已通过”结论 |

## 文档链路

默认评审与实施顺序为：

```text
PRD → Design → Requirement → Plan → Acceptance
```

先用 PRD 固定产品范围，再接受 Design 的边界和取舍；随后将其细化为可测试 Requirement、可执行 Plan，并由 Acceptance 记录真实证据。小型变更可以只创建必要文档，不为凑齐链路批量建立空文件。

Plan 内的 Checklist 追踪“下一步做什么和执行到哪里”；Acceptance 内的 Requirement Checklist 追踪“哪些契约已有按新设计重新执行的真实证据”。两者不得互相替代：Plan 勾选不能证明验收通过，历史实现证据也不能预先勾选新设计。

## 编号与命名

文件统一使用 `NNNN-中文业务主题.md`。四位编号由服务边界决定：

| 编号段 | 目标边界 | 负责内容 |
|---|---|---|
| `0001–0999` | 系统级、产品级、跨服务 | 平台产品、架构、资源所有权与总交付计划 |
| `1000–1999` | Frontend | Web、Canvas、AI UI 与 Collaboration |
| `2000–2999` | Backend | Go 业务服务、运行程序、契约、数据与基础设施 |
| `3000–3999` | Production Intelligence / Agent | Production Bible、Storyboard 与受限 Agent Harness |

- 同一业务主题跨目录时复用同一编号，例如 `3002` 同时关联 Requirement、Design、Plan 与 Acceptance。
- 一个目录内同一编号最多对应一份文档；编号分配后不重排、不复用。
- 编号表示服务归属和推荐阅读顺序，不表示优先级、版本或完成状态。
- Compose 运行角色、数据库、中间件和 Worker 不单独占用服务编号；它们归入目标 Owner 或跨服务设计。
- 架构决策直接作为 Design 保存；被取代时在正文记录替代关系，历史版本由 Git 追溯。

## 状态词

| 状态 | 含义 |
|---|---|
| 已接受目标 | 产品或设计已经确认，但不代表代码完成 |
| 待独立评审 | 从 Design 派生的 PRD/Requirement/Plan/Acceptance 尚未单独接受 |
| 待实施 | 目标契约已定义，执行 Checklist 尚未完成 |
| 待验收 | 实施可能存在，但尚无按本设计重新执行的完整证据 |
| 已验收 | Acceptance 记录了对应范围的真实输入、命令和结果 |
| 历史记录 | 只保存旧实现事实，不进入 0→1 完成判断 |
| 已取代 | 仅用于历史追溯，不再指导新实现 |

“Design 已接受”不能写成“功能已实现”，“Plan 已完成”也不能替代 Acceptance。只有目标 Acceptance 才能形成新设计的完成证据。

## 文档集索引

`—` 表示该主题当前不需要对应类型，不创建占位文件。

| 编号 | 主题 | PRD | Design | Requirement | Plan | Acceptance | 当前状态 |
|---|---|---|---|---|---|---|---|
| `0001` | 平台产品与完整设计基线 | [产品范围与验收基线](prd/0001-产品范围与验收基线.md) | [完整设计基线](design/0001-AI短剧制作平台完整设计基线.md) | [平台 V1 需求规格](requirement/0001-平台V1需求规格.md) | [见 0007 交付计划](plan/0007-平台0到1交付计划.md) | — | 目标已接受，规格待独立评审 |
| `0002` | 采用目标平台架构 | — | [架构决策](design/0002-采用目标平台架构决策.md) | — | — | — | 已接受目标 |
| `0003` | 系统总体架构 | — | [总体架构](design/0003-系统总体架构.md) | — | — | — | 已接受目标 |
| `0004` | 架构分层与依赖 | — | [分层规则](design/0004-架构分层与依赖规则.md) | — | — | — | 已接受目标 |
| `0005` | 中文语义化文档与模块命名 | — | [命名决策](design/0005-采用中文语义化文档与模块命名决策.md) | — | — | — | 已接受目标 |
| `0006` | 领域语言与模块命名 | — | [命名规范](design/0006-领域语言与模块命名规范.md) | — | — | — | 已接受目标 |
| `0007` | 平台 0→1 交付 | — | — | — | [交付计划](plan/0007-平台0到1交付计划.md) | — | 待独立评审，Checklist 全部未开始 |
| `0008` | 资源所有权与交付 | — | — | — | [所有权台账](plan/0008-资源所有权与交付台账.md) | — | 待独立评审，Checklist 全部未开始 |
| `0009` | 剧本到分镜 MVP 垂直切片 | [产品需求](prd/0009-剧本到分镜MVP产品需求.md) | [垂直切片设计](design/0009-剧本到分镜MVP垂直切片设计.md) | [需求规格](requirement/0009-剧本到分镜MVP需求规格.md) | [实施计划](plan/0009-剧本到分镜MVP实施计划.md) | [验收记录](acceptance/0009-剧本到分镜MVP验收记录.md) | 执行与验收中 |
| `1001` | 前端应用架构与交付 | — | [应用架构](design/1001-前端应用架构.md) | [架构需求规格](requirement/1001-前端应用架构需求规格.md) | [应用与功能交付计划](plan/1001-前端应用与功能交付实施计划.md) | — | 派生文档待独立评审，Checklist 全部未开始 |
| `1002` | 前端创作工作台与功能模块 | [创作工作台产品需求](prd/1002-前端创作工作台产品需求.md) | [模块设计](design/1002-前端功能模块设计.md) | [功能需求规格](requirement/1002-前端功能模块需求规格.md) | [合并至 1001 计划](plan/1001-前端应用与功能交付实施计划.md) | — | 派生文档待独立评审 |
| `2001` | 后端服务与运行架构 | — | [服务架构](design/2001-后端服务架构.md) | [运行架构需求规格](requirement/2001-后端运行架构需求规格.md) | [运行架构实施计划](plan/2001-后端运行架构实施计划.md) | — | 派生文档待独立评审，Checklist 全部未开始 |
| `2002` | 后端领域服务与生产闭环 | — | [模块设计](design/2002-后端领域模块功能设计.md) | [领域服务需求规格](requirement/2002-后端领域服务与生产闭环需求规格.md) | [生产闭环实施计划](plan/2002-后端领域服务与生产闭环实施计划.md) | [持久任务恢复](acceptance/2007-Workflow持久任务恢复验收记录.md) · [编译输入前置](acceptance/2008-Workflow编译输入前置验收记录.md) · [确定性编译](acceptance/2009-Workflow确定性编译验收记录.md) · [启动与对账](acceptance/2010-Workflow启动事实与Temporal对账验收记录.md) · [人工信号协调](acceptance/2011-Workflow人工信号协调验收记录.md) · [取消控制协调](acceptance/2012-Workflow取消控制协调验收记录.md) · [人工任务续租与释放](acceptance/2013-Workflow人工任务续租与释放验收记录.md) · [人工任务过期回收](acceptance/2014-Workflow人工任务过期回收验收记录.md) · [暂停与恢复控制](acceptance/2015-Workflow暂停与恢复控制协调验收记录.md) · [Worker 重启恢复](acceptance/2016-Workflow工作者重启恢复验收记录.md) · [Node Cache 确定性事实](acceptance/2017-Workflow节点缓存确定性事实验收记录.md) · [Node 输出绑定](acceptance/2018-Workflow节点输出绑定验收记录.md) · [Node 输入冻结](acceptance/2019-Workflow节点输入冻结验收记录.md) · [Node Runtime Cache](acceptance/2020-Workflow节点运行缓存验收记录.md) · [Human Gate 输入与决议绑定](acceptance/2021-Workflow人工栅栏输入与决议绑定验收记录.md) · [Production Bible Owner Receipt](acceptance/2022-ProductionBible确认回执验收记录.md) · [Workflow Owner Receipt 与 Gate 输出](acceptance/2023-Workflow生产回执与人工栅栏输出验收记录.md) · [Workflow 执行身份与 Script Executor](acceptance/2024-Workflow执行身份与剧本输入节点验收记录.md) | 阶段 5 切片验收中，完整 Checklist 未完成 |
| `2003` | 后端语言与运行边界 | — | [运行边界策略](design/2003-后端语言与运行边界策略.md) | — | — | — | 已接受目标 |
| `3001` | 项目制作圣经与完整剧本闭环 | [产品需求](prd/3001-项目制作圣经产品需求.md) | [执行框架设计](design/3001-项目制作圣经生成执行框架设计.md) | [需求规格](requirement/3001-项目制作圣经需求规格.md) | [实施计划](plan/3001-项目制作圣经实施计划.md) | [验收 Checklist](acceptance/3001-完整剧本业务闭环验收标准.md) | 派生文档待独立评审，Checklist 全部未开始 |
| `3002` | 本地 Codex 分镜智能体 | [产品需求](prd/3002-本地-Codex-分镜智能体产品需求.md) | [执行框架设计](design/3002-本地-Codex-分镜智能体执行框架设计.md) | [需求规格](requirement/3002-本地-Codex-分镜智能体执行框架需求规格.md) | [实施计划](plan/3002-本地-Codex-分镜智能体执行框架实施计划.md) | [验收 Checklist](acceptance/3002-本地-Codex-分镜智能体执行框架验收标准.md) | 派生文档待独立评审，Checklist 全部未开始 |

`2002` 的最新增量验收：[Cost Project Budget 唯一事实](acceptance/2041-Cost项目预算唯一事实验收记录.md)；前置边界见 [Quota 图片生成日配额](acceptance/2040-Quota图片生成日配额验收记录.md)、[Generation 人工候选选择](acceptance/2039-Generation人工候选选择验收记录.md)、[Generation 图片候选与确定性 QC](acceptance/2038-Generation图片候选与确定性QC验收记录.md) 与 [Asset 图片产物就绪](acceptance/2037-Asset图片产物就绪验收记录.md)。

## 历史实现记录

下列文件只保存旧实现的验证事实，不是有效 Design 的输入，也不能抵扣上表任何 Plan 或 Acceptance：

| 编号 | 记录 | 适用性 |
|---|---|---|
| `2004` | [后端运行边界与事件契约验收记录](acceptance/2004-后端运行边界与事件契约验收记录.md) | 历史记录，不计入 0→1 完成判断 |
| `2005` | [数据库基线与兼容窗口验收记录](acceptance/2005-数据库基线与兼容窗口验收记录.md) | 历史记录，不计入 0→1 完成判断 |
| `2006` | [单入口启动与对象存储上传验收记录](acceptance/2006-单入口启动与对象存储上传验收记录.md) | 历史记录，不计入 0→1 完成判断 |

## 事实优先级与维护

1. 已接受的 PRD 决定产品范围，已接受的 Design 决定方案与边界。
2. Requirement 决定可测试契约；既有代码不是 Requirement 的输入。
3. Plan 只描述 0→1 实施顺序和门禁，初始 Checklist 全部为 `[ ]`。
4. 目标 Acceptance 只证明按新设计重新执行且明确记录的范围；历史记录不得抵扣目标 Checklist。
5. 新增、改名、取代文档时必须同步更新本索引和相对链接，并检查目标状态、实施状态与历史证据是否被隔离。
6. 不建立全局 `misc`、重复分类、空占位文档或第二套事实来源。
