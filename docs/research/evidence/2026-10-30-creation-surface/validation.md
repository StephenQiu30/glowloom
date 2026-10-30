# 首页生成与画布样式验收 · 2026-10-30

范围为 CR-072：生成/画布切换条、标题、首页输入表面、画布卡片及图标。来源为同一 Chrome 会话中的即梦已登录首页，无提交生成或消费操作。先修改 Figma，再落实首页代码。

导航检查点已提交并推送 main：`114c548bdb4130756df674f011a0c89c229c666b`，2026-10-30 远端 refs/heads/main 一致；CI completed/success：[运行41024237329](https://github.com/StephenQiu30/glowloom/actions/runs/41024237329)。本轮样式尚未提交。

Figma 文件 `uLqmeRWuQuzfrp9xOYg6FT`：桌面生成1361:26、桌面画布1604:21114、390手机生成3292:66229、390手机画布3292:66302；切换组件3289:66045、输入表面3290:66028、画布卡片3290:66067，24px原生图标3290:66055。设计变量及共享渐变样式已建立，最终截图均回读。图层为原生可编辑文字、向量与实例。

## 验证结果

- `pnpm exec vitest run tests/unit/app/_components/home-creation.test.tsx tests/unit/app/_components/home-page.test.tsx tests/unit/components/layout/recent-canvases.test.tsx tests/unit/theme-policy.test.ts`：4文件23项通过。包括草稿隔离、未确认切换保护、画布往返、账号切换、数字人台词/音乐选项及真实项目列表边界的单元回归。这是单元回归，不等于供应商接入验收。
- 后续仅校正画布输入addon内边距，`pnpm exec vitest run tests/unit/theme-policy.test.ts`再验3项通过。
- `pnpm exec tsc --noEmit`、定向eslint（home-creation/recent-canvases/product-navigation-icon）、上述4文件Prettier检查、定向git diff --check均通过。未运行全仓测试或完整生产构建。
- 真实localhost:1122已登录页面：1463px桌面生成标题y68、切换320×40、选项157×36、输入1014×160，圆角24；画布卡片247.5×139.21875、16:9，想法输入814×70、圆角20；选中渐变从暖色至浅青，输入统一26→42灰阶72%透明渐变。
- 390与320宽实页：七类创作切换与未选项目字段均未发现整页横向溢出或控件超出视口；画布两列，选项44px高。宽度测试后恢复默认视口和图片生成。
- 本轮没有填写用户提示词，没有新建画布，没有提交付费生成，也没有修改会话安全。草稿保存逻辑保持既有代码；非空草稿隔离证据来自上述单元回归，不宣称本轮完成真实数据库回归。
- 首页无顶栏搜索、无企业服务；全部画布入口href为/projects，卡片使用真实查询的project/canvas链接。未选择项目的页面字段验收不能替代选定模型后的供应商参数验收。

## 证据

项目根目录外的 `output/creation-surface-2026-10-30/` 保存：jimeng-generate.jpg、jimeng-canvas.jpg、desktop-generate.jpg、desktop-canvas.jpg、mobile-generate.jpg、mobile-canvas.jpg、narrow-canvas.jpg、四张figma截图、source样式JSON、scoped-frontend.patch、starting-status.txt、worktree-manifest.md。本目录保存桌面/手机尺寸、320/390七类型布局及SVG摘要。截图原图不引入仓库。
