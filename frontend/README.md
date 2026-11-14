# 织光前端

正式首页为 `/`，页面与状态遵循[视觉规范](../DESIGN.md)和[产品范围](../workspace/content/requirement/01-产品范围.md)。账号目标行为见[用户系统需求](../workspace/content/requirement/02-用户系统需求.md)。

认证使用邮箱和密码调用真实会话接口。不同业务的实现及验收状态以 [PROJECT](../PROJECT.md#1-当前阶段与文件职责)、服务回执和实际验收证据为准。

复用组件位于 `src/components/<feature>/`；页面实现和独有组件位于对应 `src/app/<路由>/_components/`；基础组件使用 `components/ui/`。测试位于 `tests/`，与 `src/` 分开。目录和依赖边界见 [PROJECT](../PROJECT.md#2-可继续使用的工具)。

AI 服务与模型管理的页面流程见[统一界面设计](../workspace/content/design/07-AI平台接入市场调研与架构设计.md#ai-接入统一界面)，实现复用现有 shadcn/ui 组件与请求合同。

## 开发

本地前端、后端和协作运行时统一由仓库根目录的 `docker-compose.yml` 启动；本目录不单独启动 Next.js。请按[仓库启动说明](../README.md)操作。Compose 注入 `GLOWLOOM_PUBLIC_ORIGIN` 和 `GLOWLOOM_BACKEND_ORIGIN`，缺少必需地址时服务会明确失败，不会回退到容器自身的回环地址。质量门禁见 [PROJECT](../PROJECT.md#4-质量门禁)。

交互控件、卡片、表单与状态展示统一复用官方 shadcn/ui Radix Nova，图标使用 Lucide。HTML 仅用于框架根节点、必要语义布局、文本与媒体，不自行实现已有控件。字体直接使用 Vercel 官方 `geist` 包，中文回退系统字体；主题沿用现有 neutral 语义颜色，不增加页面专属主题。

产品不提供第三方许可导航或页面；Geist 字体由 `geist` 包提供，其许可原文位于该包的 `LICENSE.txt`。
