# 共享导航样式验证（2026-10-30）

依据：需求16 CR-071 → 设计29 §7.12 → PRD16 CR-071 → Plan25。用户补充确认：首页无顶栏搜索；织光没有企业服务，不能将团队管理改名为企业服务。

## 设计与资源

用户指定的即梦Chrome页面与两张截图为来源。只读量取：240px侧栏；主导航216×36、20px图标、4px间距、8px圆角；折叠图标16px；最近画布24px底板/14px图标、36px行高；账号区36px高，通知28px按钮内16px图标。

先修改[Figma导航预览](https://www.figma.com/design/uLqmeRWuQuzfrp9xOYg6FT?node-id=3280-64554)，再实现代码。ProductSider组件集3280:64553包含创作/探索/资产/主体四种选中态，替换原页面实例；ProductAccountActions3165:58374四变体原位修改；ProductUtilityNavigation3280:64122复用于Header。Header620:24416新增ShowSearch，首页实例1895:45563与预览3280:64604读回为false；快捷导航读回仅历史画布与积分超市。截图发现变量透明度问题后修正并重看，最终采用弱描边及选中底色，均为可编辑层与实例。

11个SVG由设计上下文提供的链接下载至public/icons/navigation，不引用临时Figma资源。新建图标导出的是12/12.2667px内部图形，在16px外框内按设计居中；其余保持原始尺寸。CSS mask继承语义色。没有复制竞品余额、促销、品牌或用户头像。资源摘要位于仓库父目录output/navigation-alignment-2026-10-30/asset-digests.json。

## 实现与实际页面

复用ProductSider、ProductAccountActions，增加ProductNavigationIcon与ProductUtilityNavigation；没有新增服务请求。首页卸载搜索组件及快捷键监听；其他页面遵循其搜索合同。历史画布沿/projects，积分超市沿/billing/recharge，开会员沿/billing/membership。

真实localhost:1122登录态，无API mock：

- 主导航216×36，图标20×20，首行x12/y86；账号组199×36、右24/y22。
- 折叠侧栏64px，四个主图标仍20px可见，展开恢复画布历史；无横向页面溢出。
- 账号菜单可展开。测试检查跨路由账号节点及积分弹层保持。
- 创作、探索、资产、主体复用同一账号组。修正资产旧padding及探索网格空列后，探索与资产也为右24/y22；页面工具保留自己的左边距。
- 390×844：通知/积分/头像44×44；抽屉主链接和关闭按钮至少44高；scrollWidth=390。320宽隐藏顶栏积分，账号菜单仍保留积分入口；scrollWidth=320。
- 最新首页确认搜索=false、企业服务=false；快捷导航只有历史画布与积分超市。最后恢复正常视口与创作首页。

截图在仓库父目录output/navigation-alignment-2026-10-30：desktop-final.jpg、mobile-header.jpg、mobile-sidebar.jpg、sidebar-collapsed.jpg。菜单截图仅为本地验证，不作为最终预览。

## 工程检查与边界

定向五文件38项通过：basic-layout、recent-canvases、account-menu、home-page、theme-policy。TypeScript、范围ESLint及TS/TSX格式检查通过。早期旧会员文案、首页搜索预期断言失败已按用户确认更新；保留原失败日志，不将初次失败记为通过。最后CSS修正再次量取实际DOM。

React审查：复用现有Client边界；静态图标没有Effect、请求或新依赖；首页不挂载搜索，从根源释放监听与弹层；工具为语义链接，图标aria-hidden，手机触达与焦点沿既有组件。

未运行本轮完整生产构建、全仓测试、后端数据库回归或真实生成。本组不宣告七类创作及全站视觉全部完成。本轮未新提交、推送或部署。共享工作区其他修改保留，逐路径清单位于仓库父目录output/navigation-alignment-2026-10-30/worktree-manifest.md。日志/tmp/glowloom-navigation-*.log。
