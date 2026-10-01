# BeefTV 完整迁移验收进度

日期：2026-10-02（持续更新）。状态：持续实施，**完整迁移尚未完成**。本记录对应 [完整迁移设计](../design/BeefTV能力引入设计.md) 第 0 节；未完成项继续保留，不以阶段提交关闭任务。

## 已核验的实现提交范围

画布核心来源固定 `0d9e9f48d407570cd431ad9730cdd522b06810c0`；完整功能来源固定 `1ae25027f7ea1c2178e1e4133c36a0f2995d0e98`。保持自身 Next.js、React、TypeScript、shadcn/Radix、Go、PostgreSQL、Temporal 和私有存储。

本阶段恢复首页、项目、表单、画布、资产、任务、设置、许可入口；增加正式模型目录及管理 API、报价/确认/任务/取消 API、类型明确的 generation/batch/timeline/director 保存合同、GLB 验证上传、Codex 同步回执恢复和模型/提示词偏好。当前管理权限仍遵循 producer/admin 边界，本地是否授予配置管理能力待用户选择。

| 完整能力组            | 本阶段已核验的部分                                                                    | 仍需完成与产品验收                                                    |
| --------------------- | ------------------------------------------------------------------------------------- | --------------------------------------------------------------------- |
| 首页与创作入口        | 真实项目入口、上下文选择、导航、移动导航焦点/无横向溢出                               | 对照固定源码逐个入口与错误恢复复核                                    |
| 项目库与管理          | 实际项目创建、列表、设置、归档/回收恢复、打开画布                                     | 文件夹、复制、排序/完整搜索、项目包及 LibTV/TapNow 导入导出           |
| 表单式创作            | 原子保存到同一画布、同一节点刷新恢复、实际模型参数与有序用途输入、报价接口            | 真实供应商生成、候选采用与所有媒体/模式交互                           |
| 无限画布与历史        | 核心算法及持久命令、500 节点 LOD/定位/保存恢复                                        | 全手势/快捷键/媒体租约与历史/版本逐项产品验收                         |
| 资产库与素材选择      | 正式上传/审核确认、同项目私有预览、GLB 上传与授权读取                                 | 文件夹、批量处理/转移、容量信息与素材包                               |
| 批量创作与分镜        | 500 行实际保存/刷新、六列有序引用、150+150+150+50 分组报价/统一确认、跨批并发冻结限制 | 真实供应商 500 行执行、逐行候选采用、完整部分失败产品验收与镜头接线   |
| 任务、候选与诊断      | 实际状态查询、取消意图与提交发送权竞争、批次恢复                                      | 候选比较/采用、诊断导出、实时订阅及迟到结果完整验收                   |
| 模型与设置            | 管理发布/权限接口、模型与九模板偏好持久化、报价前模板编译冻结及历史幂等重放           | 全管理命令持久幂等、真实供应商/凭据验证、用户权限选择、生成消费者应用 |
| 提示词与图片工具      | 裁切/宫格与透明遮罩输入；实际遮罩上传并保存 image.edit 草稿                           | 已装配创作/分析工具、付费模型用途与真实输出、绘图标注等完整对照       |
| 视频/音频/字幕/时间轴 | 轨道/字幕/裁切保存与预览、实际后台 MP4/M4A、波形、审核/下载/加入画布并刷新恢复        | 真实转写；长任务取消/失败重试的浏览器产品验收及全部工具对照           |
| 3D 导演台             | 实际 WebGL、场景/摄影机/骨骼/关键帧输入、正式 GLB 接线                                | 机位图库/封面、白膜录制与参考工具；源视频深度/单图插件能力分别核验    |
| 插件与外部素材        | 固定源码入口与功能盘点                                                                | 已可启用插件配置/权限/执行、Eagle 选择与目标环境验证                  |
| 短剧业务与 Agent      | 统一工作区、画布和 Operation 的接入基础                                               | 已接受业务实体/九场景、提案/确认、短剧结果采用与真实运行              |
| 关闭/辅助入口         | 源关闭/未装配/开发中状态登记                                                          | 按实际装配继续核对；源未完成项不能标为迁移已验收                      |

## 固定提交快照质量验证

稳定文件先暂存并导出独立 Git tree；后续功能继续开发，不混入正在检查的快照。快照在本机 Go 1.26.8、PostgreSQL 17.11、真实 MinIO 和实际 FFmpeg 下验证；不能把它视为所有 CI/生产环境已验收。

| 验证                | 实际命令/结果                                                                                                                                                                                     |
| ------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Go 格式/静态检查    | `goimports -local github.com/StephenQiu30/lanverse/backend -l .` 无输出；`go vet ./...` 通过；`golangci-lint run ./...` 0 issues                                                                  |
| Go Race             | `go test -race ./...` 通过；缺失专用环境的集成测试按测试约定跳过，不能视为实际运行通过                                                                                                            |
| 实际画布 PostgreSQL | `LV_TEST_CANVAS_DB_DSN=<隔离库> go test -race ./tests/canvas -count=1` 通过；含 revision/幂等、媒体权限/锁、类型配置、时间轴真实尺寸裁切及外层事务锁                                              |
| 实际偏好 PostgreSQL | `LV_TEST_OPERATION_STORE_DB_DSN=<隔离库> go test -race ./tests/prompt -count=1` 通过；含并发首次写入、权限角色、幂等、过期基线与故障回滚                                                          |
| 实际接口与私有存储  | 隔离 PostgreSQL/MinIO 下 catalog、operation、media 的 AdminCatalogHTTP/Public/M1ReceiptRecoveryPrivate/M1StagedIngest/GLBUpload 定向 Race 测试通过；Codex 这里使用模拟 stdio 模型输出，非真实推理 |
| 漏洞检查            | `govulncheck ./...` 无可达符号或导入包漏洞；一个仅 required module 的漏洞未被当前代码调用                                                                                                         |
| Wire/Swagger        | 固定快照重新生成 Wire 和 Swagger，与提交文件逐字一致；正式 Router/Swagger 四项合同测试通过                                                                                                        |
| 在线客户端          | 从该快照编译并启动独立 API，41 个公开 operationId 全部存在；在线 Swagger 与固定生成物一致；重新生成的客户端与提交文件逐字一致                                                                     |
| 前端                | `eslint src --max-warnings 0`、`prettier --check .`、`next typegen`、`tsc --noEmit` 通过；Vitest **44 文件/248 测试**通过；`next build` 通过                                                      |

第三方 `schema/COPYING.adoc` 的原文末尾空行触发 `git diff --check` 提示，保持固定上游原字节；自有代码未发现空白错误。全部第三方来源和许可见 [THIRD_PARTY_NOTICES](../../THIRD_PARTY_NOTICES.md)。

## 真实浏览器与运行证据

- 表单在实际项目 `639cef6d-4651-4fca-8f17-5b22741ee32e` 创建画布 `2b7fa081-99f7-4d6d-af25-e2f6467e8769`、节点 `6f2b01c7-f912-468a-bffd-f7474b199510`；刷新恢复同一内容，画布读取同一节点并提供对应表单链接。未调用付费生成。
- 500 节点项目 `92ac5588-ea13-4a50-8d72-ac8802673ad2`、画布 `bfa5f296-97e7-481f-912e-e7969ff730e1`：100% 时 DOM 16 节点、适配全图 6% 时 DOM 280 节点、小地图 500 节点；实际编辑、定位、平移、缩放后刷新仍保留 500 节点与 revision 7 视口，浏览器未报错。
- 遮罩画布 `68793b9e-7f0d-4438-9718-b9b09a8e9645`：空遮罩拒绝提交；实际上传 256×192 PNG、创建 image.edit 草稿及引用；下载解码确认 15,681 个透明可编辑像素和 33,471 个白色不透明像素。草稿刷新恢复，未报告实际重绘通过。
- 设置页实际读取九操作偏好和项目默认模型（HTTP 200），普通 producer 管理接口返回 403；未保存切换提示、跨 tab 保留草稿和 390px 响应布局通过。偏好保存不等于九个业务生成消费者已完成。
- Codex 旧成功/未知/人工处理/取消四段 Temporal 历史在现有服务的独占测试 namespace 回放通过，无新增版本分支破坏旧事件；真实账户推理、收费与完整 Operation 输出仍待验收。

## 第二阶段固定快照与真实导出

本次稳定代码 tree `2d889ca7efcc6fe73d7136d7aaf7db0f2857b237` 包括视频导出、跨批并发、模板编译、字幕/裁切预览和完整 500 行保存。两个 `export_audio*` 测试为下一阶段真实 Red，保留工作区并明确排除该快照。

- 固定快照 `go test -race -count=1 ./...` 通过。偏好/Operation 的专用 PostgreSQL 命令使用 `LV_TEST_OPERATION_STORE_DB_DSN`；media 真实集成另需 `LV_TEST_MEDIA_STORE_DB_DSN`，不能以同一命令中缺少后者而跳过的测试充作证据。随后独立设置 media DSN、`LV_TEST_MEDIA_EXPORT_REQUIRE_ROLE=1` 和私有存储配置运行 `go test -race -count=1 -v ./tests/media -run '^Test(Export|GLB)'`，实际非 owner 角色通过（9.545 秒、无跳过）；覆盖正式 HTTP、生产 Activities、实际 FFmpeg、pending 结果审核/下载、审计去重、执行实际退出及列级权限。Temporal SDK 测试环境与下述真实运行分别记录。
- 固定快照 `go vet ./...`、`golangci-lint run ./...` 0 issues、goimports、lint、tsc、Prettier 和生产构建通过。前端 49 文件/264 测试在 `vitest run --maxWorkers=2` 全部通过。首次默认高并行运行有 6 项超过原 5 秒时限，未计通过，也未放宽时限；新增保存失败/未知请求两项随后定向验证。
- 实际在线 Swagger 除运行时自动补的空 host/schemes 外与固定文件一致，**50** 个 operationId 在生成客户端全部存在；生成目录单独用 `prettier --ignore-path /dev/null` 格式化，常规忽略配置不替代该检查。
- 模板真实 PostgreSQL 验证覆盖个人修订冻结、偏好变更后同键原回执重放、陈旧请求冲突、最终内容计费/输入哈希、跨组织/停用身份、无实体明确拒绝、批次逐项失败、外层回滚和 runtime UPDATE 拒绝。原文仍先校验，未启用模板的历史请求指纹不变，视频场景保留源旁路。业务实体缺失的六场景未宣称已装配。
- 500 行画布 `6a8aef53-4f40-4127-86e0-9d8964be3b5b`、节点 `9a273ae1-8340-4aa0-813f-a79a11f3d94e`：真实滚动到第 500 行并编辑、保存 HTTP 200/revision 3，刷新后末行文案保留。短视口中保存按钮可原生滚动到达，表格只渲染 7 行输入。500 行真实供应商执行仍未完成。
- 独占合成项目 `92ac5588-ea13-4a50-8d72-ac8802673ad2`、画布 `68793b9e-7f0d-4438-9718-b9b09a8e9645` 的 timeline `501adaeb-3a5b-4804-84ff-8d6c0c0ae859` 保存实际视频 trim/crop 与中文字幕。导出 job `08abe302-f222-4b9b-b163-826171978cd2` 冻结 revision 9；首次 relay 因 Kafka 缺少主题退出，保留 queued 事实。精确补齐源码声明的 20 个 Lanverse 主题后，原任务经 Outbox/Kafka/Temporal 自动恢复，未重复创建或直接绕过工作流执行。
- 真实 Temporal namespace `lanverse-local` 中 workflow `media-export/08abe302-f222-4b9b-b163-826171978cd2/1`、run `01a0f7df-02d2-7d9c-a276-5cc8031671b0` 完成实际活动（5.71 秒/11 事件）并返回 review_required。浏览器解码并实际播放 1920×1080/1.5 秒视频，显式审核 exact SHA/job revision 8 后状态 succeeded/progress 100/revision 9。
- 浏览器真实下载 MP4 为 297,716 字节，SHA256 `8bc988df2969a81f9582b4e9719b2f38f394895e2f3f00f98a318057bfc4ea4a` 与任务一致，ffprobe H.264 1920×1080@24fps、AAC、1.500 秒。已审核媒体 `54a6755c-bfaa-547e-8e78-475b5d370b52` 加入 resource 节点 `df33856f-f04a-4b08-9c9a-a9a038533d50` 与 annotation 边 `59c4627a-72a1-4980-89ae-9aa72319ed40`；画布 revision 10，刷新后素材、字幕/crop、导出历史均恢复。实际 SRT 下载包含 0..1200ms 中文字幕，浏览器无 console error。
- 字幕验证只经公开 Renderer 生成实际 MP4 再解码，覆盖中文/混排/换行/三种对齐/颜色/描边/透明背景；长时间线在公开 progress 边界停止用于验证有界规划，未把这项测试当成 24 小时编码成功。Go 测试全部位于 `backend/tests/`。

截图与机器证据在本次本地临时目录，正式合同/实现与可重复测试进入提交；没有将私有媒体文件、数据库或环境凭据加入 Git。

## 第三阶段：音频导出与项目恢复（2026-10-02）

- 090 保留旧导出的 `video` 默认值与原指纹，增加冻结的 `audio` 输出。隔离 PostgreSQL 17.11、实际 MinIO、FFmpeg 下运行 `GIN_MODE=release LV_TEST_MEDIA_EXPORT_REQUIRE_ROLE=1 LV_TEST_MEDIA_STORE_DB_DSN=<lanverse_http，options role=lanverse_app> LV_TEST_RECEIPT_STORAGE_CONFIG=<独占私有存储配置> go test -race ./tests/media -run 'TestExport|TestTimelineRenderer' -count=1` 通过（10.908 秒）。实际 M4A、波形、无音轨失败、50 段渲染中的取消/Worker 真正退出、旧回执重放与 output_kind UPDATE 拒绝均验证；无真实供应商调用。
- 预审核波形经独立授权查询，复核 job revision、输出 SHA、状态与资产后才签名。实际私有 PNG 解码 1280×256，错项目、旧修订和假 SHA 拒绝；独立真实 HTTP/存储测试通过（3.357 秒）。普通素材的 ready/passed 读取门禁保持。
- 浏览器 timeline `8eb8d810-91cd-45e9-89fb-2c1995fc988f` 创建 job `0424477e-4875-4a46-b763-5702ebf9aa26`，冻结画布 revision 20。原任务经实际 Outbox/Kafka/Temporal 成为 review_required；实际音频播放至 ended=1.5s、波形解码后，人工确认 SHA/job revision 7，状态 succeeded/progress 100。下载 M4A 25,477 字节，ffprobe AAC/48kHz/stereo/1.500s，SHA256 `37d5da2a38df65d13725c07d8be87d22538e6a58225a1d2c00d90e5e76d6f29f` 与任务一致。
- 该已审核音频资产 `271e704b-4cab-553f-a6a7-13d1700185e2` 导入 node `50bdcb91-d5f5-4d72-bfc0-68f9e2ee9775`、edge `1e0254b7` 开头的正式连线；重复导入仍只有一个节点/连线，画布 revision 23 不增长，浏览器刷新恢复。初始合成上传因旧 localhost 与当前精确 127.0.0.1 源不同明确 403，随后同主体/项目/原幂等键通过正式 HTTP 重放；这一步与后续真实浏览器操作分别记录。
- 无音轨视频的真实浏览器 audio job `2eb3dd83-2471-4271-83e8-7c3cddb14767` 经正式工作流失败为 `no_audio_stream`，job revision 4，结果 asset/SHA 均为 null；页面显示“此素材没有可提取音轨”，没有伪造下载或导入。
- 固定源的五种导演台模板已按自身正式节点合同接入选择弹窗。浏览器选择“空场景”创建节点 `3dfea93a-f5ca-4cbe-9d98-77c1cf675167`，画布 revision 26；保存后 objects=0/cameras=1/lights=3，刷新恢复。1280×577 短视口中实际 WebGL 672×307，542px 高 Dialog 可滚动，未出现横向溢出或按钮覆盖。截图字节图库/封面另行验收。
- 项目生命周期专用 PostgreSQL 的全部 workspace Race 通过。持久幂等、8 路同键并发、异键 CAS、主体/组织隔离、30 天恢复期限、Outbox/回执故障回滚与实际非 owner 权限均验证。实际观察 Operation、媒体接管和导出申请的项目行锁；非终态工作阻止归档/删除。软删恢复前后 canvas/node/media/Operation/budget/export 行未变，未执行对象 purge 或完整复制。
- 浏览器在自己的合成项目 `639cef6d-4651-4fca-8f17-5b22741ee32e` 实际修改名称、归档、移入回收站、恢复为原 archived、取消归档，并还原原名；最终 active/is_delete=false/revision 7，恢复时间字段均为 null。操作前后画布 `2b7fa081-99f7-4d6d-af25-e2f6467e8769` 仍 revision 2/一个生成节点，完整响应 SHA256 均为 `b1abeb0fe90cd3d6212967085c1a14150bf39ee3ca71c3bbfb4e06be2d94f059`；刷新后原项目显示正常。
- 新项目设置按需加载，使用既有 shadcn/Radix 与 React Hook Form；未知写入锁住编辑与关闭，原正文/原键核验，修订冲突读取最新后重新操作。详情拒绝错误项目 ID。定向核心测试遵循 Red→Green，前端项目测试首组 7 文件/27 项通过，新增错误项目边界另通过。
- 本轮旧 /tmp 进程/隔离环境消失，以当前事实重建独占验证环境。业务 PostgreSQL/对象存储保留；缺失的本机 Temporal `lanverse-local` 命名空间按 OPS 恢复 72h。CI 新增实际私有导出与模板/回执测试；MinIO 由官方固定源 `07c3a429bfed433e49018cb0f78a52145d4bedeb` 编译，避免本轮历史 Quay 镜像实际拉取 401 导致跳过验证。已实际编译、启动和验证私有对象，不改生产技术栈。

本阶段固定快照（Go 1.26.5）已执行：`gofmt/goimports`、`go vet ./...`、`golangci-lint run ./...` 0 issues、`govulncheck ./...` 无可达/导入包漏洞，保留一个未调用模块告警。全树 Race 顶层 330 通过/285 因专用条件跳过；独占 PostgreSQL 的 workspace 61 项全通过。真实私有 MinIO/FFmpeg、`lanverse_app` 非 owner 角色下 `^Test(Export|GLB)` 16 项全通过；偏好/Operation 206 项通过、15 项依赖专用 Temporal/Kafka/schema 环境跳过。偏好首次命令因隔离库名称不符合既有安全守卫失败，修正独占测试库与 CI 的名称后重跑通过，未削弱守卫。

前端固定快照 ESLint、Prettier、`next typegen`、`tsc --noEmit`、54 文件/280 测试与 Next 生产构建通过。构建首次因快照 node_modules 软链接越出 Turbopack 根目录失败，改为快照内按固定锁文件离线安装后通过；开发服务未受影响。固定 Wire/Swagger 重新生成逐字一致；当前在线 56 operationId 完整生成客户端，字段与源合同一致，生成目录单独显式格式化。该快照不含正在实施的 Whisper/typed 图库后端，完整清单仍开放。

截图和合成 M4A 仅保留当前临时目录。Whisper、导演台图库/封面/白膜、完整复制、真实生成与其余清单继续实施，尚未关闭完整迁移目标。

## 第四阶段：导演台截图图库与封面（2026-10-02）

固定源的镜头截图与封面已适配为正式 `asset_id` 关联。保存并关闭会先确认场景保存，再捕获 beauty 并进入现有人工上传确认；正式审核通过的图片才与图库、封面、资源节点和来源边原子关联。深度/法线诊断图不会自动成为 beauty 封面，未知命令仍用原正文和原幂等键恢复。

浏览器在画布 `68793b9e-7f0d-4438-9718-b9b09a8e9645` 完成三个图库条目、两个镜头共享机位的分组、封面选择与刷新恢复。正式 PNG 资产 `2a09ab81-1d95-4ae7-8e4a-60363e9b85c1` 为 450×253、7,855 字节，SHA256 `06d0adf8d2d1f5d637fdabeb1fcefb42955f35de936044d819ceb88fc689556e`。第二镜头的相同 beauty 字节复用正式资产，最终画布 revision 34，图库条目 `2de76d89-a9cc-45f5-8b4e-08a3e28d27c8` 引用该资产；资源节点与来源边未重复增长。首次灰模预览切换的捕获时序问题通过等待两个实际 beauty 渲染帧修正，灰模历史图片未被假称为最终 beauty。

1280×577 短视口实测 Dialog 1254×542，内容纵向滚动、无横向溢出；机位标签左右方向键焦点与私有图片解码通过。截图仅保留本任务临时目录。实际 PostgreSQL 图库合同 Race、授权、资产种类与审核状态、CAS、迟到结果保护和前端模型/组件测试通过；白膜视频与其余完整清单继续实施，本阶段不关闭完整导演台或迁移目标。

固定快照已执行 `go vet ./...`、`golangci-lint run ./...`（0 issues）、`govulncheck ./...`（无可达/导入包漏洞，保留一个未调用模块告警）；全树 Race 顶层 334 项通过、288 项缺少专用条件跳过，独占 PostgreSQL 的完整 canvas Race 另通过（4.139 秒）。前端 `pnpm exec eslint .`、Prettier、`next typegen`、`tsc --noEmit`、57 文件/291 测试与 Next 生产构建通过。再次捕获同一图库资产时恢复其最新封面且不重复条目的边界，经 Red→Green 另验证。快照未纳入后续转写、WebM 规范化与项目复制的在途实现。

## 数据与 Git 状态

已在目标库按原始 forward SQL 事务应用 `030_canvas_tool_inputs`、`040_operation_canvas_source`、`050_media_model`、`060_workspace_prompt_preferences`；没有重建业务数据库、清理历史数据或提升工作区用户角色。随后按原始 SQL 单事务应用 `070_media_export` 与 `080_operation_prompt_preparation`，核对目标为同一现有本地数据库实例及运行角色列级 ACL。实际本地配置使用 `postgres` 所有者账户；隔离验收使用 `lanverse_app` 非所有者角色，两者不混称。070 SHA256 `99db57b91cd58d8468167766e4883b0326df4cdccf88614ab0b6cd9c9a3679b3`；080 SHA256 `4df0025033b32e36a6d37e6320b5bb9250cea1e4c7ff455f37ec9bac4b18a7cc`。

090 已对既有本地 `lanverse` 库精确核对地址、端口、数据库 OID、实例启动时间和 070/080 前置列后原子应用，未改变 `lanverse_app` 的冻结字段权限。当前业务实例启动时间为 2026-10-01T23:01:22+08:00，不与前轮进程混称。

首次完整范围文档 `84de390` 与首批实现 `b9d7078` 已按用户授权提交并推送 main，后者 CI `36869957190` 后端/前端/镜像全部通过。第二阶段 `9813e24` 已推送 main，CI `36878090689` 后端/前端/镜像全部通过。第三阶段 `1d1b2ec` 已推送 main，CI `36893145336` 后端/前端/镜像全部通过；后续阶段持续以独立验收快照提交。未创建 PR、标签、合并或发布。
