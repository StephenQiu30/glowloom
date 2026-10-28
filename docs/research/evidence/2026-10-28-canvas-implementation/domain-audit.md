# 画布领域与高级能力静态核查

## 1. 核查边界与结论

核查日期：2026-10-28。代码基线：`b0e65819`，共享工作区存在并行文档修改。本文只核查当前源码，不修改源码、Figma、分支或提交；未启动服务，未调用收费服务，未读取凭据或 `.env`。静态存在不等于可运行，更不等于即梦一致性验收通过。

依据：[研究10](../../10-即梦无限画布需求基线.md)、[设计14](../../../design/14-画布数据与接口设计.md)、[设计17](../../../design/17-画布Yjs协作编辑设计.md)、[设计25](../../../design/25-统一主体库与创作引用设计.md)、[设计27](../../../design/27-无限画布一致性改造设计.md)、[Plan23](../../../plan/23-无限画布一致性改造实施计划.md)。本报告记录证据与差异，不替代 BACKLOG 中的实施状态。

主要结论：

- 主体领域已经具备统一库、固定版本、主辅媒体及音色绑定和授权链；缺的是画布主体消费节点及批量选择确认，不应再建主体库。
- 时间线已有后端轨道/片段模型、保存校验、授权版本冻结和实际 MP4/M4A 导出代码；缺的是当前画布编辑器接入及工程交换格式。应复用 `canvas` 与 `mediatool`，不重建时间线服务。
- 导演已有场景、角色、相机、灯光、镜头及图片输出身份模型；当前前端没有查到可复用的导演渲染编辑器。`three` 依赖、CSS 和后端配置不能当作已经交付的 3D 引擎。
- AI 侧栏保留了前端会话类型，但入口禁用；查到的聊天执行属于管理员开发试验，不能直接开放给项目成员作为画布 AI。
- 高级节点接入不止是添加按钮：当前前端保存转换明确拦截 timeline/director/subject，且新节点固定写为 resource。必须补节点创建、配置投影与恢复合同。

## 2. JC-046～047：统一主体消费

| 需求                                | 当前代码事实及责任位置                                                                                                                                                                                                                             | 差异与接入建议                                                                                                                                                                         |
| ----------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| JC-046 主媒体、辅助媒体、音色及说明 | `frontend/src/components/subjects/unified-reference.ts:69` 的 `unifiedSubjectBinding` 固定主体 ID、version_id，展开 primary、references、voice.media，携带 asset_revision 和 binding_id。`:28` 校验模型声明的真实媒体角色。                        | 复用绑定转换；节点只保存主体来源与固定版本，不复制主体库记录，不以图片 URL 代替身份。主体说明的编辑究竟修改当前节点说明还是创建统一主体新版本，需在设计25/27明确，不能静默改库中主体。 |
| JC-046 画布入口及节点               | `frontend/src/components/canvas/controls/canvas-toolbar.tsx:251` 主体入口 disabled；`types/canvas.ts:13` 内置枚举无主体；`constants/canvas-input-ports.ts:2` 内容类型和端口来源无主体。后端 `canvas/domain/graph.go:31` 合法节点类型也无 subject。 | 需要正式主体消费节点协议及迁移/校验，不能仅注册开放字符串。优先复用 composer 的 subject source/binding 语义；新节点类型/字段由画布域定义并生成 API，主体域继续拥有来源解析。           |
| JC-047 搜索、筛选、收藏             | `frontend/src/components/subjects/unified-library.tsx:60` 已有 actorID/scopeID/query/onSelect；`:71` 有 filter；`:109` 请求键包含身份、空间、查询及分页；`:5` 使用 listSubjects/batchSubjects/exportSubjects。                                     | 复用查询与卡片呈现，保持当前空间权限和错误态。禁止画布自己发跨空间未授权查询。                                                                                                         |
| JC-047 多选数量与确认               | 同文件 `:69` 的 onSelect 接收单个主体，`:413` 点击卡片直接回调。`:84` 的 batch 模式和 `:290` 的数量是主体库批量管理，不等于导入多选确认。                                                                                                          | 增加薄选择适配层：零/一/多选、确认/取消、选择快照、查询切换与失权处理。一次确认在一个可撤销画布事务内创建全部节点；选择器内部草稿不提前落盘。                                          |

完整授权链已经有具体连接点：

- `backend/internal/app/public_api.go:160` 将 `subjectwire.NewPromptSources` 注入画布 Store；`:284`、`:302` 注入提示词草稿。
- `backend/internal/canvas/adapter/postgres/yjs_projection_store.go:64` 对 composer/draft composer 调用 `ValidateDraftSources`，使用事务内 subjectSources；因此不能只做客户端资格判断。
- `backend/internal/operation/adapter/postgres/create_free_quote.go:429` 通过 `ResolveQuote` 解析主体绑定；`workflow_consent.go:74` 对 subject_media 在执行确认时再校验；`subject_media.go:16` 在 worker 获取输入时校验不可变输入并签发有限期引用。
- `backend/internal/operation/adapter/postgres/project_media.go:43` 普通输入明确排除 subject_media，继续要求资产归属当前项目。接入主体不应放宽这条普通媒体规则。

待运行验证：跨项目同空间引用、跨空间拒绝、删除/撤权后的保存/报价/确认/worker 拒绝、主辅图片+音色混合输入、模型切换保留不兼容绑定、同一主体重复引用与多选撤销、双客户端回读固定版本。上述场景尚未在本核查执行。

## 3. JC-048～049：时间线编辑与素材进入

现有责任链：

| 层       | 可复用代码                                                    | 已有边界                                                                                                               |
| -------- | ------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------- |
| 文档     | `backend/internal/canvas/domain/document.go:25`               | NodeConfig 已有 Timeline；不需第二文档存储。                                                                           |
| 时间模型 | `backend/internal/canvas/domain/timeline.go:14`、`:25`、`:35` | 配置、轨道、片段均有稳定身份；片段使用 start_ms/duration_ms/source_start_ms，包含音量、淡入淡出、裁剪和文本。          |
| 校验     | 同文件 `:81`、`:96`、`:114`                                   | v1、三种比例、24/25/30/60fps、最多32轨/1000片段；最短100ms；同轨不重叠，引用媒体类型一致。                             |
| 来源关系 | 同文件 `:142`、`:155`                                         | 同画布节点引用必须匹配正式资产；删除来源节点时可保留 AssetID，不应把可用素材一并丢弃。                                 |
| 授权保存 | `backend/internal/canvas/adapter/postgres/store.go:449`       | 每个素材引用调用媒体域校验；不能靠前端拖入 URL 判定有效。                                                              |
| 导出快照 | `backend/internal/canvas/adapter/postgres/timeline.go:20`     | 在事务内校验项目写权限、画布可写、精确 revision、timeline/tool 类型；同画布节点解析为正式 AssetID 后再校验媒体和裁剪。 |

当前 `frontend/src/components/canvas` 未查到时间线编辑组件；仅 `snapshot-viewer.tsx:29` 有类型名称。现有 `service-graph.ts:35` 可以保留服务端节点原始数据，但新节点路径 `:327` 只接受六类基础类型，并在 `:344` 写死 resource；`lib/canvas-document-adapter.ts:110` 也拒绝高级类型。`backend/internal/canvas/application/yjs_projection.go:116` 已支持 timeline 的配置差异，所以主要接入点是前端编辑/投影和创建合同，而不是后端新增存储服务。

建议实现约束：

1. 时间线节点创建使用既有 `node_type=timeline / node_action=tool / config.timeline`；编辑状态使用既有轨道片段 DTO，临时播放游标、拖拽预览和全屏状态不写成业务配置。
2. 本地素材走既有上传→正式媒体处理→就绪资产；资产库选择和画布节点选择最终统一为获授权 AssetID/NodeID。没有正式资产的上传占位不能直接启动导出。
3. 分割须同时计算新片段 duration_ms 和 source_start_ms，保留源裁剪、音量及来源；排序/拖动须符合“不重叠”的现有业务规则。若采用即梦不同规则，先补竞品非空证据并修改设计，不绕过后端校验。
4. UI 的 visual/audio 可作为分组视图；现有轨道 kind 是 video/audio/image/text/subtitle，不能直接把 visual 写进后端。图片与视频同视觉轨是否需要协议扩展，是明确设计问题。
5. 把一次分割、删除、排序作为一组 Yjs 编辑；两个用户同时修改整个 timeline 配置存在原子配置冲突风险，需要保存拒绝/恢复反馈，不能以本地播放成功替代协作接受。

待运行验证：非空时间线导入三来源、音画时长、分割源时间偏移、静音、同轨碰撞、Shift 排序、全屏/尺寸持久化边界、撤销重做、刷新回读、并发修改恢复。竞品目前只证明空态和入口，不能宣称非空行为已与即梦一致。

## 4. JC-050：导出复用与格式缺口

`backend/internal/app/public_api.go:213`～`:219` 注册实际媒体导出服务；`app/media/media_export_wiring.go:24` 在导出事务内组合 TimelineReader、媒体访问和项目授权。`mediatool/domain/export.go:18` 明确只有 video/audio 两类输出；`:36` 定义 queued/running/review_required/succeeded/failed/cancel_requested/cancelled。

| 能力                                    | 当前事实                                                                                                                                        | 产品接入要求                                                                                                      |
| --------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------- |
| MP4                                     | `backend/internal/mediatool/adapter/ffmpeg/render.go:129` 产出 MP4；`application/worker.go:88` 发布视频资产。                                   | 已有真实编码代码，应接现有创建/查询/取消/重试/预览/审核/下载 API。尚未播放验证，不能记为验收通过。                |
| M4A                                     | `adapter/ffmpeg/audio.go:91` 产出 M4A；`application/worker.go:90` 发布音频资产。                                                                | 可复用既有音频导出，但它不替代竞品工程交换格式。                                                                  |
| SRT                                     | `application/preview.go:97` 从冻结可见字幕/文字片段生成确定性 SRT；`adapter/http/handler.go:389` 暴露字幕下载。                                 | 复用字幕输出，不把 SRT 标记为 XML/剪映。                                                                          |
| 批量素材                                | 项目中有资产 ZIP 消费接口，例如 `frontend/src/components/canvas/editor/canvas-side-panel.tsx:338`；未证明其输入、清单及顺序等同时间线素材导出。 | 需核对媒体域 ZIP 合同后组合，保留片段→来源映射和权限，不能简单把“下载库中全部素材”当作完成。                      |
| 剪映、XML、DaVinci、Premiere、Final Cut | 本次限定搜索 `backend/internal/mediatool` 的导出实现未发现这些序列化器；当前 OutputKind 仅 video/audio。                                        | 明确工程格式、版本、媒体引用/打包方式、时间基和目标软件导入验证；每一格式独立交付。不能提供改扩展名的假工程文件。 |

生成 API 已在 `frontend/src/api/mediaExports.ts`：get `:7`、cancel `:23`、download `:42`、preview `:58`、retry `:74`、review `:93`、subtitles `:112`、list `:128`、create `:144`。这些是生成文件，只消费接口，不手改。创建带画布、节点、已保存 revision；不能用当前尚未接受的编辑状态当成已冻结输入。

审核是当前导出领域的真实阶段：不能把 review_required 直接显示下载成功，或自动伪造用户已看过实际文件。具体 UI 是否保留独立审核步骤应遵循已接受的导出设计。

待运行验证：真实 MP4/M4A 播放、时长帧率/音轨、SRT 时间码、审核及下载、取消和重试幂等、来源版本变更拒绝、对象存储路由、导出失败反馈；所有工程格式须由目标解析器或目标软件实测。

## 5. JC-051：导演台

已有内容相当于场景文档与安全保存合同，尚不能等同完整导演工具：

- `backend/internal/canvas/domain/director.go:13` 包含场景、地面、环境、对象、相机、灯光、镜头、活动镜头、全景及封面；`:226` 校验对象/相机/灯光/镜头数量和参数范围；`:334` 校验来源节点与媒体类型。
- `director_outputs.go:17`、`:25` 用正式 AssetID 保存截图/封面；`:60` 校验输出身份和镜头归属。CreatedAt 是展示元数据，文件注释明确不作为执行证据。
- `adapter/postgres/store.go:471` 对封面、镜头截图、全景、对象模型和 billboard 图片重新授权；`domain/director.go:351` 支持删除来源节点后保留资产引用。
- `frontend/package.json:46` 有 three 依赖，但本次在 `frontend/src` 的 TS/TSX 中搜索 three 引用未匹配；canvas 子树只有导演 CSS 和 snapshot-viewer 名称，没有找到真正导演编辑器。工具栏 `canvas-toolbar.tsx:253` 为禁用入口。

建议沿现有 DirectorConfig 建立画布进入→编辑场景→保存→返回原节点的真实闭环。截图先经正式媒体上传/发布，再绑定截图/封面；禁止把 canvas.toDataURL、任意对象 URL 或本地时间当作已保存结果。模型文件、全景图和角色采用现有媒体授权，不建立第二资产库。

需要先收敛：即梦导演台内部功能证据、是否存在当前工作区以外已接受的渲染器、最小场景编辑范围、截图与视频结果边界。若需全新三维编辑引擎，设计27明确要求单独范围决策；不能靠后端 DTO 和依赖清单宣称只差一层 UI。

待运行验证：实际非空场景、相机/镜头编辑、视图与截图一致性、正式图片回写、取消/返回、非法模型/失权、并发场景修改、切换项目后迟到截图不会串写。竞品研究仅证明入口描述，内部一致性目前无证据。

## 6. JC-058：AI 对话

`frontend/src/components/canvas/types/canvas.ts:123`～`:148` 保留 AssistantReference/Message/Session 类型；`editor/canvas-editor.tsx:466` 保留 chatSessions 状态及恢复引用。仅有类型和本地状态不能证明正式服务或持久化会话可用。当前 `controls/canvas-toolbar.tsx:307`～`:320` AI 对话按钮禁用。

找到的真实聊天执行是 `backend/internal/operation/adapter/codex/chat.go` 与 `operation/adapter/http/trial.go`。后者 `:46` 将路由注册在 `/admin/ai-trials`，`:51` 要求 platform_admin 和 AuthorizeDevelopmentTrial；`:178` 的续聊以原私有试验会话、幂等 request ID 和工作流执行。它是管理员试验合同，不能拿掉鉴权后冒充画布助手。

复用建议：

1. UI 可复用统一提示词文档、主体选择/引用、现有模型能力与任务查询；普通生成输出仍走正式 operation/媒体采用。
2. 画布对话需明确会话归属（个人/项目）、消息及参考快照、供应商输入、取消、错误/未知状态和上下文限制；不能仅把数组持久化到前端当作聊天服务。
3. 对画布的 AI 改动先作为带来源对象/版本的建议或任务结果，再通过既有 Yjs 及业务校验落盘。执行开始后切换画布、节点删除、权限撤回和迟到返回都不得写入当前碰巧打开的项目。
4. `/技能` 只能呈现真实注册并有输入输出合同的能力；`@主体` 使用固定 subject binding，不用 AssistantReference 的 dataUrl/storageKey 充当授权依据。

待运行验证：真实消息往返、续聊/会话切换、参考快照、取消/未知、结果采用、权限撤回、跨项目防串写。没有产品级会话与执行合同前，不能将侧栏视觉稿标成完整交付。

## 7. 引用、任务、保存与协作统一约束

| 合同           | 已有责任代码                                                                                                                                                            | 本次接入必须保持                                                                                 |
| -------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------ |
| 引用资格       | `constants/canvas-input-ports.ts:4` 端口定义；`subjects/unified-reference.ts:28` 主体模型提示；`canvas/adapter/postgres/yjs_projection_store.go:61` 服务端媒体/主体复核 | 草稿连接可表示候选，真正执行必须检查全部固定版本和模型角色；不把空主体或上传中媒体当成有效输入。 |
| 报价冻结及确认 | `operation/adapter/postgres/create_quote_snapshot.go:35` 事务保存报价及全部输入快照；`confirm_single_quote.go:30` 经 confirmation key 确认                              | 修改提示词或参考后重新获取匹配报价；重复确认沿原身份查询/重放，不重复生成。                      |
| 业务配置变化   | `canvas/application/yjs_projection.go:116` 将 timeline/director 等视为配置差异；`frontend/src/components/canvas/service-graph.ts:290` 仅在本地转换投影差异              | 新节点/高级配置必须进入既有 Yjs 投影，不新增 HTTP 整图覆盖、浏览器专用数据库或 OT。              |
| 权威保存       | `canvas/adapter/postgres/yjs.go:168` CommitYjs；`:176` 重验写权限；`:279` 同事务更新接受状态和 commit sequence，`:282` 写 update                                        | UI 必须等待对应接受回执；联网、WebSocket 发出、本地编辑成功都不等于已保存。                      |
| 回执幂等       | `canvas/adapter/postgres/yjs.go:42` actor/session/epoch/update ID 范围回执查询；`:63` 写 request hash 与响应                                                            | 断网/未知结果重连按同一更新身份确认；不得靠时间最新的回执替代原更新结果。                        |
| 历史恢复       | `canvas/adapter/postgres/yjs_history.go:137` CommitYjsRestore；`:190` 重新检查主体来源；`:247` 推进 epoch                                                               | 高级节点配置、固定引用及资源失权都纳入恢复；不能回放过时执行/授权事实。                          |

特别注意：`service-graph.ts:38` 会映射 image_generation/video_generation/generation 到前端基础类型，同时保留 serviceData。扩展高级节点时不能丢失原 node_type/node_action，也不能通过当前 `create_node` 固定 resource 的路径写入 timeline/director。新的主体节点需要补后端合法类型及对应授权字段，这是协议变化，而非单纯 UI 配置。

## 8. 验证记录与后续验收入口

本次执行的是 `rg --files`、`rg -n`、`nl -ba`、`sed`、`git rev-parse`、`git status` 等静态只读核查，检查了根/前端 AGENTS 和指定设计、研究、计划。未运行单元测试、集成测试、浏览器流程、服务启动、生成或导出任务。

已定位可用于后续回归的现有测试文件（文件存在不表示本次通过）：

- `backend/tests/canvas/timeline_test.go`、`timeline_reader_test.go`。
- `backend/tests/canvas/director_test.go`、`director_outputs_test.go`、`director_outputs_http_test.go`、`director_outputs_integration_test.go`。
- `backend/tests/media/export_renderer_test.go`、`export_audio_test.go`、`export_captions_test.go`、`export_http_integration_test.go`、`export_lock_scope_integration_test.go`、`tool_routed_export_test.go`。
- `backend/tests/prompt/subject_binding_test.go`、`backend/tests/subject/actual_generation_test.go`。
- `backend/tests/canvas/yjs_integration_test.go`、`yjs_live_integration_test.go`、`yjs_browserqa_test.go`。

交付核对仍按 Plan23 的 T08/T11/T14/T15/T16 及 CASE-CJC-16/17/18 进行。菜单、类型、DTO、样式、示例结果、静态测试文件都不能代替正式入口的真实任务往返或最终文件验证。

## 9. 追加核查：正式 Yjs 画布生成的最小安全接线

### 9.1 旧 useCanvasGeneration 不能直接启用

`frontend/src/components/canvas/hooks/use-canvas-generation.ts:229` 的 generate 调用 prepare，带 expectedRevision/projectBindingRevision/epoch/commitSeq/graphHash，随后调用旧 BFF 的 list/quote/submit/refresh/retry。这些字段看起来完整，但真正的 `runtime/lib/request.ts:137` 将没有适配的路径全部拒绝为 50100。该适配器目前只处理 session、models/options、files/transfer；没有任何 `/canvas/generation/*` 转换。旧客户端方法存在不表示正式 Go API 存在。

另有两个直接不兼容点：

- `use-canvas-generation.ts:244` 固定查询 image modality；`:246` 依赖 billingEnabled 决定是否报价，而现有 request 适配器 `:119` 仅返回 parameters，因此不能靠旧流程获得真实报价及确认。
- 同 hook `:140` 仅接受 `/api/file/content?ticket=` 预览授权，正式媒体 API 使用 getMediaPreview 返回值；不得伪造 ticket URL 或结果 VO 来穿过旧判断。

正式入口 `yjs-workspace.tsx:113` 传入 CanvasPage 的 documentIntegration 没有 generation/workflow；旧 `editor/canvas-document-page.tsx:631` 的挂载属于另一宿主。应接当前 Yjs 宿主，不重新开放旧保存路径。

### 9.2 直接复用 RichGeneration 的画布宿主合同

`frontend/src/components/generation/rich-generation.tsx:129` 已提供 `CanvasComposerOwner`：draft、onChange、可选 collaboration/onSession、`flush(): Promise<API.InputSource>`、referenceSources、incomingSources。`GenerationSeed.canvasOwner` 位于 `generation-dialog.tsx:42`，并在 `:178` 传给 RichGeneration。可以使用同一组件的 `presentation="canvas"` 实现节点内联/展开，避免为画布重建报价、主体引用与任务状态机。

现有正式链路：

1. `RichGeneration.flush():791`：先拒绝未完成 IME，再调用 canvas.flush。
2. `getQuote():855`：检查模型/参考资格；捕获 localSeq；flush 后检查是否又有编辑；调用 `discover():815` 恢复当前提交。
3. `createFreeQuote():889`：请求仅使用 input_source、expected_submission_epoch、force_regenerate，Idempotency-Key 沿原提交重用；服务端从保存的文档读取输入并编译。
4. `confirm():904`、`:936`：使用服务端返回的 confirm_key 调用 confirmOperationQuote；未知确认保留原 key；不重复创建执行。
5. `generation-dialog.tsx:329` 的 TaskDetail 使用 getOperation/cancelOperation；`:351` 采用回调给出正式 assetID/kind，并处理采用失败。画布宿主负责把该资产绑定进正确节点和 Yjs 编辑。
6. `resolve():953` 经 resolvePromptSubmission 处理放弃报价/确认结果，保留 submission epoch 语义。

生成客户端位于 `frontend/src/api/operations.ts`：getOperation `:80`、cancelOperation `:96`、confirmOperationQuote `:115`、createFreeQuote `:185`、listProjectOperations `:204`。提交恢复位于 `frontend/src/api/promptEditor.ts:122` 的 currentPromptSubmission。全部复用生成 API，不手写并行 `/canvas/generation` 接口。

### 9.3 输入节点协议是当前最大阻断

后端 `backend/internal/prompt/domain/input_source.go:8` 的 InputSource 有两种正式来源：prompt_draft 与 canvas_input。后者要求 canvas_id、node_id、canvas_revision、input_revision，两个 revision 均为正值。`adapter/postgres/input_source.go:41` 锁定项目画布并检查 canvas_revision；`:52` **仅读取 image_generation/video_generation 节点**；`:56` 要求 input_revision 一致；`:60` 要求 config.draft.composer 存在；`:65` 检查固定参考。确认阶段允许仅布局修改，但仍检查输入版本。

与当前 UI 的差异：

- 新 image/video/text/audio 节点在 `service-graph.ts:327` 被序列化为普通 resource，配置在 editor；它们不是 canvas_input 的合法所有者。
- `serviceGraph():38` 虽可把已有 image_generation/video_generation 映射成 image/video 显示，但 `:36` 取的是顶层 config.composer，当前编辑器对 draft.composer 没有完整的读写桥接。
- `service-graph.ts:365` 更新 config.editor，`:398` 仅文本节点更新 text_content；没有将生成编辑意图写入 config.draft 的路径。
- `backend/internal/canvas/domain/draft.go:19` IsDraft 目前只支持图片和视频；文本/音频不能假装通过同一 canvas_input 合同。普通 quote 的 inline payload 虽存在，但不等于具有同样的保存输入/提交槽保障。

最小接入应先完成一个已有 image_generation/video_generation 的正式草稿闭环，再扩展节点统一视觉和文本/音频合同。若产品坚持“同一个资源节点直接生成”，需要在设计中明确资源节点与生成输入所有者的关系及服务端校验，不能偷偷改 node_type 或把画布输入降级成无来源的临时 prompt_draft。

后端 `canvas/domain/operations.go:413` 起会初始化/推进 InputRevision 并保存输入快照，前端不自行增加 input_revision；读取接受后的正式 DTO。`Draft` 使用 Composer 时，其旧 prompt/negative_prompt/aspect_ratio/candidate_count 字段必须为空或0（`domain/draft.go:24`），不能把两套模型同时填入。

### 9.4 Yjs epoch 与 submission epoch 必须分开

Yjs 确认头在 `yjs-live-session.ts:567` 形成，包含 epoch（UUID）、commitSeq、contentRevision、graphHash；输入提交 epoch 是 operation.prompt_submission_slot 的整数，不是 Yjs epoch。`RichGeneration.discover():823` 获得后者；严禁互相转换或把 commitSeq 作为 expected_submission_epoch。

当前 `CanvasYjsLiveSession` 只有 private drain（`:698`），没有公共 prepare/flush generation 合同。drain `:721` 调用 session.flush，且可能因离线、手势、输入法草稿、未知回执、epoch 切换或拒绝状态停止；因此 UI 调用 change 后立刻取 view.dto.revision 不安全。view.document 是 `render():266` 从本地 snapshot 生成的编辑投影，也不能冒充已提交快照。

建议补一个明确宿主方法 `prepareInput(nodeID)`（新方法建议名，并非现有接口）：

1. 固定 actor/scope/project/canvas/node 身份，捕获编辑序列；拒绝 IME、未完成手势、上传/上传失败、只读、恢复输入、迁移或历史恢复进行中。
2. 沿现有 drain/flush 队列等待当前输入得到匹配回执，禁止并发调用两个 flush；UNKNOWN 先查原结果，不能当作 clean。
3. 在确认仍为同一 epoch、无新本地编辑、无 inFlight/sealed/recovery、权限仍有效时，从与确认 contentRevision 对应的接受 DTO 取得 input_revision。若 DTO 尚未跟上回执，等待 sync 或显式读取并比对，不能用本地预测值。
4. 返回正式 API.InputSource `{kind:"canvas_input", canvas_id, node_id, canvas_revision, input_revision}`。保留 epoch/commitSeq 作为宿主内部变化围栏；当前 Go InputSource 没有这两个字段，不伪造已被服务器校验。
5. 取得报价后服务端仍会锁定 revision，后续任何变化导致 stale 都由 RichGeneration 的现有错误/恢复处理接住。确认按输入版本校验，不能把合法布局移动误认为重新生成。

若要让服务端在报价事务内额外锁定 Yjs epoch/commitSeq，需要明确扩展 Go InputSource/API/校验/生成客户端；当前 operation/prompt 代码没有这种检查。现有强约束是文档 revision + input_revision + 保存来源授权，不能在交付说明里声称当前已具备 epoch/commitSeq 报价校验。

### 9.5 必需的局部验证

先验证保存 composer→确认输入版本→真实报价→同 key 确认→任务→正式资产采用的单节点闭环，再扩展四类节点。应覆盖：保存中继续输入、IME、断网未知回执、epoch 恢复、协作者编辑输入/只移动节点、撤权、跨项目切换、刷新恢复未决报价、超时确认原 key 重试、结果采用时节点被删。真实供应商生成尚未在本核查执行。
