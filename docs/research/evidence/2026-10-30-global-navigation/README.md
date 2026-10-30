# CR-074 现有页面全局导航验收

用户确认：“先完成现有页面的全局导航同步”。顺序为既有代码提交推送 → 实测即梦 → 共享Figma → 代码 → 实页验证。首页无搜索、当前无企业服务两个既有约束继续生效。对话业务、独立短剧工作台、侧栏置顶均未扩展。

Figma文件 [织光](https://www.figma.com/design/uLqmeRWuQuzfrp9xOYg6FT?node-id=3316-67116)：共享侧栏3280:64553、共享Header620:24416、分组3316:66706、四页3316:67116、图标3314:66616、折叠/手机3317:65920。先修改组件与实例，再实现代码。最后回读修复了折叠箭头旋转后被裁切的问题。

来源为当前真实Jimeng首页、探索、资产、主体；source-pages.json保存可见工具差异，source-home.png为来源截图。normalized-icons.json是原始DOM可见图标的20px规范版本。代码资源由完整Figma图标节点原尺寸导出；探索/资产/主体选中态原图来自来源Lottie帧，保留在可复用图标组件中，不把整页截图导入设计。主体选中图标使用luminance mask保留深色镂空。

## 实现与范围

ProductNavigation是普通页面唯一组合入口，统一侧栏与顶栏。页级工具仍走现有toolbar/actions槽，账号节点保持连续。分组采用既有shadcn/Radix Collapsible，收起仅隐藏内容，最近列表保留正式服务读取；标题始终关联内容节点。侧栏取真实最近项目的前5项、首页卡片前3项，完整列表经“全部”进入资产画布。资产主画布重命名/删除仅在回执明确成功后发布既有project invalidation，未确认或失败不发布。

未将来源短剧工作室映射到本地短剧列表，未虚构会话历史或置顶服务。保留当前真实“对话功能待开放”状态。企业服务按既有用户要求不显示。保留本产品品牌；其余同范围布局、图标、配色按来源核对。

## 验证

- tests.log：8文件79项定向通过，包括跨四页节点身份、图标选中态、分组独立/收起保留、资产成功/未知/失败失效门禁、7项列表限制5项、首页3卡片、既有页级工具等。
- tsc.log为空且命令退出0：全工作区TypeScript通过；eslint.log、prettier.log为本次改动范围通过。typegen.log、build.log为官方typegen与完整生产构建通过；full-eslint.log为全量ESLint通过。
- full-prettier.log：全量格式检查失败，17个非本轮文件待处理（导演台资源/组件、既有UI原语、浏览器夹具和API测试等）。未格式化这些并行文件。scoped-diff-check.log为本轮路径检查通过；全工作区diff-check另有导演台资源许可文本和其他研究文档的空白问题，不宣称全量通过。
- full-tests.log：全量274文件、1979项，1975通过、4失败（2文件）。business-storage-policy发现并行canvas-yjs-pending-journal.ts持久化，advanced-source-combinations有3项错误提示文案断言不同；均不属于本轮导航文件，未改写或跳过。
- 初次新增回归发现折叠标题没有aria-controls，补关联后通过；初次类型检查发现测试误用了exact选项，移除后通过。未跳过失败。
- runtime-desktop.json与home/explore/assets/subjects-desktop.png：实际四页均仅一套侧栏/顶栏，主导航216×36，图标20；每页唯一aria-current，账号同一DOM ID，首页专属工具仅首页出现，无横向溢出。
- sections-collapsed-desktop.png：分组独立收起；切页及整栏收起/展开保留状态。全部实际进入 /assets?view=canvas。
- mobile-390.png、mobile-320.png：抽屉无横向溢出，分组/新建/全部44px触达；已修复Portal不继承导航色值，文字white60%、底板white12%。退出时恢复原窗口尺寸和分组展开状态。
- 真实本地服务仅剩2个最近项目，故五项限制由fixture验证，未伪造浏览器数据。没有新增模型调用、数据库增量或执行真实删除；回执触发刷新属于定向回归验证，不冒充真实删除落库证据。页面最终读取无新增console warn/error。

## Git

之前完成的首页/画布管理已以a4f101ac70d2dccdfc2bba47ca25ff1be750537e提交main并推送，远端核对一致；[CI终态成功](https://github.com/StephenQiu30/glowloom/actions/runs/41032966890)。本导航增量在实施验收后获得用户明确授权提交main并推送。共享工作区含大量其他并行变更；不做广泛stage、reset或clean。asset-canvas-list.tsx和对应测试另有并行反馈改动，本轮只追加成功项目变更后的失效通知和测试断言，不归属其他反馈改动。

交付检查：以a4f101ac为基线，使用独立索引按路径和共享文件片段组装提交快照。过程日志在本地Glowloom/output/global-navigation-2026-10-30/delivery留存，不将运行日志、缓存或构建产物加入Git。实际工作区与独立候选的检查结果分别记录。

独立提交快照复验：233文件1659项全量通过，官方typegen、完整生产构建、全量ESLint和修改代码格式检查通过。快照不含并行画布恢复/反馈增量；保留此前共享工作区4项失败与17文件格式问题。首次并行启动pnpm exec导致本地依赖自动重装目录竞争，初始失败日志保留在本地delivery目录；依赖稳定后使用同版本已安装工具重新完成检查，未跳过测试。
