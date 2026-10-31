# 主体领域来源连接实现与验证

## 候选边界

`connectCanvasSubjectSources(state, sourceIDs, targetID, resolved, projectID, pinnedSubject?)` 仅返回一次完整不可变候选；`resolved` 为 `{ sourceNodeId, asset: API.AssetSummary }[]`，由统一 UI 异步取得已授权素材详情，并在等待后重查手势身份、来源/目标指纹与 epoch。helper 复核完整来源、项目、UUID、版本、当前内容资产、媒体类型、时长、重复、容量、循环和已有绑定。空主体取首个图片/视频作主来源；图片主来源最多3个图片辅助来源及1个2–5秒音色；视频主来源不接纳附加输入，视频时长已知且四舍五入秒数不超过15。任何失败不修改原图。

主体媒体增加可选 `source_node_id`，与 `asset_id` / `asset_revision` 固定版本及 native annotation 关系一起写入。它是可验证的节点来源绑定，不是展示标签，也不等于生成权限。普通手动资产和固定主体库 binding 不补此字段；历史手动主体的120参考协议仍可读，新连接按更窄的 authoring 容量校验。固定库目标缺少完整版本资料时拒绝，不静默覆盖；已有 local 可直接使用。

## 来源变化、手动编辑与迁移

- 保存校验要求来源节点仍存在，类型/资产与 pin 一致；Editor CONTENT 引用还必须是唯一且版本匹配。普通服务节点没有资产版本字段时，由现有媒体 owner 校验精确 revision，不能把节点 content_version 当作资产版本。
- source 替换为其他资产而仍保留旧绑定时拒绝保存，要求重新连接或显式断开；不静默改主体资产，也不以旧 pin 冒充新的来源。
- 删除来源或断边清除 `source_node_id`，保留已经固定的资产快照。主体输入和关系解除作为同一次候选；后端拒绝只断边或只去绑定留下另一半。
- 手动编辑、清空主体、导入主体库版本时，只移除之前代表领域来源的 native 关系，保留普通历史视觉注释与主体输出边。复制整个选择重映射 source id；单独复制主体清除外部 source id 并保留 pin；项目复制同时重映射节点与项目资产。
- 生成侧 `ReadSubjectInput` 在调用方授权事务内锁定并重读同文档来源及 annotation，完整校验后才返回冻结输入。后续既有媒体权限/版本预检保持生效；source_node_id 不提供独立访问权。

后端新增可选 Go 字段后执行 `pnpm run api:generate`，exit 0；生成 API 的 SubjectMedia.source_node_id 未手写。

## 验证事实

- 前端先 Red：新增测试导入不存在的 `canvas-subject-connections`，失败；实现后 Green。
- 后端先 Red：旧闭合配置拒绝 source_node_id，来源关系测试失败；字段及校验、复制/删除接线后 Green。
- `pnpm exec vitest run tests/unit/components/canvas/subject-input-connections.test.ts tests/unit/components/canvas/subject/subject-node-state.test.ts --maxWorkers=1`：32测试通过，原超时未放宽。
- `go test ./tests/canvas -run 'TestSubject(InputRelation|ConnectedDuration)' -count=1`：通过。
- `go test -race ./tests/canvas -run '^TestSubject(InputRelation|ConnectedDuration|Local|Node|Connection|Sources)' -count=1`：通过。
- `pnpm exec tsc --noEmit --pretty false`：exit 0；本轮文件范围 ESLint、Prettier 通过。
- `go vet ./internal/canvas/domain ./internal/canvas/adapter/postgres`：通过；owned Go 文件执行 gofmt/goimports。
- 同组 agent 执行 `/Users/stephenqiu/go/bin/golangci-lint run ./internal/canvas/domain/... ./internal/canvas/adapter/postgres/...`：0 issues。其 `/Users/stephenqiu/go/bin/govulncheck ./internal/canvas/domain ./internal/canvas/adapter/postgres`：0 reachable vulnerabilities、0 imported package vulnerabilities、1 required module vulnerability not called。该两项复用其工具终态，未另造磁盘原始日志；本 agent 初次 golangci 被并行执行锁拒绝，未把该次算作通过。

## 未通过与待验

`go test ./tests/canvas -count=1` 完整运行失败：当前执行环境缺少 `GLOWLOOM_TEST_CANVAS_DB_DSN`，数据库集成用例明确 fail，没有跳过或改为成功。新 `TestSubjectInputReaderRechecksCurrentSourceAndRelation` 已编译，也因同一 DSN 缺失未完成运行；它覆盖正式 Yjs fixture 保存后读取、跨项目拒绝、源资产替换及断边故障注入。对应真实数据库和产品页面接受、服务端读回、撤销/重做、刷新/重开由主任务继续验收。上述局部测试与静态校验不替代这些结果。

本轮未重启服务、未提交或推送；共享编辑器、主体/时间线并行改动均保留。

## 隔离 PostgreSQL 补测终态

主任务随后提供独立临时 PostgreSQL 18 测试库（`canvas_advanced_test`），未使用业务 `.env`、生产凭据或业务数据。`TestSubjectInputReaderRechecksCurrentSourceAndRelation` 以 Race Detector 运行，通过真实 Yjs fixture 创建并接受节点/领域关系后，在同一授权项目读取冻结主体输入；跨项目读取、源资产替换和断边故障注入均拒绝。最终 exit 0，用例 3.52 秒、命令总计 5.974 秒，见 `subject-input-reader-db-final.log`。

首次 DB 尝试正常保存、正常读取与跨项目拒绝已通过，但测试的故障注入直接 UPDATE 被现有 Yjs writer guard 拒绝（SQLSTATE 42501），见 `subject-input-reader-db-first.log`。修复只限测试夹具：针对该随机 document 的独立事务设置既有 writer 上下文，事务结束自动解除；未改产品守卫、未全局禁用触发器。此后源资产和关系故障注入得到预期拒绝。此前缺 DSN 的完整 canvas 包失败仍保留；此次专项补测不表示已重新跑过完整 canvas 数据库套件。

## 精确同步预览

新增独立 `assertCanvasSubjectSourcePreview(sources, { target?, pinnedSubject?, assets? })`，返回 `requiresMediaValidation` 与 `requiresSubjectValidation`。空目标的 video+audio、5 image、2 audio、重复来源/资产、处理失败等确定冲突同步拒绝；已有 local/已解析固定主体按实际剩余容量判断。未知时长保留权威媒体校验标记；未解析的固定主体保留异步主体校验标记，不永久禁用潜在合法连接。协调器应读取 exact version，并在解析后重新校验图与权限。

新增 preview 测试先因模块不存在 Red，实现后21/21通过（maxWorkers=1、原超时），涵盖混合类型、既有容量、重复、已知时长边界、未知时长/固定版本和权威素材事实覆盖过时展示 metadata。主任务负责将此 helper 接入菜单/拖入预览，本 agent 未编辑协调器、菜单或画布编辑器。
