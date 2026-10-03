# Lanverse 界面验证

本文件规定界面验证方法与证据边界；视觉规则见 [DESIGN.md](DESIGN.md)，业务验收见 [TST-01](docs/test/01-测试策略.md)、[TST-02](docs/test/02-需求追踪矩阵.md) 与 [TST-03](docs/test/03-AI评测方案.md)。未完成范围持续记录在 [BACKLOG](BACKLOG.md)。

## 验证范围

- 以实际运行的正式项目、画布、媒体、任务和配置入口核验数据；覆盖加载、空、失败、重试、切换项目、刷新恢复与撤权。
- 检查明暗主题、桌面/平板/手机布局和横向溢出；记录 CSS 视口、浏览器与实际环境，图片像素不能反推 CSS 尺寸。
- 键盘覆盖导航、表单错误、菜单、弹窗、抽屉和预览；验证可访问名称、焦点进入/返回与 Escape 关闭语义。
- 付费链检查报价、明确确认、恢复、候选预览和用户采用；技术、真实集成与产品证据分别核验，不能互相替代。

## 自动化门禁

在 `frontend/` 运行：

```bash
pnpm exec eslint .
pnpm exec prettier --check .
pnpm exec next typegen
pnpm exec tsc --noEmit
pnpm exec vitest run
pnpm exec next build
```

按改动补充 Playwright 与真实浏览器验证；API 变化先从运行后端在线 Swagger 重新生成客户端。Vercel React 最佳实践与安装版本的 Next.js 文档审查须覆盖请求瀑布、Server/Client 边界、缓存范围、派生状态及可访问性。

## 证据与未验收条件

截图仅证明捕获时的界面；单元测试、lint、类型检查与构建不证明供应商执行、真实费用、媒体处理或完整产品验收。样例/fixture、隔离基础设施、真实供应商与正式产品命令分别说明输入和条件。中断、Skip、缺少外部条件及未覆盖浏览器或目标设备均保留为未验收，不以历史截图或旧结果替代当前验证。
