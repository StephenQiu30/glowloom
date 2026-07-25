---
layer: Design
doc_type: Architecture Decision Record
doc_no: DESIGN-01
title: MVP技术选型与范围
status: draft
version: 0.1.0
owner: Lanverse
audience: [Product, Architecture, Frontend, Backend, QA, Operations]
feature_area: AI 短剧制作 MVP
purpose: 定义首个可执行闭环的范围、技术栈、运行边界和回滚规则
canonical_path: docs/design/01-MVP技术选型与范围.md
inputs: [REQ-01至REQ-08]
outputs: [MVP 实现 allowlist, 技术栈, 运行单元, 迁移与回滚原则]
triggers: [MVP 范围变化, 技术族变化, 事实源变化, 任务编排变化, 部署边界变化]
updated: 2026-07-25
downstream: [DESIGN-02至DESIGN-06, PRODUCT-01, PLAN-01, ACCEPTANCE-01]
---

# MVP技术选型与范围

## 1. 决策

Lanverse MVP 是内部验证用的单实例 Web 应用，只交付一个 30～60 秒、6～10 镜头、9:16 单集闭环：输入文本→结构化剧本→分镜→图片/视频与最小 TTS→人工采用候选→字幕/音轨合成→MP4 下载。

采用单一 Git monorepo、Next.js 前端、NestJS 模块化单体、PostgreSQL 业务事实、Temporal 持久工作流、S3 兼容对象存储和 FFmpeg 媒体处理。应用实现仅位于 `backend/`、`frontend/`、`deploy/`。

## 2. 技术基线

| 关注点 | MVP 选择 | 最迟确定点 |
| --- | --- | --- |
| 语言/仓库 | TypeScript strict、Node.js 24 LTS、pnpm workspace | T-001 创建应用源码前：确定精确依赖、唯一 lockfile 和安装命令 |
| Web | React、Next.js App Router、Tailwind CSS、shadcn/ui | 支持浏览器与构建版本 |
| 前端状态 | TanStack Query；仅编辑会话使用 Zustand | Query/Store 边界与生成客户端命令 |
| API | NestJS、REST/JSON、OpenAPI 3.1；任务详情 2 秒轮询 | 契约校验与客户端生成工具版本 |
| 数据 | Prisma、PostgreSQL | 数据库版本、迁移和测试容器 |
| 长任务 | Temporal TypeScript、PostgreSQL OutboxEvent | Temporal 版本、命名空间、队列与重试参数 |
| 媒体 | S3 兼容存储；local 使用 MinIO；FFmpeg/ffprobe | 镜像、工具版本、对象策略和资源上限 |
| AI | 静态 Provider Adapter；每模态一个真实 Provider 与确定性 Mock | T-011 前：文本、图片、视频、TTS 的 Provider/模型/凭据/额度 |
| 质量 | Vitest、Testing Library、Playwright、结构化日志 | 命令、门禁、覆盖范围和证据位置 |

依赖 allowlist 仅包含上表技术族；T-001 必须以 lockfile、Compose 镜像 digest 和依赖扫描把它落实为可复现基线。

### 2.1 开源实现复审取舍

- [Jellyfish 固定提交](https://github.com/Forget-C/Jellyfish/tree/a9678194ddf2d9be3ccbe78d4287d87d5089e123)提供分镜准备、生成工作室、任务事实分离和 OpenAPI 生成客户端的边界证据；Lanverse 按本 ADR 的技术基线实现这些边界。
- [Toonflow 固定提交](https://github.com/HBAI-Ltd/Toonflow-app/tree/bc61ec7a1b5df31293b286981a5f4ad4635464ee)提供来源→剧本→生产最短纵向工作流的领域证据；Lanverse 按本 ADR 的六模块和运行单元实现该链路。
- 两个项目都作为领域边界和交互证据，不作为生产可靠性背书；Lanverse 的版本、幂等、恢复、私有媒体和验收仍以本组 Requirement/Design 为准。

## 3. 运行单元

| 单元 | 责任 |
| --- | --- |
| `frontend` | 五个工作台 Feature、生成客户端和 Task 2 秒轮询，不保存正式事实 |
| `backend-api` | 校验输入、幂等/并发、业务命令、查询、任务轮询和短期下载授权 |
| `backend-worker` | OutboxEvent 派发、Temporal Workflow/Activity、Provider 调用、下载、探测与渲染 |
| `postgres` | 唯一业务事实、Task/Attempt、OutboxEvent、TaskEvent 和版本记录 |
| `temporal` | 可恢复编排历史，不作为产品状态或媒体事实源 |
| `minio/S3` | 私有媒体字节、派生物和成片；URL 不是业务事实 |

数据库迁移是受控的一次性命令，不是常驻服务。API 和 Worker 必须能独立停止、启动和构建；MVP 可共用一个 Task Queue，但代码保持 Workflow 与 Activity 边界。

## 4. 选择理由

- PostgreSQL、不可变快照和明确版本保证内容、候选与交付可追溯。
- Temporal 与 OutboxEvent 直接服务“任务在进程重启和外部超时后仍能继续”的核心价值。
- 单仓库、模块化单体和两个后端进程减少首轮部署与契约协调成本。
- S3 与 FFmpeg 把媒体字节和高资源处理移出控制面。
- 静态 Adapter 通过部署配置为每种模态绑定唯一 Provider，并使每次调用可确定、可追踪。

## 5. MVP 实现 allowlist

- 一个部署边界内的内部操作者创建一个 Project，并由系统原子创建唯一 Episode。
- 操作者粘贴中文正文并声明来源权利，生成、编辑和确认结构化剧本、创作资产与分镜。
- 文本、图片、视频和 TTS 各使用一个已批准真实 Provider；测试使用确定性 Mock。
- 每个异步命令产生可恢复的 Task/Attempt；媒体结果先成为 Candidate，再由操作者形成 Adoption。
- 服务端使用已采用视频、逐句 TTS 和已确认字幕生成 MP4、SRT 与 JSON Manifest，并通过 ffprobe 质检。
- 来源权利声明、服务端密钥、私有对象、短期授权和日志脱敏是交付闭环的必需护栏；交付用途固定为受控内部评审。

任何代码目录、API、数据表、后台进程或验收项都必须直接服务上述六项之一。

## 6. 后果与风险

- 单实例、单操作者边界固定为本机 loopback：Compose 只有 frontend、backend-api、minio 可发布端口且全部绑定 `127.0.0.1`，其余服务只在内部网络可达；部署与运行时测试必须拒绝非 loopback 访问。
- 单 Worker 按[《06 MVP质量标准》](../requirement/06-MVP质量标准.md)的并发、超时和资源上限执行 FFmpeg/Provider Activity；容量不足时拒绝新任务并保留全部已受理任务。
- Temporal 是本闭环恢复协议的必需运行依赖，必须提供重启、取消、未知状态和幂等证据。
- 一模态一 Provider 可能形成供应故障阻塞；MVP 显式失败并允许用户对终态失败创建新 Task，不自动切换。

## 7. 迁移与回滚规则

当前无应用数据，首次迁移从 `0001` 建立。数据库变更遵循 expand→switch→contract；部署回滚只回退制品和入口，不删除已提交的 Task、Attempt、快照、媒体和 Delivery。

实现若偏离本 ADR 的 allowlist、技术基线、运行单元或成片规格，必须先更新 Requirement 与 Design 并重新通过准入门禁，不能在代码中预埋旁路。

## 8. Design 验收标准

- AC-ADR-001-001：[02 MVP总体架构](02-MVP总体架构.md)、[03 任务执行与媒体处理](03-任务执行与媒体处理.md)、[04 接口数据与项目结构](04-接口数据与项目结构.md)与[05 AI短剧端到端制作流程](05-端到端制作流程.md)只使用本决策的技术族、运行单元和单闭环范围。
- AC-ADR-001-002：所有代码目录、API、数据表和验收项都可定位到 §5 allowlist 与六个模块之一。
- AC-ADR-001-003：真实 Provider/模型/凭据/额度均有明确的 T-011 前关闭门禁，Mock 不伪装真实成片证据。
- AC-ADR-001-004：Worker 重启、重复请求、供应状态未知和渲染失败均有不丢事实、不重复副作用的恢复设计。
- AC-ADR-001-005：目标目录仅包含 `backend/frontend/deploy` 应用根，且 Acceptance 未在实现前创建。
