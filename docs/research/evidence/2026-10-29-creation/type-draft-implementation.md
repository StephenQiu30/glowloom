# 创作类型草稿与专属表单实施证据

日期：2026-10-29。来源：[设计29](../../../design/29-创作一致性与公共组件设计.md)→[PRD16](../../../prd/16-创作一致性与公共组件.md)→[Plan25](../../../plan/25-创作一致性与公共组件实施计划.md)。完整七类服务及70项范围不因本组调整而缩减；进度与下一步归BACKLOG。

## Figma先行

- 共享输入组件 `3171:60692`；[草稿八态](https://www.figma.com/design/uLqmeRWuQuzfrp9xOYg6FT?node-id=3171-60732)，含类型往返、身份变化、保存未确认、画布独立和页面内文字恢复。
- [七类桌面+手机表单](https://www.figma.com/design/uLqmeRWuQuzfrp9xOYg6FT?node-id=3179-56558)：Agent `3179:56560`、图片 `3179:56703`、视频 `3179:56864`、音乐 `3179:57043`、音频 `3179:57186`、数字人 `3179:57311`、动作模仿 `3179:57500`；各类型一桌面一390px手机，共14个表单状态。
- 音频参数弹层 `3183:57787`，限定模型提供的默认/已保存音色、语速、音量与单结果，不假造完整音色库。
- 复用既有Button实例、Geist和颜色变量。首次截图发现按钮实例旧文本覆盖，已回写属性与实际文本并读回；音频最终截图核对通过。草稿恢复按钮补成真实组件，画布独立示例移除生成模型/参数。
- Figma截图接口间歇429，后续音频截图恢复成功；全七类整板最终截图仍返回429，不能声称整板最终视觉验收通过。已有创建回执、组件文本读回与音频/草稿局部截图。设计状态并不证明供应商执行。

## 代码边界

- `home-creation.tsx`：actor/space所有者重建；七类与canvas独立PromptDraft；新项目用目标类型内容播种；按project/type保留draftID；先确认保存再切换。引用本身可进入图片/视频项目，仍受编辑器后续校验。
- `home-page.tsx`移除共享字符串；`home-prompt.tsx`传递完整草稿，canonical内容相等时不回传，防止聚焦/选择光标吞掉恢复候选。
- `home-draft-recovery.tsx`：类型化三段contextKey，旧home-input仅归图片；恢复只保留文字与换行；迟到响应不覆盖新输入/新作用域；写失败不删除原分支。
- `draft-save.ts`只扩展空间草稿允许的类型化key，保持原身份/空间权限复查。当前主分支的journal已经是页面内存；本组没有引入本地持久存储。页面重挂载候选≠刷新/跨设备恢复，CR-016仍未完成。
- `creation-types.ts`集中名称/提示/可执行类型；`creation-type-fields.tsx`区分音乐歌曲/纯音乐与智能时长、数字人动作描述、动作模仿双槽与禁止正文。尚无专用服务的入口明确说明未开放，不上传或发起替代生成。
- 音频接入既有`audio.generate/text2audio`项目链路；隐藏普通引用/@入口，新朗读草稿不追加视觉偏好后缀；标题、模式名与无模型状态匹配。完整音色库、全能配音仍未实现。
- `generation-parameters.tsx`时长秒数显示/编辑、毫秒传输；数值和滑块共享模型边界；枚举只转换显示，保留协议值。默认音色不让创作者手填内部ID。
- `rich-generation.tsx`与`generation-dialog.tsx`含其他任务已有改动；本组只增加音频文案/输入边界等，不认领其余画布助手或导出修改。

## 测试与检查

执行目录为`frontend`：

```sh
pnpm exec vitest run tests/unit/app/_components/home-creation.test.tsx tests/unit/app/_components/home-subjects.test.tsx tests/unit/app/_components/home-page.test.tsx tests/unit/app/_components/home-prompt.test.tsx tests/unit/app/_components/home-draft-recovery.test.tsx tests/unit/components/generation/generation-parameters.test.tsx tests/unit/components/generation/generation-dialog.test.tsx tests/unit/components/generation/draft-save.test.ts
pnpm exec tsc --noEmit
pnpm exec next build
pnpm exec vitest run
```

- 草稿先Red：4失败/6通过；隔离首轮Green17通过。类型专属行为先Red：5失败/42通过。独立审查发现时长枚举重复换算、仅光标变化取消恢复两项P2，均修正并补测试。
- 最终定向8文件79项通过：包含六类文字往返、动作禁正文、主体引用不串类型、账号重建、16项恢复竞态、音频来源草稿报价及不自动确认、时长枚举与默认音色。
- 修改源码/测试ESLint通过；最终tsc与生产构建退出0。过程中tsc先见并行画布/主体错误，后在本组新增测试发现Testing Library不接受`exact`，已修正；最终并行错误亦已消失。
- 全量207文件1372项：1370通过、2失败。失败为`theme-policy.test.ts`三处画布literal color，以及`use-subject-sources.test.ts`分页返回时page为空。它们不在本组修改范围，保留原失败日志，不以定向通过覆盖全量失败。
- 修改路径`git diff --check`通过。未重新请求付费供应商，未运行后端测试，因为本组未修改后端代码。

## 浏览器实测

实际`http://localhost:1122/`，登录态。未注入mock，没有读取密钥；未上传、报价确认、付费生成、发布或删除内容。

1. 图片输入“草稿隔离验收：图片海报”，切视频为空；视频输入后返回图片仍为原文字。
2. 音频初次为空，没有添加参考/@；填朗读原文后选择已有验收项目，进入真实音频项目编辑器。切换动作模仿等类型再返回，朗读文字保留。
3. 音频模型弹层实际显示“暂无匹配的可用模型”；因此本次不接受真实合成或计费结果。选择项目产生的音频验收草稿保留于已有验收项目，没有动其他内容。
4. 动作模仿显示角色图片/动作视频槽位和专用模式说明，无创作正文输入；数字人台词与动作分离，动作“轻轻挥手”切音乐后再返回仍保留。
5. 390×844验证音乐纯音乐选中、智能时长与数字人字段；document.scrollWidth=390，innerWidth=390，无横向页面溢出。结束恢复原视口。
6. 浏览器中途遇到并行时间线文件语法错误overlay及主体CSS热更新错误；等待源文件恢复后继续验证，未覆盖并行文件。

本地截图/日志：工作区外`/Users/stephenqiu/Desktop/StephenQiu/Glowloom/output/jimeng-creation-2026-10-29/`，含`audio-project-browser.png`、`audio-no-model-browser.png`、`motion-desktop-browser.png`、`avatar-mobile-browser.png`、`music-mobile-browser.png`、`creation-audio-design-final.png`及完整失败/成功日志。

## 供应商与Git边界

[供应商调研14](../../../research/14-音乐数字人与动作模仿供应商接口调研.md)已找到火山三类正式API与腾讯/阿里备选，但本组未实现三类服务适配，亦未确认账号开通。Agent独立会话、技能合同、高级视频模式和完整音色库仍按矩阵逐项落实。

本轮未暂存、提交或推送、未部署或重启服务。前序仅独立研究8文件已提交推送`a92732f1`；当前HEAD是并行推进后的`5c670d1d`，不将它归为本轮提交。全部遗留修改逐项记录在工作区外`retained-worktree.md`，保留并行工作。
