# 画布菜单与删除确认验收 · 2026-10-30

来源为已登录即梦资产画布页面与用户的两张截图。用户补充“即梦中没有的功能是过度设计”，本轮范围由简化删除清单扩展为统一我的画布入口、精简画布菜单及删除确认。对应需求07 UI-PC-DELETE-01、设计13§7.20、PRD06与Plan15。

## 设计先行

Figma uLqmeRWuQuzfrp9xOYg6FT：共享CanvasDeleteConfirmation 3302:66353，6个桌面/手机/主画布/处理中/失败状态；CanvasActionsMenu 3302:66354。桌面确认2809:48435、手机确认2827:43117，旧我的画布1361:627更换为统一画布设计；画布/历史画布原菜单2809:49591、2809:49807替换为三项共享菜单，旧复制项目图层已移除并回读为0。使用原生可编辑组件、文字、Lucide原始向量；没有用整页截图替代设计。首次原生写入遇到不支持的FRAME description和私有pluginData写入，事务回滚后改为仅原生布局，最终写入和截图读回成功；确认框定位已修复为桌面绝对定位并确认可见。

## 实现

- /projects改为GET兼容导航，307相对Location到/assets?view=canvas。旧state=archived/deleted不再打开另一套管理页面；保留q搜索。侧栏与首页“全部”直接到统一列表，首页历史画布直接进入legacy。资产页不再重复首页专属快捷导航。
- 列表复用AssetCanvasList及真实listAssetCanvases/applyCanvasAssetCommand，不新增收藏接口。菜单只含重命名、收藏/取消收藏、删除；图标为16px Pencil/Star/Trash2。移除复制菜单、取项目和复制弹窗调用。旧项目管理客户端模块不再由公开/projects路由挂载；后台归档/恢复/复制协议没有改写。
- 单项、批量、短剧共用CanvasDeleteConfirmation：确认删除、短说明、取消、删除、关闭图标，不接收或渲染目标名称清单。默认聚焦取消，遮罩不关闭，空闲Escape可关闭，取消后恢复入口焦点；处理中/未知回执禁止关闭，保留原命令重试。
- 即梦说明为永久删除；当前主画布真实行为是整个所属项目进入30天回收期，因此本地说明保留该真实影响和期限，不伪装立即永久删除。

## 验证

- 5文件65项定向回归通过：projects/route、canvas-delete-confirmation、basic-layout、asset-workspace、theme-policy。覆盖旧链接迁移、相对Location避免Docker内部主机泄露、取消不提交、默认焦点、提交锁定、未知命令原幂等键重试、主画布项目CAS及既有资产操作。
- Next typegen成功；删除本机过期.next/dev/types生成缓存后，使用新生成.next/types执行tsc --noEmit通过。曾因旧validator仍引用已迁移page而失败，已更新缓存，未放宽类型配置。定向eslint、13个TSX/TS/CSS文件Prettier、git diff --check通过。没有运行全仓回归或生产构建。
- 初始异步Page redirect触发开发模式Performance.measure负时间戳。改为GET路由后，实际HTTP307与浏览器均验证正常；相对Location避免0.0.0.0:3000进入公开地址。最终真实页面无Runtime TypeError及Next Issue按钮。
- 真实localhost:1122：/projects最终地址/assets?view=canvas；菜单严格三项；普通确认480×178，背景rgb26、圆角16、padding24，标题16/24、说明14/22；取消/删除均58×36、间距12。与即梦普通确认几何匹配。主画布长说明自动增高。
- 手机390：358×208；320：288×208，均左右16，取消/删除/关闭均44高，没有横向溢出。测试后恢复默认视口与普通列表状态。
- 单项与批量都无ul/ol名称清单；取消、遮罩保持、Escape、焦点恢复已实页检查。没有删除现有用户画布，删除写入/失败/重试边界证据来自fixture回归，不宣称真实删除数据库回归。
- 收藏真实服务验收：对既有附加画布执行收藏，命令完成后正式列表回读为取消收藏；再取消收藏并回读aria-pressed=false，已恢复原状态。未修改名称或画布内容。

截图、对照图、Figma图及逐文件工作区清单位于项目根output/canvas-confirm-2026-10-30。新代码未提交或推送；先前导航main检查点仍为114c548b，其他并行工作保持在工作区。
