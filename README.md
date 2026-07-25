# Lanverse

Lanverse 是一个 AI 短剧制作 MVP：让一名内部创作者把获权中文文本转换为剧本、分镜、图片、视频、逐句 TTS、字幕和可播放 MP4，并保证任务可恢复、结果可追溯。

当前仓库只包含治理文件和正式文档，尚未生成应用代码。统一入口见 [`docs/README.md`](docs/README.md)；产品边界见 [MVP目标与范围](docs/requirement/01-MVP目标与范围.md)，技术约束见 [技术与工程约束](docs/requirement/07-技术与工程约束.md)，架构与端到端闭环见 [MVP总体架构](docs/design/02-MVP总体架构.md) 和 [端到端制作流程](docs/design/05-端到端制作流程.md)，执行拆分见 [AI短剧端到端实施计划](docs/plans/01-AI短剧端到端实施计划.md)。

正式交付严格遵循 [`AGENTS.md`](AGENTS.md) 定义的 `Design → PRD → Plan → Acceptance`。当前 Requirement 为 `review`，Design、PRD 和 Plan 为 `draft`；只有逐级接受并取得 `implementation_start: passed` 记录后才能创建应用代码，Acceptance 只能在实现完成后建立。

## 获准实现时的唯一目录

```text
Lanverse/
├── backend/   # NestJS API、Temporal Workflow/Worker、Provider 与 FFmpeg 适配
├── frontend/  # Next.js 项目、故事、制作室、任务与交付页面
├── deploy/    # 本地 Compose 与容器定义
└── docs/      # Requirement、Design、PRD、Plan、Acceptance 与 Operations
```

API 与单一 Worker 同属 `backend/`。以上目录与正式文档中的六个业务模块共同构成完整实现清单；文档未定义的能力不得产生目录、接口、数据表、开关或占位代码。未满足实现准入前不会创建 `backend/`、`frontend/` 或 `deploy/`。
