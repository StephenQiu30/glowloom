# 音频轨边缘创建、清理与播放头吸附回归

2026-10-29 11:59 Asia/Shanghai。补充 TLA-13、TLA-23、TLA-25～28、TLA-48。此前证据中的“第二音轨入口未发现”“播放头候选未知”是各自时点记录；本轮主验收已提供新的即梦实测观察，因此补齐如下行为。

## 接口与行为

- `move.newAudioTrack?: { afterTrackID: string }`：音频片段拖至轨道边缘时，按显式锚点创建新音频轨并完成移动，一次命令保留绝对起点、时长及源入点。锚点允许主视觉轨/音频轨；过时锚点、非音频片段、同时指定 `trackID`、锁定源轨均拒绝。
- 音频移动后清除空的未锁定音轨；删除音频轨最后片段也清理该轨。主视觉轨、受锁定保护的空轨和导入资产保留。单次 Yjs undo/redo 同时恢复/删除片段与轨道。
- 工作区回归验证轨底6px创建、25px取消、只读不启动创建；不存在常驻“添加音频轨”按钮。DOM边界为测试布局，真实页面像素由主浏览器验收另记。
- `move.playhead?: number`：吸附开启时，播放头与邻居边界共同参与最近距离匹配，候选去重。实测对应回归为原3秒片段拖到2.112秒、播放头2秒：开启吸附得到2秒，关闭保持2.112秒；超出阈值不吸附。相等距离候选的即梦优先级仍未观测，不能声称该竞争规则已经像素/行为签收。

## 验证

- `pnpm exec vitest run tests/unit/components/canvas/timeline tests/unit/components/canvas/yjs-live-session.test.ts`：10文件、147项通过。见 completion-audio-lane-snap-tests.log。
- 变化范围 model/session/workspace测试 ESLint：无诊断。
- `pnpm exec tsc --noEmit`：本轮无诊断。

具体新增测试在 timeline-model.test.ts、timeline-workspace.test.tsx、yjs-live-session.test.ts。模型/组件/会话测试不等于服务端真只读账号或真实浏览器验收；真实证据由总验收回执引用。没有提交、推送或修改无关功能。
