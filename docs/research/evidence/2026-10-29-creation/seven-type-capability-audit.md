# 七类创作能力与可复用协议审计

- 日期：2026-10-29；范围：第二阶段只读源码审查。
- 对象：首页 Agent、图片、视频、音乐、音频、数字人、动作模仿，以及可复用的通用生成、画布助手、主体音色链路。
- 本次只新增本文件；没有修改实现、Figma、70 项矩阵或 BACKLOG，没有读取凭据、查询账户配置或调用计费接口。
- “协议已具备”指当前工作区源码提供相应真实请求路径，不等于当前环境已部署、账户有可用 release/凭据、模型已启用，或真实生成已验收。
- 画布助手应用/存储与前端目录在本次读取时为未跟踪并行新增内容；RichGeneration 有未提交修改。它们属于工作区证据，不能当成主分支已交付能力。

## 结论与接入边界

| 首页类型 | 当前可复用的真实协议 | RichGeneration 接入判断 | 不能直接宣称具备的能力 |
| --- | --- | --- | --- |
| Agent | text.generate/text_to_text；画布助手会话与 text/image 执行 | 可复用文本讨论、图片生成、草稿和轮次基础；首页会话所有权需要设计 | 即梦 Agent 自动规划、跨工具执行、偏好配置、结构化技能、完整多媒体会话 |
| 图片 | image.generate：text2image/image2image | 已有首页接入；可继续在实际模型限制内完善 | 当前 Ark 单次仅 1 张，不能只改数量菜单即支持 1–4 张；尺寸比例需协议支持 |
| 视频 | video.generate：text2video/image2video/omni_reference | 已有首页接入；全能参考/首尾帧可复用现有提交与任务查询 | 五种产品模式全部、视频编辑、多帧序列、180 秒超长、样片转高清 |
| 音乐 | 所查发布门禁与 HTTP 提交器没有音乐能力 | 不能通过把 music 映射到 audio 接通 | 歌曲/纯音乐、歌词、音乐智能时长及相关费用 |
| 音频 | audio.generate/text2audio；独立豆包语音协议、MP3 结果入库基础 | 可以接通受限“文本朗读”；需要解除首页门禁、修正文案与音色配置 | 全能配音、多音色时间轴、上传音频作为参考、音色克隆、全部/我的/收藏与四属性完整音色库 |
| 数字人 | image/video/audio 可作为组成材料；未见头像驱动/口型专用发布与提交合同 | 不能直接用通用视频表单完成 | 角色+音色/讲话内容/音频驱动/动作描述及三模式 |
| 动作模仿 | 视频全能参考能接受参考视频，但没有动作模仿专用合同 | 不能仅把人物与动作视频塞进 omni_reference 就认定一致 | 双槽位、禁正文提交、人物动作绑定、三种专用模型与授权/质量规则 |

此表的缺失判断限定所查主链路。历史筛选中出现 song、instrumental、lip_sync、motion_mimic 字符串，不能作为执行能力证据。媒体/账户模块的 avatar 是头像处理，也不等于数字人生成。

## 1. 通用链路可复用什么

### 前端所有权与能力选择

- [generation-dialog.tsx:39](../../../../frontend/src/components/generation/generation-dialog.tsx#L39)：GenerationSeed 已允许 image、video、audio、text。
- [rich-generation.tsx:207](../../../../frontend/src/components/generation/rich-generation.tsx#L207)：按项目与 capability 分页加载真实目录；[252](../../../../frontend/src/components/generation/rich-generation.tsx#L252) 使用 seed.capability + ".generate"。
- [rich-generation.tsx:278](../../../../frontend/src/components/generation/rich-generation.tsx#L278)：读已有服务端草稿；[286](../../../../frontend/src/components/generation/rich-generation.tsx#L286) 从 composer/复用输入创建草稿；[341](../../../../frontend/src/components/generation/rich-generation.tsx#L341) 使用幂等键创建。
- [rich-generation.tsx:880](../../../../frontend/src/components/generation/rich-generation.tsx#L880)：保存并冻结输入后报价；[936](../../../../frontend/src/components/generation/rich-generation.tsx#L936) 以原确认键提交，未知结果保留恢复入口。
- [home-creation.tsx:94](../../../../frontend/src/app/_components/home-creation.tsx#L94)、[138](../../../../frontend/src/app/_components/home-creation.tsx#L138)：首页 workspace/supported 仅 image/video，其他五类走未开放提示。这是音频与文本接入的前端门禁，不代表后端完全没有这两类能力。
- [generation-dialog.tsx:98](../../../../frontend/src/components/generation/generation-dialog.tsx#L98)：标题只分 video/text，否则“图片生成”；如果开放 audio，必须修正这个实际错误，不能只扩 supported。

### 后端报价、发布与结果边界

- [create_free_quote.go:35](../../../../backend/internal/operation/application/create_free_quote.go#L35)：统一请求有 capability/mode/params/output_count/media_inputs 或 InputSource；[59](../../../../backend/internal/operation/application/create_free_quote.go#L59) 禁止源草稿和重复原始参数混传，普通输入要求非空 Prompt。
- [create_free_quote.go:118](../../../../backend/internal/operation/application/create_free_quote.go#L118)：报价冻结 release/model/version/schema/limits/price；不能绕过模型目录把任意类型标识发往供应商。
- [model_releases.go:89](../../../../backend/internal/catalog/adapter/postgres/model_releases.go#L89)：已查 managed release 门禁只允许 text.generate、image.generate、video.generate、audio.generate 的特定模式/计价单位；非图片限制 max_outputs=1。视频支持查询；此门禁要求不支持供应商 cancel/callback，因此 UI“取消任务”不代表已中止供应商计费。
- [client.go:67](../../../../backend/internal/operation/adapter/providerhttp/client.go#L67)：speech 走独立分支，其余走 submission；实际 Submit 是计费请求，本次未调用。
- [managed/activities.go:146](../../../../backend/internal/operation/adapter/managed/activities.go#L146)：分别接收拒绝、未知、受理；[158](../../../../backend/internal/operation/adapter/managed/activities.go#L158) 识别供应商 pending/running；[166](../../../../backend/internal/operation/adapter/managed/activities.go#L166) 固化文本/图片/音频/视频结果。内部供应商状态存在，不等于前台任务枚举已经分别呈现排队与运行。
- [managed_result.go:21](../../../../backend/internal/operation/application/managed_result.go#L21)：结果 manifest 与私有字节是恢复证据；[33](../../../../backend/internal/operation/application/managed_result.go#L33) 校验 accepted 与 succeeded、计费 usage、文件类型和大小。
- [generation-dialog.tsx:379](../../../../frontend/src/components/generation/generation-dialog.tsx#L379)、[638](../../../../frontend/src/components/generation/generation-dialog.tsx#L638)：前端任务轮询、参数、预览、取消、复用基础可复用。公开发布仍为独立资产动作。

## 2. 图片：协议已接入，数量与尺寸不能只改 UI

- [model_candidates.go:143](../../../../backend/internal/catalog/domain/model_candidates.go#L143) 的 onboarding 默认 max_outputs=1；[151](../../../../backend/internal/catalog/domain/model_candidates.go#L151) 图片引用上限为 8，角色 reference_image/subject，仅 image2image；size 默认仅 2K/4K。
- [volcengine_models.go:41](../../../../backend/internal/catalog/domain/volcengine_models.go#L41) 有 Seedream 系列候选，但候选名称不是账户可用证明。
- [submission.go:29](../../../../backend/internal/operation/adapter/providerhttp/submission.go#L29) 区分 OpenRouter 和 Ark；Ark 显式拒绝 OutputCount!=1，并设置 sequential_image_generation=disabled。
- [submission.go:148](../../../../backend/internal/operation/adapter/providerhttp/submission.go#L148) 图生图把图片 URL 发送为 image 数组；[168](../../../../backend/internal/operation/adapter/providerhttp/submission.go#L168) 文生图要求无引用、图生图要求存在引用。
- [client.go:117](../../../../backend/internal/operation/adapter/providerhttp/client.go#L117) 要求图片返回数严格等于请求数，否则进入 unknown，不能视为已支持“部分张数成功”。

建议接入边界：保留当前文/图生图、结构化引用和单次真实结果闭环；若要多图，要同时定义发布上限、供应商请求、部分结果、报价和结算。像素 W/H 与约束比例需要明确如何映射 size，不能发送协议白名单之外的任意字段。

## 3. 视频：三种执行模式与五种产品模式并不等价

- [model_candidates.go:158](../../../../backend/internal/catalog/domain/model_candidates.go#L158)：first_frame 必需且最多 1，last_frame 可选；image2video 与普通 omni 引用有不同角色。
- [volcengine_models.go:59](../../../../backend/internal/catalog/domain/volcengine_models.go#L59)：当前 Seedance 2.5 schema 定义 4–30 秒、480p/720p/1080p、adaptive 与固定比例、generate_audio；引用总量分别图片 30/视频 10/音频 10。旧型号另有更小限制，不能共用 2.5 的范围。
- [submission.go:105](../../../../backend/internal/operation/adapter/providerhttp/submission.go#L105)：视频/音频引用只允许 Ark video.generate + omni_reference 及对应角色，按 video_url/audio_url 序列化。
- [submission.go:119](../../../../backend/internal/operation/adapter/providerhttp/submission.go#L119)：首尾帧不能重复，不能与普通参考混用，尾帧不能单独存在。
- [submission.go:164](../../../../backend/internal/operation/adapter/providerhttp/submission.go#L164)：最终只认可 text2video、image2video、omni_reference；2.5 首尾帧请求强制 adaptive。
- [reference-mode.ts:34](../../../../frontend/src/components/generation/reference-mode.ts#L34)：紧凑模式从引用角色推断，优先 omni_reference；应验证无引用/空角色情况下推断与后端“omni 必须有引用”的一致性，不能仅依赖推断函数存在。

可复用：全能参考与首尾帧表单、模式参数迁移、异步任务受理/查询、视频入库。新增服务合同：多帧顺序与时间、源视频编辑、超长分段/长时长策略、样片与高清转换的父子任务和二次费用；这些不能由现有三模式更名获得。

## 4. 音频：文本朗读能利用现有协议，完整音色库仍需桥接

### 已有通用文本朗读链路

- [volcengine_models.go:55](../../../../backend/internal/catalog/domain/volcengine_models.go#L55)：audio.generate/text2audio，资源 seed-tts-2.0。
- [model_candidates.go:153](../../../../backend/internal/catalog/domain/model_candidates.go#L153)：speaker、speech_rate、loudness_rate；速率/音量 -50..100，默认固定 speaker。
- [speech.go:24](../../../../backend/internal/operation/adapter/providerhttp/speech.go#L24)：只能单输出、无参考素材、非空正文且最多 16384 字节；拒绝其他模式和额外 endpoint。
- [speech.go:33](../../../../backend/internal/operation/adapter/providerhttp/speech.go#L33)：MP3/24000Hz，speaker 字符白名单，参数仅上述三项；[154](../../../../backend/internal/operation/adapter/providerhttp/speech.go#L154) 解析终态与真实 MP3 字节。
- [managed/activities.go:175](../../../../backend/internal/operation/adapter/managed/activities.go#L175)：audio.generate 的音频结果有暂存分支，不是仅有候选模型名；[managed_result.go:59](../../../../backend/internal/operation/application/managed_result.go#L59) 接受 MP3 manifest。

因此可以在现有 RichGeneration 接通“文本朗读”子能力：独立 audio 草稿→选择已启用模型→正文和合法 speaker 参数→冻结报价→确认→真实 MP3 资产/结果。前端必须禁止这条模式引用视频/音频/人物媒体，不能因为通用编辑器支持 @ 就放行供应商不支持的输入。

### 已有主体音色系统，不应重复建设

- [unified-voice-controls.tsx:26](../../../../frontend/src/components/subjects/unified-voice-controls.tsx#L26)：选择 release/voice、名称与试听文本；并非首页音色库。
- [subject_voice.go:30](../../../../backend/internal/catalog/adapter/postgres/subject_voice.go#L30)：主体音色目录读取已发布 speech release，验证当前凭据对应每模式的 real_provider 成功记录；同时兼容 audio.generate/text2audio 与 audio.tts/tts。
- [subject_voice.go:45](../../../../backend/internal/catalog/adapter/postgres/subject_voice.go#L45)：选项来自 schema speaker/voice_id enum，缺 enum 时用合法 default；当前 Name=key，因此不能当成已具备年龄/性别/语言/风格等属性。
- [subject/doubao/speech.go:26](../../../../backend/internal/subject/adapter/doubao/speech.go#L26)：主体试听复用同一 providerhttp 协议，要求严格终态，映射为 audio.generate/text2audio。
- [app/subject/voice_execution.go:23](../../../../backend/internal/app/subject/voice_execution.go#L23)：主体 worker 入库为 personal generated audio，拥有独立 job/receipt/ownership；[121](../../../../backend/internal/app/subject/voice_execution.go#L121) 限定个人所有权，不可直接冒充任意项目生成任务。
- 另有 [voice_catalog.go:53](../../../../backend/internal/catalog/adapter/postgres/voice_catalog.go#L53) 的旧型项目目录仅筛 audio.tts；[application/voice_catalog.go:58](../../../../backend/internal/catalog/application/voice_catalog.go#L58) 有音色模型版本与 schema hash 冻结合同。它与 release-based 主体目录不是同一个查询，应先确认调用链并统一边界，不能接错目录后认为没有音色。

需要桥接：音色选择如何转换并冻结 speaker、项目权限与个人主体权限、版本失效、可移除音色标签及跨类型草稿恢复。首页完整音色库另需属性/收藏/我的/试听回执和许可。全能配音、多说话人、时间戳、上传参考声音/克隆均不被当前通用 speech 请求接受，应单独扩协议。

## 5. Agent：画布助手可复用，但首页会话和自动编排尚不等价

- [canvas/application/assistant.go:22](../../../../backend/internal/canvas/application/assistant.go#L22)：会话保存 DraftID/OperationID/skill/execution；[30](../../../../backend/internal/canvas/application/assistant.go#L30) 强制 CanvasID 和 ProjectID。
- [assistant.go:89](../../../../backend/internal/canvas/application/assistant.go#L89)：Append 仅绑定草稿、不直接调用模型；只允许 discussion，或 poster/product_images 的 image 执行。
- [assistant-state.ts:10](../../../../frontend/src/components/canvas/assistant/assistant-state.ts#L10)：七个 skill preset；[194](../../../../frontend/src/components/canvas/assistant/assistant-state.ts#L194) 把所选技能展开为普通 text 提示词，并非结构化内联技能对象。
- [assistant-state.ts:87](../../../../frontend/src/components/canvas/assistant/assistant-state.ts#L87)：从已结束任务的 editor_draft 与结果拼接“助手/用户”正文来续轮，有未知/未终止保护与上下文上限。
- [canvas-assistant-panel.tsx:622](../../../../frontend/src/components/canvas/assistant/canvas-assistant-panel.tsx#L622)：已有 RichGeneration presentation=assistant，按 execution 选 text 或 image。
- [submission.go:21](../../../../backend/internal/operation/adapter/providerhttp/submission.go#L21)：managed text.generate 是单条 user 消息、stream=false、无媒体引用、单输出；不是工具调用协议。
- [codex/chat.go:95](../../../../backend/internal/operation/adapter/codex/chat.go#L95)：另有只读运行时会话传输和未知投递防重机制；有此适配器不等于首页已经使用它。

可复用：讨论/文本结果、用户明确触发图片、已有草稿和轮次/历史保护、同一报价提交机制。需要新增或明确：无画布首页会话归属、Agent 自动与自定义模型偏好、技能版本/参数/管理、工具调用及步骤事件、文本→图片/视频的显式执行/成本边界和恢复。不可把现有技能标题中“全流程广告片”文字当成已经执行全流程的证据。

## 6. 音乐、数字人、动作模仿的新增合同范围

所查 managed release 与 provider submission 不提供这三类专用执行模式，HomeCreation 也未接入。新类型需要同时覆盖：

| 类型 | 必须补的输入/执行合同 | 输出与费用合同 |
| --- | --- | --- |
| 音乐 | 歌曲/纯音乐模式，歌词与风格区分，智能或固定时长，模型限制和合法参数，是否允许参考音频 | 音乐音频及可选歌词/时间轴，多候选与部分失败，按次/时长的冻结和结算 |
| 数字人 | 人物角色来源/授权，文本+音色或上传音频的互斥与迁移，口型/驱动/动作参数，三模式与供应商绑定 | 视频/声音同步、时长限制、源音频关系，异步任务/取消/失败与费用 |
| 动作模仿 | 角色图片/视频与动作视频专用槽，正文禁用，模型目录、质量和时长约束，材料授权 | 人物动作映射证据、生成视频、失败重试、输入时长到报价的映射 |

[personal_history.go:45](../../../../backend/internal/operation/application/personal_history.go#L45) 的筛选枚举及 [personal_history_facts.go:27](../../../../backend/internal/operation/adapter/postgres/personal_history_facts.go#L27) 的分类 SQL 已保留一些模式名，但没有替代上述执行合同。也不能用 video omni_reference 的参考视频能力直接声称动作模仿已实现。

## 7. 复用时必须保留的验证边界

1. 每种类型先验证项目目录确有可用 release，权限、模型版本、参数、引用角色与输出上限一致；不靠候选静态名称判定。
2. 草稿切换不得把图片引用带入无引用 text2audio，也不得把普通视频引用误当首尾帧；迁移须有确定规则。
3. 报价冻结、原确认幂等、未知结果核对继续走同一机制。重试原确认与用户再次创作要分开。
4. 真实结果必须经过字节/终态/入库证据；按钮、模型列表、播放预览和对象存在都不能独立证明闭环。
5. 本轮没有执行以上运行验证。建议的协议复用不代表发布完成；生产账户支持范围仍为未知。

## 文档验证

本次仅核对文档引用文件及行号边界，并运行该新增文件的格式检查；没有运行功能测试、构建或真实供应商请求。本文件补充并扩大了前次 implementation-audit 的阅读范围：前次“创作主链路未见技能/音色入口”不应解读成全项目不存在画布助手或主体音色基础。
