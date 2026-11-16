# Huobao 与 CineGen：制作流程、Agent 与交付机制独立复核

调查日期：2026-11-12（Asia/Shanghai）。本轮只读获取仓库与固定提交源码，未安装依赖、运行参考产品、运行上游测试或调用供应商。GitHub 插件查询最新提交并获取关键源码，完整浅克隆用于交叉阅读。采用 reverse-engineer-anything 技能的证据与追踪规则；完整源码分析不需要 REA 二进制工具，未启动其 readiness 或修改连接配置。

## 固定版本与范围

| 项目 | 提交与时间（北京时间） | 本地只读副本 | 实际形态 |
| --- | --- | --- | --- |
| chatfire-AI/huobao-drama | b48e6b2312166e25b1ff141832d2e82e5d63c142，2026-11-05 18:27:28 | /tmp/glowloom-research-20261112-production/huobao | 当前 master 为 v4.0.11 桌面编排，Node/Hono/Mastra/Drizzle/SQLite；不能用历史 Go 文章描述当前版本 |
| UllrAI/CineGen-ShortDrama | f823501fc9c9f0daace80c8018db39b1acef94c3，2026-09-25 20:53:13 | /tmp/glowloom-research-20261112-production/cinegen | React/Vite 浏览器工作台，直接调用 Gemini/Veo，项目存 IndexedDB；公开托管 AniKuku 的服务实现不在此仓库 |

两项目本轮最新 SHA 均已通过 GitHub 插件和本地 git rev-parse 交叉确认。Huobao 与旧报告 SHA 一致不代表运行质量已经验收；这里只是源码基线未变化。

## Huobao：有实际生成与原任务恢复，Agent 主要承担限定制作工序

### 实际制作调用链

用户在分集页面分别触发改写、资产提取、分镜或提示词操作 → /agent/:type/chat → Mastra 按 type 取得专业 Agent，带 dramaId/episodeId/model/config 的 RequestContext，最多 20 个工具步骤 → 工具读写剧本、角色/场景/道具、分镜 → /tasks 创建图像或视频 sys_task → provider adapter 生成请求/查询 → 下载、本地文件、业务表回写 → 用户选择镜头合并 → FFmpeg 实际产生 MP4。

1. **四个专业 Agent，并不等于主 Agent 自主分工。** `backend/src/agents/index.ts:339-390` 注册 script_rewriter、extractor、storyboard_breaker、prompt_generator，各自绑定有限工具。`routes/agent.ts:31-107` 是按请求类型直接选择 Agent、generate 后返回；`frontend/app/composables/useAgent.ts:10-29` 收到请求成功即显示完成并刷新。`services/extraction.ts`、`video-prompts.ts` 等也由确定性服务直接选择专用 Agent。定向检索 backend/src、frontend/app 中 mcp/MCP/subagent/delegat/getAgent/generate，没有找到主 Agent 调用子 Agent 的委托工具、层级执行树或动态任务分解。该负面结论只限本次当前代码路径。[H1]

2. **Skill 有真实发现和加载，但主要是指令注入。** Workspace 递归发现含 SKILL.md 的目录，按 AGENT_SKILL_MAP 的目录前缀给每个专业 Agent 选择技能；loadAgentSkills 读取完整 instructions，支持语言变体，然后与 prompt、声音规则、语言指令拼接。新建子目录可被动态发现。它证明 Skill 文本实际进入 Agent，不证明有独立 Skill 执行回执、版本冻结、事务或权限边界。[H2]

3. **制作模型是“一个分镜段落对应一个视频任务”。** 默认分镜 prompt 将段落约束为 8–15 秒、内含 2–4 子镜头；这段时长/对白密度/叙事节拍要求主要是 prompt。保存工具实际只校验基础 Zod 字段、声音引用和同剧角色/场景/道具归属。duration 是可选 number，没有在此 schema 强制 8–15 秒。分镜按 shot_number upsert，重叠保存可复用已有编号；整集重做第一批 replace_existing 直接删除旧分镜再写批次，后续追加。保存前先检查声音引用是明确改进，但该路径没有覆盖全批次的数据库事务与完整性证明；中途停止可能留下部分新分镜。不能把 prompt 的“全部保存后才结束”视为服务端完整性门禁。[H4]

### 任务稳定性与局部重做

4. **任务状态比普通 HTTP 生成调用完整。** sys_task 保存 provider、configId、configBaseUrl、taskId、resultUrl、params、localPath、stage、errorCode；生命周期含 submitting/polling/transferring/downloading/saving/completed。generateVideo 使用进程内 Set 防重复点击，并读同分镜最新任务：processing 阻止第二次提交，failed 默认要求先恢复，forceNew 才允许明确重新生成。它是桌面单进程范围内的防重，不是跨节点分布式唯一准入或统一费用账本。

5. **原任务恢复做到了重要的分层。** 首次请求若取得网关任务响应头，立即保存任务 ID；没有任务 ID/结果地址的超时、断连、5xx 等可标记 submission_unknown。recoverVideoTask 无 ID/结果时拒绝自动重提；有 resultUrl 就先恢复下载，有 taskId 则查询原任务。原配置存在且 provider/baseUrl 一致才用；过期签名链接可通过原任务刷新。Firemux 的特定交付错误只调用 /recover 和查询，不调用生成。启动 resumeInterruptedTasks 只自动恢复 processing 视频中有原 ID/结果的任务，无身份的视频停到 unknown，图像中断走 failed/interrupted。[H3]

6. **旧结果覆盖防护存在，但范围不一致。** 视频 handleVideoComplete 先保存原始 URL、下载，再在事务内完成任务与业务回写；若同分镜已存在更大 sys_task.id 的视频任务，旧任务恢复不会覆盖分镜当前 videoUrl。这里“新”的判据是更大任务 ID，甚至新任务未成功也会挡旧结果，不是显式已采用版本。图片 writeBackImageAssets 直接写 first/last/composed/角色/场景/道具图片，未看到同等级迟到结果或采用版本栅栏。sys_task 历史可保留，storyboard 当前图/视频字段可被替换，没有在检索的 schema 中找到不可变 take/采用快照、下游依赖图、局部重做影响集或资产内容摘要冻结。

7. **“删除”不等于取消上游。** tasks 路由拒绝删除 processing 任务；已结束任务可删数据库行。generation.ts 未找到 AbortController 或上游 cancel 路径。重启恢复逻辑覆盖视频生成/下载，不覆盖 Agent generate 的中途工具步骤或 FFmpeg 合并重启续跑。合并进程崩溃后留存 processing 是否被其他通路接管，本轮未找到。

### 实际交付与质量边界

8. **确有 FFmpeg 成片实现，同时明确允许缺镜头。** mergeEpisodeVideos 依镜号选素材，支持传入 selected storyboard IDs，并过滤没有 videoUrl/composedVideoUrl 的镜头；只要至少一条视频就能合并。合并前检查 FFmpeg/FFprobe 可用与文件存在；实际 concat 后重编码 H264/AAC，探测时长、生成海报、更新 videoMerges 与 episode.videoUrl。由此可以确认“有真实 MP4 输出代码”；不能据 completed 推定整集镜头完整，不能据“有文件”推定内容连贯、画质、角色一致、口型或音画同步合格。当前 ffmpeg-merge 路径没有 EDL/XML、多轨音频、字幕轨或制作依赖失效机制。[H5]

9. **下载校验与内容质量不同。** video-download.ts 有限流/超时/字节上限、临时文件、MP4/WebM 头判断、退避、原子 rename 和错误脱敏。这是传输/格式防护，不是完整解码验收。video-delivery.test.ts 的视频样本是 ftyp 头加补零的 Buffer，不能证明真实 FFmpeg 可解码或媒体完整。video-recovery.test.ts 有 SQLite/本地 HTTP 契约测试，明确检查并发恢复不新增生成、重启未知提交、原配置变化、过期链接和迟到结果不覆盖新视频；本轮只审查测试源码，没有执行。[H6]

10. **声音/角色一致性方法存在，自动验收证据不足。** 项目角色、场景、道具关联和 reference audio/image 会进入生成参数；prompt 工具对声音引用做关系校验。影片的一致性更多依赖固定参考素材、风格与语言/声音指令。定向检查 agents/tools、services 未看到视觉身份评分、剧本逐句覆盖率、跨镜轴线、对白与时长的确定性判定，或质检失败后限定预算重做的闭环。不能把“quality=68”的图像压缩参数误读为内容质检评分。

11. **CI 与费用边界。** backend/tests 有 22 个测试文件，含不少 .test.mjs 结构/文本断言；backend/package.json 没有 test 脚本，当前检出的仓库没有 .github 工作流目录；主调查侧对本 SHA 查询公开 Actions runs 返回 0。该结果不排除外部 CI、其他分支或历史版本，不能概括为整个项目永远没有 CI。费用提示可说明“重生成可能再次收费”，在该桌面后端检索未发现预算预占、统一报价批准、供应商账单对账或跨租户权限账本。docs/video-delivery-implementation.md 对 Firemux 的账单/交付队列描述属于相邻服务集成说明，不能当成当前仓库已实现 Firemux 后端及生产验收。

12. **许可证。** 仓库 LICENSE 和 backend/package.json 标示 CC BY-NC-SA 4.0。它带非商业和相同方式共享约束，不能统称可不受限制商用的 MIT 项目；本轮只做研究，不引入代码。[H7]

## CineGen：可操作的镜头生成工作台，完整交付仍有明显空缺

### 实际制作链与能力

1. **固定四阶段 UI。** App 用 script/assets/director/export 枚举切换组件；StageScript 先 parseScriptToData，再 generateShotList，然后更新项目。阶段是用户切换状态，不是持久化 workflow 自动推进。Gemini 调用用角色化 prompt 和响应 schema；没有执行型 Agent 框架依赖。对全部 24 个仓库文件及 dependencies 检索未找到主子 Agent、Skill 文件/MCP 客户端/Tool 授权或持久化工作流引擎；不能把 prompt 的“professional cinematographer”叫作子 Agent。[C1]

2. **角色变体和场景参考真正参与生成。** ProjectState 有 characters/scenes、角色 variations、每镜 characterVariations、start/end keyframes、interval。StageDirector 按场景图优先、角色变体优先于基础角色图收集参考图，生成单镜首尾帧；视频调用使用该镜起始帧，可选结束帧，固定 Veo 模型/720p/16:9，返回视频 URL。批量生图逐镜执行，间隔 3 秒，缺失图可补齐；重做全部首帧需 confirm。这是具体制作方法，不是跨任务质量证明。[C2]

3. **单场景失败会静默减少交付镜头。** generateShotList 将文本按 scene 分开生成，一次一场景；单场景 catch 后 console.error 并 return []，外层仍拼接其他场景并返回成功镜头集。StageScript 随后写入 scriptData/shots 并结束解析，没有把失败场景作为未完成义务保留。sceneId 被代码强制设回原场景，能修正关系字段，但不能恢复遗漏剧情。这是值得明确标记的交付完整性缺口。[C1]

4. **生成操作的恢复只在页面内。** generateVideo 取得 SDK operation 后每 5 秒轮询直到 done，operation 只在局部变量；ProjectState 没有供应商 operation ID/账单/提交凭据。轮询没有绝对截止或恢复调度；刷新/关闭页面不能从项目状态查回原始 operation。retryOperation 主要对 429 做有限重试，不能据此推定 submit-unknown 安全重放。视频 URL 直接附带 runtime API key，未变成受控本地媒体归档，也未验证 URL 长期可用。[C1]

5. **有局部编辑，没有下游失效保证。** updateShot 按镜头 ID 替换，生成首/尾帧后保留原 interval；改变角色变体也只更新映射。旧视频可以与新首帧/参考图同时存在，没有 stale 状态、上游 revision、采用快照/候选集或重新导出失效计算。UI processingState 是组件内单个操作标记，而非持久化任务状态。[C2]

6. **“成片导出”按钮尚未实现动作。** StageExport 的完成比例只是 interval.videoUrl 是否存在；Download Master(.mp4) 与 Export EDL/XML 按钮没有 onClick 或 href；Source Assets/Share Project/Render Logs 卡片也没有处理器。在本 SHA 的 24 个文件和该导出路径中未找到拼接、真正 master 文件或 EDL/XML 文件生成代码；这不代表其他版本或托管服务永远没有导出能力。公开 README 当前也将主要链条写到 script→assets→keyframes→video，因此不能把界面标题“成片与导出”补全成已实现的交付链。[C3]

7. **本地保存不等于任务持久化。** IndexedDB 存整个项目状态，App 约 1 秒 debounce 自动保存。storageService 的 saveProjectToDB 在 objectStore.put request.onsuccess 就 resolve，未等待 tx.oncomplete：保存请求成功与事务完全提交的证据仍需区分。运行中的供应商 operation 未进入该对象，因此保存项目不能恢复原生成。[C4]

8. **验证与许可边界。** 24 个仓库文件内未发现测试目录/测试文件/CI 工作流，package.json 只有 dev/build/preview，没有 test/typecheck 脚本。主调查侧对本 SHA 查询公开 Actions runs 返回 0；外部或其他分支 CI 未作结论。未运行该应用、构建、真实 Gemini/Veo 调用或验证视频质量。license.md 为自定义 AniKuku Community License v1.0，不是 MIT；其开头授予个人、教育、研究和非商业用途，署名条款要求可见链接，第 4 节又规定商业去署名、闭源无可见归因等需商业许可。对商业保留署名的完整授权范围不作超出条文的结论；正式复用须单独确认。[C5]

## 交付质量调查状态（不导出架构推荐）

| 问题 | Huobao | CineGen |
| --- | --- | --- |
| 剧本→资产→分镜的具体代码 | 已回答：专业工序 Agent 与 DB 工具 | 已回答：固定组件与 Gemini 函数 |
| 主 Agent 自主委托子 Agent/Skill/MCP | 部分：专业 Agent 与 Skill 注入可证；未找到委托/MCP | 有界未发现；不具同等实现证据 |
| 原任务恢复、未知提交与再次计费 | 已回答视频路径；图片/Agent/合并恢复有缺口 | 页面内轮询可证，持久恢复未找到 |
| 局部重做与旧版本失效 | 部分：视频迟到结果有 ID 栅栏，其他对象无完整依赖图 | 局部字段更新可证，下游失效缺失 |
| 实际成片和导出完整性 | 有 MP4 合并，允许缺镜头；不保证整集完整 | 导出 UI 处理器缺失 |
| 画面/角色/声音/叙事质量达标 | 未运行验证；引用关系校验与 prompt 约束不能替代成片质检 | 未运行验证；引用图与 schema 不能替代成片质检 |
| 生产部署与外部 CI 终态 | 所选 SHA 公开 Actions runs 返回 0，外部 CI/部署未验证 | 所选 SHA 公开 Actions runs 返回 0，外部 CI/部署未验证 |

因此，本分报告完成了约定源码范围的制作与恢复机制追踪，但真实供应商成功率、艺术质量、长片连续性、跨设备恢复和商业服务稳定性仍未回答，不能宣称参考平台已经通过这些验收。

## 12 个固定提交核心证据

[H1]: https://github.com/chatfire-AI/huobao-drama/blob/b48e6b2312166e25b1ff141832d2e82e5d63c142/backend/src/agents/index.ts#L339-L390
[H2]: https://github.com/chatfire-AI/huobao-drama/blob/b48e6b2312166e25b1ff141832d2e82e5d63c142/backend/src/agents/skills.ts#L39-L149
[H3]: https://github.com/chatfire-AI/huobao-drama/blob/b48e6b2312166e25b1ff141832d2e82e5d63c142/backend/src/services/generation.ts#L509-L595
[H4]: https://github.com/chatfire-AI/huobao-drama/blob/b48e6b2312166e25b1ff141832d2e82e5d63c142/backend/src/agents/tools/storyboard-tools.ts#L241-L327
[H5]: https://github.com/chatfire-AI/huobao-drama/blob/b48e6b2312166e25b1ff141832d2e82e5d63c142/backend/src/services/ffmpeg-merge.ts#L26-L147
[H6]: https://github.com/chatfire-AI/huobao-drama/blob/b48e6b2312166e25b1ff141832d2e82e5d63c142/backend/tests/video-recovery.test.ts#L44-L86
[H7]: https://github.com/chatfire-AI/huobao-drama/blob/b48e6b2312166e25b1ff141832d2e82e5d63c142/LICENSE#L1
[C1]: https://github.com/UllrAI/CineGen-ShortDrama/blob/f823501fc9c9f0daace80c8018db39b1acef94c3/services/geminiService.ts#L157-L421
[C2]: https://github.com/UllrAI/CineGen-ShortDrama/blob/f823501fc9c9f0daace80c8018db39b1acef94c3/components/StageDirector.tsx#L26-L149
[C3]: https://github.com/UllrAI/CineGen-ShortDrama/blob/f823501fc9c9f0daace80c8018db39b1acef94c3/components/StageExport.tsx#L122-L164
[C4]: https://github.com/UllrAI/CineGen-ShortDrama/blob/f823501fc9c9f0daace80c8018db39b1acef94c3/services/storageService.ts#L21-L30
[C5]: https://github.com/UllrAI/CineGen-ShortDrama/blob/f823501fc9c9f0daace80c8018db39b1acef94c3/license.md#L1-L56
