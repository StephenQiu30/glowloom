# 时间线导出验证记录（2026-10-29）

此记录汇总本次工具执行结果，不是原始终端日志。工作区未提交，保留并行改动。

## 已执行并通过

- 前端：pnpm exec vitest run 'tests/unit/app/api/[...path]/timeline-export-route.test.ts' 'tests/unit/app/api/[...path]/route.test.ts' tests/unit/components/canvas/timeline/timeline-export.test.tsx --maxWorkers=2
  - 3 文件，83 项通过（BFF 新测试 2，既有 BFF 70，时间线导出 11）。
- 前端：pnpm exec eslint src/components/canvas/timeline/timeline-export.tsx tests/unit/components/canvas/timeline/timeline-export.test.tsx 'src/app/api/[...path]/route.ts' 'tests/unit/app/api/[...path]/timeline-export-route.test.ts'
- 前端：pnpm exec tsc --noEmit --pretty false
- 后端：go vet ./internal/mediatool/... ./internal/media/application ./internal/media/adapter/postgres ./internal/media/adapter/workflow ./internal/audit/application
- 后端真实隔离集成：TestTimelineExportSettingsDurableReplayAndAutomaticPublication 最终版 -race 通过：实际上传 PNG；ImportedAssets；Yjs 二进制保存混合视觉轨 image clip；生产 ProvideMediaExportStore；HTTP 创建；冻结 auto/480/名称/交付位置；幂等重放；FFmpeg 输出及720代理；系统源校验发布；HTTP 下载中文文件名。
- 后端真实隔离集成：TestProjectExportDurableFrozenCanvasArtifactReviewAndUnknownPutRecovery -race 通过（工程 ZIP / XML / unknown PUT recovery）。
- 后端既有 HTTP 集成：TestExportRealHTTPFrozenCanvasPrivateRenderingReviewAndDownload、TestExportAudioRealHTTPPrivateM4AWaveformReviewAndDownload 通过，包含真实 FFmpeg 和 Temporal SDK 测试工作流。
- 后端：TestTimelineExportHistoryRestoresNewestCreatedJob 通过，已先验证随机 UUID 排序错误再修复为 created_at/id 游标。
- 后端选择集：go test ./tests/media ./tests/audit -run '^(TestTimelineExportSettings|TestTimelineExportSelectedResolution|TestTimelineExplicitAspects|TestTimelineRenderer|TestExportAudioExtracts|TestExportAudioRejects|TestProjectExports|TestProjectExportVideoAudio|TestProjectExportRejects|TestProjectArtifact|TestMedia|TestRecorded)' -count=1 通过。无测试环境变量时数据库测试跳过；上述数据库测试另行显式执行通过。
- pnpm api:generate 已运行，生成 ExportSettings/请求字段与核心 ImportedAssets；没有手改生成类型。
- git diff --check（本 agent 负责的 tracked 目录/文件）通过。

## 环境边界

隔离 PostgreSQL 18 端口15547，非生产数据库；应用 runtime role 与 owner 仅用于合成 fixture。S3 协议 fixture 端口15548，实际读写字节并校验 SHA；不是云存储或 MinIO 验收。Compose 文件位于本目录 compose.test.yml，项目名 glowloom-timeline-test-20261029。测试服务由协作 agent 共同使用，完成后协调关闭。

未运行：golangci-lint/goimports/govulncheck（本机 PATH 无对应工具）；没有全量 Go/前端 build 测试。FCPXML/xmeml 结构与归档测试通过，但未启动 Final Cut Pro/DaVinci/Premiere 导入验收。剪映原生工程未实现，入口明确不可用，无伪造格式。

## 关键回归红→绿

1. BFF 缺少全部 media-exports 路由，浏览器请求在前端直接404：精确开放已有后端合同、限制参数及方法、保留鉴权/幂等头和附件流。
2. 明确拒绝创建后仍保留旧源 revision：仅明确400/403/404/409/422释放意图，网络/5xx未知结果保持相同幂等键重放。
3. 系统导出旧强制人工review：新 settings 任务经过真实字节/源身份验证后直接完成，不伪造owner声明；老任务保留其旧服务端状态。
4. auto画幅与720代理：依据冻结媒体尺寸生成偶数分辨率，实际渲染结果测试。
5. 导出历史用随机UUID排序：改为时间+UUID稳定游标。
6. artifact_id 已被导出审计事件发送但identity allowlist未包含：补入字段并通过旧HTTP导出审计验证。
