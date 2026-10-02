# BeefTV 完整迁移验收进度

日期：2026-10-02（持续更新）。状态：持续实施，**完整迁移尚未完成**。本记录对应 [完整迁移设计](../design/BeefTV能力引入设计.md) 第 0 节；未完成项继续保留，不以阶段提交关闭任务。

## 已核验的实现提交范围

画布核心来源固定 `0d9e9f48d407570cd431ad9730cdd522b06810c0`；完整功能来源固定 `1ae25027f7ea1c2178e1e4133c36a0f2995d0e98`。保持自身 Next.js、React、TypeScript、shadcn/Radix、Go、PostgreSQL、Temporal 和私有存储。

本阶段恢复首页、项目、表单、画布、资产、任务、设置、许可入口；增加正式模型目录及管理 API、报价/确认/任务/取消 API、类型明确的 generation/batch/timeline/director 保存合同、GLB 验证上传、Codex 同步回执恢复和模型/提示词偏好。当前管理权限仍遵循 producer/admin 边界，本地是否授予配置管理能力待用户选择。

| 完整能力组            | 本阶段已核验的部分                                                                                              | 仍需完成与产品验收                                                         |
| --------------------- | --------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------- |
| 首页与创作入口        | 真实项目入口、上下文选择、导航、移动导航焦点/无横向溢出                                                         | 对照固定源码逐个入口与错误恢复复核                                         |
| 项目库与管理          | 实际项目创建、列表、设置、归档/回收恢复、打开画布；现存完整内容复制及真实私有对象/多画布引用恢复                | 文件夹、源业务实体/对话复制、排序/完整搜索、项目包及 LibTV/TapNow 导入导出 |
| 表单式创作            | 原子保存到同一画布、同一节点刷新恢复、实际模型参数与有序用途输入、报价接口                                      | 真实供应商生成、候选采用与所有媒体/模式交互                                |
| 无限画布与历史        | 核心算法及持久命令、500 节点 LOD/定位/保存恢复                                                                  | 全手势/快捷键/媒体租约与历史/版本逐项产品验收                              |
| 资产库与素材选择      | 正式上传/审核确认、私有预览、GLB；个人/项目目录与元数据、个人原件上传、签名预览/原件附件和完整目录 Copy 已通过隔离 owning 检查 | 新素材库页面、真实跨库转移、物理回收/引用保护、容量信息与素材包 |
| 批量创作与分镜        | 500 行实际保存/刷新、六列有序引用、150+150+150+50 分组报价/统一确认、跨批并发冻结限制                           | 真实供应商 500 行执行、逐行候选采用、完整部分失败产品验收与镜头接线        |
| 任务、候选与诊断      | 实际状态查询、取消意图与提交发送权竞争、批次恢复                                                                | 候选比较/采用、诊断导出、实时订阅及迟到结果完整验收                        |
| 模型与设置            | 管理发布/权限接口、模型与九模板偏好持久化、报价前模板编译冻结及历史幂等重放                                     | 全管理命令持久幂等、真实供应商/凭据验证、用户权限选择、生成消费者应用      |
| 提示词与图片工具      | 裁切/宫格与透明遮罩输入；实际遮罩上传并保存 image.edit 草稿                                                     | 已装配创作/分析工具、付费模型用途与真实输出、绘图标注等完整对照            |
| 视频/音频/字幕/时间轴 | 轨道/字幕/裁切保存与预览、实际后台 MP4/M4A、波形；原生 Whisper 转写、人工稿件修订、SRT 下载与指定字幕轨保存恢复 | 长任务取消/失败重试的浏览器产品验收及全部工具对照                          |
| 3D 导演台             | 实际 WebGL、场景/摄影机/骨骼/关键帧输入、正式 GLB 接线；机位图库/封面、白膜视频录制与正式 MP4 保存恢复          | 参考工具及源视频深度/单图插件能力分别核验                                  |
| 插件与外部素材        | 固定源码入口与功能盘点                                                                                          | 已可启用插件配置/权限/执行、Eagle 选择与目标环境验证                       |
| 短剧业务与 Agent      | 剧本来源/恢复、全部版本历史、分集/结构人工确认、首次采纳与有序文件导入已通过隔离 owning 检查，页面实现已冻结 | 正式剧本浏览器链、角色/设定与下游失效、镜头、九场景Agent、真实AI与结果采用 |
| 关闭/辅助入口         | 源关闭/未装配/开发中状态登记                                                                                    | 按实际装配继续核对；源未完成项不能标为迁移已验收                           |

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

该阶段 main 提交 `c172c26d7b7b24fbb7b0528a952f69ccfe3f8d43` 已推送，GitHub Actions `36900418450` 的 frontend、backend、images 均成功；CI 不替代下述真实浏览器验收。

## 第五阶段：原生语音转写与字幕采纳（2026-10-02）

沿现有 Go/Temporal/私有媒体执行，010 只追加转写作业与不可变命令，不创建第二个 Agent 服务。`LV_WHISPER_ENDPOINT` 为可选本机服务根地址，未配置时新建/重试在任何副作用前明确拒绝，既有查询与取消仍可用。本机实际服务由固定 whisper.cpp `927cfce34f31707e17f2bff35c349632fb9e2c3a` 编译，使用已核验 base 模型原字节；启动、权重来源和限制见 [OPS §6.1](../operation/01-环境与部署.md#61-本机字幕转写服务)。本轮未验证中文识别质量。

- 业务 PostgreSQL 实例身份与 090 前置事实匹配后，010 原子应用；SHA256 `4601e42db2d5bbda7d51162e84c6db0696d350330ddc9520ac6b130e5700d7cd`。实际应用角色不能 UPDATE 冻结 source/project/canvas/node/language 或命令，Worker 可写阶段及结果。精确补齐 `lanverse.mediatool.transcription_command.v1`，保留既有主题和业务数据。
- 公开七操作由 Go 注解生成 Swagger，实际 API 在线合同为 63 operationId。实际非 owner PostgreSQL、私有存储、FFmpeg、原生 Whisper 的定向 Race 18 个顶层测试通过且无跳过（4.953 秒）；源画布实际 CAS/组织/素材种类/审核状态/SHA/外层锁拒绝路径独立验证。服务实际超时或退出不明保留 unknown，不重复提交；单元/数据库取消证据不冒充全部浏览器取消已验收。
- 浏览器项目 `2e73b1b8-1d99-4907-89aa-7ae8ca3c6cf6`、画布 `d15db41f-a230-43cc-9339-be529c02b0f1` 的正式音频资产 `b2afcf41-fe1b-4c72-8042-ebc8413bec64`，SHA256 `59dfb9a4acb36fe2a2affc14bacbee2920ff435cb13cc314a08c13f66ba7860e`。转写前实际保存最新 revision 5；job `2999e6e7-2cc8-44f8-af7a-11fc8c1951fa` 经 Outbox/Kafka/Temporal/真实原生服务 succeeded，result SHA256 `4e12af8577ad1538dc8431a8e264029bf2833eba5480c009a6bcf47c26c144ec`。`lanverse-local` 的 `media-transcription/2999e6e7-2cc8-44f8-af7a-11fc8c1951fa/1` 实际 Completed，11 个历史事件；17:52:08.256754Z 开始、17:52:08.984020Z 关闭。
- 实际原音频原生播放器可播放。人工修订稿件标点、选择源音轨与未锁定字幕轨并明确复核后，只一次采纳 CAS 5→6；字幕 clip `89890661-4d20-4d35-a656-fdbe1cbe60bd` 为 start 5000ms/duration 9000ms，按源 trim 500ms 正确裁切/偏移。源音轨身份、引用及其他轨道保持，刷新恢复人工文本和时间位置。
- 真实 SRT 下载 141 字节，与默认 Downloads 和显式下载逐字一致，SHA256 `32ee11ac3ba594c60c60609efa6ac33eb54194cf26f3871d867ef6313eee310b`。390×720 视口的轨道键盘选择与复核操作通过。无效终点 0 显示友好错误，未写入字幕；画布定位产生独立视口 CAS 6→7，经逐 ID 语义核对全部 nodes/edges/config 未变。

## 第六阶段：真实白膜视频与 canonical MP4（2026-10-02）

固定源录制流程在当前 shadcn 导演台工作台完成：保存镜头/场景修订后录制实际 WebGL 动态帧，先本地播放和人工确认，再经真实 FFmpeg 规范化及媒体审核进入正式画布，不建立供应商 Operation。

- 原始录制为 VP9 WebM，30,583 字节，SHA256 `003af67bc02ff225ca5a3874f0f176983e9b40a9e4bad6b0f4c13595fa0635d4`；本地解码 450×253/1.910s、实际播放。输入 SHA/大小保持上传幂等回执，正式 canonical 资产 `1c5a68f2-fee8-4b9b-8008-2d3449bd3b8a` 为 H.264 MP4、450×254、1917ms、10,410 字节，SHA256 `632b7338a5e54cd263fbb3c785fd5a22f67e9711104a0b89653c0ad645244bdf`；typed normalization 证明两者身份和实际探测事实，不改名冒充转码。
- 正式视频在浏览器实际播放，35 帧解码且 35 个帧哈希不同，确认存在运镜动态。最终画布 `68793b9e-7f0d-4438-9718-b9b09a8e9645` revision 44、17 节点/11 连线，唯一资源 `4b9d8e5d-61e4-4025-9f0f-20b63279648e`、来源边 `4e03a01d-5ba5-455e-a27f-86f7642e301d`；刷新恢复相同关联。MediaRecorder 目标 24fps，实际 `avg_frame_rate=420/23`，未宣称严格固定帧率。
- 首次冷打开的 Radix scale 动画曾导致视口尺寸变化而取消录制；仅导演台入口移除 scale 动画后，冷打开第一次实际录制成功。录制 250ms 后取消，等待 2.5s 仍明确取消、无上传/新增资产且可再次录制。所有结束路径关闭 tracks/回调/监听；本地预览关闭或卸载停止播放、清空来源并释放临时 URL，经 Red→Green 验证。
- 固定上传后端的真实非 owner PostgreSQL/私有对象/FFmpeg、旧 SHA 回执、未知提交清理和 VP8/VP9 typed 证明已验证；根额外九项定向 Race 通过且无跳过。录制/导入前端 14 文件/60 测试、TypeScript、所属 ESLint 通过，最终组合完整门禁另记录。

项目复制、源业务实体、真实供应商生成、插件及其余完整清单继续执行；上述两个闭环不关闭全迁移目标。项目取消归档/恢复现在也检查同事务工作事实；纯逻辑与真实 PostgreSQL 先复现未完成复制可绕过恢复的问题，再验证阻塞或读失败均无项目改动/无幂等成功回执，完整 workspace Race 通过（4.861 秒）。

第五/六阶段组合固定快照已执行 Go 1.26.8 的 gofmt/goimports、`go vet ./...`、`golangci-lint run ./...`（0 issues）、`govulncheck ./...`（无可达或导入包漏洞，一个未调用模块告警）；全树 Race 顶层 348 通过、306 因专用外部条件跳过。完整 canvas PostgreSQL Race 通过（2.758 秒）；实际非 owner/私有对象/原生转写 15 项通过（3.975 秒），真实 Whisper 与协议 3 项通过（2.213 秒），上传规范化 9 项通过（4.037 秒），三组均无跳过。Wire 与按 PROJECT 指定 `--parseInternal --parseDependency` 新生成的 Swagger 逐字一致；正式 Router/Swagger 合同验证通过，当前在线 63 操作完整生成客户端并通过 TypeScript 语法检查、独立显式格式化。生成器内置格式化警告未被算作格式通过，最终 Prettier/TS 门禁另实际执行。

前端 `pnpm exec eslint .`、Prettier、`next typegen`、`tsc --noEmit`、65 文件/319 测试及 `next build` 均通过。固定快照内按锁文件离线安装依赖，未打断现有开发服务。该检查点不包含仍在实施的 CopyJob/020 迁移及新插件执行。

## 数据与 Git 状态

已在目标库按原始 forward SQL 事务应用 `030_canvas_tool_inputs`、`040_operation_canvas_source`、`050_media_model`、`060_workspace_prompt_preferences`；没有重建业务数据库、清理历史数据或提升工作区用户角色。随后按原始 SQL 单事务应用 `070_media_export` 与 `080_operation_prompt_preparation`，核对目标为同一现有本地数据库实例及运行角色列级 ACL。实际本地配置使用 `postgres` 所有者账户；隔离验收使用 `lanverse_app` 非所有者角色，两者不混称。070 SHA256 `99db57b91cd58d8468167766e4883b0326df4cdccf88614ab0b6cd9c9a3679b3`；080 SHA256 `4df0025033b32e36a6d37e6320b5bb9250cea1e4c7ff455f37ec9bac4b18a7cc`。

090 已对既有本地 `lanverse` 库精确核对地址、端口、数据库 OID、实例启动时间和 070/080 前置列后原子应用，未改变 `lanverse_app` 的冻结字段权限。当前业务实例启动时间为 2026-10-01T23:01:22+08:00，不与前轮进程混称。

首次完整范围文档 `84de390` 与首批实现 `b9d7078` 已按用户授权提交并推送 main，后者 CI `36869957190` 后端/前端/镜像全部通过。第二阶段 `9813e24` 已推送 main，CI `36878090689` 后端/前端/镜像全部通过。第三阶段 `1d1b2ec` 已推送 main，CI `36893145336` 后端/前端/镜像全部通过；后续阶段持续以独立验收快照提交。未创建 PR、标签、合并或发布。

## 第七阶段：完整当前项目内容复制（2026-10-02）

此闭环完成当前正式项目的全部内容复制，仍保留固定源码中尚未迁移的 units/剧集/镜头、独立对话和文件夹缺口。采用冻结清单、独立身份和私有对象、实际持久回执及原子发布；失败目标在正式列表和普通画布入口隐藏。

- 020 迁移在同一原业务 PostgreSQL（OID 345144296）及 090/010 前置核验后原子应用，up SHA256 `e1da69383eab8e46cbb69c3feb8bc9e1e74a5407398ba82de1a9613023ce4f3f`。不可变快照、命令及对象意图字段受到实际列权限约束。补齐源码两个 copy Kafka 主题，保留原有服务与数据；公开六入口由 Go 注解产生，正式 Router/在线 Swagger 为 69 个 operationId。
- 合成源项目 `a5e2e031-fcef-4a11-b528-da6f3da022d4`，目标 `487fe218-69c6-4a06-9d71-8bb08f429259`，同一个 job `0dd119e8-3b2d-474d-9cad-9bf40e9f027b` 实际完成 succeeded/complete、revision 8/attempt 2。原创建未知回执通过人工操作用原 key/body 核验，没有生成第二个任务或目标；前端接受合法 attempt 0，并以独立当前源查询防止缓存旧项目冒充权限已复核。
- 真实业务数据库时区 +08:00 与冻结 UTC 的同一时间先误触发发布拒绝。跨 Asia/Shanghai/America/New_York/UTC 的真实 PostgreSQL 回归先失败后通过；统一比较 UTC 后，实际时间改变 1 微秒仍拒绝。真实 Temporal SDK 默认将已运行 workflow 当作成功 handle，导致原 retry 未发送 signal；显式启用 AlreadyStarted 错误后真实 Temporal 回归证明同 run 仅一次 start、一次 retry signal。
- 原合成 retry 的唯一错误 `project-copy-command` Inbox marker，在精确数据库/源/目标/queued revision 6/无 worker/无实际 signal 的核验后撤销一次；通过真实 Kafka 重投原 event/key/body，未手工改 job/project/object 或绕过工作流发送 signal。真实 `project-copy/0dd119e8-3b2d-474d-9cad-9bf40e9f027b` 完成 23 个历史事件。
- 公开 GET 验证 2 张目标画布（revision 1）、9 节点/2 边、4 个独立原件及 5 个衍生物；35 个定义身份全部独立且引用闭合，timeline 的 3 clips/5 tracks，director 的 scene/object/camera/light/shot/keyframe/screenshot/cover 均映射，语义与几何保持。源完整 SHA256 `66235aabe425208be4e037098ee132a2ddab5d8eb4c1ff2dd8b2f1adb459750a` 始终不变。
- 副本浏览器实际加载图片 160×90、播放视频 2 秒及音频 3 秒、WebGL2 渲染自包含 GLB；导演场景 3D、图库与封面实际可读，时间线实际播放视频/音频/图片且无播放器错误。390×844 视口页面宽度 390、对话框 358，无横向溢出；刷新恢复同任务、明确副本入口和主画布 7 个节点。全部取消/未知写入/来源隔离的数据库与私有存储证据独立保留，未宣称全部取消路径已做浏览器验收。

固定 Copy 快照在 Go 1.26.8 下 `gofmt/goimports`、`go vet ./...`、`golangci-lint run ./...`（0 issues）、`govulncheck ./...`（无可达/导入包漏洞，一个未调用 required module 提示）通过。全树 Race 顶层 359 通过/318 缺专用环境跳过；实际隔离 PostgreSQL、私有 MinIO、真实 Temporal 与四项正式 Router/Swagger 合同的定向 Race 27 顶层通过、无跳过。前端 68 文件/353 测试、全量 ESLint、生成接口与所属格式检查、TypeScript 及 Next.js 生产构建通过。Wire/Swag 固定快照重新生成逐字一致。CI 增加真实 PostgreSQL/私有对象 Copy 门禁；真实 Temporal 回归依赖专用环境，不能把通用 CI 中跳过视为服务验收。

第七阶段提交 `29ce5664c6ad2f712e59c71ea1ad506441a98466` 已推送 main，CI `36919525973` 的 backend/frontend/images 全部完成且通过。

## 第八阶段：原生视频深度、人工审核与恢复（2026-10-02）

此阶段完成固定 Small/MPS profile 的视频节点深度处理闭环。完整来源迁移仍继续，项目目录、正式剧本/分集/镜头、历史对话、分类及内容包等未完成项不因本阶段通过而关闭。

- `202610020030_media_depth` 在原业务 PostgreSQL（OID 345144296）核验前置结构和实际运行任务后单事务应用，up SHA256 `e38543934eaa1ecc1b5a531033aee504da4d00cae07674099dc3ba89f62340da`；实际不可变列 ACL 保持。API、worker、relay 使用同一已验证二进制，三个 health 为 200；正式 Router/在线 Swagger 为 78 个 operationId，九个 Depth 入口和生成客户端逐字一致。沿用既有 Temporal namespace `lanverse-local`，没有新建或切换 namespace。
- 固定 VDA/Small SHA 和 18 文件运行包见设计 §14、许可页及部署文档 §6.3。Python 3.11.15/Torch 2.14.0/MPS 实际执行；唯一原生 Runner 串行，8 GiB 为采样监控上限，不能当操作系统硬隔离。2 秒、15 秒、最大 453 帧/960×960 输入均完整解码、输出且实际停止，裁剪运行包前后结果字节相同。本阶段不提供 CPU/Linux/Windows 的真实原生验收。
- 正式画布源项目 `efea9b62-33cf-41ff-af67-a72d897d99a3`、画布 `7a61890a-58da-4cd0-a96c-6ad953270d26`、视频节点 `ea5cfb98-81d2-44ef-b295-d59fa29083e4`。主 job `bebcf96d-b0fa-4640-ab7d-b4f89fe17267` 在 attempt 1/revision 14 成功；结果资产 `d65bcbef-0aad-53e3-a920-4360978b773c` 为 658532 字节，SHA256 `983e9debd1d510646fb05912c3cc0b9f8f78131180228e7c695db70235207213`。真实字节为 1920×1080、H.264/yuv420p、无音轨、25fps/50 帧/2000ms，未将低于 30fps 的输入强制报为 30fps。独立私有读取/ffprobe/全片解码与实际浏览器 0.083 秒至 2 秒 ended/50 decoded frames 相互核对。
- 审核前正式下载返回 409；实际浏览器明确 SHA/revision 审核后下载同 SHA 的 MP4。采纳经正式画布命令从 revision 2 到 3，生成唯一结果节点 `319e867d-82e5-4e1b-bd00-1f6f04f9f512` 和来源边 `2b3c0681-07b9-49c5-8e01-d29a637df679`。刷新及再次采纳保持 revision 3、2 节点/1 边；源节点属性和原视频字节 SHA `f63362f4f08e195198d2818247f6743d4a1fcbae2e79349be0ae4459e7c2e6d6` 不变。
- 第二 job `96b8f834-e7bf-4b2c-8ff8-54f47c912818` 实际 running 后由页面取消，attempt 1/revision 13 为 cancelled、无结果、execution_unconfirmed=false；同 job 明确重试 attempt 2，实际完整预览/审核后 revision 27 成功。此浏览器实验没有瞬时观察到原生 child PID，不能单独当进程树停止证据；独立真实模型/PG 回归确实观察拥有的原生 PID 和停止事实。
- 浏览器在审核发送前阻断 POST，刷新仍保存原 actor/org/origin、UUID key、revision 与 SHA，人工同键核验后成功；没有证明服务器已审核而响应丢失。第三 job `3b66283a-faa2-4f46-a390-296dd68b42ab` 复现禁用旧审核按钮导致焦点出 Dialog 的缺陷，Red→Green 后焦点进入“核验原审核请求”，Tab/ShiftTab 保留在 Dialog，Enter 用原 key/body 恢复 revision 14 并清除未知意图。服务器已提交的永久审核回执重放由真实 PostgreSQL 测试另行验证。
- 四次真实 Temporal 历史分别 12/13/12/12 个事件、全部 Completed。成功原生 receipt 分别为 7437/6070/7063ms、采样 RSS 1236844544/1146535936/1133527040 字节，均 groupJoined=true；取消 attempt 无成功结果 receipt。每个实际尝试的三个安全审计事件均已由 relay 发布，实际 audit 表每事件恰一行、Inbox marker 存在，证据不记录对象键/签名 URL/输入载荷。
- 390×844 页面宽 390、Dialog 358，无横向溢出；关闭回到原视频节点。最终完整页面 axe 4.12.1 WCAG 2A/AA+2.1AA 为零 violation/零 incomplete；结果 Dialog 为零 violation、三项 incomplete，按真实键盘焦点、无音轨与移动截图逐项人工核验，未混称自动全部通过。

固定后端 55 文件在 Go 1.26.8 下 goimports、`go vet ./...`、`golangci-lint run ./...`（0 issues）、`govulncheck ./...`（无可达/导入包漏洞，一个未调用 required module 提示）及生产二进制构建通过；全树 Race 顶层 379 通过/337 缺专用环境跳过。真实 Small/MPS、非 owner PostgreSQL、私有 MinIO、正式 HTTP/画布和 Temporal 的 `TestDepthJob` 定向 Race 21 顶层通过、零跳过（69.525s），与固定候选相关代码逐字相同。新增控制者撤权回归保持原 intent/对象/attempt 不变，重放不再次推理；无进程/无对象且完整原控制证据的撤权失败允许当前主体明确新取消，缺证据或未知物理事实仍保持 fence。CI 增加真实 PostgreSQL/私有对象 Depth 门禁，至少九项通过且零失败/零跳过；Linux CI 不冒充 MPS 前向验收。

固定前端 27 文件全量 ESLint（零警告）、全量 Prettier、76 文件/389 测试、`tsc --noEmit` 和 Next.js 生产构建通过。正式 8080 的在线 78-operation Swagger 重新生成全部 12 个客户端文件，显式格式检查后逐字相同；不采用生成器忽略的格式化错误作为验证。完整安全原始证据保存在独占临时验收目录，未提交私有媒体、签名 URL、cookie、环境配置或运行日志。

第八阶段提交 `b9e50650497ab0fb0c922e47ca9666b85cb5c64c` 已推送 main，CI `36933243710` 的 backend/frontend/images 全部完成且通过。

## 第九阶段：个人项目目录与原子复制放置（2026-10-02）

本阶段将固定来源的扁平目录适配为当前 actor/org 的正式分类。允许同名目录，根目录和其他目录的查询在服务端分页前过滤；目录 revision 与个人 placement revision 独立于项目内容 revision。目录回收在一个事务中验证全部成员和全部真实工作 owner，保留原项目状态及 30 天恢复期，单独恢复项目回到根目录。

- `202610020040_workspace_project_folder` 在同一原业务 PostgreSQL（OID 345144296、相同实例启动时间）核验 030 结构及未完成工作后原子应用，up SHA256 `70c9fb739d3b1a3a3e2c0042889fbf9ebd542be4680457aa86044fcc20987a4c`；没有重置数据、数据库、工作区身份或角色。应用角色只可插入永久目录命令，冻结身份和历史没有额外 UPDATE/DELETE 权限。
- Copy admission 在项目行锁之前取个人库/目录锁，同一事务冻结来源分类、创建隐藏目标和挂入相同目录；页面没有第二次移动请求。旧请求缺省 placement 时，既有 request fingerprint 和旧 manifest 编码保持逐字兼容。重放先核当前授权再读永久回执，目录后来改名或来源移动不改变原任务。发布核真实目标 placement，完整私有对象清理后取消只将未发布 copying 目标留为不可恢复的墓碑。
- 固定后端 44 文件在 Go 1.26.8 下 goimports、`go vet ./...`、`golangci-lint run ./...`（0 issues）、`govulncheck ./...`（无可达/导入包漏洞，一个未调用 required module 提示）与生产构建通过；全树 Race 顶层 386 通过、361 缺专用条件跳过。隔离非 owner PostgreSQL 的 `^TestProject(Folder|CopyPlacement)` 30 顶层全通过、零跳过；旧 workspace/Copy 28 项及真实私有媒体/HTTP Copy 四项独立回归均零跳过。CAS、撤权、成员并发、六类 guard、审计/Outbox 故障回滚、取消墓碑和恢复拒绝分别核验。
- API、worker、relay 使用同一 final2 二进制（SHA256 `2a6d7482db842975d19b4c77dc16415475a85fcfa022b877f27550060537f3a1`），实际三项 health=200；正式 Router/在线 Swagger 为 83 个 operationId。明确 `cover=null` 的 x-nullable 合同从 Go 注解生成，12 个在线生成客户端在显式 Prettier 后重新生成逐字相同。Wire/Swag 固定快照重生逐字相同。

- 真实浏览器创建两个同名独立目录，主目录 `bb2866d4-7714-4565-a481-01232a00092b` 与另一个 `a9b4d6e1-b77f-4ca4-b6c3-d72596e1b6bd`；Unicode 改名、本地单图片检查/版权确认上传、正式封面保存与明确清除、根目录往返及三种 revision 移动通过。封面上传资产 `b92b9f1a-03fd-4bc5-ad8f-dd335bfdaac8` 为 96×64/1873 字节，SHA256 `2047eeeb4a8b74a5b7aa7437434f61a7c95a6137103b96db286fa3fa5c2cd531`，未追加画布节点。
- 同目录 Copy job `2fd231d0-630b-467e-b059-95e3b207a9b7` 实际 succeeded/revision 5，将源 `34a935a8-c861-415e-86d6-87b64a7d98a9` 复制为 `65d1e8e7-74b4-4705-9509-c746d13350ce`；一画布、两正式图片、四衍生物全部完成。项目/画布/两资产/节点共五组身份独立映射，两个原件字节分别与副本 SHA 相同；源完整画布 revision 2、SHA256 `4af4b67a065a0102e929962459b824bb392b27af8927ccd368bdab5f2114bc35` 不变。
- 浏览器第二次复制后实际发出取消；job `ef24b0ca-1815-4c8d-8224-d66bf75c2cec` 为 cancelled/revision 4，无未知执行/待对账，完成内容数为零。数据库确认目标 `dd76b4c0-8138-4283-8781-fa8750a1762d` 是 copying/is_delete=true 的不可恢复墓碑，不报告浏览器观察到了物理原生进程。这两个真实 Temporal workflow 各 12 个历史事件、均 Completed。
- 主目录实际一次回收 active 副本和 archived 源，两个项目的 30 天 purge_after 相同，分类都变为 root；随后只用页面恢复源，保留 archived/根目录，目录没有复活，源画布与两媒体事实逐字保持。另一目录的封面在来源回收后 unavailable/null，恢复后重新授权为原正式图片。精确目录/项目/Copy 的 21 个真实审计事件全部发布，每事件 audit 恰一行且 Inbox 存在。
- 真实发送前 abort 后刷新保留相同 scope/key/body，键盘 Enter 以原 key `035d7772-ea9c-48db-ac9d-92a362afe880` 人工核验成功；此证据不冒充服务端成功后丢响应，提交后未知回执由后端实际测试另证明。额外 25 个独占空目录实际跨页、带 cursor 刷新、末页目录进入/面包屑与清 cursor 通过，全部仅页面回收清理；390×844 页面宽度390，Dialog 的 Tab/Escape 与精确触发按钮焦点恢复通过。

固定前端 22 文件与正式在线生成客户端在独立 HEAD+allowlist 快照完成全树 ESLint（零警告）、全树 Prettier、Next typegen、tsc、82 文件/415 测试及 Next.js 生产构建。首次共享树运行旧 Copy beforeunload 断言失败（414/415）保留记录；旧 Copy 定向 12/12、第二次共享树 415/415 和根协调者独立冻结快照 415/415 均通过，没有更改该断言、超时或 Copy 代码。实际 390px Dialog 连续八次 Tab 保持内部、Escape 回精确目录菜单；axe 无 violation，仍有背景对比度/遮挡计算 incomplete，经实际截图和焦点核验，未报告为自动全部完成。录像停止工具因 daemon 重试失败，JSON/截图/键盘证据保留，不报告完整录像。目录切片不关闭项目自身封面、个人/项目素材分类、正式剧本/分集/镜头、对话及内容包的剩余范围。

第九阶段提交 `93be9b330efcf7da6d13121f866b8b995876536b` 已推送 main，CI `36940184637` 的 backend/frontend/images 全部完成且通过。

## 第十阶段：项目主图、永久设置回执与文档原件（2026-10-02）

本阶段完成项目主图的上传、项目图片选择、替换、移除、卡片/设置预览和完整复制，并将 TXT/DOCX 原件接入既有正式媒体生命周期。个人素材库、剧本文件导入、全部剧本历史和其 Copy 仍持续实施；文档原件通过不等于剧本抽取或完整迁移完成。

- `051_media_document` 与 `052_workspace_project_cover` 在原业务 PostgreSQL OID 345144296、相同实例启动时间下，核验冻结 SQL SHA、040 前置结构及全部真实 owner 无进行中任务后，单事务应用；既有业务计数不变，没有重置数据或修改用户角色。051 up SHA256 `0951e3dea3907c4ef555021ce356207af68ef8666e792ba00084e1bb99945f23`、052 up SHA256 `b982fc171fc1f25104ccbe024681bf6b226ef857cf00611e69c900749cd0d175`。050/053 剧本增量尚未应用于业务库。
- 文档只接受真实 TXT/DOCX、完整 SHA 与大小证明；DOCX ZIP CRC、部件数量/解压大小和 XML 深度/节点均有界，不运行宏或外部关系。1..20 MiB 私有原件沿用版权、审核、durable upload 和独立复制，不执行音视频探测。下载重新核实际原件 SHA/大小，使用 attachment/private-no-store/nosniff，不向 DTO 暴露对象 key。NOT VALID 保留旧行，后续 owning 读/写/复制仍严格核验，不把旧数据保留当作合格证明。
- 主图 nullable FK 只绑定当前项目有效、已审核图片；PATCH omitted 保持、null 明确清除、zero UUID 拒绝。当前授权/CAS、归档只读和媒体证明共享事务行锁。项目修改、归档、取消归档、回收和恢复统一永久 command owner；重放先核当前授权，再返回原始 JSON 字节。仍保留的旧回执按原 typed/raw bytes 提升；已经过期删除或覆写的旧历史无法恢复，create/defaults 的既有 24 小时合同不因本阶段改变。
- Copy 冻结主图身份及目标映射，只有 media owner 注册实际独立目标图片后才绑定目标 FK；发布重新核绑定与媒体证明。实际 PostgreSQL/私有对象测试核验后续源清除不改变冻结副本，撤销审核/缺失绑定阻止发布、修复后在原 worker fence 继续，并在只移除合成源对象后证明目标原件独立。文档复制也覆盖 UTF-16 TXT/DOCX 独立原件、源对象移除后的目标读取。
- 冻结后端 63 文件完成 goimports、全树 vet、lint（0 issues）、Race（403 顶层通过、382 缺专用环境跳过、0 失败）、govulncheck（无可达/导入包漏洞，一个未调用 required module 提示）和生产构建。独立 workspace/Copy 61 通过、1 条 Temporal 因该进程未配置而跳过；另用真实 Temporal 单独 1 通过、0 跳过。CI 同范围 `^Test(Document|ProjectCover|ProjectCopy(Cover|Document))` 实际 36 顶层通过、0 跳过。首次 helper 错用 Cover 数据库名导致 14 个保护断言失败，修正 DSN 后通过，没有放松保护或断言。
- API/worker/relay 已使用同一冻结二进制 SHA256 `5284c0f378c070cc524449938e3031395d688f8d1671811c57b1ed76566f761c`；API readyz=200、正式 Router/在线 Swagger 为 84 个 operationId，Wire 和 Swag 重生逐字相同。84 条在线合同生成客户端经显式格式化；生成器自身格式警告未被算作格式通过。剧本临时生成合同与未完成实现不在本提交中。
- 真实浏览器源项目 `8d4c9375-9add-4e0c-abf1-ec57e41b7e71` 完成 A 上传保存、B 替换、明确移除、项目媒体选择、原始刷新恢复；B 资产 `c5f6bf08-8300-46e2-a961-97124f8d8199` 为 320×180/960 字节，SHA256 `572bf5b72cdc9b8c904c6128e6fef410836b9d50894b13e9cdc4dfb52df7f11d`。真正服务端已提交 HTTP200 后、Axios 回调前破坏响应，刷新未自动重发；人工原 key `f68b52cf-598a-4f48-8d63-eaf1e9cdfbbe` 核验返回原 revision 6，实际后续 revision 7 的描述保持。早期两次没有成功破坏响应的注入未算入该证据。
- 真实 409 保留脏名称/主图，明确获取最新后只合并未修改字段，再以新 key 保存；归档 revision 10 禁止修改，取消归档 revision 11 保留 B。390×844 页面宽度390、Dialog 宽358，图片选择关闭返回精确触发器，设置关闭回项目动作，Tab/Escape 和刷新均验证。
- 实际 Copy job `8059d40f-3604-4e61-ba04-5d7fb8238b16` 成功，目标 `8ce259fe-c2e7-483b-a17b-94bb20350f81` active/revision 2，独立主图 `bb304dce-40c6-5f0b-a189-a4e81068929f`。完整刷新后实际图片解码，正式私有读取和浏览器 WebCrypto 证明源/副本均为 B 的 960 字节/SHA；源 revision 11、主图和描述不变。

冻结前端 21 文件与 84 条生成合同在独立 HEAD+allowlist 快照完成全树 ESLint（零警告）、Prettier、Next typegen、tsc、87 文件/436 测试（零失败/跳过）和 Next.js 生产构建。文件 SHA、真实原件/并发/未知响应/复制证据与八张截图保留在独立验收快照；没有用共享树中未完成的剧本组件替代本阶段验收。

第十阶段提交 `7318beff4e088c85205f1a3f886dadc456ecd168` 已推送 main，CI `36948027647` 的 backend/frontend/images 全部完成且通过。

## 第十一阶段：历史文档冻结与复制取消清理（2026-10-02）

媒体 owner 新增 `FreezeWithReferences`，只扩展明确历史引用的同项目文档原件。已回收来源必须仍有完整 ready/passed 原件事实；普通读取、Reference 和文档导入仍拒绝回收来源。own manifest 保留原件删除时间、回收期限和原修订，复制为独立目标对象与 UUID，不改来源墓碑。缺失、重复、零 UUID、跨项目和非文档引用均拒绝；未传历史引用时旧 `29ce5664` 媒体 JSON 逐字保持。

非 owner 完整取消测试发现既有 `FinishCleanup` 缺三列授权，不能标记已完成对象清理的目标媒体。独立 `202610020054_media_project_copy_cleanup` 只新增 `is_delete/delete_time/purge_after` 的 UPDATE，保持既有 revision/SHA 权限，仍拒绝物理 DELETE、project_id 和 object_key 改写。实际取消验证源媒体全部元数据与私有原件不变，target 对象移除后才软删除目标事实；054 up/down 在回滚的隔离事务中验证，未放宽归属权限。

HEAD 加十个精确冻结文件在独立快照完成全树 goimports、vet、lint（0 issues）、Race（405 顶层通过、386 缺专用条件跳过、0 失败）、govulncheck（0 可达/导入包漏洞，一个未调用 required module 提示）和生产构建。Wire 经项目 goimports、84 条 Swag 重生均与 main 逐字相同；新增 CI 独立 `lanverse_library`，实际运行角色/私有对象的同范围八项 Race 全通过、0 跳过。历史文档在合成来源对象移除后仍可读取独立目标的相同原始 bytes；这项证据不替代整剧本历史、文件导入或页面验收。

054 已在前阶段同一业务 PostgreSQL OID `345144296`、postmaster `2026-10-01T23:01:22.384635+08:00` 上应用，SQL SHA256 `015a22da1c57cd4685f7161984827d1274e4f747e0de2764ee919fd9a81675fa`。变更前后八组项目、媒体、衍生物、画布、复制任务、Operation、永久生命周期命令及 Outbox 行数保持，0 未收敛复制/上传/Operation；没有重置数据或应用未完成的050/053/055。

该阶段实现已以 `61ec787d297dd798e6c070f712f0a43e7a2e6e9b` 推送 main，同一运行环境已更换为冻结构建 SHA256 `999125e0be5c310560952a75c0cb28cbc3b49b9f3aaa60c72fe91f9e5b8fe24e`；API readyz、worker/relay healthz 均 200，正式 Swagger 仍为84条。CI `36953731113` 的 frontend 成功，backend 在较早的普通媒体数据库检查失败：新增的历史引用拒绝测试先断言专用运行角色，未先检查专用环境，因而错误进入普通 owner 数据库。修复仅补入既有隔离 library fixture 入口；实际普通入口为1项条件跳过，专门运行角色/私有对象入口仍为八项 Race 通过、0跳过。此修复待新的 main CI 终态，不把此前失败记录为通过。

测试入口修复已以 `1757a9cf5418746bf1a91b57f34650ae383fbf88` 推送 main。其 CI `36955289763` 的 frontend 成功，但全后端 Race 捕获另一个既有 native monitor 竞态：超时长 decoded frame validator 取消进程时，正在执行的 RSS 观测因同一取消返回错误，覆盖了实际 `depth_budget_exceeded`，误报 `depth_runtime_unavailable`。修复只在 caller/validator 已取消或拥有进程已完成时停止此观测，保留原错误；真实 RSS 故障及超预算仍取消，物理进程组退出核验保持。隔离固定后端两项时长/取消检查重复20轮（40次通过），实际 Small/MPS 编码 callback 停止与1MiB采样内存预算各1项通过，均0跳过；全树 Race 405通过/386专用条件跳过/0失败，goimports、vet、lint（0 issues）、构建及govulncheck通过（0可达/导入包漏洞，1个未调用required module提示）。尚待本次修复的main CI终态，不把两次失败记录为通过。

该取消分类修复已以 `574aa3a1c9361b5a6188e8fc84091d497f6272f1` 推送 main，CI `36956765840` 的 backend、frontend、images 全部成功。API/worker/relay 已替换为同一冻结构建，各自 readyz/healthz 均200；同一已固定 PostgreSQL 实例、054权限与八组保留行数全部不变，0未收敛工作、没有DDL或数据重置。临时的剧本/素材库组合 Swagger 和生成客户端属于后续工作快照；当前业务API仍为84条，050/053/055/056未在业务库应用，不能把临时接口证据当作后续完整功能验收。

## 第十二阶段：正式剧本历史、文件导入与素材库 owning 接管（2026-10-02）

本阶段增加 Source8、恢复4、分集/结构12、历史版本4、文件导入6 共34个剧本接口，保留全部来源/版本/来源映射/分集确认/结构确认与私有对象历史。文件导入经真实已审核 TXT/DOCX 原件、实际抽取/规范化与持久逐文件结果，按冻结来源顺序合并；部分失败重试只处理失败文件，已成功来源身份保留。来源正文、结构、警告与编码均为 server-owning 事实；不会从浏览器提交 `file` 来源或伪造 warnings。取消和未知提交必须实际 join I/O；公开来源恢复拒绝内部导入发布，导入 worker 使用同一实际 I/O registry 与专属恢复 store。

整项目 Copy 接入 owning Script 的13项完整历史计数、独立原件/rich/规范文本对象及同 caller transaction 的 workspace authority。新增053只增加可空 script receipt 和真实 script 阶段，旧 nil JSON/哈希保持；目标最终发布前分别核 media、Script、canvas 与 workspace 内容。素材库 Copy 冻结全部目录、正文、元数据及回收状态；Library 声明的仍保有软删除原件由媒体 owner 核实并独立复制，缺原件/衍生物/审核事实则拒绝整个受理。Script 的显式历史引用仍限定 document，不放宽普通媒体读取。

素材库6个公开接口包含个人/项目列表与详情、永久目录/元数据命令、真实个人原件上传、签名预览和核完整 SHA 后的附件下载。055明确 personal/org/actor 归属，保留项目 FK 与不可改归属；056沿既有永久上传回执增加个人闭集。个人库不创建隐藏项目，当前身份/组织、项目 active/archived/copying/deleted、同键异输入、CAS、事务回滚和撤权均按所属模块判定。新的跨库 Transfer、物理清理、引用保护、容量及素材包尚在后续连续实施，不把这6接口算作完整资产库完成。

冻结组合验证：Source83 own 文件、前端85个Script文件与3个依赖文件（其中2个 workspace 文件追加权限缓存修复）、Media Core18/Personal14/Preview4/Copy6、根组合与在线生成物按逐文件 SHA 捕获。隔离 PG17.11@25432、私有 MinIO 与真实 Temporal namespace 下，Script70、Media37、Workspace8 顶层 Race 全通过、0跳过。两项实际 Temporal 检查证明原 run 接收永久控制信号，以及 PG Inbox→正式 Workflow→真实私有原件→来源版本的链。全后端 Race 为454通过、447缺专用条件跳过、0失败；跳过项不作为验收。goimports、vet、lint（0 issues）、Wire重生、主CLI构建均通过；govulncheck为0可达/导入包漏洞、1个未调用 required module 提示。

实际生产 Router 与 Swagger 均124个唯一 operationId，在线契约与静态生成物一致，124个生成函数逐项匹配。临时契约服务仅允许 GET Swagger，其余请求404，没有业务数据库或 DML；该证据不证明正式运行服务或页面完成。新增CI分别验证68项实际PG/对象Script（明确排除两项需要独立Temporal的检查）、26项库/个人上传/预览附件，以及8项workspace authority/复制协调，要求各专用步骤零跳过。

前端 Source/Import 以正式生成接口覆盖来源编辑、版权确认、原件选择/上传/下载、有序多文件导入、实际 job分页/阶段/部分失败、历史、分集与结构确认、首次采纳、原key未知恢复及明确冲突。审查发现旧 workspace cache 可能在 fresh principal 尚未确认前挂载正文；新增四项 Red→Green，挂载后只在当前 GET 成功且 scope 可证时显示私有子树，403/404或身份变化不沿旧body cache。最终冻结全树128套553项测试、ESLint、Prettier、Next typegen/tsc和Next生产构建通过。jsdom的媒体 pause/load 未实现提示不算真实媒体产品证据；冻结依赖独立离线安装后构建，未用共享 node_modules 链接的失败 standalone tracing 当作通过。

050 up SHA `31d5411ada51f63a4e849184bfd0b920fdffa5158756f8b722715f533cbbee19`、053 `866411304acd53e90bca675cf192b8d4951b517fe3f85d990291edb61df04b0a`、055 `a21521e00f0fb06c5b5acd29cf3de371673fcbf74c12dd3ecd17417faa33104f`、056 `05ef1ffc4e2ac3aec326ac363064160eca6c9b71abdbcf5266341df9a95e513c`。已在隔离测试数据库完成验证。2026-10-02 11:57，根协调者核对固定业务 PostgreSQL OID `345144296` 与原 postmaster 后，在同一事务中应用这四份逐SHA相同的DDL；31个Script表和4个Library表已安装，八组原业务行数及054清理权限逐项不变，0未收敛任务，没有数据重置。随后用 main `52478934bacc4cebb24b44e6c7343088fecf40c7` 的冻结构建替换三个角色，API readyz、worker/relay healthz 均200，正式Swagger为124个唯一operationId。API首次启动因未恢复原loopback/browser-origin覆盖而退出，补回 `127.0.0.1:8080`/`http://localhost:3000` 后健康；没有修改产品安全限制。新建实际本地Kafka主题 `lanverse.script.import_command.v1`，其余服务继续复用。Script浏览器与完整历史Copy端到端仍在执行，角色/转移/包仍未完成。

首次采纳可用，替换已采纳版本/已确认结构的下游失效证据、非空角色绑定及含正式角色历史的Copy仍明确缺少Bible/storyboard/audio/lineage owning 实现而503。角色/主体完整版本与声音已转入 DES17§12 连续实施，素材库新页面及Transfer/包仍在推进。此阶段不关闭完整迁移，也不将手工结构、条件通过或文件入库宣称为AI抽取/真实供应商生成通过。

main `52478934` 的 CI `36961645064` 已终态成功：backend、frontend、images 均通过。浏览器验收期间共享开发树的未完成资产页引发全局构建遮罩，根协调者将自己的3000开发服务替换为同 main 的冻结 Next 生产构建，仍使用原本同源 Go rewrite 与loopback监听，避免并行WIP干扰验收；不改Library owner正在编写的页面。

同 main 的真实 Chrome 页面创建合成项目 `eda7f3f1-911d-4154-8d95-aa007c3a2aa3`，来源“夜航者 · 完整来源😀”稳定 lineage `80454e38-651b-541a-8d79-75a3eec1a7af`。完成三次真实保存及刷新恢复，不可变正文分别97/115/99 Unicode scalar；第3版实际9段、1空段、8段粗体，历史对话框仍能逐字读取第1版97 scalar原文（正文SHA `81d08c6e809fbdfa2268f47d9f32b626bdaffc5bc2c9b541d50f5d27a2a364eb`，格式SHA `32d5e24c5940f02b9f87720a491588a8dc275d4fc7bfdad1c09dbb50d4ee162c`）。富文本规范段落分隔为双换行，因此初始输入的单换行长度不能冒充已保存规范正文长度。第2版自动化光标定位未按预期移动，新增尾声实际位于开头，仍作为真实历史编辑保留；未保存的后来草稿不算版本证据。

仅对已完整审阅的合成 TXT 原件执行真实本地选择、版权确认与上传，`script-unicode-original.txt` 为169 bytes，SHA `2482da2c8802ff0ae117dbbf9833b7eb0d68a04da2f20fc4fd6136cff654215d`；页面显示已确认原件且未直接添加章节。浏览器扩展因 file URL 权限关闭而拒绝程序文件选择，首次合成TXT通过原生选择器完成；后续用户前台操作与文档选择交错，未知私人文档保留，不读取、导入或计入合成验收。完整多文件导入、失败重试、正式分集/结构确认和整项目历史 Copy 的浏览器验收继续执行，此段不表示Script完整产品验收通过。证据截图 `/tmp/lanverse-script-history-original-20261002.jpg`、fixture清单 `/tmp/lanverse-script-fixture-20261002/original-files.json`，均为本机临时证据，不提交私有对象或原件。

后续仅选中上述169 byte合成原件，真实导入任务 `72380340-de85-551c-ac66-606bdcea63c2` completed/revision7，1/1已发布、已提取1、失败0，发布不可变剧本 `d0b0b0c3-bf03-50f5-ac42-0b3967fc166a`（第4版）与来源 `c4c626b3-6316-5b15-af26-b00741a96308`。导入正文64 Unicode scalar，CRLF按规范换行、空段落、组合字符与双空格保留；原先99字符/粗体来源仍在。整版165字符/SHA `6a15ae8660eb9a07436fc53ca4f4b7e02b2ecec4f30246249c403144c517b9d4` 全文经页面实际阅读后确认两个来源边界分集 `[0,101)`/`[101,165)`，正式集合 `606322d1-0b24-5029-8507-dba27d4f46f2`、第1集身份 `bd1d40d4-6396-548c-a907-c710876b679e`。分集确认推进可变脚本修订至5，没有新增不可变正文第5版；发现前端把CAS revision标为“脚本版本”，已在后续候选修正为“脚本修订”。此时结构确认/采纳尚未验收，不能据此关闭剧本完整验收。导入页面证据 `/tmp/lanverse-script-txt-import-20261002.jpg`。

同一合成项目随后经正式浏览器保存手工结构版本 `e7ec0991-632a-50c5-a10d-2a91cc5120b4`、明确阅读并人工确认，正式集修订3，当前候选与已确认结构分别保留。场景稳定键 `3bcd70a3-3124-4c2e-b1ae-7b437a437db3`、完整范围 `[0,101)`，台词“林舟：最后一班车还没来。😀 é”保留 `[23,39)`，行动“雨水顺着站牌滴落。”保留 `[41,50)`，稳定行键、说话者与情绪均经页面核对。随后首次采纳第4版正文成功，已采纳版本显示 `d0b0b0c3-bf03-50f5-ac42-0b3967fc166a`，可变脚本修订8；第二集尚无已确认结构，不把首次采纳等同于全剧解析、AI抽取或替换已有采纳的下游失效验收。截图 `/tmp/lanverse-script-structure-confirmed-20261002.jpg`。

Library29 focus 修正以 manifest `f7f1e1177b6f39d3913d0c46214dcb5d8dd81bcc2951163e0ab7ce1c37010ca1` 重新捕获，与4项 Script CAS 文案构成冻结33文件候选。全树139文件/594项测试/0跳过、ESLint、Prettier、Next typegen/tsc及生产构建全部通过。仅替换根拥有的3000冻结前端，保留8080正式124条API与原同源rewrite；未将角色、Transfer或其他并行WIP暴露为已完成。Library已进入真实合成目录/文字/元数据、刷新、分页、回收恢复的浏览器验收，完整素材库能力继续未关闭。


### 素材库正式页面与角色、迁移候选接续（2026-10-02）

素材库33文件实现已以 main `4b47c00507304ad89b7b683b7fd227af501bb11c` 推送，CI `36970025929` 终态成功。正式3000使用该提交的独立生产构建，8080仍为124条API。真实合成验收完成个人目录创建/重命名、原始Unicode文字及空白保留、元数据、根目录移动、回收后刷新/恢复；项目目录8层与拒绝第9层、6种style/4种theme、41条实际文字分页20/20/1、跨页筛选/选择/批量移动/目录往返、390视口与键盘关闭后的按钮焦点恢复。证明为 `/tmp/lanverse-library-fixture-20261002/browser-formal-core-proof.json`；未执行永久删除最终按钮，未把binary上传与播放算作本闭环通过。

下一冻结合同为151条唯一operationId，新增Bible21与Transfer6；Bible保存角色/场景/道具的完整定义、不可变历史/确认/造型/六图引用/声音绑定，并为完整Copy保存全部身份、历史pin及15项真实计数。新增061仅扩展Bible回执与阶段，062用可空 execution_actor_id区分永久创建者和当前授权执行者；授权撤销后同步执行返回的准确worker停止无需再次读取私有正文，管理员只能接管取消和保留取消意图的对账，普通重试/发布不能借管理员身份。Bible cleanup使用调用方SQL事务，旧nil-Bible回执与历史JSON不改写。

后端冻结快照通过全树Race（479顶层通过、488缺专用条件跳过、0失败）、vet、lint零问题和govulncheck零可达/导入包漏洞（仍有一个未调用required module提示）；专门真实非owner PostgreSQL/MinIO/Temporal的Bible30、Media34、Voice6共70顶层通过且零跳过，实际公共router/audit/domain9项通过。另冻结Transfer生命周期2项真实非owner检查：两方向进行中、执行未确认和取消未停止均拒绝项目归档/删除且无永久成功回执，真实终态才释放。Root正式DI已安装Transfer owning work guard。当前这些证据分别证明隔离实际owner行为、生产组合与静态合同；尚不替代正式业务浏览器验证。

角色前端最终27文件（包含redirect严格校验与精确保留历史文案）已加入候选；冻结完整前端148个实际测试文件/644项测试/0跳过、全树ESLint/Prettier、Next typegen/tsc和生产构建通过。生成客户端从实际Go BusinessRouter在线Swagger生成，151个函数和14个文件经显式格式化后重复生成逐字相同；Wire规范化重生也与现main逐字相同。Transfer完整页面候选正在合入，不以没有页面入口的API宣称交付。主业务库只读预检已核对同一OID/postmaster、74组既有owner行数和零未收敛工作；057/060/061/062尚待精确事务应用。058永久清理、引用保护聚合、容量/内容包、Bible回收发现、Impacts/GeneratedResults与下游完整业务仍继续，完整迁移未完成。


151整包最终前端候选纳入Transfer16及新增恢复焦点测试。未知Transfer恢复与 asset_id URL详情、未知Bible恢复与 version_id历史均先确切Red复现双Dialog，再验证scopefresh后单一恢复Dialog、保留原UUID/正文/URL、解决后详情和焦点恢复；没有自动POST。最终155个实际测试文件/670项测试/0失败/0跳过，全树ESLint/Prettier、typegen、tsc及同生产源码Next构建通过。生产构建后仅新增focus回归测试，重新运行适用静态门禁和全树Vitest，不把早期外部node_modules symlink构建失败算作通过。Transfer实际源/目标任务投递、未知物理写入、正式六图/音频与完整历史Copy浏览器仍待新运行环境验证。


### 当前 main 交付与空造型范围读取修复（2026-10-02）

151接口整包已以 `4e9464ff92de142426c312a910a5f45787ab92a8` 推送 main，CI `36977343871` 的 backend、frontend、images 已终态成功。业务库057/060/061/062已按冻结SQL在同一事务中安装；固定OID/postmaster、74组原owner行数和权限保留，没有数据重置。冻结API/worker/relay健康检查均200，正式接口151条；3000前端已使用同候选生产构建。这是当时运行升级的已记录证据，不替代全部页面与供应商验收。

正式浏览器已完成文字素材个人→项目→个人双向迁移、源修订不变、跨页选择、实际取消、双标签CAS冲突、刷新恢复和390视口/键盘焦点检查。合成PNG完成正式上传、双向独立目标、实际解码及收到的下载字节核对（91294 bytes，SHA `8dc52256959a0f1ef6ad334789283b03e2d96065688bf471e3074224dba04383`）。独立私有object key与原件/缩略图的owner核验，以及操作系统最终落盘文件仍未证明；不能由浏览器收到字节推断这两项通过。临时证据为 `/tmp/lanverse-transfer-browser-proof-20261002.json` 和 `/tmp/lanverse-transfer-png-browser-proof-20261002.json`。

角色页面真实新增第二造型时，显式 `applies_to: []` 在不可变规范正文中因omitempty被省略，但逐行历史仍保留 `[]`；读取器将其与解码后的nil进行字面比较，错误返回503。修复只比较空集合语义，非空差异仍拒绝为损坏历史，不更新正文、SHA、逐行内容或任何既有版本。新增真实非owner PostgreSQL/HTTP回归先复现503，再证明列表、当前详情和新旧历史均200，读取前后原历史字节与SHA保持一致。

冻结main加两文件的完整Bible Race检查：31顶层通过、0失败、0跳过，使用既有隔离PG17与真实私有MinIO。首轮12项文件相关失败来自19000测试MinIO停止；恢复同一独立测试服务后全套重跑通过，失败记录未被覆盖。适用goimports、全树vet、golangci-lint（0 issues）和主CLI构建通过；govulncheck为0可达/导入包漏洞、1个未调用required module提示。只读代码审查确认非空历史保护保持。

本次提交仅包含上述读取修复、回归测试和本记录。工作区中的引用保护聚合、永久清理、容量、GIF/glTF、素材包及剧本角色选择器等后续实现尚未完成组合验证，保留在工作区且不作为本次交付；完整迁移仍未完成。

### GIF、原生 JSON glTF 与剧本角色选择器组合（2026-10-02）

后续组合以 `c5a6465a` 为基线，仅叠加冻结 GIF/glTF 后端12文件与前端40文件。GIF保留完整动画原件，真实首帧PNG用于静态列表；详情、上传预览和画布保留动画。全部帧解码、晚帧损坏、尾部数据和帧数/像素预算已覆盖，不将扩展名改为PNG冒充原件。JSON glTF采用 `model/gltf+json`、`gltf2` 和 `.gltf`，与既有GLB分开保留原件SHA及格式；只有自包含缓冲与图片正式准入，单文件的相对/远程资源返回明确错误。063仅放宽正式模型事实约束至上述闭集，并保留旧GLB约束。

独立准确候选 `lanverse-gltf-final-head-20261002-tcfylsex` 的真实非owner PostgreSQL/MinIO Race为11个顶层全部通过、0失败、0跳过，涵盖GIF全部帧与独立原件/缩略图复制、JSON glTF正式HTTP上传/下载/原件SHA/独立Copy，以及旧GLB回归。完整后端vet、lint零问题；Go源码冻结按manifest核对。CI增加同样11项真实格式门禁，要求零跳过，不以普通测试缺外部条件跳过作为通过。

前端原生GLTFLoader实际解析自包含JSON glTF与GLB三角几何，外部资源在loader前拒绝；模型异常、取消、授权刷新、原材质与替代材质的销毁均有回归。上传回执严格核对GIF/glTF/GLB的正式MIME和精确byteSize，修复GIF库上传回执未纳入expected image的遗漏。剧本结构角色选择器使用明确确认的不可变角色版本名称/别名；选择时并行复核当前实体与工作区，保留已有历史pin，取消绑定同时清除两项身份与pin，过期scope和晚到请求不覆盖当前选择。

冻结40文件组合的全树ESLint、Prettier、Next typegen/tsc与161文件/738项测试全部通过、零跳过。首次生产构建因临时目录外部node_modules软链接被Turbopack拒绝而失败；复制同一已安装依赖到独立候选后，默认Turbopack生产构建通过，没有修改工程配置、依赖或锁文件。此前各切片单独通过不代替本次组合结果。

本段记录代码与隔离实际合同验证。063尚未在业务库安装，当前正式8080/3000仍运行前一151接口构建；GIF/glTF新上传与角色选择器的正式浏览器验收继续，不能从上述检查推断已部署或全部页面完成。永久清理、引用保护、容量与素材包仍在连续实施中，未纳入本次格式/角色组合提交；完整迁移继续未完成。

该54文件组合已按测试、实现、CI及文档分别提交，最终 main 为 `c91d61eeedec2d19114cbbc6be14d80906c7dc2c`，已推送 origin/main。CI `36994899628` 的 backend、frontend、images 全部终态成功。工作区后续片保留，未把未完成接线混入此提交。

### 设定集历史与完整复制清单的严格读取（2026-10-02）

后续六文件冻结组合补齐 materialized result_source、造型 applies_to、声音绑定、完整复制清单和永久命令回执的严格JSON入口。实际非owner PG先证明未知字段被旧reader接受，再验证未知字段、重复键、尾随数据与非法UTF8拒绝；不更新历史正文、SHA或回执。单版本继续保留4MiB、64层和200000节点上限，完整Source+Target清单沿原大小合同使用同一严格解析器，不把单版本预算错误施加到完整历史。真实4742982字节合法清单完成Transfer/Register/Verify，原字节和SHA保持。

独立 `c91d61ee` 加六文件候选的新增与正常回归合计32个实际PG/MinIO顶层Race测试通过、0失败、0跳过。各片全树Race与Go质量门禁通过，其中durable四文件候选为486通过、497缺专用外部条件跳过；跳过不计实际验收。CI为materialized入口增加独立reference数据库，普通Bible真实门禁继续包含全部新增用例并要求零跳过。该修补不新增普通reader历史数量/字节限制，也不改变完整迁移尚未完成的状态。


### 素材包、物理清理、容量与完整历史保护的同栈组合（2026-10-02）

本批沿既有 Go/PostgreSQL/Temporal 与 Next/React/TypeScript 工具链接管完整素材 ZIP 导入/导出、永久清理和容量。Package 接入全部六条生产接口，真实探测/渲染、私有原件与衍生物写入/回读、目录与正文原子发布、永久回执、原键未知恢复和取消已闭环；完整源 ZIP 单独保留 provenance，不能把源 URL、旧身份或导出声明视为目标审核事实。个人与项目容量同时装配 Transfer、Package、完整 Copy 和 mediatool 的真实 owning 对象事实，按实际物理对象去重计量，不提供未定义的配额。

永久清理沿正式持久作业、独立 Temporal 执行与媒体原件/衍生物 owner 接线；当前与历史引用、活动/未知任务、不可读 owner 和撤权都阻断受理。前端提供逐项真实状态与回收站全部分页计划，保留原键和完整 CAS，后台/卸载/未知结果停止，刷新须明确核查与继续。计划及原意图合计 UTF8 JSON 限制1MiB，超过预算拒绝整个计划并保留原值。恢复计划期间关闭、Escape和外部点击明确锁定，须先核实原作业并结束原计划。上述页面测试使用受控 API，并不表示已点击业务永久删除。

组合审查先实际复现 Package 并发重放/受理的 PostgreSQL 40P01，随后统一 command→project 锁序，永久回执和当前权限保护保留。Workspace 历史保护原本先载入全部 JSON 再拒绝超限；两历史 family 各3次实际 allocation 与空基线证明2MiB单件、64MiB以上总量和100001行会在拒绝前扩大分配。修复保留原每 family 100000行、合计64MiB、单件1MiB合同，采用 SQL 预算预检、单件 CASE 上限和逐行读取；18次实际拒绝分配约0.44–0.85MiB，不改普通页面读取或完整 Copy manifest 合同。该证据是 owning PostgreSQL/客户端分配证明，不是媒体原件证明。

最终 backend V6 准确109文件候选：全部 Package 19项和所有历史引用32项实际 Race 通过，包含真实非owner PostgreSQL、Package 私有 MinIO、锁序与完整历史预算，所有顶层/子用例0失败、0跳过。整树 Race 为507顶层通过、572顶层缺外部条件跳过、0失败，另有134个子用例条件跳过；条件跳过不算实际验收。前一同源组合另完成普通 Bible37、Purge/Usage28、真实格式11、Temporal实际清理、生产路由5和独立对象 inventory4的零跳过验证；V6 只改变上述两个 owning reader 的锁序/预算及新增策略测试，整树及受影响家族另行重跑；全新58份正式up迁移的独立生产路由库中5项实际组合也重跑通过，0失败、0跳过。gofmt/goimports、vet、golangci-lint 0 issues、govulncheck 0可达/导入包漏洞和主 CLI 构建通过；1个未调用 required-module 提示不计为可达漏洞。

同 V6 源码重新生成正式163个唯一 operationId；Swagger 生成与 Wire+项目 goimports 格式化后逐字确定，Wire没有新增差异。在线 API 的空 host/schemes 仅按已有 Swagger 模板默认规范化，paths/definitions 全部相同。客户端完全由该实际在线合同生成，AST163个导出无遗漏/语法错误，第二次生成与显式格式化后逐字相同；未手写自动客户端。058/059只在独立测试库验证，业务8080/3000仍为前一151接口运行构建，业务库未安装058/059/063。本批提交和技术门禁不等于已部署，完整素材包页面、实际163页面/对象验收及其余源能力仍在迁移中，完整迁移没有关闭。


前端最终组合相对 `c91d61ee` 准确24项改动（22源文件与2项实际有差异的自动生成物），全部14项生成客户端与实际163接口一致，依赖/锁文件没有改动。169文件、775项测试0失败/0跳过，ESLint、Prettier、Next typegen/tsc、默认Turbopack生产构建全部通过。恢复审阅另复现已结束的 running/cancel_requested 作业在终态提交失败后 `needs_reconciliation=false` 导致对账按钮锁死；最小两文件修补仅开放沿原作业/修订/键的明确人工对账，由服务端核验 processEnded 与当前权限，execution_unconfirmed 仍禁用，409保留原键正文，刷新不自动请求。模型预览授权切换的原材质/texture泄漏已按单一恢复/销毁 owner 修复，真实 React/Query/Three 受控授权回归通过；这些组件测试不代替正式WebGL、对象或永久清理浏览器验收。

CI拆分强制 reference 库、普通Bible37/materialized5、Package15/个人guard2/锁序2、生产组合5与真实Temporal lane；普通Script68及Bible/Voice/Transfer汇总前增加全事件校验，顶层或子用例跳过、包失败和未收敛测试都会拒绝。YAML、72个shell语法块、actionlint与子Skip失败回归通过；ShellCheck未执行，远端Linux CI结果由推送后的实际终态单独确认。完整迁移和运行环境产品验收仍未完成，不以此次代码提交关闭。
