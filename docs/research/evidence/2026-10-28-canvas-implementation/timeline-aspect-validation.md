# 时间线比例合同：持久化、预览与真实成片验证

日期：2026-10-28。来源为[源页比例菜单](advanced-source/timeline-aspect-menu.txt)、[高级工具补证](advanced-parity-contracts.md)和设计27 §7.4.2。这里只验证比例合同；时间线完整宿主、非空自动比例规则、全部剪辑和工程格式导出仍不能据此标为完成。

## 实现边界

- 源菜单的11个值完整保留：auto、2.35:1、21:9、1.85:1、16:9、3:2、4:3、1:1、3:4、2:3、9:16。
- Go `TimelineConfig.Validate` 接受上述闭合集合，拒绝空值/未知值/大小写别名；`VideoDimensions` 只解析十种显式比例。没有把 auto 静默改成16:9。
- 十种显式比例使用既有织光短边1080编码规格，尺寸精确对应所选比例且为偶数：2538×1080、2520×1080、1998×1080、1920×1080、1620×1080、1440×1080、1080×1080、1080×1440、1080×1620、1080×1920。它们不是声称已经实测到的即梦默认导出清晰度。
- FFmpeg编码与worker产物校验共同消费 `VideoDimensions`；媒体720p代理预览补齐相同比例，避免成片正确但预览报不支持或拉回16:9。
- auto可以保存重开；由于尚无源端非空自动取向证据，视频导出在创建job前以可判定 `ErrTimelineAspectUnresolved` 拒绝。实际Renderer也拒绝未解析auto。音频处理没有伪造视频尺寸。
- 前端新增可复用 `TimelineAspectSelect` 和 `timelinePreviewDimensions` / `timelinePreviewAspectRatio`。选择器使用项目现有shadcn/Radix Select、受控值、可访问名称、只读保护；未知值显示不支持并保留，不隐式覆盖。auto无权威尺寸返回null/undefined，宿主可传入权威解析尺寸供预览。当前尚未接入完整剪辑宿主，不能把这两文件算作已可操作的时间线产品。

## 已运行验证

新建的一次性PostgreSQL18库与资源输入测试共用，runtime为受限glowloom_app角色；未使用现有开发/生产库或读取.env。

| 检查 | 实际结果 |
| --- | --- |
| Red→Green | 原仅三比例时，auto及7个新增显式比例测试失败；扩展后全部通过 |
| PostgreSQL/Yjs -race | `TestTimelineAspectPostgresYjsRoundTripAndUnknownRollback`：11值逐个真实保存、新service/store重开精确读回；4个未知值拒绝，最终revision和9:16保持 |
| Domain -race | 闭合集合、精确偶数像素、未知值、auto未解析均通过；与上项合计7.858s |
| 实际FFmpeg/ffprobe -race | `TestTimelineExplicitAspectsRenderAndProbeActualVideoAndProxy`：10种120ms字幕成片逐个编码MP4，probe实际尺寸，再实际生成720p代理并核对精确比例；通过11.635s，无skip |
| 导出入队拒绝 -race | `TestTimelineAutoVideoExportRejectsBeforeJobCreation`：真实PostgreSQL，未解析auto返回两个可判定错误链，export_job计数0；通过2.322s |
| 既有媒体回归 -race | 中文字幕真实像素/描边/透明叠层、长字幕取消、图音组合/淡入淡出、音频提取与无音轨拒绝、竖屏预览确定性：9测试/子测试通过，0 fail/skip，9.660s |
| 前端 | 3个Vitest通过；11选项、用户选择、已存比例、未知值保留、只读、精确预览比例、auto未解析与权威尺寸路径；作用域ESLint通过；全前端tsc --noEmit通过 |
| Go质量 | gofmt/goimports；go vet和golangci-lint覆盖canvas/mediatool/media及对应tests，0 issues；govulncheck 0可达漏洞（依赖模块另有1个未调用漏洞提示） |

原始日志在本机 `/tmp/timeline-aspect-red.log`、`/tmp/timeline-aspect-pg.log`、`/tmp/timeline-aspect-render.log`、`/tmp/timeline-aspect-render-regression.json`、`/tmp/timeline-aspect-tsc.log`、`/tmp/timeline-aspect-lint.log`、`/tmp/timeline-aspect-vuln.log`。成片为测试临时目录中的真实文件，测试结束清理，未提交生成媒体。

## 宿主接入与未闭合项

1. 宿主将 `TimelineAspectSelect.onChange` 作为现有 `config.timeline.aspect_ratio` 的Yjs set事务；只在权威接受后使用新的canvas revision导出。
2. 预览使用 `timelinePreviewAspectRatio(savedRatio, resolvedAuto)`；不得把undefined降级写回16:9或本地覆盖已存auto。
3. 继续用源端已授权非空素材证实auto的空态、首素材、排序/删除首素材和不同尺寸素材行为，再实现权威冻结解析。当前auto不是通过项。
4. 本次实际编码是Renderer与代理层测试；没有将其声称为11比例的浏览器交互验收、真实对象存储完整发布链或即梦工程格式导出验收。

前端实现参考了项目指定的[React Best Practices技能](/Users/stephenqiu/.codex/plugins/cache/openai-curated-remote/vercel/0.54.1/skills/react-best-practices/SKILL.md)，采用渲染时派生值和事件回调；核验了[React官方派生状态指导](https://react.dev/learn/you-might-not-need-an-effect)与[Next.js客户端边界](https://nextjs.org/docs/app/api-reference/directives/use-client)。本环境未找到独立nextjs/SKILL.md，因此以官方文档补充，不宣称运行了缺失技能。

隔离容器于2026-10-29验证结束后已停止并由 `--rm` 自动删除；没有保留临时数据库。
