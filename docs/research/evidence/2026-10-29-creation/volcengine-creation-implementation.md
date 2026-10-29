# 火山音乐、数字人、动作模仿实施与验证

日期：2026-10-29。依据：[设计29 §7.8](../../../design/29-创作一致性与公共组件设计.md)、[PRD16](../../../prd/16-创作一致性与公共组件.md)、[Plan25](../../../plan/25-创作一致性与公共组件实施计划.md)。进度只在 BACKLOG。本证据区分源码、离线协议、隔离数据库、本地媒体及浏览器，不等同真实供应商验收。

## 设计顺序与回读

先修改 Figma 三类对应桌面/手机表单，再实现协议与 UI：音乐 `3179:57043`、数字人 `3179:57311`、动作模仿 `3179:57500`，均位于[七类表单](https://www.figma.com/design/uLqmeRWuQuzfrp9xOYg6FT?node-id=3179-56558)。新增[三类六态说明板](https://www.figma.com/design/uLqmeRWuQuzfrp9xOYg6FT?node-id=3187-59172)：输入、报价、排队、受理未知、失败、完成归档。此板是可编辑的交互合同与按钮实例，不是六张完整运行截图。

回读发现模型按钮属性虽已更新，实际文本仍被旧 Seedream 覆盖；已同时修复嵌套实例属性与文本，动作表单截图确认显示“动作模仿 2.0”。六态说明也从 BGM 固定 120 秒预算改为按所选秒数生成。手机表单文本逐项回读；未把全部手机页面运行视为接受。

## 代码合同

- 三个独立 adapter：音乐 AK/SK HMAC，视觉 API Key；分别固定模型、路由和查询协议。签名包含真实请求字节、UTC 时间、cn-beijing/imagination/request。无自动提交重试；错误和空 data 不视为成功，过期/未知任务不自动创建新任务。
- 目录与凭据：三组固定主机预设、候选 schema、能力 mode/role 扩展；音乐和语音、普通视频和角色驱动视频按模式隔离。音乐签名密钥独立输入并进入已有加密凭据库。音乐模型不复用 TTS。
- 数字人按角色图片+驱动音频、动作模仿按图片+视频取服务端媒体事实；校验 MIME/大小/尺寸/时长。报价不接受客户端伪造视觉时长；无正文模式可正常报价，不生成空文字 part。
- 创建候选时预留公开价格对应的保守供应商成本；保存不自动启用。三类走正式逐模式试运行和发布门禁，不能使用 accepted-only 快速接入绕过真实结果。
- 结果沿现有持久任务与归档。音乐允许文档 CDN，经限量安全下载、ffmpeg 本地解码转 MP3、ffprobe 复核后归档；保留原始小数秒用量。视觉 URL 仅来自已认证查询，安全下载器限定 HTTPS origin、固定公网 DNS/IP、拒绝私网及跨源跳转，不携带 API Key。
- 前端复用 RichGeneration、ReferenceControls、ProjectMediaPicker、参数和任务组件。三类进入正式项目草稿与目录；动作双槽能进入真实视频选择器和上传入口，隐藏正文。音乐空目录保持歌曲/纯音乐标签，不显示文本朗读；Codex 图片旧 text_to_image 模式继续可选。

关键文件：`backend/internal/operation/adapter/providerhttp/volcengine_music.go`、`volcengine_visual.go`；`backend/internal/catalog/domain/creation_models.go`；`backend/internal/operation/application/creation_media.go`；`backend/internal/operation/adapter/managed/audio.go`；`frontend/src/components/generation/creation-contract.ts`、`rich-generation.tsx`、`reference-controls.tsx`；`backend/db/volcengine_creation_incremental.sql`。

## 验证记录

1. 新协议最初四例失败（adapter 尚不存在），实现后通过。最终五个包专项 Race 通过：providerhttp、managed、catalog/operation/media tests。覆盖签名固定向量、两种音乐字段、两种视觉路径、错误/空回执/错配 TaskID、不完整成功、超时仅一次提交、媒体约束与不能伪造时长、安全下载器的私网/重定向限制。
2. 真实本地 ffmpeg/ffprobe：合成 1 秒 PCM WAV 转 MP3 并探测时长；损坏输入拒绝。未上传个人媒体，没有供应商网络调用。
3. 独立 PostgreSQL 18 临时实例：完整 schema 一次、增量两次执行通过；三类预设哈希、真实连接与候选落库、幂等重放、非空成本和保持禁用的 Race 集成测试通过。此实例只含合成账号数据。未迁移业务库。
4. Go 全仓 `go test ./... -run '^$'` 编译通过（不计为全仓测试通过）；目标包 vet、golangci-lint 通过。govulncheck 可达漏洞 0，仍报告 1 个未被调用的模块级漏洞。
5. 前端最初六文件 40 通过、1 个 5 秒超时；遵照 PROJECT 使用 maxWorkers=2 后 41 项通过，未放宽超时。新增音乐空目录和动作双槽回归通过；测试中正文隐藏最初仅 class，改为原生 hidden/aria-hidden 后可访问性断言通过。
6. 全量前端：227 文件，1523 通过、1 失败；失败在并行 `components/canvas/timeline/timeline.css` 粗 outline 的 theme-policy，未修改该文件。生产构建曾通过；后续新增测试最初类型字段不完整，修正后另作最终类型与定向检查。详见本轮最终记录，不把早期通过覆盖后来的失败。

## 浏览器事实

实际登录态 `http://localhost:1122/`，无 API mock：音乐文字+纯音乐进入既有验收项目，真实创建项目草稿；空目录显示纯音乐。切换动作模仿，显示角色图片/动作视频双槽且无正文；动作视频槽打开只允许视频的真实项目资产选择器，当前列表空，上传入口可见。未选择用户素材、未点击报价确认或请求付费生成。浏览器前端为挂载源码的 dev 服务；后端未重启，三类真实执行仍不能以旧运行进程验收。

截图在父工作区 `output/jimeng-creation-2026-10-29/volcengine-motion-runtime.png`；设计截图为 `volcengine-state-design.png` / `volcengine-motion-design.png`。Figma 截图与项目空目录不会被列为真实供应商成功证据。

## 严格边界

没有读取或使用真实供应商密钥，没有开通/付费/生成/出账证据。增量未应用业务库，应用后端未重启。完整七类一致性、Agent 多步工具执行、全能配音、多音色、数字人文本驱动与更多模型、视频高级模式、刷新/跨设备草稿仍不能由这组三类代码覆盖。

Agent 只读审计发现：现有私有会话强绑定真实 canvas_id；画布技能是提示词插入；管理员 Codex trial 禁止工具，云 text.generate 尚无 tool call 合同。真实首页 Agent 需要独立项目会话、结构化工具调用、有界执行器和子 operation 关联，不能把普通文本生成改名完成。详细后续安排只见 BACKLOG。

未提交、推送或部署本组。共享工作区包含大量并行改动，逐项保留清单见父工作区 `output/jimeng-creation-2026-10-29/volcengine-worktree-manifest.md`。

## 最终增量检查

- 前端八文件 80 项全部通过（maxWorkers=2），包含签名密钥表单、音乐空目录、动作双槽与已有生成/引用合同。最终目标 ESLint 退出0，git diff --check 通过。
- 修正新测试的 required prompt / 组件回调 / Testing Library 参数后，tsc 曾退出0；最终生产构建 JS 编译通过，类型阶段被并行 `tests/unit/components/canvas/timeline/timeline-model.test.ts:614` 的 `playhead` 未定义字段阻断，退出1。不是本次创作代码的类型错误，也不声称最终构建通过。未改该并行测试或绕过类型检查。
- Go 最终目标包及 catalog 测试目录 golangci-lint 0 issues；新 Go 文件 goimports -l 无输出。五包专项 Race 再次通过。
- 浏览器音乐→动作模仿→音乐后，原文字“清晨海风与钢琴的音乐验收草稿”和纯音乐模式恢复；这证明本页项目草稿往返，不证明刷新/跨设备首页项目指针恢复。
- 独立 PostgreSQL 实例已正常停止，只保留临时合成数据与日志；业务服务与数据库未变更。
