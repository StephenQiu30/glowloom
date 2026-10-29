# 全能配音：即梦要求、Seed-Audio 1.0 官方契约与落地边界

核验日期：2026-10-29。范围：只读公开官方资料和当前源码；未读取密钥、未提交生成、未试听、未创建音色。本文是协议证据，不构成真实供应商验收。

## 1. 需求证据的强弱

已有即梦观察在 `observations.md` C11、C12、C20，以及 `../../12-即梦创作完整需求分析.md` CR-042～045：

- 音频模式分为全能配音与文本朗读，模型显示 SeedAudio 1.0；全能配音提示多音色和时间戳编排。
- 音色库有全部音色、我的音色、收藏，以及性别、年龄、语言、声音特点筛选。选择公共音色成为可移除参考，页面内切换后仍保留。
- 未执行生成、试听、收藏、创建、上传，没有证明服务端持久化、多音色真实效果、精确时间戳语法、分轨编辑或逐帧对齐。

因此多音色、时间安排、试听、创建、授权要分别验收；不得把现有单音色 TTS 改名就标为全能配音。音色属性与收藏是本地目录/账户数据，不是生成接口天然返回的能力。

## 2. 官方主协议

来源：[Seed-Audio 非流式音频生成](https://docs.volcengine.com/docs/doubao-voice/audio-generation-http?lang=zh)，DocumentID 2550782，页面更新时间 2026-09-28。本次读取服务端公开 `MDContent`，并从 `Content` 的 zone 层级确认嵌套、从 `apiSampleData` 读取请求/响应示例。

`POST https://openspeech.bytedance.com/api/v3/tts/create`

JSON 非流式响应；鉴权头 `X-Api-Key`，可选 `X-Api-Request-Id` 用于追踪。文档未要求 `X-Api-Resource-Id`，也没有此接口的 task_id 查询、取消或幂等重放协议。请求 ID 不应被解释成付费幂等保证。官方示例读取超时 300 秒。

| 位置 | 字段及约束 |
| --- | --- |
| 根 | `model: "seed-audio-1.0"`，必填 |
| 根 | `text_prompt`，必填，最多 3000 字符 |
| 根 | `references` 可选数组：最多 3 条音频参考或 1 条图片参考；图片与音频/音色不可混用 |
| 每条音频参考 | `speaker` / `audio_data` / `audio_url` 三选一；speaker 可使用 TTS 2.0 音色或复刻音色。引用音频每条 ≤30 秒且 ≤10MB，wav/mp3/pcm/ogg_opus |
| 每条图片参考 | `image_data` / `image_url` 二选一；jpeg/png/webp，≤10MB；错误码文档另说明最小 1KB |
| `audio_config` | `format`: wav 默认，或 mp3/pcm/ogg_opus；`sample_rate` 按格式校验；`speech_rate` / `loudness_rate`: 整数 -50～100，默认 0；`pitch_rate`: 整数 -12～12，默认 0；`enable_subtitle`: 默认 false |
| `watermark` | 可选 `aigc_watermark` 和 `aigc_metadata`；后者含 enable/content_producer/produce_id/content_propagator/propagate_id |

采样率：wav/pcm 默认 40000，可用 8000/16000/24000/32000/40000/44100/48000；mp3 默认 44100，可用 8000/16000/24000/32000/44100/48000；ogg_opus 仅 48000。最小闭环可固定 mp3 + 24000，避免把全部参数一次暴露到 UI。

文中 `@音频1`、`@音频2` 等严格指向 references 数组顺序。应用草稿使用稳定引用 ID，但提交时必须统一编译为当前物理序号，不能把删除后的旧展示编号原样发送。文本提示可描述总时长、各段开始结束、角色台词与音效；最长生成 120 秒。未公开结构化 segments/timeline、多个独立声轨输出或逐帧时间保证。

详细 API 列出 18 种语言。模型总览页仍给出较少语言及采样率集合，存在页面差异；实现优先精确 API 文档，实际开放按真实请求验证，不能仅凭总览扩张模型能力。

以下为本项目构造的最小请求，不是已执行记录：

```json
{
  "model": "seed-audio-1.0",
  "text_prompt": "总时长约10秒。0到4秒，@音频1平静地说：早上好。5到9秒，@音频2温柔地回答：今天也加油。",
  "references": [
    {"audio_url": "<authorized-reference-audio-1>"},
    {"audio_url": "<authorized-reference-audio-2>"}
  ],
  "audio_config": {"format": "mp3", "sample_rate": 24000, "enable_subtitle": true}
}
```

可用已获授权的 `speaker` 替换某一 audio_url。不能向用户展示任意 speaker ID 输入然后绕过目录发布与所有权校验。

## 3. 响应、持久化与费用

| 字段 | 语义 |
| --- | --- |
| `audio` | base64 音频，可直接解码归档，避免依赖短效链接 |
| `duration` | 处理后的时长，秒 |
| `original_duration` | 原始模型输出时长，秒；官方明确作为计费依据，不可替换为 duration 或解码媒体探测时长 |
| `url` | 音频链接，有效 2 小时；官方例子只有占位符，无可据以扩大 allowlist 的实际 CDN 域名 |
| `subtitle.text` | 字幕文本 |
| `subtitle.sentences[]` | start_time/end_time（毫秒）、text、words[]；words 同样有 start_time/end_time/text。没有正式 speaker ID 字段 |
| `code` / `message` | 错误信息；官方 HTTP 200 成功示例省略这两项，不能要求 code 字段一定存在 |
| 响应头 `X-Tt-Logid` | 供应商排障跟踪 |

成功判定应结合 HTTP 2xx、code 缺省或 0、有效音频、有效 original_duration；不可复用 TTS 的流式终帧 20000000。base64 解码及总响应必须有容量上限；格式、时长、字级字幕范围检查后保存。字幕应关联最终音频资产并可读回/导出，不能仅留在临时弹窗或把大型字节塞入工作流历史。对字幕缺失要如实标注，不能杜撰时间轴。

[计费说明](https://docs.volcengine.com/docs/doubao-voice/billing-instructions?lang=zh)（2026-09-29 更新）列出 Seed-Audio 按量 1 元/分钟；不是 TTS 2.0 的按字符费率。官方说明按时长计费会累加各次调用时长，精确至秒后折算分钟/小时，并明确倍速不改变原始时长计费。未说明浮点秒的小数具体舍入规则，应保留原始秒数凭据并由价格规则明确舍入，不得按每次整分钟上取整。报价冻结可按最大 120 秒封顶预留，实际按 original_duration 结算；自然语言目标时长不等于供应商硬时长参数。

非流式调用超时且未拿到结果时没有公开查询回执协议；标记请求未确认并人工/供应商核对，不自动重发付费请求。读取响应后应先持久化私有收据/音频再做后续归档，避免对象存储暂时失败导致重复生成。

[错误码](https://docs.volcengine.com/docs/6561/2534853?lang=zh)：45001001 参数/体积；45001115 音色不存在；45001116 提示词过长；45001117 参考音频超过 30 秒；45001125 文本审核；45001104 敏感声纹；45001127 音频审核；45001130 图片审核；45001131/45001132 下载引用失败；40000020 可对应预测超过两分钟、安全或图片尺寸问题，需结合 message；40402002 无可读文本；55001309 内部生成、55001310 输出音频审核、55001311 输出声纹。不要把所有 5xx 自动重发：传输不确定性仍有付费风险。

## 4. 音色目录、试听、创建是独立链路

[官方音色列表](https://docs.volcengine.com/docs/6561/1257544?lang=zh)提供 TTS 2.0 voice_type、名称、语种/方言、能力、标签。Seed-Audio 允许此列表的音色及复刻音色。目录可以引入确有来源的名称/语种/能力；性别/年龄/声音特点没有完整结构化官方保证，不应靠名称猜测填充。

[音色注册](https://docs.volcengine.com/docs/doubao-voice/tone-training-http?lang=zh)：POST `/api/v3/tts/voice_clone`，X-Api-Key + X-Api-Request-Id；speaker_id 必填，audio.data 为 base64，音频 ≤10MB，wav/mp3/ogg/m4a/aac/pcm（pcm 24k 单声道）。可选 language、参考 text、extra_params.demo_text（4～300 字）、降噪等。

[音色查询](https://docs.volcengine.com/docs/doubao-voice/tone-query-http?lang=zh)：POST `/api/v3/tts/get_voice`，同鉴权，speaker_id；状态 0 未找到、1 训练中、2 成功、3 失败、4 已激活；2/4 可合成。返回 speaker_status[]、model_type（复刻 2.0 为 5）、demo_audio（成功时有效 1 小时）、剩余训练次数等。

这是复刻音色的真实创建/查询/试听链路，不是将参考音频上传到普通资产后冒充“创建音色”。后付费自定义 ID 要按注册文档传固定 speaker_id=custom_speaker_id 与实际 custom_speaker_id。官方特别注明首次正式合成会转正并收音色槽位费；预览和正式生成必须在产品上区别，先使用注册返回 demo_audio 核验。Seed-Audio 对这种自定义 ID 的映射没有在本次精确字段中说明（references 仅有 speaker），需独立核实，不能推断支持额外 custom_speaker_id 字段。

全部/我的/收藏、用户所有权、音色复刻许可、归档后的试听文件均需本地持久化。选择音色与创建收费音色不是同一动作。

## 5. 当前实现差距及最小真实闭环

下列行号基于 2026-10-29 只读快照，共享工作区后续可能变化。

| 当前位置 | 缺口与最小变更 |
| --- | --- |
| frontend/src/app/_components/creation-type-fields.tsx:86 | 全能配音仍是“暂未开放”；先 Figma 完成模式、参考顺序/移除、时间提示、生成中/失败/未确认、字幕/播放结果与音色库状态，再接实际接口 |
| frontend/src/components/generation/creation-contract.ts:6 | audio 仅允许 text2audio；新增独立音频模式和独立草稿，不影响音乐模式 |
| frontend/src/app/assets/_components/history-results.ts:39 | 裸 mode=dubbing 被判断为视频配音；建议 omni_audio 或 audio_dubbing，统一更新模式分类与恢复 |
| backend/internal/catalog/domain/preset_model.go:66 | audio 模式仅 text2audio/music_song/music_instrumental；新增模式与 seed-audio-1.0 专用候选、能力约束、参数和真实验证发布门禁 |
| backend/internal/operation/adapter/providerhttp/speech.go:24 | 硬编码 seed-tts-2.0、text2audio、无引用、单音色；应新增独立 Seed-Audio adapter，不修改 TTS 名称冒充能力 |
| 同文件:49、168 | 参数只有 speaker/rate/loudness，usage 只有 text_words；新协议需嵌套 references/audio_config、响应字幕和 original_duration |
| backend/internal/prompt/domain/draft.go:75、307 | 现引用是媒体/主体等，Compile 输出展示标签；需要授权音色引用快照和物理序号编译，确保删除/排序后 @音频N 与发送数组一致 |
| backend/internal/catalog/application/voice_catalog.go:67 | 已有版本/模式参数快照可复用其授权原则，但目前不是完整音色库收藏/复刻所有权模型 |
| backend/internal/catalog/adapter/postgres/subject_voice.go:41 | 当前只发布 seed-tts-2.0 + 已真实验证链路；增加 Seed-Audio 能力必须独立验证，不能让新模型绕过凭据和 release 验证 |
| backend/internal/operation/application/provider_execution.go:41 | GenerationResult 无字幕字段；添加有界、类型明确的字幕结果，与资产持久关联 |
| backend/internal/operation/application/managed_result.go:29 | manifest 仅 Usage/SupplierUsage/Artifacts 等；字幕侧车/元数据的存储、回读、权限和归档需补齐，不能只存 SupplierUsage |
| backend/internal/operation/adapter/managed/activities.go:224 | music 专用 duration_seconds 与 TTS text_words；Seed-Audio 新增 original_duration 到 billed_duration_ms 的精确转换及相应计价，不能混用 music 字段 |

建议最小真实闭环：

1. 独立 Seed-Audio 模型和全能配音模式；文本必填；0～3 条已授权音频/音色引用；先支持自然语言时间安排，界面明确这是生成要求而非强制逐帧对齐。
2. 生成前校验长度、时长、引用数量、格式/体积、所有权、数组顺序，并冻结价格、凭据、模型、音色版本与引用快照。
3. 发出单次非流式请求，保留 request ID/logid；状态区分进行中、确认拒绝、请求未确认、成功。最小配置 mp3/24000/enable_subtitle=true。
4. 收到响应后持久保存音频与字幕，再建立音频资产/任务关系；结果播放、字幕毫秒时间高亮、复制/导出、刷新/历史打开均读真实归档。若字幕没有角色标签，不伪造说话人归属。
5. 双音色请求、参考删除再排序、无引用音效、超限、审核、超时、暂存恢复、实际计费秒数都须分别验收；真实供应商调用与 fixture 测试分别记录。
6. 音色收藏/目录/试听与复刻创建独立交付并验证；参考音频参与一次生成不意味着自动创建了可复用音色。

目前没有真实账号响应、供应商音频成品或字幕读回证据；没有验证超时查询恢复、硬时间对齐、声轨拆分、参考声纹质量或自定义后付费音色在 Seed-Audio 中的参数映射。
