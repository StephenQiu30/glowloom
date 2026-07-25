---
layer: Requirement
doc_type: Non-Functional Requirements Specification
doc_no: REQ-06
title: MVP质量标准
status: review
version: 0.2.0
owner: Lanverse
audience: [Architecture, Dev, QA, Operations]
feature_area: AI 短剧制作 MVP 质量
purpose: 固定端到端闭环必须达到的可靠性、性能、安全、媒体和工程质量
canonical_path: docs/requirement/06-MVP质量标准.md
inputs: [REQ-01, REQ-02至REQ-05]
outputs: [质量门槛, 测试边界, 运行限制]
triggers: [MVP范围变化, 任务模型变化, 媒体规格变化, 部署边界变化]
updated: 2026-07-25
downstream: [REQ-07, REQ-08, DESIGN-01至DESIGN-07, PRODUCT-01, PLAN-01, ACCEPTANCE-01]
---

# MVP质量标准

## 1. 适用环境

质量目标适用于单操作者、本机 loopback、单应用实例、一个活动单集和最多 3 个并发 Provider 任务。

## 2. 规范性要求

| 编号 | P0 要求 | 验证事实 |
| --- | --- | --- |
| NFR-001-001 | 除 Provider 与媒体处理外，本地普通 API 的 p95 应不高于 2 秒，异步命令应在 3 秒内返回可查询 `task_id`。 | API 时序测试 |
| NFR-001-002 | 活跃任务每 2 秒轮询；服务端状态变化后 5 秒内应在 UI 可见，刷新页面后 3 秒内恢复权威状态。 | Playwright 与 Task 记录 |
| NFR-001-003 | API 或 Worker 重启不得丢失已提交 Task、Attempt、候选、采用关系、媒体或 Delivery。 | PostgreSQL TaskJob/租约与 MinIO 恢复测试 |
| NFR-001-004 | 同一幂等键和载荷并发提交 20 次只能形成一个逻辑 Task 和一次初始 Provider 副作用；同键异载荷必须失败。 | 并发集成测试和调用计数 |
| NFR-001-005 | Provider 已受理但结果未落库时终止 Worker，重启后 120 秒内必须通过过期租约和原请求键对账，收敛为成功、失败或明确 `unknown`，不得盲目重发。 | TaskJob、Attempt 与 Mock 日志 |
| NFR-001-006 | 单镜头失败不得删除或重新生成其他成功镜头；只重试失败镜头且历史 Candidate/Attempt 不被覆盖。 | 六镜头故障夹具差异 |
| NFR-001-007 | 来源、剧本、镜头、快照、Task/Attempt、Provider/模型、MediaVersion、Adoption 和 Delivery 必须双向可追踪。 | 数据库追踪查询 |
| NFR-001-008 | 最终 MP4 必须为 720×1280、24fps、H.264/AAC 48k、30～60 秒，并包含可听 TTS 与源语言字幕；音画偏差不高于 100ms。 | ffprobe、字幕和播放检查 |
| NFR-001-009 | Provider、数据库和对象存储秘密只来自现有服务端环境配置，不得进入 Git、前端 Bundle、数据库业务字段、TaskJob、日志或错误响应。 | secret 扫描与负向测试 |
| NFR-001-010 | 来源和媒体默认私有；候选预览与成片下载授权均不超过 15 分钟，日志不得记录正文、完整 Prompt、媒体 URL 或认证头。 | 授权与日志测试 |
| NFR-001-011 | 本地环境必须通过版本化 Compose 启动；干净环境中基于 lockfile 的确定性安装、迁移、类型、lint、测试和构建均可由 Plan 中固定命令执行。 | 命令退出码与制品摘要 |
| NFR-001-012 | Mock Provider 完整 E2E 应连续 3 次通过，每次不超过 10 分钟；真实 Provider 至少完成 1 次完整样片，但不纳入普通 PR CI。 | CI 与脱敏 smoke 证据 |
| NFR-001-013 | 输入、并发、Provider 轮询、下载和 FFmpeg 必须有显式大小、次数、超时与进程资源上限；超限应安全失败。 | 边界和资源耗尽测试 |
| NFR-001-014 | 结构化日志必须含 `release_version/request_id/task_id/attempt_id/job_id/error_code` 中适用字段，且用户能看到稳定错误码与下一动作。 | 日志关联与 UI 错误测试 |

## 3. 媒体与输入基线

- BR-NFR-001：来源为符合 REQ-02 `text-normalization-v1` 的 UTF-8 中文故事纯文本：规范化后 300～3,000 个 Unicode 代码点、至少一个 Han 字符且无禁用代码点；接收形态只有粘贴后的规范化纯文本。
- BR-NFR-002：分镜包含 6～10 个镜头，每镜 3～8 秒；总目标 30～60 秒；统一 `timebase=90000`，镜头时长按 24fps 对齐 3,750 ticks。
- BR-NFR-003：项目标题 1～120、来源最多 3,000、单个生成 Prompt 最多 4,000、单条字幕最多 500、整集字幕最多 20,000 Unicode 代码点。
- BR-NFR-004：单实例最多同时调用 3 个 Provider 任务，超出进入持久队列；每个 Task 最多 3 个自动 Attempt（初始 1 次+最多重试 2 次），每个 Attempt 最多提交 1 次。提交/状态请求分别限时 30/10 秒，状态轮询间隔 2～10 秒；Text/Image/TTS 与 Video 逻辑任务分别限时 120/600 秒且最多查询状态 60/300 次，以先到边界为准。
- BR-NFR-005：远端结果最多重定向 3 次、下载最多尝试 3 次且每次限时 120 秒；Image/Video/Audio 分别最多 20/256/32 MiB，SRT/Manifest 各最多 2 MiB，最终 MP4 最多 512 MiB；下载后必须计算 SHA-256 并通过 MIME、大小和可解码检查。
- BR-NFR-006：每个 FFmpeg 进程最多 2 threads、4 GiB RSS、300 秒墙钟时间和 2 GiB 临时目录；单一 Worker 容器最多 4 CPU/8 GiB，超限安全失败并保留 Attempt。
- BR-NFR-007：候选预览和成片下载授权 TTL 均为 900 秒，过期后必须重新授权。
- BR-NFR-008：TTS 按确认对白或旁白语音条目逐句生成；失败条目可独立重试，不得以静音冒充成功。
- BR-NFR-009：字幕来自确认对白或旁白语音条目，使用 90,000 ticks/秒整数时间基；文字必须为有效 UTF-8。

## 4. 质量契约边界

本文件的质量契约只有 NFR-001-001～014、BR-NFR-001～009 和 AC-NFR-001～003。未编号的质量目标不得成为实现前置条件，也不得据此预建平台组件或测试占位。

## 5. 验收标准

- AC-NFR-001：NFR-001-001～014 均有 Plan Test ID、命令和 Evidence ID；P0 不得以手工描述替代自动测试，真实 Provider smoke 除外。
- AC-NFR-002：固定六镜头夹具覆盖重复提交、单镜失败、Worker 重启、Provider unknown、TTS 失败和渲染失败。
- AC-NFR-003：所有未达到的 P0 结果判定 `failed`；外部 Provider 波动须保留事实，不得从样本中删除。
