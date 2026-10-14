# 织光前端

正式首页为 `/`，页面与状态遵循[视觉规范](../DESIGN.md)和[产品范围](../docs/requirement/00-产品范围.md)。账号目标行为见[用户系统需求](../docs/requirement/01-用户系统需求.md)。

认证已经调用真实会话接口，当前仍使用登录名。邮箱验证与恢复尚待实现；账号资料、头像和其他设备管理未开放，主题偏好目前仅本地保存。不同业务的实际可用程度以其服务回执和验收证据为准。

复用组件位于 `src/components/<feature>/`；页面实现、独有组件和交互测试位于对应 `src/app/<路由>/_components/`；基础组件使用 `components/ui/`。目录和依赖边界见 [PROJECT](../PROJECT.md#2-可继续使用的工具)。

## 开发

本目录执行 `pnpm install --frozen-lockfile`，再执行 `pnpm exec next dev --hostname 127.0.0.1 --port 3141`。Docker 访问地址以 Compose 配置及实际启动状态为准。质量门禁见 [PROJECT](../PROJECT.md#4-质量门禁)。

第三方许可位于 `public/licenses/`，版权来源保留不变。
