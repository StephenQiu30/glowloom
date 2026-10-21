# 织光前端

正式首页为 `/`，页面与状态遵循[视觉规范](../DESIGN.md)和[产品范围](../docs/requirement/01-产品范围.md)。账号目标行为见[用户系统需求](../docs/requirement/02-用户系统需求.md)。

认证使用邮箱和密码调用真实会话接口。不同业务的实现及验收状态以 [PROJECT](../PROJECT.md#1-当前阶段与文件职责)、服务回执和实际验收证据为准。

复用组件位于 `src/components/<feature>/`；页面实现和独有组件位于对应 `src/app/<路由>/_components/`；基础组件使用 `components/ui/`。测试位于 `tests/`，与 `src/` 分开。目录和依赖边界见 [PROJECT](../PROJECT.md#2-可继续使用的工具)。

## 开发

本目录执行 `pnpm install --frozen-lockfile`，再执行 `pnpm exec next dev --hostname 127.0.0.1 --port 3141`。Docker 访问地址以 Compose 配置及实际启动状态为准。质量门禁见 [PROJECT](../PROJECT.md#4-质量门禁)。

浏览器访问地址必须与后端 `GLOWLOOM_PUBLIC_ORIGIN` 一致。Compose 默认使用 `http://127.0.0.1:3200`；`localhost` 与 `127.0.0.1` 是不同来源，地址不一致的登录及写入请求会被返回 `403 origin_forbidden`。独立启动前端时，相应后端也须配置匹配的来源；不要重写请求 Origin 或关闭校验。

交互控件、卡片、表单与状态展示统一复用官方 shadcn/ui Radix Nova，图标使用 Lucide。HTML 仅用于框架根节点、必要语义布局、文本与媒体，不自行实现已有控件。字体直接使用 Vercel 官方 `geist` 包，中文回退系统字体；主题沿用现有 neutral 语义颜色，不增加页面专属主题。

产品不提供第三方许可导航或页面；Geist 字体由 `geist` 包提供，其许可原文位于该包的 `LICENSE.txt`。
