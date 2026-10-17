# 织光前端

正式首页为 `/`，页面与状态遵循[视觉规范](../DESIGN.md)和[产品范围](../docs/requirement/00-产品范围.md)。账号目标行为见[用户系统需求](../docs/requirement/01-用户系统需求.md)。

认证使用邮箱和密码调用真实会话接口。不同业务的实现及验收状态以 [PROJECT](../PROJECT.md#1-当前阶段与文件职责)、服务回执和实际验收证据为准。

复用组件位于 `src/components/<feature>/`；页面实现、独有组件和交互测试位于对应 `src/app/<路由>/_components/`；基础组件使用 `components/ui/`。目录和依赖边界见 [PROJECT](../PROJECT.md#2-可继续使用的工具)。

## 开发

本目录执行 `pnpm install --frozen-lockfile`，再执行 `pnpm exec next dev --hostname 127.0.0.1 --port 3141`。Docker 访问地址以 Compose 配置及实际启动状态为准。质量门禁见 [PROJECT](../PROJECT.md#4-质量门禁)。

浏览器访问地址必须与后端 `LV_PUBLIC_ORIGIN` 一致。Compose 默认使用 `http://127.0.0.1:3200`；`localhost` 与 `127.0.0.1` 是不同来源，地址不一致的登录及写入请求会被返回 `403 origin_forbidden`。独立启动前端时，相应后端也须配置匹配的来源；不要重写请求 Origin 或关闭校验。

第三方许可位于 `public/licenses/`，版权来源保留不变。
