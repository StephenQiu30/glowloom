# 重建后的 AI 网关验证

本目录验证新 `ai` 事实、官方 SDK 原生协议与现有 Go Execution/Media 组合；不读取旧 AI 钱包作为兜底，也不调用真实付费供应商。

只在独立测试库离线执行 `backend/db/schema.sql` 和 `backend/db/data.sql` 后运行：

```sh
GLOWLOOM_TEST_AI_REBUILD_DSN='host=/tmp dbname=glowloom_ai_rebuild_20261117 sslmode=disable' go test -race -count=1 ./tests/aigateway
```

命令工作目录为 `backend`。测试库名是示例；不得对已有业务库直接重放空库 SQL。测试生成独立用户、空间、模型、凭据和任务；目录不包含数据库迁移器。无此环境变量时，PostgreSQL 集成用例会跳过，纯计价与协议用例仍运行。媒体验证需要可执行的 FFmpeg/ffprobe，使用真实 PNG/MP3/波形和隔离对象 SDK。

覆盖：Chat Completions、Responses、Messages 的非流式/SSE/工具调用；缺失用量、半截 SSE、错误模型、零重试；并发预算与冻结、重复结算、原凭据恢复、未知结果对账；工作台、Agent、媒体交付、语音、本地转写、SDK 原报价重放、历史与作品投影。正式采购对账、真实第三方模型质量和生产部署需要单独验收。

视频计费专项可追加 `-run 'TestVideoToken'`。测试走真实创作报价、确认、任务轮询、可播放 MP4 归档和 PostgreSQL 积分账本。方舟 HTTP 响应和对象读写使用隔离夹具，覆盖预计积分、确认上限、交付后结算、实际用量超限、缺用量后原单对账及明确失败释放。

MVP 保持现有 Go 业务进程及 Execution worker 的共享业务事务装配，网关 HTTP 接入由应用代理到 Execution。资金路径不经过 Kafka credit 消费者。测试额外验证 `glowloom_app` 当前业务角色的完整准入/结算权限；独立 owner 角色的跨服务授权协议不在本轮部署范围，不能将本轮验证当作微服务完全隔离验收。
